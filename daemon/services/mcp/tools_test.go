package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

// connectClientToServer creates an in-memory MCP client-server pair for testing.
// Returns the client session and a cleanup function.
func connectClientToServer(t *testing.T, server *Server) (*mcp.ClientSession, func()) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)

	ct, st := mcp.NewInMemoryTransports()

	_, err := server.mcpServer.Connect(ctx, st, nil)
	if err != nil {
		cancel()
		t.Fatalf("server connect failed: %v", err)
	}

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1.0"}, nil)
	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		cancel()
		t.Fatalf("client connect failed: %v", err)
	}

	return cs, cancel
}

func setupInitializedServer(t *testing.T) (*Server, *MockCacheProvider) {
	t.Helper()
	server, mock := setupTestMCPServer()
	if err := server.Initialize(); err != nil {
		t.Fatalf("failed to initialize MCP server: %v", err)
	}
	return server, mock
}

func callToolJSON(t *testing.T, cs *mcp.ClientSession, name string, args map[string]any) (*mcp.CallToolResult, string) {
	t.Helper()
	ctx := context.Background()
	result, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      name,
		Arguments: args,
	})
	if err != nil {
		t.Fatalf("CallTool(%q) error: %v", name, err)
	}
	if result == nil {
		t.Fatalf("CallTool(%q) returned nil result", name)
	}
	if len(result.Content) == 0 {
		t.Fatalf("CallTool(%q) returned empty content", name)
	}
	// Extract the text content
	text := ""
	for _, c := range result.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			text = tc.Text
			break
		}
	}
	return result, text
}

// ===== Monitoring Tools - Cache Populated =====

func TestToolGetSystemInfo(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	_, text := callToolJSON(t, cs, "get_system_info", nil)
	if !strings.Contains(text, "test-unraid") {
		t.Errorf("expected hostname in result, got: %s", text[:min(100, len(text))])
	}
}

func TestToolGetSystemInfoNil(t *testing.T) {
	ctx := &domain.Context{Config: domain.Config{Version: "test"}}
	mock := &MockCacheProvider{enabledCollectors: make(map[string]bool), collectorIntervals: make(map[string]int), collectorStatuses: map[string]*dto.CollectorStatus{}}
	server := NewServer(ctx, mock)
	if err := server.Initialize(); err != nil {
		t.Fatal(err)
	}
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	_, text := callToolJSON(t, cs, "get_system_info", nil)
	if !strings.Contains(text, "not available") {
		t.Errorf("expected 'not available' for nil cache, got: %s", text)
	}
}

func TestToolGetArrayStatus(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	_, text := callToolJSON(t, cs, "get_array_status", nil)
	if !strings.Contains(text, "Started") {
		t.Errorf("expected array state in result, got: %s", text[:min(100, len(text))])
	}
}

func TestToolListDisks(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	_, text := callToolJSON(t, cs, "list_disks", nil)
	if !strings.Contains(text, "disk1") {
		t.Errorf("expected disk1 in result, got: %s", text[:min(100, len(text))])
	}
}

func TestToolGetDiskInfo(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	t.Run("by device", func(t *testing.T) {
		_, text := callToolJSON(t, cs, "get_disk_info", map[string]any{"disk_id": "sda"})
		if !strings.Contains(text, "disk1") {
			t.Errorf("expected disk1, got: %s", text[:min(100, len(text))])
		}
	})

	t.Run("by name", func(t *testing.T) {
		_, text := callToolJSON(t, cs, "get_disk_info", map[string]any{"disk_id": "parity"})
		if !strings.Contains(text, "parity") {
			t.Errorf("expected parity disk, got: %s", text[:min(100, len(text))])
		}
	})

	t.Run("not found", func(t *testing.T) {
		_, text := callToolJSON(t, cs, "get_disk_info", map[string]any{"disk_id": "nonexistent"})
		if !strings.Contains(text, "not found") {
			t.Errorf("expected 'not found', got: %s", text)
		}
	})
}

func TestToolListShares(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	_, text := callToolJSON(t, cs, "list_shares", nil)
	if !strings.Contains(text, "appdata") {
		t.Errorf("expected appdata share, got: %s", text[:min(100, len(text))])
	}
}

