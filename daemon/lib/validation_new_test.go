package lib

import (
	"strings"
	"testing"
)

func TestValidateContainerRef(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		errMsg  string
	}{
		// Valid container IDs (short hex)
		{"valid short hex ID", "4f0dc0851511", false, ""},
		{"valid short hex ID lowercase", "abcdef012345", false, ""},
		// Valid container IDs (full hex)
		{"valid full hex ID", "4f0dc085151100000000000000000000000000000000000000000000deadbeef", false, ""},
		// Valid container names
		{"valid name simple", "jackett", false, ""},
		{"valid name with hyphens", "my-container", false, ""},
		{"valid name with dots", "my.container", false, ""},
		{"valid name with underscores", "my_container", false, ""},
		{"valid name mixed", "plex-server_v2.1", false, ""},
		// Invalid empty
		{"empty string", "", true, "cannot be empty"},
		// Invalid path traversal
		{"path traversal dots", "../etc/passwd", true, "path separators or directory traversal"},
		{"path traversal slash", "container/name", true, "path separators or directory traversal"},
		{"path traversal backslash", "container\\name", true, "path separators or directory traversal"},
		// Invalid null byte
		{"null byte", "container\x00name", true, "null bytes"},
		// Invalid too long
		{"too long name", strings.Repeat("a", 256), true, "too long"},
		// Invalid characters
		{"invalid special chars", "container@name", true, "invalid container reference"},
		{"invalid spaces", "my container", true, "invalid container reference"},
		{"starts with hyphen", "-container", true, "invalid container reference"},
		// Security edge cases
		{"SQL injection", "'; DROP TABLE--", true, ""},
		{"command injection", "$(whoami)", true, ""},
		{"command injection backtick", "`id`", true, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateContainerRef(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateContainerRef(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if err != nil && tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("ValidateContainerRef(%q) error = %q, want containing %q", tt.input, err.Error(), tt.errMsg)
			}
		})
	}
}

func TestValidatePluginName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		errMsg  string
	}{
		// Valid names
		{"simple name", "my-plugin", false, ""},
		{"dots and hyphens", "unraid-management-agent.plg", false, ""},
		{"underscores", "my_plugin", false, ""},
		{"alphanumeric", "plugin123", false, ""},
		{"single char", "a", false, ""},
		// Invalid
		{"empty string", "", true, "cannot be empty"},
		{"path traversal", "../etc/passwd", true, "path separators or directory traversal"},
		{"slash", "plugin/name", true, "path separators or directory traversal"},
		{"backslash", "plugin\\name", true, "path separators or directory traversal"},
		{"spaces", "my plugin", true, "invalid plugin name"},
		{"special chars", "plugin@name", true, "invalid plugin name"},
		// Security edge cases
		{"command injection", "$(id)", true, ""},
		{"null byte", "plugin\x00", true, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePluginName(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePluginName(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if err != nil && tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("ValidatePluginName(%q) error = %q, want containing %q", tt.input, err.Error(), tt.errMsg)
			}
		})
	}
}

func TestValidateServiceName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		errMsg  string
	}{
		// Valid names
		{"docker", "docker", false, ""},
		{"sshd", "sshd", false, ""},
		{"smb", "smb", false, ""},
		{"nfs", "nfs", false, ""},
		{"nginx", "nginx", false, ""},
		{"with hyphen", "my-service", false, ""},
		{"with numbers", "service1", false, ""},
		// Invalid
		{"empty string", "", true, "cannot be empty"},
		{"special chars", "svc@name", true, "lowercase"},
		{"starts with hyphen", "-service", true, "lowercase"},
		{"starts with number", "1service", true, "lowercase"},
		{"spaces", "my service", true, "lowercase"},
		{"too long", strings.Repeat("a", 65), true, "lowercase"},
		// Valid (uppercase is lowered before regex check)
		{"uppercase accepted", "Docker", false, ""},
		// Security edge cases
		{"path traversal", "../etc", true, ""},
		{"command injection", "$(id)", true, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateServiceName(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateServiceName(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if err != nil && tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("ValidateServiceName(%q) error = %q, want containing %q", tt.input, err.Error(), tt.errMsg)
			}
		})
	}
}

