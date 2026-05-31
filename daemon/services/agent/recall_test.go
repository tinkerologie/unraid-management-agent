package agent

import (
	"context"
	"testing"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/services/agent/llm"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/services/agent/memory"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/services/agent/tools"
)

func TestFinalizeWritesIncident(t *testing.T) {
	p := llm.NewMockProvider(&llm.ChatResponse{Text: "All healthy.", OutputTokens: 2})
	cfg := dto.DefaultAgentConfig()
	cfg.Enabled = true
	mem := memory.NewStore(t.TempDir(), 100)
	svc := NewService(cfg, p, tools.BuildDefault(fakeState{}, fakeDocker{}), NewStore(t.TempDir()), mem, &capturingBroadcaster{})
	sess, _ := svc.StartSession(context.Background(), "is plex healthy?")
	if sess.Status != dto.SessionCompleted {
		t.Fatalf("precondition: %q", sess.Status)
	}
	incs := mem.ListIncidents()
	if len(incs) != 1 || incs[0].Goal != "is plex healthy?" || incs[0].Outcome != string(dto.SessionCompleted) {
		t.Fatalf("expected one completed incident, got %+v", incs)
	}
}

func TestRecallInjectedAtStart(t *testing.T) {
	cfg := dto.DefaultAgentConfig()
	cfg.Enabled = true
	mem := memory.NewStore(t.TempDir(), 100)
	mem.AddIncident(dto.AgentIncident{ID: "i1", Signature: "plex healthy", Summary: "last time: restarted plex container"})
	p := llm.NewMockProvider(&llm.ChatResponse{Text: "ok", OutputTokens: 1})
	svc := NewService(cfg, p, tools.BuildDefault(fakeState{}, fakeDocker{}), NewStore(t.TempDir()), mem, &capturingBroadcaster{})
	_, _ = svc.StartSession(context.Background(), "is plex healthy?")
	reqs := p.Requests()
	if len(reqs) == 0 {
		t.Fatal("no provider request")
	}
	found := false
	for _, m := range reqs[0].Messages {
		if m.Role == "system" && len(m.Content) > 0 {
			found = true
		}
	}
	if !found {
		t.Fatal("expected a recalled-context system message injected into the first request")
	}
}

func TestNoIncidentOnApprovalPause(t *testing.T) {
	p := llm.NewMockProvider(&llm.ChatResponse{ToolCalls: []llm.ToolCall{{ID: "t1", Name: "stop_array", Args: "{}"}}, OutputTokens: 2})
	cfg := dto.DefaultAgentConfig()
	cfg.Enabled = true
	reg := tools.NewRegistry()
	reg.Register(tools.Tool{Name: "stop_array", RiskTier: dto.RiskHigh, Invoke: func(_ context.Context, _ string) (string, error) { return "", nil }})
	mem := memory.NewStore(t.TempDir(), 100)
	svc := NewService(cfg, p, reg, NewStore(t.TempDir()), mem, &capturingBroadcaster{})
	sess, _ := svc.StartSession(context.Background(), "stop array")
	if sess.Status != dto.SessionAwaitingApproval {
		t.Fatalf("precondition: %q", sess.Status)
	}
	if len(mem.ListIncidents()) != 0 {
		t.Fatal("must not write an incident while awaiting approval")
	}
}
