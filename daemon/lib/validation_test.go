package lib

import (
	"strings"
	"testing"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

func TestValidateHostOrIP(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		errMsg  string
	}{
		// Valid hostnames
		{name: "simple hostname", input: "localhost", wantErr: false},
		{name: "hostname with hyphen", input: "my-server", wantErr: false},
		{name: "FQDN", input: "server.example.com", wantErr: false},
		{name: "multi-label FQDN", input: "a.b.c.example.org", wantErr: false},
		{name: "hostname with numbers", input: "server01", wantErr: false},
		{name: "single letter", input: "a", wantErr: false},
		// Valid IPs
		{name: "IPv4 loopback", input: "127.0.0.1", wantErr: false},
		{name: "IPv4 address", input: "192.168.1.100", wantErr: false},
		{name: "IPv6 loopback", input: "::1", wantErr: false},
		{name: "IPv6 full", input: "2001:db8::1", wantErr: false},
		// Invalid: empty
		{name: "empty string", input: "", wantErr: true, errMsg: "cannot be empty"},
		// Invalid: flag injection via leading hyphen
		{name: "leading hyphen", input: "-n", wantErr: true, errMsg: "must not start with a hyphen"},
		{name: "leading double hyphen", input: "--verbose", wantErr: true, errMsg: "must not start with a hyphen"},
		// Invalid: whitespace
		{name: "space only", input: " ", wantErr: true},
		{name: "embedded space", input: "my server", wantErr: true},
		{name: "tab character", input: "host\tname", wantErr: true},
		// Invalid: special / shell characters
		{name: "semicolon injection", input: "host;rm -rf /", wantErr: true},
		{name: "backtick injection", input: "host`cmd`", wantErr: true},
		{name: "dollar sign", input: "host$VAR", wantErr: true},
		{name: "ampersand", input: "host&cmd", wantErr: true},
		{name: "pipe", input: "host|cmd", wantErr: true},
		// Invalid: null byte
		{name: "null byte", input: "host\x00name", wantErr: true},
		// Invalid: length
		{name: "exceeds 253 chars", input: strings.Repeat("a", 254), wantErr: true, errMsg: "exceeds 253"},
		// Edge: labels ending with a hyphen are rejected by the RFC-1123 hostname regex.
		{name: "label ending hyphen", input: "host-", wantErr: true},
		// Edge: a trailing dot makes an otherwise valid label not match the regex.
		{name: "trailing dot", input: "host.", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateHostOrIP(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateHostOrIP(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("ValidateHostOrIP(%q) error = %q, want to contain %q", tt.input, err.Error(), tt.errMsg)
			}
		})
	}
}

func TestValidateRemoteShareSource(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		errMsg  string
	}{
		// Valid SMB / NFS sources
		{name: "smb source", input: "//192.168.1.100/backup", wantErr: false},
		{name: "smb nested share", input: "//tower/media/movies", wantErr: false},
		{name: "smb hostname", input: "//nas.local/data", wantErr: false},
		{name: "nfs source", input: "192.168.1.50:/export/media", wantErr: false},
		{name: "nfs hostname", input: "nas:/srv/share", wantErr: false},
		{name: "smb share with space", input: "//server/Media Library", wantErr: false},
		// SMB hidden/admin shares legitimately contain '$' — must be accepted.
		{name: "smb hidden share", input: "//server/share$", wantErr: false},
		// Invalid: structure
		{name: "empty", input: "", wantErr: true, errMsg: "cannot be empty"},
		{name: "blank whitespace", input: "   ", wantErr: true, errMsg: "SMB"},
		{name: "smb missing share", input: "//server", wantErr: true, errMsg: "SMB"},
		{name: "smb empty share", input: "//server/", wantErr: true, errMsg: "SMB"},
		{name: "smb only slashes", input: "//", wantErr: true, errMsg: "SMB"},
		{name: "nfs missing export", input: ":/export", wantErr: true, errMsg: "NFS"},
		{name: "plain path", input: "/mnt/user/backup", wantErr: true, errMsg: "SMB"},
		{name: "bare hostname", input: "justahost", wantErr: true, errMsg: "SMB"},
		// Invalid: injection / traversal / control
		{name: "leading hyphen", input: "-rf", wantErr: true, errMsg: "must not start with a hyphen"},
		{name: "null byte", input: "//server/share\x00", wantErr: true, errMsg: "control characters"},
		{name: "newline", input: "//server/share\nrm", wantErr: true, errMsg: "control characters"},
		{name: "path traversal smb", input: "//server/../share", wantErr: true, errMsg: ".."},
		{name: "path traversal nfs", input: "nas:/../export", wantErr: true, errMsg: ".."},
		{name: "too long", input: "//s/" + strings.Repeat("a", 4096), wantErr: true, errMsg: "exceeds 4096"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRemoteShareSource(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateRemoteShareSource(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("ValidateRemoteShareSource(%q) error = %q, want to contain %q", tt.input, err.Error(), tt.errMsg)
			}
		})
	}
}

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
			vmName:  "valid name with spaces",
			wantErr: false,
			errMsg:  "",
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

