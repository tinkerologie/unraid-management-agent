package communityapps

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

func TestRecoveryRetainsStartedContainerOutsideDurableStartPhase(t *testing.T) {
	for _, phase := range []string{"creating_container", "publishing_template"} {
		t.Run(phase, func(t *testing.T) {
			service, engine, receipt := newInstallRecoveryFixture(t, phase)
			createRecoveryContainer(t, engine, &receipt, receipt.Owner)
			var native string
			if phase == "publishing_template" {
				native = publishRecoveryTemplate(t, service, &receipt)
			}
			if err := service.persistProgress(receipt); err != nil {
				t.Fatal(err)
			}
			if _, err := engine.ContainerStart(t.Context(), receipt.OwnedContainerID, client.ContainerStartOptions{}); err != nil {
				t.Fatal(err)
			}
			if !service.reconcileInstall(t.Context(), receipt) {
				t.Fatal("recovery did not identify the started resource")
			}
			got, err := service.GetAttempt(receipt.Attempt.ID)
			if err != nil || got.State != AttemptFailed || len(got.CleanupDetails) != 1 ||
				got.CleanupDetails[0].Code != "container_already_started" ||
				got.CleanupDetails[0].ContainerID != receipt.OwnedContainerID || !got.CleanupDetails[0].OwnershipVerified {
				t.Fatalf("started resource was not reported as retained: %+v, %v", got, err)
			}
			if len(engine.RemoveRequests()) != 0 {
				t.Fatal("recovery attempted removal of a started container")
			}
			if native != "" {
				if _, err := os.Stat(native); err != nil {
					t.Fatalf("template used by the started container was removed: %v", err)
				}
			}
		})
	}
}

func TestRecoveryChecksContainerNameInEditedNativeTemplate(t *testing.T) {
	for _, tc := range []struct {
		name      string
		template  string
		succeeded bool
	}{
		{name: "valid native edit without marker", template: "<Container><Name>recovery-fixture</Name><Shell>bash</Shell></Container>", succeeded: true},
		{name: "different app", template: "<Container><Name>somebody-else</Name></Container>"},
		{name: "corrupt XML", template: "<Container><Name>recovery-fixture"},
		{name: "duplicate Name", template: "<Container><Name>recovery-fixture</Name><Name>somebody-else</Name></Container>"},
		{name: "namespaced Name", template: "<Container><Name xmlns='urn:other'>recovery-fixture</Name></Container>"},
		{name: "unrelated XML root", template: "<Metadata><Name>recovery-fixture</Name></Metadata>"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service, engine, receipt := newInstallRecoveryFixture(t, "starting_container")
			createRecoveryContainer(t, engine, &receipt, receipt.Owner)
			native := publishRecoveryTemplate(t, service, &receipt)
			if err := os.WriteFile(native, []byte(tc.template), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := service.persistProgress(receipt); err != nil {
				t.Fatal(err)
			}
			if _, err := engine.ContainerStart(t.Context(), receipt.OwnedContainerID, client.ContainerStartOptions{}); err != nil {
				t.Fatal(err)
			}
			if !service.reconcileInstall(t.Context(), receipt) {
				t.Fatal("readable native template did not produce an outcome")
			}
			got, err := service.GetAttempt(receipt.Attempt.ID)
			if err != nil || got.ContainerID != receipt.OwnedContainerID || len(engine.RemoveRequests()) != 0 {
				t.Fatalf("incorrect recovered identity: %+v, %v", got, err)
			}
			if tc.succeeded {
				if got.State != AttemptSucceeded || got.Problem != nil {
					t.Fatalf("valid edit was not recognized: %+v", got)
				}
			} else if got.State != AttemptFailed || got.Problem == nil || got.Problem.Code != ErrStartedTemplateMissing.Code {
				t.Fatalf("foreign or invalid template counted as completed: %+v", got)
			}
			if content, err := os.ReadFile(native); err != nil || !bytes.Equal(content, []byte(tc.template)) {
				t.Fatalf("edited template was changed: %v", err)
			}
		})
	}
}

