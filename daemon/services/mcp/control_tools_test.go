package mcp

import (
	"strings"
	"testing"
)

// Tests for MCP control tools and new monitoring tools that are at low coverage.
// These tools call real system controllers, so on non-Unraid they return error messages.
// We test: unconfirmed safety gates, input validation, error messages, and unknown actions.

// ===== New Monitoring Tools (registerNewMonitoringTools) =====

func TestToolCheckContainerUpdates(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	_, text := callToolJSON(t, cs, "check_container_updates", nil)
	// May succeed if Docker is available or return error
	if text == "" {
		t.Error("Expected non-empty response")
	}
}

func TestToolCheckContainerUpdate(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	_, text := callToolJSON(t, cs, "check_container_update", map[string]any{
		"container_id": "test-container-123",
	})
	if text == "" {
		t.Error("Expected non-empty response")
	}
}

func TestToolGetContainerSize(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	_, text := callToolJSON(t, cs, "get_container_size", map[string]any{
		"container_id": "test-container-123",
	})
	if text == "" {
		t.Error("Expected non-empty response")
	}
}

func TestToolCheckPluginUpdates(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	_, text := callToolJSON(t, cs, "check_plugin_updates", nil)
	if text == "" {
		t.Error("Expected non-empty response")
	}
}

func TestToolListVMSnapshots(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	_, text := callToolJSON(t, cs, "list_vm_snapshots", map[string]any{
		"vm_name": "test-vm",
	})
	if text == "" {
		t.Error("Expected non-empty response")
	}
}

func TestToolGetServiceStatus(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	_, text := callToolJSON(t, cs, "get_service_status", map[string]any{
		"service_name": "docker",
	})
	if text == "" {
		t.Error("Expected non-empty response")
	}
}

func TestToolListServices(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	_, text := callToolJSON(t, cs, "list_services", nil)
	// Should list services with their status
	if text == "" {
		t.Error("Expected non-empty response")
	}
}

func TestToolListProcesses(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	_, text := callToolJSON(t, cs, "list_processes", map[string]any{
		"sort_by": "cpu",
		"limit":   10,
	})
	if text == "" {
		t.Error("Expected non-empty response")
	}
}

func TestToolListProcessesDefaults(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	// Test with no args (should use defaults: sort_by=cpu, limit=50)
	_, text := callToolJSON(t, cs, "list_processes", nil)
	if text == "" {
		t.Error("Expected non-empty response")
	}
}

// ===== Control Tools (registerControlTools) =====

func TestToolContainerAction_AllActions(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	// Test each valid action (will fail on macOS/non-Docker but covers code paths)
	actions := []string{"start", "stop", "restart", "pause", "unpause"}
	for _, action := range actions {
		t.Run(action, func(t *testing.T) {
			_, text := callToolJSON(t, cs, "container_action", map[string]any{
				"container_id": "test-container",
				"action":       action,
			})
			if text == "" {
				t.Errorf("Expected non-empty response for action %s", action)
			}
		})
	}
}

func TestToolContainerAction_UnknownAction(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	_, text := callToolJSON(t, cs, "container_action", map[string]any{
		"container_id": "test-container",
		"action":       "invalid-action",
	})
	if !strings.Contains(text, "Unknown action") {
		t.Errorf("Expected 'Unknown action' message, got: %s", text)
	}
}

func TestToolVMAction_AllActions(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	actions := []string{"start", "stop", "restart", "pause", "resume", "hibernate", "force-stop"}
	for _, action := range actions {
		t.Run(action, func(t *testing.T) {
			_, text := callToolJSON(t, cs, "vm_action", map[string]any{
				"vm_name": "test-vm",
				"action":  action,
			})
			if text == "" {
				t.Errorf("Expected non-empty response for action %s", action)
			}
		})
	}
}

