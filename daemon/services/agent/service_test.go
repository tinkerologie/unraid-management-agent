package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tinkerologie/unraid-management-agent/daemon/dto"
	"github.com/tinkerologie/unraid-management-agent/daemon/services/agent/llm"
	"github.com/tinkerologie/unraid-management-agent/daemon/services/agent/memory"
	"github.com/tinkerologie/unraid-management-agent/daemon/services/agent/tools"
)

// TestNextIDResumesAfterRestart verifies that a Service built over a store with
// an existing "sess-5" resumes numbering at sess-6 rather than colliding at sess-1.
func TestNextIDResumesAfterRestart(t *testing.T) {
	store := NewStore(t.TempDir())
	store.Put(dto.AgentSession{ID: "sess-5", Goal: "old", Status: dto.SessionCompleted, StartedAt: time.Now()})

	cfg := dto.DefaultAgentConfig()
	cfg.Enabled = true
	provider := llm.NewMockProvider(
		&llm.ChatResponse{Text: "[]"},
		&llm.ChatResponse{Text: "done", OutputTokens: 1},
	)
	reg := tools.BuildDefault(nil, nil)

	svc := NewService(cfg, provider, reg, store, memory.NewStore(t.TempDir(), 0), nil)
	sess, err := svc.StartSession(context.Background(), "check status")
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	if sess.ID != "sess-6" {
		t.Fatalf("expected resumed ID sess-6, got %q", sess.ID)
	}
}

func pausedSvc(t *testing.T, toolCalled *bool) *Service {
	t.Helper()
	p := llm.NewMockProvider(
		&llm.ChatResponse{Text: "[]"},
		&llm.ChatResponse{ToolCalls: []llm.ToolCall{{ID: "tu1", Name: "stop_array", Args: "{}"}}, OutputTokens: 2},
		&llm.ChatResponse{Text: "Array stopped, all done.", OutputTokens: 2}, // returned after resume
	)
	cfg := dto.DefaultAgentConfig()
	cfg.Enabled = true
	reg := tools.NewRegistry()
	reg.Register(tools.Tool{Name: "stop_array", RiskTier: dto.RiskHigh,
		Invoke: func(_ context.Context, _ string) (string, error) { *toolCalled = true; return "stopped", nil }})
	return NewService(cfg, p, reg, NewStore(t.TempDir()), memory.NewStore(t.TempDir(), 0), &capturingBroadcaster{})
}

func TestApproveExecutesAndCompletes(t *testing.T) {
	called := false
	svc := pausedSvc(t, &called)
	sess, _ := svc.StartSession(context.Background(), "stop array")
	if sess.Status != dto.SessionAwaitingApproval {
		t.Fatalf("precondition: want awaiting_approval, got %q", sess.Status)
	}
	out, err := svc.ApproveAction(context.Background(), sess.ID, sess.PendingApproval.ActionID, true)
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	if !called {
		t.Fatal("approved tool should have executed")
	}
	if out.Status != dto.SessionCompleted || out.PendingApproval != nil {
		t.Fatalf("want completed + cleared approval, got %q / %+v", out.Status, out.PendingApproval)
	}
}

func TestDenyDoesNotExecute(t *testing.T) {
	called := false
	svc := pausedSvc(t, &called)
	sess, _ := svc.StartSession(context.Background(), "stop array")
	out, err := svc.ApproveAction(context.Background(), sess.ID, sess.PendingApproval.ActionID, false)
	if err != nil {
		t.Fatalf("deny: %v", err)
	}
	if called {
		t.Fatal("denied tool must not execute")
	}
	if out.Status != dto.SessionCompleted {
		t.Fatalf("want completed after deny+continue, got %q", out.Status)
	}
}

func TestApproveWrongActionIDErrors(t *testing.T) {
	called := false
	svc := pausedSvc(t, &called)
	sess, _ := svc.StartSession(context.Background(), "stop array")
	if _, err := svc.ApproveAction(context.Background(), sess.ID, "wrong-id", true); err == nil {
		t.Fatal("expected error on action_id mismatch")
	}
}

