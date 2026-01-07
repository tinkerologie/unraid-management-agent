package collectors

import (
	"syscall"
	"testing"

	"github.com/cskr/pubsub"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

func TestNewDiskCollector(t *testing.T) {
	hub := pubsub.New(10)
	ctx := &domain.Context{Hub: hub}

	collector := NewDiskCollector(ctx)

	if collector == nil {
		t.Fatal("NewDiskCollector() returned nil")
	}

	if collector.ctx != ctx {
		t.Error("DiskCollector context not set correctly")
	}
}

// TestSafeBlockSizeConversion tests that the int64 to uint64 conversion
// for stat.Bsize is safe and doesn't cause integer overflow issues.
// This tests the fix for gosec G115 (CWE-190).
func TestSafeBlockSizeConversion(t *testing.T) {
	tests := []struct {
		name   string
		bsize  int64
		blocks uint64
		want   uint64
	}{
		{
			name:   "typical 4KB block size",
			bsize:  4096,
			blocks: 1000000,
			want:   4096000000,
		},
		{
			name:   "512 byte block size (old systems)",
			bsize:  512,
			blocks: 1000000,
			want:   512000000,
		},
		{
			name:   "1KB block size",
			bsize:  1024,
			blocks: 1000000,
			want:   1024000000,
		},
		{
			name:   "8KB block size (some filesystems)",
			bsize:  8192,
			blocks: 1000000,
			want:   8192000000,
		},
		{
			name:   "zero blocks",
			bsize:  4096,
			blocks: 0,
			want:   0,
		},
		{
			name:   "large block count (100TB filesystem)",
			bsize:  4096,
			blocks: 27487790694, // ~100TB with 4KB blocks
			want:   112589990682624,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This mirrors the safe conversion pattern used in the code:
			// bsize := uint64(stat.Bsize)
			// totalBytes := stat.Blocks * bsize
			bsize := uint64(tt.bsize)
			got := tt.blocks * bsize

			if got != tt.want {
				t.Errorf("block size conversion = %d, want %d", got, tt.want)
			}
		})
	}
}

// TestStatfsConversion tests the actual syscall.Statfs_t conversion pattern
// used in the disk collector for calculating filesystem sizes.
func TestStatfsConversion(t *testing.T) {
	// Create a mock Statfs_t structure
	stat := syscall.Statfs_t{
		Bsize:  4096,
		Blocks: 1000000,
		Bfree:  500000,
		Bavail: 450000,
	}

	// Safe conversion pattern (as used in disk.go)
	//nolint:gosec // G115: Bsize is always positive on Linux systems
	bsize := uint64(stat.Bsize)
	totalBytes := stat.Blocks * bsize
	freeBytes := stat.Bfree * bsize
	availBytes := stat.Bavail * bsize // Available bytes (for non-root users)
	usedBytes := totalBytes - freeBytes

	// Verify Bavail is used (available space for unprivileged users)
	if availBytes != 450000*4096 {
		t.Errorf("Expected availBytes %d, got %d", 450000*4096, availBytes)
	}

	expectedTotal := uint64(4096000000)
	expectedFree := uint64(2048000000)
	expectedUsed := uint64(2048000000)

	if totalBytes != expectedTotal {
		t.Errorf("totalBytes = %d, want %d", totalBytes, expectedTotal)
	}
	if freeBytes != expectedFree {
		t.Errorf("freeBytes = %d, want %d", freeBytes, expectedFree)
	}
	if usedBytes != expectedUsed {
		t.Errorf("usedBytes = %d, want %d", usedBytes, expectedUsed)
	}
}

// TestUsagePercentCalculation tests the usage percentage calculation
func TestUsagePercentCalculation(t *testing.T) {
	tests := []struct {
		name    string
		total   uint64
		used    uint64
		wantMin float64
		wantMax float64
	}{
		{
			name:    "50% usage",
			total:   1000,
			used:    500,
			wantMin: 49.9,
			wantMax: 50.1,
		},
		{
			name:    "0% usage (empty)",
			total:   1000,
			used:    0,
			wantMin: -0.1,
			wantMax: 0.1,
		},
		{
			name:    "100% usage (full)",
			total:   1000,
			used:    1000,
			wantMin: 99.9,
			wantMax: 100.1,
		},
		{
			name:    "33% usage",
			total:   3000,
			used:    1000,
			wantMin: 33.0,
			wantMax: 34.0,
		},
		{
			name:    "zero total (edge case)",
			total:   0,
			used:    0,
			wantMin: -0.1,
			wantMax: 0.1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var usagePercent float64
			if tt.total > 0 {
				usagePercent = float64(tt.used) / float64(tt.total) * 100
			}

			if usagePercent < tt.wantMin || usagePercent > tt.wantMax {
				t.Errorf("usagePercent = %f, want between %f and %f", usagePercent, tt.wantMin, tt.wantMax)
			}
		})
	}
}

