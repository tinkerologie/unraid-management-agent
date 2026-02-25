package collectors

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

// setupNotificationCollectorTestDirs overrides the package-level notification directory
// variables with temp dirs and returns a cleanup function.
func setupNotificationCollectorTestDirs(t *testing.T) (string, string, func()) {
	t.Helper()
	tmpDir := t.TempDir()
	tmpUnread := filepath.Join(tmpDir, "notifications")
	tmpArchive := filepath.Join(tmpUnread, "archive")

	if err := os.MkdirAll(tmpUnread, 0755); err != nil {
		t.Fatalf("Failed to create temp notification dir: %v", err)
	}
	if err := os.MkdirAll(tmpArchive, 0755); err != nil {
		t.Fatalf("Failed to create temp archive dir: %v", err)
	}

	oldNotifDir := notificationsDir
	oldArchiveDir := notificationsArchiveDir
	notificationsDir = tmpUnread
	notificationsArchiveDir = tmpArchive

	return tmpUnread, tmpArchive, func() {
		notificationsDir = oldNotifDir
		notificationsArchiveDir = oldArchiveDir
	}
}

func TestNewNotificationCollector(t *testing.T) {
	hub := domain.NewEventBus(10)
	ctx := &domain.Context{Hub: hub}

	collector := NewNotificationCollector(ctx)

	if collector == nil {
		t.Fatal("NewNotificationCollector() returned nil")
	}

	if collector.ctx != ctx {
		t.Error("NotificationCollector context not set correctly")
	}
}

func TestNotificationCollectorInit(t *testing.T) {
	hub := domain.NewEventBus(10)
	ctx := &domain.Context{Hub: hub}

	collector := NewNotificationCollector(ctx)

	if collector == nil {
		t.Fatal("Collector should not be nil")
	}

	if collector.ctx == nil {
		t.Fatal("Collector context should not be nil")
	}

	if collector.ctx.Hub == nil {
		t.Fatal("Collector context Hub should not be nil")
	}

	// Watcher should be nil until Start is called
	if collector.watcher != nil {
		t.Error("Watcher should be nil before Start() is called")
	}
}

func TestNotificationCollectorStart(t *testing.T) {
	tmpDir := t.TempDir()
	tmpUnread := filepath.Join(tmpDir, "unread")
	tmpArchive := filepath.Join(tmpDir, "archive")

	if err := os.MkdirAll(tmpUnread, 0755); err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	if err := os.MkdirAll(tmpArchive, 0755); err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}

	// Create a mock notification file
	notificationContent := "title=Test\nsubject=Test Subject\ndescription=Test Notification\nimportance=normal\n"
	notifFile := filepath.Join(tmpUnread, "test.notification")
	if err := os.WriteFile(notifFile, []byte(notificationContent), 0644); err != nil {
		t.Fatalf("Failed to create test notification file: %v", err)
	}

	hub := domain.NewEventBus(10)
	ctx := &domain.Context{Hub: hub}
	collector := NewNotificationCollector(ctx)

	// Subscribe to notifications_update topic
	ch := hub.Sub("notifications_update")
	defer hub.Unsub(ch, "notifications_update")

	// Create a context that cancels after a short delay
	testCtx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	// Run collector in goroutine (it will return when context is cancelled)
	go collector.Start(testCtx, 10*time.Millisecond)

	// Wait for an event or timeout
	select {
	case msg := <-ch:
		if notifList, ok := msg.(*dto.NotificationList); ok {
			if notifList == nil {
				t.Error("Expected NotificationList, got nil")
			}
		} else {
			t.Errorf("Expected *dto.NotificationList, got %T", msg)
		}
	case <-testCtx.Done():
		// Context expired - this is expected behavior when testing Start()
	}
}

