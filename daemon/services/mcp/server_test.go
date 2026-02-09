package mcp

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

// MockCacheProvider implements CacheProvider interface for testing
type MockCacheProvider struct {
	systemInfo    *dto.SystemInfo
	arrayStatus   *dto.ArrayStatus
	disks         []dto.DiskInfo
	shares        []dto.ShareInfo
	containers    []dto.ContainerInfo
	vms           []dto.VMInfo
	ups           *dto.UPSStatus
	gpus          []*dto.GPUMetrics
	network       []dto.NetworkInfo
	hardware      *dto.HardwareInfo
	registration  *dto.Registration
	notifications *dto.NotificationList
	zfsPools      []dto.ZFSPool
	zfsDatasets   []dto.ZFSDataset
	zfsSnapshots  []dto.ZFSSnapshot
	zfsARCStats   *dto.ZFSARCStats
	unassigned    *dto.UnassignedDeviceList
	nutResponse   *dto.NUTResponse
	parityHistory *dto.ParityCheckHistory
	// Log and collector mock data
	logFiles           []dto.LogFile
	collectorsStatus   dto.CollectorsStatusResponse
	collectorStatuses  map[string]*dto.CollectorStatus
	enabledCollectors  map[string]bool
	collectorIntervals map[string]int
}

func (m *MockCacheProvider) GetSystemCache() *dto.SystemInfo                { return m.systemInfo }
func (m *MockCacheProvider) GetArrayCache() *dto.ArrayStatus                { return m.arrayStatus }
func (m *MockCacheProvider) GetDisksCache() []dto.DiskInfo                  { return m.disks }
func (m *MockCacheProvider) GetSharesCache() []dto.ShareInfo                { return m.shares }
func (m *MockCacheProvider) GetDockerCache() []dto.ContainerInfo            { return m.containers }
func (m *MockCacheProvider) GetVMsCache() []dto.VMInfo                      { return m.vms }
func (m *MockCacheProvider) GetUPSCache() *dto.UPSStatus                    { return m.ups }
func (m *MockCacheProvider) GetGPUCache() []*dto.GPUMetrics                 { return m.gpus }
func (m *MockCacheProvider) GetNetworkCache() []dto.NetworkInfo             { return m.network }
func (m *MockCacheProvider) GetHardwareCache() *dto.HardwareInfo            { return m.hardware }
func (m *MockCacheProvider) GetRegistrationCache() *dto.Registration        { return m.registration }
func (m *MockCacheProvider) GetNotificationsCache() *dto.NotificationList   { return m.notifications }
func (m *MockCacheProvider) GetZFSPoolsCache() []dto.ZFSPool                { return m.zfsPools }
func (m *MockCacheProvider) GetZFSDatasetsCache() []dto.ZFSDataset          { return m.zfsDatasets }
func (m *MockCacheProvider) GetZFSSnapshotsCache() []dto.ZFSSnapshot        { return m.zfsSnapshots }
func (m *MockCacheProvider) GetZFSARCStatsCache() *dto.ZFSARCStats          { return m.zfsARCStats }
func (m *MockCacheProvider) GetUnassignedCache() *dto.UnassignedDeviceList  { return m.unassigned }
func (m *MockCacheProvider) GetNUTCache() *dto.NUTResponse                  { return m.nutResponse }
func (m *MockCacheProvider) GetParityHistoryCache() *dto.ParityCheckHistory { return m.parityHistory }

// Log methods
func (m *MockCacheProvider) ListLogFiles() []dto.LogFile { return m.logFiles }
func (m *MockCacheProvider) GetLogContent(path, lines, start string) (*dto.LogFileContent, error) {
	return &dto.LogFileContent{
		Path:          path,
		Content:       "Test log content line 1\nTest log content line 2\nTest log content line 3",
		Lines:         []string{"Test log content line 1", "Test log content line 2", "Test log content line 3"},
		TotalLines:    3,
		LinesReturned: 3,
		StartLine:     0,
		EndLine:       3,
	}, nil
}

