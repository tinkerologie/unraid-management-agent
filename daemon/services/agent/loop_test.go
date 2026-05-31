package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/services/agent/llm"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/services/agent/memory"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/services/agent/tools"
)

type fakeState struct{}

func (fakeState) SystemJSON() (any, bool) { return map[string]string{"host": "tower"}, true }
func (fakeState) ArrayJSON() (any, bool)  { return map[string]string{"state": "STARTED"}, true }
func (fakeState) DockerJSON() (any, bool) { return []string{"plex"}, true }

type fakeDocker struct{}

func (fakeDocker) Restart(string) error { return nil }

type capturingBroadcaster struct{ events int }

func (c *capturingBroadcaster) BroadcastAgentEvent(dto.WSEvent) { c.events++ }

func TestLoopToolThenAnswer(t *testing.T) {
	p := llm.NewMockProvider(
		&llm.ChatResponse{ToolCalls: []llm.ToolCall{{ID: "1", Name: "get_system_info", Args: "{}"}}, InputTokens: 10, OutputTokens: 5},
		&llm.ChatResponse{Text: "Host is tower, all healthy.", InputTokens: 8, OutputTokens: 7},
	)
	cfg := dto.DefaultAgentConfig()
	cfg.Enabled = true
	reg := tools.BuildDefault(fakeState{}, fakeDocker{})
	bc := &capturingBroadcaster{}
	svc := NewService(cfg, p, reg, NewStore(t.TempDir()), memory.NewStore(t.TempDir(), 0), bc)

	sess, err := svc.StartSession(context.Background(), "is my system healthy?")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if sess.Status != dto.SessionCompleted {
		t.Fatalf("status=%q err=%q", sess.Status, sess.Error)
	}
	if sess.Answer == "" {
		t.Fatal("expected an answer")
	}
	if sess.TokensUsed != 30 {
		t.Fatalf("tokens=%d want 30", sess.TokensUsed)
	}
	gotCalls := 0
	for _, s := range sess.Steps {
		gotCalls += len(s.ToolCalls)
	}
	if gotCalls != 1 {
		t.Fatalf("expected 1 tool call, got %d", gotCalls)
	}
	if bc.events == 0 {
		t.Fatal("expected broadcast events")
	}
}

func TestLoopHitsIterationCap(t *testing.T) {
	loop := []*llm.ChatResponse{}
	for i := 0; i < 50; i++ {
		loop = append(loop, &llm.ChatResponse{ToolCalls: []llm.ToolCall{{ID: "x", Name: "get_system_info", Args: "{}"}}, OutputTokens: 1})
	}
	p := llm.NewMockProvider(loop...)
	cfg := dto.DefaultAgentConfig()
	cfg.Enabled = true
	cfg.MaxIterations = 3
	svc := NewService(cfg, p, tools.BuildDefault(fakeState{}, fakeDocker{}), NewStore(t.TempDir()), memory.NewStore(t.TempDir(), 0), &capturingBroadcaster{})

	sess, err := svc.StartSession(context.Background(), "loop forever")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if len(sess.Steps) != 3 {
		t.Fatalf("expected 3 steps, got %d", len(sess.Steps))
	}
	if sess.Status != dto.SessionCompleted {
		t.Fatalf("expected completed (truncated), got %q", sess.Status)
	}
}