func TestApproveRefusesToolBlockedByCurrentPolicy(t *testing.T) {
	const (
		actionID = "restart-1"
		toolName = "restart_container"
	)
	for _, tc := range []struct {
		name           string
		currentTier    dto.RiskTier
		currentMode    dto.AutonomyMode
		forbiddenNames []string
	}{
		{name: "tier becomes forbidden", currentTier: dto.RiskLow, currentMode: dto.ModeForbid},
		{name: "tool moves to forbidden tier", currentTier: dto.RiskHigh, currentMode: dto.ModeForbid},
		{name: "tool name is added to forbid-list", currentTier: dto.RiskLow, currentMode: dto.ModeApprove, forbiddenNames: []string{toolName}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			configDir := t.TempDir()
			docker := &recordingDocker{}
			reg := tools.BuildDefault(fakeState{}, docker)
			cfg := dto.DefaultAgentConfig()
			cfg.Enabled = true
			cfg.Autonomy[dto.RiskLow] = dto.ModeApprove
			startProvider := llm.NewMockProvider(
				&llm.ChatResponse{Text: "[]"},
				&llm.ChatResponse{ToolCalls: []llm.ToolCall{{ID: actionID, Name: toolName, Args: `{"container_id":"0123456789ab"}`}}},
			)
			svc := NewService(cfg, startProvider, reg, NewStore(configDir), nil, nil)
			paused, err := svc.StartSession(ctx, "restart plex")
			if err != nil {
				t.Fatalf("start: %v", err)
			}
			if paused.Status != dto.SessionAwaitingApproval || paused.PendingApproval == nil ||
				paused.PendingApproval.ActionID != actionID || paused.PendingApproval.RiskTier != dto.RiskLow {
				t.Fatalf("want a pending low-risk approval for %q, got %q / %+v", actionID, paused.Status, paused.PendingApproval)
			}

			store := NewStore(configDir)
			if err := store.Load(); err != nil {
				t.Fatalf("load session after restart: %v", err)
			}
			currentTool, ok := reg.Get(toolName)
			if !ok {
				t.Fatalf("missing tool %q", toolName)
			}
			currentTool.RiskTier = tc.currentTier
			reg.Register(currentTool)
			cfg.Autonomy[tc.currentTier] = tc.currentMode
			cfg.ForbidList = append(cfg.ForbidList, tc.forbiddenNames...)
			resumeProvider := llm.NewMockProvider(&llm.ChatResponse{Text: "The restart was blocked."})
			resumed := NewService(cfg, resumeProvider, reg, store, nil, nil)
			out, err := resumed.ApproveAction(ctx, paused.ID, actionID, true)
			if err != nil {
				t.Fatalf("approve: %v", err)
			}
			if len(docker.restarts) != 0 {
				t.Fatalf("forbidden restart reached Docker after approval: %v", docker.restarts)
			}
			if out.Status != dto.SessionCompleted || out.PendingApproval != nil {
				t.Fatalf("want completed with approval cleared, got %q / %+v", out.Status, out.PendingApproval)
			}
			assertForbiddenToolResult(t, out, actionID)
		})
	}
}

func TestCancelSession(t *testing.T) {
	called := false
	svc := pausedSvc(t, &called)
	sess, _ := svc.StartSession(context.Background(), "stop array")
	out, err := svc.CancelSession(sess.ID)
	if err != nil || out.Status != dto.SessionCancelled {
		t.Fatalf("cancel: status=%q err=%v", out.Status, err)
	}
}

