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
