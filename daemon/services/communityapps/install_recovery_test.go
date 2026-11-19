package communityapps_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/containerd/errdefs"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"

	"github.com/tinkerologie/unraid-management-agent/daemon/services/communityapps"
	"github.com/tinkerologie/unraid-management-agent/daemon/services/internal/communityappstest"
)

func TestRestartRecoversStartedContainerWithPublishedTemplate(t *testing.T) {
	root := communityappstest.CopyHeimdallHost(t)
	engine := &communityappstest.InstallEngine{
		PostStartInspectError: errors.New("private Docker configuration"),
	}
	before := newInstallService(t, root, engine)
	preview, err := before.Prepare(t.Context(), communityapps.PrepareRequest{
		Reference:   communityappstest.HeimdallReference,
		Environment: map[string]string{"PUID": "private environment value"},
	})
	if err != nil || !preview.Eligible {
		t.Fatalf("prepare: %v (%+v)", err, preview.Issues)
	}
	claim, err := before.Confirm(t.Context(), preview.PlanID, preview.ApprovalToken)
	if err != nil {
		t.Fatal(err)
	}
	waitForDockerStart(t, engine)
	before.Close()

	observed := newInstallService(t, root, engine)
	pending, err := observed.GetAttempt(claim.ID)
	if err != nil || pending.Phase != "starting_container" {
		t.Fatalf("read-only service returned %+v, %v", pending, err)
	}

	engine.PostStartInspectError = nil
	var available atomic.Bool
	attempted := make(chan struct{}, 1)
	after := newRestartService(t, root, engine, func() (communityapps.InstallDocker, error) {
		select {
		case attempted <- struct{}{}:
		default:
		}
		if !available.Load() {
			return nil, errors.New("private Engine configuration")
		}
		return engine, nil
	})
	after.StartRecovery(t.Context())
	after.StartRecovery(t.Context())
	select {
	case <-attempted:
	case <-time.After(5 * time.Second):
		t.Fatal("startup did not attempt recovery")
	}
	during, err := after.Confirm(t.Context(), preview.PlanID, preview.ApprovalToken)
	if err != nil || during.ID != claim.ID || during.State != communityapps.AttemptRunning ||
		during.Problem == nil || during.Problem.Code != "container_start_unverified" {
		t.Fatalf("unavailable Engine did not retain the original pending attempt: %+v, %v", during, err)
	}
	available.Store(true)
	recovered := waitForAttempt(t, after, claim.ID)
	if recovered.State != communityapps.AttemptSucceeded || recovered.ContainerID != communityappstest.ContainerID {
		t.Fatalf("did not recover the installed app: %+v", recovered)
	}
	replayed, err := after.Confirm(t.Context(), preview.PlanID, preview.ApprovalToken)
	if err != nil || !reflect.DeepEqual(recovered, replayed) {
		t.Fatalf("recovered confirmation changed: %+v, %v", replayed, err)
	}
	if creates, containers, starts, _, _ := engine.Observation(); creates != 1 || containers != 1 || len(starts) != 1 {
		t.Fatal("recovery restarted, removed or recreated the installed app")
	}

	if _, err := engine.ContainerRemove(context.Background(), recovered.ContainerID, client.ContainerRemoveOptions{}); err != nil {
		t.Fatal(err)
	}
	restarted := newInstallService(t, root, engine)
	restarted.StartRecovery(t.Context())
	stored, err := restarted.GetAttempt(claim.ID)
	if err != nil || !reflect.DeepEqual(recovered, stored) {
		t.Fatalf("a later container removal changed original receipt: %+v, %v", stored, err)
	}
}

func waitForDockerStart(t *testing.T, engine *communityappstest.InstallEngine) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, _, starts, _, _ := engine.Observation(); len(starts) > 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("Docker start was never issued")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

const (
	recoveryReceiptDir  = "boot/config/plugins/unraid-management-agent/community-app-attempts"
	recoveryTemplateDir = "boot/config/plugins/dockerMan/templates-user"
	recoveryAppdata     = "mnt/cache/application-data/heimdall/keep.txt"
)

type receiptCheckpoint struct {
	Raw              string
	Native           string
	PlanID           string
	Owner            string
	OwnedContainerID string
	Attempt          communityapps.Attempt
}

func captureReceipt(root string, checkpoint *receiptCheckpoint) error {
	files, err := filepath.Glob(filepath.Join(root, recoveryReceiptDir, "*.json"))
	if err != nil || len(files) != 1 {
		return fmt.Errorf("expected one durable receipt, found %d: %v", len(files), err)
	}
	raw, err := os.ReadFile(files[0])
	if err != nil {
		return err
	}
	var wire struct {
		PlanID           string                `json:"plan_id"`
		Owner            string                `json:"owner"`
		OwnedContainerID string                `json:"owned_container_id"`
		Attempt          communityapps.Attempt `json:"attempt"`
	}
	if err := json.Unmarshal(raw, &wire); err != nil {
		return err
	}
	*checkpoint = receiptCheckpoint{
		Raw: string(raw), PlanID: wire.PlanID, Owner: wire.Owner, OwnedContainerID: wire.OwnedContainerID, Attempt: wire.Attempt,
	}
	native, err := os.ReadFile(filepath.Join(root, recoveryTemplateDir, "my-"+wire.Attempt.Configuration.Name+".xml"))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	checkpoint.Native = string(native)
	return nil
}

