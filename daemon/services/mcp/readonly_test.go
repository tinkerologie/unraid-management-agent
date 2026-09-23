package mcp

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
)

// setupReadOnlyServer creates an initialized MCP server with read-only mode enabled.
func setupReadOnlyServer(t *testing.T) (*Server, *MockCacheProvider) {
	t.Helper()
	ctx := &domain.Context{
		Version:  "test-1.0.0",
		Port:     8043,
		ReadOnly: true,
	}
	mock := newMockCacheProvider()
	server := NewServer(ctx, mock)
	if err := server.Initialize(); err != nil {
		t.Fatalf("failed to initialize MCP server: %v", err)
	}
	return server, mock
}

// TestReadOnlyModeBlocksWriteTools verifies that representative write tools
// from every register*Tools() group are rejected in read-only mode, even when
// the caller sets confirm=true.
func TestReadOnlyModeBlocksWriteTools(t *testing.T) {
	server, _ := setupReadOnlyServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	writeCalls := []struct {
		tool string
		args map[string]any
	}{
		// registerControlTools
		{"container_action", map[string]any{"container_id": "abc", "action": "stop"}},
		{"set_container_autostart", map[string]any{"container_id": "abc", "enabled": true}},
		{"vm_action", map[string]any{"vm_name": "vm1", "action": "stop"}},
		{"array_action", map[string]any{"action": "stop", "confirm": true}},
		{"system_reboot", map[string]any{"confirm": true}},
		{"system_shutdown", map[string]any{"confirm": true}},
		{"parity_check_action", map[string]any{}},
		{"disk_spin_down", map[string]any{"disk_id": "disk1"}},
		{"execute_user_script", map[string]any{"script_name": "test", "confirm": true}},
		// registerNewControlTools
		{"update_container", map[string]any{"container_id": "abc", "confirm": true}},
		{"create_vm_snapshot", map[string]any{"vm_name": "vm1", "snapshot_name": "snap1"}},
		{"delete_vm_snapshot", map[string]any{"vm_name": "vm1", "snapshot_name": "snap1", "confirm": true}},
		{"clear_disk_stats", map[string]any{}},
		{"collector_action", map[string]any{"collector_name": "docker", "action": "disable"}},
		{"update_collector_interval", map[string]any{"collector_name": "docker", "interval": 60}},
		{"remote_share_action", map[string]any{"source": "//server/share", "action": "mount", "confirm": true}},
		// registerNewMonitoringTools
		{"refresh_container_updates", map[string]any{}},
		// registerAlertingTools
		{"create_alert_rule", map[string]any{"id": "r1", "name": "rule 1", "expression": "CPU > 90"}},
		{"delete_alert_rule", map[string]any{"rule_id": "r1", "confirm": true}},
		// registerWatchdogTools
		{"create_health_check", map[string]any{"id": "hc1", "name": "hc 1", "type": "http", "target": "http://localhost"}},
		{"run_health_check", map[string]any{"check_id": "hc1"}},
		// registerFanControlTools / CPU / tuning
		{"set_fan_speed", map[string]any{"fan_id": "hwmon0_fan1", "pwm_percent": 50, "confirm": true}},
		{"set_fan_mode", map[string]any{"fan_id": "hwmon0_fan1", "mode": "automatic", "confirm": true}},
		{"set_fan_profile", map[string]any{"fan_id": "hwmon0_fan1", "profile_name": "quiet", "confirm": true}},
		{"create_fan_profile", map[string]any{"name": "p1", "curve_points": `[{"temp_celsius": 40, "speed_percent": 30}]`, "confirm": true}},
		{"restore_fan_defaults", map[string]any{"confirm": true}},
		{"set_cpu_governor", map[string]any{"governor": "performance", "confirm": true}},
		{"set_turbo_boost", map[string]any{"enabled": true, "confirm": true}},
		// registerAgentTools
		{"agent_start_session", map[string]any{"goal": "check status"}},
	}

	for _, tc := range writeCalls {
		t.Run(tc.tool, func(t *testing.T) {
			_, text := callToolJSON(t, cs, tc.tool, tc.args)
			if !strings.Contains(text, "read-only mode") {
				t.Errorf("expected %q to be blocked in read-only mode, got: %s", tc.tool, text)
			}
		})
	}
}

