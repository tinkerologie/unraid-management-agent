package communityapps

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"

	"github.com/tinkerologie/unraid-management-agent/daemon/services/internal/communityappstest"
)

func newInstallRecoveryFixture(t *testing.T, phase string) (*Service, *communityappstest.InstallEngine, installReceipt) {
	t.Helper()
	engine := &communityappstest.InstallEngine{}
	service := NewService(nil, ServiceOptions{
		HostRoot:          t.TempDir(),
		OpenInstallDocker: func() (InstallDocker, error) { return engine, nil },
	})
	t.Cleanup(service.Close)
	receipt := installFixtureReceipt(phase, AttemptRunning)
	receipt.ImageID = communityappstest.ImageID
	if err := service.loadReceipts(); err != nil {
		t.Fatal(err)
	}
	return service, engine, receipt
}

func createRecoveryContainer(t *testing.T, engine *communityappstest.InstallEngine, receipt *installReceipt, owner string) {
	t.Helper()
	created, err := engine.ContainerCreate(t.Context(), client.ContainerCreateOptions{
		Name: receipt.Attempt.Configuration.Name,
		Config: &container.Config{
			Image: receipt.Attempt.Configuration.Image, Labels: map[string]string{installOwnerLabel: owner},
		},
		HostConfig: &container.HostConfig{},
	})
	if err != nil {
		t.Fatal(err)
	}
	receipt.OwnedContainerID = created.ID
}

func publishRecoveryTemplate(t *testing.T, service *Service, receipt *installReceipt) string {
	t.Helper()
	template, err := prepareNativeTemplate(installSettings{configuration: receipt.Attempt.Configuration}, receipt, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.publishNativeTemplate(template, *receipt); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(service.hostRoot, userTemplatesDir, service.finalTemplateName(*receipt))
}

func TestRecoveryRejectsInvalidReceiptsBeforeOpeningDocker(t *testing.T) {
	for _, tc := range []struct {
		name    string
		phase   string
		state   AttemptState
		cleanup string
		problem *Problem
	}{
		{name: "accepted queued plan", phase: "queued", state: AttemptPending, problem: ErrInstallInterrupted},
		{name: "unknown cleanup intent", phase: "cleaning_up", state: AttemptRunning, cleanup: "bogus-phase", problem: ErrInstallReceiptProof},
		{name: "unknown work phase", phase: "bogus-phase", state: AttemptRunning, problem: ErrInstallReceiptProof},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service, _, receipt := newInstallRecoveryFixture(t, tc.phase)
			receipt.Attempt.State, receipt.CleanupPhase = tc.state, tc.cleanup
			service.openInstaller = func() (InstallDocker, error) {
				t.Error("recovery contacted Docker without a valid mutation intent")
				return nil, ErrDockerState
			}
			if err := service.persistProgress(receipt); err != nil {
				t.Fatal(err)
			}
			stopped, cancel := context.WithCancel(t.Context())
			cancel()
			if service.reconcileInstall(stopped, receipt) {
				t.Fatal("cancelled recovery settled an attempt")
			}
			if !service.reconcileInstall(t.Context(), receipt) {
				t.Fatal("valid recovery context could not settle the receipt")
			}
			got, err := service.GetAttempt(receipt.Attempt.ID)
			if err != nil || got.State != AttemptFailed || got.Problem == nil || got.Problem.Code != tc.problem.Code {
				t.Fatalf("unexpected recovery: %+v, %v", got, err)
			}
		})
	}
}

