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
