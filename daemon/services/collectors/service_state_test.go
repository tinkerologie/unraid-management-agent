package collectors

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTempCfg(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.cfg")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write temp cfg: %v", err)
	}
	return path
}

func TestReadFlatCfg(t *testing.T) {
	path := writeTempCfg(t, "# comment\nDOCKER_ENABLED=\"no\"\n\nKEY = \"value with spaces\"\nbare\n")
	cfg, err := readFlatCfg(path)
	if err != nil {
		t.Fatalf("readFlatCfg: %v", err)
	}
	if cfg["DOCKER_ENABLED"] != "no" {
		t.Errorf("DOCKER_ENABLED = %q, want %q", cfg["DOCKER_ENABLED"], "no")
	}
	if cfg["KEY"] != "value with spaces" {
		t.Errorf("KEY = %q, want %q", cfg["KEY"], "value with spaces")
	}
	if _, ok := cfg["bare"]; ok {
		t.Error("line without '=' should be skipped")
	}
}

func TestReadFlatCfgMissingFile(t *testing.T) {
	if _, err := readFlatCfg(filepath.Join(t.TempDir(), "nope.cfg")); err == nil {
		t.Error("expected error for missing file")
	}
}

func TestIsCfgTruthy(t *testing.T) {
	for _, v := range []string{"yes", "true", "1", "enable", "enabled", "ON", " Yes "} {
		if !isCfgTruthy(v) {
			t.Errorf("isCfgTruthy(%q) = false, want true", v)
		}
	}
	for _, v := range []string{"no", "false", "0", "disable", "", "maybe"} {
		if isCfgTruthy(v) {
			t.Errorf("isCfgTruthy(%q) = true, want false", v)
		}
	}
}

func TestDockerServiceDisabledAt(t *testing.T) {
	tests := []struct {
		name string
		body string
		want bool
	}{
		{"explicitly disabled", `DOCKER_ENABLED="no"`, true},
		{"enabled", `DOCKER_ENABLED="yes"`, false},
		{"key absent (conservative: not disabled)", `DOCKER_AUTOSTART="yes"`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := dockerServiceDisabledAt(writeTempCfg(t, tt.body)); got != tt.want {
				t.Errorf("dockerServiceDisabledAt = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDockerServiceDisabledAtMissingFile(t *testing.T) {
	if dockerServiceDisabledAt(filepath.Join(t.TempDir(), "nope.cfg")) {
		t.Error("missing config must be treated as not-disabled (conservative)")
	}
}

func TestVMServiceDisabledAt(t *testing.T) {
	tests := []struct {
		name string
		body string
		want bool
	}{
		{"service enabled", `SERVICE="enable"`, false},
		{"service disabled", `SERVICE="disable"`, true},
		{"disable flag wins", "SERVICE=\"enable\"\nDISABLE=\"yes\"", true},
		{"neither key (conservative: not disabled)", `FOO="bar"`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := vmServiceDisabledAt(writeTempCfg(t, tt.body)); got != tt.want {
				t.Errorf("vmServiceDisabledAt = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestVMServiceDisabledAtMissingFile(t *testing.T) {
	if vmServiceDisabledAt(filepath.Join(t.TempDir(), "nope.cfg")) {
		t.Error("missing config must be treated as not-disabled (conservative)")
	}
}
