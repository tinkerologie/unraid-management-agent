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
	for _, operation := range []string{"create", "start"} {
		t.Run(operation, func(t *testing.T) {
			root := communityappstest.CopyHeimdallHost(t)
			communityappstest.WriteFile(t, root, bootIDPath, "11111111-1111-1111-1111-111111111111\n")
			dispatched := make(chan struct{})
			engine := &communityappstest.InstallEngine{}
			unconfirmed := func() error {
				close(dispatched)
				return errors.New("transport ended without an Engine operation")
			}
			if operation == "create" {
				engine.BeforeCreate = func(client.ContainerCreateOptions) error { return unconfirmed() }
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
				t.Fatalf("unconfirmed %s was never dispatched", operation)
			}
			original.Close()

			sameBoot := newInstallService(t, root, engine)
			sameBoot.StartRecovery(t.Context())
			pending, err := sameBoot.GetAttempt(claim.ID)
			if err != nil || pending.State != communityapps.AttemptRunning || pending.Problem == nil ||
				pending.Problem.Code != "container_"+operation+"_unverified" {
				t.Fatalf("daemon restart lost unconfirmed %s: %+v, %v", operation, pending, err)
			}
			sameBoot.Close()
			if len(engine.RemoveRequests()) != 0 {
				t.Fatal("recovery removed a container while the old request could still complete")
			}

			communityappstest.WriteFile(t, root, bootIDPath, "22222222-2222-2222-2222-222222222222\n")
			rebooted := newInstallService(t, root, engine)
			rebooted.StartRecovery(t.Context())
			done := waitForAttempt(t, rebooted, claim.ID)
			if done.State != communityapps.AttemptFailed || done.Problem == nil ||
				(done.Problem.Code != "container_"+operation+"_failed" && done.Problem.Code != "install_interrupted") {
				t.Fatalf("boot proof did not release %s attempt: %+v", operation, done)
			}
			creates, present, starts, _, _ := engine.Observation()
			expectedStarts := 0
			if operation == "start" {
				expectedStarts = 1
			}
			if creates > 1 || present != 0 || len(starts) != expectedStarts {
				t.Fatal("boot recovery replayed a Docker operation or left the never-started fixture")
			}
			for _, removal := range engine.RemoveRequests() {
				if removal.Options.Force || removal.Options.RemoveVolumes {
					t.Fatal("boot recovery requested force or volume removal")
				}
			}
			if next, err := rebooted.Prepare(t.Context(), communityapps.PrepareRequest{Reference: communityappstest.HeimdallReference}); err != nil || !next.Eligible {
				t.Fatalf("drained attempt did not release the name: %+v, %v", next, err)
			}
		})
	}
}
