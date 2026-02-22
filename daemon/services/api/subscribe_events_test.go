package api

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cskr/pubsub"
	"github.com/gorilla/websocket"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

// startSubscribeToEvents starts the subscribeToEvents goroutine and returns a cancel func.
func startSubscribeToEvents(t *testing.T, server *Server) context.CancelFunc {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	go server.subscribeToEvents(ctx)
	// Give the subscription a moment to start
	time.Sleep(50 * time.Millisecond)
	return cancel
}

func TestSubscribeToEvents_SystemUpdate(t *testing.T) {
	hub := pubsub.New(10)
	appCtx := &domain.Context{Hub: hub, Config: domain.Config{Version: "test"}}
	server := NewServer(appCtx)

	cancel := startSubscribeToEvents(t, server)
	defer cancel()

	sysInfo := &dto.SystemInfo{Hostname: "test-sub", Uptime: 999}
	hub.Pub(sysInfo, "system_update")
	time.Sleep(100 * time.Millisecond)

	server.cacheMutex.RLock()
	cached := server.systemCache
	server.cacheMutex.RUnlock()

	if cached == nil {
		t.Fatal("systemCache not updated")
	}
	if cached.Hostname != "test-sub" {
		t.Errorf("Hostname = %q, want %q", cached.Hostname, "test-sub")
	}
}

func TestSubscribeToEvents_ArrayStatusUpdate(t *testing.T) {
	hub := pubsub.New(10)
	appCtx := &domain.Context{Hub: hub, Config: domain.Config{Version: "test"}}
	server := NewServer(appCtx)

	cancel := startSubscribeToEvents(t, server)
	defer cancel()

	arrayStatus := &dto.ArrayStatus{State: "Started", NumDisks: 5}
	hub.Pub(arrayStatus, "array_status_update")
	time.Sleep(100 * time.Millisecond)

	server.cacheMutex.RLock()
	cached := server.arrayCache
	server.cacheMutex.RUnlock()

	if cached == nil {
		t.Fatal("arrayCache not updated")
	}
	if cached.State != "Started" {
		t.Errorf("State = %q, want %q", cached.State, "Started")
	}
}

func TestSubscribeToEvents_DiskListUpdate(t *testing.T) {
	hub := pubsub.New(10)
	appCtx := &domain.Context{Hub: hub, Config: domain.Config{Version: "test"}}
	server := NewServer(appCtx)

	cancel := startSubscribeToEvents(t, server)
	defer cancel()

	disks := []dto.DiskInfo{{Name: "disk1", Device: "sda"}, {Name: "disk2", Device: "sdb"}}
	hub.Pub(disks, "disk_list_update")
	time.Sleep(100 * time.Millisecond)

	server.cacheMutex.RLock()
	cached := server.disksCache
	server.cacheMutex.RUnlock()

	if len(cached) != 2 {
		t.Fatalf("disksCache len = %d, want 2", len(cached))
	}
}

func TestSubscribeToEvents_ShareListUpdate(t *testing.T) {
	hub := pubsub.New(10)
	appCtx := &domain.Context{Hub: hub, Config: domain.Config{Version: "test"}}
	server := NewServer(appCtx)

	cancel := startSubscribeToEvents(t, server)
	defer cancel()

	shares := []dto.ShareInfo{{Name: "appdata"}, {Name: "media"}}
	hub.Pub(shares, "share_list_update")
	time.Sleep(100 * time.Millisecond)

	server.cacheMutex.RLock()
	cached := server.sharesCache
	server.cacheMutex.RUnlock()

	if len(cached) != 2 {
		t.Fatalf("sharesCache len = %d, want 2", len(cached))
	}
}

func TestSubscribeToEvents_ContainerListUpdate(t *testing.T) {
	hub := pubsub.New(10)
	appCtx := &domain.Context{Hub: hub, Config: domain.Config{Version: "test"}}
	server := NewServer(appCtx)

	cancel := startSubscribeToEvents(t, server)
	defer cancel()

	// Collectors publish as pointer slice — subscribeToEvents converts to value slice
	containers := []*dto.ContainerInfo{
		{ID: "abc", Name: "plex", State: "running"},
		{ID: "def", Name: "nginx", State: "exited"},
	}
	hub.Pub(containers, "container_list_update")
	time.Sleep(100 * time.Millisecond)

	server.cacheMutex.RLock()
	cached := server.dockerCache
	server.cacheMutex.RUnlock()

	if len(cached) != 2 {
		t.Fatalf("dockerCache len = %d, want 2", len(cached))
	}
	if cached[0].Name != "plex" {
		t.Errorf("dockerCache[0].Name = %q, want %q", cached[0].Name, "plex")
	}
}

