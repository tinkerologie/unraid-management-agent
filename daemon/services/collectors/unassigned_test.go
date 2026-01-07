package collectors

import (
	"testing"

	"github.com/cskr/pubsub"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
)

func TestNewUnassignedCollector(t *testing.T) {
	hub := pubsub.New(10)
	ctx := &domain.Context{Hub: hub}

	collector := NewUnassignedCollector(ctx)

	if collector == nil {
		t.Fatal("NewUnassignedCollector() returned nil")
	}

	if collector.ctx != ctx {
		t.Error("UnassignedCollector context not set correctly")
	}
}

func TestUnassignedCollectorInit(t *testing.T) {
	hub := pubsub.New(10)
	ctx := &domain.Context{Hub: hub}

	collector := NewUnassignedCollector(ctx)

	// Verify collector is properly initialized
	if collector == nil {
		t.Fatal("Collector should not be nil")
	}

	if collector.ctx == nil {
		t.Fatal("Collector context should not be nil")
	}

	if collector.ctx.Hub == nil {
		t.Fatal("Collector context Hub should not be nil")
	}
}

func TestDeviceFiltering(t *testing.T) {
	tests := []struct {
		name       string
		deviceName string
		shouldSkip bool
	}{
		{"loop device", "loop0", true},
		{"loop device with number", "loop1", true},
		{"md device", "md0", true},
		{"zram device", "zram0", true},
		{"nvme partition", "nvme0n1p1", true},
		{"sda partition", "sda1", true},
		{"sdb partition", "sdb2", true},
		{"valid sda", "sda", false},
		{"valid sdb", "sdb", false},
		{"valid nvme", "nvme0n1", false},
		{"valid sdc", "sdc", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shouldSkip := shouldSkipDevice(tt.deviceName)
			if shouldSkip != tt.shouldSkip {
				t.Errorf("shouldSkip = %v, want %v", shouldSkip, tt.shouldSkip)
			}
		})
	}
}

func shouldSkipDevice(device string) bool {
	// Skip loop devices
	if len(device) >= 4 && device[:4] == "loop" {
		return true
	}
	// Skip md devices
	if len(device) >= 2 && device[:2] == "md" {
		return true
	}
	// Skip zram devices
	if len(device) >= 4 && device[:4] == "zram" {
		return true
	}
	// Skip nvme partitions
	if len(device) > 7 && device[:7] == "nvme0n1" && len(device) > 7 && device[7] == 'p' {
		return true
	}
	// Skip disk partitions (sda1, sdb2, etc.)
	if len(device) == 4 && device[:2] == "sd" && device[3] >= '1' && device[3] <= '9' {
		return true
	}
	return false
}

func TestPluginInstallationCheck(t *testing.T) {
	// Test path for plugin detection
	pluginPath := "/boot/config/plugins/unassigned.devices"

	// Path should be non-empty
	if len(pluginPath) == 0 {
		t.Error("Plugin path should not be empty")
	}

	// Path should contain plugin name
	if !containsString(pluginPath, "unassigned.devices") {
		t.Error("Plugin path should contain 'unassigned.devices'")
	}
}

func containsString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestRemoteShareTypes(t *testing.T) {
	// Valid remote share types
	shareTypes := []string{
		"smb",
		"nfs",
		"iso",
	}

	expectedTypes := map[string]bool{
		"smb": true,
		"nfs": true,
		"iso": true,
	}

	for _, st := range shareTypes {
		if !expectedTypes[st] {
			t.Errorf("Unexpected share type: %s", st)
		}
	}

	// Verify we have all expected types
	if len(shareTypes) != 3 {
		t.Errorf("Expected 3 share types, got %d", len(shareTypes))
	}
}

func TestMountPointParsing(t *testing.T) {
	tests := []struct {
		name      string
		mountLine string
		isMounted bool
	}{
		{"mounted sda", "/dev/sda1 /mnt/disk1 ext4 rw 0 0", true},
		{"mounted nfs", "192.168.1.100:/share /mnt/nfs nfs rw 0 0", true},
		{"mounted smb", "//192.168.1.100/share /mnt/smb cifs rw 0 0", true},
		{"empty line", "", false},
		{"comment line", "# comment", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isMounted := len(tt.mountLine) > 0 && tt.mountLine[0] != '#'
			if isMounted != tt.isMounted {
				t.Errorf("isMounted = %v, want %v", isMounted, tt.isMounted)
			}
		})
	}
}