func newRestartService(t *testing.T, root string, engine *communityappstest.InstallEngine, open func() (communityapps.InstallDocker, error)) *communityapps.Service {
	t.Helper()
	service := communityapps.NewService(communityapps.NewCatalog(os.DirFS(root)), communityapps.ServiceOptions{
		HostRoot:          root,
		OpenDocker:        func() (communityapps.PlanningDocker, error) { return engine, nil },
		OpenInstallDocker: open,
		PortAvailable:     func(context.Context, communityapps.PortBinding) error { return nil },
	})
	t.Cleanup(service.Close)
	return service
}

func (r receiptCheckpoint) restore(t *testing.T, root string, expectedPhase string) {
	t.Helper()
	if r.Attempt.Phase != expectedPhase {
		t.Fatalf("captured phase %q, wanted %q", r.Attempt.Phase, expectedPhase)
	}
	communityappstest.WriteFile(t, root, recoveryReceiptDir+"/"+r.PlanID+".json", r.Raw)
}

func checkpointPublishedInstall(root string, docker *communityappstest.InstallEngine, r *receiptCheckpoint) {
	docker.AfterCreate = func() error {
		docker.BeforeNetwork = func() error {
			if err := captureReceipt(root, r); err != nil {
				return err
			}
			return errors.New("private Engine diagnostic")
		}
		return nil
	}
}

func checkpointCleanupInstall(root string, docker *communityappstest.InstallEngine, r *receiptCheckpoint) {
	docker.BeforeStart = func() error {
		return errdefs.ErrInvalidArgument.WithMessage("bind: address already in use: private Engine diagnostic")
	}
	docker.BeforeRemove = func() error {
		if r.Raw == "" {
			if err := captureReceipt(root, r); err != nil {
				return err
			}
		}
		return errors.New("private Engine diagnostic")
	}
}

