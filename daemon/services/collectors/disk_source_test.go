package collectors

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"

	"github.com/tinkerologie/unraid-management-agent/daemon/dto"
)

func TestParseDiskSourceKeyValue(t *testing.T) {
	tests := []struct {
		line string
		want dto.DiskInfo
	}{
		{`fsType="zfs"`, dto.DiskInfo{FileSystem: "zfs"}},
		{`format="GPT: 4KiB-aligned"`, dto.DiskInfo{}},
		{`spundown="0"`, dto.DiskInfo{SpinState: "active"}},
		{`spundown="1"`, dto.DiskInfo{SpinState: "standby"}},
		{`spundown=""`, dto.DiskInfo{SpinState: "unknown"}},
		{`spundown="2"`, dto.DiskInfo{SpinState: "unknown"}},
		{`type="Flash"`, dto.DiskInfo{Role: "flash"}},
		{`type="fLaSh"`, dto.DiskInfo{Role: "flash"}},
		{`type="Data"`, dto.DiskInfo{}},
		{`fsFree="7"`, dto.DiskInfo{Free: 7168, FreeFromSource: true}},
		{`fsFree="0"`, dto.DiskInfo{FreeFromSource: true}},
		{`fsUsed="3"`, dto.DiskInfo{Used: 3072}},
		{`fsSize="10"`, dto.DiskInfo{FilesystemSize: 10240}},
		{`fsFree="18014398509481983"`, dto.DiskInfo{Free: 18446744073709550592, FreeFromSource: true}},
	}
	for _, key := range []string{"fsFree", "fsUsed", "fsSize"} {
		for _, value := range []string{"", "*", "invalid", "-1", "1.5", "18014398509481984", "18446744073709551616"} {
			tests = append(tests, struct {
				line string
				want dto.DiskInfo
			}{key + "=" + value, dto.DiskInfo{}})
		}
	}
	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			var disk dto.DiskInfo
			(&DiskCollector{}).parseDiskKeyValue(&disk, tt.line)
			if !reflect.DeepEqual(disk, tt.want) {
				t.Errorf("got %+v, want %+v", disk, tt.want)
			}
		})
	}
}

