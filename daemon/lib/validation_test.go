package lib

import (
	"strings"
	"testing"
)

func TestValidateContainerID(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid short ID lowercase",
			id:      "bbb57ffa3c50",
			wantErr: false,
		},
		{
			name:    "valid short ID uppercase",
			id:      "BBB57FFA3C50",
			wantErr: false,
		},
		{
			name:    "valid full ID",
			id:      "bbb57ffa3c50a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6",
			wantErr: false,
		},
		{
			name:    "empty ID",
			id:      "",
			wantErr: true,
			errMsg:  "cannot be empty",
		},
		{
			name:    "too short",
			id:      "bbb57ffa3c5",
			wantErr: true,
			errMsg:  "invalid container ID format",
		},
		{
			name:    "too long (not 64)",
			id:      "bbb57ffa3c50a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7",
			wantErr: true,
			errMsg:  "invalid container ID format",
		},
		{
			name:    "contains non-hex characters",
			id:      "bbb57ffa3cXY",
			wantErr: true,
			errMsg:  "invalid container ID format",
		},
		{
			name:    "contains special characters",
			id:      "bbb57ffa-3c50",
			wantErr: true,
			errMsg:  "invalid container ID format",
		},
		{
			name:    "SQL injection attempt",
			id:      "'; DROP TABLE--",
			wantErr: true,
			errMsg:  "invalid container ID format",
		},
		{
			name:    "command injection attempt",
			id:      "abc123; rm -rf /",
			wantErr: true,
			errMsg:  "invalid container ID format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateContainerID(tt.id)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateContainerID() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil && tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("ValidateContainerID() error = %v, want error containing %q", err, tt.errMsg)
			}
		})
	}
}

func TestValidateVMName(t *testing.T) {
	tests := []struct {
		name    string
		vmName  string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid simple name",
			vmName:  "windows10",
			wantErr: false,
		},
		{
			name:    "valid name with hyphen",
			vmName:  "ubuntu-server",
			wantErr: false,
		},
		{
			name:    "valid name with underscore",
			vmName:  "debian_vm",
			wantErr: false,
		},
		{
			name:    "valid name with dot",
			vmName:  "vm.test",
			wantErr: false,
		},
		{
			name:    "valid complex name",
			vmName:  "prod-web-server_01.domain",
			wantErr: false,
		},
		{
			name:    "empty name",
			vmName:  "",
			wantErr: true,
			errMsg:  "cannot be empty",
		},
		{
			name:    "name too long",
			vmName:  strings.Repeat("a", 254),
			wantErr: true,
			errMsg:  "too long",
		},
		{
			name:    "starts with hyphen",
			vmName:  "-invalid",
			wantErr: true,
			errMsg:  "cannot start or end with hyphen",
		},
		{
			name:    "ends with hyphen",
			vmName:  "invalid-",
			wantErr: true,
			errMsg:  "cannot start or end with hyphen",
		},
		{
			name:    "starts with dot",
			vmName:  ".invalid",
			wantErr: true,
			errMsg:  "cannot start or end with dot",
		},
		{
			name:    "ends with dot",
			vmName:  "invalid.",
			wantErr: true,
			errMsg:  "cannot start or end with dot",
		},
		{
			name:    "contains spaces",
			vmName:  "invalid name",
			wantErr: true,
			errMsg:  "invalid VM name format",
		},
		{
			name:    "contains special characters",
			vmName:  "invalid@name",
			wantErr: true,
			errMsg:  "invalid VM name format",
		},
		{
			name:    "command injection attempt",
			vmName:  "vm; rm -rf /",
			wantErr: true,
			errMsg:  "invalid VM name format",
		},
		{
			name:    "path traversal attempt",
			vmName:  "../../../etc/passwd",
			wantErr: true,
			errMsg:  "invalid VM name format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateVMName(tt.vmName)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateVMName() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil && tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("ValidateVMName() error = %v, want error containing %q", err, tt.errMsg)
			}
		})
	}
}

func TestValidateDiskID(t *testing.T) {
	tests := []struct {
		name    string
		diskID  string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid sda",
			diskID:  "sda",
			wantErr: false,
		},
		{
			name:    "valid sdb1",
			diskID:  "sdb1",
			wantErr: false,
		},
		{
			name:    "valid nvme0n1",
			diskID:  "nvme0n1",
			wantErr: false,
		},
		{
			name:    "valid nvme0n1p1",
			diskID:  "nvme0n1p1",
			wantErr: false,
		},
		{
			name:    "valid md0",
			diskID:  "md0",
			wantErr: false,
		},
		{
			name:    "valid loop0",
			diskID:  "loop0",
			wantErr: false,
		},
		{
			name:    "empty ID",
			diskID:  "",
			wantErr: true,
			errMsg:  "cannot be empty",
		},
		{
			name:    "invalid format",
			diskID:  "invalid",
			wantErr: true,
			errMsg:  "invalid disk ID format",
		},
		{
			name:    "command injection attempt",
			diskID:  "sda; rm -rf /",
			wantErr: true,
			errMsg:  "invalid disk ID format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateDiskID(tt.diskID)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateDiskID() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil && tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("ValidateDiskID() error = %v, want error containing %q", err, tt.errMsg)
			}
		})
	}
}

