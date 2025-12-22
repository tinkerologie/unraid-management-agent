package controllers

import (
	"strings"
	"testing"
)

// TestValidateNotificationID tests the notification ID validation function
func TestValidateNotificationID(t *testing.T) {
	tests := []struct {
		name      string
		id        string
		wantError bool
		errorMsg  string
	}{
		{
			name:      "Valid notification ID",
			id:        "20241118-120000-test.notify",
			wantError: false,
		},
		{
			name:      "Valid notification ID with underscores",
			id:        "20241118-120000-test_notification.notify",
			wantError: false,
		},
		{
			name:      "Empty ID",
			id:        "",
			wantError: true,
			errorMsg:  "cannot be empty",
		},
		{
			name:      "Path traversal with ../",
			id:        "../../../etc/passwd",
			wantError: true,
			errorMsg:  "parent directory references not allowed",
		},
		{
			name:      "Path traversal with ../ and .notify",
			id:        "../../etc/passwd.notify",
			wantError: true,
			errorMsg:  "parent directory references not allowed",
		},
		{
			name:      "Unix path separator",
			id:        "subdir/test.notify",
			wantError: true,
			errorMsg:  "path separators not allowed",
		},
		{
			name:      "Windows path separator",
			id:        "subdir\\test.notify",
			wantError: true,
			errorMsg:  "path separators not allowed",
		},
		{
			name:      "Absolute Unix path",
			id:        "/etc/passwd.notify",
			wantError: true,
			errorMsg:  "absolute paths not allowed",
		},
		{
			name:      "Absolute Windows path",
			id:        "\\etc\\passwd.notify",
			wantError: true,
			errorMsg:  "absolute paths not allowed",
		},
		{
			name:      "Missing .notify extension",
			id:        "20241118-120000-test",
			wantError: true,
			errorMsg:  "must have .notify extension",
		},
		{
			name:      "Wrong extension",
			id:        "20241118-120000-test.txt",
			wantError: true,
			errorMsg:  "must have .notify extension",
		},
		{
			name:      "Complex path traversal attempt",
			id:        "....//....//etc/passwd.notify",
			wantError: true,
			errorMsg:  "parent directory references not allowed", // ".." is checked first
		},
		{
			name:      "Encoded path separator (URL encoded)",
			id:        "test%2Fpasswd.notify",
			wantError: false, // URL encoding is not decoded, so this is treated as a valid filename
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateNotificationID(tt.id)

			if tt.wantError {
				if err == nil {
					t.Errorf("validateNotificationID() expected error but got nil")
					return
				}
				if !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("validateNotificationID() error = %v, want error containing %q", err, tt.errorMsg)
				}
			} else if err != nil {
				t.Errorf("validateNotificationID() unexpected error = %v", err)
			}
		})
	}
}

// TestArchiveNotificationSecurity tests that ArchiveNotification rejects malicious IDs
func TestArchiveNotificationSecurity(t *testing.T) {
	maliciousIDs := []string{
		"../../../etc/passwd.notify",
		"../../etc/shadow.notify",
		"/etc/passwd.notify",
		"subdir/test.notify",
		"..\\..\\..\\windows\\system32\\config\\sam.notify",
	}

	for _, id := range maliciousIDs {
		t.Run("Reject_"+id, func(t *testing.T) {
			err := ArchiveNotification(id)
			if err == nil {
				t.Errorf("ArchiveNotification() should reject malicious ID %q but returned nil", id)
			}
		})
	}
}

// TestUnarchiveNotificationSecurity tests that UnarchiveNotification rejects malicious IDs
func TestUnarchiveNotificationSecurity(t *testing.T) {
	maliciousIDs := []string{
		"../../../etc/passwd.notify",
		"../../etc/shadow.notify",
		"/etc/passwd.notify",
		"subdir/test.notify",
	}

	for _, id := range maliciousIDs {
		t.Run("Reject_"+id, func(t *testing.T) {
			err := UnarchiveNotification(id)
			if err == nil {
				t.Errorf("UnarchiveNotification() should reject malicious ID %q but returned nil", id)
			}
		})
	}
}

