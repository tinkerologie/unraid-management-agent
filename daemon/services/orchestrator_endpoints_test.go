package services

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tinkerologie/unraid-management-agent/daemon/domain"
	"github.com/tinkerologie/unraid-management-agent/daemon/dto"
	"github.com/tinkerologie/unraid-management-agent/daemon/platform"
	"github.com/tinkerologie/unraid-management-agent/daemon/services/api"
	"github.com/tinkerologie/unraid-management-agent/daemon/services/mcp"
)

// newInitializedMCP sets up an MCP server the way Orchestrator.Run does.
func newInitializedMCP(t *testing.T, ctx *domain.Context, srv *api.Server, store *domain.ToolPolicyStore) *mcp.Server {
	t.Helper()
	server := mcp.NewServer(ctx, srv)
	server.SetToolPolicyStore(store)
	if err := server.Initialize(); err != nil {
		t.Fatal(err)
	}
	return server
}

func TestMountHTTPMCP(t *testing.T) {
	secret := strings.Repeat("s", 32)
	build := func(t *testing.T, disabled bool) (*Orchestrator, *api.Server, *domain.ToolPolicyStore) {
		t.Helper()
		ctx := &domain.Context{
			Hub:      domain.NewEventBus(10),
			Platform: platform.NewRegistry(),
			Config:   domain.Config{MCPDisabled: disabled, MCPConnectSecret: secret},
		}
		srv := api.NewServer(ctx)
		t.Cleanup(srv.Stop)
		return CreateOrchestrator(ctx), srv, domain.NewToolPolicyStore(t.TempDir(), nil)
	}
	paths := []string{"/mcp", "/mcp/", "/mcp/" + secret, "/mcp-stateless"}
	status := func(srv *api.Server, path string) int {
		w := httptest.NewRecorder()
		srv.GetRouter().ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		return w.Code
	}

	t.Run("disabled mounts nothing but keeps the tool catalog", func(t *testing.T) {
		o, srv, store := build(t, true)
		o.mountHTTPMCP(srv, newInitializedMCP(t, o.ctx, srv, store))
		for _, p := range paths {
			if code := status(srv, p); code != http.StatusNotFound {
				t.Errorf("%s returned %d, want 404", p, code)
			}
		}
		if len(store.GetCatalog(false)) == 0 {
			t.Error("the tool catalog must stay filled, or Apply on the settings page erases saved policies")
		}
	})

	t.Run("enabled mounts the routes", func(t *testing.T) {
		o, srv, store := build(t, false)
		o.mountHTTPMCP(srv, newInitializedMCP(t, o.ctx, srv, store))
		for _, p := range paths {
			if code := status(srv, p); code == http.StatusNotFound {
				t.Errorf("%s returned 404, want it mounted", p)
			}
		}
	})
}

// The settings page rewrites tool_policy.json from this response, so saved
// policies must survive while MCP is disabled.
func TestDisabledMCPKeepsSavedToolPolicies(t *testing.T) {
	ctx := &domain.Context{
		Hub:      domain.NewEventBus(10),
		Platform: platform.NewRegistry(),
		Config:   domain.Config{MCPDisabled: true},
	}
	srv := api.NewServer(ctx)
	t.Cleanup(srv.Stop)
	store := domain.NewToolPolicyStore(t.TempDir(), map[string]domain.ToolPolicyValue{"system_reboot": "hidden"})
	srv.SetToolPolicyStore(store)
	CreateOrchestrator(ctx).mountHTTPMCP(srv, newInitializedMCP(t, ctx, srv, store))

	w := httptest.NewRecorder()
	srv.GetRouter().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/mcp/tool-policy", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("tool policy returned %d", w.Code)
	}
	var got dto.MCPToolPolicyResponse
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Policies["system_reboot"] != "hidden" {
		t.Errorf("policies = %v, want the saved system_reboot=hidden", got.Policies)
	}
	if len(got.Tools) == 0 {
		t.Error("the tool list must not be empty")
	}
}

func TestStartedMessage(t *testing.T) {
	for _, disabled := range []bool{false, true} {
		o := CreateOrchestrator(&domain.Context{Config: domain.Config{Port: 8043, MCPDisabled: disabled}})
		if got := strings.Contains(o.startedMessage("http"), "/mcp"); got == disabled {
			t.Errorf("MCPDisabled=%v: message %q", disabled, o.startedMessage("http"))
		}
	}
}

// initializeDiscovery must never fail startup, with MCP on or off. Discovery
// tolerates a network with no usable interface, as on some CI runners.
func TestInitializeDiscoveryWithMCPDisabled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	o := CreateOrchestrator(&domain.Context{Config: domain.Config{Port: 18043, MCPDisabled: true}})
	o.initializeDiscovery(ctx)
	if o.discoveryService != nil {
		t.Cleanup(o.discoveryService.Shutdown)
	}
}