// These seams must not be used in parallel with collector tests that access them.
func mockDiskMounts(t *testing.T, content string, statfs func(string, *syscall.Statfs_t) error) {
	t.Helper()
	oldMounts, oldStatfs := diskMountsPath, diskStatfs
	t.Cleanup(func() { diskMountsPath, diskStatfs = oldMounts, oldStatfs })
	diskMountsPath = filepath.Join(t.TempDir(), "mounts")
	if err := os.WriteFile(diskMountsPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	diskStatfs = statfs
}

func TestDiskSourceFixture(t *testing.T) {
	content, err := os.ReadFile("testdata/fixtures/unraid-7.3.1/disks.ini")
	if err != nil {
		t.Fatal(err)
	}
	collector := &DiskCollector{}
	disks, err := collector.parseDisksINIFrom(strings.NewReader(string(content)))
	if err != nil {
		t.Fatal(err)
	}
	mockDiskMounts(t, `/dev/md1p1 /mnt/disk1 xfs rw 0 0
cache /mnt/cache zfs rw 0 0
/dev/usb_issue296_missing1 /boot vfat rw 0 0
pool1 /mnt/pool1 zfs rw 0 0
/dev/md3p1 /mnt/disk3 btrfs rw 0 0
`, func(path string, _ *syscall.Statfs_t) error {
		t.Errorf("source-described filesystem must not be queried: %s", path)
		return errors.New("unexpected statfs")
	})
	poolUsages := map[string]zfsPoolUsage{
		"cache": {Size: 9999, Used: 1111, Free: 8888, UsagePercent: 11},
		"pool":  {Size: 9999, Used: 9999, Free: 0, UsagePercent: 100},
	}
	wants := []struct {
		name, fs, role, spin, mount string
		size, used, free            uint64
		percent                     float64
		present                     bool
	}{
		{"disk1", "xfs", "data", "standby", "/mnt/disk1", 2000, 400, 600, 40, true},
		{"cache", "zfs", "cache", "active", "/mnt/cache", 2000, 500, 1500, 25, true},
		{"flash", "vfat", "flash", "active", "/boot", 3000, 500, 500, 50, true},
		{"disk2", "auto", "data", "", "", 0, 0, 0, 0, true},
		{"parity2", "", "parity2", "standby", "", 2000, 0, 0, 0, false},
		{"pool1", "zfs", "pool", "unknown", "/mnt/pool1", 1000, 1000, 0, 100, true},
		{"disk3", "btrfs", "data", "unknown", "/mnt/disk3", 2000, 250, 750, 25, true},
	}
	if len(disks) != len(wants) {
		t.Fatalf("got %d disks, want %d", len(disks), len(wants))
	}
	for i, want := range wants {
		t.Run(want.name, func(t *testing.T) {
			disk := &disks[i]
			collector.enrichWithRole(disk)
			collector.enrichWithSpinState(disk)
			collector.enrichWithMountInfo(disk)
			if collector.enrichWithZFSPoolUsage(disk, poolUsages) {
				t.Error("ZFS must not override source capacity")
			}
			if disk.Name != want.name || disk.FileSystem != want.fs || disk.Role != want.role || disk.SpinState != want.spin || disk.MountPoint != want.mount {
				t.Errorf("incorrect source fields: %+v; want %+v", disk, want)
			}
			if disk.Size != want.size*1024 || disk.Used != want.used*1024 || disk.Free != want.free*1024 || disk.UsagePercent != want.percent || disk.FreeFromSource != want.present {
				t.Errorf("incorrect capacity: %+v; want %+v KiB", disk, want)
			}
			data, err := json.Marshal(disk)
			if err != nil {
				t.Fatal(err)
			}
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(data, &fields); err != nil {
				t.Fatal(err)
			}
			if _, ok := fields["FreeFromSource"]; ok {
				t.Error("internal presence metadata leaked into JSON")
			}
			if _, ok := fields["FilesystemSize"]; ok {
				t.Error("internal filesystem size leaked into JSON")
			}
			if _, ok := fields["spin_state"]; ok != (want.spin != "") {
				t.Error("empty slot spin state must be omitted")
			}
		})
	}
}

func TestDiskSourceCapacityFallback(t *testing.T) {
	collector := &DiskCollector{}
	calls := 0
	mockDiskMounts(t, "ignored\n/dev/md1p1 /mnt/disk1 xfs rw 0 0\n", func(path string, stat *syscall.Statfs_t) error {
		calls++
		if path != "/mnt/disk1" {
			t.Errorf("statfs path = %q", path)
		}
		*stat = syscall.Statfs_t{Bsize: 4096, Blocks: 100, Bfree: 40, Bavail: 30}
		return nil
	})
	for _, source := range []string{"", "fsFree=invalid\n", "fsFree=\n"} {
		t.Run(source, func(t *testing.T) {
			disks, err := collector.parseDisksINIFrom(strings.NewReader("[disk1]\nname=disk1\ndevice=sda\n" + source))
			if err != nil || len(disks) != 1 {
				t.Fatalf("parse: %v, %v", disks, err)
			}
			disk := &disks[0]
			collector.enrichWithMountInfo(disk)
			if disk.Free != 40*4096 || disk.Used != 60*4096 || disk.UsagePercent != 60 {
				t.Errorf("fallback changed: %+v", disk)
			}
		})
	}
	if calls != 3 {
		t.Errorf("statfs calls = %d, want 3", calls)
	}
	// Without source capacity, ZFS still supersedes statfs, including pool Size.
	disk := &dto.DiskInfo{Name: "cache", Role: "cache", Free: 123}
	usage := zfsPoolUsage{Size: 400, Used: 100, Free: 300, UsagePercent: 25}
	if !collector.enrichWithZFSPoolUsage(disk, map[string]zfsPoolUsage{"cache": usage}) || disk.Size != 400 || disk.Free != 300 || disk.Used != 100 || disk.UsagePercent != 25 {
		t.Errorf("ZFS fallback changed: %+v", disk)
	}
}

func TestDiskSourcePoolCapacityTotals(t *testing.T) {
	const memberKiB = uint64(1 << 30) // One TiB physical member.
	tests := []struct {
		name, filesystem                string
		filesystemKiB, usedKiB, freeKiB uint64
		wantSizeKiB                     uint64
	}{
		{"striped ZFS pool", "zfs", 2 * memberKiB, memberKiB / 2, 3 * memberKiB / 2, 2 * memberKiB},
		{"case insensitive ZFS", "ZFS", 2 * memberKiB, memberKiB / 2, 3 * memberKiB / 2, 2 * memberKiB},
		{"smaller ZFS filesystem", "zfs", memberKiB / 2, memberKiB / 4, memberKiB / 4, memberKiB / 2},
		{"full ZFS pool", "zfs", 2 * memberKiB, 2 * memberKiB, 0, 2 * memberKiB},
		{"ordinary XFS disk", "xfs", memberKiB / 2, memberKiB / 4, memberKiB / 4, memberKiB},
		{"ordinary btrfs pool", "btrfs", memberKiB / 2, memberKiB / 4, memberKiB / 4, memberKiB},
		{"striped btrfs free exceeds member", "btrfs", 2 * memberKiB, memberKiB / 2, 3 * memberKiB / 2, 2 * memberKiB},
		{"btrfs combined usage exceeds member", "btrfs", 2 * memberKiB, memberKiB, memberKiB, 2 * memberKiB},
	}
	collector := &DiskCollector{}
	mockDiskMounts(t, "cache /mnt/cache zfs rw 0 0\n", func(path string, _ *syscall.Statfs_t) error {
		t.Errorf("source capacity must not trigger statfs: %s", path)
		return errors.New("unexpected statfs")
	})
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Put filesystem type and physical size last to exercise section finalization.
			source := fmt.Sprintf("[cache]\nname=cache\nfsSize=%d\nfsUsed=%d\nfsFree=%d\nsize=%d\nfsType=%s\n",
				tt.filesystemKiB, tt.usedKiB, tt.freeKiB, memberKiB, tt.filesystem)
			disks, err := collector.parseDisksINIFrom(strings.NewReader(source))
			if err != nil || len(disks) != 1 {
				t.Fatalf("parse: %v, %v", disks, err)
			}
			disk := &disks[0]
			collector.enrichWithRole(disk)
			collector.enrichWithMountInfo(disk)
			if collector.enrichWithZFSPoolUsage(disk, map[string]zfsPoolUsage{
				"cache": {Size: 9999, Used: 1111, Free: 8888, UsagePercent: 11},
			}) {
				t.Error("ZFS fallback must not override source capacity")
			}
			if disk.Size != tt.wantSizeKiB*1024 || disk.Used != tt.usedKiB*1024 || disk.Free != tt.freeKiB*1024 {
				t.Errorf("capacity = size %d, used %d, free %d; want %d, %d, %d bytes",
					disk.Size, disk.Used, disk.Free, tt.wantSizeKiB*1024, tt.usedKiB*1024, tt.freeKiB*1024)
			}
			if disk.Used+disk.Free > disk.Size {
				t.Errorf("filesystem usage exceeds reported size: %+v", disk)
			}
			wantPercent := float64(tt.usedKiB) / float64(tt.filesystemKiB) * 100
			if disk.UsagePercent != wantPercent {
				t.Errorf("usage percent = %v, want %v", disk.UsagePercent, wantPercent)
			}
		})
	}
}

