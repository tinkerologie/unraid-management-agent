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

func newMetricsTestServer() *Server {
	ctx := &domain.Context{
		Config: domain.Config{Port: 8043},
	}
	return NewServer(ctx)
}

func getMetricsBody(t *testing.T, server *Server) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()
	server.handleMetrics(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", w.Code)
	}
	return w.Body.String()
}

func TestMetricsGPUCache(t *testing.T) {
	server := newMetricsTestServer()
	server.cacheMutex.Lock()
	server.gpuCache = []*dto.GPUMetrics{
		{
			Available:      true,
			Name:           "NVIDIA RTX 3080",
			Temperature:    65.0,
			UtilizationGPU: 85.0,
			MemoryUsed:     6442450944,
			MemoryTotal:    10737418240,
			PowerDraw:      250.5,
			Timestamp:      time.Now(),
		},
		nil, // nil entry — exercises the nil guard
		{
			Available:      true,
			Name:           "Intel UHD 630",
			Temperature:    45.0,
			UtilizationGPU: 10.0,
			MemoryUsed:     256000000,
			MemoryTotal:    1024000000,
			PowerDraw:      15.0,
			Timestamp:      time.Now(),
		},
	}
	server.cacheMutex.Unlock()

	body := getMetricsBody(t, server)

	checks := []struct {
		name   string
		needle string
	}{
		{"gpu temp label", `name="NVIDIA RTX 3080"`},
		{"gpu temp metric", "unraid_gpu_temperature_celsius"},
		{"gpu utilization", "unraid_gpu_utilization_percent"},
		{"gpu memory used", "unraid_gpu_memory_used_bytes"},
		{"gpu memory total", "unraid_gpu_memory_total_bytes"},
		{"gpu power", "unraid_gpu_power_watts"},
		{"second gpu label", `name="Intel UHD 630"`},
	}
	for _, c := range checks {
		if !strings.Contains(body, c.needle) {
			t.Errorf("%s: expected %q in metrics output", c.name, c.needle)
		}
	}
}

func TestMetricsCPUPowerRAPL_Present(t *testing.T) {
	server := newMetricsTestServer()
	cpuPower := 65.5
	dramPower := 5.2
	server.cacheMutex.Lock()
	server.systemCache = &dto.SystemInfo{
		Hostname:       "tower",
		Version:        "7.0",
		AgentVersion:   "2025.12.01",
		CPUPowerWatts:  &cpuPower,
		DRAMPowerWatts: &dramPower,
		Timestamp:      time.Now(),
	}
	server.cacheMutex.Unlock()

	body := getMetricsBody(t, server)

	if !strings.Contains(body, "unraid_cpu_power_watts 65.5") {
		t.Error("Expected unraid_cpu_power_watts 65.5")
	}
	if !strings.Contains(body, "unraid_dram_power_watts 5.2") {
		t.Error("Expected unraid_dram_power_watts 5.2")
	}
}

func TestMetricsCPUPowerRAPL_Nil(t *testing.T) {
	server := newMetricsTestServer()
	server.cacheMutex.Lock()
	server.systemCache = &dto.SystemInfo{
		Hostname:       "tower",
		Version:        "7.0",
		AgentVersion:   "2025.12.01",
		CPUPowerWatts:  nil,
		DRAMPowerWatts: nil,
		Timestamp:      time.Now(),
	}
	server.cacheMutex.Unlock()

	body := getMetricsBody(t, server)

	if !strings.Contains(body, "unraid_cpu_power_watts 0") {
		t.Error("Expected unraid_cpu_power_watts 0 when RAPL unavailable")
	}
	if !strings.Contains(body, "unraid_dram_power_watts 0") {
		t.Error("Expected unraid_dram_power_watts 0 when RAPL unavailable")
	}
}

func TestMetricsDiskSMARTFailed(t *testing.T) {
	server := newMetricsTestServer()
	server.cacheMutex.Lock()
	server.disksCache = []dto.DiskInfo{
		{
			Name:        "disk1",
			Device:      "sda",
			Role:        "data",
			Status:      "DISK_OK",
			SMARTStatus: "FAILED",
			SpinState:   "active",
			Timestamp:   time.Now(),
		},
	}
	server.cacheMutex.Unlock()

	body := getMetricsBody(t, server)

	// SMART FAILED should produce value 0
	if !strings.Contains(body, `unraid_disk_smart_status{device="sda",disk="disk1"} 0`) {
		t.Error("Expected unraid_disk_smart_status 0 for FAILED SMART")
	}
}

