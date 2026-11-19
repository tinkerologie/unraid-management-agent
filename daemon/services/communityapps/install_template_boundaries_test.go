package communityapps

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func TestTemplatePublicationDoesNotOverwriteExistingStage(t *testing.T) {
	service, _, receipt := newInstallRecoveryFixture(t, "publishing_template")
	content, err := prepareNativeTemplate(installSettings{configuration: receipt.Attempt.Configuration}, &receipt, 1)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(service.hostRoot, userTemplatesDir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	blocked := filepath.Join(dir, service.stagedTemplateName(receipt))
	const sentinel = "an unrelated native staging file"
	if err := os.WriteFile(blocked, []byte(sentinel), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := service.publishNativeTemplate(content, receipt); !errors.Is(err, ErrNativeTemplate) {
		t.Fatalf("publishing over an existing stage returned %v", err)
	}
	if got, err := os.ReadFile(blocked); err != nil || string(got) != sentinel {
		t.Fatalf("unrelated stage was changed: %q, %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(dir, service.finalTemplateName(receipt))); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("publication continued without staging: %v", err)
	}
}

func TestNativeTemplateOwnershipRejectsMissingOrUntrustedProof(t *testing.T) {
	service := NewService(nil, ServiceOptions{HostRoot: filepath.Join(t.TempDir(), "host")})
	t.Cleanup(service.Close)
	receipt := installFixtureReceipt("publishing_template", AttemptRunning)
	if owned, err := service.ownsPublishedTemplate(receipt); owned || err != nil {
		t.Fatalf("unmounted native root returned %v, %v", owned, err)
	}
	dir := filepath.Join(service.hostRoot, userTemplatesDir)
	if issue := service.removeOwnedNativeFile(receipt, false); issue.Code != "" {
		t.Fatalf("missing native path could not settle cleanup: %+v", issue)
	}
	if err := os.MkdirAll(filepath.Dir(dir), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Dir(dir), dir); err != nil {
		t.Fatal(err)
	}
	if owned, err := service.ownsPublishedTemplate(receipt); owned || !errors.Is(err, ErrNativeTemplate) {
		t.Fatalf("symlinked native directory returned %v, %v", owned, err)
	}
	if err := os.Remove(dir); err != nil {
		t.Fatal(err)
	}
	filename := publishRecoveryTemplate(t, service, &receipt)
	content, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		data []byte
	}{
		{name: "valid XML lacks an ownership marker", data: []byte(xml.Header + "<Container/>\n")},
		{name: "owner marker contains a different proof", data: bytes.Replace(content,
			[]byte(" -->\n"), []byte("INVALID -->\n"), 1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.WriteFile(filename, tc.data, 0o600); err != nil {
				t.Fatal(err)
			}
			if owned, err := service.ownsPublishedTemplate(receipt); owned || err != nil {
				t.Fatalf("unproved template returned %v, %v", owned, err)
			}
			_ = service.removeOwnedNativeFile(receipt, false)
			if got, err := os.ReadFile(filename); err != nil || !bytes.Equal(got, tc.data) {
				t.Fatalf("unproved native template was removed or modified: %v", err)
			}
		})
	}
}

func TestNativeTemplateCleanupReportsPermissionFailures(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses fixture file and directory permissions")
	}
	for _, tc := range []struct {
		name       string
		quarantine bool
		unreadable bool
	}{
		{name: "owned template cannot be quarantined"},
		{name: "owned quarantine cannot be removed", quarantine: true},
		{name: "template cannot be read for ownership proof", unreadable: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service, _, receipt := newInstallRecoveryFixture(t, "cleaning_up")
			final := publishRecoveryTemplate(t, service, &receipt)
			path := final
			if tc.quarantine {
				path = filepath.Join(filepath.Dir(final), nativePrivatePrefix(receipt)+".published-cleanup")
				if err := os.Rename(final, path); err != nil {
					t.Fatal(err)
				}
			}
			blocked, mode, restored := filepath.Dir(final), fs.FileMode(0o500), fs.FileMode(0o700)
			if tc.unreadable {
				blocked, mode, restored = path, 0o200, 0o600
			}
			if err := os.Chmod(blocked, mode); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.Chmod(blocked, restored) })
			issue := service.removeOwnedNativeFile(receipt, false)
			if issue.Code != "remove_template" || issue.TemplateFile != filepath.Base(path) ||
				issue.OwnershipVerified == tc.unreadable {
				t.Fatalf("cleanup did not identify the preserved native file: %+v", issue)
			}
			if _, err := os.Stat(path); err != nil {
				t.Fatalf("failed cleanup discarded a native file: %v", err)
			}
		})
	}
}

func TestNativeTemplateCleanupRestoresUnexpectedQuarantineContents(t *testing.T) {
	for _, tc := range []struct {
		name      string
		contents  func([]byte) []byte
		wantIssue bool
	}{
		{name: "unrelated native XML", contents: func(_ []byte) []byte {
			return []byte(xml.Header + "<Container><Name>someone-else</Name></Container>\n")
		}},
		{name: "native XML edited without preserving proof", contents: func(content []byte) []byte {
			return append(content, []byte("<!-- edited -->\n")...)
		}, wantIssue: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service, _, receipt := newInstallRecoveryFixture(t, "cleaning_up")
			final := publishRecoveryTemplate(t, service, &receipt)
			original, err := os.ReadFile(final)
			if err != nil {
				t.Fatal(err)
			}
			quarantine := filepath.Join(filepath.Dir(final), nativePrivatePrefix(receipt)+".published-cleanup")
			if err := os.Rename(final, quarantine); err != nil {
				t.Fatal(err)
			}
			replacement := tc.contents(original)
			if err := os.WriteFile(quarantine, replacement, 0o600); err != nil {
				t.Fatal(err)
			}

			issue := service.removeOwnedNativeFile(receipt, false)
			if tc.wantIssue {
				if issue.Code != "remove_template" || issue.OwnershipVerified || issue.TemplateFile != filepath.Base(final) {
					t.Fatalf("cleanup did not report the restored native file: %+v", issue)
				}
			} else if issue.Code != "" {
				t.Fatalf("restored unrelated native file returned a cleanup issue: %+v", issue)
			}
			if got, err := os.ReadFile(final); err != nil || !bytes.Equal(got, replacement) {
				t.Fatalf("unexpected quarantine contents were lost: %v", err)
			}
			if _, err := os.Stat(quarantine); !errors.Is(err, fs.ErrNotExist) {
				t.Fatalf("restored quarantine remained under both names: %v", err)
			}
		})
	}
}

func TestNativeTemplateOmitsWebUIWithInvalidURLSnapshot(t *testing.T) {
	if got := safeNativeURL("http://[IP]:[PORT:80]/%zz"); got != "" {
		t.Fatalf("malformed percent escape was retained in the WebUI URL: %q", got)
	}
}