func TestToolListContainers(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	t.Run("all", func(t *testing.T) {
		_, text := callToolJSON(t, cs, "list_containers", nil)
		if !strings.Contains(text, "plex") {
			t.Errorf("expected plex, got: %s", text[:min(100, len(text))])
		}
	})

	t.Run("running only", func(t *testing.T) {
		_, text := callToolJSON(t, cs, "list_containers", map[string]any{"state": "running"})
		if strings.Contains(text, "backup") {
			t.Error("filtered result should not contain exited container 'backup'")
		}
		if !strings.Contains(text, "plex") {
			t.Errorf("expected running container 'plex' in result")
		}
	})

	t.Run("stopped only", func(t *testing.T) {
		_, text := callToolJSON(t, cs, "list_containers", map[string]any{"state": "stopped"})
		if !strings.Contains(text, "backup") {
			t.Errorf("expected stopped container 'backup'")
		}
	})
}

func TestToolGetContainerInfo(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	t.Run("by name", func(t *testing.T) {
		_, text := callToolJSON(t, cs, "get_container_info", map[string]any{"container_id": "plex"})
		if !strings.Contains(text, "plex") {
			t.Errorf("expected plex, got: %s", text[:min(100, len(text))])
		}
	})

	t.Run("by id", func(t *testing.T) {
		_, text := callToolJSON(t, cs, "get_container_info", map[string]any{"container_id": "abc123"})
		if !strings.Contains(text, "plex") {
			t.Errorf("expected plex by ID, got: %s", text[:min(100, len(text))])
		}
	})

	t.Run("not found", func(t *testing.T) {
		_, text := callToolJSON(t, cs, "get_container_info", map[string]any{"container_id": "nonexistent"})
		if !strings.Contains(text, "not found") {
			t.Errorf("expected not found, got: %s", text)
		}
	})
}

func TestToolListVMs(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	t.Run("all", func(t *testing.T) {
		_, text := callToolJSON(t, cs, "list_vms", nil)
		if !strings.Contains(text, "Windows10") {
			t.Errorf("expected Windows10 VM")
		}
	})

	t.Run("running", func(t *testing.T) {
		_, text := callToolJSON(t, cs, "list_vms", map[string]any{"state": "running"})
		if !strings.Contains(text, "Windows10") {
			t.Error("expected running VM")
		}
		if strings.Contains(text, "shut off") {
			t.Error("should not contain stopped VMs")
		}
	})

	t.Run("stopped", func(t *testing.T) {
		_, text := callToolJSON(t, cs, "list_vms", map[string]any{"state": "stopped"})
		if !strings.Contains(text, "Ubuntu") {
			t.Error("expected stopped Ubuntu VM")
		}
	})
}

func TestToolGetVMInfo(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	t.Run("found", func(t *testing.T) {
		_, text := callToolJSON(t, cs, "get_vm_info", map[string]any{"vm_name": "Windows10"})
		if !strings.Contains(text, "Windows10") {
			t.Errorf("expected VM info")
		}
	})

	t.Run("not found", func(t *testing.T) {
		_, text := callToolJSON(t, cs, "get_vm_info", map[string]any{"vm_name": "nonexist"})
		if !strings.Contains(text, "not found") {
			t.Errorf("expected not found")
		}
	})
}

func TestToolGetUPSStatus(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	_, text := callToolJSON(t, cs, "get_ups_status", nil)
	if !strings.Contains(text, "APC") {
		t.Errorf("expected UPS model, got: %s", text[:min(100, len(text))])
	}
}

func TestToolGetGPUMetrics(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	_, text := callToolJSON(t, cs, "get_gpu_metrics", nil)
	if !strings.Contains(text, "NVIDIA") {
		t.Errorf("expected GPU info")
	}
}

func TestToolGetNetworkInfo(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	_, text := callToolJSON(t, cs, "get_network_info", nil)
	if !strings.Contains(text, "eth0") {
		t.Errorf("expected network info")
	}
}

