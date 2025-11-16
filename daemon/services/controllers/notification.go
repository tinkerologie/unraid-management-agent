package controllers

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
)

const (
	notificationsDir        = "/usr/local/emhttp/state/notifications"
	notificationsArchiveDir = "/usr/local/emhttp/state/notifications/archive"
)

// CreateNotification creates a new notification file
func CreateNotification(title, subject, description, importance, link string) error {
	// Validate importance
	if importance != "alert" && importance != "warning" && importance != "info" {
		return fmt.Errorf("invalid importance level: %s (must be alert, warning, or info)", importance)
	}

	timestamp := time.Now()
	filename := fmt.Sprintf("%s-%s.notify",
		timestamp.Format("20060102-150405"),
		sanitizeFilename(title))

	path := filepath.Join(notificationsDir, filename)

	content := fmt.Sprintf(`event="%s"
subject="%s"
description="%s"
importance="%s"
timestamp="%s"
link="%s"`,
		title, subject, description, importance,
		timestamp.Format("2006-01-02 15:04:05"), link)

	// #nosec G306 - Notification files need to be readable by Unraid web UI (0644)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		logger.Error("Failed to create notification: %v", err)
		return fmt.Errorf("failed to create notification: %w", err)
	}

	logger.Info("Created notification: %s", filename)
	return nil
}

// ArchiveNotification moves a notification to the archive directory
func ArchiveNotification(id string) error {
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

	if err := os.Rename(src, dst); err != nil {
		logger.Error("Failed to archive notification %s: %v", id, err)
		return fmt.Errorf("failed to archive notification: %w", err)
	}

	logger.Info("Archived notification: %s", id)
	return nil
}

// UnarchiveNotification moves a notification from archive back to active
func UnarchiveNotification(id string) error {
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

		if err := os.Rename(src, dst); err != nil {
			logger.Warning("Failed to archive %s: %v", file.Name(), err)
			continue
		}
		count++
	}

	logger.Info("Archived %d notifications", count)
	return nil
}

// sanitizeFilename removes invalid characters from filename
func sanitizeFilename(s string) string {
	// Replace spaces with underscores
	s = strings.ReplaceAll(s, " ", "_")
	// Remove any character that's not alphanumeric, underscore, or hyphen
	reg := regexp.MustCompile(`[^a-zA-Z0-9_-]`)
	s = reg.ReplaceAllString(s, "")
	// Limit length
	if len(s) > 50 {
		s = s[:50]
	}
	return s
}