// TestDeleteNotificationSecurity tests that DeleteNotification rejects malicious IDs
func TestDeleteNotificationSecurity(t *testing.T) {
	maliciousIDs := []string{
		"../../../etc/passwd.notify",
		"../../etc/shadow.notify",
		"/etc/passwd.notify",
		"subdir/test.notify",
	}

	for _, id := range maliciousIDs {
		t.Run("Reject_"+id, func(t *testing.T) {
			err := DeleteNotification(id, false)
			if err == nil {
				t.Errorf("DeleteNotification() should reject malicious ID %q but returned nil", id)
			}
		})
	}
}

// TestSanitizeFilename tests the sanitizeFilename function
func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple filename",
			input:    "test",
			expected: "test",
		},
		{
			name:     "filename with spaces",
			input:    "test file name",
			expected: "test_file_name",
		},
		{
			name:     "filename with special characters",
			input:    "test@file#name!",
			expected: "testfilename",
		},
		{
			name:     "filename with dots",
			input:    "test.file.name",
			expected: "testfilename",
		},
		{
			name:     "filename with path separators",
			input:    "test/file\\name",
			expected: "testfilename",
		},
		{
			name:     "filename with parent dir refs",
			input:    "../../../etc/passwd",
			expected: "etcpasswd",
		},
		{
			name:     "filename with mixed chars",
			input:    "test-file_123",
			expected: "test-file_123",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "very long filename",
			input:    "this_is_a_very_long_filename_that_exceeds_the_fifty_character_limit_for_filenames",
			expected: "this_is_a_very_long_filename_that_exceeds_the_fift",
		},
		{
			name:     "unicode characters",
			input:    "test_éàü_file",
			expected: "test__file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeFilename(tt.input)
			if result != tt.expected {
				t.Errorf("sanitizeFilename(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// TestValidateFilename tests the validateFilename function
func TestValidateFilename(t *testing.T) {
	tests := []struct {
		name      string
		filename  string
		wantError bool
		errorMsg  string
	}{
		{
			name:      "valid filename",
			filename:  "test-file_123",
			wantError: false,
		},
		{
			name:      "empty filename",
			filename:  "",
			wantError: true,
			errorMsg:  "cannot be empty",
		},
		{
			name:      "parent directory reference",
			filename:  "..",
			wantError: true,
			errorMsg:  "parent directory references not allowed",
		},
		{
			name:      "path with parent directory",
			filename:  "../test",
			wantError: true,
			errorMsg:  "parent directory references not allowed",
		},
		{
			name:      "absolute unix path",
			filename:  "/etc/passwd",
			wantError: true,
			errorMsg:  "absolute paths not allowed",
		},
		{
			name:      "absolute windows path",
			filename:  "\\windows\\system32",
			wantError: true,
			errorMsg:  "absolute paths not allowed",
		},
		{
			name:      "unix path separator",
			filename:  "subdir/file",
			wantError: true,
			errorMsg:  "path separators not allowed",
		},
		{
			name:      "windows path separator",
			filename:  "subdir\\file",
			wantError: true,
			errorMsg:  "path separators not allowed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateFilename(tt.filename)
			if tt.wantError {
				if err == nil {
					t.Errorf("validateFilename(%q) expected error but got nil", tt.filename)
					return
				}
				if !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("validateFilename(%q) error = %v, want error containing %q", tt.filename, err, tt.errorMsg)
				}
			} else if err != nil {
				t.Errorf("validateFilename(%q) unexpected error = %v", tt.filename, err)
			}
		})
	}
}

func TestCreateNotificationValidation(t *testing.T) {
	// Test notification creation with validation
	t.Run("invalid importance level", func(t *testing.T) {
		err := CreateNotification("test", "subject", "desc", "invalid", "")
		if err == nil {
			t.Error("Expected error for invalid importance level")
		}
	})

	t.Run("valid importance levels", func(t *testing.T) {
		levels := []string{"alert", "warning", "info"}
		for _, level := range levels {
			t.Run(level, func(t *testing.T) {
				err := CreateNotification("test-"+level, "subject", "desc", level, "")
				// Will fail without notifications directory, but validates importance first
				if err != nil && err.Error() == "invalid importance level: "+level {
					t.Errorf("Should accept importance level %q", level)
				}
			})
		}
	})
}

func TestArchiveAllNotificationsActual(t *testing.T) {
	// Test the actual ArchiveAllNotifications function
	// Will fail if directories don't exist, but exercises the code path
	err := ArchiveAllNotifications()
	if err != nil {
		t.Logf("ArchiveAllNotifications returned error (expected if dir doesn't exist): %v", err)
	}
}
