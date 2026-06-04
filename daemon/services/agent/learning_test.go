package agent

import (
	"context"
	"testing"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/services/agent/llm"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/services/agent/memory"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/services/agent/tools"
)

func TestProposePreferenceIsPending(t *testing.T) {
	cfg := dto.DefaultAgentConfig()
	cfg.Enabled = true
	mem := memory.NewStore(t.TempDir(), 100)
	reg := tools.BuildDefault(fakeState{}, fakeDocker{})
	svc := NewService(cfg, llm.NewMockProvider(), reg, NewStore(t.TempDir()), mem, &capturingBroadcaster{})
	svc.RegisterLearningTools(reg)
	tool, ok := reg.Get("propose_preference")
	if !ok {
		t.Fatal("propose_preference not registered")
	}
	if tool.RiskTier != dto.RiskReadOnly {
		t.Fatalf("propose_preference must be read-only, got %q", tool.RiskTier)
	}
	out, err := tool.Invoke(context.Background(), `{"kind":"auto_approve_tool","subject":"restart_container","note":"plex flaps"}`)
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	_ = out
	if len(mem.ListPreferences()) != 1 || len(mem.ActivePreferences()) != 0 {
		t.Fatalf("expected one PENDING preference, active=%d", len(mem.ActivePreferences()))
	}
}

func TestProposePreferenceRejectsEmptyFields(t *testing.T) {
	cfg := dto.DefaultAgentConfig()
	cfg.Enabled = true
	mem := memory.NewStore(t.TempDir(), 100)
	reg := tools.BuildDefault(fakeState{}, fakeDocker{})
	svc := NewService(cfg, llm.NewMockProvider(), reg, NewStore(t.TempDir()), mem, &capturingBroadcaster{})
	svc.RegisterLearningTools(reg)

	prefTool, _ := reg.Get("propose_preference")
	for _, args := range []string{`{"kind":"","subject":"x"}`, `{"kind":"auto_approve_tool","subject":""}`} {
		if _, err := prefTool.Invoke(context.Background(), args); err == nil {
			t.Errorf("propose_preference(%s) expected validation error", args)
		}
	}
	if len(mem.ListPreferences()) != 0 {
		t.Fatalf("no preference should be stored for invalid input, got %d", len(mem.ListPreferences()))
	}

	rbTool, _ := reg.Get("propose_runbook")
	for _, args := range []string{`{"name":"","description":"x"}`, `{"name":"x","description":""}`} {
		if _, err := rbTool.Invoke(context.Background(), args); err == nil {
			t.Errorf("propose_runbook(%s) expected validation error", args)
		}
	}
}

func newAutoApproveSvc(t *testing.T, called *bool) (*Service, *memory.Store) {
	t.Helper()
	// planner [] then a stop_array tool call (RiskHigh/ModeApprove).
	p := llm.NewMockProvider(
		&llm.ChatResponse{Text: "[]"},
		&llm.ChatResponse{ToolCalls: []llm.ToolCall{{ID: "t1", Name: "stop_array", Args: "{}"}}, OutputTokens: 2},
		&llm.ChatResponse{Text: "done", OutputTokens: 1},
	)
	cfg := dto.DefaultAgentConfig()
	cfg.Enabled = true
	reg := tools.NewRegistry()
	reg.Register(tools.Tool{Name: "stop_array", RiskTier: dto.RiskHigh, Invoke: func(_ context.Context, _ string) (string, error) { *called = true; return "stopped", nil }})
	mem := memory.NewStore(t.TempDir(), 100)
	return NewService(cfg, p, reg, NewStore(t.TempDir()), mem, &capturingBroadcaster{}), mem
}

func TestActivePreferenceAutoApproves(t *testing.T) {
	called := false
	svc, mem := newAutoApproveSvc(t, &called)
	mem.AddPreference(dto.AgentPreference{ID: "p1", Kind: "auto_approve_tool", Subject: "stop_array", Status: dto.PreferencePending})
	if err := mem.ConfirmPreference("p1"); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	sess, _ := svc.StartSession(context.Background(), "stop the array")
	if sess.Status != dto.SessionCompleted {
		t.Fatalf("active auto-approve pref should let it complete without pausing, got %q", sess.Status)
	}
	if !called {
		t.Fatal("tool should have auto-executed under an active auto_approve_tool preference")
	}
}

func TestPendingPreferenceDoesNotAutoApprove(t *testing.T) {
	called := false
	svc, mem := newAutoApproveSvc(t, &called)
	mem.AddPreference(dto.AgentPreference{ID: "p1", Kind: "auto_approve_tool", Subject: "stop_array", Status: dto.PreferencePending}) // left PENDING
	sess, _ := svc.StartSession(context.Background(), "stop the array")
	if sess.Status != dto.SessionAwaitingApproval {
		t.Fatalf("pending pref must NOT auto-approve; expected awaiting_approval, got %q", sess.Status)
	}
	if called {
		t.Fatal("tool must not execute under a pending preference")
	}
}
