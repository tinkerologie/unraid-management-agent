package api

import (
	"fmt"
	"testing"
	"time"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

// ===== Mock CollectorManager =====

type mockCollectorManager struct {
	statuses    map[string]*dto.CollectorStatus
	enableErr   error
	disableErr  error
	intervalErr error
}

func newMockCollectorManager() *mockCollectorManager {
	return &mockCollectorManager{
		statuses: map[string]*dto.CollectorStatus{
			"system": {
				Name:     "system",
				Enabled:  true,
				Interval: 15,
				Status:   "running",
				Required: true,
			},
			"docker": {
				Name:     "docker",
				Enabled:  true,
				Interval: 30,
				Status:   "running",
				Required: false,
			},
			"gpu": {
				Name:     "gpu",
				Enabled:  false,
				Interval: 0,
				Status:   "disabled",
				Required: false,
			},
		},
	}
}

func (m *mockCollectorManager) EnableCollector(name string) error {
	if m.enableErr != nil {
		return m.enableErr
	}
	s, ok := m.statuses[name]
	if !ok {
		return fmt.Errorf("unknown collector: %s", name)
	}
	s.Enabled = true
	s.Status = "running"
	s.Interval = 30
	return nil
}

func (m *mockCollectorManager) DisableCollector(name string) error {
	if m.disableErr != nil {
		return m.disableErr
	}
	s, ok := m.statuses[name]
	if !ok {
		return fmt.Errorf("unknown collector: %s", name)
	}
	if s.Required {
		return fmt.Errorf("collector %s is required and cannot be disabled", name)
	}
	s.Enabled = false
	s.Status = "disabled"
	s.Interval = 0
	return nil
}

func (m *mockCollectorManager) UpdateInterval(name string, intervalSeconds int) error {
	if m.intervalErr != nil {
		return m.intervalErr
	}
	s, ok := m.statuses[name]
	if !ok {
		return fmt.Errorf("unknown collector: %s", name)
	}
	if intervalSeconds < 5 {
		return fmt.Errorf("interval must be at least 5 seconds")
	}
	s.Interval = intervalSeconds
	return nil
}

func (m *mockCollectorManager) GetStatus(name string) (*dto.CollectorStatus, error) {
	s, ok := m.statuses[name]
	if !ok {
		return nil, fmt.Errorf("unknown collector: %s", name)
	}
	return s, nil
}

func (m *mockCollectorManager) GetAllStatus() dto.CollectorsStatusResponse {
	var collectors []dto.CollectorStatus
	enabled := 0
	for _, s := range m.statuses {
		collectors = append(collectors, *s)
		if s.Enabled {
			enabled++
		}
	}
	return dto.CollectorsStatusResponse{
		Collectors:    collectors,
		Total:         len(collectors),
		EnabledCount:  enabled,
		DisabledCount: len(collectors) - enabled,
		Timestamp:     time.Now(),
	}
}

// ===== Mock MQTTClient =====

type mockMQTTClient struct {
	connected  bool
	testErr    error
	publishErr error
}

func (m *mockMQTTClient) IsConnected() bool {
	return m.connected
}

func (m *mockMQTTClient) GetStatus() *dto.MQTTStatus {
	return &dto.MQTTStatus{
		Connected: m.connected,
		Enabled:   true,
		Broker:    "tcp://localhost:1883",
		Timestamp: time.Now(),
	}
}

func (m *mockMQTTClient) TestConnection() error {
	return m.testErr
}

func (m *mockMQTTClient) PublishCustom(topic string, payload any, retain bool) error {
	return m.publishErr
}

// ===== Tests for Get*Cache methods =====

func TestGetSystemCache(t *testing.T) {
	server, _ := setupTestServer()

	// Nil cache
	if got := server.GetSystemCache(); got != nil {
		t.Errorf("expected nil, got %v", got)
	}

	// Populated cache
	populateTestCaches(server)
	got := server.GetSystemCache()
	if got == nil {
		t.Fatal("expected non-nil")
	}
	if got.Hostname != "TestServer" {
		t.Errorf("Hostname = %q, want %q", got.Hostname, "TestServer")
	}
}

func TestGetArrayCache(t *testing.T) {
	server, _ := setupTestServer()
	if got := server.GetArrayCache(); got != nil {
		t.Errorf("expected nil, got %v", got)
	}
	populateTestCaches(server)
	got := server.GetArrayCache()
	if got == nil || got.State != "Started" {
		t.Errorf("unexpected result: %v", got)
	}
}

func TestGetDisksCache(t *testing.T) {
	server, _ := setupTestServer()
	if got := server.GetDisksCache(); got != nil {
		t.Errorf("expected nil, got %v", got)
	}
	populateTestCaches(server)
	got := server.GetDisksCache()
	if len(got) != 3 {
		t.Errorf("expected 3 disks, got %d", len(got))
	}
}

func TestGetSharesCache(t *testing.T) {
	server, _ := setupTestServer()
	if got := server.GetSharesCache(); got != nil {
		t.Errorf("expected nil, got %v", got)
	}
	populateTestCaches(server)
	got := server.GetSharesCache()
	if len(got) != 2 {
		t.Errorf("expected 2 shares, got %d", len(got))
	}
}

func TestGetDockerCache(t *testing.T) {
	server, _ := setupTestServer()
	if got := server.GetDockerCache(); got != nil {
		t.Errorf("expected nil, got %v", got)
	}
	populateTestCaches(server)
	got := server.GetDockerCache()
	if len(got) != 2 {
		t.Errorf("expected 2 containers, got %d", len(got))
	}
}

func TestGetVMsCache(t *testing.T) {
	server, _ := setupTestServer()
	if got := server.GetVMsCache(); got != nil {
		t.Errorf("expected nil, got %v", got)
	}
	populateTestCaches(server)
	got := server.GetVMsCache()
	if len(got) != 2 {
		t.Errorf("expected 2 VMs, got %d", len(got))
	}
}

func TestGetUPSCache(t *testing.T) {
	server, _ := setupTestServer()
	if got := server.GetUPSCache(); got != nil {
		t.Errorf("expected nil, got %v", got)
	}
	populateTestCaches(server)
	got := server.GetUPSCache()
	if got == nil || got.Model != "APC Back-UPS 600" {
		t.Errorf("unexpected result: %v", got)
	}
}

func TestGetGPUCache(t *testing.T) {
	server, _ := setupTestServer()
	if got := server.GetGPUCache(); got != nil {
		t.Errorf("expected nil, got %v", got)
	}
	populateTestCaches(server)
	got := server.GetGPUCache()
	if len(got) != 1 {
		t.Errorf("expected 1 GPU, got %d", len(got))
	}
}

func TestGetNetworkCache(t *testing.T) {
	server, _ := setupTestServer()
	if got := server.GetNetworkCache(); got != nil {
		t.Errorf("expected nil, got %v", got)
	}
	populateTestCaches(server)
	got := server.GetNetworkCache()
	if len(got) != 2 {
		t.Errorf("expected 2 interfaces, got %d", len(got))
	}
}

func TestGetHardwareCache(t *testing.T) {
	server, _ := setupTestServer()
	if got := server.GetHardwareCache(); got != nil {
		t.Errorf("expected nil, got %v", got)
	}
	populateTestCaches(server)
	got := server.GetHardwareCache()
	if got == nil || got.BIOS == nil {
		t.Error("expected non-nil with BIOS")
	}
}

func TestGetRegistrationCache(t *testing.T) {
	server, _ := setupTestServer()
	if got := server.GetRegistrationCache(); got != nil {
		t.Errorf("expected nil, got %v", got)
	}
	populateTestCaches(server)
	got := server.GetRegistrationCache()
	if got == nil || got.Type != "Pro" {
		t.Errorf("unexpected result: %v", got)
	}
}

func TestGetNotificationsCache(t *testing.T) {
	server, _ := setupTestServer()
	if got := server.GetNotificationsCache(); got != nil {
		t.Errorf("expected nil, got %v", got)
	}
	populateTestCaches(server)
	got := server.GetNotificationsCache()
	if got == nil || len(got.Notifications) != 4 {
		t.Errorf("expected 4 notifications, got %v", got)
	}
}

func TestGetZFSPoolsCache(t *testing.T) {
	server, _ := setupTestServer()
	if got := server.GetZFSPoolsCache(); got != nil {
		t.Errorf("expected nil, got %v", got)
	}
	populateTestCaches(server)
	got := server.GetZFSPoolsCache()
	if len(got) != 1 {
		t.Errorf("expected 1 pool, got %d", len(got))
	}
}

func TestGetZFSDatasetsCache(t *testing.T) {
	server, _ := setupTestServer()
	if got := server.GetZFSDatasetsCache(); got != nil {
		t.Errorf("expected nil, got %v", got)
	}
	populateTestCaches(server)
	got := server.GetZFSDatasetsCache()
	if len(got) != 1 {
		t.Errorf("expected 1 dataset, got %d", len(got))
	}
}

func TestGetZFSSnapshotsCache(t *testing.T) {
	server, _ := setupTestServer()
	if got := server.GetZFSSnapshotsCache(); got != nil {
		t.Errorf("expected nil, got %v", got)
	}
	populateTestCaches(server)
	got := server.GetZFSSnapshotsCache()
	if len(got) != 1 {
		t.Errorf("expected 1 snapshot, got %d", len(got))
	}
}

func TestGetZFSARCStatsCache(t *testing.T) {
	server, _ := setupTestServer()
	if got := server.GetZFSARCStatsCache(); got != nil {
		t.Errorf("expected nil, got %v", got)
	}
	populateTestCaches(server)
	got := server.GetZFSARCStatsCache()
	if got == nil || got.SizeBytes != 8589934592 {
		t.Errorf("unexpected result: %v", got)
	}
}

func TestGetUnassignedCache(t *testing.T) {
	server, _ := setupTestServer()
	if got := server.GetUnassignedCache(); got != nil {
		t.Errorf("expected nil, got %v", got)
	}
	populateTestCaches(server)
	got := server.GetUnassignedCache()
	if got == nil || len(got.Devices) != 1 {
		t.Errorf("unexpected result: %v", got)
	}
}

func TestGetNUTCache(t *testing.T) {
	server, _ := setupTestServer()
	if got := server.GetNUTCache(); got != nil {
		t.Errorf("expected nil, got %v", got)
	}
	populateTestCaches(server)
	got := server.GetNUTCache()
	if got == nil || !got.Installed {
		t.Errorf("unexpected result: %v", got)
	}
}

func TestGetParityHistoryCache(t *testing.T) {
	server, _ := setupTestServer()
	// Returns an empty sentinel (never nil) so callers don't need nil checks.
	got := server.GetParityHistoryCache()
	if got == nil {
		t.Fatal("expected non-nil empty sentinel, got nil")
	}
	if len(got.Records) != 0 {
		t.Errorf("expected empty records, got %d entries", len(got.Records))
	}
}

// ===== Tests for utility methods =====

func TestGetRouter(t *testing.T) {
	server, _ := setupTestServer()
	router := server.GetRouter()
	if router == nil {
		t.Fatal("expected non-nil router")
	}
}

func TestGetContext(t *testing.T) {
	server, ctx := setupTestServer()
	got := server.GetContext()
	if got != ctx {
		t.Error("expected same context")
	}
}

func TestSetMQTTClient(t *testing.T) {
	server, _ := setupTestServer()

	// Initially nil
	if server.mqttClient != nil {
		t.Fatal("expected nil MQTT client")
	}

	// Set client
	mock := &mockMQTTClient{connected: true}
	server.SetMQTTClient(mock)
	if server.mqttClient == nil {
		t.Fatal("expected non-nil MQTT client")
	}
}

// ===== Tests for collector manager proxy methods =====

func TestGetCollectorsStatus_NilManager(t *testing.T) {
	server, _ := setupTestServer()
	got := server.GetCollectorsStatus()
	if got.Total != 0 {
		t.Errorf("expected empty response, got total=%d", got.Total)
	}
}

func TestGetCollectorsStatus_WithManager(t *testing.T) {
	ctx := &domain.Context{Config: domain.Config{Port: 8080}}
	server := NewServerWithCollectorManager(ctx, newMockCollectorManager())
	got := server.GetCollectorsStatus()
	if got.Total != 3 {
		t.Errorf("expected 3 collectors, got %d", got.Total)
	}
}

func TestGetCollectorStatus_NilManager(t *testing.T) {
	server, _ := setupTestServer()
	_, err := server.GetCollectorStatus("system")
	if err == nil {
		t.Fatal("expected error for nil manager")
	}
}

func TestGetCollectorStatus_WithManager(t *testing.T) {
	ctx := &domain.Context{Config: domain.Config{Port: 8080}}
	server := NewServerWithCollectorManager(ctx, newMockCollectorManager())
	status, err := server.GetCollectorStatus("system")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.Name != "system" {
		t.Errorf("expected system, got %s", status.Name)
	}
}

func TestGetCollectorStatus_NotFound(t *testing.T) {
	ctx := &domain.Context{Config: domain.Config{Port: 8080}}
	server := NewServerWithCollectorManager(ctx, newMockCollectorManager())
	_, err := server.GetCollectorStatus("nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown collector")
	}
}