func TestApproveResumesMultiToolTurn(t *testing.T) {
	ctx := context.Background()
	toolIDs := []string{"before", "approve", "between", "deny", "blocked", "after"}
	var requests atomic.Int32
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch requests.Add(1) {
		case 1: // Goal planner
			_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"[]"}]}`))
		case 2:
			_, _ = w.Write([]byte(`{"content":[
				{"type":"tool_use","id":"before","name":"check","input":{}},
				{"type":"tool_use","id":"approve","name":"restart_container","input":{"container_id":"0123456789ab"}},
				{"type":"tool_use","id":"between","name":"check","input":{}},
				{"type":"tool_use","id":"deny","name":"restart_container","input":{"container_id":"0123456789cd"}},
				{"type":"tool_use","id":"blocked","name":"restricted","input":{}},
				{"type":"tool_use","id":"after","name":"check","input":{}}
			]}`))
		case 3:
			var req struct {
				Messages []struct {
					Role    string
					Content json.RawMessage
				}
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.Messages) != 3 {
				http.Error(w, "expected one user message for all tool results", http.StatusBadRequest)
				return
			}
			var blocks []struct {
				Type    string
				ID      string `json:"tool_use_id"`
				Content string
			}
			results := req.Messages[2]
			if results.Role != "user" || json.Unmarshal(results.Content, &blocks) != nil || len(blocks) != len(toolIDs) {
				http.Error(w, "incomplete tool results", http.StatusBadRequest)
				return
			}
			for i, block := range blocks {
				if block.Type != "tool_result" || block.ID != toolIDs[i] {
					http.Error(w, "misordered tool results", http.StatusBadRequest)
					return
				}
			}
			if !strings.Contains(blocks[1].Content, "restarted") || !strings.Contains(blocks[3].Content, "denied") {
				http.Error(w, "incorrect approval results", http.StatusBadRequest)
				return
			}
			if !strings.Contains(blocks[4].Content, "forbidden") {
				http.Error(w, "missing refusal for the forbidden call", http.StatusBadRequest)
				return
			}
			_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"Finished the checks."}]}`))
		default:
			http.Error(w, "unexpected provider request", http.StatusBadRequest)
		}
	}))
	defer provider.Close()

	docker := &recordingDocker{}
	reg := tools.BuildDefault(fakeState{}, docker)
	checks := 0
	reg.Register(tools.Tool{Name: "check", RiskTier: dto.RiskReadOnly, Invoke: func(context.Context, string) (string, error) {
		checks++
		return "checked", nil
	}})
	reg.Register(tools.Tool{Name: "restricted", RiskTier: dto.RiskHigh, Invoke: func(context.Context, string) (string, error) {
		t.Error("the forbidden call reached its tool")
		return "executed", nil
	}})
	cfg := dto.DefaultAgentConfig()
	cfg.Enabled = true
	cfg.Autonomy[dto.RiskLow] = dto.ModeApprove
	cfg.Autonomy[dto.RiskHigh] = dto.ModeForbid
	configDir := t.TempDir()
	backend := llm.NewAnthropicProvider("test-key", "test-model", provider.URL)
	svc := NewService(cfg, backend, reg, NewStore(configDir), nil, nil)
	sess, err := svc.StartSession(ctx, "check and restart containers")
	if err != nil || sess.Status != dto.SessionAwaitingApproval || sess.PendingApproval == nil ||
		sess.PendingApproval.ActionID != "approve" || checks != 1 || len(docker.restarts) != 0 {
		t.Fatalf("first pause: status=%q pending=%+v checks=%d restarts=%v err=%v",
			sess.Status, sess.PendingApproval, checks, docker.restarts, err)
	}

	store := NewStore(configDir)
	if err := store.Load(); err != nil {
		t.Fatalf("reload paused session: %v", err)
	}
	svc = NewService(cfg, backend, reg, store, nil, nil)
	sess, err = svc.ApproveAction(ctx, sess.ID, "approve", true)
	if err != nil || sess.Status != dto.SessionAwaitingApproval || sess.PendingApproval == nil ||
		sess.PendingApproval.ActionID != "deny" || checks != 2 || !slices.Equal(docker.restarts, []string{"0123456789ab"}) {
		t.Fatalf("second pause: status=%q pending=%+v checks=%d restarts=%v err=%v provider error=%q",
			sess.Status, sess.PendingApproval, checks, docker.restarts, err, sess.Error)
	}

	sess, err = svc.ApproveAction(ctx, sess.ID, "deny", false)
	if err != nil || sess.Status != dto.SessionCompleted || sess.PendingApproval != nil || sess.Answer != "Finished the checks." {
		t.Fatalf("resume: status=%q pending=%+v answer=%q err=%v provider error=%q",
			sess.Status, sess.PendingApproval, sess.Answer, err, sess.Error)
	}
	if checks != 3 || !slices.Equal(docker.restarts, []string{"0123456789ab"}) {
		t.Fatalf("want each check once and only the approved restart, got %d checks and %v restarts", checks, docker.restarts)
	}
	if len(sess.Steps) != 2 || len(sess.Steps[0].ToolCalls) != len(toolIDs) {
		t.Fatalf("want one tool step and one final answer step, got %+v", sess.Steps)
	}
}