func TestToolGetHardwareInfo(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	_, text := callToolJSON(t, cs, "get_hardware_info", nil)
	if !strings.Contains(text, "ASRock") {
		t.Errorf("expected hardware info")
	}
}

func TestToolGetRegistration(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	_, text := callToolJSON(t, cs, "get_registration", nil)
	if !strings.Contains(text, "Pro") {
		t.Errorf("expected registration type")
	}
}

func TestToolGetNotifications(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	_, text := callToolJSON(t, cs, "get_notifications", nil)
	if !strings.Contains(text, "Test Alert") {
		t.Errorf("expected notification")
	}
}

func TestToolGetZFSPools(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	t.Run("all pools", func(t *testing.T) {
		_, text := callToolJSON(t, cs, "get_zfs_pools", nil)
		if !strings.Contains(text, "tank") {
			t.Errorf("expected pool")
		}
	})

	t.Run("specific pool", func(t *testing.T) {
		_, text := callToolJSON(t, cs, "get_zfs_pools", map[string]any{"pool_name": "tank"})
		if !strings.Contains(text, "tank") {
			t.Errorf("expected specific pool")
		}
	})

	t.Run("pool not found", func(t *testing.T) {
		_, text := callToolJSON(t, cs, "get_zfs_pools", map[string]any{"pool_name": "nonexist"})
		if !strings.Contains(text, "not found") {
			t.Errorf("expected not found")
		}
	})
}

func TestToolGetZFSDatasets(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	_, text := callToolJSON(t, cs, "get_zfs_datasets", nil)
	if !strings.Contains(text, "tank/data") {
		t.Errorf("expected dataset")
	}
}

func TestToolGetZFSSnapshots(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	_, text := callToolJSON(t, cs, "get_zfs_snapshots", nil)
	if !strings.Contains(text, "backup1") {
		t.Errorf("expected snapshot")
	}
}

func TestToolGetZFSARCStats(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	_, text := callToolJSON(t, cs, "get_zfs_arc_stats", nil)
	if !strings.Contains(text, "99.9") {
		t.Errorf("expected ARC hit ratio")
	}
}

func TestToolGetUnassignedDevices(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	_, text := callToolJSON(t, cs, "get_unassigned_devices", nil)
	if !strings.Contains(text, "WD Blue") {
		t.Errorf("expected device model")
	}
}

func TestToolGetNUTStatus(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	_, text := callToolJSON(t, cs, "get_nut_status", nil)
	if !strings.Contains(text, "OL") {
		t.Errorf("expected NUT status")
	}
}

func TestToolGetParityHistory(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	_, text := callToolJSON(t, cs, "get_parity_history", nil)
	if !strings.Contains(text, "Parity-Check") {
		t.Errorf("expected parity history")
	}
}

func TestToolListLogFiles(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	_, text := callToolJSON(t, cs, "list_log_files", nil)
	if !strings.Contains(text, "syslog") {
		t.Errorf("expected log files")
	}
}

func TestToolGetLogContent(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	t.Run("with log file", func(t *testing.T) {
		_, text := callToolJSON(t, cs, "get_log_content", map[string]any{"log_file": "syslog"})
		if !strings.Contains(text, "Test log content") {
			t.Errorf("expected log content")
		}
	})

	t.Run("empty log file", func(t *testing.T) {
		_, text := callToolJSON(t, cs, "get_log_content", nil)
		if !strings.Contains(text, "required") {
			t.Errorf("expected 'required' error")
		}
	})
}

func TestToolGetSyslog(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	_, text := callToolJSON(t, cs, "get_syslog", nil)
	if !strings.Contains(text, "Test log content") {
		t.Errorf("expected syslog content")
	}
}

func TestToolGetDockerLog(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	_, text := callToolJSON(t, cs, "get_docker_log", nil)
	if !strings.Contains(text, "Test log content") {
		t.Errorf("expected docker log content")
	}
}

func TestToolListCollectors(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	_, text := callToolJSON(t, cs, "list_collectors", nil)
	if !strings.Contains(text, "system") {
		t.Errorf("expected collector list")
	}
}

