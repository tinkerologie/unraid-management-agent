package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/tinkerologie/unraid-management-agent/daemon/domain"
)

// setupNetworkLockedServer creates an initialized MCP server as the HTTP boot
// path does when no API token is configured (#245).
func setupNetworkLockedServer(t *testing.T) *Server {
	t.Helper()
	server := NewServer(&domain.Context{Version: "test-1.0.0", Port: 8043}, newMockCacheProvider())
	server.SetNetworkWriteLock(true)
	if err := server.Initialize(); err != nil {
		t.Fatalf("failed to initialize MCP server: %v", err)
	}
	return server
}

// TestNetworkWriteLockBlocksWriteTools verifies write tools are refused with
// the token message, even with confirm=true.
func TestNetworkWriteLockBlocksWriteTools(t *testing.T) {
	cs, cleanup := connectClientToServer(t, setupNetworkLockedServer(t))
	defer cleanup()

	writeCalls := []struct {
		tool string
		args map[string]any
	}{
		{"container_action", map[string]any{"container_id": "abc", "action": "stop"}},
		{"system_reboot", map[string]any{"confirm": true}},
		{"execute_user_script", map[string]any{"script_name": "test", "confirm": true}},
		{"set_fan_speed", map[string]any{"fan_id": "hwmon0_fan1", "pwm_percent": 50, "confirm": true}},
		{"agent_start_session", map[string]any{"goal": "check status"}},
	}
	for _, tc := range writeCalls {
		t.Run(tc.tool, func(t *testing.T) {
			_, text := callToolJSON(t, cs, tc.tool, tc.args)
			if text != writesLockedBlockedMessage {
				t.Errorf("expected token lock message for %s, got: %s", tc.tool, text)
			}
		})
	}
}

// TestNetworkWriteLockAllowsReadTools verifies monitoring keeps working.
func TestNetworkWriteLockAllowsReadTools(t *testing.T) {
	cs, cleanup := connectClientToServer(t, setupNetworkLockedServer(t))
	defer cleanup()

	_, text := callToolJSON(t, cs, "get_system_info", nil)
	if !strings.Contains(text, "test-unraid") {
		t.Errorf("expected get_system_info to work under the write lock, got: %s", text)
	}
}

// TestNetworkWriteLockDualModeTools verifies the report and runbook tools
// return their read-only results and never execute.
func TestNetworkWriteLockDualModeTools(t *testing.T) {
	cs, cleanup := connectClientToServer(t, setupNetworkLockedServer(t))
	defer cleanup()

	calls := []struct {
		tool string
		args map[string]any
	}{
		{"system_health_report", map[string]any{
			"confirm": true,
			"actions": []map[string]any{{"action": "restart_container", "target": "abc"}},
		}},
		{"run_runbook", map[string]any{"name": "restart_unhealthy_containers", "confirm": true}},
	}
	for _, tc := range calls {
		t.Run(tc.tool, func(t *testing.T) {
			_, text := callToolJSON(t, cs, tc.tool, tc.args)
			var resp struct {
				Executed bool   `json:"executed"`
				Note     string `json:"note"`
			}
			if err := json.Unmarshal([]byte(text), &resp); err != nil {
				t.Fatalf("expected JSON response, got: %s", text)
			}
			if resp.Executed {
				t.Errorf("%s must not execute under the write lock", tc.tool)
			}
			if resp.Note != writesLockedBlockedMessage {
				t.Errorf("expected token lock note, got: %q", resp.Note)
			}
		})
	}
}

// TestNetworkWriteLockOffKeepsConfirmGate verifies that without the lock (the
// STDIO path never sets it) write tools reach their normal confirm gate.
func TestNetworkWriteLockOffKeepsConfirmGate(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	_, text := callToolJSON(t, cs, "system_reboot", map[string]any{"confirm": false})
	if text == writesLockedBlockedMessage {
		t.Fatal("write tool blocked although the network write lock is off")
	}
	if !strings.Contains(text, "confirm") {
		t.Errorf("expected confirm gating message, got: %s", text)
	}
}

// TestNetworkWriteLockKeepsHiddenTools verifies a hidden tool is still unknown
// under the lock rather than being reported as read-only.
func TestNetworkWriteLockKeepsHiddenTools(t *testing.T) {
	server, _, _ := setupPolicyServer(t, map[string]domain.ToolPolicyValue{"system_reboot": domain.PolicyHidden}, false)
	server.SetNetworkWriteLock(true)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	ctx := context.Background()
	listed, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools failed: %v", err)
	}
	names := make(map[string]bool, len(listed.Tools))
	for _, tool := range listed.Tools {
		names[tool.Name] = true
	}
	if names["system_reboot"] {
		t.Error("hidden tool system_reboot must not be listed under the write lock")
	}
	// Blocked write tools stay visible, as in read-only mode.
	if !names["container_action"] {
		t.Error("write tool container_action must stay listed under the write lock")
	}

	if _, err := cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "system_reboot",
		Arguments: map[string]any{"confirm": true},
	}); err == nil {
		t.Error("expected hidden tool system_reboot to be rejected as unknown")
	}
}

// TestNetworkWriteLockReadOnlyModeMessage verifies explicit read-only mode
// keeps its own message when both apply.
func TestNetworkWriteLockReadOnlyModeMessage(t *testing.T) {
	server, _ := setupReadOnlyServer(t)
	server.SetNetworkWriteLock(true)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	_, text := callToolJSON(t, cs, "system_reboot", map[string]any{"confirm": true})
	if text != readOnlyBlockedMessage {
		t.Errorf("expected read-only mode message, got: %s", text)
	}
}

// TestNetworkWriteLockWithoutPolicyStore covers effectivePolicy's fallback
// when no tool policy store is configured. NewServer always creates a store,
// so an initialized client/server cannot reach this branch; the client/server
// behaviour of the lock is covered by the tests above.
func TestNetworkWriteLockWithoutPolicyStore(t *testing.T) {
	s := &Server{ctx: &domain.Context{}}
	if got := s.effectivePolicy("system_reboot"); got != domain.PolicyDefault {
		t.Errorf("unlocked effectivePolicy = %q, want %q", got, domain.PolicyDefault)
	}
	s.SetNetworkWriteLock(true)
	if got := s.effectivePolicy("system_reboot"); got != domain.PolicyReadOnly {
		t.Errorf("locked effectivePolicy = %q, want %q", got, domain.PolicyReadOnly)
	}
}
