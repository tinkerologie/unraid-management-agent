package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

func TestHandleMetrics(t *testing.T) {
	// Create server with test context
	ctx := &domain.Context{
		Config: domain.Config{
			Port: 8043,
		},
	}
	server := NewServer(ctx)

	// Populate cache with test data
	server.cacheMutex.Lock()
	server.systemCache = &dto.SystemInfo{
		Hostname:     "test-tower",
		Version:      "7.2.3",
		AgentVersion: "2026.01.01",
		Uptime:       3600,
		CPUUsage:     45.5,
		CPUTemp:      55.0,
		RAMTotal:     34359738368,
		RAMUsed:      17179869184,
		RAMUsage:     50.0,
		Timestamp:    time.Now(),
	}
	server.arrayCache = &dto.ArrayStatus{
		State:               "Started",
		TotalBytes:          8000000000000,
		FreeBytes:           3000000000000,
		UsedPercent:         62.5,
		ParityValid:         true,
		ParityCheckStatus:   "idle",
		ParityCheckProgress: 0,
		NumDisks:            5,
		NumDataDisks:        3,
		NumParityDisks:      2,
		Timestamp:           time.Now(),
	}
	server.disksCache = []dto.DiskInfo{
		{
			Name:        "parity",
			Device:      "sda",
			Role:        "parity",
			Status:      "DISK_OK",
			Size:        4000000000000,
			Used:        0,
			Free:        0,
			Temperature: 35.0,
			SMARTStatus: "PASSED",
			SpinState:   "active",
			Timestamp:   time.Now(),
		},
		{
			Name:        "disk1",
			Device:      "sdb",
			Role:        "data",
			Status:      "DISK_OK",
			Size:        4000000000000,
			Used:        2500000000000,
			Free:        1500000000000,
			Temperature: 32.0,
			SMARTStatus: "PASSED",
			SpinState:   "standby",
			Timestamp:   time.Now(),
		},
	}
	server.dockerCache = []dto.ContainerInfo{
		{ID: "abc123", Name: "plex", Image: "plexinc/pms-docker", State: "running"},
		{ID: "def456", Name: "sonarr", Image: "linuxserver/sonarr", State: "running"},
		{ID: "ghi789", Name: "nginx", Image: "nginx:latest", State: "exited"},
	}
	server.vmsCache = []dto.VMInfo{
		{ID: "1", Name: "Windows10", State: "running"},
		{ID: "2", Name: "Ubuntu", State: "shutoff"},
	}
	server.sharesCache = []dto.ShareInfo{
		{Name: "appdata", Used: 50000000000},
		{Name: "media", Used: 4000000000000},
	}
	server.upsCache = &dto.UPSStatus{
		Connected:     true,
		Status:        "OL",
		Model:         "APC Smart-UPS 1500",
		BatteryCharge: 100,
		LoadPercent:   25,
		RuntimeLeft:   3600,
		Timestamp:     time.Now(),
	}
	server.cacheMutex.Unlock()

	// Create request
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()

	// Call handler
	server.handleMetrics(w, req)

	// Check response
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	body := w.Body.String()

	// Verify system metrics
	if !strings.Contains(body, "unraid_system_info") {
		t.Error("Expected unraid_system_info metric")
	}
	if !strings.Contains(body, `hostname="test-tower"`) {
		t.Error("Expected hostname label")
	}
	if !strings.Contains(body, "unraid_system_uptime_seconds") {
		t.Error("Expected unraid_system_uptime_seconds metric")
	}
	if !strings.Contains(body, "unraid_cpu_usage_percent") {
		t.Error("Expected unraid_cpu_usage_percent metric")
	}

	// Verify array metrics
	if !strings.Contains(body, "unraid_array_state 1") {
		t.Error("Expected unraid_array_state = 1 for started array")
	}
	if !strings.Contains(body, "unraid_array_total_bytes") {
		t.Error("Expected unraid_array_total_bytes metric")
	}
	if !strings.Contains(body, "unraid_parity_valid 1") {
		t.Error("Expected unraid_parity_valid = 1")
	}

	// Verify disk metrics
	if !strings.Contains(body, "unraid_disk_temperature_celsius") {
		t.Error("Expected unraid_disk_temperature_celsius metric")
	}
	if !strings.Contains(body, `disk="parity"`) {
		t.Error("Expected parity disk label")
	}
	if !strings.Contains(body, "unraid_disk_standby") {
		t.Error("Expected unraid_disk_standby metric")
	}

	// Verify Docker metrics
	if !strings.Contains(body, "unraid_docker_containers_total 3") {
		t.Error("Expected unraid_docker_containers_total = 3")
	}
	if !strings.Contains(body, "unraid_docker_containers_running 2") {
		t.Error("Expected unraid_docker_containers_running = 2")
	}
	if !strings.Contains(body, `name="plex"`) {
		t.Error("Expected plex container label")
	}

	// Verify VM metrics
	if !strings.Contains(body, "unraid_vms_total 2") {
		t.Error("Expected unraid_vms_total = 2")
	}
	if !strings.Contains(body, "unraid_vms_running 1") {
		t.Error("Expected unraid_vms_running = 1")
	}

	// Verify share metrics
	if !strings.Contains(body, "unraid_shares_total 2") {
		t.Error("Expected unraid_shares_total = 2")
	}
	if !strings.Contains(body, "unraid_share_used_bytes") {
		t.Error("Expected unraid_share_used_bytes metric")
	}

	// Verify UPS metrics
	if !strings.Contains(body, "unraid_ups_status") {
		t.Error("Expected unraid_ups_status metric")
	}
	if !strings.Contains(body, "unraid_ups_battery_charge_percent") {
		t.Error("Expected unraid_ups_battery_charge_percent metric")
	}

	// Verify service metrics (from network services)
	if !strings.Contains(body, "unraid_service_enabled") {
		t.Error("Expected unraid_service_enabled metric")
	}
	if !strings.Contains(body, "unraid_service_running") {
		t.Error("Expected unraid_service_running metric")
	}
}

