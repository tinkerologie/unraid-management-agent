package communityapps

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/moby/moby/api/types/container"

	"github.com/tinkerologie/unraid-management-agent/daemon/services/internal/communityappstest"
)

func newWorkerFixture(t *testing.T) (*Service, *communityappstest.InstallEngine, Preview) {
	t.Helper()
	root := communityappstest.CopyHeimdallHost(t)
	engine := &communityappstest.InstallEngine{}
	service := NewService(NewCatalog(os.DirFS(root)), ServiceOptions{
		HostRoot:          root,
		OpenDocker:        func() (PlanningDocker, error) { return engine, nil },
		OpenInstallDocker: func() (InstallDocker, error) { return engine, nil },
		PortAvailable:     func(context.Context, PortBinding) error { return nil },
	})
	t.Cleanup(service.Close)
	preview, err := service.Prepare(t.Context(), PrepareRequest{Reference: communityappstest.HeimdallReference})
	if err != nil || !preview.Eligible {
		t.Fatalf("worker fixture prepare: %v, %+v", err, preview.Issues)
	}
	return service, engine, preview
}

func TestConfirmRefusesWorkWhenServiceClosesAfterRevalidation(t *testing.T) {
	service, engine, preview := newWorkerFixture(t)
	engine.BeforeList = func() error {
		service.Close()
		return nil
	}
	if _, err := service.Confirm(t.Context(), preview.PlanID, preview.ApprovalToken); !errors.Is(err, ErrInstallStopped) {
		t.Fatalf("a stopped service accepted a plan: %v", err)
	}
	if creates, _, starts, _, _ := engine.Observation(); creates != 0 || len(starts) != 0 {
		t.Fatal("a stopped service contacted Docker for mutations")
	}
}

func TestWorkerFlashFailureKeepsLastDurableCheckpoint(t *testing.T) {
	for _, tc := range []struct {
		name               string
		listObservation    int
		inspectObservation int
		lastPhase          string
		containerCreated   bool
	}{
		{name: "before validation", lastPhase: "queued"},
		{name: "before pulling", listObservation: 1, lastPhase: "validating"},
		{name: "before storage preparation", listObservation: 2, lastPhase: "pulling_image"},
		{name: "before publication", inspectObservation: 1, lastPhase: "creating_container", containerCreated: true},
		{name: "before start", inspectObservation: 2, lastPhase: "publishing_template", containerCreated: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service, engine, preview := newWorkerFixture(t)
			plan, err := service.lookupPending(preview.PlanID, preview.ApprovalToken)
			if err != nil {
				t.Fatal(err)
			}
			receipt := installFixtureReceipt("queued", AttemptPending)
			receipt.Attempt.Configuration = plan.effective.configuration
			if err := service.loadReceipts(); err != nil {
				t.Fatal(err)
			}
			if err := service.persistProgress(receipt); err != nil {
				t.Fatal(err)
			}
			dir := filepath.Join(service.hostRoot, attemptDirectory)
			held := dir + "-held"
			flashUnavailable := func() error {
				if err := os.Rename(dir, held); err != nil {
					return err
				}
				return os.WriteFile(dir, []byte("fixture flash unavailable"), 0o600)
			}
			var lists, inspections int
			engine.BeforeList = func() error {
				lists++
				if lists == tc.listObservation {
					return flashUnavailable()
				}
				return nil
			}
			engine.BeforeContainerInspect = func(_ string, c *container.InspectResponse) error {
				if c != nil {
					inspections++
					if inspections == tc.inspectObservation {
						return flashUnavailable()
					}
				}
				return nil
			}
			if tc.listObservation == 0 && tc.inspectObservation == 0 {
				if err := flashUnavailable(); err != nil {
					t.Fatal(err)
				}
			}
			service.runInstall(t.Context(), receipt, plan)
			if err := os.Remove(dir); err != nil {
				t.Fatal(err)
			}
			if err := os.Rename(held, dir); err != nil {
				t.Fatal(err)
			}
			restarted := NewService(nil, ServiceOptions{HostRoot: service.hostRoot})
			t.Cleanup(restarted.Close)
			got, err := restarted.GetAttempt(receipt.Attempt.ID)
			if err != nil || got.Phase != tc.lastPhase {
				t.Fatalf("worker advanced before saving the checkpoint: %+v, %v", got, err)
			}
			created, present, started, _, _ := engine.Observation()
			if (created == 1) != tc.containerCreated || present != created || len(started) != 0 || len(engine.RemoveRequests()) != 0 {
				t.Fatalf("worker made unauthorized Docker changes: creates %d, remaining %d, starts %v, removes %v",
					created, present, started, engine.RemoveRequests())
			}
		})
	}
}