func TestToolGetCollectorStatus(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	t.Run("found", func(t *testing.T) {
		_, text := callToolJSON(t, cs, "get_collector_status", map[string]any{"collector_name": "system"})
		if !strings.Contains(text, "system") {
			t.Errorf("expected collector status")
		}
	})

	t.Run("not found", func(t *testing.T) {
		_, text := callToolJSON(t, cs, "get_collector_status", map[string]any{"collector_name": "nonexist"})
		if !strings.Contains(text, "Failed") {
			t.Errorf("expected failure message")
		}
	})

	t.Run("empty name", func(t *testing.T) {
		_, text := callToolJSON(t, cs, "get_collector_status", nil)
		if !strings.Contains(text, "required") {
			t.Errorf("expected required message")
		}
	})
}

func TestToolGetSystemSettings(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	_, text := callToolJSON(t, cs, "get_system_settings", nil)
	if !strings.Contains(text, "Test-Unraid") {
		t.Errorf("expected server name")
	}
}

func TestToolGetDockerSettings(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	_, text := callToolJSON(t, cs, "get_docker_settings", nil)
	if !strings.Contains(text, "docker") {
		t.Errorf("expected docker settings")
	}
}

func TestToolGetVMSettings(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	_, text := callToolJSON(t, cs, "get_vm_settings", nil)
	if !strings.Contains(text, "true") {
		t.Errorf("expected VM enabled setting")
	}
}

func TestToolGetDiskSettings(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	_, text := callToolJSON(t, cs, "get_disk_settings", nil)
	if !strings.Contains(text, "xfs") {
		t.Errorf("expected disk settings")
	}
}

func TestToolGetShareConfig(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	t.Run("found", func(t *testing.T) {
		_, text := callToolJSON(t, cs, "get_share_config", map[string]any{"share_name": "appdata"})
		if !strings.Contains(text, "appdata") {
			t.Errorf("expected share config")
		}
	})

	t.Run("not found", func(t *testing.T) {
		_, text := callToolJSON(t, cs, "get_share_config", map[string]any{"share_name": "nonexist"})
		if !strings.Contains(text, "not found") {
			t.Errorf("expected not found")
		}
	})

	t.Run("empty name", func(t *testing.T) {
		_, text := callToolJSON(t, cs, "get_share_config", nil)
		if !strings.Contains(text, "required") {
			t.Errorf("expected required")
		}
	})
}

func TestToolGetNetworkAccessURLs(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	_, text := callToolJSON(t, cs, "get_network_access_urls", nil)
	if !strings.Contains(text, "192.168.1.100") {
		t.Errorf("expected URL in result")
	}
}

func TestToolGetHealthStatus(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	_, text := callToolJSON(t, cs, "get_health_status", nil)
	if !strings.Contains(text, "cpu_usage") {
		t.Errorf("expected health data")
	}
}

func TestToolGetNotificationsOverview(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	_, text := callToolJSON(t, cs, "get_notifications_overview", nil)
	// Overview may be nil if not populated; just verify no error
	if text == "" {
		t.Error("expected non-empty result")
	}
}

func TestToolSearchContainers(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	t.Run("by name", func(t *testing.T) {
		_, text := callToolJSON(t, cs, "search_containers", map[string]any{"query": "plex"})
		if !strings.Contains(text, "plex") {
			t.Errorf("expected plex in search results")
		}
	})

	t.Run("by image", func(t *testing.T) {
		_, text := callToolJSON(t, cs, "search_containers", map[string]any{"query": "sonarr"})
		if !strings.Contains(text, "sonarr") {
			t.Errorf("expected sonarr")
		}
	})

	t.Run("no matches", func(t *testing.T) {
		_, text := callToolJSON(t, cs, "search_containers", map[string]any{"query": "zzzzz"})
		if !strings.Contains(text, "No containers matching") {
			t.Errorf("expected no matches message")
		}
	})

	t.Run("by state", func(t *testing.T) {
		_, text := callToolJSON(t, cs, "search_containers", map[string]any{"query": "running"})
		if !strings.Contains(text, "plex") {
			t.Errorf("expected running containers")
		}
	})
}

