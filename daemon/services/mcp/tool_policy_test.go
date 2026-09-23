package mcp

import (
	"context"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
)

func setupPolicyServer(t *testing.T, policies map[string]domain.ToolPolicyValue, readOnly bool) (*Server, *MockCacheProvider, *domain.ToolPolicyStore) {
	t.Helper()
	ctx := &domain.Context{
		Version:    "test-1.0.0",
		Port:       8043,
		ReadOnly:   readOnly,
		ToolPolicy: policies,
	}
	mock := newMockCacheProvider()
	server := NewServer(ctx, mock)
	store := domain.NewToolPolicyStore(t.TempDir(), policies)
	server.SetToolPolicyStore(store)

	if err := server.Initialize(); err != nil {
		t.Fatalf("failed to initialize MCP server: %v", err)
	}
	return server, mock, store
}

func TestToolPolicyHiddenHidesFromListAndCall(t *testing.T) {
	policies := map[string]domain.ToolPolicyValue{
		"system_reboot": domain.PolicyHidden,
		"list_disks":    domain.PolicyHidden,
	}
	server, _, _ := setupPolicyServer(t, policies, false)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	ctx := context.Background()
	toolsRes, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools failed: %v", err)
	}

	for _, tool := range toolsRes.Tools {
		if tool.Name == "system_reboot" || tool.Name == "list_disks" {
			t.Errorf("expected tool %q to be hidden from ListTools, but found it", tool.Name)
		}
	}

	// Calling a hidden tool should fail
	_, err = cs.CallTool(ctx, &mcp.CallToolParams{
		Name:      "system_reboot",
		Arguments: map[string]any{"confirm": true},
	})
	if err == nil {
		t.Errorf("expected CallTool for hidden tool 'system_reboot' to fail, but got nil err")
	}

	_, err = cs.CallTool(ctx, &mcp.CallToolParams{
		Name: "list_disks",
	})
	if err == nil {
		t.Errorf("expected CallTool for hidden tool 'list_disks' to fail, but got nil err")
	}
}

func TestToolPolicyReadOnlyBlocksWrite(t *testing.T) {
	policies := map[string]domain.ToolPolicyValue{
		"container_action": domain.PolicyReadOnly,
		"get_system_info":  domain.PolicyReadOnly,
	}
	server, _, _ := setupPolicyServer(t, policies, false)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	// Write tool with read_only policy should be blocked
	_, text := callToolJSON(t, cs, "container_action", map[string]any{
		"container_id": "abc123def456",
		"action":       "stop",
	})
	if !strings.Contains(text, "set to read-only") {
		t.Errorf("expected container_action to be blocked with read-only policy message, got: %s", text)
	}

	// Read tool with read_only policy should continue to work normally
	_, text = callToolJSON(t, cs, "get_system_info", nil)
	if !strings.Contains(text, "test-unraid") {
		t.Errorf("expected get_system_info to work normally under read_only policy, got: %s", text)
	}
}

func TestToolPolicyAllowBypassesConfirm(t *testing.T) {
	policies := map[string]domain.ToolPolicyValue{
		"system_reboot": domain.PolicyAllow,
	}
	server, _, _ := setupPolicyServer(t, policies, false)
	mockSys := &mockSystemController{}
	server.SetSystemController(mockSys)

	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	// Calling system_reboot with confirm=false should NOT be blocked by confirm gate when policy is 'allow'
	_, text := callToolJSON(t, cs, "system_reboot", map[string]any{"confirm": false})
	if strings.Contains(text, "requires confirm=true") || strings.Contains(text, "To proceed, call this tool again with confirm=true") {
		t.Errorf("expected confirm requirement to be bypassed when policy is 'allow', got: %s", text)
	}
	if !mockSys.rebootCalled {
		t.Errorf("expected Reboot to be called on system controller under PolicyAllow")
	}
}

func TestToolPolicyAskEnforcesConfirm(t *testing.T) {
	policies := map[string]domain.ToolPolicyValue{
		"container_action": domain.PolicyAsk,
	}
	server, _, _ := setupPolicyServer(t, policies, false)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	// container_action normally does not require confirm, but under policy 'ask' it must require confirm
	_, text := callToolJSON(t, cs, "container_action", map[string]any{
		"container_id": "abc123def456",
		"action":       "stop",
	})
	if !strings.Contains(text, "confirm=true") {
		t.Errorf("expected confirm requirement for policy 'ask' without confirm=true, got: %s", text)
	}
}

func TestToolPolicyGlobalReadOnlyOverridesAllowAndAsk(t *testing.T) {
	policies := map[string]domain.ToolPolicyValue{
		"system_reboot":    domain.PolicyAllow,
		"container_action": domain.PolicyAsk,
	}
	// readOnly = true overrides per-tool allow/ask
	server, _, _ := setupPolicyServer(t, policies, true)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	_, text := callToolJSON(t, cs, "system_reboot", map[string]any{"confirm": true})
	if !strings.Contains(text, "read-only mode") {
		t.Errorf("expected global read-only mode to override tool policy allow, got: %s", text)
	}

	_, text = callToolJSON(t, cs, "container_action", map[string]any{
		"container_id": "abc123def456",
		"action":       "stop",
		"confirm":      true,
	})
	if !strings.Contains(text, "read-only mode") {
		t.Errorf("expected global read-only mode to override tool policy ask, got: %s", text)
	}
}

func TestToolPolicyDynamicUpdate(t *testing.T) {
	policies := map[string]domain.ToolPolicyValue{
		"container_action": domain.PolicyAllow,
	}
	server, _, store := setupPolicyServer(t, policies, false)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	// Initially container_action is allow
	_, text := callToolJSON(t, cs, "container_action", map[string]any{
		"container_id": "abc123def456",
		"action":       "stop",
	})
	if strings.Contains(text, "set to read-only") {
		t.Fatalf("unexpected read-only block on initial allow: %s", text)
	}

	// Dynamically update policy to read_only
	store.Replace(map[string]domain.ToolPolicyValue{
		"container_action": domain.PolicyReadOnly,
	})

	// Now it should be blocked
	_, text = callToolJSON(t, cs, "container_action", map[string]any{
		"container_id": "abc123def456",
		"action":       "stop",
	})
	if !strings.Contains(text, "set to read-only") {
		t.Errorf("expected dynamic update to block container_action, got: %s", text)
	}
}
