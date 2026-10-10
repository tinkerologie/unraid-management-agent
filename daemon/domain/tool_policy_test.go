package domain

import (
	"os"
	"path/filepath"
	"testing"
)

func TestToolPolicyStore_PrecedenceAndEffectivePolicy(t *testing.T) {
	tempDir := t.TempDir()
	store := NewToolPolicyStore(tempDir, nil)

	// Register tools
	store.RegisterTool("test_read", "Read tool", true, false)
	store.RegisterTool("test_write", "Write tool", false, false)
	store.RegisterTool("test_destructive", "Destructive tool", false, true)

	// Test default behavior when no policy is set
	if eff := store.GetEffectivePolicy("test_read", false); eff != PolicyDefault {
		t.Errorf("expected test_read effective policy default, got %v", eff)
	}
	if eff := store.GetEffectivePolicy("test_write", false); eff != PolicyDefault {
		t.Errorf("expected test_write effective policy default, got %v", eff)
	}
	if eff := store.GetEffectivePolicy("test_destructive", false); eff != PolicyDefault {
		t.Errorf("expected test_destructive effective policy default, got %v", eff)
	}

	// Set per-tool policies
	store.Replace(map[string]ToolPolicyValue{
		"test_read":        PolicyReadOnly, // read tools ignore write gates
		"test_write":       PolicyAsk,
		"test_destructive": PolicyAllow,
	})

	if eff := store.GetEffectivePolicy("test_read", false); eff != PolicyDefault {
		t.Errorf("read-only tool should never be blocked by policy, got %v", eff)
	}
	if eff := store.GetEffectivePolicy("test_write", false); eff != PolicyAsk {
		t.Errorf("expected PolicyAsk, got %v", eff)
	}
	if eff := store.GetEffectivePolicy("test_destructive", false); eff != PolicyAllow {
		t.Errorf("expected PolicyAllow, got %v", eff)
	}

	// Global read-only mode takes precedence over per-tool policy (kill-switch)
	if eff := store.GetEffectivePolicy("test_write", true); eff != PolicyReadOnly {
		t.Errorf("global read-only should override per-tool policy to read_only, got %v", eff)
	}
	if eff := store.GetEffectivePolicy("test_destructive", true); eff != PolicyReadOnly {
		t.Errorf("global read-only should override allow to read_only, got %v", eff)
	}
	if eff := store.GetEffectivePolicy("test_read", true); eff != PolicyDefault {
		t.Errorf("read tools keep working even in global read-only mode, got %v", eff)
	}

	// Hidden policy takes precedence over everything including global read-only
	store.Replace(map[string]ToolPolicyValue{
		"test_read":  PolicyHidden,
		"test_write": PolicyHidden,
	})
	if eff := store.GetEffectivePolicy("test_read", false); eff != PolicyHidden {
		t.Errorf("hidden policy should hide read tools, got %v", eff)
	}
	if eff := store.GetEffectivePolicy("test_read", true); eff != PolicyHidden {
		t.Errorf("hidden policy should hide read tools even in global read-only, got %v", eff)
	}
	if eff := store.GetEffectivePolicy("test_write", true); eff != PolicyHidden {
		t.Errorf("hidden policy should hide write tools, got %v", eff)
	}
}

func TestToolPolicyStore_SaveAndLoad(t *testing.T) {
	tempDir := t.TempDir()
	store1 := NewToolPolicyStore(tempDir, map[string]ToolPolicyValue{
		"tool_a": PolicyHidden,
		"tool_b": PolicyAsk,
	})

	if err := store1.Save(); err != nil {
		t.Fatalf("failed to save store: %v", err)
	}

	store2 := NewToolPolicyStore(tempDir, nil)
	if err := store2.Load(); err != nil {
		t.Fatalf("failed to load store: %v", err)
	}

	if store2.GetPolicy("tool_a") != PolicyHidden {
		t.Errorf("expected tool_a to be hidden, got %v", store2.GetPolicy("tool_a"))
	}
	if store2.GetPolicy("tool_b") != PolicyAsk {
		t.Errorf("expected tool_b to be ask, got %v", store2.GetPolicy("tool_b"))
	}
	if store2.GetPolicy("tool_c") != PolicyDefault {
		t.Errorf("expected tool_c to be default, got %v", store2.GetPolicy("tool_c"))
	}
}