func TestRecoveryMissingOrChangedIdentityKeepsPublishedTemplate(t *testing.T) {
	tests := []struct {
		name    string
		inspect func(*container.InspectResponse)
		problem *Problem
	}{
		{name: "malformed Docker ID", inspect: func(c *container.InspectResponse) { c.ID = "short" }, problem: ErrStartUnverified},
		{name: "different full Docker ID", inspect: func(c *container.InspectResponse) { c.ID = strings.Repeat("b", 64) }, problem: ErrInstallInterrupted},
		{name: "missing Docker state", inspect: func(c *container.InspectResponse) { c.State = nil }, problem: ErrStartUnverified},
		{name: "owner label changed", inspect: func(c *container.InspectResponse) {
			c.Config = &container.Config{Image: c.Config.Image, Labels: map[string]string{installOwnerLabel: "foreign"}}
		}, problem: errContainerIdentity},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			service, engine, receipt := newInstallRecoveryFixture(t, "starting_container")
			createRecoveryContainer(t, engine, &receipt, receipt.Owner)
			filename := publishRecoveryTemplate(t, service, &receipt)
			if err := service.persistProgress(receipt); err != nil {
				t.Fatal(err)
			}
			engine.BeforeContainerInspect = func(_ string, inspected *container.InspectResponse) error {
				if inspected != nil {
					tc.inspect(inspected)
				}
				return nil
			}
			settled := service.reconcileInstall(t.Context(), receipt)
			got, err := service.GetAttempt(receipt.Attempt.ID)
			if err != nil || got.Problem == nil || got.Problem.Code != tc.problem.Code ||
				settled != (got.State == AttemptFailed) || len(engine.RemoveRequests()) != 0 {
				t.Fatalf("unproved Docker identity was acted on: %+v, %v", got, err)
			}
			if _, err := os.Stat(filename); err != nil {
				t.Fatalf("native template was not preserved: %v", err)
			}
		})
	}
}

func TestLegacyCleanupReceiptPreservesVerifiedStart(t *testing.T) {
	service, engine, receipt := newInstallRecoveryFixture(t, "cleaning_up")
	receipt.Version = 1
	createRecoveryContainer(t, engine, &receipt, receipt.Owner)
	publishRecoveryTemplate(t, service, &receipt)
	if err := service.persistProgress(receipt); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.ContainerStart(t.Context(), receipt.OwnedContainerID, client.ContainerStartOptions{}); err != nil {
		t.Fatal(err)
	}
	if !service.reconcileInstall(t.Context(), receipt) {
		t.Fatal("legacy receipt could not verify the started container")
	}
	checkRecoveredContainer(t, service, engine, receipt, 0)
}