func TestValidateShareName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid share name lowercase",
			input:   "appdata",
			wantErr: false,
		},
		{
			name:    "valid share name uppercase",
			input:   "MEDIA",
			wantErr: false,
		},
		{
			name:    "valid share name mixed case",
			input:   "MyShare",
			wantErr: false,
		},
		{
			name:    "valid share name with underscore",
			input:   "app_data",
			wantErr: false,
		},
		{
			name:    "valid share name with hyphen",
			input:   "app-data",
			wantErr: false,
		},
		{
			name:    "valid share name with numbers",
			input:   "share123",
			wantErr: false,
		},
		{
			name:    "empty share name",
			input:   "",
			wantErr: true,
			errMsg:  "cannot be empty",
		},
		{
			name:    "path traversal with ../",
			input:   "../etc/passwd",
			wantErr: true,
			errMsg:  "cannot contain parent directory references",
		},
		{
			name:    "path traversal with ..",
			input:   "..",
			wantErr: true,
			errMsg:  "cannot contain parent directory references",
		},
		{
			name:    "absolute path",
			input:   "/etc/passwd",
			wantErr: true,
			errMsg:  "cannot contain path separators",
		},
		{
			name:    "forward slash in name",
			input:   "app/data",
			wantErr: true,
			errMsg:  "cannot contain path separators",
		},
		{
			name:    "backslash in name",
			input:   "app\\data",
			wantErr: true,
			errMsg:  "cannot contain path separators",
		},
		{
			name:    "starts with hyphen",
			input:   "-appdata",
			wantErr: true,
			errMsg:  "cannot start or end with hyphen",
		},
		{
			name:    "ends with hyphen",
			input:   "appdata-",
			wantErr: true,
			errMsg:  "cannot start or end with hyphen",
		},
		{
			name:    "contains special characters",
			input:   "app@data",
			wantErr: true,
			errMsg:  "invalid share name format",
		},
		{
			name:    "contains spaces",
			input:   "app data",
			wantErr: true,
			errMsg:  "invalid share name format",
		},
		{
			name:    "too long (256 chars)",
			input:   strings.Repeat("a", 256),
			wantErr: true,
			errMsg:  "too long",
		},
		{
			name:    "max length (255 chars)",
			input:   strings.Repeat("a", 255),
			wantErr: false,
		},
		{
			name:    "SQL injection attempt",
			input:   "'; DROP TABLE shares--",
			wantErr: true,
			errMsg:  "invalid share name format",
		},
		{
			name:    "command injection attempt",
			input:   "share; rm -rf /",
			wantErr: true,
			errMsg:  "cannot contain path separators", // Contains "/" which is caught first
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateShareName(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateShareName() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("ValidateShareName() error = %v, expected to contain %q", err, tt.errMsg)
			}
		})
	}
}

func TestValidateNonEmpty(t *testing.T) {
	tests := []struct {
		name      string
		value     string
		fieldName string
		wantErr   bool
	}{
		{
			name:      "valid non-empty",
			value:     "test",
			fieldName: "field",
			wantErr:   false,
		},
		{
			name:      "empty string",
			value:     "",
			fieldName: "field",
			wantErr:   true,
		},
		{
			name:      "whitespace only",
			value:     "   ",
			fieldName: "field",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateNonEmpty(tt.value, tt.fieldName)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateNonEmpty() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateMaxLength(t *testing.T) {
	tests := []struct {
		name      string
		value     string
		fieldName string
		maxLength int
		wantErr   bool
	}{
		{
			name:      "within limit",
			value:     "test",
			fieldName: "field",
			maxLength: 10,
			wantErr:   false,
		},
		{
			name:      "at limit",
			value:     "test",
			fieldName: "field",
			maxLength: 4,
			wantErr:   false,
		},
		{
			name:      "exceeds limit",
			value:     "test",
			fieldName: "field",
			maxLength: 3,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateMaxLength(tt.value, tt.fieldName, tt.maxLength)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateMaxLength() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
