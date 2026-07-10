package controllers

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
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
	files, _ := filepath.Glob(filepath.Join(notificationsDir, "Test_Notification_*.notify"))
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
		name    string
		title   string
		wantErr bool
	}{
		{"with spaces", "Test With Spaces", false},
		{"with special chars", "Test!@#$%^&*()", false},
		{"with slashes", "Test/With/Slashes", false},
		// Dots survive the stock safe_filename() replica, so a title whose
		// sanitized form contains ".." is rejected by the pre-existing
		// path-traversal guard (stricter than stock, which would allow it).
		{"with consecutive dots", "Test...Dots", true},
		// safeFilename trims whitespace before converting spaces, like stock,
		// so whitespace-only titles sanitize to empty and are rejected.
		{"whitespace only", "   ", true},
		// Characters stock's safe_filename() keeps (@ % ^ + .) survive the
		// replica too, so punctuation-only titles are accepted like stock
		// (the old sanitizer stripped them all and rejected the empty result).
		{"punctuation only", "@%^+.", false},
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

			if tt.wantErr {
				if err == nil {
					t.Errorf("CreateNotification with %q should be rejected by the filename guard", tt.title)
				}
			} else if err != nil {
				t.Errorf("CreateNotification with %q failed: %v", tt.title, err)
			}
		})
	}
}