// Collector methods
func (m *MockCacheProvider) GetCollectorsStatus() dto.CollectorsStatusResponse {
	return m.collectorsStatus
}
func (m *MockCacheProvider) GetCollectorStatus(name string) (*dto.CollectorStatus, error) {
	if status, ok := m.collectorStatuses[name]; ok {
		return status, nil
	}
	return nil, fmt.Errorf("unknown collector: %s", name)
}
func (m *MockCacheProvider) EnableCollector(name string) error {
	if _, ok := m.collectorStatuses[name]; !ok {
		return fmt.Errorf("unknown collector: %s", name)
	}
	m.enabledCollectors[name] = true
	return nil
}
func (m *MockCacheProvider) DisableCollector(name string) error {
	if status, ok := m.collectorStatuses[name]; ok {
		if status.Required {
			return fmt.Errorf("cannot disable %s collector (always required)", name)
		}
		m.enabledCollectors[name] = false
		return nil
	}
	return fmt.Errorf("unknown collector: %s", name)
}
func (m *MockCacheProvider) UpdateCollectorInterval(name string, interval int) error {
	if _, ok := m.collectorStatuses[name]; !ok {
		return fmt.Errorf("unknown collector: %s", name)
	}
	m.collectorIntervals[name] = interval
	return nil
}

// Settings and health methods
func (m *MockCacheProvider) GetSystemSettings() *dto.SystemSettings {
	return &dto.SystemSettings{ServerName: "Test-Unraid", Timezone: "America/New_York"}
}
func (m *MockCacheProvider) GetDockerSettings() *dto.DockerSettings {
	return &dto.DockerSettings{Enabled: true, ImagePath: "/mnt/user/docker"}
}
func (m *MockCacheProvider) GetVMSettings() *dto.VMSettings {
	return &dto.VMSettings{Enabled: true}
}
func (m *MockCacheProvider) GetDiskSettings() *dto.DiskSettings {
	return &dto.DiskSettings{SpindownDelay: 30, DefaultFsType: "xfs"}
}
func (m *MockCacheProvider) GetShareConfig(name string) *dto.ShareConfig {
	if name == "appdata" {
		return &dto.ShareConfig{Name: "appdata", UseCache: "prefer"}
	}
	return nil
}
func (m *MockCacheProvider) GetNetworkAccessURLs() *dto.NetworkAccessURLs {
	return &dto.NetworkAccessURLs{
		URLs: []dto.AccessURL{{Type: "lan", Name: "LAN", IPv4: "192.168.1.100"}},
	}
}
func (m *MockCacheProvider) GetHealthStatus() map[string]interface{} {
	return map[string]interface{}{
		"cpu_usage":   25.5,
		"ram_usage":   60.0,
		"array_state": "Started",
	}
}

