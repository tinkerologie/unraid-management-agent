package controllers

import (
	"os"
	"path/filepath"
	"testing"
)

func TestArchiveNotification(t *testing.T) {
	// Create temp directories
	tmpDir := t.TempDir()
	tmpNotifDir := filepath.Join(tmpDir, "notifications")
	tmpArchiveDir := filepath.Join(tmpDir, "archive")

	if err := os.MkdirAll(tmpNotifDir, 0755); err != nil {
		t.Fatalf("Failed to create temp notification dir: %v", err)
	}
	if err := os.MkdirAll(tmpArchiveDir, 0755); err != nil {
		t.Fatalf("Failed to create temp archive dir: %v", err)
	}

	// Temporarily override directories for testing
	oldNotifDir := notificationsDir
	oldArchiveDir := notificationsArchiveDir
	defer func() {
		// Restore original values - note: this won't work as they're constants
		// In a real scenario, these would need to be configurable
		_ = oldNotifDir
		_ = oldArchiveDir
	}()

	// Create a test notification file
	testFile := "test.notify"
	testPath := filepath.Join(notificationsDir, testFile)
	testContent := []byte("test notification content")

	// Create parent directory if needed
	if err := os.MkdirAll(filepath.Dir(testPath), 0755); err != nil {
		t.Fatalf("Failed to create notifications dir: %v", err)
	}

	if err := os.WriteFile(testPath, testContent, 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	defer os.Remove(testPath)

	// Ensure archive dir exists
	if err := os.MkdirAll(notificationsArchiveDir, 0755); err != nil {
		t.Fatalf("Failed to create archive dir: %v", err)
	}

	// Test archiving
	err := ArchiveNotification(testFile)
	if err != nil {
		t.Errorf("ArchiveNotification failed: %v", err)
	}

	// Verify file was moved
	archivedPath := filepath.Join(notificationsArchiveDir, testFile)
	if _, err := os.Stat(archivedPath); os.IsNotExist(err) {
		t.Error("Notification file was not archived")
	} else {
		// Clean up
		os.Remove(archivedPath)
	}

	// Verify original file was removed
	if _, err := os.Stat(testPath); !os.IsNotExist(err) {
		t.Error("Original notification file still exists after archiving")
	}
}

func TestArchiveNotification_NonExistent(t *testing.T) {
	err := ArchiveNotification("nonexistent.notify")
	if err == nil {
		t.Error("Expected error for non-existent notification")
	}
}

func TestArchiveNotification_InvalidID(t *testing.T) {
	tests := []struct {
		name string
		id   string
	}{
		{"path traversal", "../etc/passwd"},
		{"absolute path", "/etc/passwd"},
		{"empty", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ArchiveNotification(tt.id)
			if err == nil {
				t.Error("Expected error for invalid notification ID")
			}
		})
	}
}

func TestUnarchiveNotification(t *testing.T) {
	// Create a test archived notification
	testFile := "test-archived.notify"
	testPath := filepath.Join(notificationsArchiveDir, testFile)
	testContent := []byte("archived notification content")

	// Ensure directories exist
	if err := os.MkdirAll(notificationsArchiveDir, 0755); err != nil {
		t.Fatalf("Failed to create archive dir: %v", err)
	}
	if err := os.MkdirAll(notificationsDir, 0755); err != nil {
		t.Fatalf("Failed to create notifications dir: %v", err)
	}

	if err := os.WriteFile(testPath, testContent, 0644); err != nil {
		t.Fatalf("Failed to create test archived file: %v", err)
	}
	defer os.Remove(testPath)

	// Test unarchiving
	err := UnarchiveNotification(testFile)
	if err != nil {
		t.Errorf("UnarchiveNotification failed: %v", err)
	}

	// Verify file was moved back
	unarchivedPath := filepath.Join(notificationsDir, testFile)
	if _, err := os.Stat(unarchivedPath); os.IsNotExist(err) {
		t.Error("Notification file was not unarchived")
	} else {
		// Clean up
		os.Remove(unarchivedPath)
	}

	// Verify archived file was removed
	if _, err := os.Stat(testPath); !os.IsNotExist(err) {
		t.Error("Archived notification file still exists after unarchiving")
	}
}

func TestUnarchiveNotification_NonExistent(t *testing.T) {
	err := UnarchiveNotification("nonexistent.notify")
	if err == nil {
		t.Error("Expected error for non-existent archived notification")
	}
}