// TestCreateNotification_StockNotifyFormat verifies notification files match the
// format written by Unraid's stock webGui notify script so the legacy PHP parser
// and unraid-api can read them: an unquoted unix-epoch timestamp as the first
// line, stock field order, escaped quotes/backslashes, a trailing newline, and
// a safe_filename() style file name (issue #134).
func TestCreateNotification_StockNotifyFormat(t *testing.T) {
	cleanup := setupNotificationTestDirs(t)
	defer cleanup()

	before := time.Now().Unix()
	err := CreateNotification(
		"Management Agent Alert",
		"Resolved: Agent data source degraded",
		`Rule "cpu" resolved on \srv01`,
		"warning",
		"",
	)
	after := time.Now().Unix()

	if err != nil {
		t.Fatalf("CreateNotification failed: %v", err)
	}

	files, err := filepath.Glob(filepath.Join(notificationsDir, "*.notify"))
	if err != nil {
		t.Fatalf("Glob failed: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("Expected exactly one .notify file, got %d", len(files))
	}

	name := filepath.Base(files[0])
	if !regexp.MustCompile(`^Management_Agent_Alert_\d+\.notify$`).MatchString(name) {
		t.Errorf("Filename %q does not match the stock safe_filename() scheme", name)
	}

	content, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatalf("Failed to read notification file: %v", err)
	}

	info, err := os.Stat(files[0])
	if err != nil {
		t.Fatalf("Failed to stat notification file: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o644 {
		t.Errorf("Notification file mode = %04o, want 0644 (readable by the Unraid web UI)", perm)
	}

	firstLine := strings.SplitN(string(content), "\n", 2)[0]
	match := regexp.MustCompile(`^timestamp=(\d+)$`).FindStringSubmatch(firstLine)
	if match == nil {
		t.Fatalf("First line %q is not an unquoted unix-epoch timestamp", firstLine)
	}
	epoch, err := strconv.ParseInt(match[1], 10, 64)
	if err != nil {
		t.Fatalf("Failed to parse timestamp %q: %v", match[1], err)
	}
	if epoch < before || epoch > after {
		t.Errorf("Timestamp %d outside test window [%d, %d]", epoch, before, after)
	}

	want := fmt.Sprintf("timestamp=%d\n"+
		"event=\"Management Agent Alert\"\n"+
		"subject=\"Resolved: Agent data source degraded\"\n"+
		"description=\"Rule \\\"cpu\\\" resolved on \\\\srv01\"\n"+
		"importance=\"warning\"\n"+
		"link=\"\"\n", epoch)
	if string(content) != want {
		t.Errorf("Content mismatch:\ngot:\n%q\nwant:\n%q", string(content), want)
	}
}

// TestCreateNotification_WritesArchiveCopy verifies a stock-style archive copy
// (same fields, no link line) is written alongside the unread file, matching
// the stock notify script behavior (issue #134).
func TestCreateNotification_WritesArchiveCopy(t *testing.T) {
	cleanup := setupNotificationTestDirs(t)
	defer cleanup()

	err := CreateNotification("Test Event", "Subject", "Description", "info", "http://example.com/x")
	if err != nil {
		t.Fatalf("CreateNotification failed: %v", err)
	}

	unreadFiles, err := filepath.Glob(filepath.Join(notificationsDir, "*.notify"))
	if err != nil {
		t.Fatalf("Glob failed: %v", err)
	}
	if len(unreadFiles) != 1 {
		t.Fatalf("Expected one unread file, got %d", len(unreadFiles))
	}
	name := filepath.Base(unreadFiles[0])

	archiveContent, err := os.ReadFile(filepath.Join(notificationsArchiveDir, name))
	if err != nil {
		t.Fatalf("Expected archive copy of %s: %v", name, err)
	}

	unreadContent, err := os.ReadFile(unreadFiles[0])
	if err != nil {
		t.Fatalf("Failed to read unread file: %v", err)
	}
	wantArchive := strings.Replace(string(unreadContent), "link=\"http://example.com/x\"\n", "", 1)
	if string(archiveContent) != wantArchive {
		t.Errorf("Archive copy mismatch:\ngot:\n%q\nwant:\n%q", string(archiveContent), wantArchive)
	}
}

// TestCreateNotification_LongTitleCapped verifies the event part of the file
// name keeps the pre-existing 50 character cap.
func TestCreateNotification_LongTitleCapped(t *testing.T) {
	cleanup := setupNotificationTestDirs(t)
	defer cleanup()

	longTitle := strings.Repeat("a", 80)
	if err := CreateNotification(longTitle, "Subject", "Description", "info", ""); err != nil {
		t.Fatalf("CreateNotification failed: %v", err)
	}

	files, err := filepath.Glob(filepath.Join(notificationsDir, "*.notify"))
	if err != nil || len(files) != 1 {
		t.Fatalf("Expected exactly one .notify file, got %d (err=%v)", len(files), err)
	}

	event := strings.SplitN(filepath.Base(files[0]), "_", 2)[0]
	if len(event) != 50 {
		t.Errorf("Expected event part capped at 50 characters, got %d (%q)", len(event), event)
	}
}

// TestCreateNotification_NoFileLeftOnWriteFailure verifies a failed dispatch
// leaves no partial, empty, or temporary file behind (issue #134).
func TestCreateNotification_NoFileLeftOnWriteFailure(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("directory permissions are not enforced for root")
	}

	cleanup := setupNotificationTestDirs(t)
	defer cleanup()

	for _, dir := range []string{notificationsDir, notificationsArchiveDir} {
		if err := os.Chmod(dir, 0555); err != nil {
			t.Fatalf("Failed to make %s read-only: %v", dir, err)
		}
		defer os.Chmod(dir, 0755) //nolint:errcheck
	}

	if err := CreateNotification("Test Event", "Subject", "Description", "info", ""); err == nil {
		t.Fatal("Expected an error when the notifications directory is not writable")
	}

	entries, err := os.ReadDir(notificationsDir)
	if err != nil {
		t.Fatalf("Failed to read notifications dir: %v", err)
	}
	for _, e := range entries {
		if e.IsDir() && e.Name() == filepath.Base(notificationsArchiveDir) {
			continue // archive dir is nested inside the unread dir in tests
		}
		t.Errorf("Unexpected leftover entry after failed write: %s", e.Name())
	}
	archiveEntries, err := os.ReadDir(notificationsArchiveDir)
	if err != nil {
		t.Fatalf("Failed to read archive dir: %v", err)
	}
	for _, e := range archiveEntries {
		t.Errorf("Unexpected leftover entry in archive after failed write: %s", e.Name())
	}
}