func TestToolSearchVMs(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	t.Run("by name", func(t *testing.T) {
		_, text := callToolJSON(t, cs, "search_vms", map[string]any{"query": "Windows"})
		if !strings.Contains(text, "Windows10") {
			t.Errorf("expected Windows VM")
		}
	})

	t.Run("no matches", func(t *testing.T) {
		_, text := callToolJSON(t, cs, "search_vms", map[string]any{"query": "zzzzz"})
		if !strings.Contains(text, "No VMs matching") {
			t.Errorf("expected no matches")
		}
	})

	t.Run("by state", func(t *testing.T) {
		_, text := callToolJSON(t, cs, "search_vms", map[string]any{"query": "running"})
		if !strings.Contains(text, "Windows10") {
			t.Errorf("expected running VM")
		}
	})
}

func TestToolGetDiagnosticSummary(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	_, text := callToolJSON(t, cs, "get_diagnostic_summary", nil)
	// Verify it returns a JSON with expected sections
	var summary map[string]any
	if err := json.Unmarshal([]byte(text), &summary); err != nil {
		t.Fatalf("failed to unmarshal diagnostic summary: %v", err)
	}
	if _, ok := summary["system"]; !ok {
		t.Error("expected 'system' in diagnostic summary")
	}
	if _, ok := summary["array"]; !ok {
		t.Error("expected 'array' in diagnostic summary")
	}
	if _, ok := summary["stopped_containers"]; !ok {
		t.Error("expected 'stopped_containers' in diagnostic summary")
	}
	// backup container is exited, should appear in stopped_containers
	stoppedList, ok := summary["stopped_containers"].([]any)
	if !ok {
		t.Errorf("stopped_containers is not a list: %T", summary["stopped_containers"])
	} else if len(stoppedList) != 1 || stoppedList[0] != "backup" {
		t.Errorf("expected [backup], got %v", stoppedList)
	}
}

// ===== Monitoring tools with nil caches =====

func TestToolsNilCaches(t *testing.T) {
	ctx := &domain.Context{Config: domain.Config{Version: "test"}}
	mock := &MockCacheProvider{
		enabledCollectors:  make(map[string]bool),
		collectorIntervals: make(map[string]int),
		collectorStatuses:  map[string]*dto.CollectorStatus{},
	}
	server := NewServer(ctx, mock)
	if err := server.Initialize(); err != nil {
		t.Fatal(err)
	}
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	nilCacheTests := []struct {
		tool     string
		contains string
	}{
		{"get_system_info", "not available"},
		{"get_array_status", "not available"},
		{"list_disks", "not available"},
		{"list_shares", "not available"},
		{"list_containers", "not available"},
		{"get_ups_status", "not configured"},
		{"get_gpu_metrics", "No GPUs"},
		{"get_network_info", "not available"},
		{"get_hardware_info", "not available"},
		{"get_registration", "not available"},
		{"get_notifications", "not available"},
		{"get_zfs_pools", "No ZFS pools"},
		{"get_zfs_datasets", "No ZFS datasets"},
		{"get_zfs_snapshots", "No ZFS snapshots"},
		{"get_zfs_arc_stats", "not available"},
		{"get_unassigned_devices", "No unassigned"},
		{"get_nut_status", "not configured"},
		{"get_parity_history", "No parity"},
		{"list_log_files", "No log"},
		{"get_notifications_overview", "not available"},
	}

	for _, tt := range nilCacheTests {
		t.Run(tt.tool, func(t *testing.T) {
			_, text := callToolJSON(t, cs, tt.tool, nil)
			if !strings.Contains(text, tt.contains) {
				t.Errorf("expected %q in result, got: %s", tt.contains, text)
			}
		})
	}
}

// ===== Prompt Tests (registered as prompts, not tools) =====

func TestPromptSystemOverview(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	ctx := context.Background()
	result, err := cs.GetPrompt(ctx, &mcp.GetPromptParams{Name: "system_overview"})
	if err != nil {
		t.Fatalf("GetPrompt error: %v", err)
	}
	if len(result.Messages) == 0 {
		t.Error("expected messages in prompt result")
	}
}