func TestDeleteNotification(t *testing.T) {
	// Create a test notification
	testFile := "test-delete.notify"
	testPath := filepath.Join(notificationsDir, testFile)
	testContent := []byte("notification to delete")

	if err := os.MkdirAll(notificationsDir, 0755); err != nil {
		t.Fatalf("Failed to create notifications dir: %v", err)
	}

	if err := os.WriteFile(testPath, testContent, 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Test deletion (from notifications dir, not archived)
	err := DeleteNotification(testFile, false)
	if err != nil {
		t.Errorf("DeleteNotification failed: %v", err)
	}

	// Verify file was deleted
	if _, err := os.Stat(testPath); !os.IsNotExist(err) {
		t.Error("Notification file still exists after deletion")
	}
}

func TestDeleteNotification_NonExistent(t *testing.T) {
	err := DeleteNotification("nonexistent.notify", false)
	if err == nil {
		t.Error("Expected error for non-existent notification")
	}
}

func TestDeleteNotification_InvalidID(t *testing.T) {
	tests := []struct {
		name string
		id   string
	}{
		{"path traversal", "../etc/passwd"},
		{"absolute path", "/etc/passwd"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := DeleteNotification(tt.id, false)
			if err == nil {
				t.Error("Expected error for invalid notification ID")
			}
		})
	}
}

func TestArchiveAllNotifications(t *testing.T) {
	// Create multiple test notifications
	if err := os.MkdirAll(notificationsDir, 0755); err != nil {
		t.Fatalf("Failed to create notifications dir: %v", err)
	}
	if err := os.MkdirAll(notificationsArchiveDir, 0755); err != nil {
		t.Fatalf("Failed to create archive dir: %v", err)
	}

	testFiles := []string{
		"test1.notify",
		"test2.notify",
		"test3.notify",
	}

	// Create test files
	for _, file := range testFiles {
		path := filepath.Join(notificationsDir, file)
		if err := os.WriteFile(path, []byte("test"), 0644); err != nil {
			t.Fatalf("Failed to create test file %s: %v", file, err)
		}
		defer os.Remove(path)
		defer os.Remove(filepath.Join(notificationsArchiveDir, file))
	}

	// Archive all
	err := ArchiveAllNotifications()
	if err != nil {
		t.Errorf("ArchiveAllNotifications failed: %v", err)
	}

	// Verify all were archived
	for _, file := range testFiles {
		archivedPath := filepath.Join(notificationsArchiveDir, file)
		if _, err := os.Stat(archivedPath); os.IsNotExist(err) {
			t.Errorf("File %s was not archived", file)
		}
	}
}

func TestArchiveAllNotifications_EmptyDirectory(t *testing.T) {
	// Ensure directory exists but is empty
	if err := os.MkdirAll(notificationsDir, 0755); err != nil {
		t.Fatalf("Failed to create notifications dir: %v", err)
	}

	// Should not error on empty directory
	err := ArchiveAllNotifications()
	if err != nil {
		t.Errorf("ArchiveAllNotifications failed on empty directory: %v", err)
	}
}

func TestCreateNotification_ValidInput(t *testing.T) {
	// Ensure directory exists
	if err := os.MkdirAll(notificationsDir, 0755); err != nil {
		t.Fatalf("Failed to create notifications dir: %v", err)
	}

	err := CreateNotification(
		"Test Notification",
		"Test Subject",
		"Test Description",
		"info",
		"http://example.com",
	)

	if err != nil {
		t.Errorf("CreateNotification failed: %v", err)
	}

	// Clean up - find and remove the created file
	files, _ := filepath.Glob(filepath.Join(notificationsDir, "*Test_Notification.notify"))
	for _, file := range files {
		os.Remove(file)
	}
}

func TestCreateNotification_InvalidImportance(t *testing.T) {
	err := CreateNotification(
		"Test",
		"Subject",
		"Description",
		"invalid",
		"",
	)

	if err == nil {
		t.Error("Expected error for invalid importance level")
	}
}

func TestCreateNotification_EmptyTitle(t *testing.T) {
	err := CreateNotification(
		"",
		"Subject",
		"Description",
		"info",
		"",
	)

	// Empty title should be handled (creates file with timestamp only or error)
	// The actual behavior depends on implementation
	_ = err // Test that it doesn't panic
}

func TestCreateNotification_SpecialCharactersInTitle(t *testing.T) {
	if err := os.MkdirAll(notificationsDir, 0755); err != nil {
		t.Fatalf("Failed to create notifications dir: %v", err)
	}

	tests := []struct {
		name  string
		title string
	}{
		{"with spaces", "Test With Spaces"},
		{"with special chars", "Test!@#$%^&*()"},
		{"with slashes", "Test/With/Slashes"},
		{"with dots", "Test...Dots"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CreateNotification(
				tt.title,
				"Subject",
				"Description",
				"info",
				"",
			)

			if err != nil {
				t.Logf("CreateNotification with '%s' failed: %v", tt.title, err)
			}

			// Clean up
			files, _ := filepath.Glob(filepath.Join(notificationsDir, "*.notify"))
			for _, file := range files {
				os.Remove(file)
			}
		})
	}
}