func TestEnableCollector_NilManager(t *testing.T) {
	server, _ := setupTestServer()
	err := server.EnableCollector("system")
	if err == nil {
		t.Fatal("expected error for nil manager")
	}
}

func TestEnableCollector_Success(t *testing.T) {
	ctx := &domain.Context{Config: domain.Config{Port: 8080}}
	server := NewServerWithCollectorManager(ctx, newMockCollectorManager())
	err := server.EnableCollector("gpu")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEnableCollector_NotFound(t *testing.T) {
	ctx := &domain.Context{Config: domain.Config{Port: 8080}}
	server := NewServerWithCollectorManager(ctx, newMockCollectorManager())
	err := server.EnableCollector("nonexistent")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDisableCollector_NilManager(t *testing.T) {
	server, _ := setupTestServer()
	err := server.DisableCollector("system")
	if err == nil {
		t.Fatal("expected error for nil manager")
	}
}

func TestDisableCollector_Success(t *testing.T) {
	ctx := &domain.Context{Config: domain.Config{Port: 8080}}
	server := NewServerWithCollectorManager(ctx, newMockCollectorManager())
	err := server.DisableCollector("docker")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUpdateCollectorInterval_NilManager(t *testing.T) {
	server, _ := setupTestServer()
	err := server.UpdateCollectorInterval("system", 30)
	if err == nil {
		t.Fatal("expected error for nil manager")
	}
}

func TestUpdateCollectorInterval_Success(t *testing.T) {
	ctx := &domain.Context{Config: domain.Config{Port: 8080}}
	server := NewServerWithCollectorManager(ctx, newMockCollectorManager())
	err := server.UpdateCollectorInterval("system", 30)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUpdateCollectorInterval_NotFound(t *testing.T) {
	ctx := &domain.Context{Config: domain.Config{Port: 8080}}
	server := NewServerWithCollectorManager(ctx, newMockCollectorManager())
	err := server.UpdateCollectorInterval("nonexistent", 30)
	if err == nil {
		t.Fatal("expected error")
	}
}

// ===== Test GetHealthStatus =====

func TestGetHealthStatus_Empty(t *testing.T) {
	server, _ := setupTestServer()
	health := server.GetHealthStatus()
	if health == nil {
		t.Fatal("expected non-nil health map")
	}
	// No caches populated, so minimal data
	if _, ok := health["healthy_disks"]; !ok {
		t.Error("expected healthy_disks key")
	}
}

func TestGetHealthStatus_Populated(t *testing.T) {
	server, _ := setupTestServer()
	populateTestCaches(server)

	health := server.GetHealthStatus()

	// Check system metrics
	if _, ok := health["cpu_usage"]; !ok {
		t.Error("expected cpu_usage")
	}
	if _, ok := health["uptime"]; !ok {
		t.Error("expected uptime")
	}

	// Check array
	if state, ok := health["array_state"]; !ok || state != "Started" {
		t.Errorf("array_state = %v, want Started", state)
	}

	// Check containers
	if running, ok := health["running_containers"]; !ok || running != 1 {
		t.Errorf("running_containers = %v, want 1", running)
	}
	if total, ok := health["total_containers"]; !ok || total != 2 {
		t.Errorf("total_containers = %v, want 2", total)
	}

	// Check VMs
	if running, ok := health["running_vms"]; !ok || running != 1 {
		t.Errorf("running_vms = %v, want 1", running)
	}
}

// ===== Settings Methods (error path on non-Unraid) =====

func TestGetSystemSettings_ErrorPath(t *testing.T) {
	server, _ := setupTestServer()
	// On non-Unraid systems, config files don't exist, returns nil
	result := server.GetSystemSettings()
	if result != nil {
		t.Log("Unexpected non-nil result (may be running on Unraid)")
	}
}

func TestGetDockerSettings_ErrorPath(t *testing.T) {
	server, _ := setupTestServer()
	result := server.GetDockerSettings()
	if result != nil {
		t.Log("Unexpected non-nil result (may be running on Unraid)")
	}
}

func TestGetVMSettings_ErrorPath(t *testing.T) {
	server, _ := setupTestServer()
	result := server.GetVMSettings()
	if result != nil {
		t.Log("Unexpected non-nil result (may be running on Unraid)")
	}
}

func TestGetDiskSettings_ErrorPath(t *testing.T) {
	server, _ := setupTestServer()
	result := server.GetDiskSettings()
	if result != nil {
		t.Log("Unexpected non-nil result (may be running on Unraid)")
	}
}

func TestGetShareConfig_ErrorPath(t *testing.T) {
	server, _ := setupTestServer()
	result := server.GetShareConfig("nonexistent-share")
	if result != nil {
		t.Log("Unexpected non-nil result (may be running on Unraid)")
	}
}

func TestGetNetworkAccessURLs(t *testing.T) {
	server, _ := setupTestServer()
	result := server.GetNetworkAccessURLs()
	// May return nil or valid URLs depending on system
	_ = result
}

// ===== StartSubscriptions and Stop =====

func TestStartSubscriptions_And_Stop(t *testing.T) {
	server, ctx := setupTestServer()
	// Ensure Hub is initialized for subscription goroutines
	if ctx.Hub == nil {
		ctx.Hub = domain.NewEventBus(10)
		server.ctx = ctx
	}

	// Start subscriptions (launches background goroutines)
	server.StartSubscriptions()

	// Wait for subscriptions to be fully wired
	<-server.Ready()

	// Stop should cancel all goroutines gracefully
	server.Stop()
}

func TestStop_WithoutHTTPServer(t *testing.T) {
	server, _ := setupTestServer()
	// Stop without ever starting HTTP - should not panic
	server.Stop()
}