func TestToolPolicyStore_LoadInvalidFileFallsBack(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, ToolPolicyConfigFile)
	// Write file with one invalid policy value and one valid
	if err := os.WriteFile(filePath, []byte(`{"tool_good": "allow", "tool_bad": "superadmin"}`), 0o644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	store := NewToolPolicyStore(tempDir, nil)
	if err := store.Load(); err != nil {
		t.Fatalf("load failed: %v", err)
	}

	if store.GetPolicy("tool_good") != PolicyAllow {
		t.Errorf("expected tool_good to be allow, got %v", store.GetPolicy("tool_good"))
	}
	if store.GetPolicy("tool_bad") != PolicyDefault {
		t.Errorf("expected tool_bad with invalid policy to fall back to default, got %v", store.GetPolicy("tool_bad"))
	}
}

func TestToolPolicyStore_LoadReplacesSeededPolicies(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, ToolPolicyConfigFile)
	if err := os.WriteFile(filePath, []byte(`{"tool_a": "hidden"}`), 0o644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	store := NewToolPolicyStore(tempDir, map[string]ToolPolicyValue{
		"tool_b": PolicyAllow,
	})
	if err := store.Load(); err != nil {
		t.Fatalf("load failed: %v", err)
	}

	if store.GetPolicy("tool_a") != PolicyHidden {
		t.Errorf("expected tool_a to be hidden, got %v", store.GetPolicy("tool_a"))
	}
	if store.GetPolicy("tool_b") != PolicyDefault {
		t.Errorf("expected tool_b to be reset to default, got %v", store.GetPolicy("tool_b"))
	}
}

func TestToolPolicyStore_IsValidTool(t *testing.T) {
	store := NewToolPolicyStore(t.TempDir(), nil)
	store.RegisterTool("get_system_info", "System info", true, false)

	if !store.IsValidTool("get_system_info") {
		t.Error("expected registered tool to be valid")
	}
	if store.IsValidTool("nonexistent_tool") {
		t.Error("expected unregistered tool to be invalid")
	}
	if store.IsValidTool("") {
		t.Error("expected empty name to be invalid")
	}
}

func TestToolPolicyStore_GetAll(t *testing.T) {
	store := NewToolPolicyStore(t.TempDir(), nil)
	store.RegisterTool("container_start", "Start", false, false)
	store.RegisterTool("get_system_info", "Info", true, false)

	store.Replace(map[string]ToolPolicyValue{
		"container_start": PolicyAsk,
		"get_system_info": PolicyHidden,
		// Not registered in the catalog: must be excluded once a catalog exists.
		"unregistered_tool": PolicyAllow,
	})

	all := store.GetAll()
	if all["container_start"] != PolicyAsk {
		t.Errorf("container_start = %v, want ask", all["container_start"])
	}
	if all["get_system_info"] != PolicyHidden {
		t.Errorf("get_system_info = %v, want hidden", all["get_system_info"])
	}
	if _, ok := all["unregistered_tool"]; ok {
		t.Error("unregistered tool should be excluded from GetAll when a catalog exists")
	}
}

func TestToolPolicyStore_GetAll_NoCatalog(t *testing.T) {
	// Before any tool is registered the catalog is empty, so GetAll returns all
	// non-default policies regardless of registration.
	store := NewToolPolicyStore(t.TempDir(), map[string]ToolPolicyValue{
		"tool_a": PolicyAsk,
		"tool_b": PolicyAllow,
	})
	all := store.GetAll()
	if all["tool_a"] != PolicyAsk || all["tool_b"] != PolicyAllow {
		t.Errorf("expected both policies returned with empty catalog, got %v", all)
	}
}

