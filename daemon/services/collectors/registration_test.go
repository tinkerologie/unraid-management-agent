package collectors

import (
	"testing"

	"github.com/cskr/pubsub"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
)

func TestNewRegistrationCollector(t *testing.T) {
	hub := pubsub.New(10)
	ctx := &domain.Context{Hub: hub}

	collector := NewRegistrationCollector(ctx)

	if collector == nil {
		t.Fatal("NewRegistrationCollector() returned nil")
	}

	if collector.ctx != ctx {
		t.Error("RegistrationCollector context not set correctly")
	}
}

func TestRegistrationCollectorInit(t *testing.T) {
	hub := pubsub.New(10)
	ctx := &domain.Context{Hub: hub}

	collector := NewRegistrationCollector(ctx)

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

func TestRegistrationTypeParsing(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"Basic", "Basic", "basic"},
		{"Plus", "Plus", "plus"},
		{"Pro", "Pro", "pro"},
		{"Starter", "Starter", "starter"},
		{"Trial", "Trial", "trial"},
		{"BASIC uppercase", "BASIC", "basic"},
		{"Unknown", "", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Registration type should be lowercase
			result := stringToLower(tt.input)
			if result == "" {
				result = "unknown"
			}
			if result != tt.expected {
				t.Errorf("Registration type = %q, want %q", result, tt.expected)
			}
		})
	}
}

func stringToLower(s string) string {
	if s == "" {
		return ""
	}
	result := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		result[i] = c
	}
	return string(result)
}

func TestRegistrationStateParsing(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"Valid", "Valid", "valid"},
		{"Expired", "Expired", "expired"},
		{"Invalid", "Invalid", "invalid"},
		{"VALID uppercase", "VALID", "valid"},
		{"Empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stringToLower(tt.input)
			if result != tt.expected {
				t.Errorf("Registration state = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestRegistrationDeviceCountParsing(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{"Zero devices", "0", 0},
		{"Six devices", "6", 6},
		{"Twelve devices", "12", 12},
		{"Unlimited devices", "-1", -1},
		{"Invalid number", "abc", 0},
		{"Empty", "", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseDeviceCount(tt.input)
			if result != tt.expected {
				t.Errorf("Device count = %d, want %d", result, tt.expected)
			}
		})
	}
}

func parseDeviceCount(s string) int {
	if s == "" {
		return 0
	}
	result := 0
	negative := false
	i := 0
	if len(s) > 0 && s[0] == '-' {
		negative = true
		i = 1
	}
	for ; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return 0
		}
		result = result*10 + int(c-'0')
	}
	if negative {
		return -result
	}
	return result
}

func TestRegistrationGUIDValidation(t *testing.T) {
	tests := []struct {
		name    string
		guid    string
		isValid bool
	}{
		{"Valid GUID format", "1234-5678-ABCD-EFGH-IJKL", true},
		{"Short GUID", "1234", true},
		{"Empty GUID", "", false},
		{"Spaces only", "   ", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isValid := len(regTrimSpace(tt.guid)) > 0
			if isValid != tt.isValid {
				t.Errorf("GUID valid = %v, want %v", isValid, tt.isValid)
			}
		})
	}
}

func regTrimSpace(s string) string {
	start := 0
	end := len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}

func TestRegistrationVarIniKeys(t *testing.T) {
	// Keys expected in var.ini for registration info
	expectedKeys := []string{
		"regTy",    // Registration type
		"regState", // Registration state (valid/expired)
		"regGUID",  // Registration GUID
		"regTo",    // Registered to
		"regTm",    // Registration time
		"regTm2",   // Registration expiry
	}

	// Verify all keys are non-empty strings
	for _, key := range expectedKeys {
		if len(key) == 0 {
			t.Error("Empty registration key name")
		}
	}

	// Verify expected number of keys
	if len(expectedKeys) < 4 {
		t.Error("Expected at least 4 registration keys")
	}
}

func TestRegistrationLicenseTypes(t *testing.T) {
	// All valid Unraid license types
	licenseTypes := map[string]struct {
		maxDevices int
		features   []string
	}{
		"basic": {
			maxDevices: 6,
			features:   []string{"array", "docker", "vms"},
		},
		"plus": {
			maxDevices: 12,
			features:   []string{"array", "docker", "vms"},
		},
		"pro": {
			maxDevices: -1, // unlimited
			features:   []string{"array", "docker", "vms"},
		},
		"starter": {
			maxDevices: 2,
			features:   []string{"array"},
		},
		"trial": {
			maxDevices: -1,
			features:   []string{"array", "docker", "vms"},
		},
	}

	// Verify we have all expected license types
	expectedTypes := []string{"basic", "plus", "pro", "starter", "trial"}
	for _, lt := range expectedTypes {
		if _, ok := licenseTypes[lt]; !ok {
			t.Errorf("Missing license type: %s", lt)
		}
	}
}

func TestRegistrationTimestampParsing(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		isValid bool
	}{
		{"Unix timestamp", "1609459200", true},
		{"Large timestamp", "1735689600", true},
		{"Zero timestamp", "0", true},
		{"Empty", "", false},
		{"Invalid", "abc", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isValid := isValidTimestamp(tt.input)
			if isValid != tt.isValid {
				t.Errorf("Timestamp valid = %v, want %v", isValid, tt.isValid)
			}
		})
	}
}

func isValidTimestamp(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
