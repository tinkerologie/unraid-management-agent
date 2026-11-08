package collectors

import (
	"context"
	"os"
	"sync"

	"github.com/fsnotify/fsnotify"

	"github.com/tinkerologie/unraid-management-agent/daemon/logger"
)

// Open resolves and creates the directories for this generation.
func (c *NotificationCollector) Open(context.Context) error {
	c.unreadDir, c.archiveDir = notificationsDir, notificationsArchiveDir
	if c.unreadDir == defaultNotificationsBase+"/unread" {
		c.unreadDir, c.archiveDir = ResolveNotificationDirs(c.dynamixCfgPath)
	}
	for _, dir := range []string{c.unreadDir, c.archiveDir} {
		// #nosec G301 - Unraid standard permissions (0755 for directories)
		if err := os.MkdirAll(dir, 0755); err != nil {
			logger.Warning("Failed to create notifications directory %s: %v", dir, err)
		}
	}
	return nil
}

// Refresh collects and publishes the notifications once.
func (c *NotificationCollector) Refresh(context.Context) error {
	c.collect()
	return nil
}

// Close does nothing: the runner owns the directory watchers.
func (c *NotificationCollector) Close() {}

// ManualEvents preserves immediate create, remove and write triggers.
func (c *NotificationCollector) ManualEvents(ctx context.Context, trigger func()) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		logger.Warning("Notification collector: cannot create watcher: %v", err)
		return
	}
	defer func() { _ = w.Close() }()
	for _, dir := range []string{c.unreadDir, c.archiveDir} {
		if err := w.Add(dir); err != nil {
			logger.Warning("Notification collector: cannot watch %s: %v", dir, err)
		}
	}
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-w.Events:
			if !ok {
				return
			}
			if event.Op&(fsnotify.Create|fsnotify.Remove|fsnotify.Write) != 0 {
				trigger()
			}
		case err, ok := <-w.Errors:
			if !ok {
				return
			}
			logger.Warning("Notification watcher: %v", err)
		}
	}
}

// Open binds mount-churn checks to the runner's lifetime.
func (c *UnassignedCollector) Open(ctx context.Context) error {
	c.runCtx = ctx
	return nil
}

// Refresh collects and publishes the unassigned devices once.
func (c *UnassignedCollector) Refresh(context.Context) error {
	c.collect()
	return nil
}

// Close does nothing.
func (c *UnassignedCollector) Close() {}

// Open does nothing.
func (c *ZFSCollector) Open(context.Context) error { return nil }

// Refresh collects and publishes the ZFS data once.
func (c *ZFSCollector) Refresh(context.Context) error {
	c.collect()
	return nil
}

// Close does nothing.
func (c *ZFSCollector) Close() {}

// Open does nothing: Collect creates the Docker client on first use.
func (c *DockerCollector) Open(context.Context) error { return nil }

// Refresh collects and publishes the containers once.
func (c *DockerCollector) Refresh(context.Context) error {
	c.Collect()
	return nil
}

// Close closes the Docker client on exit. The SDK's Close
// only drops idle connections and never fails.
func (c *DockerCollector) Close() {
	if c.dockerClient != nil {
		_ = c.dockerClient.Close()
		c.dockerClient = nil
		c.initialized = false
	}
}

// Open starts the guest-agent poller for the configured VMs.
// It keeps its own cadence and stops with ctx.
func (c *VMCollector) Open(ctx context.Context) error {
	if len(c.appCtx.VMGuest.VMs) == 0 {
		return nil
	}
	c.guests = &sync.WaitGroup{}
	c.guests.Go(func() { newVMGuestPoller(c.appCtx.VMGuest, c.appCtx.Hub).Start(ctx) })
	return nil
}

// Refresh collects and publishes the VMs once.
func (c *VMCollector) Refresh(context.Context) error {
	c.Collect()
	return nil
}

// Close waits for the guest-agent poller, which stops with Open's ctx.
func (c *VMCollector) Close() {
	if c.guests != nil {
		c.guests.Wait()
		c.guests = nil
	}
}

// Open does nothing because PSU reads do not retain resources across polls.
func (c *PSUCollector) Open(context.Context) error { return nil }

// Refresh reads and publishes the PSU snapshot once.
func (c *PSUCollector) Refresh(ctx context.Context) error {
	c.Collect(ctx)
	return nil
}

// Close does nothing because Refresh owns and releases its reads.
func (c *PSUCollector) Close() {}
