package communityapps

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

func TestVerifiedStartRecoversOwnedQuarantinedTemplate(t *testing.T) {
	for _, tc := range []struct {
		name      string
		version   int
		competing bool
	}{
		{name: "legacy cleanup restores the published XML", version: 1},
		{name: "current cleanup restores the published XML", version: installReceiptVersion},
		{name: "an unrelated native template blocks exclusive restore", version: installReceiptVersion, competing: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service, engine, receipt := newInstallRecoveryFixture(t, "cleaning_up")
			receipt.Version = tc.version
			if tc.version != 1 {
				receipt.CleanupPhase = "starting_container"
			}
			createRecoveryContainer(t, engine, &receipt, receipt.Owner)
			filename := publishRecoveryTemplate(t, service, &receipt)
			original, err := os.ReadFile(filename)
			if err != nil {
				t.Fatal(err)
			}
			quarantine := filepath.Join(filepath.Dir(filename), nativePrivatePrefix(receipt)+".published-cleanup")
			if err := os.Rename(filename, quarantine); err != nil {
				t.Fatal(err)
			}
			const foreign = "<Container><Name>different-app</Name></Container>"
			if tc.competing {
				if err := os.WriteFile(filename, []byte(foreign), 0o600); err != nil {
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
				t.Fatal("recovery did not settle after verifying the paths")
			}
			got, err := service.GetAttempt(receipt.Attempt.ID)
			if err != nil || got.ContainerID != receipt.OwnedContainerID || len(engine.RemoveRequests()) != 0 {
				t.Fatalf("recovery did not preserve the verified started container: %+v, %v", got, err)
			}
			if tc.competing {
				if got.State != AttemptFailed || got.Problem == nil || got.Problem.Code != ErrStartedTemplateMissing.Code ||
					len(got.CleanupDetails) != 1 || got.CleanupDetails[0].Code != "restore_template" ||
					got.CleanupDetails[0].TemplateFile != filepath.Base(quarantine) || !got.CleanupDetails[0].OwnershipVerified {
					t.Fatalf("blocked restore did not report the owned quarantine: %+v", got)
				}
				if data, err := os.ReadFile(filename); err != nil || string(data) != foreign {
					t.Fatalf("exclusive restore changed the competing native template: %v", err)
				}
				if data, err := os.ReadFile(quarantine); err != nil || !bytes.Equal(data, original) {
					t.Fatalf("exclusive restore lost the quarantined native template: %v", err)
				}
				return
			}
			if got.State != AttemptSucceeded || got.Problem != nil || len(got.CleanupDetails) != 0 {
				t.Fatalf("restored native template did not complete the install: %+v", got)
			}
			if data, err := os.ReadFile(filename); err != nil || !bytes.Equal(data, original) {
				t.Fatalf("native template was not restored: %v", err)
			}
			if _, err := os.Lstat(quarantine); !errors.Is(err, fs.ErrNotExist) {
				t.Fatalf("native template remained in quarantine after restoration: %v", err)
			}
		})
	}
}

func TestVerifiedStartWaitsWhenRestoredTemplateCannotBeRechecked(t *testing.T) {
	service, engine, receipt := newInstallRecoveryFixture(t, "starting_container")
	createRecoveryContainer(t, engine, &receipt, receipt.Owner)
	filename := publishRecoveryTemplate(t, service, &receipt)
	quarantine := filepath.Join(filepath.Dir(filename), nativePrivatePrefix(receipt)+".published-cleanup")
	if err := os.Rename(filename, quarantine); err != nil {
		t.Fatal(err)
	}
	if err := service.persistProgress(receipt); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.ContainerStart(t.Context(), receipt.OwnedContainerID, client.ContainerStartOptions{}); err != nil {
		t.Fatal(err)
	}
	observed, err := service.observeInstallStart(t.Context(), engine, receipt)
	if err != nil || observed != startTemplateMissing {
		t.Fatalf("fixture did not observe a verified start with missing native XML: %v, %v", observed, err)
	}
	nativeDir := filepath.Dir(filename)
	held := nativeDir + "-held"
	if err := os.Rename(nativeDir, held); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(held, nativeDir); err != nil {
		t.Fatal(err)
	}
	if service.finishObservedStart(&receipt, observed) {
		t.Fatal("an unreadable final native template caused recovery to settle")
	}
	if got, err := service.GetAttempt(receipt.Attempt.ID); err != nil || got.State != AttemptRunning ||
		len(got.CleanupDetails) != 0 || len(engine.RemoveRequests()) != 0 {
		t.Fatalf("unavailable native template changed a pending attempt: %+v, %v", got, err)
	}
	if err := os.Remove(nativeDir); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(held, nativeDir); err != nil {
		t.Fatal(err)
	}
	if !service.reconcileInstall(t.Context(), receipt) {
		t.Fatal("recovery could not resume after native template inspection became available")
	}
	checkRecoveredContainer(t, service, engine, receipt, 0)
}

func TestCleanupRetainsTemplateWhenDockerRemovalAndReinspectionFail(t *testing.T) {
	service, engine, receipt := newInstallRecoveryFixture(t, "publishing_template")
	createRecoveryContainer(t, engine, &receipt, receipt.Owner)
	filename := publishRecoveryTemplate(t, service, &receipt)
	original, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.persistProgress(receipt); err != nil {
		t.Fatal(err)
	}
	engine.NoStartEffect = true
	if _, err := engine.ContainerStart(t.Context(), receipt.OwnedContainerID, client.ContainerStartOptions{}); err != nil {
		t.Fatal(err)
	}
	var removeAttempted bool
	engine.BeforeRemove = func() error {
		removeAttempted = true
		return errors.New("Docker could not remove the container")
	}
	engine.BeforeContainerInspect = func(key string, _ *container.InspectResponse) error {
		if removeAttempted && key == receipt.OwnedContainerID {
			return errors.New("Docker could not recheck the container")
		}
		return nil
	}
	if service.reconcileInstall(t.Context(), receipt) || !removeAttempted {
		t.Fatal("cleanup settled after a failed removal without further Docker evidence")
	}
	if got, err := service.GetAttempt(receipt.Attempt.ID); err != nil || got.Phase != "cleaning_up" || len(got.CleanupDetails) != 0 {
		t.Fatalf("unconfirmed removal did not retain cleanup intent: %+v, %v", got, err)
	}
	engine.CompletePendingStart(t)
	engine.BeforeContainerInspect = nil
	if !service.reconcileInstall(t.Context(), service.receipts[receipt.PlanID]) {
		t.Fatal("cleanup did not settle after Docker reported the completed start")
	}
	got, err := service.GetAttempt(receipt.Attempt.ID)
	if err != nil || got.State != AttemptFailed || len(got.CleanupDetails) != 1 ||
		got.CleanupDetails[0].Code != cleanupContainerStarted || len(engine.RemoveRequests()) != 1 {
		t.Fatalf("cleanup did not report the protected container: %+v, %v", got, err)
	}
	if data, err := os.ReadFile(filename); err != nil || !bytes.Equal(data, original) {
		t.Fatalf("cleanup discarded XML needed by the started container: %v", err)
	}
}