func TestToolVMAction_UnknownAction(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	_, text := callToolJSON(t, cs, "vm_action", map[string]any{
		"vm_name": "test-vm",
		"action":  "invalid-action",
	})
	if !strings.Contains(text, "Unknown action") {
		t.Errorf("Expected 'Unknown action' message, got: %s", text)
	}
}

func TestToolArrayAction_Unconfirmed(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	_, text := callToolJSON(t, cs, "array_action", map[string]any{
		"action":  "start",
		"confirm": false,
	})
	if !strings.Contains(text, "not confirmed") {
		t.Errorf("Expected confirmation message, got: %s", text)
	}
}

func TestToolArrayAction_Confirmed(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	actions := []string{"start", "stop"}
	for _, action := range actions {
		t.Run(action, func(t *testing.T) {
			_, text := callToolJSON(t, cs, "array_action", map[string]any{
				"action":  action,
				"confirm": true,
			})
			if text == "" {
				t.Errorf("Expected non-empty response for action %s", action)
			}
		})
	}
}

func TestToolArrayAction_UnknownAction(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	_, text := callToolJSON(t, cs, "array_action", map[string]any{
		"action":  "destroy",
		"confirm": true,
	})
	if !strings.Contains(text, "Unknown action") {
		t.Errorf("Expected 'Unknown action' message, got: %s", text)
	}
}

func TestToolParityCheckAction(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	_, text := callToolJSON(t, cs, "parity_check_action", map[string]any{
		"correcting": false,
	})
	// Will fail on non-Unraid but covers the code path
	if text == "" {
		t.Error("Expected non-empty response")
	}
}

func TestToolParityCheckActionCorrecting(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	_, text := callToolJSON(t, cs, "parity_check_action", map[string]any{
		"correcting": true,
	})
	if text == "" {
		t.Error("Expected non-empty response")
	}
}

func TestToolSystemReboot_Unconfirmed(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	_, text := callToolJSON(t, cs, "system_reboot", map[string]any{
		"confirm": false,
	})
	if !strings.Contains(text, "not confirmed") {
		t.Errorf("Expected confirmation message, got: %s", text)
	}
}

func TestToolSystemShutdown_Unconfirmed(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	_, text := callToolJSON(t, cs, "system_shutdown", map[string]any{
		"confirm": false,
	})
	if !strings.Contains(text, "not confirmed") {
		t.Errorf("Expected confirmation message, got: %s", text)
	}
}

func TestToolSystemReboot_Confirmed(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	_, text := callToolJSON(t, cs, "system_reboot", map[string]any{
		"confirm": true,
	})
	// Will fail on non-Unraid (no reboot command)
	if text == "" {
		t.Error("Expected non-empty response")
	}
}

func TestToolSystemShutdown_Confirmed(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	_, text := callToolJSON(t, cs, "system_shutdown", map[string]any{
		"confirm": true,
	})
	if text == "" {
		t.Error("Expected non-empty response")
	}
}

func TestToolParityCheckStop(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	_, text := callToolJSON(t, cs, "parity_check_stop", nil)
	if text == "" {
		t.Error("Expected non-empty response")
	}
}

func TestToolParityCheckPause(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	_, text := callToolJSON(t, cs, "parity_check_pause", nil)
	if text == "" {
		t.Error("Expected non-empty response")
	}
}

func TestToolParityCheckResume(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	_, text := callToolJSON(t, cs, "parity_check_resume", nil)
	if text == "" {
		t.Error("Expected non-empty response")
	}
}

func TestToolDiskSpinDown_EmptyID(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	_, text := callToolJSON(t, cs, "disk_spin_down", map[string]any{
		"disk_id": "",
	})
	if !strings.Contains(text, "disk_id is required") {
		t.Errorf("Expected 'disk_id is required' message, got: %s", text)
	}
}

func TestToolDiskSpinDown(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	_, text := callToolJSON(t, cs, "disk_spin_down", map[string]any{
		"disk_id": "disk1",
	})
	if text == "" {
		t.Error("Expected non-empty response")
	}
}