func newMockCacheProvider() *MockCacheProvider {
	return &MockCacheProvider{
		systemInfo: &dto.SystemInfo{
			Hostname: "test-unraid",
			CPUUsage: 25.5,
			RAMUsage: 60.0,
			RAMTotal: 32 * 1024 * 1024 * 1024,
			RAMUsed:  19 * 1024 * 1024 * 1024,
			Uptime:   86400,
		},
		arrayStatus: &dto.ArrayStatus{
			State:          "Started",
			NumDataDisks:   4,
			NumParityDisks: 1,
			TotalBytes:     16 * 1024 * 1024 * 1024 * 1024,
			FreeBytes:      8 * 1024 * 1024 * 1024 * 1024,
			UsedPercent:    50.0,
			ParityValid:    true,
		},
		disks: []dto.DiskInfo{
			{ID: "disk1", Name: "disk1", Device: "sda", Size: 4 * 1024 * 1024 * 1024 * 1024, Temperature: 35},
			{ID: "disk2", Name: "disk2", Device: "sdb", Size: 4 * 1024 * 1024 * 1024 * 1024, Temperature: 36},
			{ID: "parity", Name: "parity", Device: "sdc", Size: 4 * 1024 * 1024 * 1024 * 1024, Temperature: 34},
		},
		shares: []dto.ShareInfo{
			{Name: "appdata", Path: "/mnt/user/appdata"},
			{Name: "media", Path: "/mnt/user/media"},
		},
		containers: []dto.ContainerInfo{
			{ID: "abc123", Name: "plex", State: "running", Image: "plexinc/pms-docker"},
			{ID: "def456", Name: "sonarr", State: "running", Image: "linuxserver/sonarr"},
			{ID: "ghi789", Name: "backup", State: "exited", Image: "duplicati/duplicati"},
		},
		vms: []dto.VMInfo{
			{ID: "vm-123", Name: "Windows10", State: "running", CPUCount: 4, MemoryAllocated: 8192 * 1024 * 1024},
			{ID: "vm-456", Name: "Ubuntu", State: "shut off", CPUCount: 2, MemoryAllocated: 4096 * 1024 * 1024},
		},
		ups: &dto.UPSStatus{
			Model:         "APC Back-UPS 1500",
			Status:        "ONLINE",
			BatteryCharge: 100,
			RuntimeLeft:   1800,
			LoadPercent:   25,
		},
		gpus: []*dto.GPUMetrics{
			{Name: "NVIDIA RTX 3080", UtilizationGPU: 45, UtilizationMemory: 60, Temperature: 65, Vendor: "NVIDIA"},
		},
		network: []dto.NetworkInfo{
			{Name: "eth0", IPAddress: "192.168.1.100", Speed: 1000, State: "up"},
			{Name: "br0", IPAddress: "192.168.1.101", Speed: 1000, State: "up"},
		},
		hardware: &dto.HardwareInfo{
			BIOS:      &dto.BIOSInfo{Vendor: "American Megatrends", Version: "1.0"},
			Baseboard: &dto.BaseboardInfo{Manufacturer: "ASRock", ProductName: "X570 Taichi"},
			CPU:       &dto.CPUHardwareInfo{Manufacturer: "AMD", CurrentSpeed: 3800},
		},
		registration: &dto.Registration{
			GUID:  "test-guid-12345",
			State: "valid",
			Type:  "Pro",
		},
		notifications: &dto.NotificationList{
			Notifications: []dto.Notification{
				{ID: "1", Subject: "Test Alert", Description: "This is a test notification", Importance: "alert"},
			},
		},
		zfsPools: []dto.ZFSPool{
			{Name: "tank", Health: "ONLINE", SizeBytes: 8 * 1024 * 1024 * 1024 * 1024},
		},
		zfsDatasets: []dto.ZFSDataset{
			{Name: "tank/data", UsedBytes: 1024 * 1024 * 1024 * 1024},
		},
		zfsSnapshots: []dto.ZFSSnapshot{
			{Name: "tank/data@backup1"},
		},
		zfsARCStats: &dto.ZFSARCStats{
			SizeBytes:   4 * 1024 * 1024 * 1024,
			Hits:        1000000,
			Misses:      1000,
			HitRatioPct: 99.9,
		},
		unassigned: &dto.UnassignedDeviceList{
			Devices: []dto.UnassignedDevice{
				{Device: "sdd", Model: "WD Blue 4TB", Status: "unmounted"},
			},
		},
		nutResponse: &dto.NUTResponse{
			Installed: true,
			Running:   true,
			Status: &dto.NUTStatus{
				Status: "OL",
			},
		},
		parityHistory: &dto.ParityCheckHistory{
			Records: []dto.ParityCheckRecord{
				{Action: "Parity-Check", Status: "OK", Errors: 0},
			},
		},
		// Logs
		logFiles: []dto.LogFile{
			{Name: "syslog", Path: "/var/log/syslog", Size: 1024 * 1024},
			{Name: "docker.log", Path: "/var/log/docker.log", Size: 512 * 1024},
			{Name: "messages", Path: "/var/log/messages", Size: 256 * 1024},
		},
		// Collectors
		collectorsStatus: dto.CollectorsStatusResponse{
			Collectors: []dto.CollectorStatus{
				{Name: "system", Enabled: true, Interval: 5, Status: "running", Required: true},
				{Name: "docker", Enabled: true, Interval: 10, Status: "running", Required: false},
				{Name: "vm", Enabled: true, Interval: 10, Status: "running", Required: false},
				{Name: "array", Enabled: true, Interval: 10, Status: "running", Required: false},
				{Name: "gpu", Enabled: false, Interval: 0, Status: "disabled", Required: false},
			},
			Total:         5,
			EnabledCount:  4,
			DisabledCount: 1,
		},
		collectorStatuses: map[string]*dto.CollectorStatus{
			"system": {Name: "system", Enabled: true, Interval: 5, Status: "running", Required: true},
			"docker": {Name: "docker", Enabled: true, Interval: 10, Status: "running", Required: false},
			"vm":     {Name: "vm", Enabled: true, Interval: 10, Status: "running", Required: false},
			"array":  {Name: "array", Enabled: true, Interval: 10, Status: "running", Required: false},
			"gpu":    {Name: "gpu", Enabled: false, Interval: 0, Status: "disabled", Required: false},
		},
		enabledCollectors:  make(map[string]bool),
		collectorIntervals: make(map[string]int),
	}
}

