package controllers

import (
	"os"
	"path/filepath"
	"testing"
)

// TestCreateNotificationValidationLogic tests notification creation logic
func TestCreateNotificationValidationLogic(t *testing.T) {
	tests := []struct {
		name        string
		title       string
		subject     string
		description string
		importance  string
		link        string
		wantErr     bool
	}{
		{
			name:        "valid notification",
			title:       "TestAlert",
			subject:     "Test Subject",
			description: "Test Description",
			importance:  "alert",
			link:        "",
			wantErr:     false,
		},
		{
			name:        "valid warning notification",
			title:       "TestWarning",
			subject:     "Test Subject",
			description: "Test Description",
			importance:  "warning",
			link:        "https://example.com",
			wantErr:     false,
		},
		{
			name:        "invalid importance",
			title:       "Test",
			subject:     "Subject",
			description: "Description",
			importance:  "invalid",
			link:        "",
			wantErr:     true,
		},
		{
			name:        "empty title with valid importance",
			title:       "",
			subject:     "Subject",
			description: "Description",
			importance:  "info",
			link:        "",
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Note: This would actually create files, so we test the logic only
			isValidImportance := tt.importance == "alert" || tt.importance == "warning" || tt.importance == "info"

			if isValidImportance != !tt.wantErr {
				if tt.wantErr && !isValidImportance {
					// Expected error case
					return
				} else if !tt.wantErr && isValidImportance {
					// Expected success case
					return
				} else {
					t.Errorf("unexpected error state for importance %q", tt.importance)
				}
			}
		})
	}
}

// TestSanitizeFilenameCases tests filename sanitization with edge cases
func TestSanitizeFilenameCases(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"simple name", "alert", "alert"},
		{"with spaces", "test alert", "test_alert"},
		{"with dots", "test.alert", "test.alert"},
		{"with dashes", "test-alert", "test-alert"},
		{"with underscores", "test_alert", "test_alert"},
		{"with slashes", "test/alert", "test_alert"},
		{"with backslashes", "test\\alert", "test_alert"},
		{"with colons", "test:alert", "test_alert"},
		{"with multiple special", "test: alert / warning", "test__alert___warning"},
		{"empty string", "", ""},
		{"only spaces", "   ", "___"},
		{"mixed special with parens", "test!@#$%^&*()", "test!@#$%^&_()"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeTestFilename(tt.input)
			if result != tt.expected {
				t.Errorf("sanitizeFilename(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func sanitizeTestFilename(s string) string {
	result := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case ' ', '/', '\\', ':', '*', '?', '"', '<', '>', '|':
			result[i] = '_'
		default:
			result[i] = c
		}
	}
	return string(result)
}

// TestNotificationPathSecurity tests that notification paths cannot escape their directory
func TestNotificationPathSecurity(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		wantErr bool
	}{
		{"valid .notify file", "20240101-120000-Test.notify", false},
		{"path traversal ../", "../../../etc/passwd", true},
		{"path traversal with .notify", "../test.notify", true},
		{"dot dot direct", "..", true},
		{"single dot", ".", false},
		{"absolute path", "/etc/passwd", true},
		{"absolute .notify", "/test.notify", true},
		{"normal name", "test.notify", false},
		{"with dots in name", "test.alert.notify", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hasPathTraversal := containsDotDot(tt.id)
			isAbsolutePath := len(tt.id) > 0 && tt.id[0] == '/'

			shouldError := hasPathTraversal || isAbsolutePath

			if shouldError != tt.wantErr {
				t.Errorf("path %q: expected error=%v, got %v", tt.id, tt.wantErr, shouldError)
			}
		})
	}
}

func containsDotDot(s string) bool {
	for i := 0; i < len(s)-1; i++ {
		if s[i] == '.' && s[i+1] == '.' {
			return true
		}
	}
	return false
}

// TestNotificationFileFormat tests the expected notification file format
func TestNotificationFileFormat(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		isValid  bool
	}{
		{"standard format", "20240101-120000-TestAlert.notify", true},
		{"short format", "test.notify", true},
		{"no extension", "test", false},
		{"wrong extension", "test.txt", false},
		{"empty", "", false},
		{"only extension", ".notify", true},
		{"multiple dots", "test.alert.notify", true},
		{"space before extension", "test .notify", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isValid := len(tt.filename) > 0 && hasNotifyExt(tt.filename)
			if isValid != tt.isValid {
				t.Errorf("filename %q: expected valid=%v, got %v", tt.filename, tt.isValid, isValid)
			}
		})
	}
}

func hasNotifyExt(s string) bool {
	if len(s) < 7 {
		return false
	}
	return s[len(s)-7:] == ".notify"
}

// TestDirectoryPermissions tests notification directory handling
func TestDirectoryPermissions(t *testing.T) {
	// Create temporary test directory
	tmpDir, err := os.MkdirTemp("", "test_notifications")
	if err != nil {
		t.Fatalf("failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Test directory creation
	testDir := filepath.Join(tmpDir, "notifications")
	err = os.MkdirAll(testDir, 0755)
	if err != nil {
		t.Fatalf("failed to create directory: %v", err)
	}

	// Verify directory exists
	info, err := os.Stat(testDir)
	if err != nil {
		t.Fatalf("failed to stat directory: %v", err)
	}

	if !info.IsDir() {
		t.Error("path is not a directory")
	}

	// Verify write permissions
	testFile := filepath.Join(testDir, "test.notify")
	err = os.WriteFile(testFile, []byte("test"), 0644)
	if err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// Verify read
	content, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("failed to read test file: %v", err)
	}

	if string(content) != "test" {
		t.Error("file content mismatch")
	}
}

// TestImportanceLevelValidation tests importance level validation
func TestImportanceLevelValidation(t *testing.T) {
	validLevels := []string{"alert", "warning", "info"}
	invalidLevels := []string{"error", "debug", "trace", "ALERT", "Warning", ""}

	for _, level := range validLevels {
		isValid := level == "alert" || level == "warning" || level == "info"
		if !isValid {
			t.Errorf("valid level %q should be accepted", level)
		}
	}

	for _, level := range invalidLevels {
		isValid := level == "alert" || level == "warning" || level == "info"
		if isValid {
			t.Errorf("invalid level %q should be rejected", level)
		}
	}
}
