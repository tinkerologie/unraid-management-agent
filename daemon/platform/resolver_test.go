package platform

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPathExists(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "present")
	if err := os.WriteFile(f, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !PathExists(f) {
		t.Error("expected present file to exist")
	}
	if PathExists(filepath.Join(dir, "missing")) {
		t.Error("expected missing file to be absent")
	}
}
