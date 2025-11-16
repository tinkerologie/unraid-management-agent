package collectors

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
)

const (
	notificationsDir        = "/usr/local/emhttp/state/notifications"
	notificationsArchiveDir = "/usr/local/emhttp/state/notifications/archive"
)

// NotificationCollector collects Unraid notifications
type NotificationCollector struct {
	ctx     *domain.Context
	watcher *fsnotify.Watcher
}

// NewNotificationCollector creates a new notification collector
func NewNotificationCollector(ctx *domain.Context) *NotificationCollector {
	return &NotificationCollector{ctx: ctx}
}

// Start begins collecting notification data
func (c *NotificationCollector) Start(ctx context.Context, interval time.Duration) {
	defer func() {
		if r := recover(); r != nil {
			logger.Error("Notification collector panic: %v", r)
		}
	}()

	// Initialize file watcher
	var err error
	c.watcher, err = fsnotify.NewWatcher()
	if err != nil {
		logger.Error("Failed to create file watcher: %v", err)
		return
	}
	defer func() {
		if err := c.watcher.Close(); err != nil {
			logger.Error("Failed to close file watcher: %v", err)
		}
	}()

	// Ensure directories exist
	if err := os.MkdirAll(notificationsDir, 0755); err != nil { // #nosec G301 - Unraid standard permissions
		logger.Warning("Failed to create notifications directory: %v", err)
	}
	if err := os.MkdirAll(notificationsArchiveDir, 0755); err != nil { // #nosec G301 - Unraid standard permissions
		logger.Warning("Failed to create notifications archive directory: %v", err)
	}

	// Watch notification directories
	if err := c.watcher.Add(notificationsDir); err != nil {
		logger.Warning("Failed to watch notifications directory: %v", err)
	}
	if err := c.watcher.Add(notificationsArchiveDir); err != nil {
		logger.Warning("Failed to watch notifications archive directory: %v", err)
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Initial collection
	c.collect()

	for {
		select {
		case <-ctx.Done():
			logger.Info("Notification collector stopped")
			return
		case <-ticker.C:
			c.collect()
		case event := <-c.watcher.Events:
			// Trigger immediate collection on file changes
			if event.Op&fsnotify.Create == fsnotify.Create ||
				event.Op&fsnotify.Remove == fsnotify.Remove ||
				event.Op&fsnotify.Write == fsnotify.Write {
				logger.Debug("Notification file change detected: %s", event.Name)
				c.collect()
			}
		case err := <-c.watcher.Errors:
			logger.Error("File watcher error: %v", err)
		}
	}
}

// collect gathers all notifications and publishes to event bus
func (c *NotificationCollector) collect() {
	unread := c.collectNotifications(notificationsDir, "unread")
	archived := c.collectNotifications(notificationsArchiveDir, "archive")

	overview := c.calculateOverview(unread, archived)

	notificationList := &dto.NotificationList{
		Overview:      overview,
		Notifications: append(unread, archived...),
		Timestamp:     time.Now(),
	}

	c.ctx.Hub.Pub(notificationList, "notifications_update")
}

// collectNotifications reads all notification files from a directory
func (c *NotificationCollector) collectNotifications(dir string, notifType string) []dto.Notification {
	files, err := os.ReadDir(dir)
	if err != nil {
		logger.Debug("Failed to read notifications directory %s: %v", dir, err)
		return []dto.Notification{}
	}

	var notifications []dto.Notification
	for _, file := range files {
		if !strings.HasSuffix(file.Name(), ".notify") {
			continue
		}

		notification := c.parseNotificationFile(filepath.Join(dir, file.Name()), notifType)
		if notification != nil {
			notifications = append(notifications, *notification)
		}
	}

	// Sort by timestamp descending (newest first)
	sort.Slice(notifications, func(i, j int) bool {
		return notifications[i].Timestamp.After(notifications[j].Timestamp)
	})

	return notifications
}

// parseNotificationFile parses a notification file and returns a Notification
func (c *NotificationCollector) parseNotificationFile(path string, notifType string) *dto.Notification {
	content, err := os.ReadFile(path) // #nosec G304 - Path is from controlled directory scan
	if err != nil {
		logger.Debug("Failed to read notification file %s: %v", path, err)
		return nil
	}

	notification := &dto.Notification{
		ID:   filepath.Base(path),
		Type: notifType,
	}

	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.Trim(strings.TrimSpace(parts[1]), `"`)

		switch key {
		case "event":
			notification.Title = value
		case "subject":
			notification.Subject = value
		case "description":
			notification.Description = value
		case "importance":
			notification.Importance = value
		case "timestamp":
			if ts, err := time.Parse("2006-01-02 15:04:05", value); err == nil {
				notification.Timestamp = ts
				notification.FormattedTimestamp = ts.Format(time.RFC3339)
			}
		case "link":
			notification.Link = value
		}
	}

	// If timestamp wasn't parsed, use file modification time
	if notification.Timestamp.IsZero() {
		if info, err := os.Stat(path); err == nil {
			notification.Timestamp = info.ModTime()
			notification.FormattedTimestamp = info.ModTime().Format(time.RFC3339)
		}
	}

	return notification
}

// calculateOverview calculates notification counts by type and importance
func (c *NotificationCollector) calculateOverview(unread, archived []dto.Notification) dto.NotificationOverview {
	return dto.NotificationOverview{
		Unread:  c.countByImportance(unread),
		Archive: c.countByImportance(archived),
	}
}

// countByImportance counts notifications by importance level
func (c *NotificationCollector) countByImportance(notifications []dto.Notification) dto.NotificationCounts {
	counts := dto.NotificationCounts{}
	for _, n := range notifications {
		switch n.Importance {
		case "alert":
			counts.Alert++
		case "warning":
			counts.Warning++
		case "info":
			counts.Info++
		}
	}
	counts.Total = counts.Alert + counts.Warning + counts.Info
	return counts
}
