package communityapps_test

import (
	"errors"
	"testing"
	"time"

	"github.com/moby/moby/client"

	"github.com/tinkerologie/unraid-management-agent/daemon/services/communityapps"
	"github.com/tinkerologie/unraid-management-agent/daemon/services/internal/communityappstest"
)

func TestRecoverySettlesUndispatchedOperationsAfterHostBoot(t *testing.T) {
	const bootIDPath = "proc/sys/kernel/random/boot_id"
	for _, scenario := range []struct {
		name, operation string
		lateCreate      bool
	}{
		{name: "absent create", operation: "create"},
		{name: "retained late create", operation: "create", lateCreate: true},
		{name: "unverified start", operation: "start"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			root := communityappstest.CopyHeimdallHost(t)
			communityappstest.WriteFile(t, root, bootIDPath, "11111111-1111-1111-1111-111111111111\n")
			dispatched := make(chan struct{})
			engine := &communityappstest.InstallEngine{}
			var delayedCreate client.ContainerCreateOptions
			unconfirmed := func() error {
				close(dispatched)
				return errors.New("transport ended without an Engine operation")
			}
			if scenario.operation == "create" {
				engine.BeforeCreate = func(options client.ContainerCreateOptions) error {
					delayedCreate = options
					return unconfirmed()
				}
			} else {
				engine.NoStartEffect = true
				engine.AfterStart = unconfirmed
			}
			original := newInstallService(t, root, engine)
			preview, err := original.Prepare(t.Context(), communityapps.PrepareRequest{Reference: communityappstest.HeimdallReference})
			if err != nil || !preview.Eligible {
				t.Fatalf("prepare returned %+v, %v", preview, err)
			}
			claim, err := original.Confirm(t.Context(), preview.PlanID, preview.ApprovalToken)
			if err != nil {
				t.Fatal(err)
			}
			select {
			case <-dispatched:
			case <-time.After(5 * time.Second):
				t.Fatalf("unconfirmed %s was never dispatched", scenario.operation)
			}
			original.Close()

			sameBoot := newInstallService(t, root, engine)
			sameBoot.StartRecovery(t.Context())
			pending, err := sameBoot.GetAttempt(claim.ID)
			if err != nil || pending.State != communityapps.AttemptRunning || pending.Problem == nil ||
				pending.Problem.Code != "container_"+scenario.operation+"_unverified" {
				t.Fatalf("daemon restart lost unconfirmed %s: %+v, %v", scenario.operation, pending, err)
			}
			sameBoot.Close()
			if len(engine.RemoveRequests()) != 0 {
				t.Fatal("recovery removed a container while the old request could still complete")
			}
			if scenario.lateCreate {
				// The original Engine request took effect after the client lost
				// its response, leaving a stopped container across the boot.
				engine.BeforeCreate = nil
				if _, err := engine.ContainerCreate(t.Context(), delayedCreate); err != nil {
					t.Fatal(err)
				}
			}

			communityappstest.WriteFile(t, root, bootIDPath, "22222222-2222-2222-2222-222222222222\n")
			rebooted := newInstallService(t, root, engine)
			rebooted.StartRecovery(t.Context())
			done := waitForAttempt(t, rebooted, claim.ID)
			if done.State != communityapps.AttemptFailed || done.Problem == nil || done.Problem.Code != "install_interrupted" {
				t.Fatalf("boot proof did not release %s attempt: %+v", scenario.operation, done)
			}
			creates, present, starts, _, _ := engine.Observation()
			expectedStarts := 0
			if scenario.operation == "start" {
				expectedStarts = 1
			}
			expectedPresent := expectedStarts
			if scenario.lateCreate {
				expectedPresent = 1
			}
			if creates != expectedPresent || present != expectedPresent || len(starts) != expectedStarts || len(engine.RemoveRequests()) != 0 {
				t.Fatal("boot recovery replayed a Docker operation or removed an existing container")
			}
			name := "heimdall"
			if scenario.operation == "start" {
				_ = readNativeInstall(t, root, name)
			}
			if expectedPresent != 0 {
				if done.ContainerID != communityappstest.ContainerID {
					t.Fatal("interrupted result did not identify the retained container")
				}
				name = "heimdall-next"
			}
			if replay, err := rebooted.Confirm(t.Context(), preview.PlanID, preview.ApprovalToken); err != nil ||
				replay.ID != done.ID || replay.State != communityapps.AttemptFailed || replay.ContainerID != done.ContainerID {
				t.Fatalf("exact replay lost the durable boot outcome: %+v, %v", replay, err)
			}
			if next, err := rebooted.Prepare(t.Context(), communityapps.PrepareRequest{Reference: communityappstest.HeimdallReference, Name: name}); err != nil || !next.Eligible {
				t.Fatalf("drained attempt did not release the name: %+v, %v", next, err)
			}
		})
	}
}
