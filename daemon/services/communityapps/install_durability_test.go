package communityapps

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestRetrySyncsParentOfExistingInstallDirectory(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can open a parent directory without read permission")
	}
	root := t.TempDir()
	parent := filepath.Join(root, "unreadable")
	if err := os.Mkdir(parent, 0o300); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(parent, 0o700) })
	relative := filepath.Join("unreadable", "attempts")
	for range 2 {
		if err := ensureInstallDirectory(root, relative, 0o700); !errors.Is(err, fs.ErrPermission) {
			t.Fatalf("directory setup did not require a readable parent to sync: %v", err)
		}
		if info, err := os.Stat(filepath.Join(root, relative)); err != nil || !info.IsDir() {
			t.Fatalf("failed sync did not leave an existing directory to retry: %v", err)
		}
	}
	if err := os.Chmod(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := ensureInstallDirectory(root, relative, 0o700); err != nil {
		t.Fatalf("sync could not recover when the parent was readable: %v", err)
	}
}

func TestReceiptRejectsOversizedProgressWithoutPoisoningRestart(t *testing.T) {
	root := t.TempDir()
	service := NewService(nil, ServiceOptions{HostRoot: root})
	receipt := installReceipt{
		Version: installReceiptVersion, PlanID: rand.Text(), Owner: rand.Text(),
		Attempt: Attempt{ID: rand.Text(), State: AttemptPending, Phase: "queued", Configuration: Configuration{Name: "fixture"}},
	}
	if err := service.saveReceipt(receipt, true); err != nil {
		t.Fatal(err)
	}
	progress := receipt
	progress.Attempt.Problem = &Problem{Code: "fixture", Message: strings.Repeat("a", maxInstallReceipt)}
	if err := service.saveReceipt(progress, false); !errors.Is(err, ErrAttemptStorage) {
		t.Fatalf("oversized progress returned %v", err)
	}
	reader := NewService(nil, ServiceOptions{HostRoot: root})
	got, err := reader.GetAttempt(receipt.Attempt.ID)
	if err != nil || got.Phase != "queued" || got.Problem != nil {
		t.Fatalf("oversized progress damaged a readable receipt: %+v, %v", got, err)
	}
}

func TestAppdataCreationRejectsSymlinkAtPreviouslyCreatedPrefix(t *testing.T) {
	root := t.TempDir()
	service := NewService(nil, ServiceOptions{HostRoot: root})
	for _, location := range []string{"mnt/appdata", "mnt/unrelated"} {
		if err := os.MkdirAll(filepath.Join(root, location), 0o750); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink("../unrelated", filepath.Join(root, "mnt/appdata/missing")); err != nil {
		t.Fatal(err)
	}
	anchor, err := os.Stat(filepath.Join(root, "mnt/appdata"))
	if err != nil {
		t.Fatal(err)
	}
	stat := anchor.Sys().(*syscall.Stat_t)
	identity := mountIdentity{anchor: "/mnt/appdata", device: fmt.Sprint(stat.Dev), inode: stat.Ino}
	dir, err := os.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = dir.Close() })
	created := map[string]bool{"/mnt/appdata/missing": true}
	err = service.createAppdata(dir, identity, "/mnt/appdata/missing/nested", created)
	if !errors.Is(err, ErrPlanChanged) {
		t.Fatalf("appdata creation did not refuse a substituted prefix: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "mnt/unrelated/nested")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("appdata creation changed an unrelated directory: %v", err)
	}
}
