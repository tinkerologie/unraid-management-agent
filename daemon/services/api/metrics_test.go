package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/tinkerologie/unraid-management-agent/daemon/domain"
	"github.com/tinkerologie/unraid-management-agent/daemon/dto"
)

func TestHandleMetrics(t *testing.T) {
	// Create server with test context
	ctx := &domain.Context{
		Port: 8043,
	}
	server := NewServer(ctx)

	// Populate cache with test data
	server.systemCache.Store(&dto.SystemInfo{
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
	})
	server.arrayCache.Store(&dto.ArrayStatus{
		State:               "Started",
		CapacitySampledAt:   new(time.Unix(3600, 0)),
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
	})
	disksVal := []dto.DiskInfo{{
		Name:        "parity",
		Device:      "sda",
		Role:        "parity",
		Status:      "DISK_OK",
		Size:        4000000000000,
		Used:        0,
		Free:        0,
		Temperature: new(float64(35.0)),
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
			Temperature: new(float64(32.0)),
			SMARTStatus: "PASSED",
			SpinState:   "standby",
			Timestamp:   time.Now(),
		},
	}
	server.disksCache.Store(&disksVal)
	dockerVal := []dto.ContainerInfo{{ID: "abc123", Name: "plex", Image: "plexinc/pms-docker", State: "running"},
		{ID: "def456", Name: "sonarr", Image: "linuxserver/sonarr", State: "running"},
		{ID: "ghi789", Name: "nginx", Image: "nginx:latest", State: "exited"},
	}
	server.dockerCache.Store(&dockerVal)
	vmsVal := []dto.VMInfo{{ID: "1", Name: "Windows10", State: "running"},
		{ID: "2", Name: "Ubuntu", State: "shutoff"},
	}
	server.vmsCache.Store(&vmsVal)
	sharesVal := []dto.ShareInfo{{Name: "appdata", Used: new(uint64(50000000000))},
		{Name: "media", Used: new(uint64(4000000000000))},
	}
	server.sharesCache.Store(&sharesVal)
	server.upsCache.Store(&dto.UPSStatus{
		Connected:     true,
		Status:        "OL",
		Model:         "APC Smart-UPS 1500",
		BatteryCharge: new(100.0),
		LoadPercent:   new(25.0),
		RuntimeLeft:   new(3600),
		Timestamp:     time.Now(),
	})

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
	if !strings.Contains(body, "unraid_array_capacity_sample_timestamp_seconds 3600\n") {
		t.Error("Expected the array capacity sample timestamp")
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
	ctx := &domain.Context{Port: 8043}
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
	ctx := &domain.Context{Port: 8043}
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
	ctx := &domain.Context{Port: 8043}
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
			server.arrayCache.Store(&dto.ArrayStatus{
				State:      tt.state,
				TotalBytes: 1000,
				FreeBytes:  500,
			})

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

// TestRuntimeMetricsExposed verifies the Go runtime collectors are registered
// so go_goroutines and heap stats are scrapable for long-term leak monitoring.
// (process_* metrics are Linux-only, so they are not asserted here.)
func TestRuntimeMetricsExposed(t *testing.T) {
	ctx := &domain.Context{Port: 8043}
	server := NewServer(ctx)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()
	server.handleMetrics(w, req)

	body := w.Body.String()
	for _, want := range []string{"go_goroutines", "go_memstats_heap_inuse_bytes"} {
		if !strings.Contains(body, want) {
			t.Errorf("expected runtime metric %q in /metrics output", want)
		}
	}
}

func TestShareMetricsOptionalUsage(t *testing.T) {
	server := NewServer(&domain.Context{Port: 8043})
	scrape := func(shares []dto.ShareInfo) string {
		server.sharesCache.Store(&shares)
		rr := httptest.NewRecorder()
		server.handleMetrics(rr, httptest.NewRequest("GET", "/metrics", nil))
		if rr.Code != http.StatusOK {
			t.Fatalf("status=%d", rr.Code)
		}
		return rr.Body.String()
	}
	for _, tt := range []struct {
		name   string
		used   *uint64
		sample string
	}{
		{"measured", new(uint64(7)), "unraid_share_used_bytes{name=\"media\"} 7\n"},
		{"known zero", new(uint64(0)), "unraid_share_used_bytes{name=\"media\"} 0\n"},
		{"unknown", nil, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			body := scrape([]dto.ShareInfo{{Name: "media", Used: tt.used, StorageUsed: 40, StorageFree: 60, StorageTotal: 100}})
			if tt.sample == "" {
				if strings.Contains(body, "unraid_share_used_bytes{name=\"media\"}") {
					t.Error("unknown usage emitted")
				}
			} else if !strings.Contains(body, tt.sample) {
				t.Errorf("missing %s", tt.sample)
			}
			for _, want := range []string{"unraid_share_storage_used_bytes{name=\"media\"} 40\n", "unraid_share_storage_free_bytes{name=\"media\"} 60\n", "unraid_share_storage_total_bytes{name=\"media\"} 100\n"} {
				if !strings.Contains(body, want) {
					t.Errorf("missing %s", want)
				}
			}
		})
	}
	body := scrape([]dto.ShareInfo{})
	for _, metric := range []string{"unraid_share_used_bytes{", "unraid_share_storage_used_bytes{", "unraid_share_storage_free_bytes{", "unraid_share_storage_total_bytes{"} {
		if strings.Contains(body, metric) {
			t.Errorf("stale series: %s", metric)
		}
	}
}

func TestMetricsNullableMeasurements(t *testing.T) {
	server := newMetricsTestServer()
	gpu := []*dto.GPUMetrics{{Name: "nullable", Temperature: new(0.0), MemoryUsed: new(uint64(0)), MemoryTotal: new(uint64(0)), PowerDraw: new(0.0)}}
	disks := []dto.DiskInfo{{Name: "nullable", Device: "sda", Temperature: new(0.0)}}
	server.gpuCache.Store(&gpu)
	server.disksCache.Store(&disks)
	names := []string{"unraid_gpu_temperature_celsius", "unraid_gpu_memory_used_bytes", "unraid_gpu_memory_total_bytes", "unraid_gpu_power_watts", "unraid_disk_temperature_celsius"}
	body := getMetricsBody(t, server)
	for _, name := range names {
		if !strings.Contains(body, name+"{") {
			t.Errorf("measured zero missing for %s", name)
		}
	}
	gpu[0] = &dto.GPUMetrics{Name: "nullable"}
	disks[0].Temperature = nil
	body = getMetricsBody(t, server)
	for _, name := range names {
		if strings.Contains(body, name+"{") {
			t.Errorf("unavailable series remains for %s", name)
		}
	}
	// An inventory disappearing must clear the formerly available series too.
	gpu[0].Temperature = new(40.0)
	disks[0].Temperature = new(40.0)
	getMetricsBody(t, server)
	server.gpuCache.Store(nil)
	server.disksCache.Store(nil)
	body = getMetricsBody(t, server)
	for _, name := range names {
		if strings.Contains(body, name+"{") {
			t.Errorf("stale inventory series remains for %s", name)
		}
	}
}

func TestHealthNullableDiskTemperature(t *testing.T) {
	server := newMetricsTestServer()
	disks := []dto.DiskInfo{
		{Name: "standby", Device: "sdd", Role: "data", Status: "DISK_OK", SMARTStatus: "UNKNOWN", SpinState: "standby"},
		{Name: "hot", Device: "sdb", Role: "data", Status: "DISK_OK", SMARTStatus: "PASSED", Temperature: new(60.0)},
		{Name: "cool", Device: "sda", Role: "parity", Status: "DISK_OK", SMARTStatus: "PASSED", Temperature: new(32.0)},
	}
	server.disksCache.Store(&disks)
	health := server.GetHealthStatus()
	if health["healthy_disks"] != 1 || health["warning_disks"] != 1 || health["unknown_disks"] != 1 {
		t.Fatalf("health=%v", health)
	}
	report := BuildHealthReport(nil, &dto.ArrayStatus{State: "Started"}, disks, nil)
	for _, finding := range report.Findings {
		if strings.Contains(finding.Title, "standby") && strings.Contains(finding.Title, "temperature") {
			t.Errorf("unknown temperature caused finding: %+v", finding)
		}
	}
	if report.Warning != 1 {
		t.Errorf("expected one hot-disk warning: %+v", report)
	}
}