// TestReadOnlyModeAllowsReadTools verifies that monitoring tools keep working
// in read-only mode.
func TestReadOnlyModeAllowsReadTools(t *testing.T) {
	server, _ := setupReadOnlyServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	_, text := callToolJSON(t, cs, "get_system_info", nil)
	if !strings.Contains(text, "test-unraid") {
		t.Errorf("expected get_system_info to work in read-only mode, got: %s", text)
	}

	_, text = callToolJSON(t, cs, "get_array_status", nil)
	if strings.Contains(text, "read-only mode") {
		t.Errorf("read tool get_array_status must not be blocked, got: %s", text)
	}
}

// TestReadOnlyModeHealthReportNeverExecutes verifies the dual-mode
// system_health_report tool still returns a report but refuses to execute
// remediation actions in read-only mode.
func TestReadOnlyModeHealthReportNeverExecutes(t *testing.T) {
	server, _ := setupReadOnlyServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	_, text := callToolJSON(t, cs, "system_health_report", map[string]any{
		"confirm": true,
		"actions": []map[string]any{{"action": "restart_container", "target": "abc"}},
	})

	var resp struct {
		Executed bool   `json:"executed"`
		Note     string `json:"note"`
	}
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("expected JSON response, got: %s", text)
	}
	if resp.Executed {
		t.Error("system_health_report must not execute actions in read-only mode")
	}
	if !strings.Contains(resp.Note, "read-only mode") {
		t.Errorf("expected read-only note in response, got: %s", text)
	}
}

// TestReadOnlyModeRunbookForcesDryRun verifies run_runbook degrades to a
// dry-run in read-only mode even when confirm=true.
func TestReadOnlyModeRunbookForcesDryRun(t *testing.T) {
	server, _ := setupReadOnlyServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	_, text := callToolJSON(t, cs, "run_runbook", map[string]any{
		"name":    "restart_unhealthy_containers",
		"confirm": true,
	})

	var resp struct {
		Executed bool   `json:"executed"`
		Note     string `json:"note"`
	}
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("expected JSON dry-run response, got: %s", text)
	}
	if resp.Executed {
		t.Error("run_runbook must not execute in read-only mode")
	}
	if !strings.Contains(resp.Note, "read-only mode") {
		t.Errorf("expected read-only note in response, got: %s", text)
	}
}

// TestWriteToolsStillWorkWhenNotReadOnly guards against the wrapper blocking
// calls when read-only mode is disabled: the confirm gate must answer, not
// the read-only gate.
func TestWriteToolsStillWorkWhenNotReadOnly(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	toolsToTest := []struct {
		tool string
		args map[string]any
	}{
		{"array_action", map[string]any{"action": "stop", "confirm": false}},
		{"delete_vm_snapshot", map[string]any{"vm_name": "vm1", "snapshot_name": "snap1", "confirm": false}},
		{"set_fan_speed", map[string]any{"fan_id": "hwmon0_fan1", "pwm_percent": 50, "confirm": false}},
		{"remote_share_action", map[string]any{"source": "//server/share", "action": "mount", "confirm": false}},
		{"restore_fan_defaults", map[string]any{"confirm": false}},
	}

	for _, tt := range toolsToTest {
		t.Run(tt.tool, func(t *testing.T) {
			_, text := callToolJSON(t, cs, tt.tool, tt.args)
			if strings.Contains(text, "read-only mode") {
				t.Errorf("write tool %s blocked despite read-only mode being disabled: %s", tt.tool, text)
			}
			if !strings.Contains(text, "confirm") {
				t.Errorf("expected confirm gating message for %s, got: %s", tt.tool, text)
			}
		})
	}
}