func TestConcurrentRequestsCannotResolveTheSameApproval(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	svc := pausedSvc(t, new(bool))
	entered := make(chan struct{})
	release := make(chan struct{})
	var invocations atomic.Int32
	svc.tools.Register(tools.Tool{Name: "stop_array", RiskTier: dto.RiskHigh, Invoke: func(ctx context.Context, _ string) (string, error) {
		if invocations.Add(1) == 1 {
			close(entered)
			select {
			case <-release:
			case <-ctx.Done():
				return "", ctx.Err()
			}
		}
		return "stopped", nil
	}})
	sess, err := svc.StartSession(ctx, "stop array")
	if err != nil || sess.PendingApproval == nil {
		t.Fatalf("start: pending=%+v err=%v", sess.PendingApproval, err)
	}
	actionID := sess.PendingApproval.ActionID
	done := make(chan error, 1)
	go func() {
		_, err := svc.ApproveAction(ctx, sess.ID, actionID, true)
		done <- err
	}()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("approved tool did not start")
	}

	_, duplicateErr := svc.ApproveAction(ctx, sess.ID, actionID, true)
	_, cancelErr := svc.CancelSession(sess.ID)
	close(release)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("first approval: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("approval did not finish")
	}
	if duplicateErr == nil || cancelErr == nil || invocations.Load() != 1 {
		t.Fatalf("want one invocation and rejected competing requests, got invocations=%d approval=%v cancel=%v",
			invocations.Load(), duplicateErr, cancelErr)
	}
}

func TestCancelTerminalSessionErrors(t *testing.T) {
	called := false
	svc := pausedSvc(t, &called)
	sess, _ := svc.StartSession(context.Background(), "stop array")
	// First cancel succeeds (session was awaiting_approval).
	if _, err := svc.CancelSession(sess.ID); err != nil {
		t.Fatalf("first cancel: %v", err)
	}
	// Second cancel must error (already cancelled) and not re-mutate.
	if _, err := svc.CancelSession(sess.ID); err == nil {
		t.Fatal("expected error cancelling an already-terminal session")
	}
}

func TestSweepExpiredApprovals(t *testing.T) {
	called := false
	svc := pausedSvc(t, &called)
	svc.cfg.ApprovalTTLSecs = 60
	sess, _ := svc.StartSession(context.Background(), "stop array")
	if sess.Status != dto.SessionAwaitingApproval {
		t.Fatalf("precondition: want awaiting_approval, got %q", sess.Status)
	}
	// Backdate the pending approval beyond TTL and persist.
	sess.PendingApproval.RequestedAt = time.Now().Add(-2 * time.Hour)
	svc.store.Put(sess)

	n := svc.SweepExpiredApprovals(context.Background(), time.Now())
	if n != 1 {
		t.Fatalf("expected 1 swept, got %d", n)
	}
	if called {
		t.Fatal("expired approval must NOT execute the tool")
	}
	out, _ := svc.GetSession(sess.ID)
	if out.PendingApproval != nil || out.Status == dto.SessionAwaitingApproval {
		t.Fatalf("expired session should no longer await approval: %+v", out)
	}
}