// TestCreateNotification_ArchiveFailureDoesNotAbort verifies that a failed
// archive copy is tolerated like the stock notify script tolerates it: the
// unread notification is still delivered and no error is returned (issue #134).
func TestCreateNotification_ArchiveFailureDoesNotAbort(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("directory permissions are not enforced for root")
	}

	cleanup := setupNotificationTestDirs(t)
	defer cleanup()

	if err := os.Chmod(notificationsArchiveDir, 0555); err != nil {
		t.Fatalf("Failed to make archive dir read-only: %v", err)
	}
	defer os.Chmod(notificationsArchiveDir, 0755) //nolint:errcheck

	if err := CreateNotification("Test Event", "Subject", "Description", "info", ""); err != nil {
		t.Fatalf("CreateNotification should tolerate an archive write failure, got: %v", err)
	}

	unreadFiles, err := filepath.Glob(filepath.Join(notificationsDir, "*.notify"))
	if err != nil || len(unreadFiles) != 1 {
		t.Fatalf("Expected exactly one unread .notify file, got %d (err=%v)", len(unreadFiles), err)
	}

	archiveEntries, err := os.ReadDir(notificationsArchiveDir)
	if err != nil {
		t.Fatalf("Failed to read archive dir: %v", err)
	}
	for _, e := range archiveEntries {
		t.Errorf("Unexpected entry in read-only archive dir: %s", e.Name())
	}
}

// TestCreateNotification_UnreadFailureKeepsArchiveCopy pins the stock write
// order: the archive copy is written first, so when only the unread write
// fails the archive record is retained, exactly like the stock notify script
// (issue #134).
func TestCreateNotification_UnreadFailureKeepsArchiveCopy(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("directory permissions are not enforced for root")
	}

	cleanup := setupNotificationTestDirs(t)
	defer cleanup()

	if err := os.Chmod(notificationsDir, 0555); err != nil {
		t.Fatalf("Failed to make notifications dir read-only: %v", err)
	}
	defer os.Chmod(notificationsDir, 0755) //nolint:errcheck

	if err := CreateNotification("Test Event", "Subject", "Description", "info", ""); err == nil {
		t.Fatal("Expected an error when the unread directory is not writable")
	}

	archiveFiles, err := filepath.Glob(filepath.Join(notificationsArchiveDir, "Test_Event_*.notify"))
	if err != nil || len(archiveFiles) != 1 {
		t.Fatalf("Expected the archive copy to be retained after unread write failure, got %d (err=%v)", len(archiveFiles), err)
	}
}

// TestArchiveNotification_KeepsCreationArchiveCopy verifies that archiving via
// the agent preserves the archive copy written at creation (which has no link
// line), like the stock notify script's 'archive' verb, which only deletes the
// unread file. Renaming the unread file over it would clobber that copy
// (issue #134).
func TestArchiveNotification_KeepsCreationArchiveCopy(t *testing.T) {
	cleanup := setupNotificationTestDirs(t)
	defer cleanup()

	if err := CreateNotification("Test Event", "Subject", "Description", "info", "http://example.com/x"); err != nil {
		t.Fatalf("CreateNotification failed: %v", err)
	}
	unreadFiles, err := filepath.Glob(filepath.Join(notificationsDir, "*.notify"))
	if err != nil || len(unreadFiles) != 1 {
		t.Fatalf("Expected one unread file, got %d (err=%v)", len(unreadFiles), err)
	}
	name := filepath.Base(unreadFiles[0])

	if err := ArchiveNotification(name); err != nil {
		t.Fatalf("ArchiveNotification failed: %v", err)
	}

	if _, err := os.Stat(unreadFiles[0]); !os.IsNotExist(err) {
		t.Errorf("Expected unread file to be removed, stat err: %v", err)
	}
	content, err := os.ReadFile(filepath.Join(notificationsArchiveDir, name))
	if err != nil {
		t.Fatalf("Expected the creation archive copy to remain: %v", err)
	}
	if strings.Contains(string(content), "link=") {
		t.Errorf("Archive copy was clobbered by the unread file (contains a link line):\n%s", content)
	}
}

