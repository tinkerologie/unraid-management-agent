package communityapps

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"

	"github.com/tinkerologie/unraid-management-agent/daemon/services/internal/communityappstest"
)

func TestBindSourceRequiresProtectedAncestors(t *testing.T) {
	root, err := filepath.EvalSymlinks(communityappstest.CopyHeimdallHost(t))
	if err != nil {
		t.Fatal(err)
	}
	host, err := readInstallHost(root)
	if err != nil {
		t.Fatal(err)
	}
	const parent = "/mnt/cache/application-data"
	const source = parent + "/nested/fixture"
	resolved, _, issue := host.resolveSource(source)
	if issue != "" || resolved.Source != source || !resolved.CreateDirectory {
		t.Fatalf("protected storage returned %+v, issue %q", resolved, issue)
	}
	parentDir := filepath.Join(root, parent)
	if err := os.Chmod(parentDir, 0o775); err != nil {
		t.Fatal(err)
	}
	if _, _, issue := host.resolveSource(source); issue != "unprotected_mount_source" {
		t.Fatalf("a group-writable ancestor returned %q", issue)
	}
	if err := os.Chmod(parentDir, 0o755); err != nil {
		t.Fatal(err)
	}
	leaf := filepath.Join(root, source)
	if err := os.MkdirAll(leaf, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(leaf, 0o777); err != nil {
		t.Fatal(err)
	}
	resolved, _, issue = host.resolveSource(source)
	if issue != "" || resolved.CreateDirectory {
		t.Fatalf("app-writable leaf in protected storage returned %+v, issue %q", resolved, issue)
	}
	if protectedBindParents(root, parent+"/missing") || protectedBindParents(root, source) {
		t.Fatal("missing and world-writable directories did not fail the parent check")
	}
	host.mounts[0].fsType = "fuse.shfs"
	if _, _, issue := host.resolveSource(source); issue != "unprotected_mount_source" {
		t.Fatalf("a user-share alias returned %q", issue)
	}
}

func TestMountLookupStopsAtRemoteFilesystems(t *testing.T) {
	root, err := filepath.EvalSymlinks(communityappstest.CopyHeimdallHost(t))
	if err != nil {
		t.Fatal(err)
	}
	communityappstest.WriteFile(t, root, "proc/self/mountinfo",
		"1 0 8:1 / /mnt/cache rw - btrfs /dev/sdb1 rw\n"+
			"2 0 0:12 / /mnt/remotes/server rw - nfs4 example.invalid:/export rw\n")
	host, err := readInstallHost(root)
	if err != nil {
		t.Fatal(err)
	}
	const remote = "/mnt/remotes/server"
	if _, _, issue := host.resolveSource(remote + "/data"); issue != "unprotected_mount_source" {
		t.Fatalf("a hard remote mount returned %q", issue)
	}
	if err := os.MkdirAll(filepath.Join(root, remote), 0o755); err != nil {
		t.Fatal(err)
	}
	link := "/mnt/cache/application-data/linked"
	if err := os.Symlink(remote, filepath.Join(root, link)); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := host.existingAncestor(link + "/data"); !errors.Is(err, ErrStorageState) {
		t.Fatalf("local symlink allowed a remote lookup: %v", err)
	}
	if source := host.canonicalBindSource(remote + "/data"); source != remote+"/data" {
		t.Fatalf("blocked lookup did not preserve the recorded Docker source: %q", source)
	}
	if err := os.Symlink("loop", filepath.Join(root, "mnt/cache/application-data/loop")); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := host.existingAncestor("/mnt/cache/application-data/loop"); !errors.Is(err, ErrStorageState) {
		t.Fatalf("symlink cycle was not bounded: %v", err)
	}
}

func TestBindConflictsRejectParentSharingEvenWhenAcknowledged(t *testing.T) {
	root := communityappstest.CopyHeimdallHost(t)
	host, err := readInstallHost(root)
	if err != nil {
		t.Fatal(err)
	}
	const parent = "/mnt/cache/application-data"
	const leaf = parent + "/fixture"
	request := PrepareRequest{Paths: map[string]PathOverride{"/config": {Source: leaf, AllowShared: true}}}
	external := []container.Summary{{Mounts: []container.MountPoint{{Type: mount.TypeBind, RW: true, Source: parent}}}}
	config := Configuration{Name: "fixture", Mounts: []Mount{{Source: leaf, Target: "/config", Mode: "ro"}}}
	if err := host.checkConflicts(&config, request, external); !errors.Is(err, ErrExposedMountParent) {
		t.Fatalf("writable parent of read-only source returned %v", err)
	}
	external[0].Mounts[0].Source = leaf
	config.Mounts[0].Mode = "rw"
	if err := host.checkConflicts(&config, request, external); err != nil || !config.Mounts[0].Shared {
		t.Fatalf("explicit sharing of the protected leaf returned %v, shared=%t", err, config.Mounts[0].Shared)
	}
	config.Mounts = append(config.Mounts, Mount{Source: parent, Target: "/data", Mode: "rw"})
	if err := host.checkConflicts(&config, request, nil); !errors.Is(err, ErrExposedMountParent) {
		t.Fatalf("writable parent in the same install returned %v", err)
	}
}

func TestBindOverlapRecognizesWritableUserShareParents(t *testing.T) {
	host := installHost{mounts: []hostMount{
		{point: "/mnt/cache", fsType: "btrfs", storage: true},
		{point: "/mnt/user", fsType: "fuse.shfs", storage: true},
		{point: "/mnt/disks/external", fsType: "ext4", storage: true},
	}}
	for _, tc := range []struct {
		wanted, existing string
		overlap, parent  bool
	}{
		{"/mnt/cache/appdata/fixture", "/mnt/user/appdata", true, true},
		{"/mnt/cache/appdata/fixture", "/mnt/user/appdata/fixture", true, false},
		{"/mnt/cache/appdata", "/mnt/user/appdata/fixture", true, false},
		{"/mnt/cache/appdata", "/mnt/cache/appdata/fixture", true, false},
		{"/mnt/disks/external/fixture", "/mnt/user/appdata", false, false},
	} {
		if overlap, parent := host.bindOverlap(tc.wanted, tc.existing); overlap != tc.overlap || parent != tc.parent {
			t.Errorf("unexpected relation between %q and %q: overlap=%t parent=%t", tc.wanted, tc.existing, overlap, parent)
		}
	}
}

func TestAppdataCreatesProtectedParentsAndWritableLeaf(t *testing.T) {
	root := communityappstest.CopyHeimdallHost(t)
	const parent = "/mnt/cache/application-data"
	anchor, err := os.Stat(filepath.Join(root, parent))
	if err != nil {
		t.Fatal(err)
	}
	stat := anchor.Sys().(*syscall.Stat_t)
	identity := mountIdentity{anchor: parent, device: fmt.Sprint(stat.Dev), inode: stat.Ino}
	dir, err := os.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = dir.Close() })
	service := NewService(nil, ServiceOptions{HostRoot: root})
	t.Cleanup(service.Close)
	created := make(map[string]mountIdentity)
	for _, source := range []string{parent + "/shared/first", parent + "/shared/second"} {
		id, err := service.createAppdata(dir, identity, source, created)
		if err != nil {
			t.Fatal(err)
		}
		if !protectedBindParents(root, filepath.Dir(source)) {
			t.Fatalf("new parent of %q is not protected", source)
		}
		info, err := dir.Stat(strings.TrimPrefix(source, "/"))
		if err != nil || !info.IsDir() || info.Mode().Perm()&0o200 == 0 || !hasMountIdentity(info, id) {
			t.Fatalf("new directory %q is not owner-writable: %v", source, err)
		}
	}
	if uid, gid := appdataOwnership("/"); uid != 99 || gid != 100 {
		t.Fatalf("native appdata owner was %d:%d", uid, gid)
	}
	if err := os.Chmod(filepath.Join(root, parent, "shared"), 0o777); err != nil {
		t.Fatal(err)
	}
	if _, err := service.createAppdata(dir, identity, parent+"/shared/third", created); !errors.Is(err, ErrPlanChanged) {
		t.Fatalf("appdata creation accepted an unprotected intermediate directory: %v", err)
	}
}

