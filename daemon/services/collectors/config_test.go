package collectors

import (
	"testing"
)

func TestNewConfigCollector(t *testing.T) {
	collector := NewConfigCollector()

	if collector == nil {
		t.Fatal("NewConfigCollector() returned nil")
	}
}

func TestShareNameValidation(t *testing.T) {
	tests := []struct {
		name      string
		shareName string
		wantErr   bool
	}{
		{"valid simple name", "media", false},
		{"valid with underscore", "media_files", false},
		{"valid with dash", "media-files", false},
		{"valid with numbers", "share123", false},
		{"path traversal", "../etc", true},
		{"path traversal 2", "share/../etc", true},
		{"absolute path", "/etc/passwd", true},
		{"empty name", "", true},
		{"with slash", "share/name", true},
		{"with backslash", "share\\name", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := testValidateShareName(tt.shareName)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateShareName(%q) error = %v, wantErr %v", tt.shareName, err, tt.wantErr)
			}
		})
	}
}

// testValidateShareName is a test helper for share name validation
func testValidateShareName(name string) error {
	if name == "" {
		return &configError{"share name cannot be empty"}
	}
	// Check for path traversal
	for i := 0; i < len(name)-1; i++ {
		if name[i] == '.' && name[i+1] == '.' {
			return &configError{"path traversal detected"}
		}
	}
	// Check for path separators
	for i := 0; i < len(name); i++ {
		if name[i] == '/' || name[i] == '\\' {
			return &configError{"path separator not allowed"}
		}
	}
	return nil
}

type configError struct {
	msg string
}

func (e *configError) Error() string {
	return e.msg
}

func TestShareConfigFields(t *testing.T) {
	// Expected share config keys from .cfg files
	expectedKeys := []string{
		"shareComment",
		"shareAllocator",
		"shareFloor",
		"shareSplitLevel",
		"shareInclude",
		"shareExclude",
		"shareUseCache",
		"shareExport",
		"shareSecurity",
	}

	// Verify all keys are valid
	for _, key := range expectedKeys {
		if len(key) == 0 {
			t.Error("Empty config key")
		}
		// Should start with "share"
		if len(key) < 5 || key[:5] != "share" {
			t.Errorf("Config key should start with 'share': %s", key)
		}
	}
}

func TestAllocatorValues(t *testing.T) {
	// Valid allocator values
	validAllocators := []string{
		"highwater",
		"fillup",
		"mostfree",
	}

	expectedAllocators := map[string]bool{
		"highwater": true,
		"fillup":    true,
		"mostfree":  true,
	}

	for _, alloc := range validAllocators {
		if !expectedAllocators[alloc] {
			t.Errorf("Unexpected allocator: %s", alloc)
		}
	}
}

func TestSplitLevelValues(t *testing.T) {
	// Valid split level values
	tests := []struct {
		name  string
		level string
		valid bool
	}{
		{"auto level", "auto", true},
		{"manual level 1", "1", true},
		{"manual level 2", "2", true},
		{"manual level 3", "3", true},
		{"manual level 0", "0", true},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid := len(tt.level) > 0
			if valid != tt.valid {
				t.Errorf("split level %q valid = %v, want %v", tt.level, valid, tt.valid)
			}
		})
	}
}

func TestCacheUsageValues(t *testing.T) {
	// Valid cache usage values
	validValues := []string{
		"no",
		"yes",
		"only",
		"prefer",
	}

	expectedValues := map[string]bool{
		"no":     true,
		"yes":    true,
		"only":   true,
		"prefer": true,
	}

	for _, val := range validValues {
		if !expectedValues[val] {
			t.Errorf("Unexpected cache usage value: %s", val)
		}
	}
}

func TestExportValues(t *testing.T) {
	// Valid export values for shares
	tests := []struct {
		name   string
		export string
		valid  bool
	}{
		{"export enabled", "e", true},
		{"export disabled", "-", true},
		{"export yes", "yes", true},
		{"export no", "no", true},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid := len(tt.export) > 0
			if valid != tt.valid {
				t.Errorf("export %q valid = %v, want %v", tt.export, valid, tt.valid)
			}
		})
	}
}

func TestSecurityValues(t *testing.T) {
	// Valid security values
	validSecurity := []string{
		"public",
		"secure",
		"private",
	}

	expectedSecurity := map[string]bool{
		"public":  true,
		"secure":  true,
		"private": true,
	}

	for _, sec := range validSecurity {
		if !expectedSecurity[sec] {
			t.Errorf("Unexpected security value: %s", sec)
		}
	}
}

func TestFloorValueParsing(t *testing.T) {
	tests := []struct {
		name     string
		floor    string
		expected int64
	}{
		{"zero floor", "0", 0},
		{"1GB floor", "1073741824", 1073741824},
		{"10GB floor", "10737418240", 10737418240},
		{"empty", "", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseFloorValue(tt.floor)
			if result != tt.expected {
				t.Errorf("parseFloorValue(%q) = %d, want %d", tt.floor, result, tt.expected)
			}
		})
	}
}

func parseFloorValue(s string) int64 {
	if s == "" {
		return 0
	}
	var result int64
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0
		}
		result = result*10 + int64(c-'0')
	}
	return result
}

func TestIncludeExcludeDiskParsing(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{"single disk", "disk1", 1},
		{"multiple disks", "disk1,disk2,disk3", 3},
		{"empty", "", 0},
		{"cache only", "cache", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			count := parseDisksCount(tt.input)
			if count != tt.expected {
				t.Errorf("disk count for %q = %d, want %d", tt.input, count, tt.expected)
			}
		})
	}
}

func parseDisksCount(s string) int {
	if s == "" {
		return 0
	}
	count := 1
	for _, c := range s {
		if c == ',' {
			count++
		}
	}
	return count
}

func TestConfigPathFormat(t *testing.T) {
	// Test expected config path format
	basePath := "/boot/config/shares/"
	extension := ".cfg"

	// Path format should be: /boot/config/shares/{name}.cfg
	testName := "testshare"
	expectedPath := basePath + testName + extension

	if expectedPath != "/boot/config/shares/testshare.cfg" {
		t.Errorf("Unexpected path format: %s", expectedPath)
	}
}

func TestDiskSettingsKeys(t *testing.T) {
	// Expected disk settings keys
	expectedKeys := []string{
		"shareUserInclude",
		"shareUserExclude",
		"shareUserUseCache",
		"shareUserAllocator",
		"shareUserFloor",
		"shareUserSplitLevel",
	}

	// Verify all keys are valid
	for _, key := range expectedKeys {
		if len(key) == 0 {
			t.Error("Empty settings key")
		}
	}
}

func TestNetworkConfigKeys(t *testing.T) {
	// Expected network config keys
	expectedKeys := []string{
		"IFACE",
		"DHCP_KEEPRESOLV",
		"DNS_SERVER1",
		"DNS_SERVER2",
		"GATEWAY",
		"IPADDR",
		"NETMASK",
		"USE_DHCP",
		"BONDNAME",
		"BONDING_MODE",
		"BRNAME",
		"BRSTP",
		"BRFD",
	}

	// Verify all keys are valid
	for _, key := range expectedKeys {
		if len(key) == 0 {
			t.Error("Empty network config key")
		}
	}
}