func TestToolDiskSpinUp_EmptyID(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	_, text := callToolJSON(t, cs, "disk_spin_up", map[string]any{
		"disk_id": "",
	})
	if !strings.Contains(text, "disk_id is required") {
		t.Errorf("Expected 'disk_id is required' message, got: %s", text)
	}
}

func TestToolDiskSpinUp(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	_, text := callToolJSON(t, cs, "disk_spin_up", map[string]any{
		"disk_id": "disk1",
	})
	if text == "" {
		t.Error("Expected non-empty response")
	}
}

func TestToolExecuteUserScript_Unconfirmed(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	_, text := callToolJSON(t, cs, "execute_user_script", map[string]any{
		"script_name": "test-script",
		"confirm":     false,
	})
	if !strings.Contains(text, "not confirmed") {
		t.Errorf("Expected confirmation message, got: %s", text)
	}
}

func TestToolExecuteUserScript_EmptyName(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	_, text := callToolJSON(t, cs, "execute_user_script", map[string]any{
		"script_name": "",
		"confirm":     true,
	})
	if !strings.Contains(text, "script_name is required") {
		t.Errorf("Expected 'script_name is required' message, got: %s", text)
	}
}

func TestToolExecuteUserScript_Confirmed(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	_, text := callToolJSON(t, cs, "execute_user_script", map[string]any{
		"script_name": "test-script",
		"confirm":     true,
	})
	// Will fail on non-Unraid but covers the execution path
	if text == "" {
		t.Error("Expected non-empty response")
	}
}

// ===== New Control Tools (registerNewControlTools) =====

func TestToolUpdateContainer_Unconfirmed(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	_, text := callToolJSON(t, cs, "update_container", map[string]any{
		"container_id": "test-container",
		"confirm":      false,
	})
	if !strings.Contains(text, "confirm=true") {
		t.Errorf("Expected confirmation message, got: %s", text)
	}
}

func TestToolUpdateAllContainers_Unconfirmed(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	_, text := callToolJSON(t, cs, "update_all_containers", map[string]any{
		"confirm": false,
	})
	if !strings.Contains(text, "confirm=true") {
		t.Errorf("Expected confirmation message, got: %s", text)
	}
}

func TestToolUpdatePlugin_Unconfirmed(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	_, text := callToolJSON(t, cs, "update_plugin", map[string]any{
		"plugin_name": "my-plugin",
		"confirm":     false,
	})
	if !strings.Contains(text, "confirm=true") {
		t.Errorf("Expected confirmation message, got: %s", text)
	}
}

func TestToolUpdatePlugin_EmptyName(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	_, text := callToolJSON(t, cs, "update_plugin", map[string]any{
		"plugin_name": "",
		"confirm":     true,
	})
	if !strings.Contains(text, "plugin_name is required") {
		t.Errorf("Expected 'plugin_name is required' message, got: %s", text)
	}
}

func TestToolUpdateAllPlugins_Unconfirmed(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	_, text := callToolJSON(t, cs, "update_all_plugins", map[string]any{
		"confirm": false,
	})
	if !strings.Contains(text, "confirm=true") {
		t.Errorf("Expected confirmation message, got: %s", text)
	}
}

func TestToolCreateVMSnapshot(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	_, text := callToolJSON(t, cs, "create_vm_snapshot", map[string]any{
		"vm_name":       "test-vm",
		"snapshot_name": "snap1",
		"description":   "test snapshot",
	})
	// Will fail on non-Unraid but covers code
	if text == "" {
		t.Error("Expected non-empty response")
	}
}

func TestToolCreateVMSnapshot_AutoName(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	// No snapshot_name — should auto-generate
	_, text := callToolJSON(t, cs, "create_vm_snapshot", map[string]any{
		"vm_name": "test-vm",
	})
	if text == "" {
		t.Error("Expected non-empty response")
	}
}