func TestNativeNameUsesDockerValidationAndTemplateLength(t *testing.T) {
	for _, name := range []string{"fixture-1.2", strings.Repeat("a", 128)} {
		if !validNativeName(name) {
			t.Errorf("rejected a native container name of length %d", len(name))
		}
	}
	for _, name := range []string{"", "a..b", "a/b", "a\x00b", strings.Repeat("a", 129)} {
		if validNativeName(name) {
			t.Errorf("accepted invalid native container name %q", name)
		}
	}
}

func TestAppdataPublishPreservesAnExistingDirectory(t *testing.T) {
	root := t.TempDir()
	communityappstest.WriteFile(t, root, "prepared/keep", "appdata")
	parent, err := os.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = parent.Close() })
	file, _, err := stageAppdataComponent(parent, "prepared", "/mnt/cache/prepared", hostMount{})
	if file != nil || !errors.Is(err, ErrPlanChanged) {
		t.Fatalf("publication accepted an existing directory: %v", err)
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 1 || entries[0].Name() != "prepared" {
		t.Fatalf("lost the original directory or leaked the staging directory: %+v, %v", entries, err)
	}
	if data, err := os.ReadFile(filepath.Join(root, "prepared/keep")); err != nil || string(data) != "appdata" {
		t.Fatalf("publication changed existing contents: %v", err)
	}
}
