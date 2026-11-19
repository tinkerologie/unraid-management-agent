// Package communityappstest supplies a synthetic CA host root to service tests.
package communityappstest

import (
	"os"
	"path/filepath"
	"testing"
)

const CachePath = "tmp/community.applications/tempFiles/templates_new.json"

type fixtureAssertions interface {
	Helper()
	Fatalf(string, ...any)
}

// Require stops fixture setup when its prerequisites or state cannot be verified.
func Require(t fixtureAssertions, ok bool, format string, args ...any) {
	t.Helper()
	if !ok {
		t.Fatalf(format, args...)
	}
}

// CopyHost copies the synthetic communityapps fixture into a test-owned root.
func CopyHost(t testing.TB) string {
	t.Helper()
	root := t.TempDir()
	fixture := os.DirFS("../communityapps/testdata/ca-2026.07.21")
	err := os.CopyFS(root, fixture)
	Require(t, err == nil, "copy host fixture: %v", err)
	cache := filepath.Join(root, CachePath)
	// CA uses a .json suffix for PHP serialization. The source fixture uses
	// .txt so repository tooling handles it as a byte-exact capture.
	err = os.Rename(cache+".txt", cache)
	Require(t, err == nil, "rename catalog cache fixture: %v", err)
	return root
}

// CopyHeimdallHost supplies the scrubbed supported listing and host defaults.
func CopyHeimdallHost(t testing.TB) string {
	t.Helper()
	root := CopyHost(t)
	catalog, err := os.ReadFile("../communityapps/testdata/heimdall-2026.07.21.serialized.txt")
	Require(t, err == nil, "read Heimdall catalog fixture: %v", err)
	WriteFile(t, root, CachePath, string(catalog))
	for file, content := range map[string]string{
		"boot/config/plugins/community.applications/accepted": "",
		"boot/config/docker.cfg":                              "DOCKER_APP_CONFIG_PATH=\"/mnt/cache/application-data/\"\n",
		"proc/self/mountinfo":                                 "1 0 8:1 / /mnt/cache rw - btrfs /dev/sdb1 rw\n",
	} {
		WriteFile(t, root, file, content)
	}
	err = os.MkdirAll(filepath.Join(root, "mnt/cache/application-data"), 0750)
	Require(t, err == nil, "create appdata fixture: %v", err)
	return root
}

// WriteFile writes an Unraid system fixture into the test-owned root.
func WriteFile(t fixtureAssertions, root, file, content string) {
	t.Helper()
	dir, err := os.OpenRoot(root)
	Require(t, err == nil, "open fixture root: %v", err)
	defer func() { _ = dir.Close() }()
	err = dir.MkdirAll(filepath.Dir(file), 0750)
	Require(t, err == nil, "create fixture directory: %v", err)
	err = dir.WriteFile(file, []byte(content), 0600)
	Require(t, err == nil, "write fixture file: %v", err)
}
