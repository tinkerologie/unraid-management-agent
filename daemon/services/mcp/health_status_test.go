package mcp

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/tinkerologie/unraid-management-agent/daemon/constants"
	"github.com/tinkerologie/unraid-management-agent/daemon/domain"
	"github.com/tinkerologie/unraid-management-agent/daemon/dto"
	"github.com/tinkerologie/unraid-management-agent/daemon/services/api"
)

// TestGetHealthStatusRealProviderDiskCounts sends the disk inventory from
// issue #344 through the API cache to get_health_status.
func TestGetHealthStatusRealProviderDiskCounts(t *testing.T) {
	ctx := &domain.Context{Config: domain.Config{Version: "test-1.0.0", Port: 8043}, Hub: domain.NewEventBus(64)}
	apiServer := api.NewServer(ctx)
	t.Cleanup(apiServer.Stop)
	apiServer.StartSubscriptions()
	select {
	case <-apiServer.Ready():
	case <-time.After(5 * time.Second):
		t.Fatal("API subscriptions did not become ready")
	}

	disks := []dto.DiskInfo{
		{ID: "WDC_WD140EDGZ_7LGGXR5K", Device: "sdb", Name: "parity", Role: "parity", Status: "DISK_OK", SMARTStatus: "PASSED", Temperature: new(32.0)},
		{ID: "WDC_WD140EDGZ_7LGH9KAK", Device: "sda", Name: "disk1", Role: "data", Status: "DISK_OK", SMARTStatus: "PASSED", Temperature: new(33.0)},
		{ID: "WDC_WD140EDGZ_9MJDHH8T", Device: "sde", Name: "disk2", Role: "data", Status: "DISK_OK", SMARTStatus: "PASSED", Temperature: new(33.0)},
		{ID: "WDC_WD140EDGZ_7LGHGJXK", Device: "sdd", Name: "disk3", Role: "data", Status: "DISK_OK", SMARTStatus: "UNKNOWN", SpinState: "standby"},
		{Name: "parity2", Role: "parity2", Status: "DISK_NP_DSBL"},
		{ID: "CT2000T500SSD8_25285175763C", Device: "nvme0n1", Name: "cache", Role: "cache", Status: "DISK_OK", SMARTStatus: "PASSED", Temperature: new(38.0)},
		{ID: "Samsung_Flash_Drive_FIT", Device: "sdc", Name: "flash", Role: "flash", Status: "DISK_OK", SMARTStatus: "UNKNOWN"},
		{ID: "docker_vdisk", Name: "Docker vDisk", Role: "docker_vdisk", Status: "DISK_OK"},
		{ID: "log_filesystem", Device: "tmpfs", Name: "Log", Role: "log", Status: "DISK_OK"},
	}
	domain.Publish(ctx.Hub, constants.TopicDiskListUpdate, disks)
	deadline := time.Now().Add(5 * time.Second)
	for len(apiServer.GetDisksCache()) != len(disks) {
		if time.Now().After(deadline) {
			t.Fatal("disk snapshot was not installed in the API cache")
		}
		time.Sleep(2 * time.Millisecond)
	}

	server := NewServer(ctx, apiServer)
	if err := server.Initialize(); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()
	_, text := callToolJSON(t, cs, "get_health_status", nil)
	var counts struct {
		HealthyDisks *int `json:"healthy_disks"`
		WarningDisks *int `json:"warning_disks"`
		UnknownDisks *int `json:"unknown_disks"`
	}
	if err := json.Unmarshal([]byte(text), &counts); err != nil {
		t.Fatal(err)
	}
	if counts.HealthyDisks == nil || *counts.HealthyDisks != 4 ||
		counts.WarningDisks == nil || *counts.WarningDisks != 0 ||
		counts.UnknownDisks == nil || *counts.UnknownDisks != 1 {
		t.Errorf("want healthy_disks=4 warning_disks=0 unknown_disks=1, got %s", text)
	}
}
