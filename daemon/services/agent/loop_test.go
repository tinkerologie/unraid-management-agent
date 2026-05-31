package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/services/agent/llm"
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
	svc := NewService(cfg, p, reg, NewStore(t.TempDir()), bc)

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
	svc := NewService(cfg, p, tools.BuildDefault(fakeState{}, fakeDocker{}), NewStore(t.TempDir()), &capturingBroadcaster{})

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

func TestLoopHighRiskToolRefused(t *testing.T) {
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
	svc := NewService(cfg, p, reg, NewStore(t.TempDir()), &capturingBroadcaster{})

	sess, err := svc.StartSession(context.Background(), "stop the array")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if called {
		t.Fatal("high-risk tool must NOT be executed under ModeApprove")
	}
	if sess.Status != dto.SessionCompleted {
		t.Fatalf("expected completed, got %q err=%q", sess.Status, sess.Error)
	}
	var rec *dto.AgentToolCall
	for i := range sess.Steps {
		for j := range sess.Steps[i].ToolCalls {
			if sess.Steps[i].ToolCalls[j].Name == "stop_array" {
				rec = &sess.Steps[i].ToolCalls[j]
			}
		}
	}
	if rec == nil {
		t.Fatal("expected a recorded stop_array tool call")
	}
	if rec.Error == "" {
		t.Fatal("expected a non-empty Error indicating approval required")
	}
	if rec.Result == "" {
		t.Fatal("expected a non-empty Result indicating approval required")
	}
}

func TestLoopDisabledAgentReturnsError(t *testing.T) {
	cfg := dto.DefaultAgentConfig() // Enabled defaults to false
	p := llm.NewMockProvider(&llm.ChatResponse{Text: "should never run"})
	svc := NewService(cfg, p, tools.BuildDefault(fakeState{}, fakeDocker{}), NewStore(t.TempDir()), &capturingBroadcaster{})

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
	svc := NewService(cfg, p, tools.BuildDefault(fakeState{}, fakeDocker{}), NewStore(t.TempDir()), &capturingBroadcaster{})

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

func TestLoopProviderErrorFails(t *testing.T) {
	p := llm.NewMockProvider() // empty script → errors on first call
	cfg := dto.DefaultAgentConfig()
	cfg.Enabled = true
	svc := NewService(cfg, p, tools.BuildDefault(fakeState{}, fakeDocker{}), NewStore(t.TempDir()), &capturingBroadcaster{})
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