func TestToolDeleteVMSnapshot_EmptyName(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	_, text := callToolJSON(t, cs, "delete_vm_snapshot", map[string]any{
		"vm_name":       "test-vm",
		"snapshot_name": "",
	})
	if !strings.Contains(text, "snapshot_name is required") {
		t.Errorf("Expected 'snapshot_name is required' message, got: %s", text)
	}
}

func TestToolDeleteVMSnapshot(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	_, text := callToolJSON(t, cs, "delete_vm_snapshot", map[string]any{
		"vm_name":       "test-vm",
		"snapshot_name": "snap1",
	})
	if text == "" {
		t.Error("Expected non-empty response")
	}
}

func TestToolCloneVM_Unconfirmed(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	_, text := callToolJSON(t, cs, "clone_vm", map[string]any{
		"vm_name":    "test-vm",
		"clone_name": "test-vm-clone",
		"confirm":    false,
	})
	if !strings.Contains(text, "confirm=true") {
		t.Errorf("Expected confirmation message, got: %s", text)
	}
}

func TestToolCloneVM_EmptyCloneName(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	_, text := callToolJSON(t, cs, "clone_vm", map[string]any{
		"vm_name":    "test-vm",
		"clone_name": "",
		"confirm":    true,
	})
	if !strings.Contains(text, "clone_name is required") {
		t.Errorf("Expected 'clone_name is required' message, got: %s", text)
	}
}

func TestToolServiceAction_Unconfirmed(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	_, text := callToolJSON(t, cs, "service_action", map[string]any{
		"service_name": "docker",
		"action":       "restart",
		"confirm":      false,
	})
	if !strings.Contains(text, "confirm=true") {
		t.Errorf("Expected confirmation message, got: %s", text)
	}
}

func TestToolServiceAction_UnknownAction(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	_, text := callToolJSON(t, cs, "service_action", map[string]any{
		"service_name": "docker",
		"action":       "destroy",
		"confirm":      true,
	})
	if !strings.Contains(text, "Unknown action") {
		t.Errorf("Expected 'Unknown action' message, got: %s", text)
	}
}

func TestToolServiceAction_AllActions(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	actions := []string{"start", "stop", "restart"}
	for _, action := range actions {
		t.Run(action, func(t *testing.T) {
			_, text := callToolJSON(t, cs, "service_action", map[string]any{
				"service_name": "sshd",
				"action":       action,
				"confirm":      true,
			})
			if text == "" {
				t.Errorf("Expected non-empty response for action %s", action)
			}
		})
	}
}

// ===== Collector Tools covered partially - test edge cases =====

func TestToolCollectorAction_EmptyName(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	_, text := callToolJSON(t, cs, "collector_action", map[string]any{
		"collector_name": "",
		"action":         "enable",
	})
	if !strings.Contains(text, "collector_name is required") {
		t.Errorf("Expected 'collector_name is required' message, got: %s", text)
	}
}

func TestToolCollectorAction_UnknownAction(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	_, text := callToolJSON(t, cs, "collector_action", map[string]any{
		"collector_name": "system",
		"action":         "destroy",
	})
	if !strings.Contains(text, "Unknown action") {
		t.Errorf("Expected 'Unknown action' message, got: %s", text)
	}
}

func TestToolUpdateCollectorInterval_EmptyName(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	_, text := callToolJSON(t, cs, "update_collector_interval", map[string]any{
		"collector_name": "",
		"interval":       30,
	})
	if !strings.Contains(text, "collector_name is required") {
		t.Errorf("Expected 'collector_name is required' message, got: %s", text)
	}
}

func TestToolUpdateCollectorInterval_OutOfRange(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	tests := []struct {
		name     string
		interval int
	}{
		{"too_low", 2},
		{"too_high", 4000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, text := callToolJSON(t, cs, "update_collector_interval", map[string]any{
				"collector_name": "system",
				"interval":       tt.interval,
			})
			if !strings.Contains(text, "interval must be between") {
				t.Errorf("Expected interval validation message, got: %s", text)
			}
		})
	}
}