func TestRestartReconcilesCheckpointedInstall(t *testing.T) {
	for _, tt := range []struct {
		name            string
		phase           string
		setup           func(string, *communityappstest.InstallEngine, *receiptCheckpoint)
		retainContainer bool
		wantProblem     string
		wantCleanup     string
		stage           string
		editPublished   bool
		legacyReceipt   bool
		wrongID         bool
		wantVerified    bool
	}{
		{
			name:  "lost create response",
			phase: "creating_container",
			setup: func(root string, docker *communityappstest.InstallEngine, r *receiptCheckpoint) {
				docker.AfterCreate = func() error {
					if err := captureReceipt(root, r); err != nil {
						return err
					}
					return errors.New("private Engine diagnostic")
				}
			},
		}, {
			name:  "native template was published before start intent",
			phase: "publishing_template",
			setup: checkpointPublishedInstall,
		}, {
			name:        "unproven staged file remains for review",
			phase:       "publishing_template",
			setup:       checkpointPublishedInstall,
			wantCleanup: "remove_staged_template",
			stage:       "unproven-stage",
		}, {
			name:          "authenticated torn legacy stage is removed",
			phase:         "publishing_template",
			setup:         checkpointPublishedInstall,
			stage:         "authenticated-stage",
			legacyReceipt: true,
		}, {
			name:            "unusable durable Docker ID preserves all resources",
			phase:           "publishing_template",
			setup:           checkpointPublishedInstall,
			retainContainer: true,
			wantProblem:     "install_receipt_unverified",
			wrongID:         true,
		}, {
			name:          "edited published template is preserved",
			phase:         "cleaning_up",
			setup:         checkpointCleanupInstall,
			wantProblem:   "port_conflict",
			wantCleanup:   "remove_template",
			editPublished: true,
		}, {
			name:            "container cleanup fails after restart",
			phase:           "cleaning_up",
			setup:           checkpointCleanupInstall,
			retainContainer: true,
			wantProblem:     "port_conflict",
			wantCleanup:     "remove_container",
			wantVerified:    true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := communityappstest.CopyHeimdallHost(t)
			communityappstest.WriteFile(t, root, recoveryAppdata, "appdata sentinel")
			communityappstest.WriteFile(t, root, recoveryTemplateDir+"/my-unrelated.xml", "<Container><Name>unrelated</Name></Container>")
			engine := &communityappstest.InstallEngine{}
			var checkpoint receiptCheckpoint
			tt.setup(root, engine, &checkpoint)

			// Freeze Docker cleanup to keep its state at the interrupted
			// checkpoint while the original process exits.
			if engine.BeforeRemove == nil {
				engine.BeforeRemove = func() error { return errors.New("Docker unavailable before restart") }
			}
			before := newInstallService(t, root, engine)
			preview, err := before.Prepare(t.Context(), communityapps.PrepareRequest{
				Reference:   communityappstest.HeimdallReference,
				Environment: map[string]string{"PUID": "private environment value"},
			})
			if err != nil || !preview.Eligible {
				t.Fatalf("prepare: %v (%+v)", err, preview.Issues)
			}
			claim, err := before.Confirm(t.Context(), preview.PlanID, preview.ApprovalToken)
			if err != nil {
				t.Fatal(err)
			}
			_ = waitForAttempt(t, before, claim.ID)
			before.Close()
			if tt.legacyReceipt {
				checkpoint.Raw = strings.Replace(checkpoint.Raw, "\"version\":2", "\"version\":1", 1)
				checkpoint.Raw = strings.Replace(checkpoint.Raw, ",\"cleanup_phase\":\"starting_container\"", "", 1)
			}
			if tt.wrongID {
				checkpoint.Raw = strings.Replace(checkpoint.Raw, checkpoint.OwnedContainerID, checkpoint.OwnedContainerID[:12], 1)
			}
			checkpoint.restore(t, root, tt.phase)
			if tt.phase == "creating_container" && checkpoint.OwnedContainerID != "" {
				t.Fatal("fixture unexpectedly persisted create-result ID")
			}
			if !tt.retainContainer {
				engine.BeforeRemove = nil
			}
			stageName := ".uma-ca-" + claim.ID + ".tmp"
			if tt.legacyReceipt {
				stageName = ".uma-ca-" + checkpoint.Owner + ".tmp"
			}
			switch tt.stage {
			case "unproven-stage":
				communityappstest.WriteFile(t, root, recoveryTemplateDir+"/"+stageName, "non-XML sentinel")
			case "authenticated-stage":
				lines := strings.SplitN(checkpoint.Native, "\n", 3)
				if len(lines) != 3 {
					t.Fatal("checkpoint missing published template proof")
				}
				communityappstest.WriteFile(t, root, recoveryTemplateDir+"/"+stageName, lines[0]+"\n"+lines[1]+"\n<Container")
			default:
				if checkpoint.Native != "" {
					if tt.editPublished {
						checkpoint.Native = strings.Replace(checkpoint.Native, "<Shell>sh</Shell>", "<Shell>bash</Shell>", 1)
					}
					communityappstest.WriteFile(t, root, recoveryTemplateDir+"/my-heimdall.xml", checkpoint.Native)
				}
			}

			after := newInstallService(t, root, engine)
			after.StartRecovery(t.Context())
			recovered := waitForAttempt(t, after, claim.ID)
			wantProblem := tt.wantProblem
			if wantProblem == "" {
				wantProblem = "install_interrupted"
			}
			if recovered.State != communityapps.AttemptFailed || recovered.Problem == nil || recovered.Problem.Code != wantProblem {
				t.Fatalf("unexpected recovery: %+v", recovered)
			}
			if tt.wantCleanup != "" {
				details := recovered.CleanupDetails
				if len(details) != 1 || details[0].Code != tt.wantCleanup || details[0].OwnershipVerified != tt.wantVerified {
					t.Fatalf("unexpected cleanup details: %+v", details)
				}
				if tt.retainContainer && details[0].ContainerID != communityappstest.ContainerID {
					t.Fatalf("cleanup did not identify exact Docker ID: %+v", details)
				}
				if tt.stage == "unproven-stage" {
					if details[0].TemplateFile != stageName {
						t.Fatalf("cleanup did not identify retained stage: %+v", details)
					}
					assertRecoveryFile(t, root, recoveryTemplateDir+"/"+stageName, "non-XML sentinel")
				}
			} else if len(recovered.CleanupFailures) != 0 {
				t.Fatalf("cleanup returned unexpected failures: %+v", recovered.CleanupFailures)
			}
			data, err := json.Marshal(recovered)
			if err != nil || bytes.Contains(data, []byte(checkpoint.Owner)) || bytes.Contains(data, []byte("private ")) ||
				bytes.Contains(data, []byte(preview.ApprovalToken)) {
				t.Fatalf("recovered response leaked private data: %v", err)
			}
			var stored receiptCheckpoint
			if err := captureReceipt(root, &stored); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(stored.Raw, "private ") || strings.Contains(stored.Raw, preview.ApprovalToken) {
				t.Fatal("receipt leaked private data")
			}
			creates, containers, starts, _, _ := engine.Observation()
			wantStart := 0
			if tt.phase == "starting_container" || tt.phase == "cleaning_up" {
				wantStart = 1
			}
			if creates != 1 || (containers > 0) != tt.retainContainer || len(starts) != wantStart {
				t.Fatal("recovery changed an unrelated resource or repeated Docker creation/start")
			}
			if tt.editPublished || tt.wrongID {
				assertRecoveryFile(t, root, recoveryTemplateDir+"/my-heimdall.xml", checkpoint.Native)
			} else if _, err := os.Lstat(filepath.Join(root, recoveryTemplateDir, "my-heimdall.xml")); !os.IsNotExist(err) {
				t.Fatalf("owned final template was retained: %v", err)
			}
			if tt.stage == "authenticated-stage" {
				if _, err := os.Lstat(filepath.Join(root, recoveryTemplateDir, stageName)); !os.IsNotExist(err) {
					t.Fatalf("authenticated stage was retained: %v", err)
				}
			}
			for _, req := range engine.RemoveRequests() {
				if req.ID != communityappstest.ContainerID || req.Options.RemoveVolumes || req.Options.Force && !tt.editPublished {
					t.Fatalf("cleanup used unsafe remove options: %+v", req)
				}
			}
			assertRecoveryFile(t, root, recoveryAppdata, "appdata sentinel")
			assertRecoveryFile(t, root, recoveryTemplateDir+"/my-unrelated.xml", "<Container><Name>unrelated</Name></Container>")
		})
	}
}

func assertRecoveryFile(t *testing.T, root, name, expected string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, name))
	if err != nil || string(data) != expected {
		t.Fatalf("file %s is missing or changed: %v", name, err)
	}
}

