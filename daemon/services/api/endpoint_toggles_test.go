package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/tinkerologie/unraid-management-agent/daemon/domain"
	"github.com/tinkerologie/unraid-management-agent/daemon/dto"
)

func TestEndpointToggleRoutes(t *testing.T) {
	tests := []struct {
		name string
		path string
		off  domain.Config
	}{
		{"metrics", "/metrics", domain.Config{MetricsDisabled: true}},
		{"websocket", "/api/v1/ws", domain.Config{WebSocketDisabled: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches := func(s *Server) bool {
				var m mux.RouteMatch
				return s.router.Match(httptest.NewRequest(http.MethodGet, tt.path, nil), &m) && m.MatchErr == nil
			}
			if !matches(NewServer(&domain.Context{})) {
				t.Errorf("%s must be registered by default", tt.path)
			}
			off := NewServer(&domain.Context{Config: tt.off})
			if matches(off) {
				t.Errorf("%s must not be registered when disabled", tt.path)
			}
			w := httptest.NewRecorder()
			off.router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, tt.path, nil))
			if w.Code != http.StatusNotFound {
				t.Errorf("disabled %s returned %d, want 404", tt.path, w.Code)
			}
		})
	}
}

// REST routes stay available when every optional endpoint is off.
func TestEndpointTogglesKeepREST(t *testing.T) {
	s := NewServer(&domain.Context{Config: domain.Config{MCPDisabled: true, MetricsDisabled: true, WebSocketDisabled: true}})
	w := httptest.NewRecorder()
	s.router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/health", nil))
	if w.Code != http.StatusOK {
		t.Errorf("/api/v1/health returned %d, want 200", w.Code)
	}
}

func TestWebSocketDisabledStartsNoHub(t *testing.T) {
	hub := domain.NewEventBus(10)
	s := NewServerWithCollectorManager(
		&domain.Context{Hub: hub, Config: domain.Config{WebSocketDisabled: true}},
		newFakeDynamic(),
	)
	t.Cleanup(s.Stop)
	if s.wsHub != nil {
		t.Fatal("the hub must not be built while WebSocket is disabled")
	}

	s.StartSubscriptions()
	select {
	case <-s.Ready():
	case <-time.After(5 * time.Second):
		t.Fatal("readiness did not complete with WebSocket disabled")
	}

	// The cache subscription keeps serving REST and MCP.
	hub.Pub(&dto.SystemInfo{Hostname: "toggle-test"}, "system_update")
	eventually(t, func() bool {
		info := s.GetSystemCache()
		return info != nil && info.Hostname == "toggle-test"
	})

	// A direct agent broadcast must not touch the missing hub.
	s.BroadcastAgentEvent(dto.WSEvent{Event: "agent_stream"})
}
