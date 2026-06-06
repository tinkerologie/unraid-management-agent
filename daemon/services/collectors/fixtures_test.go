package collectors

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/lib"
)

// TestFixturesParseAndValidate runs the array shape validator against every
// captured (sanitized) Unraid state fixture. Adding a future Unraid version is
// just dropping a new testdata/fixtures/unraid-<ver>/ directory.
func TestFixturesParseAndValidate(t *testing.T) {
	dirs, err := filepath.Glob("testdata/fixtures/unraid-*")
	if err != nil {
		t.Fatalf("glob fixtures: %v", err)
	}
	if len(dirs) == 0 {
		t.Skip("no fixtures captured yet")
	}
	for _, d := range dirs {
		parsed, err := lib.ParseINIFile(filepath.Join(d, "var.ini"))
		if err != nil {
			t.Fatalf("%s: parse var.ini: %v", d, err)
		}
		if ok, reason := validateRequiredKeys(parsed, "mdState", "mdNumDisks"); !ok {
			t.Errorf("%s: var.ini failed validation: %s", d, reason)
		}
	}
}

// TestFixturesBreakage is the crux of the OS-resilience guarantee: malformed or
// empty source data must be detected as NOT valid (degraded) WITHOUT panicking,
// so the collector can flag it instead of silently emitting healthy-looking
// empty data.
func TestFixturesBreakage(t *testing.T) {
	cases := map[string]string{
		"garbage":        "this is not ini at all\n%%%\n",
		"empty":          "",
		"missing keys":   "version=\"7.3.1\"\nNAME=\"Tower\"\n", // valid INI, but no array keys
		"null bytes":     "mdState=\x00STARTED\x00\n",
		"path traversal": "mdState=\"../../etc/passwd\"\nNAME=\"x\"\n", // no mdNumDisks
		"huge value":     "mdState=\"" + string(make([]byte, 100000)) + "\"\n",
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			tmp := filepath.Join(t.TempDir(), "var.ini")
			if err := os.WriteFile(tmp, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			parsed, err := lib.ParseINIFile(tmp)
			if err != nil {
				// A parse error is an acceptable "unavailable" signal — must not panic.
				return
			}
			if ok, _ := validateRequiredKeys(parsed, "mdState", "mdNumDisks"); ok {
				t.Errorf("expected validation to FAIL on %q input, but it passed", name)
			}
		})
	}
}