func TestStartRecoveryHandlesMissingOrNonregularNativeTemplate(t *testing.T) {
	for _, missingDirectory := range []bool{false, true} {
		t.Run(map[bool]string{false: "nonregular entry", true: "missing directory"}[missingDirectory], func(t *testing.T) {
			service, engine, receipt := newInstallRecoveryFixture(t, "starting_container")
			createRecoveryContainer(t, engine, &receipt, receipt.Owner)
			filename := publishRecoveryTemplate(t, service, &receipt)
			if missingDirectory {
				if err := os.RemoveAll(filepath.Dir(filename)); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := os.Remove(filename); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(filename, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			if err := service.persistProgress(receipt); err != nil {
				t.Fatal(err)
			}
			if _, err := engine.ContainerStart(t.Context(), receipt.OwnedContainerID, client.ContainerStartOptions{}); err != nil {
				t.Fatal(err)
			}
			if !service.reconcileInstall(t.Context(), receipt) {
				t.Fatal("Docker started and the template's absence could be proved")
			}
			got, err := service.GetAttempt(receipt.Attempt.ID)
			if err != nil || got.State != AttemptFailed || got.Problem == nil ||
				got.Problem.Code != ErrStartedTemplateMissing.Code || got.ContainerID != receipt.OwnedContainerID ||
				len(engine.RemoveRequests()) != 0 {
				t.Fatalf("missing template caused recovery to remove the started app: %+v, %v", got, err)
			}
		})
	}
}

func TestRecoveryRechecksStartWhenEngineInitiallyRejectedTheCall(t *testing.T) {
	for _, tc := range []struct {
		name       string
		startedOn  int
		removeSeen int
	}{
		{name: "independent start observed before cleanup", startedOn: 2},
		{name: "independent start observed after refused removal", startedOn: 3, removeSeen: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service, engine, receipt := newInstallRecoveryFixture(t, "starting_container")
			receipt.EngineRejected = true
			receipt.Attempt.Problem = ProblemForError(ErrContainerStart)
			createRecoveryContainer(t, engine, &receipt, receipt.Owner)
			publishRecoveryTemplate(t, service, &receipt)
			if err := service.persistProgress(receipt); err != nil {
				t.Fatal(err)
			}
			var observed int
			engine.BeforeContainerInspect = func(_ string, c *container.InspectResponse) error {
				if c != nil {
					observed++
					if observed >= tc.startedOn {
						c.State.Status = "exited"
						c.State.StartedAt = time.Unix(1, 0).UTC().Format(time.RFC3339Nano)
					}
				}
				return nil
			}
			engine.BeforeRemove = func() error { return errors.New("Docker refused removal") }
			if !service.reconcileInstall(t.Context(), receipt) {
				t.Fatal("independently started container could not be verified")
			}
			checkRecoveredContainer(t, service, engine, receipt, tc.removeSeen)
		})
	}
}

func checkRecoveredContainer(t *testing.T, service *Service, engine *communityappstest.InstallEngine, receipt installReceipt, removeCalls int) {
	t.Helper()
	got, err := service.GetAttempt(receipt.Attempt.ID)
	if err != nil || got.State != AttemptSucceeded || got.ContainerID != receipt.OwnedContainerID || got.Problem != nil {
		t.Fatalf("started container was not recovered: %+v, %v", got, err)
	}
	if removes := engine.RemoveRequests(); len(removes) != removeCalls {
		t.Fatalf("unexpected Docker cleanup: %+v", removes)
	}
	if _, err := engine.ContainerInspect(t.Context(), receipt.OwnedContainerID, client.ContainerInspectOptions{}); err != nil {
		t.Fatalf("the original Docker container was not preserved: %v", err)
	}
}

func TestRecoveryRetriesCleanupWhenDockerIdentityCanBeChecked(t *testing.T) {
	service, engine, receipt := newInstallRecoveryFixture(t, "cleaning_up")
	receipt.CleanupPhase = "publishing_template"
	createRecoveryContainer(t, engine, &receipt, receipt.Owner)
	filename := publishRecoveryTemplate(t, service, &receipt)
	if err := service.persistProgress(receipt); err != nil {
		t.Fatal(err)
	}
	engine.BeforeContainerInspect = func(_ string, _ *container.InspectResponse) error {
		return errors.New("Docker temporarily unavailable")
	}
	if service.reconcileInstall(t.Context(), receipt) || len(engine.RemoveRequests()) != 0 {
		t.Fatal("cleanup settled while Docker ownership could not be checked")
	}
	if _, err := os.Stat(filename); err != nil {
		t.Fatalf("uncertain Docker state removed the native template: %v", err)
	}
	engine.BeforeContainerInspect = nil
	if !service.reconcileInstall(t.Context(), receipt) {
		t.Fatal("cleanup did not resume when Docker returned")
	}
	if removes := engine.RemoveRequests(); len(removes) != 1 || removes[0].ID != receipt.OwnedContainerID {
		t.Fatalf("cleanup did not remove the exact partial container: %+v", removes)
	}
}

func TestLegacyCleanupRequiresDurableIdentityBeforeContainerRemoval(t *testing.T) {
	service, engine, receipt := newInstallRecoveryFixture(t, "cleaning_up")
	receipt.Version = 1
	createRecoveryContainer(t, engine, &receipt, receipt.Owner)
	id := receipt.OwnedContainerID
	receipt.OwnedContainerID = ""
	engine.Containers = []container.Summary{{
		ID: strings.Repeat("b", 64), Labels: map[string]string{installOwnerLabel: "another-owner"},
	}}
	if err := service.persistProgress(receipt); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(service.hostRoot, attemptDirectory)
	held := dir + "-held"
	if err := os.Rename(dir, held); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir, []byte("fixture flash unavailable"), 0o600); err != nil {
		t.Fatal(err)
	}
	if service.reconcileInstall(t.Context(), receipt) || len(engine.RemoveRequests()) != 0 {
		t.Fatal("legacy cleanup removed a container before its exact ID was recorded")
	}
	if err := os.Remove(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(held, dir); err != nil {
		t.Fatal(err)
	}
	if !service.reconcileInstall(t.Context(), receipt) {
		t.Fatal("legacy cleanup could not resume with writable storage")
	}
	if removes := engine.RemoveRequests(); len(removes) != 1 || removes[0].ID != id {
		t.Fatalf("legacy cleanup targeted an unexpected resource: %+v", removes)
	}
}

func TestCleanupKeepsUnverifiedStartAndUnexpectedStartedImage(t *testing.T) {
	service, engine, receipt := newInstallRecoveryFixture(t, "cleaning_up")
	receipt.CleanupPhase = "starting_container"
	createRecoveryContainer(t, engine, &receipt, receipt.Owner)
	publishRecoveryTemplate(t, service, &receipt)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if ready, _ := service.cleanupInstall(ctx, engine, &receipt); ready {
		t.Fatal("cancelled cleanup settled the attempt")
	}
	if ready, _ := service.cleanupInstall(t.Context(), engine, &receipt); ready || len(engine.RemoveRequests()) != 0 {
		t.Fatal("cleanup removed a container while the original Start was unresolved")
	}
	if _, err := engine.ContainerStart(t.Context(), receipt.OwnedContainerID, client.ContainerStartOptions{}); err != nil {
		t.Fatal(err)
	}
	receipt.EngineRejected = true
	engine.BeforeContainerInspect = func(_ string, c *container.InspectResponse) error {
		if c != nil {
			c.Image = "sha256:" + strings.Repeat("b", 64)
		}
		return nil
	}
	issue, observed, retry := service.removeOwnedContainer(t.Context(), engine, &receipt)
	if issue.Code != cleanupContainerStarted || !issue.OwnershipVerified || observed != startUnobserved ||
		retry || len(engine.RemoveRequests()) != 0 {
		t.Fatalf("cleanup acted on a started container with a different image: %+v", issue)
	}
}

func TestPartialInstallCleanupWaitsForVerifiedDockerStartState(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*container.InspectResponse)
	}{
		{name: "missing Docker state", mutate: func(c *container.InspectResponse) { c.State = nil }},
		{name: "contradictory state with a zero timestamp", mutate: func(c *container.InspectResponse) { c.State.Status = "exited" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service, engine, receipt := newInstallRecoveryFixture(t, "cleaning_up")
			receipt.CleanupPhase = "creating_container"
			createRecoveryContainer(t, engine, &receipt, receipt.Owner)
			if err := service.persistProgress(receipt); err != nil {
				t.Fatal(err)
			}
			engine.BeforeContainerInspect = func(_ string, c *container.InspectResponse) error {
				if c != nil {
					tc.mutate(c)
				}
				return nil
			}
			if service.reconcileInstall(t.Context(), receipt) || len(engine.RemoveRequests()) != 0 {
				t.Fatal("cleanup removed an owned container without verifying Docker start state")
			}
		})
	}
}

func TestStartedContainerRemainsUnverifiedWhenNativeTemplateStatFails(t *testing.T) {
	const name = "my-recovery-fixture.xml"
	got, err := observePublishedNativeTemplate(func(path string) (fs.FileInfo, error) {
		if path != name {
			t.Fatalf("verification looked up a different native template: %q", path)
		}
		return nil, fs.ErrPermission
	}, nil, nil, name, "recovery-fixture")
	if got != startUnobserved || !errors.Is(err, ErrNativeTemplate) {
		t.Fatalf("unreadable native template was treated as absent: %v, %v", got, err)
	}
}