func TestPromptDiagnoseDiskHealth(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	ctx := context.Background()
	result, err := cs.GetPrompt(ctx, &mcp.GetPromptParams{Name: "diagnose_disk_health"})
	if err != nil {
		t.Fatalf("GetPrompt error: %v", err)
	}
	if len(result.Messages) == 0 {
		t.Error("expected messages")
	}
	// Verify the prompt contains structured analysis instructions
	text := ""
	for _, msg := range result.Messages {
		if tc, ok := msg.Content.(*mcp.TextContent); ok {
			text = tc.Text
		}
	}
	if !strings.Contains(text, "SMART Status") {
		t.Error("expected prompt to mention SMART Status")
	}
	if !strings.Contains(text, "disk1") {
		t.Error("expected prompt to contain disk data")
	}
}

func TestPromptDiagnoseDiskHealthNilCache(t *testing.T) {
	server, mock := setupInitializedServer(t)
	mock.disks = nil
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	ctx := context.Background()
	result, err := cs.GetPrompt(ctx, &mcp.GetPromptParams{Name: "diagnose_disk_health"})
	if err != nil {
		t.Fatalf("GetPrompt error: %v", err)
	}
	if len(result.Messages) == 0 {
		t.Error("expected messages")
	}
	text := ""
	for _, msg := range result.Messages {
		if tc, ok := msg.Content.(*mcp.TextContent); ok {
			text = tc.Text
		}
	}
	if !strings.Contains(text, "not available") {
		t.Error("expected unavailable message when cache is nil")
	}
}

func TestPromptDiagnosePerformanceIssue(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	ctx := context.Background()
	result, err := cs.GetPrompt(ctx, &mcp.GetPromptParams{Name: "diagnose_performance_issue"})
	if err != nil {
		t.Fatalf("GetPrompt error: %v", err)
	}
	if len(result.Messages) == 0 {
		t.Error("expected messages")
	}
	text := ""
	for _, msg := range result.Messages {
		if tc, ok := msg.Content.(*mcp.TextContent); ok {
			text = tc.Text
		}
	}
	if !strings.Contains(text, "CPU Pressure") {
		t.Error("expected prompt to mention CPU Pressure")
	}
	if !strings.Contains(text, "Memory Pressure") {
		t.Error("expected prompt to mention Memory Pressure")
	}
	if !strings.Contains(text, "Docker Resource Usage") {
		t.Error("expected prompt to mention Docker Resource Usage")
	}
}

func TestPromptSuggestMaintenance(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	ctx := context.Background()
	result, err := cs.GetPrompt(ctx, &mcp.GetPromptParams{Name: "suggest_maintenance"})
	if err != nil {
		t.Fatalf("GetPrompt error: %v", err)
	}
	if len(result.Messages) == 0 {
		t.Error("expected messages")
	}
	text := ""
	for _, msg := range result.Messages {
		if tc, ok := msg.Content.(*mcp.TextContent); ok {
			text = tc.Text
		}
	}
	if !strings.Contains(text, "Parity Check Status") {
		t.Error("expected prompt to mention Parity Check Status")
	}
	if !strings.Contains(text, "Disk Health") {
		t.Error("expected prompt to mention Disk Health")
	}
	if !strings.Contains(text, "Critical") {
		t.Error("expected prompt to mention priority levels")
	}
}

func TestPromptExplainArrayState(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	ctx := context.Background()
	result, err := cs.GetPrompt(ctx, &mcp.GetPromptParams{Name: "explain_array_state"})
	if err != nil {
		t.Fatalf("GetPrompt error: %v", err)
	}
	if len(result.Messages) == 0 {
		t.Error("expected messages")
	}
	text := ""
	for _, msg := range result.Messages {
		if tc, ok := msg.Content.(*mcp.TextContent); ok {
			text = tc.Text
		}
	}
	if !strings.Contains(text, "Array State") {
		t.Error("expected prompt to mention Array State")
	}
	if !strings.Contains(text, "Parity Status") {
		t.Error("expected prompt to mention Parity Status")
	}
	if !strings.Contains(text, "Data Protection") {
		t.Error("expected prompt to mention Data Protection Level")
	}
}

