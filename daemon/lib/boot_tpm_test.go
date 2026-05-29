package lib

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBaseBlockDevice(t *testing.T) {
	tests := []struct {
		part string
		want string
	}{
		{"sda1", "sda"},
		{"sdb", "sdb"},
		{"nvme0n1p1", "nvme0n1"},
		{"nvme0n1", "nvme0n1"},
		{"mmcblk0p1", "mmcblk0"},
		{"mmcblk0", "mmcblk0"},
	}
	for _, tt := range tests {
		if got := baseBlockDevice(tt.part); got != tt.want {
			t.Errorf("baseBlockDevice(%q) = %q, want %q", tt.part, got, tt.want)
		}
	}
}

func TestChassisTypeName(t *testing.T) {
	tests := []struct{ code, want string }{
		{"3", "Desktop"},
		{"7", "Tower"},
		{"23", "Rack Mount Chassis"},
		{"Tower", "Tower"}, // already-decoded dmidecode label passes through
		{"999", "999"},     // unknown code passes through
		{"", ""},
	}
	for _, tt := range tests {
		if got := chassisTypeName(tt.code); got != tt.want {
			t.Errorf("chassisTypeName(%q) = %q, want %q", tt.code, got, tt.want)
		}
	}
}

func TestReadTPMInfoAbsent(t *testing.T) {
	orig := sysClassTPM
	sysClassTPM = filepath.Join(t.TempDir(), "no-tpm")
	t.Cleanup(func() { sysClassTPM = orig })

	info := ReadTPMInfo()
	if info == nil {
		t.Fatal("ReadTPMInfo() returned nil")
	}
	if info.Present {
		t.Error("expected TPM not present when sysfs path is missing")
	}
}

func TestReadTPMInfoPresent(t *testing.T) {
	tmp := t.TempDir()
	dev := filepath.Join(tmp, "tpm0")
	if err := os.MkdirAll(dev, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dev, "tpm_version_major"), []byte("2\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	orig := sysClassTPM
	sysClassTPM = tmp
	t.Cleanup(func() { sysClassTPM = orig })

	info := ReadTPMInfo()
	if !info.Present {
		t.Fatal("expected TPM present")
	}
	if info.Version != "2.0" {
		t.Errorf("Version = %q, want 2.0", info.Version)
	}
}

func TestDetectBootInfoUSB(t *testing.T) {
	tmp := t.TempDir()

	// Fake /proc/mounts with /boot on /dev/sda1 (vfat).
	mounts := filepath.Join(tmp, "mounts")
	if err := os.WriteFile(mounts, []byte("/dev/sda1 /boot vfat rw 0 0\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Fake /sys/block/sda/removable = 1 (USB flash).
	blk := filepath.Join(tmp, "block", "sda")
	if err := os.MkdirAll(blk, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(blk, "removable"), []byte("1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	origMounts, origBlock := procMountsPath, sysBlockPath
	procMountsPath = mounts
	sysBlockPath = filepath.Join(tmp, "block")
	t.Cleanup(func() { procMountsPath = origMounts; sysBlockPath = origBlock })

	info := DetectBootInfo()
	if info.DeviceType != "usb" {
		t.Errorf("DeviceType = %q, want usb", info.DeviceType)
	}
	if info.Device != "sda1" {
		t.Errorf("Device = %q, want sda1", info.Device)
	}
	if info.FileSystem != "vfat" {
		t.Errorf("FileSystem = %q, want vfat", info.FileSystem)
	}
}

func TestDetectBootInfoZFSPool(t *testing.T) {
	tmp := t.TempDir()
	mounts := filepath.Join(tmp, "mounts")
	if err := os.WriteFile(mounts, []byte("boot/root /boot zfs rw 0 0\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	orig := procMountsPath
	procMountsPath = mounts
	t.Cleanup(func() { procMountsPath = orig })

	info := DetectBootInfo()
	if info.DeviceType != "internal" {
		t.Errorf("DeviceType = %q, want internal", info.DeviceType)
	}
	if info.BootPool != "boot" {
		t.Errorf("BootPool = %q, want boot", info.BootPool)
	}
}