func TestMetricsContentType(t *testing.T) {
	ctx := &domain.Context{Config: domain.Config{Port: 8043}}
	server := NewServer(ctx)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()

	server.handleMetrics(w, req)

	contentType := w.Header().Get("Content-Type")
	if !strings.Contains(contentType, "text/plain") && !strings.Contains(contentType, "application/openmetrics-text") {
		t.Errorf("Expected text/plain or openmetrics content type, got %s", contentType)
	}
}

func TestMetricsWithEmptyCache(t *testing.T) {
	ctx := &domain.Context{Config: domain.Config{Port: 8043}}
	server := NewServer(ctx)

	// Don't populate any cache - should not panic
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()

	server.handleMetrics(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200 with empty cache, got %d", w.Code)
	}
}

func TestMetricsArrayStateValues(t *testing.T) {
	ctx := &domain.Context{Config: domain.Config{Port: 8043}}
	server := NewServer(ctx)

	tests := []struct {
		name          string
		state         string
		expectedValue string
	}{
		{"Started uppercase", "STARTED", "1"},
		{"Started lowercase", "Started", "1"},
		{"Stopped", "Stopped", "0"},
		{"Maintenance", "Maintenance", "0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server.cacheMutex.Lock()
			server.arrayCache = &dto.ArrayStatus{
				State:      tt.state,
				TotalBytes: 1000,
				FreeBytes:  500,
			}
			server.cacheMutex.Unlock()

			req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
			w := httptest.NewRecorder()

			server.handleMetrics(w, req)

			body := w.Body.String()
			expected := "unraid_array_state " + tt.expectedValue
			if !strings.Contains(body, expected) {
				t.Errorf("Expected %s for state %s", expected, tt.state)
			}
		})
	}
}
