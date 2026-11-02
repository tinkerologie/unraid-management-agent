package services

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tinkerologie/unraid-management-agent/daemon/domain"
	"github.com/tinkerologie/unraid-management-agent/daemon/platform"
	"github.com/tinkerologie/unraid-management-agent/daemon/services/api"
)

func TestInitializeHTTPMCP(t *testing.T) {
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
	paths := []string{"/mcp", "/mcp/", "/mcp/" + secret}
	status := func(srv *api.Server, path string) int {
		w := httptest.NewRecorder()
		srv.GetRouter().ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		return w.Code
	}

	t.Run("disabled returns nil and mounts nothing", func(t *testing.T) {
		o, srv, store := build(t, true)
		if got := o.initializeHTTPMCP(srv, store); got != nil {
			t.Fatal("the MCP server must not be built while disabled")
		}
		for _, p := range paths {
			if code := status(srv, p); code != http.StatusNotFound {
				t.Errorf("%s returned %d, want 404", p, code)
			}
		}
	})

	t.Run("enabled mounts the routes", func(t *testing.T) {
		o, srv, store := build(t, false)
		if got := o.initializeHTTPMCP(srv, store); got == nil {
			t.Fatal("the MCP server must be built while enabled")
		}
		for _, p := range paths {
			if code := status(srv, p); code == http.StatusNotFound {
				t.Errorf("%s returned 404, want it mounted", p)
			}
		}
	})
}