func TestValidateUserScriptName(t *testing.T) {
	tests := []struct {
		name    string
		script  string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid simple name",
			script:  "my_script",
			wantErr: false,
		},
		{
			name:    "valid name with hyphen",
			script:  "my-script",
			wantErr: false,
		},
		{
			name:    "valid name with underscore",
			script:  "my_script_v2",
			wantErr: false,
		},
		{
			name:    "valid name with dot",
			script:  "backup.sh",
			wantErr: false,
		},
		{
			name:    "valid alphanumeric",
			script:  "script123",
			wantErr: false,
		},
		{
			name:    "empty name",
			script:  "",
			wantErr: true,
			errMsg:  "cannot be empty",
		},
		{
			name:    "too long",
			script:  strings.Repeat("a", 256),
			wantErr: true,
			errMsg:  "too long",
		},
		{
			name:    "path traversal with ../",
			script:  "../etc/passwd",
			wantErr: true,
			errMsg:  "cannot contain parent directory references",
		},
		{
			name:    "path traversal with ..",
			script:  "script..evil",
			wantErr: true,
			errMsg:  "cannot contain parent directory references",
		},
		{
			name:    "forward slash",
			script:  "path/to/script",
			wantErr: true,
			errMsg:  "cannot contain path separators",
		},
		{
			name:    "backslash",
			script:  "path\\to\\script",
			wantErr: true,
			errMsg:  "cannot contain path separators",
		},
		{
			name:    "starts with hyphen",
			script:  "-script",
			wantErr: true,
			errMsg:  "cannot start or end with hyphen",
		},
		{
			name:    "ends with hyphen",
			script:  "script-",
			wantErr: true,
			errMsg:  "cannot start or end with hyphen",
		},
		{
			name:    "starts with dot",
			script:  ".hidden",
			wantErr: true,
			errMsg:  "cannot start or end with dot",
		},
		{
			name:    "ends with dot",
			script:  "script.",
			wantErr: true,
			errMsg:  "cannot start or end with dot",
		},
		{
			name:    "contains spaces",
			script:  "my script",
			wantErr: true,
			errMsg:  "invalid user script name format",
		},
		{
			name:    "command injection attempt",
			script:  "script;rm -rf",
			wantErr: true,
			errMsg:  "invalid user script name format",
		},
		{
			name:    "SQL injection attempt",
			script:  "script' OR '1'='1",
			wantErr: true,
			errMsg:  "invalid user script name format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateUserScriptName(tt.script)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateUserScriptName() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && tt.errMsg != "" && err != nil {
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("ValidateUserScriptName() error = %v, want error containing %q", err, tt.errMsg)
				}
			}
		})
	}
}