func TestValidateSnapshotName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		errMsg  string
	}{
		// Valid names
		{"simple name", "snapshot1", false, ""},
		{"with hyphens", "pre-update-snapshot", false, ""},
		{"with underscores", "snapshot_v1", false, ""},
		{"with dots", "snapshot.2024", false, ""},
		{"mixed", "backup-2024.01.15_v1", false, ""},
		// Invalid
		{"empty string", "", true, "cannot be empty"},
		{"path traversal", "../etc", true, "path separators or directory traversal"},
		{"slash", "snap/name", true, "path separators or directory traversal"},
		{"backslash", "snap\\name", true, "path separators or directory traversal"},
		{"starts with dot", ".hidden", true, "must start with alphanumeric"},
		{"starts with hyphen", "-snapshot", true, "must start with alphanumeric"},
		{"starts with underscore", "_snapshot", true, "must start with alphanumeric"},
		{"spaces", "my snapshot", true, "must start with alphanumeric"},
		{"special chars", "snap@shot", true, "must start with alphanumeric"},
		// Security edge cases
		{"command injection", "$(whoami)", true, ""},
		{"null traversal", "snap\x00shot", true, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSnapshotName(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateSnapshotName(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if err != nil && tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("ValidateSnapshotName(%q) error = %q, want containing %q", tt.input, err.Error(), tt.errMsg)
			}
		})
	}
}

// Test regex patterns directly for edge cases
func TestContainerNameRegex(t *testing.T) {
	tests := []struct {
		name  string
		input string
		match bool
	}{
		{"simple", "nginx", true},
		{"with digits", "container123", true},
		{"with dots", "my.container", true},
		{"with hyphens", "my-container", true},
		{"with underscores", "my_container", true},
		{"starts with digit", "1container", true},
		{"empty", "", false},
		{"starts with dot", ".container", false},
		{"starts with hyphen", "-container", false},
		{"starts with underscore", "_container", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := containerNameRegex.MatchString(tt.input)
			if got != tt.match {
				t.Errorf("containerNameRegex.MatchString(%q) = %v, want %v", tt.input, got, tt.match)
			}
		})
	}
}

func TestPluginNameRegex(t *testing.T) {
	tests := []struct {
		name  string
		input string
		match bool
	}{
		{"simple", "myplugin", true},
		{"with hyphens", "my-plugin", true},
		{"with dots", "my.plugin.plg", true},
		{"with underscores", "my_plugin", true},
		{"single char", "a", true},
		{"max length", strings.Repeat("a", 255), true},
		{"over max length", strings.Repeat("a", 256), false},
		{"empty", "", false},
		{"spaces", "my plugin", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pluginNameRegex.MatchString(tt.input)
			if got != tt.match {
				t.Errorf("pluginNameRegex.MatchString(%q) = %v, want %v", tt.input, got, tt.match)
			}
		})
	}
}

func TestServiceNameRegex(t *testing.T) {
	tests := []struct {
		name  string
		input string
		match bool
	}{
		{"simple", "docker", true},
		{"with hyphen", "my-service", true},
		{"with numbers", "service1", true},
		{"single char", "a", true},
		{"starts with number", "1service", false},
		{"uppercase", "Docker", false},
		{"spaces", "my service", false},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := serviceNameRegex.MatchString(tt.input)
			if got != tt.match {
				t.Errorf("serviceNameRegex.MatchString(%q) = %v, want %v", tt.input, got, tt.match)
			}
		})
	}
}

func TestSnapshotNameRegex(t *testing.T) {
	tests := []struct {
		name  string
		input string
		match bool
	}{
		{"simple", "snapshot1", true},
		{"with hyphens", "pre-update", true},
		{"with dots", "snap.2024", true},
		{"with underscores", "snap_v1", true},
		{"starts with dot", ".hidden", false},
		{"starts with hyphen", "-snap", false},
		{"starts with underscore", "_snap", false},
		{"empty", "", false},
		{"spaces", "my snapshot", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := snapshotNameRegex.MatchString(tt.input)
			if got != tt.match {
				t.Errorf("snapshotNameRegex.MatchString(%q) = %v, want %v", tt.input, got, tt.match)
			}
		})
	}
}
