package lib

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadTurboBoost_Intel(t *testing.T) {
	tmpDir := t.TempDir()
	intelDir := filepath.Join(tmpDir, "intel_pstate")
	if err := os.MkdirAll(intelDir, 0o755); err != nil {
		t.Fatal(err)
	}
	turboFile := filepath.Join(intelDir, "no_turbo")

	orig := intelTurboPath
	origAMD := amdBoostPath
	intelTurboPath = turboFile
	amdBoostPath = filepath.Join(tmpDir, "nonexistent", "boost")
	t.Cleanup(func() {
		intelTurboPath = orig
		amdBoostPath = origAMD
	})

	t.Run("turbo enabled (no_turbo=0)", func(t *testing.T) {
		if err := os.WriteFile(turboFile, []byte("0\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		status := ReadTurboBoost()
		if !status.Available {
			t.Error("expected Available=true")
		}
		if !status.Enabled {
			t.Error("expected Enabled=true when no_turbo=0")
		}
		if status.Vendor != "intel" {
			t.Errorf("Vendor = %q, want %q", status.Vendor, "intel")
		}
	})

	t.Run("turbo disabled (no_turbo=1)", func(t *testing.T) {
		if err := os.WriteFile(turboFile, []byte("1\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		status := ReadTurboBoost()
		if !status.Available {
			t.Error("expected Available=true")
		}
		if status.Enabled {
			t.Error("expected Enabled=false when no_turbo=1")
		}
	})
}

func TestReadTurboBoost_AMD(t *testing.T) {
	tmpDir := t.TempDir()
	amdDir := filepath.Join(tmpDir, "cpufreq")
	if err := os.MkdirAll(amdDir, 0o755); err != nil {
		t.Fatal(err)
	}
	boostFile := filepath.Join(amdDir, "boost")

	orig := intelTurboPath
	origAMD := amdBoostPath
	intelTurboPath = filepath.Join(tmpDir, "nonexistent", "no_turbo")
	amdBoostPath = boostFile
	t.Cleanup(func() {
		intelTurboPath = orig
		amdBoostPath = origAMD
	})

	t.Run("boost enabled (boost=1)", func(t *testing.T) {
		if err := os.WriteFile(boostFile, []byte("1\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		status := ReadTurboBoost()
		if !status.Available {
			t.Error("expected Available=true")
		}
		if !status.Enabled {
			t.Error("expected Enabled=true when boost=1")
		}
		if status.Vendor != "amd" {
			t.Errorf("Vendor = %q, want %q", status.Vendor, "amd")
		}
	})

	t.Run("boost disabled (boost=0)", func(t *testing.T) {
		if err := os.WriteFile(boostFile, []byte("0\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		status := ReadTurboBoost()
		if !status.Available {
			t.Error("expected Available=true")
		}
		if status.Enabled {
			t.Error("expected Enabled=false when boost=0")
		}
	})
}

func TestReadTurboBoost_NotAvailable(t *testing.T) {
	orig := intelTurboPath
	origAMD := amdBoostPath
	intelTurboPath = "/nonexistent/intel/no_turbo"
	amdBoostPath = "/nonexistent/amd/boost"
	t.Cleanup(func() {
		intelTurboPath = orig
		amdBoostPath = origAMD
	})

	status := ReadTurboBoost()
	if status.Available {
		t.Error("expected Available=false when no sysfs files exist")
	}
	if status.Enabled {
		t.Error("expected Enabled=false")
	}
	if status.Vendor != "" {
		t.Errorf("Vendor = %q, want empty", status.Vendor)
	}
}

func TestWriteTurboBoost_Intel(t *testing.T) {
	tmpDir := t.TempDir()
	intelDir := filepath.Join(tmpDir, "intel_pstate")
	if err := os.MkdirAll(intelDir, 0o755); err != nil {
		t.Fatal(err)
	}
	turboFile := filepath.Join(intelDir, "no_turbo")
	if err := os.WriteFile(turboFile, []byte("0\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	orig := intelTurboPath
	origAMD := amdBoostPath
	intelTurboPath = turboFile
	amdBoostPath = filepath.Join(tmpDir, "nonexistent", "boost")
	t.Cleanup(func() {
		intelTurboPath = orig
		amdBoostPath = origAMD
	})

	t.Run("disable turbo", func(t *testing.T) {
		if err := WriteTurboBoost(false); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		data, _ := os.ReadFile(turboFile)
		if string(data) != "1" {
			t.Errorf("expected no_turbo=1 after disabling, got %q", string(data))
		}
	})

	t.Run("enable turbo", func(t *testing.T) {
		if err := WriteTurboBoost(true); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		data, _ := os.ReadFile(turboFile)
		if string(data) != "0" {
			t.Errorf("expected no_turbo=0 after enabling, got %q", string(data))
		}
	})
}

func TestWriteTurboBoost_NotAvailable(t *testing.T) {
	orig := intelTurboPath
	origAMD := amdBoostPath
	intelTurboPath = "/nonexistent/intel/no_turbo"
	amdBoostPath = "/nonexistent/amd/boost"
	t.Cleanup(func() {
		intelTurboPath = orig
		amdBoostPath = origAMD
	})

	err := WriteTurboBoost(true)
	if err == nil {
		t.Error("expected error when no turbo/boost is available")
	}
}