// TestArchiveAllNotifications_KeepsCreationArchiveCopies verifies the bulk
// archive operation preserves creation archive copies the same way (issue #134).
func TestArchiveAllNotifications_KeepsCreationArchiveCopies(t *testing.T) {
	cleanup := setupNotificationTestDirs(t)
	defer cleanup()

	if err := CreateNotification("Bulk Event", "Subject", "Description", "info", "http://example.com/y"); err != nil {
		t.Fatalf("CreateNotification failed: %v", err)
	}
	unreadFiles, err := filepath.Glob(filepath.Join(notificationsDir, "*.notify"))
	if err != nil || len(unreadFiles) != 1 {
		t.Fatalf("Expected one unread file, got %d (err=%v)", len(unreadFiles), err)
	}
	name := filepath.Base(unreadFiles[0])

	if err := ArchiveAllNotifications(); err != nil {
		t.Fatalf("ArchiveAllNotifications failed: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(notificationsArchiveDir, name))
	if err != nil {
		t.Fatalf("Expected the creation archive copy to remain: %v", err)
	}
	if strings.Contains(string(content), "link=") {
		t.Errorf("Archive copy was clobbered by the unread file (contains a link line):\n%s", content)
	}
}

// TestArchiveOrRemove_AmbiguousCheckTouchesNothing verifies that when the
// archive destination cannot be checked at all (stat fails with something
// other than not-exists), the helper returns that error without attempting a
// rename (which could clobber an existing archive copy) or a remove (which
// could delete the only copy). The returned error's Op pins that the helper
// stopped at the failed check rather than attempting a mutation.
func TestArchiveOrRemove_AmbiguousCheckTouchesNothing(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("directory permissions are not enforced for root")
	}

	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "unread")
	dstDir := filepath.Join(tmpDir, "archive")
	for _, d := range []string{srcDir, dstDir} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatalf("Failed to create %s: %v", d, err)
		}
	}
	src := filepath.Join(srcDir, "test.notify")
	if err := os.WriteFile(src, []byte("data"), 0644); err != nil {
		t.Fatalf("Failed to write source file: %v", err)
	}
	if err := os.Chmod(dstDir, 0000); err != nil {
		t.Fatalf("Failed to make archive dir unreadable: %v", err)
	}
	defer os.Chmod(dstDir, 0755) //nolint:errcheck

	err := archiveOrRemove(src, filepath.Join(dstDir, "test.notify"))
	if err == nil {
		t.Fatal("Expected an error when the destination cannot be checked")
	}
	var pathErr *fs.PathError
	if !errors.As(err, &pathErr) || pathErr.Op != "stat" {
		t.Errorf("Expected the stat error itself (no rename/remove attempted), got: %v", err)
	}

	if _, statErr := os.Stat(src); statErr != nil {
		t.Errorf("Source file should be untouched after an ambiguous check: %v", statErr)
	}
	if err := os.Chmod(dstDir, 0755); err != nil {
		t.Fatalf("Failed to restore archive dir permissions: %v", err)
	}
	entries, err := os.ReadDir(dstDir)
	if err != nil {
		t.Fatalf("Failed to read archive dir: %v", err)
	}
	for _, e := range entries {
		t.Errorf("Archive dir should be untouched, found: %s", e.Name())
	}
}

// TestWriteFileAtomic_CleansUpTempOnFailure verifies the atomic writer removes
// its temporary file when the write cannot complete, so a failure mid-dispatch
// (e.g. ENOSPC) never leaves anything behind in the directory (issue #134).
func TestWriteFileAtomic_CleansUpTempOnFailure(t *testing.T) {
	tmpDir := t.TempDir()

	// Renaming onto a path inside a nonexistent subdirectory must fail after
	// the temporary file has already been created and written.
	err := writeFileAtomic(tmpDir, filepath.Join("missing-subdir", "test.notify"), []byte("data"))
	if err == nil {
		t.Fatal("Expected an error when the rename target cannot be created")
	}

	entries, readErr := os.ReadDir(tmpDir)
	if readErr != nil {
		t.Fatalf("Failed to read temp dir: %v", readErr)
	}
	for _, e := range entries {
		t.Errorf("Unexpected leftover entry after failed atomic write: %s", e.Name())
	}
}