func TestWorkerStopsWhenSettingsOrNativeTemplateCannotBeVerified(t *testing.T) {
	for _, tc := range []struct {
		name      string
		configure func(*communityappstest.InstallEngine, *pendingPlan)
		problem   *Problem
		creates   int
	}{
		{name: "Docker state unavailable on first worker revalidation",
			configure: func(e *communityappstest.InstallEngine, _ *pendingPlan) {
				e.BeforeList = func() error { return errors.New("fixture Docker failure") }
			}, problem: ErrDockerState},
		{name: "native template would exceed the publication bound",
			configure: func(_ *communityappstest.InstallEngine, p *pendingPlan) {
				p.effective.template.Overview = strings.Repeat(">", maxNativeTemplate/4)
			}, problem: ErrNativeTemplate, creates: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service, engine, preview := newWorkerFixture(t)
			plan, err := service.lookupPending(preview.PlanID, preview.ApprovalToken)
			if err != nil {
				t.Fatal(err)
			}
			receipt := installFixtureReceipt("queued", AttemptPending)
			receipt.Attempt.Configuration = plan.effective.configuration
			if err := service.loadReceipts(); err != nil {
				t.Fatal(err)
			}
			if err := service.persistProgress(receipt); err != nil {
				t.Fatal(err)
			}
			tc.configure(engine, plan)
			service.runInstall(t.Context(), receipt, plan)
			got, err := service.GetAttempt(receipt.Attempt.ID)
			if err != nil || got.State != AttemptFailed || got.Problem == nil || got.Problem.Code != tc.problem.Code {
				t.Fatalf("worker did not return the redacted failure: %+v, %v", got, err)
			}
			created, present, started, _, _ := engine.Observation()
			if created != tc.creates || present != 0 || len(started) != 0 {
				t.Fatalf("worker left an unexpected Docker resource: creates %d, remaining %d, starts %v", created, present, started)
			}
		})
	}
}

func TestWorkerDoesNotCreateWhenIntentReceiptCannotBeWritten(t *testing.T) {
	service, engine, preview := newWorkerFixture(t)
	plan, err := service.lookupPending(preview.PlanID, preview.ApprovalToken)
	if err != nil {
		t.Fatal(err)
	}
	receipt := installFixtureReceipt("queued", AttemptPending)
	receipt.Attempt.Configuration = plan.effective.configuration
	if err := service.loadReceipts(); err != nil {
		t.Fatal(err)
	}
	if err := service.persistProgress(receipt); err != nil {
		t.Fatal(err)
	}
	var failedCreateIntentWrite bool
	service.createReceiptTemp = func(dir, pattern string) (*os.File, error) {
		if !failedCreateIntentWrite {
			raw, err := os.ReadFile(filepath.Join(dir, receipt.PlanID+".json"))
			if err != nil {
				return nil, err
			}
			var last installReceipt
			if err := json.Unmarshal(raw, &last); err != nil {
				return nil, err
			}
			if last.Attempt.Phase == "preparing_storage" {
				failedCreateIntentWrite = true
				return createReadOnlyReceiptTemp(dir, pattern)
			}
		}
		return os.CreateTemp(dir, pattern)
	}
	service.runInstall(t.Context(), receipt, plan)
	got, err := service.GetAttempt(receipt.Attempt.ID)
	if err != nil || got.State != AttemptFailed || got.Problem == nil || got.Problem.Code != ErrAttemptStorage.Code {
		t.Fatalf("failed create-intent write did not settle as a storage error: %+v, %v", got, err)
	}
	if !failedCreateIntentWrite {
		t.Fatal("fixture never reached the write after durable storage preparation")
	}
	if creates, _, starts, _, _ := engine.Observation(); creates != 0 || len(starts) != 0 {
		t.Fatal("worker mutated Docker without a durable create intent")
	}
}