func TestSubscribeToEvents_VMListUpdate(t *testing.T) {
	hub := pubsub.New(10)
	appCtx := &domain.Context{Hub: hub, Config: domain.Config{Version: "test"}}
	server := NewServer(appCtx)

	cancel := startSubscribeToEvents(t, server)
	defer cancel()

	// Collectors publish as pointer slice
	vms := []*dto.VMInfo{
		{ID: "1", Name: "Windows10", State: "running"},
	}
	hub.Pub(vms, "vm_list_update")
	time.Sleep(100 * time.Millisecond)

	server.cacheMutex.RLock()
	cached := server.vmsCache
	server.cacheMutex.RUnlock()

	if len(cached) != 1 {
		t.Fatalf("vmsCache len = %d, want 1", len(cached))
	}
}

func TestSubscribeToEvents_UPSStatusUpdate(t *testing.T) {
	hub := pubsub.New(10)
	appCtx := &domain.Context{Hub: hub, Config: domain.Config{Version: "test"}}
	server := NewServer(appCtx)

	cancel := startSubscribeToEvents(t, server)
	defer cancel()

	ups := &dto.UPSStatus{Status: "OL", Model: "APC", BatteryCharge: 100}
	hub.Pub(ups, "ups_status_update")
	time.Sleep(100 * time.Millisecond)

	server.cacheMutex.RLock()
	cached := server.upsCache
	server.cacheMutex.RUnlock()

	if cached == nil {
		t.Fatal("upsCache not updated")
	}
	if cached.Status != "OL" {
		t.Errorf("Status = %q, want %q", cached.Status, "OL")
	}
}

func TestSubscribeToEvents_NUTStatusUpdate(t *testing.T) {
	hub := pubsub.New(10)
	appCtx := &domain.Context{Hub: hub, Config: domain.Config{Version: "test"}}
	server := NewServer(appCtx)

	cancel := startSubscribeToEvents(t, server)
	defer cancel()

	nut := &dto.NUTResponse{Installed: true, Running: true}
	hub.Pub(nut, "nut_status_update")
	time.Sleep(100 * time.Millisecond)

	server.cacheMutex.RLock()
	cached := server.nutCache
	server.cacheMutex.RUnlock()

	if cached == nil {
		t.Fatal("nutCache not updated")
	}
	if !cached.Installed {
		t.Error("Expected Installed=true")
	}
}

func TestSubscribeToEvents_GPUMetricsUpdate(t *testing.T) {
	hub := pubsub.New(10)
	appCtx := &domain.Context{Hub: hub, Config: domain.Config{Version: "test"}}
	server := NewServer(appCtx)

	cancel := startSubscribeToEvents(t, server)
	defer cancel()

	gpus := []*dto.GPUMetrics{{Name: "RTX 3080", Temperature: 65}}
	hub.Pub(gpus, "gpu_metrics_update")
	time.Sleep(100 * time.Millisecond)

	server.cacheMutex.RLock()
	cached := server.gpuCache
	server.cacheMutex.RUnlock()

	if len(cached) != 1 {
		t.Fatalf("gpuCache len = %d, want 1", len(cached))
	}
}

func TestSubscribeToEvents_NetworkListUpdate(t *testing.T) {
	hub := pubsub.New(10)
	appCtx := &domain.Context{Hub: hub, Config: domain.Config{Version: "test"}}
	server := NewServer(appCtx)

	cancel := startSubscribeToEvents(t, server)
	defer cancel()

	networks := []dto.NetworkInfo{{Name: "eth0", Speed: 1000, State: "up"}}
	hub.Pub(networks, "network_list_update")
	time.Sleep(100 * time.Millisecond)

	server.cacheMutex.RLock()
	cached := server.networkCache
	server.cacheMutex.RUnlock()

	if len(cached) != 1 {
		t.Fatalf("networkCache len = %d, want 1", len(cached))
	}
}

func TestSubscribeToEvents_HardwareUpdate(t *testing.T) {
	hub := pubsub.New(10)
	appCtx := &domain.Context{Hub: hub, Config: domain.Config{Version: "test"}}
	server := NewServer(appCtx)

	cancel := startSubscribeToEvents(t, server)
	defer cancel()

	hw := &dto.HardwareInfo{BIOS: &dto.BIOSInfo{Vendor: "AMI"}}
	hub.Pub(hw, "hardware_update")
	time.Sleep(100 * time.Millisecond)

	server.cacheMutex.RLock()
	cached := server.hardwareCache
	server.cacheMutex.RUnlock()

	if cached == nil {
		t.Fatal("hardwareCache not updated")
	}
	if cached.BIOS == nil || cached.BIOS.Vendor != "AMI" {
		t.Error("Expected BIOS.Vendor=AMI")
	}
}

