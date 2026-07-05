package controllers

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/constants"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/services/collectors"
)

// Package-level variables for notification directories (overridable in tests)
var (
	notificationsDir        = "/boot/config/plugins/dynamix/notifications/unread"
	notificationsArchiveDir = "/boot/config/plugins/dynamix/notifications/archive"
)

func init() {
	// Resolve the actual notification paths from dynamix.cfg at startup so that
	// controller operations (create/archive/delete) target the same directories
	// the notification collector watches.
	notificationsDir, notificationsArchiveDir = collectors.ResolveNotificationDirs(constants.DynamixCfg)
}

// CreateNotification creates a new notification file in the same format as the
// stock webGui notify script so both stock consumers (the legacy PHP parser and
// unraid-api) can read it: an unquoted unix-epoch timestamp as the first line
// (the stock parser treats the first line as the timestamp), stock field order
// and value escaping, a safe_filename() style name, and an archive copy written
// first. Files are written atomically so a failed write (e.g. ENOSPC) never
// leaves a partial .notify file behind. See issue #134.
func CreateNotification(title, subject, description, importance, link string) error {
	// Validate importance
	if importance != "alert" && importance != "warning" && importance != "info" {
		return fmt.Errorf("invalid importance level: %s (must be alert, warning, or info)", importance)
	}

	timestamp := time.Now()
	event := safeFilename(title)
	if len(event) > 50 {
		event = event[:50]
	}

	// Validate sanitized title to prevent path traversal
	if err := validateFilename(event); err != nil {
		return fmt.Errorf("invalid title: %w", err)
	}

	filename := fmt.Sprintf("%s_%d.notify", event, timestamp.Unix())

	path := filepath.Join(notificationsDir, filename)

	// Verify the final path is within the notifications directory
	cleanPath := filepath.Clean(path)
	if !strings.HasPrefix(cleanPath, notificationsDir) {
		return fmt.Errorf("invalid notification path: path escapes notifications directory")
	}

	fields := fmt.Sprintf("timestamp=%d\nevent=%s\nsubject=%s\ndescription=%s\nimportance=%s\n",
		timestamp.Unix(), iniQuote(title), iniQuote(subject), iniQuote(description), iniQuote(importance))

	// The stock script always writes the archive copy (without link) first and
	// ignores its failure; only the unread copy decides the call's outcome.
	// #nosec G301 - Unraid standard permissions (0755 for directories)
	if err := os.MkdirAll(notificationsArchiveDir, 0755); err != nil {
		logger.Warning("Failed to create archive directory for notification %s: %v", filename, err)
	} else if err := writeFileAtomic(notificationsArchiveDir, filename, []byte(fields)); err != nil {
		logger.Warning("Failed to write archive copy of notification %s: %v", filename, err)
	}

	unread := fields + fmt.Sprintf("link=%s\n", iniQuote(link))
	if err := writeFileAtomic(notificationsDir, filename, []byte(unread)); err != nil {
		logger.Error("Failed to create notification: %v", err)
		return fmt.Errorf("failed to create notification: %w", err)
	}

	logger.Info("Created notification: %s", filename)
	return nil
}

// ArchiveNotification moves a notification to the archive directory
func ArchiveNotification(id string) error {
	// Validate notification ID to prevent path traversal
	if err := validateNotificationID(id); err != nil {
		return err
	}

	src := filepath.Join(notificationsDir, id)
	dst := filepath.Join(notificationsArchiveDir, id)

	// Check if source file exists
	if _, err := os.Stat(src); os.IsNotExist(err) {
		return fmt.Errorf("notification not found: %s", id)
	}

	// Ensure archive directory exists
	// #nosec G301 - Unraid standard permissions (0755 for directories)
	if err := os.MkdirAll(notificationsArchiveDir, 0755); err != nil {
		return fmt.Errorf("failed to create archive directory: %w", err)
	}

	if err := archiveOrRemove(src, dst); err != nil {
		logger.Error("Failed to archive notification %s: %v", id, err)
		return fmt.Errorf("failed to archive notification: %w", err)
	}

	logger.Info("Archived notification: %s", id)
	return nil
}