func TestRecoveryDoesNotInterruptActiveWorkerOrProbeDockerForEarlyPhase(t *testing.T) {
	root := communityappstest.CopyHeimdallHost(t)
	pulling := make(chan struct{})
	docker := &communityappstest.InstallEngine{BeforePull: func(ctx context.Context) error {
		close(pulling)
		<-ctx.Done()
		return ctx.Err()
	}}
	service := newInstallService(t, root, docker)
	preview, err := service.Prepare(t.Context(), communityapps.PrepareRequest{Reference: communityappstest.HeimdallReference})
	if err != nil || !preview.Eligible {
		t.Fatalf("prepare: %v (%+v)", err, preview.Issues)
	}
	first, err := service.Confirm(t.Context(), preview.PlanID, preview.ApprovalToken)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-pulling:
	case <-time.After(5 * time.Second):
		t.Fatal("worker never entered pulling phase")
	}
	var checkpoint receiptCheckpoint
	if err := captureReceipt(root, &checkpoint); err != nil {
		t.Fatal(err)
	}
	service.StartRecovery(t.Context())
	active, err := service.GetAttempt(first.ID)
	if err != nil || active.Phase != "pulling_image" || active.Problem != nil {
		t.Fatalf("active request treated as interrupted: %+v, %v", active, err)
	}
	service.Close()
	checkpoint.restore(t, root, "pulling_image")

	var opened atomic.Bool
	after := newRestartService(t, root, docker, func() (communityapps.InstallDocker, error) {
		opened.Store(true)
		return nil, errors.New("Docker disabled")
	})
	after.StartRecovery(t.Context())
	failed := waitForAttempt(t, after, first.ID)
	if failed.State != communityapps.AttemptFailed || failed.Problem == nil || failed.Problem.Code != "install_interrupted" ||
		opened.Load() {
		t.Fatalf("recovery of a pre-create phase required Docker: %+v", failed)
	}
}

func TestRecoveryReservesUnresolvedNameAndRetries(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		t.Run(fmt.Sprintf("legacy cleanup checkpoint %v", legacy), func(t *testing.T) {
			checkUnresolvedNameRecovery(t, legacy)
		})
	}
}

func checkUnresolvedNameRecovery(t *testing.T, legacy bool) {
	t.Helper()
	root := communityappstest.CopyHeimdallHost(t)
	var createOptions client.ContainerCreateOptions
	issued := make(chan struct{})
	var dispatched atomic.Bool
	engine := &communityappstest.InstallEngine{BeforeCreate: func(opts client.ContainerCreateOptions) error {
		if dispatched.CompareAndSwap(false, true) {
			createOptions = opts
			close(issued)
			return errors.New("create acknowledgement lost before the Engine made it visible")
		}
		return nil
	}}
	before := newInstallService(t, root, engine)
	req := communityapps.PrepareRequest{Reference: communityappstest.HeimdallReference}
	original, err := before.Prepare(t.Context(), req)
	if err != nil || !original.Eligible {
		t.Fatalf("prepare: %v (%+v)", err, original.Issues)
	}
	first, err := before.Confirm(t.Context(), original.PlanID, original.ApprovalToken)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-issued:
	case <-time.After(5 * time.Second):
		t.Fatal("Docker create was never issued")
	}
	before.Close()
	if legacy {
		var saved receiptCheckpoint
		if err := captureReceipt(root, &saved); err != nil {
			t.Fatal(err)
		}
		saved.Raw = strings.Replace(saved.Raw, `"version":2`, `"version":1`, 1)
		saved.Raw = strings.Replace(saved.Raw, `"phase":"creating_container"`, `"phase":"cleaning_up"`, 1)
		saved.restore(t, root, "creating_container")
	}

	var available atomic.Bool
	after := newRestartService(t, root, engine, func() (communityapps.InstallDocker, error) {
		if !available.Load() {
			return nil, errors.New("Docker temporarily disabled")
		}
		return engine, nil
	})
	after.StartRecovery(t.Context())
	next, err := after.Prepare(t.Context(), req)
	if err != nil || !next.Eligible {
		t.Fatalf("read-only preview: %v (%+v)", err, next.Issues)
	}
	if _, err := after.Confirm(t.Context(), next.PlanID, next.ApprovalToken); !errors.Is(err, communityapps.ErrInstallRecovery) {
		t.Fatalf("unresolved name was not reserved: %v", err)
	}
	other, err := after.Prepare(t.Context(), communityapps.PrepareRequest{Reference: communityappstest.HeimdallReference, Name: "another-name"})
	if err != nil || !other.Eligible {
		t.Fatalf("prepare another name: %v (%+v)", err, other.Issues)
	}
	second, err := after.Confirm(t.Context(), other.PlanID, other.ApprovalToken)
	if err != nil {
		t.Fatalf("install of a distinct name was blocked by unresolved attempt: %v", err)
	}
	if outcome := waitForAttempt(t, after, second.ID); outcome.State != communityapps.AttemptFailed {
		t.Fatalf("fixture cannot access Docker: %+v", outcome)
	}
	available.Store(true)
	time.Sleep(1500 * time.Millisecond)
	pending, err := after.GetAttempt(first.ID)
	if err != nil || pending.State != communityapps.AttemptRunning || pending.Problem == nil ||
		pending.Problem.Code != "container_create_unverified" {
		t.Fatalf("Docker absence prematurely settled a lost response: %+v, %v", pending, err)
	}
	replayed, err := after.Confirm(t.Context(), original.PlanID, original.ApprovalToken)
	if err != nil || replayed.ID != first.ID {
		t.Fatalf("pending attempt could not be replayed: %+v, %v", replayed, err)
	}
	if _, err := engine.ContainerCreate(t.Context(), createOptions); err != nil {
		t.Fatalf("fixture could not make the original create visible: %v", err)
	}
	failed := waitForAttempt(t, after, first.ID)
	if failed.State != communityapps.AttemptFailed || failed.Problem == nil ||
		failed.Problem.Code != "container_create_failed" || len(failed.CleanupFailures) != 0 {
		t.Fatalf("late create did not settle and clean up: %+v", failed)
	}
	if removes := engine.RemoveRequests(); len(removes) != 1 || removes[0].ID != communityappstest.ContainerID || removes[0].Options.Force {
		t.Fatalf("recovery did not remove only the stopped late container: %+v", removes)
	}
	claim, err := after.Confirm(t.Context(), next.PlanID, next.ApprovalToken)
	if err != nil || claim.ID == first.ID {
		t.Fatalf("approved new plan could not retry by the original name: %+v, %v", claim, err)
	}
	if installed := waitForAttempt(t, after, claim.ID); installed.State != communityapps.AttemptSucceeded {
		t.Fatalf("fresh plan did not install: %+v", installed)
	}
}