func TestToolPolicyStore_GetCatalog(t *testing.T) {
	store := NewToolPolicyStore(t.TempDir(), nil)
	store.RegisterTool("container_start", "Start a container", false, false)
	store.RegisterTool("get_system_info", "Read system info", true, false)
	store.Replace(map[string]ToolPolicyValue{
		"container_start": PolicyAsk,
	})

	catalog := store.GetCatalog(false)
	if len(catalog) != 2 {
		t.Fatalf("expected 2 catalog items, got %d", len(catalog))
	}

	byName := make(map[string]string, len(catalog))
	effByName := make(map[string]string, len(catalog))
	for _, item := range catalog {
		byName[item.Name] = item.ConfiguredPolicy
		effByName[item.Name] = item.EffectivePolicy
	}

	if byName["container_start"] != string(PolicyAsk) {
		t.Errorf("container_start configured = %q, want ask", byName["container_start"])
	}
	if effByName["container_start"] != string(PolicyAsk) {
		t.Errorf("container_start effective = %q, want ask", effByName["container_start"])
	}
	// Read-only tool with no configured policy stays default.
	if effByName["get_system_info"] != string(PolicyDefault) {
		t.Errorf("get_system_info effective = %q, want default", effByName["get_system_info"])
	}

	// Global read-only forces the write tool to read_only but leaves the
	// read-only tool at default.
	catalogRO := store.GetCatalog(true)
	if len(catalogRO) != 2 {
		t.Fatalf("GetCatalog(true) returned %d items, want 2", len(catalogRO))
	}
	roByName := make(map[string]string, len(catalogRO))
	for _, item := range catalogRO {
		roByName[item.Name] = item.EffectivePolicy
	}
	if _, ok := roByName["container_start"]; !ok {
		t.Error("GetCatalog(true) missing container_start")
	}
	if _, ok := roByName["get_system_info"]; !ok {
		t.Error("GetCatalog(true) missing get_system_info")
	}
	if roByName["container_start"] != string(PolicyReadOnly) {
		t.Errorf("container_start effective under global RO = %q, want read_only", roByName["container_start"])
	}
	if roByName["get_system_info"] != string(PolicyDefault) {
		t.Errorf("get_system_info effective under global RO = %q, want default", roByName["get_system_info"])
	}
}

func TestCategorizeMCPTool(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"container_start", "docker"},
		{"get_docker_info", "docker"},
		{"vm_start", "vm"},
		{"array_start", "array"},
		{"parity_check", "array"},
		{"disk_spin_down", "disk"},
		{"smart_test", "disk"},
		{"fan_set_speed", "fancontrol"},
		{"cpu_set_governor", "cpu"},
		{"set_turbo", "cpu"},
		{"tuning_apply", "tuning"},
		{"alert_create", "alerting"},
		{"watchdog_probe", "watchdog"},
		{"agent_run", "agent"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CategorizeMCPTool(tt.name); got != tt.want {
				t.Errorf("CategorizeMCPTool(%q) = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}

func TestValidateToolPolicyValue(t *testing.T) {
	tests := []struct {
		val     string
		wantErr bool
	}{
		{"default", false},
		{"hidden", false},
		{"read_only", false},
		{"allow", false},
		{"ask", false},
		{"", false},
		{"invalid", true},
		{"deny", true},
		{"admin", true},
		{"read", true},
		{"WRITE", true},
		{" ALLOW ", true},
		{"allow ", true},
		{"\tread_only", true},
	}

	for _, tt := range tests {
		t.Run(tt.val, func(t *testing.T) {
			err := ValidateToolPolicyValue(tt.val)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateToolPolicyValue(%q) err = %v, wantErr %v", tt.val, err, tt.wantErr)
			}
		})
	}
}
