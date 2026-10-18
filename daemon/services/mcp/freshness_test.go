package mcp

import (
	"context"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tinkerologie/unraid-management-agent/daemon/domain"
	"github.com/tinkerologie/unraid-management-agent/daemon/services/api"
	"github.com/tinkerologie/unraid-management-agent/daemon/services/scheduler"
)

// freshProvider is the mock cache provider plus an EnsureFresh recorder.
type freshProvider struct {
	*MockCacheProvider
	mu    sync.Mutex
	calls [][]string
	// recordDemand holds the flag of every call.
	recordDemand []bool
}

func (p *freshProvider) EnsureFresh(_ context.Context, recordDemand bool, collectors ...string) api.Freshness {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls = append(p.calls, collectors)
	p.recordDemand = append(p.recordDemand, recordDemand)
	return api.Freshness{}
}

func (p *freshProvider) lastCall() ([]string, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.calls) == 0 {
		return nil, false
	}
	return p.calls[len(p.calls)-1], p.recordDemand[len(p.recordDemand)-1]
}

func setupFreshServer(t *testing.T) (*mcp.ClientSession, *freshProvider) {
	t.Helper()
	p := &freshProvider{MockCacheProvider: newMockCacheProvider()}
	server := NewServer(&domain.Context{Version: "test", Port: 8043}, p)
	if err := server.Initialize(); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	cs, cleanup := connectClientToServer(t, server)
	t.Cleanup(cleanup)
	return cs, p
}

// TestReadCollectorsCoverReadTools makes every read-only tool either map to
// the collectors it serves or be listed as serving none.
func TestReadCollectorsCoverReadTools(t *testing.T) {
	server, _ := setupInitializedServer(t)
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()
	ctx := context.Background()

	names := map[string]bool{}
	tools, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range tools.Tools {
		names[tool.Name] = true
		if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint {
			continue
		}
		_, mapped := readCollectors[tool.Name]
		if mapped == toolsWithoutCollectors[tool.Name] {
			t.Errorf("read tool %s must be in exactly one of readCollectors and toolsWithoutCollectors", tool.Name)
		}
	}
	prompts, err := cs.ListPrompts(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range prompts.Prompts {
		names[p.Name] = true
	}
	resources, err := cs.ListResources(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range resources.Resources {
		names[r.Name] = true
	}

	for name, collectors := range readCollectors {
		if !names[name] {
			t.Errorf("readCollectors has unknown tool, prompt or resource %s", name)
		}
		for _, src := range collectors {
			name, topic, _ := strings.Cut(src, ":")
			p, ok := scheduler.PolicyFor(name)
			if !ok {
				t.Errorf("%s names collector %q without a policy", name, src)
			} else if topic != "" && !slices.Contains(p.Topics, topic) {
				t.Errorf("%s: collector %s does not publish %s", name, src, topic)
			}
		}
	}
	for name := range toolsWithoutCollectors {
		if !names[name] {
			t.Errorf("toolsWithoutCollectors has unknown tool %s", name)
		}
	}
}

func TestEnsureFresh_ToolsPromptsResources(t *testing.T) {
	cs, p := setupFreshServer(t)
	ctx := context.Background()

	tests := []struct {
		name string
		call func() error
		want []string
	}{
		{"read tool", func() error {
			_, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "get_system_info"})
			return err
		}, readCollectors["get_system_info"]},
		{"tool with arguments", func() error {
			_, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "get_disk_info", Arguments: map[string]any{"disk_id": "disk1"}})
			return err
		}, readCollectors["get_disk_info"]},
		{"prompt", func() error {
			_, err := cs.GetPrompt(ctx, &mcp.GetPromptParams{Name: "system_overview"})
			return err
		}, readCollectors["system_overview"]},
		{"resource", func() error {
			_, err := cs.ReadResource(ctx, &mcp.ReadResourceParams{URI: "unraid://system"})
			return err
		}, readCollectors["system-info"]},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.call(); err != nil {
				t.Fatal(err)
			}
			got, record := p.lastCall()
			if !slices.Equal(got, tt.want) || !record {
				t.Errorf("EnsureFresh(%v, record=%v), want (%v, record=true)", got, record, tt.want)
			}
		})
	}

	t.Run("unmapped tool does not refresh", func(t *testing.T) {
		p.mu.Lock()
		before := len(p.calls)
		p.mu.Unlock()
		if _, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "list_collectors"}); err != nil {
			t.Fatal(err)
		}
		p.mu.Lock()
		defer p.mu.Unlock()
		if len(p.calls) != before {
			t.Errorf("list_collectors called EnsureFresh")
		}
	})
}

func TestEnsureFresh_ProviderWithoutFreshener(t *testing.T) {
	server, _ := setupTestMCPServer()
	// The mock has no EnsureFresh: ensureFresh must be a no-op.
	server.ensureFresh(context.Background(), "get_system_info")
	server.ensureFresh(context.Background(), "not_a_tool")
}

// conflictProvider rejects interval changes as Dynamic mode does.
type conflictProvider struct{ *MockCacheProvider }

func (conflictProvider) UpdateCollectorInterval(string, int) error {
	return domain.ErrDynamicCollectionMode
}

func TestToolUpdateCollectorInterval_DynamicModeConflict(t *testing.T) {
	server := NewServer(&domain.Context{Version: "test", Port: 8043}, conflictProvider{newMockCacheProvider()})
	if err := server.Initialize(); err != nil {
		t.Fatal(err)
	}
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()
	_, text := callToolJSON(t, cs, "update_collector_interval", map[string]any{
		"collector_name": "docker",
		"interval":       float64(30),
	})
	if !strings.HasPrefix(text, "Conflict:") {
		t.Errorf("text = %q, want a Conflict: result", text)
	}
}

// toolsWithoutCollectors are the read-only tools that serve no cached
// collector data, so calls neither refresh nor record demand.
var toolsWithoutCollectors = map[string]bool{
	"agent_get_memory":         true,
	"agent_get_session":        true,
	"agent_list_sessions":      true,
	"check_container_update":   true,
	"check_container_updates":  true,
	"get_alert_history":        true,
	"get_alert_rule":           true,
	"get_alert_status":         true,
	"get_collector_status":     true,
	"get_container_logs":       true,
	"get_container_size":       true,
	"get_disk_settings":        true,
	"get_docker_log":           true,
	"get_docker_settings":      true,
	"get_fan_sensors":          true,
	"get_firing_alerts":        true,
	"get_health_check":         true,
	"get_health_check_history": true,
	"get_health_check_status":  true,
	"get_log_content":          true,
	"get_network_access_urls":  true,
	"get_parity_history":       true,
	"get_port_conflicts":       true,
	"get_service_status":       true,
	"get_share_config":         true,
	"get_syslog":               true,
	"get_system_settings":      true,
	"get_vm_settings":          true,
	"list_alert_rules":         true,
	"list_alert_templates":     true,
	"list_collectors":          true,
	"list_health_checks":       true,
	"list_log_files":           true,
	"list_process_io":          true,
	"list_processes":           true,
	"list_runbooks":            true,
	"list_services":            true,
	"list_user_scripts":        true,
	"list_vm_snapshots":        true,
	"query_metric_history":     true,
	"run_self_test":            true,
}
