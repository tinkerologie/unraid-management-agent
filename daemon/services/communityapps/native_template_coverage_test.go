package communityapps

import (
	"encoding/xml"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tinkerologie/unraid-management-agent/daemon/services/internal/communityappstest"
)

func TestNativeTemplateRejectsOversizeAndPreexistingStagingFile(t *testing.T) {
	t.Run("oversize rendered template", func(t *testing.T) {
		receipt := templateCoverageReceipt(2)
		settings := installSettings{
			template:    Template{Config: []TemplateConfig{{Kind: ConfigEnvironment, Target: "VARIABLE"}}},
			environment: []environmentSetting{{name: "VARIABLE", value: strings.Repeat("&", maxNativeTemplate/4)}},
		}
		if template, err := prepareNativeTemplate(settings, &receipt, 10); !errors.Is(err, ErrNativeTemplate) || template != nil {
			t.Fatalf("oversize template returned %d bytes, %v", len(template), err)
		}
	})
	t.Run("occupied staging name", func(t *testing.T) {
		root := t.TempDir()
		service := NewService(nil, ServiceOptions{HostRoot: root})
		receipt := templateCoverageReceipt(2)
		file := filepath.Join(userTemplatesDir, service.stagedTemplateName(receipt))
		communityappstest.WriteFile(t, root, file, "existing staging data")
		if err := service.publishNativeTemplate([]byte("candidate"), receipt); !errors.Is(err, ErrNativeTemplate) {
			t.Fatalf("staging collision returned %v", err)
		}
		assertNativeFile(t, filepath.Join(root, file), "existing staging data")
	})
}

func TestNativeTemplateOwnershipHandlesMissingAndUnsafeEarlierPaths(t *testing.T) {
	receipt := templateCoverageReceipt(2)
	for _, rootExists := range []bool{false, true} {
		root := t.TempDir()
		if !rootExists {
			root = filepath.Join(root, "absent")
		}
		service := NewService(nil, ServiceOptions{HostRoot: root})
		if owned, err := service.ownsPublishedTemplate(receipt); err != nil || owned {
			t.Fatalf("missing native directory returned %t, %v", owned, err)
		}
		if issue := service.removeOwnedNativeFile(receipt, false); issue.Code != "" {
			t.Fatalf("missing native directory required cleanup: %+v", issue)
		}
	}
	root := t.TempDir()
	service := NewService(nil, ServiceOptions{HostRoot: root})
	communityappstest.WriteFile(t, root, "boot/config/plugins/dockerMan/real/marker", "")
	if err := os.Symlink("real", filepath.Join(root, userTemplatesDir)); err != nil {
		t.Fatal(err)
	}
	if owned, err := service.ownsPublishedTemplate(receipt); !errors.Is(err, ErrNativeTemplate) || owned {
		t.Fatalf("symlinked native directory returned %t, %v", owned, err)
	}
}

func TestNativeTemplateDoesNotTrustAForeignOrAlteredOwnershipMarker(t *testing.T) {
	root := t.TempDir()
	service := NewService(nil, ServiceOptions{HostRoot: root})
	receipt := templateCoverageReceipt(2)
	name := service.finalTemplateName(receipt)
	file := filepath.Join(userTemplatesDir, name)
	for _, tc := range []struct {
		name, marker string
	}{
		{"foreign marker", templateMarker("another-worker", "unrelated")},
		{"altered marker", templateMarker(receipt.Owner, "invalid")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			payload := xml.Header + tc.marker + "<Container/>"
			communityappstest.WriteFile(t, root, file, payload)
			if owned, err := service.ownsPublishedTemplate(receipt); err != nil || owned {
				t.Fatalf("unverified native marker returned %t, %v", owned, err)
			}
			assertNativeFile(t, filepath.Join(root, file), payload)
		})
	}
}

func TestCleanupRestoresQuarantinedForeignOrAlteredNativeTemplate(t *testing.T) {
	for _, altered := range []bool{false, true} {
		root := t.TempDir()
		service := NewService(nil, ServiceOptions{HostRoot: root})
		receipt := templateCoverageReceipt(2)
		native := "<Container><Name>operator-template</Name></Container>"
		if altered {
			native = xml.Header + templateMarker(receipt.Owner, "invalid") + native
		}
		quarantine := filepath.Join(userTemplatesDir, nativePrivatePrefix(receipt)+".published-cleanup")
		communityappstest.WriteFile(t, root, quarantine, native)
		outcome := service.removeOwnedNativeFile(receipt, false)
		if outcome.OwnershipVerified || altered && outcome.TemplateFile != service.finalTemplateName(receipt) || !altered && outcome.Code != "" {
			t.Fatalf("recovering foreign=%t template returned %+v", !altered, outcome)
		}
		assertNativeFile(t, filepath.Join(root, userTemplatesDir, service.finalTemplateName(receipt)), native)
		if _, err := os.Stat(filepath.Join(root, quarantine)); !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("quarantine should have been restored: %v", err)
		}
	}
}

func TestNativeTemplateChecksFileAccessAndReportsUnremovableVerifiedFile(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root bypasses fixture file permissions")
	}
	root := t.TempDir()
	service := NewService(nil, ServiceOptions{HostRoot: root})
	receipt := templateCoverageReceipt(2)
	template, err := prepareNativeTemplate(installSettings{}, &receipt, 10)
	if err != nil {
		t.Fatal(err)
	}
	quarantine := nativePrivatePrefix(receipt) + ".published-cleanup"
	path := filepath.Join(root, userTemplatesDir, quarantine)
	communityappstest.WriteFile(t, root, filepath.Join(userTemplatesDir, quarantine), string(template))
	dirPath := filepath.Dir(path)
	if err := os.Chmod(path, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o600) })
	dir, err := service.openNativeTemplates()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = dir.Close() }()
	if owned, err := nativeFileOwnership(dir, quarantine, receipt, false); !errors.Is(err, ErrNativeTemplate) || owned {
		t.Fatalf("unreadable template returned %t, %v", owned, err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dirPath, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dirPath, 0o700) })
	result := service.removeOwnedNativeFile(receipt, false)
	if result.TemplateFile != quarantine || !result.OwnershipVerified {
		t.Fatalf("failed verified cleanup did not identify the native file: %+v", result)
	}
	assertNativeFile(t, path, string(template))
}

func TestNativeTemplateRejectsUnsafeMetadataURL(t *testing.T) {
	for _, value := range []string{" https://example.invalid", "https://example.invalid/<script>", strings.Repeat("x", 2049)} {
		if got := safeNativeURL(value); got != "" {
			t.Fatalf("unsafe native metadata link was retained: %q", got)
		}
	}
}

func TestNativeHostReaderRejectsFileExpandedAfterStat(t *testing.T) {
	root := t.TempDir()
	communityappstest.WriteFile(t, root, "host.conf", "set")
	name := filepath.Join(root, "host.conf")
	err := readInstallFile(name, 4, func(reader io.Reader) error {
		communityappstest.WriteFile(t, root, "host.conf", "more than the approved limit")
		_, err := io.Copy(io.Discard, reader)
		return err
	})
	if !errors.Is(err, ErrStorageState) {
		t.Fatalf("growing host state file returned %v", err)
	}
}

func templateCoverageReceipt(version int) installReceipt {
	return installReceipt{
		Version: version, Owner: "test-owner",
		Attempt: Attempt{ID: "test-attempt", Configuration: Configuration{Name: "sample"}},
	}
}

func assertNativeFile(t *testing.T, path string, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil || string(data) != want {
		t.Fatalf("native file changed or unavailable: %v", err)
	}
}