func TestValidateFanTempSource(t *testing.T) {
	tests := []struct {
		name    string
		src     dto.FanTempSource
		wantErr bool
	}{
		{"hwmon ok", dto.FanTempSource{Type: dto.FanTempSourceHwmon, SensorPath: "/sys/class/hwmon/hwmon0/temp1_input"}, false},
		{"drives ok", dto.FanTempSource{Type: dto.FanTempSourceDrives, DriveIDs: []string{"disk1"}, FallbackSensorPath: "/sys/class/hwmon/hwmon0/temp1_input"}, false},
		{"bad type", dto.FanTempSource{Type: "bogus"}, true},
		{"drives empty list", dto.FanTempSource{Type: dto.FanTempSourceDrives}, true},
		{"path traversal", dto.FanTempSource{Type: dto.FanTempSourceHwmon, SensorPath: "/sys/class/hwmon/../../etc/passwd"}, true},
		{"path outside hwmon", dto.FanTempSource{Type: dto.FanTempSourceHwmon, SensorPath: "/etc/passwd"}, true},
		{"bad fallback path", dto.FanTempSource{Type: dto.FanTempSourceDrives, DriveIDs: []string{"disk1"}, FallbackSensorPath: "/etc/shadow"}, true},
		{"path too long", dto.FanTempSource{Type: dto.FanTempSourceHwmon, SensorPath: "/sys/class/hwmon/" + strings.Repeat("a", 5000)}, true},
		{"null byte in path", dto.FanTempSource{Type: dto.FanTempSourceHwmon, SensorPath: "/sys/class/hwmon/hwmon0/temp1_input\x00"}, true},
		{"non-input hwmon path", dto.FanTempSource{Type: dto.FanTempSourceHwmon, SensorPath: "/sys/class/hwmon/hwmon0/name"}, true},
		{"empty drive id", dto.FanTempSource{Type: dto.FanTempSourceDrives, DriveIDs: []string{"disk1", ""}}, true},
		{"drive id with slash", dto.FanTempSource{Type: dto.FanTempSourceDrives, DriveIDs: []string{"../etc"}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateFanTempSource(tt.src)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateFanTempSource(%+v) err=%v wantErr=%v", tt.src, err, tt.wantErr)
			}
		})
	}
}

func TestValidateLogFilename(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		wantErr  bool
	}{
		{
			name:     "valid simple filename",
			filename: "syslog",
			wantErr:  false,
		},
		{
			name:     "valid filename with extension",
			filename: "app.log",
			wantErr:  false,
		},
		{
			name:     "valid plugin log path",
			filename: "plugin/my-plugin.log",
			wantErr:  false,
		},
		{
			name:     "valid nested path",
			filename: "logs/2024/01/app.log",
			wantErr:  false,
		},
		{
			name:     "empty filename",
			filename: "",
			wantErr:  true,
		},
		{
			name:     "too long",
			filename: strings.Repeat("a", 256),
			wantErr:  true,
		},
		{
			name:     "path traversal with ../",
			filename: "../etc/passwd",
			wantErr:  true,
		},
		{
			name:     "path traversal with ..",
			filename: "logs/../etc/passwd",
			wantErr:  true,
		},
		{
			name:     "backslash",
			filename: "path\\to\\file",
			wantErr:  true,
		},
		{
			name:     "absolute path",
			filename: "/var/log/syslog",
			wantErr:  true,
		},
		{
			name:     "null byte injection",
			filename: "file\x00.log",
			wantErr:  true,
		},
		{
			name:     "valid with numbers",
			filename: "log123.txt",
			wantErr:  false,
		},
		{
			name:     "valid with hyphen",
			filename: "my-log-file.log",
			wantErr:  false,
		},
		{
			name:     "valid with underscore",
			filename: "my_log_file.log",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateLogFilename(tt.filename)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateLogFilename(%q) error = %v, wantErr %v", tt.filename, err, tt.wantErr)
			}
		})
	}
}
