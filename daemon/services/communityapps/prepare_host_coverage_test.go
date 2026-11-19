package communityapps_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"syscall"
	"testing"

	"github.com/tinkerologie/unraid-management-agent/daemon/services/communityapps"
	"github.com/tinkerologie/unraid-management-agent/daemon/services/internal/communityappstest"
)

func TestPrepareFailsClosedOnUnsafeHostStorageState(t *testing.T) {
	const cache = "/mnt/cache/application-data"
	const mountInfo = "proc/self/mountinfo"

	for _, tc := range []struct {
		name   string
		change func(*testing.T, string, *communityapps.PrepareRequest)
		want   error
		issue  string
	}{
		{name: "unreadable hostname state", change: func(t *testing.T, root string, _ *communityapps.PrepareRequest) {
			const info = "var/local/emhttp/var.ini"
			communityappstest.WriteFile(t, root, info, "NAME=example")
			if err := os.Chmod(filepath.Join(root, info), 0o666); err != nil {
				t.Fatal(err)
			}
		}, want: communityapps.ErrStorageState},
		{name: "hostname unsupported by native Docker manager", change: func(t *testing.T, root string, _ *communityapps.PrepareRequest) {
			communityappstest.WriteFile(t, root, "var/local/emhttp/var.ini", "NAME=\"example$host\"")
		}, want: communityapps.ErrStorageState},
		{name: "malformed native hostname settings", change: func(t *testing.T, root string, _ *communityapps.PrepareRequest) {
			communityappstest.WriteFile(t, root, "var/local/emhttp/var.ini", "[")
		}, want: communityapps.ErrStorageState},
		{name: "malformed mount information", change: func(t *testing.T, root string, _ *communityapps.PrepareRequest) {
			communityappstest.WriteFile(t, root, mountInfo, "unexpected mount format\n")
		}, want: communityapps.ErrStorageState},
		{name: "two storage mounts at the same path", change: func(t *testing.T, root string, _ *communityapps.PrepareRequest) {
			communityappstest.WriteFile(t, root, mountInfo,
				"1 0 8:1 / /mnt/cache rw - btrfs /dev/sdb1 rw\n2 0 8:2 / /mnt/cache rw - btrfs /dev/sdb2 rw\n")
		}, issue: "unsafe_mount_source"},
		{name: "a native source escapes the host root", change: func(t *testing.T, root string, r *communityapps.PrepareRequest) {
			outside := t.TempDir()
			if err := os.Symlink(outside, filepath.Join(root, cache, "escape")); err != nil {
				t.Fatal(err)
			}
			r.Paths = map[string]communityapps.PathOverride{"/config": {Source: cache + "/escape"}}
		}, issue: "unsafe_mount_source"},
		{name: "bind mount imports a subdirectory", change: func(t *testing.T, root string, _ *communityapps.PrepareRequest) {
			communityappstest.WriteFile(t, root, mountInfo, "1 0 8:1 /untrusted /mnt/cache rw - btrfs /dev/sdb1 rw\n")
		}, issue: "unsafe_mount_source"},
		{name: "mount source is a broken link", change: func(t *testing.T, root string, r *communityapps.PrepareRequest) {
			if err := os.Symlink("missing", filepath.Join(root, cache, "broken")); err != nil {
				t.Fatal(err)
			}
			r.Paths = map[string]communityapps.PathOverride{"/config": {Source: cache + "/broken"}}
		}, issue: "unsafe_mount_source"},
		{name: "mount source traverses a regular file", change: func(t *testing.T, root string, r *communityapps.PrepareRequest) {
			communityappstest.WriteFile(t, root, "mnt/cache/application-data/plain-file", "fixture")
			r.Paths = map[string]communityapps.PathOverride{"/config": {Source: cache + "/plain-file/subdir"}}
		}, issue: "unsafe_mount_source"},
		{name: "mount source is a special file", change: func(t *testing.T, root string, r *communityapps.PrepareRequest) {
			if err := syscall.Mkfifo(filepath.Join(root, cache, "pipe"), 0o600); err != nil {
				t.Fatal(err)
			}
			r.Paths = map[string]communityapps.PathOverride{"/config": {Source: cache + "/pipe"}}
		}, issue: "unsafe_mount_source"},
		{name: "mount source resolves to Docker socket", change: func(t *testing.T, root string, r *communityapps.PrepareRequest) {
			communityappstest.WriteFile(t, root, "mnt/cache/application-data/docker.sock", "")
			if err := os.Symlink("docker.sock", filepath.Join(root, cache, "alias")); err != nil {
				t.Fatal(err)
			}
			r.Paths = map[string]communityapps.PathOverride{"/config": {Source: cache + "/alias"}}
		}, issue: "docker_socket_mount"},
		{name: "native template directory replaced by a file", change: func(t *testing.T, root string, _ *communityapps.PrepareRequest) {
			communityappstest.WriteFile(t, root, recoveryTemplateDir, "unexpected file")
		}, want: communityapps.ErrStorageState},
		{name: "template parent is a symlink loop", change: func(t *testing.T, root string, _ *communityapps.PrepareRequest) {
			if err := os.Symlink("dockerMan", filepath.Join(root, "boot/config/plugins/dockerMan")); err != nil {
				t.Fatal(err)
			}
		}, want: communityapps.ErrStorageState},
		{name: "existing saved template reserves its default mount", change: func(t *testing.T, root string, _ *communityapps.PrepareRequest) {
			communityappstest.WriteFile(t, root, recoveryTemplateDir+"/readme.txt", "non-template")
			communityappstest.WriteFile(t, root, recoveryTemplateDir+"/retained.xml",
				`<Container><Name>retained</Name><Config Type="Path" Mode="rw" Default="`+cache+`/heimdall"></Config></Container>`)
		}, want: communityapps.ErrSharedAppdata},
		{name: "saved template count exceeds inspection bound", change: func(t *testing.T, root string, _ *communityapps.PrepareRequest) {
			dir := filepath.Join(root, recoveryTemplateDir)
			if err := os.MkdirAll(dir, 0o700); err != nil {
				t.Fatal(err)
			}
			for i := range 1025 {
				filename := filepath.Join(dir, fmt.Sprintf("fixture-%04d.xml", i))
				if err := os.WriteFile(filename, []byte("<Container><Name>retained</Name></Container>"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
		}, want: communityapps.ErrStorageState},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := communityappstest.CopyHeimdallHost(t)
			service := newPlanningService(t, root, &communityappstest.Docker{}, nil)
			t.Cleanup(service.Close)
			req := communityapps.PrepareRequest{Reference: communityappstest.HeimdallReference}
			tc.change(t, root, &req)
			preview, err := service.Prepare(t.Context(), req)
			if tc.want != nil {
				if !errors.Is(err, tc.want) {
					t.Fatalf("Prepare returned %v, want %v", err, tc.want)
				}
				return
			}
			if err != nil || preview.Eligible || !slices.Contains(preview.Issues, tc.issue) {
				t.Fatalf("Prepare returned %+v, %v; want ineligible with %q", preview, err, tc.issue)
			}
		})
	}
}

func TestPrepareRecognizesMountedZFSDatasetAndIndependentRemoteStorage(t *testing.T) {
	root := communityappstest.CopyHeimdallHost(t)
	communityappstest.WriteFile(t, root, "proc/self/mountinfo",
		"1 0 8:1 / / rw - ext4 /dev/sda rw\n"+
			"2 1 9:0 / /mnt/user rw - fuse.shfs shfs rw\n"+
			"3 1 9:1 / /mnt/data rw - zfs data rw\n"+
			"4 3 9:2 / /mnt/data/child rw - zfs data/child rw\n"+
			"5 1 9:3 / /mnt/remotes/file-share rw - cifs //fixture/remote rw\n"+
			"6 1 9:4 / /mnt/remotes rw - cifs //fixture/remote rw\n"+
			"7 1 9:5 / /mnt/cache/nested rw - btrfs /dev/sdc rw\n"+
			"8 1 9:6 / /mnt/ephemeral rw - tmpfs tmpfs rw\n")
	communityappstest.WriteFile(t, root, recoveryTemplateDir+"/retained.xml",
		`<Container><Name>retained</Name><Config Type="Path" Mode="rw" Default="/mnt/remotes/file-share/shared"></Config></Container>`)
	for _, path := range []string{"mnt/data/child", "mnt/user/shared", "mnt/remotes/file-share/shared"} {
		if err := os.MkdirAll(filepath.Join(root, path), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	service := newPlanningService(t, root, &communityappstest.Docker{}, nil)
	t.Cleanup(service.Close)
	for _, source := range []string{"/mnt/data/child/export", "/mnt/user/shared"} {
		req := communityapps.PrepareRequest{
			Reference: communityappstest.HeimdallReference,
			Paths:     map[string]communityapps.PathOverride{"/config": {Source: source}},
		}
		preview, err := service.Prepare(t.Context(), req)
		if err != nil || !preview.Eligible || len(preview.Mounts) != 1 || preview.Mounts[0].Source != source || preview.Mounts[0].Shared {
			t.Fatalf("independent storage %s returned %+v, %v", source, preview, err)
		}
	}
}

func TestPrepareRejectsUnrepresentableDeclaredMountSource(t *testing.T) {
	root := communityappstest.CopyHeimdallHost(t)
	entry := testCatalogEntry()
	entry["Config"] = []any{publisherField("Path", "/data", "rw", "/mnt/cache/control\tchar", "", true)}
	req := prepareFixtureRequest(t, root, entry)
	service := newPlanningService(t, root, &communityappstest.Docker{}, nil)
	t.Cleanup(service.Close)
	preview, err := service.Prepare(t.Context(), req)
	if err != nil || preview.Eligible || !slices.Contains(preview.Issues, "unsafe_mount_source") {
		t.Fatalf("unrepresentable path returned %+v, %v", preview, err)
	}
}