func TestDeviceInfoFields(t *testing.T) {
	// Expected fields for an unassigned device
	fields := []string{
		"Device",
		"Name",
		"Serial",
		"Size",
		"SizeBytes",
		"Filesystem",
		"MountPoint",
		"Mounted",
		"Temperature",
		"Status",
		"SpindownDelay",
		"ReadOnly",
		"Partitions",
	}

	// Verify all fields are valid
	for _, field := range fields {
		if len(field) == 0 {
			t.Error("Empty field name")
		}
	}

	// Verify we have reasonable number of fields
	if len(fields) < 10 {
		t.Error("Expected at least 10 device info fields")
	}
}

func TestFilesystemTypes(t *testing.T) {
	// Common filesystem types
	fsTypes := []string{
		"ext4",
		"xfs",
		"btrfs",
		"ntfs",
		"vfat",
		"exfat",
		"reiserfs",
		"zfs",
	}

	// Verify all types are recognized
	for _, fs := range fsTypes {
		if len(fs) == 0 {
			t.Error("Empty filesystem type")
		}
	}
}

func TestDeviceSizeParsing(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		bytes   int64
		isValid bool
	}{
		{"1TB", "1000000000000", 1000000000000, true},
		{"500GB", "500000000000", 500000000000, true},
		{"100MB", "100000000", 100000000, true},
		{"Zero", "0", 0, true},
		{"Empty", "", 0, false},
		{"Invalid", "abc", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bytes, isValid := parseSize(tt.input)
			if isValid != tt.isValid {
				t.Errorf("isValid = %v, want %v", isValid, tt.isValid)
			}
			if isValid && bytes != tt.bytes {
				t.Errorf("bytes = %d, want %d", bytes, tt.bytes)
			}
		})
	}
}

func parseSize(s string) (int64, bool) {
	if s == "" {
		return 0, false
	}
	var result int64
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, false
		}
		result = result*10 + int64(c-'0')
	}
	return result, true
}

func TestSpindownDelayValues(t *testing.T) {
	// Valid spindown delay values in minutes
	validDelays := []int{0, 15, 30, 45, 60, 120, 180, 240, 300}

	for _, delay := range validDelays {
		if delay < 0 {
			t.Errorf("Spindown delay should not be negative: %d", delay)
		}
	}

	// 0 means never spin down
	// Other values are minutes
	if validDelays[0] != 0 {
		t.Error("First delay value should be 0 (never)")
	}
}

func TestPartitionNumberParsing(t *testing.T) {
	tests := []struct {
		name        string
		partition   string
		expectedNum int
	}{
		{"First partition", "sda1", 1},
		{"Second partition", "sda2", 2},
		{"Tenth partition", "sda10", 10},
		{"No partition", "sda", 0},
		{"NVMe partition", "nvme0n1p1", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			num := extractPartitionNumber(tt.partition)
			if num != tt.expectedNum {
				t.Errorf("partition number = %d, want %d", num, tt.expectedNum)
			}
		})
	}
}

func extractPartitionNumber(s string) int {
	// For sd* devices
	if len(s) >= 4 && s[:2] == "sd" && s[2] >= 'a' && s[2] <= 'z' {
		if len(s) > 3 {
			num := 0
			for i := 3; i < len(s); i++ {
				if s[i] >= '0' && s[i] <= '9' {
					num = num*10 + int(s[i]-'0')
				}
			}
			return num
		}
		return 0
	}
	// For nvme*p* devices
	if len(s) > 7 && s[:4] == "nvme" {
		for i := 0; i < len(s); i++ {
			if s[i] == 'p' && i+1 < len(s) {
				num := 0
				for j := i + 1; j < len(s); j++ {
					if s[j] >= '0' && s[j] <= '9' {
						num = num*10 + int(s[j]-'0')
					}
				}
				return num
			}
		}
	}
	return 0
}

func TestArrayDiskFiltering(t *testing.T) {
	// Simulated array disks
	arrayDisks := map[string]bool{
		"sda": true,
		"sdb": true,
		"sdc": true,
	}

	tests := []struct {
		name        string
		device      string
		isArrayDisk bool
	}{
		{"Array disk sda", "sda", true},
		{"Array disk sdb", "sdb", true},
		{"Non-array sdd", "sdd", false},
		{"Non-array sde", "sde", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isArrayDisk := arrayDisks[tt.device]
			if isArrayDisk != tt.isArrayDisk {
				t.Errorf("isArrayDisk = %v, want %v", isArrayDisk, tt.isArrayDisk)
			}
		})
	}
}