func TestDiskSourceSpinDefaults(t *testing.T) {
	for _, device := range []string{"", "sd_issue296_nonexistent"} {
		for _, source := range []string{"", "spundown=invalid\n", "spundown=0\n", "spundown=1\n"} {
			for _, temp := range []string{"*", "0", "35"} {
				t.Run(device+source+temp, func(t *testing.T) {
					collector := &DiskCollector{}
					disks, err := collector.parseDisksINIFrom(strings.NewReader("[disk]\ndevice=" + device + "\ntemp=" + temp + "\n" + source))
					if err != nil || len(disks) != 1 {
						t.Fatalf("parse: %v, %v", disks, err)
					}
					want := "unknown"
					switch source {
					case "spundown=0\n":
						want = "active"
					case "spundown=1\n":
						want = "standby"
					}
					if device == "" {
						want = ""
					}
					collector.enrichWithSpinState(&disks[0])
					if disks[0].SpinState != want {
						t.Errorf("spin state = %q, want %q", disks[0].SpinState, want)
					}
				})
			}
		}
	}
	// Also exercise the missing default on DTOs not constructed by the parser.
	disk := &dto.DiskInfo{Device: "sd_issue296_nonexistent", Temperature: 35}
	(&DiskCollector{}).enrichWithSpinState(disk)
	if disk.SpinState != "unknown" {
		t.Errorf("spin state = %q, want unknown", disk.SpinState)
	}
	// A source flash role wins over name/ID substring heuristics.
	disk = &dto.DiskInfo{Name: "disk1", ID: "SanDisk_REDACTED", Role: "flash"}
	(&DiskCollector{}).enrichWithRole(disk)
	if disk.Role != "flash" {
		t.Errorf("role = %q, want flash", disk.Role)
	}
}
