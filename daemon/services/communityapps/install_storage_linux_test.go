package communityapps

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"

	"golang.org/x/sys/unix"
)

func TestNativeAppdataCreationKeepsFilesystemCredentialsScoped(t *testing.T) {
	directory := t.TempDir()
	if err := os.Chmod(directory, 0o777); err != nil {
		t.Fatal(err)
	}
	parent, err := os.Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = parent.Close() })

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	uid, err := unix.SetfsuidRetUid(-1)
	if err != nil {
		t.Fatal(err)
	}
	gid, err := unix.SetfsgidRetGid(-1)
	if err != nil {
		t.Fatal(err)
	}

	mkdirErr := mkdirInstallDirectory(parent, "installation", true)
	actualUID, uidErr := unix.SetfsuidRetUid(-1)
	actualGID, gidErr := unix.SetfsgidRetGid(-1)
	if uidErr != nil || gidErr != nil || actualUID != uid || actualGID != gid {
		t.Fatal("native appdata creation changed the caller thread's filesystem credentials")
	}

	info, statErr := os.Lstat(filepath.Join(directory, "installation"))
	if mkdirErr != nil {
		// An ordinary CI user cannot assume Unraid's appdata owner.
		if !errors.Is(mkdirErr, ErrStorageState) || !errors.Is(statErr, fs.ErrNotExist) {
			t.Fatalf("unavailable native identity must fail without creating a directory: mkdir=%v stat=%v", mkdirErr, statErr)
		}
		return
	}
	if statErr != nil || !info.IsDir() {
		t.Fatalf("missing native appdata directory: %v", statErr)
	}
	owner, ok := info.Sys().(*syscall.Stat_t)
	if !ok || owner.Uid != 99 || owner.Gid != 100 || info.Mode().Perm()&0o200 == 0 {
		t.Fatal("new appdata directory is not writable by the native UID and GID")
	}
}
