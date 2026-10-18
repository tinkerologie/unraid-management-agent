package collectors

import (
	"context"
	"os"
	"sync"

	"github.com/tinkerologie/unraid-management-agent/daemon/logger"
)

// The Open, Refresh and Close methods below are the Dynamic-mode contract
// (#280) of the collectors whose Start does setup or teardown around Collect.
// The Dynamic runner calls them from one goroutine; Manual mode keeps Start.

// Open resolves the notification directories and creates them, as Start does.
func (c *NotificationCollector) Open(context.Context) error {
	if notificationsDir == defaultNotificationsBase+"/unread" {
		notificationsDir, notificationsArchiveDir = ResolveNotificationDirs(c.dynamixCfgPath)
	}
	for _, dir := range []string{notificationsDir, notificationsArchiveDir} {
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

// Open binds mount-churn checks to the runner's lifetime, as Start does.
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

// Close closes the Docker client, as Start does on exit. The SDK's Close
// only drops idle connections and never fails.
func (c *DockerCollector) Close() {
	if c.dockerClient != nil {
		_ = c.dockerClient.Close()
	}
}

// Open starts the guest-agent poller for the configured VMs, as Start does.
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
	}
}