// UnarchiveNotification moves a notification from archive back to active
func UnarchiveNotification(id string) error {
	// Validate notification ID to prevent path traversal
	if err := validateNotificationID(id); err != nil {
		return err
	}

	src := filepath.Join(notificationsArchiveDir, id)
	dst := filepath.Join(notificationsDir, id)

	// Check if source file exists
	if _, err := os.Stat(src); os.IsNotExist(err) {
		return fmt.Errorf("archived notification not found: %s", id)
	}

	if err := os.Rename(src, dst); err != nil {
		logger.Error("Failed to unarchive notification %s: %v", id, err)
		return fmt.Errorf("failed to unarchive notification: %w", err)
	}

	logger.Info("Unarchived notification: %s", id)
	return nil
}

// DeleteNotification deletes a notification file
func DeleteNotification(id string, isArchived bool) error {
	// Validate notification ID to prevent path traversal
	if err := validateNotificationID(id); err != nil {
		return err
	}

	dir := notificationsDir
	if isArchived {
		dir = notificationsArchiveDir
	}
	path := filepath.Join(dir, id)

	// Check if file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("notification not found: %s", id)
	}

	if err := os.Remove(path); err != nil {
		logger.Error("Failed to delete notification %s: %v", id, err)
		return fmt.Errorf("failed to delete notification: %w", err)
	}

	logger.Info("Deleted notification: %s", id)
	return nil
}

// ArchiveAllNotifications archives all unread notifications
func ArchiveAllNotifications() error {
	files, err := os.ReadDir(notificationsDir)
	if err != nil {
		return fmt.Errorf("failed to read notifications directory: %w", err)
	}

	// Ensure archive directory exists
	// #nosec G301 - Unraid standard permissions (0755 for directories)
	if err := os.MkdirAll(notificationsArchiveDir, 0755); err != nil {
		return fmt.Errorf("failed to create archive directory: %w", err)
	}

	count := 0
	for _, file := range files {
		if !strings.HasSuffix(file.Name(), ".notify") {
			continue
		}

		src := filepath.Join(notificationsDir, file.Name())
		dst := filepath.Join(notificationsArchiveDir, file.Name())

		if err := archiveOrRemove(src, dst); err != nil {
			logger.Warning("Failed to archive %s: %v", file.Name(), err)
			continue
		}
		count++
	}

	logger.Info("Archived %d notifications", count)
	return nil
}