func TestLoopHighRiskToolPauses(t *testing.T) {
	called := false
	reg := tools.NewRegistry()
	reg.Register(tools.Tool{
		Name:     "stop_array",
		RiskTier: dto.RiskHigh,
		Invoke: func(_ context.Context, _ string) (string, error) {
			called = true
			return "", nil
		},
	})

	p := llm.NewMockProvider(
		&llm.ChatResponse{ToolCalls: []llm.ToolCall{{ID: "1", Name: "stop_array", Args: "{}"}}, OutputTokens: 1},
		&llm.ChatResponse{Text: "I cannot stop the array without approval.", OutputTokens: 1},
	)
	cfg := dto.DefaultAgentConfig() // RiskHigh -> ModeApprove
	cfg.Enabled = true
	svc := NewService(cfg, p, reg, NewStore(t.TempDir()), memory.NewStore(t.TempDir(), 0), &capturingBroadcaster{})

	sess, err := svc.StartSession(context.Background(), "stop the array")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if called {
		t.Fatal("high-risk tool must NOT be executed under ModeApprove")
	}
	if sess.Status != dto.SessionAwaitingApproval {
		t.Fatalf("expected awaiting_approval, got %q err=%q", sess.Status, sess.Error)
	}
	if sess.PendingApproval == nil || sess.PendingApproval.ToolName != "stop_array" {
		t.Fatalf("expected pending approval for stop_array, got %+v", sess.PendingApproval)
	}
}

func TestLoopDisabledAgentReturnsError(t *testing.T) {
	cfg := dto.DefaultAgentConfig() // Enabled defaults to false
	p := llm.NewMockProvider(&llm.ChatResponse{Text: "should never run"})
	svc := NewService(cfg, p, tools.BuildDefault(fakeState{}, fakeDocker{}), NewStore(t.TempDir()), memory.NewStore(t.TempDir(), 0), &capturingBroadcaster{})

	sess, err := svc.StartSession(context.Background(), "do something")
	if err == nil {
		t.Fatal("expected an error when agent is disabled")
	}
	if sess.Status != "" {
		t.Fatalf("expected zero-value session, got status %q", sess.Status)
	}
}

func TestLoopTokenBudgetStops(t *testing.T) {
	p := llm.NewMockProvider(
		// First response alone blows the 5-token budget while still asking for a tool.
		&llm.ChatResponse{ToolCalls: []llm.ToolCall{{ID: "1", Name: "get_system_info", Args: "{}"}}, InputTokens: 10, OutputTokens: 10},
	)
	cfg := dto.DefaultAgentConfig()
	cfg.Enabled = true
	cfg.MaxTokensPerSession = 5
	cfg.MaxIterations = 12
	svc := NewService(cfg, p, tools.BuildDefault(fakeState{}, fakeDocker{}), NewStore(t.TempDir()), memory.NewStore(t.TempDir(), 0), &capturingBroadcaster{})

	sess, err := svc.StartSession(context.Background(), "use lots of tokens")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if sess.Status != dto.SessionCompleted {
		t.Fatalf("expected completed, got %q err=%q", sess.Status, sess.Error)
	}
	if !strings.Contains(strings.ToLower(sess.Answer), "token budget") {
		t.Fatalf("expected answer to mention token budget, got %q", sess.Answer)
	}
	if len(sess.Steps) >= cfg.MaxIterations {
		t.Fatalf("expected to stop before MaxIterations, got %d steps", len(sess.Steps))
	}
}

func TestLoopPausesForApproval(t *testing.T) {
	p := llm.NewMockProvider(
		&llm.ChatResponse{ToolCalls: []llm.ToolCall{{ID: "tu1", Name: "stop_array", Args: "{}"}}, OutputTokens: 3},
	)
	cfg := dto.DefaultAgentConfig()
	cfg.Enabled = true
	reg := tools.NewRegistry()
	called := false
	reg.Register(tools.Tool{Name: "stop_array", RiskTier: dto.RiskHigh,
		Invoke: func(_ context.Context, _ string) (string, error) { called = true; return "stopped", nil }})
	svc := NewService(cfg, p, reg, NewStore(t.TempDir()), memory.NewStore(t.TempDir(), 0), &capturingBroadcaster{})

	sess, err := svc.StartSession(context.Background(), "stop the array")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if sess.Status != dto.SessionAwaitingApproval {
		t.Fatalf("status=%q want awaiting_approval", sess.Status)
	}
	if sess.PendingApproval == nil || sess.PendingApproval.ToolName != "stop_array" {
		t.Fatalf("pending approval missing: %+v", sess.PendingApproval)
	}
	if called {
		t.Fatal("high-risk tool must NOT execute before approval")
	}
	if len(sess.Transcript) == 0 {
		t.Fatal("transcript must be persisted for resume")
	}
}