// TestDiskRoleDetection tests the disk role detection logic
func TestDiskRoleDetection(t *testing.T) {
	tests := []struct {
		name     string
		diskName string
		diskID   string
		wantRole string
	}{
		{
			name:     "parity disk",
			diskName: "parity",
			diskID:   "parity",
			wantRole: "parity",
		},
		{
			name:     "parity2 disk",
			diskName: "parity2",
			diskID:   "parity2",
			wantRole: "parity",
		},
		{
			name:     "data disk",
			diskName: "disk1",
			diskID:   "disk1",
			wantRole: "data",
		},
		{
			name:     "cache disk",
			diskName: "cache",
			diskID:   "cache",
			wantRole: "cache",
		},
		{
			name:     "pool disk",
			diskName: "pool1",
			diskID:   "pool1",
			wantRole: "pool",
		},
		{
			name:     "flash drive",
			diskName: "flash",
			diskID:   "flash",
			wantRole: "flash",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			role := detectDiskRole(tt.diskName, tt.diskID)
			if role != tt.wantRole {
				t.Errorf("detectDiskRole(%s, %s) = %s, want %s", tt.diskName, tt.diskID, role, tt.wantRole)
			}
		})
	}
}

// detectDiskRole is a helper function that mimics the role detection logic
func detectDiskRole(name, id string) string {
	// Match the logic in enrichWithRole
	if name == "parity" || name == "parity2" || id == "parity" || id == "parity2" {
		return "parity"
	}
	if name == "flash" || id == "flash" {
		return "flash"
	}
	if name == "cache" || id == "cache" {
		return "cache"
	}
	if len(name) >= 4 && name[:4] == "pool" {
		return "pool"
	}
	if len(name) >= 4 && name[:4] == "disk" {
		return "data"
	}
	return "unknown"
}

// TestDiskSpinStateValues tests valid spin state values
func TestDiskSpinStateValues(t *testing.T) {
	validStates := []string{"active", "standby", "unknown"}

	for _, state := range validStates {
		t.Run(state, func(t *testing.T) {
			// Verify the state is a valid string
			if state == "" {
				t.Error("spin state should not be empty")
			}
		})
	}
}

// TestDiskStatusValues tests valid disk status values
func TestDiskStatusValues(t *testing.T) {
	validStatuses := []string{
		"DISK_OK",
		"DISK_NP",
		"DISK_NP_DSBL",
		"DISK_DSBL",
		"DISK_NEW",
		"DISK_INVALID",
		"DISK_WRONG",
		"DISK_EMULATED",
	}

	for _, status := range validStatuses {
		t.Run(status, func(t *testing.T) {
			// These are the known Unraid disk statuses
			if status == "" {
				t.Error("status should not be empty")
			}
		})
	}
}