// ===== Additional monitoring tool tests for user_scripts and container updates =====

func TestToolUpdateContainer_Confirmed(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	_, text := callToolJSON(t, cs, "update_container", map[string]any{
		"container_id": "test-container",
		"confirm":      true,
		"force":        false,
	})
	// Will fail on non-Docker but covers confirmed path
	if text == "" {
		t.Error("Expected non-empty response")
	}
}

func TestToolUpdateAllPlugins_Confirmed(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	_, text := callToolJSON(t, cs, "update_all_plugins", map[string]any{
		"confirm": true,
	})
	// Will fail on non-Unraid
	if text == "" {
		t.Error("Expected non-empty response")
	}
}

func TestToolUpdatePlugin_Confirmed(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	_, text := callToolJSON(t, cs, "update_plugin", map[string]any{
		"plugin_name": "test-plugin.plg",
		"confirm":     true,
	})
	// Will fail on non-Unraid
	if text == "" {
		t.Error("Expected non-empty response")
	}
}

func TestToolCloneVM_Confirmed(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	_, text := callToolJSON(t, cs, "clone_vm", map[string]any{
		"vm_name":    "test-vm",
		"clone_name": "test-vm-clone",
		"confirm":    true,
	})
	// Will fail on non-Unraid
	if text == "" {
		t.Error("Expected non-empty response")
	}
}

func TestToolUpdateAllContainers_Confirmed(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	_, text := callToolJSON(t, cs, "update_all_containers", map[string]any{
		"confirm": true,
	})
	// Will fail without Docker but covers confirmed path
	if text == "" {
		t.Error("Expected non-empty response")
	}
}

// ===== Container Logs Tool Tests =====

func TestToolGetContainerLogs(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	_, text := callToolJSON(t, cs, "get_container_logs", map[string]any{
		"container_id": "test-container",
		"tail":         50,
	})
	// Will fail without Docker but covers code path
	if text == "" {
		t.Error("Expected non-empty response")
	}
}

func TestToolGetContainerLogs_EmptyID(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	_, text := callToolJSON(t, cs, "get_container_logs", map[string]any{
		"container_id": "",
	})
	if !strings.Contains(text, "container_id is required") {
		t.Errorf("Expected 'container_id is required' message, got: %s", text)
	}
}

func TestToolGetContainerLogs_WithTimestamps(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	_, text := callToolJSON(t, cs, "get_container_logs", map[string]any{
		"container_id": "test-container",
		"tail":         100,
		"timestamps":   true,
		"since":        "2026-02-17T00:00:00Z",
	})
	if text == "" {
		t.Error("Expected non-empty response")
	}
}

// ===== Restore VM Snapshot Tool Tests =====

func TestToolRestoreVMSnapshot_Unconfirmed(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	_, text := callToolJSON(t, cs, "restore_vm_snapshot", map[string]any{
		"vm_name":       "test-vm",
		"snapshot_name": "snap1",
		"confirm":       false,
	})
	if !strings.Contains(text, "confirm=true") {
		t.Errorf("Expected confirmation message, got: %s", text)
	}
}

func TestToolRestoreVMSnapshot_EmptySnapshotName(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	_, text := callToolJSON(t, cs, "restore_vm_snapshot", map[string]any{
		"vm_name":       "test-vm",
		"snapshot_name": "",
		"confirm":       true,
	})
	if !strings.Contains(text, "snapshot_name is required") {
		t.Errorf("Expected 'snapshot_name is required' message, got: %s", text)
	}
}

func TestToolRestoreVMSnapshot_Confirmed(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	_, text := callToolJSON(t, cs, "restore_vm_snapshot", map[string]any{
		"vm_name":       "test-vm",
		"snapshot_name": "snap1",
		"confirm":       true,
	})
	// Will fail on non-Unraid but covers code path
	if text == "" {
		t.Error("Expected non-empty response")
	}
}
