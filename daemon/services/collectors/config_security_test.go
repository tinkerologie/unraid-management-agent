package collectors

import (
	"strings"
	"testing"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

// TestValidateShareName tests the share name validation function
func TestValidateShareName(t *testing.T) {
	tests := []struct {
		name      string
		shareName string
		wantError bool
		errorMsg  string
	}{
		{
			name:      "Valid share name",
			shareName: "appdata",
			wantError: false,
		},
		{
			name:      "Valid share name with hyphen",
			shareName: "my-share",
			wantError: false,
		},
		{
			name:      "Valid share name with underscore",
			shareName: "my_share",
			wantError: false,
		},
		{
			name:      "Empty share name",
			shareName: "",
			wantError: true,
			errorMsg:  "cannot be empty",
		},
		{
			name:      "Path traversal with ../",
			shareName: "../../../etc/passwd",
			wantError: true,
			errorMsg:  "parent directory references not allowed",
		},
		{
			name:      "Unix path separator",
			shareName: "subdir/share",
			wantError: true,
			errorMsg:  "path separators not allowed",
		},
		{
			name:      "Windows path separator",
			shareName: "subdir\\share",
			wantError: true,
			errorMsg:  "path separators not allowed",
		},
		{
			name:      "Absolute Unix path",
			shareName: "/etc/passwd",
			wantError: true,
			errorMsg:  "absolute paths not allowed",
		},
		{
			name:      "Absolute Windows path",
			shareName: "\\etc\\passwd",
			wantError: true,
			errorMsg:  "absolute paths not allowed",
		},
		{
			name:      "Share name too long",
			shareName: strings.Repeat("a", 256),
			wantError: true,
			errorMsg:  "too long",
		},
		{
			name:      "Complex path traversal",
			shareName: "....//....//etc/passwd",
			wantError: true,
			errorMsg:  "parent directory references not allowed", // ".." is checked first
		},
		{
			name:      "Null byte injection",
			shareName: "share\x00.cfg",
			wantError: false, // Null bytes are handled by the OS, not our validation
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateShareName(tt.shareName)

			if tt.wantError {
				if err == nil {
					t.Errorf("validateShareName() expected error but got nil")
					return
				}
				if !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("validateShareName() error = %v, want error containing %q", err, tt.errorMsg)
				}
			} else if err != nil {
				t.Errorf("validateShareName() unexpected error = %v", err)
			}
		})
	}
}

// TestGetShareConfigSecurity tests that GetShareConfig rejects malicious share names
func TestGetShareConfigSecurity(t *testing.T) {
	collector := NewConfigCollector()

	maliciousNames := []string{
		"../../../etc/passwd",
		"../../etc/shadow",
		"/etc/passwd",
		"subdir/share",
		"..\\..\\..\\windows\\system32\\config\\sam",
	}

	for _, name := range maliciousNames {
		t.Run("Reject_"+name, func(t *testing.T) {
			_, err := collector.GetShareConfig(name)
			if err == nil {
				t.Errorf("GetShareConfig() should reject malicious name %q but returned nil", name)
			}
			if !strings.Contains(err.Error(), "invalid share name") {
				t.Errorf("GetShareConfig() error should mention 'invalid share name', got: %v", err)
			}
		})
	}
}

// TestUpdateShareConfigSecurity tests that UpdateShareConfig rejects malicious share names
func TestUpdateShareConfigSecurity(t *testing.T) {
	collector := NewConfigCollector()

	maliciousNames := []string{
		"../../../etc/passwd",
		"../../etc/shadow",
		"/etc/passwd",
		"subdir/share",
	}

	for _, name := range maliciousNames {
		t.Run("Reject_"+name, func(t *testing.T) {
			config := &dto.ShareConfig{
				Name:      name,
				Allocator: "highwater",
			}
			err := collector.UpdateShareConfig(config)
			if err == nil {
				t.Errorf("UpdateShareConfig() should reject malicious name %q but returned nil", name)
			}
			if !strings.Contains(err.Error(), "invalid share name") {
				t.Errorf("UpdateShareConfig() error should mention 'invalid share name', got: %v", err)
			}
		})
	}
}
