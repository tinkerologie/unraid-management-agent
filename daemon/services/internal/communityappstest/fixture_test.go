package communityappstest

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fixtureFailure string

type fixtureFailureRecorder struct{}

func (fixtureFailureRecorder) Helper() {}

func (fixtureFailureRecorder) Fatalf(format string, args ...any) {
	panic(fixtureFailure(fmt.Sprintf(format, args...)))
}

func TestWriteFileRefusesSymlinkOutsideFixtureRoot(t *testing.T) {
	root := t.TempDir()
	out := filepath.Join(t.TempDir(), "outside.txt")
	const original = "outside fixture"
	if err := os.WriteFile(out, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(out, filepath.Join(root, "link.txt")); err != nil {
		t.Fatal(err)
	}
	func() {
		defer func() {
			failure, ok := recover().(fixtureFailure)
			if !ok || !strings.Contains(string(failure), "write fixture file") {
				t.Fatalf("expected a fixture write error, got %q", failure)
			}
		}()
		WriteFile(fixtureFailureRecorder{}, root, "link.txt", "replacement")
	}()
	if data, err := os.ReadFile(out); err != nil || string(data) != original {
		t.Fatalf("outside file changed to %q, error %v", data, err)
	}
}