func TestReplacementProtectsAndRestoresNativeTemplateQuarantine(t *testing.T) {
	for _, tc := range []struct {
		name           string
		oldStartIntent bool
		legacyReceipt  bool
		finalExists    bool
		foreignStage   bool
		wantRestored   bool
		wantIssue      bool
		ownershipProof bool
	}{
		{name: "restore when the final path is empty", wantRestored: true},
		{name: "keep a replacement template and report the owned quarantine", finalExists: true, wantIssue: true, ownershipProof: true},
		{name: "report an unowned quarantine without restoring it", foreignStage: true, wantIssue: true},
		{name: "legacy start cleanup restores when the original ID is gone", oldStartIntent: true, wantRestored: true},
		{name: "legacy start cleanup reports the conflicting quarantine", oldStartIntent: true, finalExists: true, wantIssue: true, ownershipProof: true},
		{name: "version one reports retained quarantine without its owner token", oldStartIntent: true, legacyReceipt: true, finalExists: true, wantIssue: true, ownershipProof: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service, engine, receipt := newInstallRecoveryFixture(t, "publishing_template")
			createRecoveryContainer(t, engine, &receipt, receipt.Owner)
			if tc.oldStartIntent {
				receipt.Attempt.Phase, receipt.CleanupPhase = "cleaning_up", "starting_container"
			}
			if tc.legacyReceipt {
				receipt.Version, receipt.CleanupPhase = 1, ""
			}
			final := publishRecoveryTemplate(t, service, &receipt)
			original, err := os.ReadFile(final)
			if err != nil {
				t.Fatal(err)
			}
			quarantine := filepath.Join(filepath.Dir(final), nativePrivatePrefix(receipt)+".published-cleanup")
			if err := os.Rename(final, quarantine); err != nil {
				t.Fatal(err)
			}
			const replacementTemplate = "<Container><Name>replacement</Name></Container>"
			if tc.finalExists {
				if err := os.WriteFile(final, []byte(replacementTemplate), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if tc.foreignStage {
				original = []byte(replacementTemplate)
				if err := os.WriteFile(quarantine, original, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			replacement := strings.Repeat("e", 64)
			engine.ReplaceForUpdate(t, replacement)
			if err := service.persistProgress(receipt); err != nil {
				t.Fatal(err)
			}
			if !service.reconcileInstall(t.Context(), receipt) || len(engine.RemoveRequests()) != 0 {
				t.Fatal("replacement did not veto Docker and template deletion")
			}
			results, err := service.GetAttempt(receipt.Attempt.ID)
			if err != nil {
				t.Fatal(err)
			}
			if tc.wantIssue {
				safeFilename := filepath.Base(quarantine)
				if tc.legacyReceipt {
					safeFilename = ""
					data, err := json.Marshal(results)
					if err != nil || bytes.Contains(data, []byte(receipt.Owner)) {
						t.Fatal("cleanup response disclosed the legacy ownership token")
					}
				}
				if len(results.CleanupDetails) != 1 || results.CleanupDetails[0].Code != "restore_template" ||
					results.CleanupDetails[0].TemplateFile != safeFilename ||
					results.CleanupDetails[0].OwnershipVerified != tc.ownershipProof {
					t.Fatalf("leftover quarantine was not reported: %+v", results)
				}
			} else if len(results.CleanupDetails) != 0 {
				t.Fatalf("restored file was reported as leftover: %+v", results)
			}
			read := func(path string, want []byte) {
				t.Helper()
				got, err := os.ReadFile(path)
				if err != nil || !bytes.Equal(got, want) {
					t.Fatalf("unexpected bytes in %s: %v", filepath.Base(path), err)
				}
			}
			switch {
			case tc.wantRestored:
				read(final, original)
				if _, err := os.Stat(quarantine); !errors.Is(err, fs.ErrNotExist) {
					t.Fatalf("quarantine was retained after successful restoration: %v", err)
				}
			case tc.finalExists:
				read(final, []byte(replacementTemplate))
				read(quarantine, original)
			default:
				if _, err := os.Stat(final); !errors.Is(err, fs.ErrNotExist) {
					t.Fatalf("unowned quarantine was restored: %v", err)
				}
				read(quarantine, original)
			}
			if _, err := engine.ContainerInspect(t.Context(), replacement, client.ContainerInspectOptions{}); err != nil {
				t.Fatal(fmt.Errorf("inspect the preserved replacement: %w", err))
			}
		})
	}
}

func TestRecoveredNativeTemplateRequiresReadableStableFile(t *testing.T) {
	root := t.TempDir()
	filename := filepath.Join(root, "my-recovery-fixture.xml")
	if err := os.WriteFile(filename, []byte("<Container><Name>recovery-fixture</Name></Container>"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		open func(string) (fs.File, error)
	}{
		{name: "open failed", open: func(string) (fs.File, error) { return nil, fs.ErrPermission }},
		{name: "entry changed before open", open: func(string) (fs.File, error) { return os.Open(root) }},
		{name: "read failed after identity check", open: func(name string) (fs.File, error) {
			return os.OpenFile(name, os.O_WRONLY, 0)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := observePublishedNativeTemplate(os.Lstat, tc.open, filename, "recovery-fixture")
			if got != startUnobserved || !errors.Is(err, ErrNativeTemplate) {
				t.Fatalf("unreadable native template derived a terminal proof: %v, %v", got, err)
			}
		})
	}
}

type failedNativeRestoreSync struct{ *os.File }

func (failedNativeRestoreSync) Sync() error { return fs.ErrPermission }

func TestRetainedTemplateRestoreReportsUncertainPublicationPath(t *testing.T) {
	service, _, receipt := newInstallRecoveryFixture(t, "cleaning_up")
	final := publishRecoveryTemplate(t, service, &receipt)
	quarantine := nativePrivatePrefix(receipt) + ".published-cleanup"
	if err := os.Rename(final, filepath.Join(filepath.Dir(final), quarantine)); err != nil {
		t.Fatal(err)
	}
	dir, err := service.openNativeTemplates()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = dir.Close() })
	issue := CleanupIssue{Resource: "template", Code: "restore_template", TemplateFile: quarantine}
	got := restorePublishedQuarantine(dir, receipt, issue, quarantine, filepath.Base(final), func() (nativeRestoreAnchor, error) {
		return nil, fs.ErrPermission
	})
	if !got.OwnershipVerified || got.TemplateFile != quarantine || got.Code != issue.Code {
		t.Fatalf("an unopened directory hid the owned quarantine: %+v", got)
	}

	anchor, err := dir.Open(".")
	if err != nil {
		t.Fatal(err)
	}
	got = restorePublishedQuarantine(dir, receipt, issue, quarantine, filepath.Base(final), func() (nativeRestoreAnchor, error) {
		return failedNativeRestoreSync{anchor}, nil
	})
	if !got.OwnershipVerified || got.TemplateFile != filepath.Base(final) || got.Code != issue.Code {
		t.Fatalf("a failed directory sync reported the wrong file location: %+v", got)
	}
	if _, err := os.Stat(final); err != nil {
		t.Fatalf("sync failure did not preserve the moved file: %v", err)
	}
}

func TestRecoveryDoesNotTreatUnreadableReceiptStoreAsEmpty(t *testing.T) {
	service := NewService(nil, ServiceOptions{HostRoot: t.TempDir()})
	t.Cleanup(service.Close)
	blocked := filepath.Join(service.hostRoot, attemptDirectory)
	if err := os.MkdirAll(filepath.Dir(blocked), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(blocked, []byte("fixture flash unavailable"), 0o600); err != nil {
		t.Fatal(err)
	}
	if pending, err := service.interruptedInstalls(); !errors.Is(err, ErrAttemptStorage) || len(pending) != 0 {
		t.Fatalf("an unreadable receipt directory appeared empty: %+v, %v", pending, err)
	}
}

func TestRecoveryRechecksStartAfterDockerRefusesPreStartCleanup(t *testing.T) {
	service, engine, receipt := newInstallRecoveryFixture(t, "publishing_template")
	createRecoveryContainer(t, engine, &receipt, receipt.Owner)
	final := publishRecoveryTemplate(t, service, &receipt)
	if err := service.persistProgress(receipt); err != nil {
		t.Fatal(err)
	}
	var observed int
	engine.BeforeContainerInspect = func(_ string, c *container.InspectResponse) error {
		if c != nil {
			observed++
			if observed >= 2 {
				c.State.Status, c.State.StartedAt = "exited", "2026-11-19T00:00:00Z"
			}
		}
		return nil
	}
	engine.BeforeRemove = func() error { return errors.New("Docker refused removal") }
	if !service.reconcileInstall(t.Context(), receipt) {
		t.Fatal("recovery did not identify an independently started container")
	}
	got, err := service.GetAttempt(receipt.Attempt.ID)
	if err != nil || len(got.CleanupDetails) != 1 || got.CleanupDetails[0].Code != cleanupContainerStarted {
		t.Fatalf("cleanup lost the newly observed start: %+v, %v", got, err)
	}
	if requests := engine.RemoveRequests(); len(requests) != 1 || requests[0].Options.Force {
		t.Fatalf("unexpected cleanup requests: %+v", requests)
	}
	if _, err := os.Stat(final); err != nil {
		t.Fatalf("attempted cleanup lost a template used by a started container: %v", err)
	}
}

func TestRecoveryConservativelyChecksNativeDirectoryForRetainedTemplate(t *testing.T) {
	service, _, receipt := newInstallRecoveryFixture(t, "cleaning_up")
	if got := service.restoreRetainedNativeTemplate(receipt); got.Code != "" {
		t.Fatalf("missing native directory reported a leftover quarantine: %+v", got)
	}
	path := filepath.Join(service.hostRoot, userTemplatesDir)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), path); err != nil {
		t.Fatal(err)
	}
	got := service.restoreRetainedNativeTemplate(receipt)
	if got.Code != "restore_template" || got.OwnershipVerified {
		t.Fatalf("an unverified native directory was treated as empty: %+v", got)
	}
}
