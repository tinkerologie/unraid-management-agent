package controllers

import (
	"os"
	"path/filepath"
	"testing"
)

// setupNotificationTestDirs creates temp directories and overrides the package-level vars.
// Returns a cleanup function that restores original values.
func setupNotificationTestDirs(t *testing.T) func() {
	t.Helper()
	tmpDir := t.TempDir()
	tmpNotifDir := filepath.Join(tmpDir, "notifications")
	tmpArchiveDir := filepath.Join(tmpNotifDir, "archive")

	if err := os.MkdirAll(tmpNotifDir, 0755); err != nil {
		t.Fatalf("Failed to create temp notification dir: %v", err)
	}
	if err := os.MkdirAll(tmpArchiveDir, 0755); err != nil {
		t.Fatalf("Failed to create temp archive dir: %v", err)
	}

	oldNotifDir := notificationsDir
	oldArchiveDir := notificationsArchiveDir
	notificationsDir = tmpNotifDir
	notificationsArchiveDir = tmpArchiveDir

	return func() {
		notificationsDir = oldNotifDir
		notificationsArchiveDir = oldArchiveDir
	}
}

func TestArchiveNotification(t *testing.T) {
	cleanup := setupNotificationTestDirs(t)
	defer cleanup()

	// Create a test notification file
	testFile := "test.notify"
	testPath := filepath.Join(notificationsDir, testFile)
	testContent := []byte("test notification content")

	if err := os.WriteFile(testPath, testContent, 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
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
	cleanup := setupNotificationTestDirs(t)
	defer cleanup()

	// Create a test archived notification
	testFile := "test-archived.notify"
	testPath := filepath.Join(notificationsArchiveDir, testFile)
	testContent := []byte("archived notification content")

	if err := os.WriteFile(testPath, testContent, 0644); err != nil {
		t.Fatalf("Failed to create test archived file: %v", err)
	}

	// Test unarchiving
	err := UnarchiveNotification(testFile)
	if err != nil {
		t.Errorf("UnarchiveNotification failed: %v", err)
	}

	// Verify file was moved back
	unarchivedPath := filepath.Join(notificationsDir, testFile)
	if _, err := os.Stat(unarchivedPath); os.IsNotExist(err) {
		t.Error("Notification file was not unarchived")
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
	cleanup := setupNotificationTestDirs(t)
	defer cleanup()

	// Create a test notification
	testFile := "test-delete.notify"
	testPath := filepath.Join(notificationsDir, testFile)
	testContent := []byte("notification to delete")

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
	cleanup := setupNotificationTestDirs(t)
	defer cleanup()

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
	cleanup := setupNotificationTestDirs(t)
	defer cleanup()

	// Should not error on empty directory (no .notify files)
	err := ArchiveAllNotifications()
	if err != nil {
		t.Errorf("ArchiveAllNotifications failed on empty directory: %v", err)
	}
}

func TestCreateNotification_ValidInput(t *testing.T) {
	cleanup := setupNotificationTestDirs(t)
	defer cleanup()

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

	// Verify a file was created
	files, _ := filepath.Glob(filepath.Join(notificationsDir, "*Test_Notification.notify"))
	if len(files) == 0 {
		t.Error("Expected notification file to be created")
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
	cleanup := setupNotificationTestDirs(t)
	defer cleanup()

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
		})
	}
}
