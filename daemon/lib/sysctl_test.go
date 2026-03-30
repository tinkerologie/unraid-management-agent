package lib

import (
	"os"
	"path/filepath"
	"testing"
)

// setupProcSys creates a temp /proc/sys tree and overrides procSysPath for the test.
func setupProcSys(t *testing.T, files map[string]string) string {
	t.Helper()
	tmpDir := t.TempDir()
	orig := procSysPath
	procSysPath = tmpDir
	t.Cleanup(func() { procSysPath = orig })

	for relPath, content := range files {
		absPath := filepath.Join(tmpDir, filepath.Join(filepath.SplitList(relPath)...))
		// Handle dot-notation paths (e.g. "vm/dirty_ratio")
		dir := filepath.Dir(absPath)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("failed to create dir %s: %v", dir, err)
		}
		if err := os.WriteFile(absPath, []byte(content), 0o644); err != nil {
			t.Fatalf("failed to write %s: %v", absPath, err)
		}
	}
	return tmpDir
}

func TestSysctlKeyToPath(t *testing.T) {
	orig := procSysPath
	defer func() { procSysPath = orig }()
	procSysPath = "/proc/sys"

	tests := []struct {
		name    string
		key     string
		want    string
		wantErr bool
	}{
		{"vm.dirty_ratio", "vm.dirty_ratio", "/proc/sys/vm/dirty_ratio", false},
		{"fs.inotify.max_user_watches", "fs.inotify.max_user_watches", "/proc/sys/fs/inotify/max_user_watches", false},
		{"single level", "kernel.hostname", "/proc/sys/kernel/hostname", false},
		{"empty key", "", "", true},
		{"path traversal", "vm..dirty_ratio", "", true},
		{"slash in segment", "vm/evil.dirty_ratio", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := sysctlKeyToPath(tt.key)
			if tt.wantErr {
				if err == nil {
					t.Errorf("sysctlKeyToPath(%q) expected error, got %q", tt.key, got)
				}
				return
			}
			if err != nil {
				t.Errorf("sysctlKeyToPath(%q) unexpected error: %v", tt.key, err)
				return
			}
			if got != tt.want {
				t.Errorf("sysctlKeyToPath(%q) = %q, want %q", tt.key, got, tt.want)
			}
		})
	}
}

func TestReadSysctl(t *testing.T) {
	setupProcSys(t, map[string]string{
		"vm/dirty_ratio": "20\n",
	})

	t.Run("existing key", func(t *testing.T) {
		val, err := ReadSysctl("vm.dirty_ratio")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if val != "20" {
			t.Errorf("ReadSysctl() = %q, want %q", val, "20")
		}
	})

	t.Run("non-existent key", func(t *testing.T) {
		_, err := ReadSysctl("vm.nonexistent")
		if err == nil {
			t.Error("expected error for non-existent key")
		}
	})
}

func TestReadSysctlInt(t *testing.T) {
	setupProcSys(t, map[string]string{
		"vm/dirty_ratio":     "20\n",
		"vm/dirty_bad_value": "abc\n",
	})

	t.Run("valid integer", func(t *testing.T) {
		val, err := ReadSysctlInt("vm.dirty_ratio")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if val != 20 {
			t.Errorf("ReadSysctlInt() = %d, want %d", val, 20)
		}
	})

	t.Run("non-integer value", func(t *testing.T) {
		_, err := ReadSysctlInt("vm.dirty_bad_value")
		if err == nil {
			t.Error("expected error for non-integer value")
		}
	})

	t.Run("non-existent key", func(t *testing.T) {
		_, err := ReadSysctlInt("vm.nonexistent")
		if err == nil {
			t.Error("expected error for non-existent key")
		}
	})
}

func TestWriteSysctl(t *testing.T) {
	setupProcSys(t, map[string]string{
		"vm/dirty_ratio": "20\n",
	})

	t.Run("write existing key", func(t *testing.T) {
		if err := WriteSysctl("vm.dirty_ratio", "30"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		val, err := ReadSysctl("vm.dirty_ratio")
		if err != nil {
			t.Fatalf("read after write failed: %v", err)
		}
		if val != "30" {
			t.Errorf("WriteSysctl roundtrip: got %q, want %q", val, "30")
		}
	})

	t.Run("write to non-existent dir", func(t *testing.T) {
		err := WriteSysctl("nonexistent.key", "1")
		if err == nil {
			t.Error("expected error writing to non-existent path")
		}
	})
}

func TestReadDiskCacheSettings(t *testing.T) {
	setupProcSys(t, map[string]string{
		"vm/dirty_background_ratio":    "10\n",
		"vm/dirty_ratio":               "20\n",
		"vm/dirty_writeback_centisecs": "500\n",
		"vm/dirty_expire_centisecs":    "3000\n",
	})

	dc, err := ReadDiskCacheSettings()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if dc.DirtyBackgroundRatio != 10 {
		t.Errorf("DirtyBackgroundRatio = %d, want 10", dc.DirtyBackgroundRatio)
	}
	if dc.DirtyRatio != 20 {
		t.Errorf("DirtyRatio = %d, want 20", dc.DirtyRatio)
	}
	if dc.DirtyWritebackCenti != 500 {
		t.Errorf("DirtyWritebackCenti = %d, want 500", dc.DirtyWritebackCenti)
	}
	if dc.DirtyExpireCenti != 3000 {
		t.Errorf("DirtyExpireCenti = %d, want 3000", dc.DirtyExpireCenti)
	}
}

func TestReadInotifySettings(t *testing.T) {
	setupProcSys(t, map[string]string{
		"fs/inotify/max_user_watches":   "524288\n",
		"fs/inotify/max_user_instances": "128\n",
		"fs/inotify/max_queued_events":  "16384\n",
	})

	ino, err := ReadInotifySettings()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if ino.MaxUserWatches != 524288 {
		t.Errorf("MaxUserWatches = %d, want 524288", ino.MaxUserWatches)
	}
	if ino.MaxUserInstances != 128 {
		t.Errorf("MaxUserInstances = %d, want 128", ino.MaxUserInstances)
	}
	if ino.MaxQueuedEvents != 16384 {
		t.Errorf("MaxQueuedEvents = %d, want 16384", ino.MaxQueuedEvents)
	}
}

func TestReadDiskCacheSettings_MissingFile(t *testing.T) {
	setupProcSys(t, map[string]string{
		"vm/dirty_ratio": "20\n",
		// Missing dirty_background_ratio
	})

	_, err := ReadDiskCacheSettings()
	if err == nil {
		t.Error("expected error when sysctl files are missing")
	}
}

func TestReadInotifySettings_MissingFile(t *testing.T) {
	setupProcSys(t, map[string]string{
		"fs/inotify/max_user_watches": "524288\n",
		// Missing other inotify files
	})

	_, err := ReadInotifySettings()
	if err == nil {
		t.Error("expected error when inotify files are missing")
	}
}