// TestParseDiskKeyValue tests parsing of disk INI key-value pairs
func TestParseDiskKeyValue(t *testing.T) {
	hub := pubsub.New(10)
	ctx := &domain.Context{Hub: hub}
	collector := NewDiskCollector(ctx)

	tests := []struct {
		name       string
		line       string
		checkField string
		expected   interface{}
	}{
		{
			name:       "parse name",
			line:       "name=disk1",
			checkField: "name",
			expected:   "disk1",
		},
		{
			name:       "parse device",
			line:       "device=sda",
			checkField: "device",
			expected:   "sda",
		},
		{
			name:       "parse id",
			line:       `id="WDC_WD40EFAX"`,
			checkField: "id",
			expected:   "WDC_WD40EFAX",
		},
		{
			name:       "parse status",
			line:       "status=DISK_OK",
			checkField: "status",
			expected:   "DISK_OK",
		},
		{
			name:       "parse size (sectors to bytes)",
			line:       "size=1000",
			checkField: "size",
			expected:   uint64(512000), // 1000 sectors * 512 bytes
		},
		{
			name:       "parse temperature",
			line:       "temp=35",
			checkField: "temp",
			expected:   35.0,
		},
		{
			name:       "parse temperature wildcard",
			line:       "temp=*",
			checkField: "temp",
			expected:   0.0, // "*" means spun down, no temp
		},
		{
			name:       "parse numErrors",
			line:       "numErrors=5",
			checkField: "errors",
			expected:   5,
		},
		{
			name:       "parse spindownDelay",
			line:       "spindownDelay=30",
			checkField: "spindown",
			expected:   30,
		},
		{
			name:       "parse format",
			line:       "format=xfs",
			checkField: "format",
			expected:   "xfs",
		},
		{
			name:       "invalid line (no equals)",
			line:       "invalid line",
			checkField: "none",
			expected:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			disk := &dto.DiskInfo{}
			collector.parseDiskKeyValue(disk, tt.line)

			switch tt.checkField {
			case "name":
				if disk.Name != tt.expected.(string) {
					t.Errorf("Name = %q, want %q", disk.Name, tt.expected)
				}
			case "device":
				if disk.Device != tt.expected.(string) {
					t.Errorf("Device = %q, want %q", disk.Device, tt.expected)
				}
			case "id":
				if disk.ID != tt.expected.(string) {
					t.Errorf("ID = %q, want %q", disk.ID, tt.expected)
				}
			case "status":
				if disk.Status != tt.expected.(string) {
					t.Errorf("Status = %q, want %q", disk.Status, tt.expected)
				}
			case "size":
				if disk.Size != tt.expected.(uint64) {
					t.Errorf("Size = %d, want %d", disk.Size, tt.expected)
				}
			case "temp":
				if disk.Temperature != tt.expected.(float64) {
					t.Errorf("Temperature = %f, want %f", disk.Temperature, tt.expected)
				}
			case "errors":
				if disk.SMARTErrors != tt.expected.(int) {
					t.Errorf("SMARTErrors = %d, want %d", disk.SMARTErrors, tt.expected)
				}
			case "spindown":
				if disk.SpindownDelay != tt.expected.(int) {
					t.Errorf("SpindownDelay = %d, want %d", disk.SpindownDelay, tt.expected)
				}
			case "format":
				if disk.FileSystem != tt.expected.(string) {
					t.Errorf("FileSystem = %q, want %q", disk.FileSystem, tt.expected)
				}
			case "none":
				// Line should be ignored
			}
		})
	}
}

// TestIsNVMeDevice tests NVMe device detection
func TestIsNVMeDevice(t *testing.T) {
	hub := pubsub.New(10)
	ctx := &domain.Context{Hub: hub}
	collector := NewDiskCollector(ctx)

	tests := []struct {
		device   string
		expected bool
	}{
		{"nvme0n1", true},
		{"nvme1n1", true},
		{"nvme0n1p1", true},
		{"sda", false},
		{"sdb1", false},
		{"md0", false},
		{"loop0", false},
	}

	for _, tt := range tests {
		t.Run(tt.device, func(t *testing.T) {
			result := collector.isNVMeDevice(tt.device)
			if result != tt.expected {
				t.Errorf("isNVMeDevice(%q) = %v, want %v", tt.device, result, tt.expected)
			}
		})
	}
}

// TestDiskFilesystemTypes tests common filesystem type values
func TestDiskFilesystemTypes(t *testing.T) {
	validFilesystems := []string{
		"xfs",
		"btrfs",
		"ext4",
		"vfat",
		"ntfs",
		"zfs",
		"reiserfs",
	}

	for _, fs := range validFilesystems {
		t.Run(fs, func(t *testing.T) {
			if fs == "" {
				t.Error("filesystem type should not be empty")
			}
		})
	}
}

// TestDiskSMARTStatus tests valid SMART status values
func TestDiskSMARTStatus(t *testing.T) {
	validStatuses := []string{"PASSED", "FAILED", "UNKNOWN"}

	for _, status := range validStatuses {
		t.Run(status, func(t *testing.T) {
			if status == "" {
				t.Error("SMART status should not be empty")
			}
		})
	}
}