func TestSweepWithinTTLUntouched(t *testing.T) {
	called := false
	svc := pausedSvc(t, &called)
	svc.cfg.ApprovalTTLSecs = 3600
	sess, _ := svc.StartSession(context.Background(), "stop array")
	if n := svc.SweepExpiredApprovals(context.Background(), time.Now()); n != 0 {
		t.Fatalf("expected 0 swept within TTL, got %d", n)
	}
	out, _ := svc.GetSession(sess.ID)
	if out.Status != dto.SessionAwaitingApproval {
		t.Fatalf("within-TTL session should remain awaiting, got %q", out.Status)
	}
}

func TestSendMessageContinuesSession(t *testing.T) {
	p := llm.NewMockProvider(
		&llm.ChatResponse{Text: "[]"},                                         // planner (StartSession)
		&llm.ChatResponse{Text: "Plex is healthy.", OutputTokens: 2},          // first answer
		&llm.ChatResponse{Text: "Yes, it restarted 2h ago.", OutputTokens: 2}, // follow-up answer
	)
	cfg := dto.DefaultAgentConfig()
	cfg.Enabled = true
	svc := NewService(cfg, p, tools.BuildDefault(fakeState{}, fakeDocker{}), NewStore(t.TempDir()), memory.NewStore(t.TempDir(), 0), &capturingBroadcaster{})
	sess, _ := svc.StartSession(context.Background(), "is plex healthy?")
	if sess.Status != dto.SessionCompleted {
		t.Fatalf("precondition: %q", sess.Status)
	}
	out, err := svc.SendMessage(context.Background(), sess.ID, "did it restart recently?")
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if out.Status != dto.SessionCompleted || out.Answer != "Yes, it restarted 2h ago." {
		t.Fatalf("follow-up not handled: %q / %q", out.Status, out.Answer)
	}
}

func TestSendMessageRejectsAwaitingOrEmpty(t *testing.T) {
	p := llm.NewMockProvider(&llm.ChatResponse{Text: "[]"}, &llm.ChatResponse{ToolCalls: []llm.ToolCall{{ID: "t1", Name: "stop_array", Args: "{}"}}})
	cfg := dto.DefaultAgentConfig()
	cfg.Enabled = true
	reg := tools.NewRegistry()
	reg.Register(tools.Tool{Name: "stop_array", RiskTier: dto.RiskHigh, Invoke: func(_ context.Context, _ string) (string, error) { return "", nil }})
	svc := NewService(cfg, p, reg, NewStore(t.TempDir()), memory.NewStore(t.TempDir(), 0), &capturingBroadcaster{})
	sess, _ := svc.StartSession(context.Background(), "stop array")
	if sess.Status != dto.SessionAwaitingApproval {
		t.Fatalf("precondition: %q", sess.Status)
	}
	if _, err := svc.SendMessage(context.Background(), sess.ID, "hi"); err == nil {
		t.Fatal("expected error continuing an awaiting_approval session")
	}

	p2 := llm.NewMockProvider(&llm.ChatResponse{Text: "[]"}, &llm.ChatResponse{Text: "done"})
	svc2 := NewService(cfg, p2, tools.BuildDefault(fakeState{}, fakeDocker{}), NewStore(t.TempDir()), memory.NewStore(t.TempDir(), 0), &capturingBroadcaster{})
	s2, _ := svc2.StartSession(context.Background(), "x")
	if _, err := svc2.SendMessage(context.Background(), s2.ID, "  "); err == nil {
		t.Fatal("expected error on empty message")
	}
	if _, err := svc2.SendMessage(context.Background(), "no-such-session", "hi"); err == nil {
		t.Fatal("expected error for unknown session")
	}
}