func setupTestMCPServer() (*Server, *MockCacheProvider) {
	ctx := &domain.Context{
		Config: domain.Config{
			Version: "test-1.0.0",
			Port:    8043,
		},
	}

	mock := newMockCacheProvider()
	server := NewServer(ctx, mock)

	return server, mock
}

func TestNewServer(t *testing.T) {
	server, mock := setupTestMCPServer()

	if server == nil {
		t.Fatal("expected server to be created")
	}

	if server.ctx == nil {
		t.Error("expected context to be set")
	}

	if server.cacheProvider != mock {
		t.Error("expected cache provider to be set")
	}
}

func TestServerInitialize(t *testing.T) {
	server, _ := setupTestMCPServer()

	err := server.Initialize()
	if err != nil {
		t.Fatalf("failed to initialize server: %v", err)
	}

	if server.httpHandler == nil {
		t.Error("expected HTTP handler to be initialized")
	}

	if server.mcpServer == nil {
		t.Error("expected MCP server to be initialized")
	}
}

func TestServerGetHTTPHandler(t *testing.T) {
	server, _ := setupTestMCPServer()

	err := server.Initialize()
	if err != nil {
		t.Fatalf("failed to initialize server: %v", err)
	}

	handler := server.GetHTTPHandler()
	if handler == nil {
		t.Error("expected handler to be returned")
	}
}

func TestMCPEndpointHandler(t *testing.T) {
	server, _ := setupTestMCPServer()

	err := server.Initialize()
	if err != nil {
		t.Fatalf("failed to initialize server: %v", err)
	}

	// Test that handler responds to requests
	req, _ := http.NewRequest("GET", "/mcp", nil)
	rr := httptest.NewRecorder()

	handler := server.GetHTTPHandler()
	handler.ServeHTTP(rr, req)

	// The StreamableHTTPHandler handles all HTTP methods internally
	// We're testing that the handler is functional
	if handler == nil {
		t.Error("expected handler to be returned")
	}
}

func TestMCPJSONResult(t *testing.T) {
	// Test jsonResult helper
	testData := map[string]string{"test": "value"}
	result, _, err := jsonResult(testData)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if result == nil {
		t.Fatal("expected result to be non-nil")
	}

	if len(result.Content) == 0 {
		t.Error("expected content in result")
	}
}

func TestMCPNilCacheResponses(t *testing.T) {
	ctx := &domain.Context{
		Config: domain.Config{
			Version: "test-1.0.0",
		},
	}

	// Create mock with nil caches
	mock := &MockCacheProvider{}
	server := NewServer(ctx, mock)

	err := server.Initialize()
	if err != nil {
		t.Fatalf("failed to initialize server: %v", err)
	}

	// Just verify it doesn't panic with nil caches
	if server.mcpServer == nil {
		t.Error("expected MCP server to be initialized even with nil caches")
	}
}