func TestPromptTroubleshootIssue(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	ctx := context.Background()
	result, err := cs.GetPrompt(ctx, &mcp.GetPromptParams{Name: "troubleshoot_issue"})
	if err != nil {
		t.Fatalf("GetPrompt error: %v", err)
	}
	if len(result.Messages) == 0 {
		t.Error("expected messages")
	}
}

// ===== Collector management tools =====

func TestToolCollectorAction(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	t.Run("enable", func(t *testing.T) {
		_, text := callToolJSON(t, cs, "collector_action", map[string]any{
			"collector_name": "docker",
			"action":         "enable",
		})
		if !strings.Contains(text, "success") {
			t.Errorf("expected success, got: %s", text)
		}
	})

	t.Run("disable", func(t *testing.T) {
		_, text := callToolJSON(t, cs, "collector_action", map[string]any{
			"collector_name": "docker",
			"action":         "disable",
		})
		if !strings.Contains(text, "success") {
			t.Errorf("expected success, got: %s", text)
		}
	})

	t.Run("disable required", func(t *testing.T) {
		_, text := callToolJSON(t, cs, "collector_action", map[string]any{
			"collector_name": "system",
			"action":         "disable",
		})
		if !strings.Contains(text, "cannot disable") && !strings.Contains(text, "required") && !strings.Contains(text, "Failed") {
			t.Errorf("expected error for disabling required collector, got: %s", text)
		}
	})
}

func TestToolUpdateCollectorInterval(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	t.Run("valid", func(t *testing.T) {
		_, text := callToolJSON(t, cs, "update_collector_interval", map[string]any{
			"collector_name": "docker",
			"interval":       float64(30),
		})
		if !strings.Contains(text, "success") {
			t.Errorf("expected success, got: %s", text)
		}
	})

	t.Run("unknown collector", func(t *testing.T) {
		_, text := callToolJSON(t, cs, "update_collector_interval", map[string]any{
			"collector_name": "nonexist",
			"interval":       float64(30),
		})
		if !strings.Contains(text, "Failed") && !strings.Contains(text, "unknown") {
			t.Errorf("expected error for unknown collector")
		}
	})
}

// ===== Resource and prompt tests =====

func TestToolListTools(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	ctx := context.Background()
	result, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools error: %v", err)
	}
	if len(result.Tools) == 0 {
		t.Error("expected tools to be registered")
	}
	// Verify at least some key tools are registered
	toolNames := make(map[string]bool)
	for _, tool := range result.Tools {
		toolNames[tool.Name] = true
	}
	expected := []string{
		"get_system_info", "get_array_status", "list_disks", "list_containers",
		"list_vms", "get_health_status", "get_diagnostic_summary",
	}
	for _, name := range expected {
		if !toolNames[name] {
			t.Errorf("expected tool %q to be registered", name)
		}
	}
}

func TestToolListResources(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	ctx := context.Background()
	result, err := cs.ListResources(ctx, nil)
	if err != nil {
		t.Fatalf("ListResources error: %v", err)
	}
	if len(result.Resources) == 0 {
		t.Error("expected resources to be registered")
	}
}

func TestToolListPrompts(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	ctx := context.Background()
	result, err := cs.ListPrompts(ctx, nil)
	if err != nil {
		t.Fatalf("ListPrompts error: %v", err)
	}
	if len(result.Prompts) == 0 {
		t.Error("expected prompts to be registered")
	}
}

// ===== Resource Read Tests =====

func TestResourceReadSystem(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	ctx := context.Background()
	result, err := cs.ReadResource(ctx, &mcp.ReadResourceParams{URI: "unraid://system"})
	if err != nil {
		t.Fatalf("ReadResource error: %v", err)
	}
	if result == nil || len(result.Contents) == 0 {
		t.Fatal("expected resource contents")
	}
	text := result.Contents[0].Text
	if !strings.Contains(text, "hostname") {
		t.Errorf("expected system info JSON, got: %s", text[:min(len(text), 100)])
	}
}