func TestSubscribeToEvents_RegistrationUpdate(t *testing.T) {
	hub := pubsub.New(10)
	appCtx := &domain.Context{Hub: hub, Config: domain.Config{Version: "test"}}
	server := NewServer(appCtx)

	cancel := startSubscribeToEvents(t, server)
	defer cancel()

	reg := &dto.Registration{Type: "Pro", State: "valid"}
	hub.Pub(reg, "registration_update")
	time.Sleep(100 * time.Millisecond)

	server.cacheMutex.RLock()
	cached := server.registrationCache
	server.cacheMutex.RUnlock()

	if cached == nil {
		t.Fatal("registrationCache not updated")
	}
	if cached.Type != "Pro" {
		t.Errorf("Type = %q, want %q", cached.Type, "Pro")
	}
}

func TestSubscribeToEvents_NotificationsUpdate(t *testing.T) {
	hub := pubsub.New(10)
	appCtx := &domain.Context{Hub: hub, Config: domain.Config{Version: "test"}}
	server := NewServer(appCtx)

	cancel := startSubscribeToEvents(t, server)
	defer cancel()

	notifs := &dto.NotificationList{
		Overview: dto.NotificationOverview{
			Unread: dto.NotificationCounts{Total: 5},
		},
	}
	hub.Pub(notifs, "notifications_update")
	time.Sleep(100 * time.Millisecond)

	server.cacheMutex.RLock()
	cached := server.notificationsCache
	server.cacheMutex.RUnlock()

	if cached == nil {
		t.Fatal("notificationsCache not updated")
	}
	if cached.Overview.Unread.Total != 5 {
		t.Errorf("Unread.Total = %d, want 5", cached.Overview.Unread.Total)
	}
}

func TestSubscribeToEvents_UnassignedDevicesUpdate(t *testing.T) {
	hub := pubsub.New(10)
	appCtx := &domain.Context{Hub: hub, Config: domain.Config{Version: "test"}}
	server := NewServer(appCtx)

	cancel := startSubscribeToEvents(t, server)
	defer cancel()

	devices := &dto.UnassignedDeviceList{
		Devices: []dto.UnassignedDevice{{Device: "sdd"}},
	}
	hub.Pub(devices, "unassigned_devices_update")
	time.Sleep(100 * time.Millisecond)

	server.cacheMutex.RLock()
	cached := server.unassignedCache
	server.cacheMutex.RUnlock()

	if cached == nil {
		t.Fatal("unassignedCache not updated")
	}
	if len(cached.Devices) != 1 {
		t.Errorf("Devices len = %d, want 1", len(cached.Devices))
	}
}

func TestSubscribeToEvents_ZFSPoolsUpdate(t *testing.T) {
	hub := pubsub.New(10)
	appCtx := &domain.Context{Hub: hub, Config: domain.Config{Version: "test"}}
	server := NewServer(appCtx)

	cancel := startSubscribeToEvents(t, server)
	defer cancel()

	pools := []dto.ZFSPool{{Name: "tank", Health: "ONLINE"}}
	hub.Pub(pools, "zfs_pools_update")
	time.Sleep(100 * time.Millisecond)

	server.cacheMutex.RLock()
	cached := server.zfsPoolsCache
	server.cacheMutex.RUnlock()

	if len(cached) != 1 {
		t.Fatalf("zfsPoolsCache len = %d, want 1", len(cached))
	}
}

func TestSubscribeToEvents_ZFSDatasetsUpdate(t *testing.T) {
	hub := pubsub.New(10)
	appCtx := &domain.Context{Hub: hub, Config: domain.Config{Version: "test"}}
	server := NewServer(appCtx)

	cancel := startSubscribeToEvents(t, server)
	defer cancel()

	datasets := []dto.ZFSDataset{{Name: "tank/data", Type: "filesystem"}}
	hub.Pub(datasets, "zfs_datasets_update")
	time.Sleep(100 * time.Millisecond)

	server.cacheMutex.RLock()
	cached := server.zfsDatasetsCache
	server.cacheMutex.RUnlock()

	if len(cached) != 1 {
		t.Fatalf("zfsDatasetsCache len = %d, want 1", len(cached))
	}
}