func TestCacheProviderInterface(t *testing.T) {
	mock := newMockCacheProvider()

	// Test all cache getters return expected values
	if mock.GetSystemCache() == nil {
		t.Error("GetSystemCache should return non-nil")
	}

	if mock.GetArrayCache() == nil {
		t.Error("GetArrayCache should return non-nil")
	}

	if len(mock.GetDisksCache()) == 0 {
		t.Error("GetDisksCache should return non-empty")
	}

	if len(mock.GetSharesCache()) == 0 {
		t.Error("GetSharesCache should return non-empty")
	}

	if len(mock.GetDockerCache()) == 0 {
		t.Error("GetDockerCache should return non-empty")
	}

	if len(mock.GetVMsCache()) == 0 {
		t.Error("GetVMsCache should return non-empty")
	}

	if mock.GetUPSCache() == nil {
		t.Error("GetUPSCache should return non-nil")
	}

	if len(mock.GetGPUCache()) == 0 {
		t.Error("GetGPUCache should return non-empty")
	}

	if len(mock.GetNetworkCache()) == 0 {
		t.Error("GetNetworkCache should return non-empty")
	}

	if mock.GetHardwareCache() == nil {
		t.Error("GetHardwareCache should return non-nil")
	}

	if mock.GetRegistrationCache() == nil {
		t.Error("GetRegistrationCache should return non-nil")
	}

	if mock.GetNotificationsCache() == nil {
		t.Error("GetNotificationsCache should return non-nil")
	}

	if len(mock.GetZFSPoolsCache()) == 0 {
		t.Error("GetZFSPoolsCache should return non-empty")
	}

	if len(mock.GetZFSDatasetsCache()) == 0 {
		t.Error("GetZFSDatasetsCache should return non-empty")
	}

	if len(mock.GetZFSSnapshotsCache()) == 0 {
		t.Error("GetZFSSnapshotsCache should return non-empty")
	}

	if mock.GetZFSARCStatsCache() == nil {
		t.Error("GetZFSARCStatsCache should return non-nil")
	}

	if mock.GetUnassignedCache() == nil {
		t.Error("GetUnassignedCache should return non-nil")
	}

	if mock.GetNUTCache() == nil {
		t.Error("GetNUTCache should return non-nil")
	}

	if mock.GetParityHistoryCache() == nil {
		t.Error("GetParityHistoryCache should return non-nil")
	}
}

func TestMockCacheProviderNil(t *testing.T) {
	// Test empty mock returns nil for all caches
	mock := &MockCacheProvider{}

	if mock.GetSystemCache() != nil {
		t.Error("Empty mock GetSystemCache should return nil")
	}

	if mock.GetArrayCache() != nil {
		t.Error("Empty mock GetArrayCache should return nil")
	}

	if mock.GetDisksCache() != nil {
		t.Error("Empty mock GetDisksCache should return nil")
	}

	if mock.GetDockerCache() != nil {
		t.Error("Empty mock GetDockerCache should return nil")
	}

	if mock.GetVMsCache() != nil {
		t.Error("Empty mock GetVMsCache should return nil")
	}

	if mock.GetUPSCache() != nil {
		t.Error("Empty mock GetUPSCache should return nil")
	}
}

func TestJSONResultMarshal(t *testing.T) {
	tests := []struct {
		name    string
		input   interface{}
		wantErr bool
	}{
		{
			name:    "simple map",
			input:   map[string]string{"key": "value"},
			wantErr: false,
		},
		{
			name:    "nested struct",
			input:   dto.SystemInfo{Hostname: "test"},
			wantErr: false,
		},
		{
			name:    "array",
			input:   []string{"a", "b", "c"},
			wantErr: false,
		},
		{
			name:    "nil input",
			input:   nil,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, _, err := jsonResult(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("jsonResult() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && result == nil {
				t.Error("expected non-nil result")
			}
		})
	}
}

func TestTextResult(t *testing.T) {
	// Test textResult helper
	text := "This is a test message"

	result := textResult(text)

	// Verify the result has content
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	if len(result.Content) == 0 {
		t.Error("expected content in result")
	}
}

func TestServerVersion(t *testing.T) {
	ctx := &domain.Context{
		Config: domain.Config{
			Version: "2025.01.15",
			Port:    8043,
		},
	}

	mock := newMockCacheProvider()
	server := NewServer(ctx, mock)

	if server.ctx.Config.Version != "2025.01.15" {
		t.Errorf("expected version 2025.01.15, got %s", server.ctx.Config.Version)
	}
}

func TestServerPort(t *testing.T) {
	ctx := &domain.Context{
		Config: domain.Config{
			Version: "test",
			Port:    9999,
		},
	}

	mock := newMockCacheProvider()
	server := NewServer(ctx, mock)

	if server.ctx.Config.Port != 9999 {
		t.Errorf("expected port 9999, got %d", server.ctx.Config.Port)
	}
}
