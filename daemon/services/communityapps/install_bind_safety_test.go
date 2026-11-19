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
	"golang.org/x/sys/unix"

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
	if source, err := host.canonicalBindSource(remote + "/data"); err != nil || source != remote+"/data" {
		t.Fatalf("blocked lookup did not preserve the recorded Docker source: %q, %v", source, err)
	}
	if err := os.Symlink("loop", filepath.Join(root, "mnt/cache/application-data/loop")); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := host.existingAncestor("/mnt/cache/application-data/loop"); !errors.Is(err, ErrStorageState) {
		t.Fatalf("symlink cycle was not bounded: %v", err)
	}
}

func TestBindSourceRejectsAmbiguousSymlinkTargets(t *testing.T) {
	root := communityappstest.CopyHeimdallHost(t)
	for _, name := range []string{"mnt/cache/data", "mnt/cache/real/data", "mnt/cache/real/sub"} {
		if err := os.MkdirAll(filepath.Join(root, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for name, target := range map[string]string{
		"alias": "real/sub",
		"app":   "alias/../data",
	} {
		if err := os.Symlink(target, filepath.Join(root, "mnt/cache", name)); err != nil {
			t.Fatal(err)
		}
	}
	host, err := readInstallHost(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, issue := host.resolveSource("/mnt/cache/app"); issue != "unsafe_mount_source" {
		t.Fatalf("a link that must follow another link before traversing its parent returned %q", issue)
	}
	for _, source := range []string{"/mnt/cache/app", "/mnt/cache/alias/../data", "/mnt/cache/alias/.."} {
		existing := []container.Summary{{Mounts: []container.MountPoint{{Type: mount.TypeBind, RW: true, Source: source}}}}
		config := Configuration{Name: "fixture", Mounts: []Mount{{Source: "/mnt/cache/real/data/fixture", Mode: "rw"}}}
		if err := host.checkConflicts(&config, PrepareRequest{}, existing); !errors.Is(err, ErrStorageState) {
			t.Fatalf("ambiguous existing writable bind %q returned %v", source, err)
		}
		config.Mounts = nil
		if err := host.checkConflicts(&config, PrepareRequest{}, existing); err != nil {
			t.Fatalf("app without mounts was blocked by unrelated bind %q: %v", source, err)
		}
	}
}

func TestBindSourceAcceptsOrdinarySymlinkTargets(t *testing.T) {
	root := communityappstest.CopyHeimdallHost(t)
	host, err := readInstallHost(root)
	if err != nil {
		t.Fatal(err)
	}
	const expected = "/mnt/cache/application-data"
	for i, target := range []string{
		"./application-data", "application-data/", "application-data/.",
		"/mnt//cache/./application-data", "../cache/application-data",
	} {
		alias := fmt.Sprintf("/mnt/cache/alias-%d", i)
		if err := os.Symlink(target, filepath.Join(root, alias)); err != nil {
			t.Fatal(err)
		}
		if mount, _, issue := host.resolveSource(alias); issue != "" || mount.Source != expected {
			t.Fatalf("ordinary link %q returned %+v, issue %q", target, mount, issue)
		}
		if canonical, err := host.canonicalBindSource(alias); err != nil || canonical != expected {
			t.Fatalf("existing writable alias %q returned %q, %v", target, canonical, err)
		}
	}
	communityappstest.WriteFile(t, root, "mnt/cache/regular", "fixture data")
	communityappstest.WriteFile(t, root, "mnt/cache/application-data/regular", "fixture data")
	for alias, target := range map[string]string{
		"leaf-link":             "regular",
		"directory-link":        "leaf-link/",
		"intermediate":          "application-data",
		"intermediate-file-dir": "intermediate/regular/",
	} {
		if err := os.Symlink(target, filepath.Join(root, "mnt/cache", alias)); err != nil {
			t.Fatal(err)
		}
	}
	for _, alias := range []string{"directory-link", "intermediate-file-dir"} {
		if _, _, issue := host.resolveSource("/mnt/cache/" + alias); issue != "unsafe_mount_source" {
			t.Fatalf("link %q requiring a directory accepted a regular file: %q", alias, issue)
		}
	}
}

func TestBindLookupFailsClosedWhenFilesystemChanges(t *testing.T) {
	root := communityappstest.CopyHeimdallHost(t)
	host, err := readInstallHost(root)
	if err != nil {
		t.Fatal(err)
	}
	communityappstest.WriteFile(t, root, "outside/fixture", "existing data")
	alias := filepath.Join(root, "mnt/cache/application-data/alias")
	if err := os.Symlink("/outside/fixture", alias); err != nil {
		t.Fatal(err)
	}
	if _, _, issue := host.resolveSource("/mnt/cache/application-data/alias"); issue != "unsafe_mount_source" {
		t.Fatalf("a link to a regular file outside storage returned %q", issue)
	}
	protected := filepath.Join(root, "mnt/cache/application-data")
	if err := os.Chmod(protected, 0o777); err != nil {
		t.Fatal(err)
	}
	if protectedBindParents(root, "/mnt/cache/application-data/subdirectory") {
		t.Fatal("an unprotected intermediate directory passed the ancestor check")
	}
	if _, _, _, err := host.existingAncestor("relative"); !errors.Is(err, ErrStorageState) {
		t.Fatalf("relative path returned %v", err)
	}
	if got, anchor, _, err := host.existingAncestor("/"); got != "/" || anchor != "/" || err != nil {
		t.Fatalf("root lookup returned %q, %q, %v", got, anchor, err)
	}
	missingRoot := installHost{root: filepath.Join(root, "missing")}
	if _, _, _, err := missingRoot.existingAncestor("/"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing root returned %v", err)
	}

	for _, fault := range []string{"root metadata", "entry metadata", "entry open"} {
		t.Run(fault, func(t *testing.T) {
			ops := bindLookupOps{statRoot: unix.Fstat, statAt: unix.Fstatat, openAt: unix.Openat}
			switch fault {
			case "root metadata":
				ops.statRoot = func(int, *unix.Stat_t) error { return unix.EIO }
			case "entry metadata":
				ops.statAt = func(int, string, *unix.Stat_t, int) error { return unix.EIO }
			case "entry open":
				ops.openAt = func(int, string, int, uint32) (int, error) { return -1, unix.ELOOP }
			}
			if _, _, _, err := host.lookupAncestor("/mnt/cache/application-data", ops); err == nil {
				t.Fatalf("bind lookup accepted %s failure", fault)
			}
		})
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

func TestBindOverlapRecognizesAlternateLinuxMountRoots(t *testing.T) {
	host := installHost{mounts: []hostMount{
		{point: "/mnt/cache", fsType: "btrfs", id: "1:8:1", root: "/", storage: true},
		{point: "/srv/alias", fsType: "btrfs", id: "2:8:1", root: "/appdata"},
		{point: "/srv/other", fsType: "btrfs", id: "3:8:2", root: "/appdata"},
	}}
	const source = "/mnt/cache/appdata/fixture"
	for _, tc := range []struct {
		existing        string
		overlap, parent bool
	}{
		{"/srv/alias", true, true},
		{"/srv/alias/fixture", true, false},
		{"/srv/alias/fixture/child", true, false},
		{"/srv/alias/unrelated", false, false},
		{"/srv/other", false, false},
	} {
		if overlap, parent := host.bindOverlap(source, tc.existing); overlap != tc.overlap || parent != tc.parent {
			t.Errorf("alias %q gave overlap=%t parent=%t", tc.existing, overlap, parent)
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
