package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

// ===== Helper to populate all caches for testing =====

func populateTestCaches(server *Server) {
	server.cacheMutex.Lock()
	defer server.cacheMutex.Unlock()

	server.systemCache = &dto.SystemInfo{
		Hostname: "TestServer",
		Uptime:   86400,
	}

	server.arrayCache = &dto.ArrayStatus{
		State: "Started",
	}

	server.disksCache = []dto.DiskInfo{
		{ID: "disk1", Name: "disk1", Device: "sdb", Status: "active", Role: "data"},
		{ID: "disk2", Name: "disk2", Device: "sdc", Status: "active", Role: "data"},
		{ID: "parity", Name: "parity", Device: "sda", Status: "active", Role: "parity"},
	}

	server.sharesCache = []dto.ShareInfo{
		{Name: "appdata"},
		{Name: "isos"},
	}

	server.dockerCache = []dto.ContainerInfo{
		{ID: "abc123def456", Name: "plex", State: "running", Image: "plexinc/plex-media-server"},
		{ID: "deadbeef1234", Name: "nginx", State: "exited", Image: "nginx:latest"},
	}

	server.vmsCache = []dto.VMInfo{
		{ID: "vm-uuid-1", Name: "Windows10", State: "running", CPUCount: 4},
		{ID: "vm-uuid-2", Name: "Ubuntu", State: "shut off", CPUCount: 2},
	}

	server.upsCache = &dto.UPSStatus{
		Model:         "APC Back-UPS 600",
		Status:        "ONLINE",
		BatteryCharge: 100.0,
		NominalPower:  360.0,
		LoadPercent:   25.0,
	}

	server.gpuCache = []*dto.GPUMetrics{
		{Available: true, Vendor: "nvidia"},
	}

	server.networkCache = []dto.NetworkInfo{
		{Name: "eth0", Speed: 1000, State: "up"},
		{Name: "br0", Speed: 1000, State: "up"},
	}

	server.hardwareCache = &dto.HardwareInfo{
		BIOS: &dto.BIOSInfo{
			Vendor:  "American Megatrends",
			Version: "3.4",
		},
		Baseboard: &dto.BaseboardInfo{
			Manufacturer: "ASRock",
			ProductName:  "X570 Taichi",
		},
		CPU: &dto.CPUHardwareInfo{
			Family: "Core i7",
		},
		Cache: []dto.CPUCacheInfo{
			{Level: 1, InstalledSize: "512 KB"},
		},
		MemoryArray: &dto.MemoryArrayInfo{
			MaximumCapacity: "128 GB",
		},
		MemoryDevices: []dto.MemoryDeviceInfo{
			{Size: "16 GB", Type: "DDR4"},
		},
	}

	server.registrationCache = &dto.Registration{
		Type: "Pro",
	}

	now := time.Now()
	server.notificationsCache = &dto.NotificationList{
		Overview: dto.NotificationOverview{
			Unread:  dto.NotificationCounts{Info: 2, Warning: 1, Alert: 0, Total: 3},
			Archive: dto.NotificationCounts{Info: 5, Warning: 2, Alert: 1, Total: 8},
		},
		Notifications: []dto.Notification{
			{ID: "n1", Title: "Array started", Importance: "info", Type: "unread", Timestamp: now},
			{ID: "n2", Title: "Disk temp high", Importance: "warning", Type: "unread", Timestamp: now},
			{ID: "n3", Title: "Plugin updated", Importance: "info", Type: "unread", Timestamp: now},
			{ID: "n4", Title: "Old alert", Importance: "alert", Type: "archive", Timestamp: now},
		},
		Timestamp: now,
	}

	server.unassignedCache = &dto.UnassignedDeviceList{
		Devices: []dto.UnassignedDevice{
			{Device: "sdd", Model: "Samsung 870 EVO", Status: "unmounted"},
		},
		RemoteShares: []dto.UnassignedRemoteShare{
			{Type: "smb", Source: "//nas/share", Status: "mounted"},
		},
		Timestamp: now,
	}

	server.zfsPoolsCache = []dto.ZFSPool{
		{Name: "tank", Health: "ONLINE", SizeBytes: 4000000000000},
	}

	server.zfsDatasetsCache = []dto.ZFSDataset{
		{Name: "tank/data", Type: "filesystem", UsedBytes: 1000000000},
	}

	server.zfsSnapshotsCache = []dto.ZFSSnapshot{
		{Name: "tank/data@backup1", Dataset: "tank/data", UsedBytes: 500000},
	}

	server.zfsARCStatsCache = &dto.ZFSARCStats{
		SizeBytes: 8589934592,
	}

	server.nutCache = &dto.NUTResponse{
		Installed: true,
	}
}