func TestResourceReadSystemNilCache(t *testing.T) {
	server, mock := setupInitializedServer(t)
	mock.systemInfo = nil
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	ctx := context.Background()
	result, err := cs.ReadResource(ctx, &mcp.ReadResourceParams{URI: "unraid://system"})
	if err != nil {
		t.Fatalf("ReadResource error: %v", err)
	}
	if !strings.Contains(result.Contents[0].Text, "not available") {
		t.Errorf("expected not available message, got: %s", result.Contents[0].Text)
	}
}

func TestResourceReadArray(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	ctx := context.Background()
	result, err := cs.ReadResource(ctx, &mcp.ReadResourceParams{URI: "unraid://array"})
	if err != nil {
		t.Fatalf("ReadResource error: %v", err)
	}
	if result == nil || len(result.Contents) == 0 {
		t.Fatal("expected resource contents")
	}
	text := result.Contents[0].Text
	if !strings.Contains(text, "state") {
		t.Errorf("expected array status JSON, got: %s", text[:min(len(text), 100)])
	}
}

func TestResourceReadArrayNilCache(t *testing.T) {
	server, mock := setupInitializedServer(t)
	mock.arrayStatus = nil
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	ctx := context.Background()
	result, err := cs.ReadResource(ctx, &mcp.ReadResourceParams{URI: "unraid://array"})
	if err != nil {
		t.Fatalf("ReadResource error: %v", err)
	}
	if !strings.Contains(result.Contents[0].Text, "not available") {
		t.Errorf("expected not available message")
	}
}

func TestResourceReadContainers(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	ctx := context.Background()
	result, err := cs.ReadResource(ctx, &mcp.ReadResourceParams{URI: "unraid://containers"})
	if err != nil {
		t.Fatalf("ReadResource error: %v", err)
	}
	text := result.Contents[0].Text
	if !strings.Contains(text, "plex") {
		t.Errorf("expected container data, got: %s", text[:min(len(text), 100)])
	}
}

func TestResourceReadContainersNilCache(t *testing.T) {
	server, mock := setupInitializedServer(t)
	mock.containers = nil
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	ctx := context.Background()
	result, err := cs.ReadResource(ctx, &mcp.ReadResourceParams{URI: "unraid://containers"})
	if err != nil {
		t.Fatalf("ReadResource error: %v", err)
	}
	if !strings.Contains(result.Contents[0].Text, "not available") {
		t.Errorf("expected not available message")
	}
}

func TestResourceReadVMs(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	ctx := context.Background()
	result, err := cs.ReadResource(ctx, &mcp.ReadResourceParams{URI: "unraid://vms"})
	if err != nil {
		t.Fatalf("ReadResource error: %v", err)
	}
	text := result.Contents[0].Text
	if !strings.Contains(text, "Windows") {
		t.Errorf("expected VM data, got: %s", text[:min(len(text), 100)])
	}
}

func TestResourceReadVMsNilCache(t *testing.T) {
	server, mock := setupInitializedServer(t)
	mock.vms = nil
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	ctx := context.Background()
	result, err := cs.ReadResource(ctx, &mcp.ReadResourceParams{URI: "unraid://vms"})
	if err != nil {
		t.Fatalf("ReadResource error: %v", err)
	}
	if !strings.Contains(result.Contents[0].Text, "not available") {
		t.Errorf("expected not available message")
	}
}

func TestResourceReadDisks(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	ctx := context.Background()
	result, err := cs.ReadResource(ctx, &mcp.ReadResourceParams{URI: "unraid://disks"})
	if err != nil {
		t.Fatalf("ReadResource error: %v", err)
	}
	text := result.Contents[0].Text
	if !strings.Contains(text, "sda") {
		t.Errorf("expected disk data, got: %s", text[:min(len(text), 100)])
	}
}

func TestResourceReadDisksNilCache(t *testing.T) {
	server, mock := setupInitializedServer(t)
	mock.disks = nil
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	ctx := context.Background()
	result, err := cs.ReadResource(ctx, &mcp.ReadResourceParams{URI: "unraid://disks"})
	if err != nil {
		t.Fatalf("ReadResource error: %v", err)
	}
	if !strings.Contains(result.Contents[0].Text, "not available") {
		t.Errorf("expected not available message")
	}
}