func TestSubscribeToEvents_ZFSSnapshotsUpdate(t *testing.T) {
	hub := pubsub.New(10)
	appCtx := &domain.Context{Hub: hub, Config: domain.Config{Version: "test"}}
	server := NewServer(appCtx)

	cancel := startSubscribeToEvents(t, server)
	defer cancel()

	snapshots := []dto.ZFSSnapshot{{Name: "tank/data@snap1", Dataset: "tank/data"}}
	hub.Pub(snapshots, "zfs_snapshots_update")
	time.Sleep(100 * time.Millisecond)

	server.cacheMutex.RLock()
	cached := server.zfsSnapshotsCache
	server.cacheMutex.RUnlock()

	if len(cached) != 1 {
		t.Fatalf("zfsSnapshotsCache len = %d, want 1", len(cached))
	}
}

func TestSubscribeToEvents_ZFSARCStatsUpdate(t *testing.T) {
	hub := pubsub.New(10)
	appCtx := &domain.Context{Hub: hub, Config: domain.Config{Version: "test"}}
	server := NewServer(appCtx)

	cancel := startSubscribeToEvents(t, server)
	defer cancel()

	// ZFSARCStats is a value type (not pointer)
	arcStats := dto.ZFSARCStats{SizeBytes: 8589934592, HitRatioPct: 95.5}
	hub.Pub(arcStats, "zfs_arc_stats_update")
	time.Sleep(100 * time.Millisecond)

	server.cacheMutex.RLock()
	cached := server.zfsARCStatsCache
	server.cacheMutex.RUnlock()

	if cached == nil {
		t.Fatal("zfsARCStatsCache not updated")
	}
	if cached.HitRatioPct != 95.5 {
		t.Errorf("HitRatioPct = %f, want 95.5", cached.HitRatioPct)
	}
}

func TestSubscribeToEvents_ContextCancellation(t *testing.T) {
	hub := pubsub.New(10)
	appCtx := &domain.Context{Hub: hub, Config: domain.Config{Version: "test"}}
	server := NewServer(appCtx)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		server.subscribeToEvents(ctx)
		close(done)
	}()
	time.Sleep(50 * time.Millisecond)

	cancel()

	select {
	case <-done:
		// Success — goroutine exited
	case <-time.After(2 * time.Second):
		t.Error("subscribeToEvents did not exit after context cancellation")
	}
}

func TestSubscribeToEvents_UnknownType(t *testing.T) {
	hub := pubsub.New(10)
	appCtx := &domain.Context{Hub: hub, Config: domain.Config{Version: "test"}}
	server := NewServer(appCtx)

	cancel := startSubscribeToEvents(t, server)
	defer cancel()

	// Publish an unknown type — should not panic, just logs a warning
	hub.Pub("unknown string type", "system_update")
	time.Sleep(100 * time.Millisecond)

	// Verify no caches were changed
	server.cacheMutex.RLock()
	defer server.cacheMutex.RUnlock()
	if server.systemCache != nil {
		t.Error("systemCache should still be nil after unknown type")
	}
}

func TestBroadcastEvents_ContextCancellation(t *testing.T) {
	hub := pubsub.New(10)
	appCtx := &domain.Context{Hub: hub, Config: domain.Config{Version: "test"}}
	server := NewServer(appCtx)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		server.broadcastEvents(ctx)
		close(done)
	}()
	time.Sleep(50 * time.Millisecond)

	cancel()

	select {
	case <-done:
		// Success
	case <-time.After(2 * time.Second):
		t.Error("broadcastEvents did not exit after context cancellation")
	}
}

func TestBroadcastEvents_ForwardsToWSHub(t *testing.T) {
	hub := pubsub.New(10)
	appCtx := &domain.Context{Hub: hub, Config: domain.Config{Version: "test"}}
	server := NewServer(appCtx)

	// Start WSHub
	go server.wsHub.Run(server.cancelCtx)
	defer server.cancelFunc()

	ctx := t.Context()
	go server.broadcastEvents(ctx)
	time.Sleep(50 * time.Millisecond)

	// Create test server for WS connections
	ts := httptest.NewServer(server.router)
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/v1/ws"
	ws, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("WebSocket dial failed: %v", err)
	}
	if resp != nil {
		resp.Body.Close()
	}
	defer ws.Close()
	time.Sleep(50 * time.Millisecond)

	// Publish event — broadcastEvents should forward it to WSHub
	hub.Pub(&dto.SystemInfo{Hostname: "broadcast-test"}, "system_update")

	ws.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _, err = ws.ReadMessage()
	if err != nil {
		t.Fatalf("WebSocket client did not receive broadcast: %v", err)
	}
}
