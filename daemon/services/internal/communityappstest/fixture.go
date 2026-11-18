// Package communityappstest supplies a synthetic CA host root to service tests.
package communityappstest

import (
	"os"
	"path/filepath"
	"testing"
)

const CachePath = "tmp/community.applications/tempFiles/templates_new.json"

// CopyHost copies the synthetic communityapps fixture into a test-owned root.
func CopyHost(t testing.TB) string {
	t.Helper()
	root := t.TempDir()
	fixture := os.DirFS("../communityapps/testdata/ca-2026.07.21")
	if err := os.CopyFS(root, fixture); err != nil {
		t.Fatal(err)
	}
	cache := filepath.Join(root, CachePath)
	// CA uses a .json suffix for PHP serialization. The source fixture uses
	// .txt so repository tooling handles it as a byte-exact capture.
	if err := os.Rename(cache+".txt", cache); err != nil {
		t.Fatal(err)
	}
	return root
}

// CopyHeimdallHost supplies the scrubbed supported listing and host defaults.
func CopyHeimdallHost(t testing.TB) string {
	t.Helper()
	root := CopyHost(t)
	catalog, err := os.ReadFile("../communityapps/testdata/heimdall-2026.07.21.serialized.txt")
	if err != nil {
		t.Fatal(err)
	}
	WriteFile(t, root, CachePath, string(catalog))
	for file, content := range map[string]string{
		"boot/config/plugins/community.applications/accepted": "",
		"boot/config/docker.cfg":                              "DOCKER_APP_CONFIG_PATH=\"/mnt/cache/application-data/\"\n",
		"proc/self/mountinfo":                                 "1 0 8:1 / /mnt/cache rw - btrfs /dev/sdb1 rw\n",
	} {
		WriteFile(t, root, file, content)
	}
	if err := os.MkdirAll(filepath.Join(root, "mnt/cache/application-data"), 0750); err != nil {
		t.Fatal(err)
	}
	return root
}

// WriteFile writes an Unraid system fixture into the test-owned root.
func WriteFile(t testing.TB, root, file, content string) {
	t.Helper()
	dir, err := os.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = dir.Close() }()
	if err := dir.MkdirAll(filepath.Dir(file), 0750); err != nil {
		t.Fatal(err)
	}
	if err := dir.WriteFile(file, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}