// ===== Simple GET cache handler tests =====

func TestHandleSystem_WithCache(t *testing.T) {
	server, _ := setupTestServer()
	populateTestCaches(server)

	req := httptest.NewRequest("GET", "/api/v1/system", nil)
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var result dto.SystemInfo
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if result.Hostname != "TestServer" {
		t.Errorf("Hostname = %q, want %q", result.Hostname, "TestServer")
	}
}

func TestHandleSystem_NilCache(t *testing.T) {
	server, _ := setupTestServer()

	req := httptest.NewRequest("GET", "/api/v1/system", nil)
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestHandleArray_WithCache(t *testing.T) {
	server, _ := setupTestServer()
	populateTestCaches(server)

	req := httptest.NewRequest("GET", "/api/v1/array", nil)
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var result dto.ArrayStatus
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if result.State != "Started" {
		t.Errorf("State = %q, want %q", result.State, "Started")
	}
}

func TestHandleArray_NilCache(t *testing.T) {
	server, _ := setupTestServer()

	req := httptest.NewRequest("GET", "/api/v1/array", nil)
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestHandleDisks_WithCache(t *testing.T) {
	server, _ := setupTestServer()
	populateTestCaches(server)

	req := httptest.NewRequest("GET", "/api/v1/disks", nil)
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var result []dto.DiskInfo
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if len(result) != 3 {
		t.Errorf("len = %d, want 3", len(result))
	}
}

func TestHandleDisks_NilCache(t *testing.T) {
	server, _ := setupTestServer()

	req := httptest.NewRequest("GET", "/api/v1/disks", nil)
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestHandleDisk_ByID(t *testing.T) {
	server, _ := setupTestServer()
	populateTestCaches(server)

	req := httptest.NewRequest("GET", "/api/v1/disks/disk1", nil)
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var result dto.DiskInfo
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if result.ID != "disk1" {
		t.Errorf("ID = %q, want %q", result.ID, "disk1")
	}
}

func TestHandleDisk_ByDevice(t *testing.T) {
	server, _ := setupTestServer()
	populateTestCaches(server)

	req := httptest.NewRequest("GET", "/api/v1/disks/sdb", nil)
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestHandleDisk_NotFound(t *testing.T) {
	server, _ := setupTestServer()
	populateTestCaches(server)

	req := httptest.NewRequest("GET", "/api/v1/disks/nonexistent", nil)
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

func TestHandleShares_WithCache(t *testing.T) {
	server, _ := setupTestServer()
	populateTestCaches(server)

	req := httptest.NewRequest("GET", "/api/v1/shares", nil)
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var result []dto.ShareInfo
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if len(result) != 2 {
		t.Errorf("len = %d, want 2", len(result))
	}
}

func TestHandleDockerList_WithCache(t *testing.T) {
	server, _ := setupTestServer()
	populateTestCaches(server)

	req := httptest.NewRequest("GET", "/api/v1/docker", nil)
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var result []dto.ContainerInfo
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if len(result) != 2 {
		t.Errorf("len = %d, want 2", len(result))
	}
}

func TestHandleDockerInfo_ByID(t *testing.T) {
	server, _ := setupTestServer()
	populateTestCaches(server)

	req := httptest.NewRequest("GET", "/api/v1/docker/abc123def456", nil)
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var result dto.ContainerInfo
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if result.Name != "plex" {
		t.Errorf("Name = %q, want %q", result.Name, "plex")
	}
}

func TestHandleDockerInfo_ByName(t *testing.T) {
	server, _ := setupTestServer()
	populateTestCaches(server)

	req := httptest.NewRequest("GET", "/api/v1/docker/nginx", nil)
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var result dto.ContainerInfo
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if result.Name != "nginx" {
		t.Errorf("Name = %q, want %q", result.Name, "nginx")
	}
}

func TestHandleDockerInfo_NotFound(t *testing.T) {
	server, _ := setupTestServer()
	populateTestCaches(server)

	req := httptest.NewRequest("GET", "/api/v1/docker/nonexistent", nil)
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

func TestHandleVMList_WithCache(t *testing.T) {
	server, _ := setupTestServer()
	populateTestCaches(server)

	req := httptest.NewRequest("GET", "/api/v1/vm", nil)
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var result []dto.VMInfo
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if len(result) != 2 {
		t.Errorf("len = %d, want 2", len(result))
	}
}

func TestHandleVMInfo_ByName(t *testing.T) {
	server, _ := setupTestServer()
	populateTestCaches(server)

	req := httptest.NewRequest("GET", "/api/v1/vm/Windows10", nil)
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var result dto.VMInfo
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if result.Name != "Windows10" {
		t.Errorf("Name = %q, want %q", result.Name, "Windows10")
	}
}

func TestHandleVMInfo_NotFound(t *testing.T) {
	server, _ := setupTestServer()
	populateTestCaches(server)

	req := httptest.NewRequest("GET", "/api/v1/vm/nonexistent", nil)
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

func TestHandleUPS_WithCache(t *testing.T) {
	server, _ := setupTestServer()
	populateTestCaches(server)

	req := httptest.NewRequest("GET", "/api/v1/ups", nil)
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var result dto.UPSStatus
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if result.Status != "ONLINE" {
		t.Errorf("Status = %q, want %q", result.Status, "ONLINE")
	}
}

func TestHandleUPS_NilCache(t *testing.T) {
	server, _ := setupTestServer()

	req := httptest.NewRequest("GET", "/api/v1/ups", nil)
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestHandleGPU_WithCache(t *testing.T) {
	server, _ := setupTestServer()
	populateTestCaches(server)

	req := httptest.NewRequest("GET", "/api/v1/gpu", nil)
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var result []*dto.GPUMetrics
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if len(result) != 1 {
		t.Errorf("len = %d, want 1", len(result))
	}
}

func TestHandleGPU_NilCache(t *testing.T) {
	server, _ := setupTestServer()

	req := httptest.NewRequest("GET", "/api/v1/gpu", nil)
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestHandleNetwork_WithCache(t *testing.T) {
	server, _ := setupTestServer()
	populateTestCaches(server)

	req := httptest.NewRequest("GET", "/api/v1/network", nil)
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var result []dto.NetworkInfo
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if len(result) != 2 {
		t.Errorf("len = %d, want 2", len(result))
	}
}

func TestHandleNetwork_NilCache(t *testing.T) {
	server, _ := setupTestServer()

	req := httptest.NewRequest("GET", "/api/v1/network", nil)
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestHandleHardwareFull_WithCache(t *testing.T) {
	server, _ := setupTestServer()
	populateTestCaches(server)

	req := httptest.NewRequest("GET", "/api/v1/hardware/full", nil)
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var result dto.HardwareInfo
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if result.BIOS == nil || result.BIOS.Vendor != "American Megatrends" {
		t.Errorf("unexpected BIOS data")
	}
}

func TestHandleHardwareFull_NilCache(t *testing.T) {
	server, _ := setupTestServer()

	req := httptest.NewRequest("GET", "/api/v1/hardware/full", nil)
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestHandleHardwareBIOS_WithCache(t *testing.T) {
	server, _ := setupTestServer()
	populateTestCaches(server)

	req := httptest.NewRequest("GET", "/api/v1/hardware/bios", nil)
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestHandleHardwareBaseboard_WithCache(t *testing.T) {
	server, _ := setupTestServer()
	populateTestCaches(server)

	req := httptest.NewRequest("GET", "/api/v1/hardware/baseboard", nil)
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestHandleHardwareCPU_WithCache(t *testing.T) {
	server, _ := setupTestServer()
	populateTestCaches(server)

	req := httptest.NewRequest("GET", "/api/v1/hardware/cpu", nil)
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestHandleHardwareCache_WithCache(t *testing.T) {
	server, _ := setupTestServer()
	populateTestCaches(server)

	req := httptest.NewRequest("GET", "/api/v1/hardware/cache", nil)
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestHandleHardwareMemoryArray_WithCache(t *testing.T) {
	server, _ := setupTestServer()
	populateTestCaches(server)

	req := httptest.NewRequest("GET", "/api/v1/hardware/memory-array", nil)
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestHandleHardwareMemoryDevices_WithCache(t *testing.T) {
	server, _ := setupTestServer()
	populateTestCaches(server)

	req := httptest.NewRequest("GET", "/api/v1/hardware/memory-devices", nil)
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestHandleRegistration_WithCache(t *testing.T) {
	server, _ := setupTestServer()
	populateTestCaches(server)

	req := httptest.NewRequest("GET", "/api/v1/registration", nil)
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var result dto.Registration
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if result.Type != "Pro" {
		t.Errorf("Type = %q, want %q", result.Type, "Pro")
	}
}

func TestHandleRegistration_NilCache(t *testing.T) {
	server, _ := setupTestServer()

	req := httptest.NewRequest("GET", "/api/v1/registration", nil)
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestHandleNotifications_WithCache(t *testing.T) {
	server, _ := setupTestServer()
	populateTestCaches(server)

	req := httptest.NewRequest("GET", "/api/v1/notifications", nil)
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestHandleNotifications_WithImportanceFilter(t *testing.T) {
	server, _ := setupTestServer()
	populateTestCaches(server)

	req := httptest.NewRequest("GET", "/api/v1/notifications?importance=warning", nil)
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestHandleNotifications_NilCache(t *testing.T) {
	server, _ := setupTestServer()

	req := httptest.NewRequest("GET", "/api/v1/notifications", nil)
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestHandleNotificationsUnread_WithCache(t *testing.T) {
	server, _ := setupTestServer()
	populateTestCaches(server)

	req := httptest.NewRequest("GET", "/api/v1/notifications/unread", nil)
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestHandleNotificationsArchive_WithCache(t *testing.T) {
	server, _ := setupTestServer()
	populateTestCaches(server)

	req := httptest.NewRequest("GET", "/api/v1/notifications/archive", nil)
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestHandleNotificationsOverview_WithCache(t *testing.T) {
	server, _ := setupTestServer()
	populateTestCaches(server)

	req := httptest.NewRequest("GET", "/api/v1/notifications/overview", nil)
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var result dto.NotificationOverview
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if result.Unread.Total != 3 {
		t.Errorf("Unread.Total = %d, want 3", result.Unread.Total)
	}
}

func TestHandleNotificationByID_Found(t *testing.T) {
	server, _ := setupTestServer()
	populateTestCaches(server)

	req := httptest.NewRequest("GET", "/api/v1/notifications/n1", nil)
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var result dto.Notification
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if result.ID != "n1" {
		t.Errorf("ID = %q, want %q", result.ID, "n1")
	}
}

func TestHandleNotificationByID_NotFound(t *testing.T) {
	server, _ := setupTestServer()
	populateTestCaches(server)

	req := httptest.NewRequest("GET", "/api/v1/notifications/nonexistent", nil)
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

func TestHandleUnassignedDevices_WithCache(t *testing.T) {
	server, _ := setupTestServer()
	populateTestCaches(server)

	req := httptest.NewRequest("GET", "/api/v1/unassigned", nil)
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestHandleUnassignedDevices_NilCache(t *testing.T) {
	server, _ := setupTestServer()

	req := httptest.NewRequest("GET", "/api/v1/unassigned", nil)
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestHandleUnassignedDevicesList_WithCache(t *testing.T) {
	server, _ := setupTestServer()
	populateTestCaches(server)

	req := httptest.NewRequest("GET", "/api/v1/unassigned/devices", nil)
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestHandleUnassignedRemoteShares_WithCache(t *testing.T) {
	server, _ := setupTestServer()
	populateTestCaches(server)

	req := httptest.NewRequest("GET", "/api/v1/unassigned/remote-shares", nil)
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestHandleZFSPools_WithCache(t *testing.T) {
	server, _ := setupTestServer()
	populateTestCaches(server)

	req := httptest.NewRequest("GET", "/api/v1/zfs/pools", nil)
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var result []dto.ZFSPool
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if len(result) != 1 {
		t.Errorf("len = %d, want 1", len(result))
	}
}

func TestHandleZFSPools_NilCache(t *testing.T) {
	server, _ := setupTestServer()

	req := httptest.NewRequest("GET", "/api/v1/zfs/pools", nil)
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestHandleZFSPool_ByName(t *testing.T) {
	server, _ := setupTestServer()
	populateTestCaches(server)

	req := httptest.NewRequest("GET", "/api/v1/zfs/pools/tank", nil)
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var result dto.ZFSPool
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if result.Name != "tank" {
		t.Errorf("Name = %q, want %q", result.Name, "tank")
	}
}

func TestHandleZFSPool_NotFound(t *testing.T) {
	server, _ := setupTestServer()
	populateTestCaches(server)

	req := httptest.NewRequest("GET", "/api/v1/zfs/pools/nonexistent", nil)
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

func TestHandleZFSDatasets_WithCache(t *testing.T) {
	server, _ := setupTestServer()
	populateTestCaches(server)

	req := httptest.NewRequest("GET", "/api/v1/zfs/datasets", nil)
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var result []dto.ZFSDataset
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if len(result) != 1 {
		t.Errorf("len = %d, want 1", len(result))
	}
}

func TestHandleZFSDatasets_NilCache(t *testing.T) {
	server, _ := setupTestServer()

	req := httptest.NewRequest("GET", "/api/v1/zfs/datasets", nil)
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestHandleZFSSnapshots_WithCache(t *testing.T) {
	server, _ := setupTestServer()
	populateTestCaches(server)

	req := httptest.NewRequest("GET", "/api/v1/zfs/snapshots", nil)
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestHandleZFSSnapshots_NilCache(t *testing.T) {
	server, _ := setupTestServer()

	req := httptest.NewRequest("GET", "/api/v1/zfs/snapshots", nil)
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestHandleZFSARC_WithCache(t *testing.T) {
	server, _ := setupTestServer()
	populateTestCaches(server)

	req := httptest.NewRequest("GET", "/api/v1/zfs/arc", nil)
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestHandleZFSARC_NilCache(t *testing.T) {
	server, _ := setupTestServer()

	req := httptest.NewRequest("GET", "/api/v1/zfs/arc", nil)
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestHandleNUT_WithCache(t *testing.T) {
	server, _ := setupTestServer()
	populateTestCaches(server)

	req := httptest.NewRequest("GET", "/api/v1/nut", nil)
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestHandleNUT_NilCache(t *testing.T) {
	server, _ := setupTestServer()

	req := httptest.NewRequest("GET", "/api/v1/nut", nil)
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

// ===== Services list endpoint =====

func TestHandleServicesList(t *testing.T) {
	server, _ := setupTestServer()

	req := httptest.NewRequest("GET", "/api/v1/services", nil)
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}