func TestLoopForbidListRefusesEvenIfAuto(t *testing.T) {
	p := llm.NewMockProvider(
		&llm.ChatResponse{ToolCalls: []llm.ToolCall{{ID: "f1", Name: "format_disk", Args: "{}"}}, OutputTokens: 2},
		&llm.ChatResponse{Text: "I won't do that.", OutputTokens: 2},
	)
	cfg := dto.DefaultAgentConfig()
	cfg.Enabled = true
	cfg.Autonomy[dto.RiskHigh] = dto.ModeAuto // even if mis-set to auto...
	cfg.ForbidList = []string{"format_disk"}  // ...the forbid-list still wins (set explicitly, not via defaults)
	reg := tools.NewRegistry()
	called := false
	reg.Register(tools.Tool{Name: "format_disk", RiskTier: dto.RiskHigh,
		Invoke: func(_ context.Context, _ string) (string, error) { called = true; return "", nil }})
	svc := NewService(cfg, p, reg, NewStore(t.TempDir()), memory.NewStore(t.TempDir(), 0), &capturingBroadcaster{})

	sess, err := svc.StartSession(context.Background(), "format disk1")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if called {
		t.Fatal("forbid-list tool must never execute")
	}
	if sess.Status != dto.SessionCompleted {
		t.Fatalf("status=%q want completed (loop continues after refusal)", sess.Status)
	}
}

func TestLoopAutoThenApprovalPreservesFirstResult(t *testing.T) {
	p := llm.NewMockProvider(
		&llm.ChatResponse{ToolCalls: []llm.ToolCall{
			{ID: "a1", Name: "get_system_info", Args: "{}"},
			{ID: "a2", Name: "stop_array", Args: "{}"},
		}, OutputTokens: 3},
	)
	cfg := dto.DefaultAgentConfig()
	cfg.Enabled = true
	reg := tools.BuildDefault(fakeState{}, fakeDocker{}) // provides get_system_info (read-only)
	stopped := false
	reg.Register(tools.Tool{Name: "stop_array", RiskTier: dto.RiskHigh,
		Invoke: func(_ context.Context, _ string) (string, error) { stopped = true; return "stopped", nil }})
	svc := NewService(cfg, p, reg, NewStore(t.TempDir()), memory.NewStore(t.TempDir(), 0), &capturingBroadcaster{})

	sess, err := svc.StartSession(context.Background(), "check then stop")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if sess.Status != dto.SessionAwaitingApproval {
		t.Fatalf("want awaiting_approval, got %q", sess.Status)
	}
	if stopped {
		t.Fatal("stop_array must not execute before approval")
	}
	if sess.PendingApproval == nil || sess.PendingApproval.ActionID != "a2" {
		t.Fatalf("pending approval should target call a2: %+v", sess.PendingApproval)
	}
	foundA1 := false
	for _, m := range sess.Transcript {
		if m.Role == "tool" && m.ToolCallID == "a1" {
			foundA1 = true
		}
	}
	if !foundA1 {
		t.Fatal("first (auto) tool result must be preserved in transcript before pause")
	}
}

func TestLoopProviderErrorFails(t *testing.T) {
	p := llm.NewMockProvider() // empty script → errors on first call
	cfg := dto.DefaultAgentConfig()
	cfg.Enabled = true
	svc := NewService(cfg, p, tools.BuildDefault(fakeState{}, fakeDocker{}), NewStore(t.TempDir()), memory.NewStore(t.TempDir(), 0), &capturingBroadcaster{})
	sess, err := svc.StartSession(context.Background(), "anything")
	if err != nil {
		t.Fatalf("StartSession itself should not error: %v", err)
	}
	if sess.Status != dto.SessionFailed {
		t.Fatalf("expected failed, got %q", sess.Status)
	}
	if sess.Error == "" {
		t.Fatal("expected non-empty Error field")
	}
}