var (
	// notifySpecialChars removes the special characters the stock notify
	// script's safe_filename() strips from notification file names.
	notifySpecialChars = strings.NewReplacer(
		"?", "", "[", "", "]", "", "/", "", "\\", "", "=", "", "<", "", ">", "",
		":", "", ";", "", ",", "", "'", "", "\"", "", "&", "", "$", "", "#", "",
		"*", "", "(", "", ")", "", "|", "", "~", "", "`", "", "!", "", "{", "", "}", "",
	)
	// notifyDisallowedChars matches everything else the stock safe_filename()
	// removes: characters outside 0-9, a-z and the ASCII range 0x20-0x5F
	// (written " -_" in the stock script; it already contains A-Z, which is
	// why stock's /i flag is redundant). Deliberately no (?i): Go would apply
	// Unicode case folding to the class, keeping U+212A/U+017F, which the
	// stock byte-oriented pattern strips like any other non-ASCII input.
	notifyDisallowedChars = regexp.MustCompile(`[^0-9a-z -_]`)
	// notifyDashSpace converts hyphens and spaces to underscores like the
	// stock safe_filename().
	notifyDashSpace = strings.NewReplacer("-", "_", " ", "_")
	// iniEscaper escapes backslashes and double quotes the same way the stock
	// notify script's ini_encode_value() does (PHP strtr, single pass).
	iniEscaper = strings.NewReplacer(`\`, `\\`, `"`, `\"`)
)

// safeFilename replicates the stock webGui notify script's safe_filename()
// so agent notifications get the same file names the stock script produces.
func safeFilename(s string) string {
	s = strings.TrimSpace(notifySpecialChars.Replace(s))
	s = notifyDisallowedChars.ReplaceAllString(s, "")
	s = notifyDashSpace.Replace(s)
	return strings.TrimSpace(s)
}

// iniQuote wraps a string value in double quotes with stock-compatible escaping.
func iniQuote(v string) string {
	return `"` + iniEscaper.Replace(v) + `"`
}

// writeFileAtomic writes content to dir/name via a temporary file and rename,
// so a failed write (e.g. ENOSPC) never leaves a partial or empty .notify
// file for the Unraid UI to flag as invalid. The temporary name does not end
// in .notify, keeping it invisible to notification consumers.
func writeFileAtomic(dir, name string, content []byte) error {
	tmp, err := os.CreateTemp(dir, ".notify-tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	_, err = tmp.Write(content)
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		// #nosec G302 - Notification files need to be readable by Unraid web UI (0644)
		err = os.Chmod(tmpName, 0644)
	}
	if err == nil {
		err = os.Rename(tmpName, filepath.Join(dir, name))
	}
	if err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return nil
}

// archiveOrRemove moves the unread file src into the archive as dst. Since the
// issue #134 fix an archive copy (without link) is written at creation, so
// archiving normally only needs to remove the unread file, like the stock
// notify script's 'archive' verb; renaming over the copy would clobber it.
// Files with no creation copy (legacy format) fall back to the rename. When
// the check itself fails, the error is returned without touching either file:
// renaming could clobber an existing copy and removing could delete the only
// copy, and neither risk is worth taking blind.
func archiveOrRemove(src, dst string) error {
	if _, err := os.Stat(dst); err == nil {
		return os.Remove(src)
	} else if !os.IsNotExist(err) {
		return err
	}
	return os.Rename(src, dst)
}

// validateFilename validates a filename to prevent path traversal attacks
// This is used after safeFilename to ensure the sanitized result is safe
func validateFilename(filename string) error {
	if filename == "" {
		return fmt.Errorf("filename cannot be empty")
	}

	// Check for parent directory references
	if strings.Contains(filename, "..") {
		return fmt.Errorf("parent directory references not allowed")
	}

	// Check for absolute paths
	if strings.HasPrefix(filename, "/") || strings.HasPrefix(filename, "\\") {
		return fmt.Errorf("absolute paths not allowed")
	}

	// Check for path separators
	if strings.Contains(filename, "/") || strings.Contains(filename, "\\") {
		return fmt.Errorf("path separators not allowed")
	}

	return nil
}

// validateNotificationID validates a notification ID to prevent path traversal attacks
// Notification IDs should be filenames only (no path separators or parent directory references)
func validateNotificationID(id string) error {
	if id == "" {
		return fmt.Errorf("notification ID cannot be empty")
	}

	// Check for parent directory references first (most specific attack)
	if strings.Contains(id, "..") {
		return fmt.Errorf("invalid notification ID: parent directory references not allowed")
	}

	// Check for absolute paths
	if strings.HasPrefix(id, "/") || strings.HasPrefix(id, "\\") {
		return fmt.Errorf("invalid notification ID: absolute paths not allowed")
	}

	// Check for path separators (both Unix and Windows)
	if strings.Contains(id, "/") || strings.Contains(id, "\\") {
		return fmt.Errorf("invalid notification ID: path separators not allowed")
	}

	// Validate file extension (must be .notify)
	if !strings.HasSuffix(id, ".notify") {
		return fmt.Errorf("invalid notification ID: must have .notify extension")
	}

	// Additional security: ensure the resolved path stays within the notifications directory
	// This prevents symlink attacks and other edge cases
	cleanPath := filepath.Clean(filepath.Join(notificationsDir, id))
	if !strings.HasPrefix(cleanPath, notificationsDir) {
		return fmt.Errorf("invalid notification ID: path escapes notifications directory")
	}

	return nil
}
