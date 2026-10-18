package collectors

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tinkerologie/unraid-management-agent/daemon/domain"
)

func TestNotificationDynamicContract(t *testing.T) {
	unread, archive, cleanup := setupNotificationCollectorTestDirs(t)
	defer cleanup()
	// Open recreates missing directories.
	if err := os.RemoveAll(unread); err != nil {
		t.Fatal(err)
	}
	hub := domain.NewEventBus(10)
	ch := hub.Sub("notifications_update")
	c := NewNotificationCollector(&domain.Context{Hub: hub})
	if err := c.Open(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{unread, archive} {
		if _, err := os.Stat(dir); err != nil {
			t.Fatalf("Open should create %s: %v", dir, err)
		}
	}
	if err := c.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatal("Refresh should publish notifications")
	}
	c.Close()
}

func TestNotificationOpenSurvivesMkdirFailure(t *testing.T) {
	old, oldArchive := notificationsDir, notificationsArchiveDir
	defer func() { notificationsDir, notificationsArchiveDir = old, oldArchive }()
	// A regular file where a parent directory should be makes MkdirAll fail.
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	notificationsDir, notificationsArchiveDir = filepath.Join(blocker, "unread"), filepath.Join(blocker, "archive")
	c := &NotificationCollector{ctx: &domain.Context{Hub: domain.NewEventBus(1)}}
	// Open logs the failure and still succeeds: the collector polls whatever it can read.
	if err := c.Open(context.Background()); err != nil {
		t.Fatalf("Open = %v, want nil", err)
	}
}

func TestNotificationOpenResolvesDefaultDirs(t *testing.T) {
	old, oldArchive := notificationsDir, notificationsArchiveDir
	defer func() { notificationsDir, notificationsArchiveDir = old, oldArchive }()
	notificationsDir = defaultNotificationsBase + "/unread"
	tmp := t.TempDir()
	cfg := filepath.Join(tmp, "dynamix.cfg")
	if err := os.WriteFile(cfg, []byte("[notify]\npath=\""+filepath.Join(tmp, "n")+"\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	c := &NotificationCollector{ctx: &domain.Context{Hub: domain.NewEventBus(1)}, dynamixCfgPath: cfg}
	if err := c.Open(context.Background()); err != nil {
		t.Fatal(err)
	}
	if notificationsDir == defaultNotificationsBase+"/unread" {
		t.Fatal("Open should resolve the notification dirs from dynamix.cfg")
	}
}

func TestUnassignedDynamicContract(t *testing.T) {
	hub := domain.NewEventBus(10)
	ch := hub.Sub("unassigned_devices_update")
	c := NewUnassignedCollector(&domain.Context{Hub: hub})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := c.Open(ctx); err != nil {
		t.Fatal(err)
	}
	if c.runCtx != ctx {
		t.Fatal("Open should bind runCtx")
	}
	if err := c.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ch:
	case <-time.After(10 * time.Second):
		t.Fatal("Refresh should publish devices")
	}
	c.Close()
}

func TestZFSDynamicContract(t *testing.T) {
	c := NewZFSCollector(&domain.Context{Hub: domain.NewEventBus(10)})
	c.execOutput = func(string, ...string) (string, error) { return "", errors.New("no zfs") }
	if err := c.Open(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := c.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	c.Close()
}

func TestDockerDynamicContract(t *testing.T) {
	t.Setenv("DOCKER_HOST", "unix://"+filepath.Join(t.TempDir(), "missing.sock"))
	hub := domain.NewEventBus(10)
	ch := hub.Sub("container_list_update")
	c := NewDockerCollector(&domain.Context{Hub: hub})
	if err := c.Open(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := c.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ch: // an unreachable daemon publishes an empty list
	case <-time.After(10 * time.Second):
		t.Fatal("Refresh should publish containers")
	}
	c.Close() // closes the client Refresh created
	(&DockerCollector{}).Close()
}

func TestVMDynamicContract(t *testing.T) {
	c := NewVMCollector(&domain.Context{Hub: domain.NewEventBus(10)})
	if err := c.Open(context.Background()); err != nil {
		t.Fatal(err)
	}
	if c.guests != nil {
		t.Fatal("no guest VMs: Open starts no poller")
	}
	c.Close()

	// With guest VMs, Close waits for the poller, which stops with Open's ctx.
	c.appCtx.VMGuest = domain.VMGuestConfig{VMs: []string{"win"}, Interval: 30}
	ctx, cancel := context.WithCancel(context.Background())
	if err := c.Open(ctx); err != nil {
		t.Fatal(err)
	}
	cancel()
	done := make(chan struct{})
	go func() {
		c.Close()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Close should return once the poller stopped")
	}
}

func TestVMDynamicRefresh(t *testing.T) {
	c := NewVMCollector(&domain.Context{Hub: domain.NewEventBus(10)})
	if err := c.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestCollectWithWatchdogExported(t *testing.T) {
	ran := false
	CollectWithWatchdog(context.Background(), "test", time.Second, func() { ran = true })
	if !ran {
		t.Fatal("collect should run")
	}
}