func TestNotificationCollectorCollect(t *testing.T) {
	tmpUnread, _, cleanup := setupNotificationCollectorTestDirs(t)
	defer cleanup()

	// Create multiple notification files
	for i := 1; i <= 2; i++ {
		content := "title=Test\nsubject=Subject\nimportance=info\n"
		file := filepath.Join(tmpUnread, "test"+string(rune('0'+i))+".notify")
		if err := os.WriteFile(file, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to create notification file: %v", err)
		}
	}

	hub := domain.NewEventBus(10)
	ctx := &domain.Context{Hub: hub}
	collector := NewNotificationCollector(ctx)

	// Subscribe to get published events
	ch := hub.Sub("notifications_update")
	defer hub.Unsub(ch, "notifications_update")

	// Call collect directly
	collector.collect()

	// Check if we received a notification
	select {
	case msg := <-ch:
		notifList, ok := msg.(*dto.NotificationList)
		if !ok {
			t.Errorf("Expected *dto.NotificationList, got %T", msg)
			return
		}
		if notifList == nil {
			t.Fatal("NotificationList is nil")
		}
		if len(notifList.Notifications) == 0 {
			t.Error("Expected notifications in list")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Did not receive notification event within timeout")
	}
}

func TestNotificationCollectorEmptyDirectories(t *testing.T) {
	_, _, cleanup := setupNotificationCollectorTestDirs(t)
	defer cleanup()

	hub := domain.NewEventBus(10)
	ctx := &domain.Context{Hub: hub}
	collector := NewNotificationCollector(ctx)

	ch := hub.Sub("notifications_update")
	defer hub.Unsub(ch, "notifications_update")

	// Collect from empty directories
	collector.collect()

	select {
	case msg := <-ch:
		notifList, ok := msg.(*dto.NotificationList)
		if !ok {
			t.Errorf("Expected *dto.NotificationList, got %T", msg)
			return
		}
		if notifList == nil {
			t.Fatal("NotificationList is nil")
		}
		if len(notifList.Notifications) != 0 {
			t.Error("Expected zero notifications for empty directories")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Did not receive notification event within timeout")
	}
}

func TestParseNotificationFile(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name      string
		content   string
		checkFunc func(*dto.Notification) bool
	}{
		{
			name:    "valid notification",
			content: "event=Test\nsubject=Test Subject\ndescription=Test Desc\nimportance=info\n",
			checkFunc: func(n *dto.Notification) bool {
				return n != nil && n.Title != "" && n.Subject != ""
			},
		},
		{
			name:    "minimal valid notification",
			content: "event=Array\nsubject=Min\n",
			checkFunc: func(n *dto.Notification) bool {
				return n != nil && n.Title == "Array"
			},
		},
		{
			name:    "empty file",
			content: "",
			checkFunc: func(n *dto.Notification) bool {
				return n != nil
			},
		},
	}

	hub := domain.NewEventBus(10)
	ctx := &domain.Context{Hub: hub}
	collector := NewNotificationCollector(ctx)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filePath := filepath.Join(tmpDir, "test.notification")
			if err := os.WriteFile(filePath, []byte(tt.content), 0644); err != nil {
				t.Fatalf("Failed to write test file: %v", err)
			}

			notif := collector.parseNotificationFile(filePath, "unread")

			if !tt.checkFunc(notif) {
				t.Errorf("Notification check failed: %+v", notif)
			}

			os.Remove(filePath)
		})
	}
}

func TestCalculateOverview(t *testing.T) {
	hub := domain.NewEventBus(10)
	ctx := &domain.Context{Hub: hub}
	collector := NewNotificationCollector(ctx)

	tests := []struct {
		name     string
		unread   []dto.Notification
		archived []dto.Notification
		check    func(*dto.NotificationOverview) bool
	}{
		{
			name:     "empty notifications",
			unread:   []dto.Notification{},
			archived: []dto.Notification{},
			check: func(o *dto.NotificationOverview) bool {
				return o.Unread.Total == 0 && o.Archive.Total == 0
			},
		},
		{
			name: "with unread",
			unread: []dto.Notification{
				{Title: "System", Importance: "info"},
				{Title: "Array", Importance: "warning"},
			},
			archived: []dto.Notification{},
			check: func(o *dto.NotificationOverview) bool {
				return o.Unread.Total == 2 && o.Archive.Total == 0
			},
		},
		{
			name:   "with archived",
			unread: []dto.Notification{},
			archived: []dto.Notification{
				{Title: "Disk", Importance: "alert"},
			},
			check: func(o *dto.NotificationOverview) bool {
				return o.Archive.Total == 1 && o.Unread.Total == 0
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			overview := collector.calculateOverview(tt.unread, tt.archived)
			if !tt.check(&overview) {
				t.Errorf("Overview check failed: %+v", overview)
			}
		})
	}
}

func TestCountByImportance(t *testing.T) {
	hub := domain.NewEventBus(10)
	ctx := &domain.Context{Hub: hub}
	collector := NewNotificationCollector(ctx)

	notifications := []dto.Notification{
		{Title: "System", Importance: "info"},
		{Title: "Array", Importance: "warning"},
		{Title: "Disk", Importance: "alert"},
		{Title: "Share", Importance: "info"},
	}

	counts := collector.countByImportance(notifications)

	expectedInfo := 2 // normal = info
	expectedWarning := 1
	expectedAlert := 1

	if counts.Info != expectedInfo {
		t.Errorf("Expected %d info, got %d", expectedInfo, counts.Info)
	}
	if counts.Warning != expectedWarning {
		t.Errorf("Expected %d warning, got %d", expectedWarning, counts.Warning)
	}
	if counts.Alert != expectedAlert {
		t.Errorf("Expected %d alert, got %d", expectedAlert, counts.Alert)
	}
}

func TestNotificationCollectorPanic(t *testing.T) {
	hub := domain.NewEventBus(10)
	ctx := &domain.Context{Hub: hub}
	collector := NewNotificationCollector(ctx)

	testCtx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// Should complete without panicking
	panicOccurred := false
	go func() {
		defer func() {
			if r := recover(); r != nil {
				panicOccurred = true
			}
		}()
		collector.Start(testCtx, 100*time.Millisecond)
	}()

	<-testCtx.Done()

	if panicOccurred {
		t.Error("Unexpected panic occurred")
	}
}
