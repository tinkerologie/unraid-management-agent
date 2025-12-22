package controllers

import (
	"testing"
)

func TestArchiveNotificationValidation(t *testing.T) {
	// Test with empty ID
	t.Run("empty id", func(t *testing.T) {
		err := ArchiveNotification("")
		if err == nil {
			t.Error("Expected error for empty notification ID")
		}
	})

	// Test with path traversal attempt
	t.Run("path traversal", func(t *testing.T) {
		err := ArchiveNotification("../../../etc/passwd.notify")
		if err == nil {
			t.Error("Expected error for path traversal attempt")
		}
	})

	// Test with invalid extension
	t.Run("invalid extension", func(t *testing.T) {
		err := ArchiveNotification("test-notification.txt")
		if err == nil {
			t.Error("Expected error for invalid extension")
		}
	})

	// Test with forward slash in ID
	t.Run("forward slash in id", func(t *testing.T) {
		err := ArchiveNotification("path/to/notification.notify")
		if err == nil {
			t.Error("Expected error for path separator")
		}
	})
}

func TestUnarchiveNotificationValidation(t *testing.T) {
	// Test with empty ID
	t.Run("empty id", func(t *testing.T) {
		err := UnarchiveNotification("")
		if err == nil {
			t.Error("Expected error for empty notification ID")
		}
	})

	// Test with path traversal attempt
	t.Run("path traversal", func(t *testing.T) {
		err := UnarchiveNotification("../../../etc/passwd.notify")
		if err == nil {
			t.Error("Expected error for path traversal attempt")
		}
	})

	// Test with invalid extension
	t.Run("invalid extension", func(t *testing.T) {
		err := UnarchiveNotification("test-notification.txt")
		if err == nil {
			t.Error("Expected error for invalid extension")
		}
	})
}

func TestDeleteNotificationValidation(t *testing.T) {
	// Test with empty ID
	t.Run("empty id", func(t *testing.T) {
		err := DeleteNotification("", false)
		if err == nil {
			t.Error("Expected error for empty notification ID")
		}
	})

	// Test with path traversal attempt
	t.Run("path traversal", func(t *testing.T) {
		err := DeleteNotification("../../../etc/passwd.notify", false)
		if err == nil {
			t.Error("Expected error for path traversal attempt")
		}
	})

	// Test with backslash in ID
	t.Run("backslash in id", func(t *testing.T) {
		err := DeleteNotification("path\\to\\notification.notify", false)
		if err == nil {
			t.Error("Expected error for path separator")
		}
	})

	// Test with archived=true as well
	t.Run("archived path traversal", func(t *testing.T) {
		err := DeleteNotification("../../../etc/passwd.notify", true)
		if err == nil {
			t.Error("Expected error for path traversal attempt with archived=true")
		}
	})
}
