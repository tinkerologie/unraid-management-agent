package lib

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// withExternalFanRoots points the detection at temp dirs and restores the real
// paths afterwards.
func withExternalFanRoots(t *testing.T, emhttp, flash, proc string) {
	t.Helper()
	prevEmhttp, prevFlash, prevProc := emhttpPluginsDir, flashPluginsDir, procDirPath
	emhttpPluginsDir, flashPluginsDir, procDirPath = emhttp, flash, proc
	t.Cleanup(func() {
		emhttpPluginsDir, flashPluginsDir, procDirPath = prevEmhttp, prevFlash, prevProc
	})
}

// installPlugin creates the emhttp plugin dir (marks it "installed").
func installPlugin(t *testing.T, emhttp, dir string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(emhttp, dir), 0o755); err != nil {
		t.Fatal(err)
	}
}

// writeCfg writes a plugin .cfg file with the given service value.
func writeCfg(t *testing.T, flash, dir, name, service string) {
	t.Helper()
	cfgDir := filepath.Join(flash, dir)
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "controller=\"\"\nservice=\"" + service + "\"\npwm=\"102\"\n"
	if err := os.WriteFile(filepath.Join(cfgDir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// writeProc creates a fake /proc/<pid>/cmdline entry.
func writeProc(t *testing.T, proc, pid, cmdline string) {
	t.Helper()
	pidDir := filepath.Join(proc, pid)
	if err := os.MkdirAll(pidDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// real cmdline is NUL-separated argv
	if err := os.WriteFile(filepath.Join(pidDir, "cmdline"), []byte(cmdline), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDetectExternalFanControl(t *testing.T) {
	tests := []struct {
		name            string
		setup           func(t *testing.T, emhttp, flash, proc string)
		wantActive      bool
		wantControllers []string
	}{
		{
			name:            "nothing installed",
			setup:           func(t *testing.T, emhttp, flash, proc string) {},
			wantActive:      false,
			wantControllers: nil,
		},
		{
			name: "fanctrlplus installed and enabled via cfg",
			setup: func(t *testing.T, emhttp, flash, proc string) {
				installPlugin(t, emhttp, "fanctrlplus")
				writeCfg(t, flash, "fanctrlplus", "fanctrlplus_temp_0.cfg", "1")
			},
			wantActive:      true,
			wantControllers: []string{"FanCTRL Plus"},
		},
		{
			name: "fanctrlplus installed but disabled cfg",
			setup: func(t *testing.T, emhttp, flash, proc string) {
				installPlugin(t, emhttp, "fanctrlplus")
				writeCfg(t, flash, "fanctrlplus", "fanctrlplus_temp_0.cfg", "0")
			},
			wantActive:      false,
			wantControllers: nil,
		},
		{
			name: "fanctrlplus disabled in cfg but control process still running",
			setup: func(t *testing.T, emhttp, flash, proc string) {
				installPlugin(t, emhttp, "fanctrlplus")
				writeCfg(t, flash, "fanctrlplus", "fanctrlplus_temp_0.cfg", "0")
				writeProc(t, proc, "5151", "/bin/bash\x00/usr/local/emhttp/plugins/fanctrlplus/scripts/fanctrlplus_loop.sh\x00")
			},
			wantActive:      true,
			wantControllers: []string{"FanCTRL Plus"},
		},
		{
			name: "autofan enabled cfg but NOT installed is ignored",
			setup: func(t *testing.T, emhttp, flash, proc string) {
				writeCfg(t, flash, "dynamix.system.autofan", "dynamix.system.autofan.cfg", "1")
			},
			wantActive:      false,
			wantControllers: nil,
		},
		{
			name: "autofan installed and enabled via running process",
			setup: func(t *testing.T, emhttp, flash, proc string) {
				installPlugin(t, emhttp, "dynamix.system.autofan")
				writeProc(t, proc, "4242", "/bin/bash\x00/usr/local/emhttp/plugins/dynamix.system.autofan/scripts/autofan\x00-c\x00")
				writeProc(t, proc, "self", "ignored") // non-numeric dir must be skipped
			},
			wantActive:      true,
			wantControllers: []string{"Dynamix Auto Fan Control"},
		},
		{
			name: "both plugins active",
			setup: func(t *testing.T, emhttp, flash, proc string) {
				installPlugin(t, emhttp, "fanctrlplus")
				writeCfg(t, flash, "fanctrlplus", "fanctrlplus_temp_0.cfg", "1")
				installPlugin(t, emhttp, "dynamix.system.autofan")
				writeCfg(t, flash, "dynamix.system.autofan", "dynamix.system.autofan.cfg", "1")
			},
			wantActive:      true,
			wantControllers: []string{"FanCTRL Plus", "Dynamix Auto Fan Control"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			emhttp := filepath.Join(t.TempDir(), "emhttp")
			flash := filepath.Join(t.TempDir(), "flash")
			proc := filepath.Join(t.TempDir(), "proc")
			withExternalFanRoots(t, emhttp, flash, proc)
			tt.setup(t, emhttp, flash, proc)

			got := DetectExternalFanControl()

			if got.Active != tt.wantActive {
				t.Errorf("Active = %v, want %v", got.Active, tt.wantActive)
			}
			if !slices.Equal(got.Controllers, tt.wantControllers) {
				t.Errorf("Controllers = %v, want %v", got.Controllers, tt.wantControllers)
			}
		})
	}
}