func TestMetricsDiskCachePoolType(t *testing.T) {
	server := newMetricsTestServer()
	server.cacheMutex.Lock()
	server.disksCache = []dto.DiskInfo{
		{
			Name:        "cache",
			Device:      "nvme0n1",
			Role:        "cache",
			Status:      "DISK_OK",
			Temperature: 40.0,
			SMARTStatus: "PASSED",
			SpinState:   "active",
		},
		{
			Name:        "pool1",
			Device:      "nvme1n1",
			Role:        "pool",
			Status:      "DISK_OK",
			Temperature: 38.0,
			SMARTStatus: "PASSED",
			SpinState:   "active",
		},
	}
	server.cacheMutex.Unlock()

	body := getMetricsBody(t, server)

	// cache and pool roles should have type="SSD" in temperature metric
	if !strings.Contains(body, `device="nvme0n1"`) {
		t.Error("Expected nvme0n1 device in metrics")
	}
	if !strings.Contains(body, `device="nvme1n1"`) {
		t.Error("Expected nvme1n1 device in metrics")
	}
	// The type label appears on the temperature metric
	if !strings.Contains(body, `type="SSD"`) {
		t.Error("Expected type=SSD label for cache/pool disks")
	}
}

func TestMetricsDiskProblemStatus(t *testing.T) {
	server := newMetricsTestServer()
	server.cacheMutex.Lock()
	server.disksCache = []dto.DiskInfo{
		{
			Name:        "disk2",
			Device:      "sdb",
			Role:        "data",
			Status:      "DISK_DSBL",
			SMARTStatus: "PASSED",
			SpinState:   "active",
		},
	}
	server.cacheMutex.Unlock()

	body := getMetricsBody(t, server)

	if !strings.Contains(body, `unraid_disk_status{device="sdb",disk="disk2",status="DISK_DSBL"} 0`) {
		t.Error("Expected unraid_disk_status 0 for disabled disk")
	}
}

func TestMetricsUPSOnBattery(t *testing.T) {
	server := newMetricsTestServer()
	server.cacheMutex.Lock()
	server.upsCache = &dto.UPSStatus{
		Status:        "OB",
		Model:         "APC Back-UPS 600",
		BatteryCharge: 85.0,
		LoadPercent:   40.0,
		RuntimeLeft:   1200,
		Timestamp:     time.Now(),
	}
	server.cacheMutex.Unlock()

	body := getMetricsBody(t, server)

	// OB (On Battery) is neither ONLINE nor OL, so status should be 0
	if !strings.Contains(body, `unraid_ups_status{model="APC Back-UPS 600",name="ups"} 0`) {
		t.Error("Expected unraid_ups_status 0 for OB (on battery)")
	}
}

func TestMetricsParityCheckRunning(t *testing.T) {
	server := newMetricsTestServer()
	server.cacheMutex.Lock()
	server.arrayCache = &dto.ArrayStatus{
		State:               "Started",
		ParityCheckStatus:   "RUNNING",
		ParityCheckProgress: 45.5,
		ParityValid:         true,
		TotalBytes:          8000000000000,
		FreeBytes:           3000000000000,
	}
	server.cacheMutex.Unlock()

	body := getMetricsBody(t, server)

	if !strings.Contains(body, "unraid_parity_check_running 1") {
		t.Error("Expected unraid_parity_check_running 1 for RUNNING status")
	}
	if !strings.Contains(body, "unraid_parity_check_progress 45.5") {
		t.Error("Expected unraid_parity_check_progress 45.5")
	}
}

func TestMetricsParityInvalid(t *testing.T) {
	server := newMetricsTestServer()
	server.cacheMutex.Lock()
	server.arrayCache = &dto.ArrayStatus{
		State:       "Started",
		ParityValid: false,
		TotalBytes:  8000000000000,
		FreeBytes:   3000000000000,
	}
	server.cacheMutex.Unlock()

	body := getMetricsBody(t, server)

	if !strings.Contains(body, "unraid_parity_valid 0") {
		t.Error("Expected unraid_parity_valid 0")
	}
}

func TestMetricsDiskStandby(t *testing.T) {
	server := newMetricsTestServer()
	server.cacheMutex.Lock()
	server.disksCache = []dto.DiskInfo{
		{
			Name:        "disk1",
			Device:      "sda",
			Role:        "data",
			Status:      "DISK_OK",
			SMARTStatus: "PASSED",
			SpinState:   "standby",
		},
		{
			Name:        "disk2",
			Device:      "sdb",
			Role:        "data",
			Status:      "DISK_OK",
			SMARTStatus: "PASSED",
			SpinState:   "active",
		},
	}
	server.cacheMutex.Unlock()

	body := getMetricsBody(t, server)

	if !strings.Contains(body, `unraid_disk_standby{device="sda",disk="disk1"} 1`) {
		t.Error("Expected disk1 standby=1")
	}
	if !strings.Contains(body, `unraid_disk_standby{device="sdb",disk="disk2"} 0`) {
		t.Error("Expected disk2 standby=0")
	}
}