func TestRecoveryRetriesTerminalWriteWithoutReprobingDocker(t *testing.T) {
	root := communityappstest.CopyHeimdallHost(t)
	store := filepath.Join(root, recoveryReceiptDir)
	holding := store + "-held"
	docker := &communityappstest.InstallEngine{AfterStart: func() error {
		// Simulate a terminal write interruption without hiding the actual
		// completed Docker and native template proofs from the worker.
		if err := os.Rename(store, holding); err != nil {
			return err
		}
		return os.WriteFile(store, []byte("flash temporarily unavailable"), 0600)
	}}
	service := newInstallService(t, root, docker)
	service.StartRecovery(t.Context())
	preview, err := service.Prepare(t.Context(), communityapps.PrepareRequest{Reference: communityappstest.HeimdallReference})
	if err != nil || !preview.Eligible {
		t.Fatalf("prepare: %v (%+v)", err, preview.Issues)
	}
	claim, err := service.Confirm(t.Context(), preview.PlanID, preview.ApprovalToken)
	if err != nil {
		t.Fatal(err)
	}
	for deadline := time.Now().Add(5 * time.Second); ; {
		current, err := service.GetAttempt(claim.ID)
		if err != nil {
			t.Fatal(err)
		}
		if current.Problem != nil && current.Problem.Code == "install_recovery_pending" {
			if current.State != communityapps.AttemptRunning || current.Phase != "starting_container" {
				t.Fatalf("uncommitted result leaked: %+v", current)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("attempt never waited for the terminal write: %+v", current)
		}
		time.Sleep(5 * time.Millisecond)
	}
	if _, err := docker.ContainerRemove(t.Context(), communityappstest.ContainerID, client.ContainerRemoveOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(store); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(holding, store); err != nil {
		t.Fatal(err)
	}
	got := waitForAttempt(t, service, claim.ID)
	if got.State != communityapps.AttemptSucceeded || got.ContainerID != communityappstest.ContainerID {
		t.Fatalf("storage retry reevaluated the historical outcome: %+v", got)
	}
}

func TestRestartPreservesReplacementAndNativeTemplate(t *testing.T) {
	for _, phase := range []string{"creating_container", "starting_container"} {
		t.Run(phase, func(t *testing.T) {
			root := communityappstest.CopyHeimdallHost(t)
			engine := &communityappstest.InstallEngine{}
			var checkpoint receiptCheckpoint
			if phase == "creating_container" {
				engine.AfterCreate = func() error {
					if err := captureReceipt(root, &checkpoint); err != nil {
						return err
					}
					return errors.New("create response was lost")
				}
				engine.BeforeRemove = func() error { return errors.New("removal unavailable") }
			} else {
				engine.BeforeStart = func() error { return captureReceipt(root, &checkpoint) }
				engine.PostStartInspectError = errors.New("start response could not be verified")
			}
			before := newInstallService(t, root, engine)
			preview, err := before.Prepare(t.Context(), communityapps.PrepareRequest{Reference: communityappstest.HeimdallReference})
			if err != nil || !preview.Eligible {
				t.Fatalf("prepare: %v (%+v)", err, preview.Issues)
			}
			claim, err := before.Confirm(t.Context(), preview.PlanID, preview.ApprovalToken)
			if err != nil {
				t.Fatal(err)
			}
			if phase == "creating_container" {
				_ = waitForAttempt(t, before, claim.ID)
			} else {
				waitForDockerStart(t, engine)
			}
			before.Close()
			checkpoint.restore(t, root, phase)
			engine.BeforeRemove = nil
			engine.PostStartInspectError = nil
			replacement := strings.Repeat("e", 64)
			engine.ReplaceForUpdate(t, replacement)
			beforeRemovals := len(engine.RemoveRequests())
			after := newInstallService(t, root, engine)
			after.StartRecovery(t.Context())
			outcome := waitForAttempt(t, after, claim.ID)
			var wantCleanup []string
			if phase == "creating_container" {
				wantCleanup = []string{"container_identity_unverified"}
			}
			if outcome.State != communityapps.AttemptFailed || outcome.Problem == nil || outcome.Problem.Code != "install_interrupted" ||
				!reflect.DeepEqual(outcome.CleanupFailures, wantCleanup) {
				t.Fatalf("replacement changed historical recovery: %+v", outcome)
			}
			if len(engine.RemoveRequests()) != beforeRemovals {
				t.Fatal("recovery attempted to remove a replacement container")
			}
			if _, err := engine.ContainerInspect(t.Context(), replacement, client.ContainerInspectOptions{}); err != nil {
				t.Fatalf("replacement did not survive: %v", err)
			}
			if phase == "starting_container" {
				assertRecoveryFile(t, root, recoveryTemplateDir+"/my-heimdall.xml", checkpoint.Native)
			}
		})
	}
}

func TestRecoveryPreservesNativeTemplateUntilProofIsReadable(t *testing.T) {
	for _, fault := range []string{"malformed Docker start", "symlinked templates directory"} {
		t.Run(fault, func(t *testing.T) {
			root := communityappstest.CopyHeimdallHost(t)
			engine := &communityappstest.InstallEngine{
				PostStartInspectError: errors.New("inspect unavailable in previous daemon"),
			}
			prior := newInstallService(t, root, engine)
			preview, err := prior.Prepare(t.Context(), communityapps.PrepareRequest{Reference: communityappstest.HeimdallReference})
			if err != nil || !preview.Eligible {
				t.Fatalf("prepare: %v (%+v)", err, preview.Issues)
			}
			claim, err := prior.Confirm(t.Context(), preview.PlanID, preview.ApprovalToken)
			if err != nil {
				t.Fatal(err)
			}
			waitForDockerStart(t, engine)
			prior.Close()
			engine.PostStartInspectError = nil

			templateDir := filepath.Join(root, recoveryTemplateDir)
			safeDir := templateDir + "-real"
			restore := func() {}
			inspected := make(chan struct{}, 1)
			if fault == "symlinked templates directory" {
				if err := os.Rename(templateDir, safeDir); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(safeDir, templateDir); err != nil {
					t.Fatal(err)
				}
				restore = func() {
					t.Helper()
					if err := os.Remove(templateDir); err != nil {
						t.Fatal(err)
					}
					if err := os.Rename(safeDir, templateDir); err != nil {
						t.Fatal(err)
					}
				}
				engine.BeforeContainerInspect = func(_ string, _ *container.InspectResponse) error {
					select {
					case inspected <- struct{}{}:
					default:
					}
					return nil
				}
			} else {
				var known atomic.Bool
				restore = func() { known.Store(true) }
				engine.BeforeContainerInspect = func(_ string, c *container.InspectResponse) error {
					if !known.Load() {
						c.State.StartedAt = "invalid time"
					}
					select {
					case inspected <- struct{}{}:
					default:
					}
					return nil
				}
			}
			after := newInstallService(t, root, engine)
			after.StartRecovery(t.Context())
			select {
			case <-inspected:
			case <-time.After(5 * time.Second):
				t.Fatal("startup never inspected original Docker ID")
			}
			// Observe across the grace period in which a definite negative
			// would have resulted in cleanup.
			time.Sleep(400 * time.Millisecond)
			pending, err := after.GetAttempt(claim.ID)
			if err != nil || pending.State != communityapps.AttemptRunning || len(engine.RemoveRequests()) != 0 {
				t.Fatalf("unknown proof caused destructive cleanup: %+v, %v", pending, err)
			}
			restore()
			if result := waitForAttempt(t, after, claim.ID); result.State != communityapps.AttemptSucceeded {
				t.Fatalf("restored proof was not recovered: %+v", result)
			}
		})
	}
}

func checkpointUnverifiedStart(t *testing.T, engine *communityappstest.InstallEngine) (string, communityapps.Attempt) {
	t.Helper()
	root := communityappstest.CopyHeimdallHost(t)
	engine.PostStartInspectError = errors.New("unverifiable start in previous daemon")
	old := newInstallService(t, root, engine)
	preview, err := old.Prepare(t.Context(), communityapps.PrepareRequest{Reference: communityappstest.HeimdallReference})
	if err != nil || !preview.Eligible {
		t.Fatalf("prepare: %v (%+v)", err, preview.Issues)
	}
	claim, err := old.Confirm(t.Context(), preview.PlanID, preview.ApprovalToken)
	if err != nil {
		t.Fatal(err)
	}
	waitForDockerStart(t, engine)
	old.Close()
	engine.PostStartInspectError = nil
	return root, claim
}

func TestRecoveryKeepsPendingStartUntilDockerReportsFastExit(t *testing.T) {
	engine := &communityappstest.InstallEngine{
		NoStartEffect: true,
		AfterStart:    func() error { return errors.New("lost Start response") },
	}
	root, claim := checkpointUnverifiedStart(t, engine)
	thirdMiss := make(chan struct{})
	var observations int
	engine.BeforeContainerInspect = func(key string, c *container.InspectResponse) error {
		if key == communityappstest.ContainerID && c != nil {
			observations++
			if observations == 3 {
				close(thirdMiss)
			}
		}
		return nil
	}
	after := newInstallService(t, root, engine)
	after.StartRecovery(t.Context())
	select {
	case <-thirdMiss:
	case <-time.After(8 * time.Second):
		t.Fatal("recovery did not keep observing the lost Start")
	}

	pending, err := after.GetAttempt(claim.ID)
	if err != nil || pending.State != communityapps.AttemptRunning || pending.Problem == nil ||
		pending.Problem.Code != "container_start_unverified" || len(engine.RemoveRequests()) != 0 {
		t.Fatalf("recovery attempted cleanup while Docker Start could still finish: %+v, %v", pending, err)
	}

	// A late start followed by an immediate exit can be removed without Force.
	// Model the original request taking effect without issuing a new request.
	engine.CompletePendingStart(t)
	fresh := newInstallService(t, root, engine)
	after.Close()
	fresh.StartRecovery(t.Context())
	exact := waitForAttempt(t, fresh, claim.ID)
	if exact.State != communityapps.AttemptSucceeded || exact.ContainerID != communityappstest.ContainerID ||
		exact.Problem != nil || len(engine.RemoveRequests()) != 0 {
		t.Fatalf("late verified start was not preserved: %+v", exact)
	}
	if creates, present, starts, _, _ := engine.Observation(); creates != 1 || present != 1 || len(starts) != 1 {
		t.Fatal("recovery replayed Docker commands or removed the completed install")
	}
	_ = readNativeInstall(t, root, "heimdall")
}

func TestRecoveryProtectsPendingStartWhenDaemonStops(t *testing.T) {
	engine := &communityappstest.InstallEngine{NoStartEffect: true}
	root, claim := checkpointUnverifiedStart(t, engine)
	observed := make(chan struct{})
	var first sync.Once
	engine.BeforeContainerInspect = func(_ string, _ *container.InspectResponse) error {
		first.Do(func() { close(observed) })
		return nil
	}
	after := newInstallService(t, root, engine)
	after.StartRecovery(t.Context())
	select {
	case <-observed:
	case <-time.After(5 * time.Second):
		t.Fatal("first observation never ran")
	}
	after.Close()
	attempt, err := after.GetAttempt(claim.ID)
	if err != nil || attempt.State != communityapps.AttemptRunning || attempt.Phase != "starting_container" ||
		len(engine.RemoveRequests()) != 0 {
		t.Fatalf("shutdown converted incomplete proof to cleanup: %+v, %v", attempt, err)
	}
}

func TestRecoveryPreservesStartedContainerAfterNativeTemplateChange(t *testing.T) {
	for _, change := range []string{"edited", "replaced", "missing"} {
		t.Run(change, func(t *testing.T) {
			engine := &communityappstest.InstallEngine{}
			root, claim := checkpointUnverifiedStart(t, engine)
			native := filepath.Join(root, recoveryTemplateDir, "my-heimdall.xml")
			var expected string
			switch change {
			case "edited":
				content, err := os.ReadFile(native)
				if err != nil {
					t.Fatal(err)
				}
				expected = strings.Replace(string(content), "<Shell>sh</Shell>", "<Shell>bash</Shell>", 1)
				if expected == string(content) {
					t.Fatal("fixture native template did not contain the editable shell")
				}
			case "replaced":
				expected = "<Container><Name>heimdall</Name><Shell>bash</Shell></Container>"
			case "missing":
				if err := os.Remove(native); err != nil {
					t.Fatal(err)
				}
			}
			if expected != "" {
				if err := os.WriteFile(native, []byte(expected), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			communityappstest.WriteFile(t, root, recoveryAppdata, "protected appdata sentinel")
			after := newInstallService(t, root, engine)
			after.StartRecovery(t.Context())
			outcome := waitForAttempt(t, after, claim.ID)
			if outcome.ContainerID != communityappstest.ContainerID || len(outcome.CleanupFailures) != 0 ||
				len(engine.RemoveRequests()) != 0 {
				t.Fatalf("recovery did not preserve the original install: %+v", outcome)
			}
			if expected == "" {
				if outcome.State != communityapps.AttemptFailed || outcome.Problem == nil || outcome.Problem.Code != "started_template_missing" {
					t.Fatalf("recovery did not report the missing native template: %+v", outcome)
				}
			} else {
				if outcome.State != communityapps.AttemptSucceeded || outcome.Problem != nil {
					t.Fatalf("native edit changed the completed install outcome: %+v", outcome)
				}
				assertRecoveryFile(t, root, recoveryTemplateDir+"/my-heimdall.xml", expected)
			}
			assertRecoveryFile(t, root, recoveryAppdata, "protected appdata sentinel")
		})
	}
}

func TestRecoveryPreservesRenamedOrAmbiguousLostCreate(t *testing.T) {
	for _, duplicate := range []bool{false, true} {
		t.Run(fmt.Sprintf("duplicate %v", duplicate), func(t *testing.T) {
			root := communityappstest.CopyHeimdallHost(t)
			var checkpoint receiptCheckpoint
			captured := make(chan error, 1)
			engine := &communityappstest.InstallEngine{}
			engine.AfterCreate = func() error {
				engine.BeforeList = func() error { return errors.New("Engine went offline before the ID was recorded") }
				captured <- captureReceipt(root, &checkpoint)
				return errors.New("lost create response")
			}
			before := newInstallService(t, root, engine)
			preview, err := before.Prepare(t.Context(), communityapps.PrepareRequest{Reference: communityappstest.HeimdallReference})
			if err != nil || !preview.Eligible {
				t.Fatalf("prepare: %v (%+v)", err, preview.Issues)
			}
			claim, err := before.Confirm(t.Context(), preview.PlanID, preview.ApprovalToken)
			if err != nil {
				t.Fatal(err)
			}
			select {
			case err := <-captured:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("Docker create was never issued")
			}
			before.Close()
			attempt, err := before.GetAttempt(claim.ID)
			if err != nil || attempt.Phase != "creating_container" || checkpoint.OwnedContainerID != "" {
				t.Fatalf("temporarily unavailable Engine lost create intent: %+v, %v", attempt, err)
			}
			engine.BeforeList = nil

			if duplicate {
				engine.Containers = []container.Summary{{
					ID: strings.Repeat("f", 64), Names: []string{"/unrelated"},
					Labels: map[string]string{"net.unraid.management-agent.ca-install": checkpoint.Owner},
				}}
			} else {
				engine.BeforeContainerInspect = func(_ string, c *container.InspectResponse) error {
					if c != nil {
						c.Name = "/operator-renamed"
					}
					return nil
				}
			}
			after := newInstallService(t, root, engine)
			after.StartRecovery(t.Context())
			settled := waitForAttempt(t, after, claim.ID)
			if settled.State != communityapps.AttemptFailed || len(settled.CleanupDetails) != 1 ||
				settled.CleanupDetails[0].Code != "container_identity_unverified" ||
				settled.CleanupDetails[0].OwnershipVerified || len(engine.RemoveRequests()) != 0 {
				t.Fatalf("recovery touched an ambiguous lost-create resource: %+v", settled)
			}
		})
	}
}

func TestRecoveryRequiresDurableCleanupAndFullIDBeforeRemoval(t *testing.T) {
	root := communityappstest.CopyHeimdallHost(t)
	store := filepath.Join(root, recoveryReceiptDir)
	holding := store + "-held"
	blockStorage := func() error {
		if err := os.Rename(store, holding); err != nil {
			return err
		}
		return os.WriteFile(store, []byte("flash temporarily unavailable"), 0600)
	}
	restoreStorage := func() {
		t.Helper()
		if err := os.Remove(store); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(holding, store); err != nil {
			t.Fatal(err)
		}
	}
	duringCreate := make(chan error, 1)
	engine := &communityappstest.InstallEngine{AfterCreate: func() error {
		duringCreate <- blockStorage()
		return nil
	}}
	old := newInstallService(t, root, engine)
	preview, err := old.Prepare(t.Context(), communityapps.PrepareRequest{Reference: communityappstest.HeimdallReference})
	if err != nil || !preview.Eligible {
		t.Fatalf("prepare: %v (%+v)", err, preview.Issues)
	}
	claim, err := old.Confirm(t.Context(), preview.PlanID, preview.ApprovalToken)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-duringCreate:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Docker create was never issued")
	}
	old.Close()
	if len(engine.RemoveRequests()) != 0 {
		t.Fatal("install removed a container before persisting cleanup intent")
	}
	restoreStorage()

	var observed int
	duringCleanup := make(chan error, 1)
	engine.BeforeContainerInspect = func(_ string, _ *container.InspectResponse) error {
		observed++
		if observed == 1 {
			duringCleanup <- blockStorage()
		}
		return nil
	}
	after := newInstallService(t, root, engine)
	after.StartRecovery(t.Context())
	select {
	case err := <-duringCleanup:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cleanup never tried to find the full container ID")
	}
	// Recovery records the exact ID before leaving create intent.
	// Refuse cleanup while that progress record cannot be saved.
	time.Sleep(100 * time.Millisecond)
	pending, err := after.GetAttempt(claim.ID)
	if err != nil || pending.Phase != "creating_container" || len(engine.RemoveRequests()) != 0 {
		t.Fatalf("cleanup continued without durable exact ID: %+v, %v", pending, err)
	}
	restoreStorage()
	settled := waitForAttempt(t, after, claim.ID)
	calls := engine.RemoveRequests()
	if settled.State != communityapps.AttemptFailed || len(settled.CleanupFailures) != 0 ||
		len(calls) != 1 || calls[0].ID != communityappstest.ContainerID || calls[0].Options.RemoveVolumes {
		t.Fatalf("cleanup failed to resume by exact durable ID: %+v (%+v)", settled, calls)
	}
}

func TestRecoveryWaitsForReadableReceipt(t *testing.T) {
	engine := &communityappstest.InstallEngine{}
	root, claim := checkpointUnverifiedStart(t, engine)
	var saved receiptCheckpoint
	if err := captureReceipt(root, &saved); err != nil {
		t.Fatal(err)
	}
	path := recoveryReceiptDir + "/" + saved.PlanID + ".json"
	communityappstest.WriteFile(t, root, path, saved.Raw+"true")
	after := newInstallService(t, root, engine)
	after.StartRecovery(t.Context())
	if _, err := after.GetAttempt(claim.ID); !errors.Is(err, communityapps.ErrAttemptStorage) {
		t.Fatalf("invalid receipt was accepted: %v", err)
	}
	saved.restore(t, root, "starting_container")
	if attempt := waitForAttempt(t, after, claim.ID); attempt.State != communityapps.AttemptSucceeded ||
		len(engine.RemoveRequests()) != 0 {
		t.Fatalf("fixing durable state did not recover the original install: %+v", attempt)
	}
}

func TestRecoveryResumesIfDockerDisappearsAfterContainerRemoval(t *testing.T) {
	engine := &communityappstest.InstallEngine{
		BeforeStart: func() error {
			return errdefs.ErrInvalidArgument.WithMessage("start fixture was rejected")
		},
	}
	root, claim := checkpointUnverifiedStart(t, engine)
	unavailable := make(chan struct{})
	var resolved atomic.Bool
	var notified sync.Once
	engine.BeforeContainerInspect = func(key string, c *container.InspectResponse) error {
		if !resolved.Load() && c == nil && key == "heimdall" {
			notified.Do(func() { close(unavailable) })
			return errors.New("temporary Docker failure during template safety check")
		}
		return nil
	}
	after := newInstallService(t, root, engine)
	after.StartRecovery(t.Context())
	select {
	case <-unavailable:
	case <-time.After(5 * time.Second):
		t.Fatal("recovery did not check the Docker name after cleanup")
	}
	if pending, err := after.GetAttempt(claim.ID); err != nil || pending.Phase != "cleaning_up" {
		t.Fatalf("recovery did not retain durable cleanup intent: %+v, %v", pending, err)
	}
	_ = readNativeInstall(t, root, "heimdall")
	resolved.Store(true)
	if outcome := waitForAttempt(t, after, claim.ID); outcome.State != communityapps.AttemptFailed ||
		len(outcome.CleanupFailures) != 0 || len(engine.RemoveRequests()) != 1 {
		t.Fatalf("recovery did not resume idempotent template cleanup: %+v", outcome)
	}
}
