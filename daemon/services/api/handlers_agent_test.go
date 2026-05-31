package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/services/agent"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/services/agent/llm"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/services/agent/tools"
)

type agentTestState struct{}

func (agentTestState) SystemJSON() (any, bool) { return map[string]string{"host": "tower"}, true }
func (agentTestState) ArrayJSON() (any, bool)  { return map[string]string{"state": "STARTED"}, true }
func (agentTestState) DockerJSON() (any, bool) { return []string{}, true }

type agentTestDocker struct{}

func (agentTestDocker) Restart(string) error { return nil }

func newAgentServer(t *testing.T) *Server {
	t.Helper()
	hub := domain.NewEventBus(10)
	s := NewServer(&domain.Context{Hub: hub})
	cfg := dto.DefaultAgentConfig()
	cfg.Enabled = true
	p := llm.NewMockProvider(&llm.ChatResponse{Text: "Healthy.", OutputTokens: 3})
	reg := tools.BuildDefault(agentTestState{}, agentTestDocker{})
	svc := agent.NewService(cfg, p, reg, agent.NewStore(t.TempDir()), s)
	s.SetAgent(svc)
	return s
}

func TestAgentStartSession(t *testing.T) {
	s := newAgentServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agent/sessions", strings.NewReader(`{"goal":"status?"}`))
	rr := httptest.NewRecorder()
	s.GetRouter().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "Healthy.") {
		t.Fatalf("expected answer in body, got %s", rr.Body.String())
	}
}

func TestAgentStartSessionMissingGoal(t *testing.T) {
	s := newAgentServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agent/sessions", strings.NewReader(`{}`))
	rr := httptest.NewRecorder()
	s.GetRouter().ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestAgentListSessions(t *testing.T) {
	s := newAgentServer(t)
	// Start one session so the list is non-empty.
	start := httptest.NewRequest(http.MethodPost, "/api/v1/agent/sessions", strings.NewReader(`{"goal":"status?"}`))
	s.GetRouter().ServeHTTP(httptest.NewRecorder(), start)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agent/sessions", nil)
	rr := httptest.NewRecorder()
	s.GetRouter().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "status?") {
		t.Fatalf("expected listed session in body, got %s", rr.Body.String())
	}
}

func TestAgentGetSession(t *testing.T) {
	s := newAgentServer(t)
	start := httptest.NewRequest(http.MethodPost, "/api/v1/agent/sessions", strings.NewReader(`{"goal":"status?"}`))
	startRR := httptest.NewRecorder()
	s.GetRouter().ServeHTTP(startRR, start)

	sessions := s.agentSvc.ListSessions()
	if len(sessions) == 0 {
		t.Fatalf("expected at least one session")
	}
	id := sessions[0].ID

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agent/sessions/"+id, nil)
	rr := httptest.NewRecorder()
	s.GetRouter().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), id) {
		t.Fatalf("expected session id in body, got %s", rr.Body.String())
	}
}

func TestAgentGetSessionNotFound(t *testing.T) {
	s := newAgentServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agent/sessions/does-not-exist", nil)
	rr := httptest.NewRecorder()
	s.GetRouter().ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestAgentStartSessionRejectsBadInput(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{name: "empty goal", body: `{"goal":""}`},
		{name: "whitespace goal", body: `{"goal":"   "}`},
		{name: "over-length goal", body: `{"goal":"` + strings.Repeat("a", 5000) + `"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newAgentServer(t)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/agent/sessions", strings.NewReader(tc.body))
			rr := httptest.NewRecorder()
			s.GetRouter().ServeHTTP(rr, req)
			if rr.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d body=%s", rr.Code, rr.Body.String())
			}
		})
	}
}

func TestAgentGetSessionInjectionID(t *testing.T) {
	// A path-traversal/injection-flavored ID must be treated as an unknown
	// session: the handler looks it up by exact key and never touches the
	// filesystem, so the response is 404 with no leak.
	cases := []string{
		"..etc-passwd",        // dotted, single segment (reaches the handler)
		"sess-1%3Bcat%20/etc", // injection-flavored, single segment
	}
	for _, id := range cases {
		t.Run(id, func(t *testing.T) {
			s := newAgentServer(t)
			req := httptest.NewRequest(http.MethodGet, "/api/v1/agent/sessions/"+id, nil)
			rr := httptest.NewRecorder()
			s.GetRouter().ServeHTTP(rr, req)
			if rr.Code != http.StatusNotFound {
				t.Fatalf("expected 404 for injection-ish id %q, got %d body=%s", id, rr.Code, rr.Body.String())
			}
		})
	}
}

func TestAgentDisabledReturns503(t *testing.T) {
	hub := domain.NewEventBus(10)
	s := NewServer(&domain.Context{Hub: hub}) // agent NOT set
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agent/sessions", strings.NewReader(`{"goal":"x"}`))
	rr := httptest.NewRecorder()
	s.GetRouter().ServeHTTP(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "\"success\":false") {
		t.Fatalf("expected Success=false in body, got %s", rr.Body.String())
	}
}
