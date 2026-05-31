package agent

import (
	"context"
	"testing"
	"time"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/services/agent/llm"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/services/agent/tools"
)

// TestNextIDResumesAfterRestart verifies that a Service built over a store with
// an existing "sess-5" resumes numbering at sess-6 rather than colliding at sess-1.
func TestNextIDResumesAfterRestart(t *testing.T) {
	store := NewStore(t.TempDir())
	store.Put(dto.AgentSession{ID: "sess-5", Goal: "old", Status: dto.SessionCompleted, StartedAt: time.Now()})

	cfg := dto.DefaultAgentConfig()
	cfg.Enabled = true
	provider := llm.NewMockProvider(&llm.ChatResponse{Text: "done", OutputTokens: 1})
	reg := tools.BuildDefault(nil, nil)

	svc := NewService(cfg, provider, reg, store, nil)
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
		&llm.ChatResponse{ToolCalls: []llm.ToolCall{{ID: "tu1", Name: "stop_array", Args: "{}"}}, OutputTokens: 2},
		&llm.ChatResponse{Text: "Array stopped, all done.", OutputTokens: 2}, // returned after resume
	)
	cfg := dto.DefaultAgentConfig()
	cfg.Enabled = true
	reg := tools.NewRegistry()
	reg.Register(tools.Tool{Name: "stop_array", RiskTier: dto.RiskHigh,
		Invoke: func(_ context.Context, _ string) (string, error) { *toolCalled = true; return "stopped", nil }})
	return NewService(cfg, p, reg, NewStore(t.TempDir()), &capturingBroadcaster{})
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

func TestApproveForbiddenStillRefused(t *testing.T) {
	called := false
	p := llm.NewMockProvider(
		&llm.ChatResponse{ToolCalls: []llm.ToolCall{{ID: "tu1", Name: "format_disk", Args: "{}"}}, OutputTokens: 2},
		&llm.ChatResponse{Text: "ok, won't.", OutputTokens: 1},
	)
	cfg := dto.DefaultAgentConfig()
	cfg.Enabled = true
	cfg.Autonomy[dto.RiskHigh] = dto.ModeApprove
	cfg.ForbidList = []string{"format_disk"}
	reg := tools.NewRegistry()
	reg.Register(tools.Tool{Name: "format_disk", RiskTier: dto.RiskHigh,
		Invoke: func(_ context.Context, _ string) (string, error) { called = true; return "", nil }})
	svc := NewService(cfg, p, reg, NewStore(t.TempDir()), &capturingBroadcaster{})
	// format_disk is forbidden, so the loop refuses it inline and never pauses; session completes.
	sess, _ := svc.StartSession(context.Background(), "format disk1")
	if called {
		t.Fatal("forbidden tool must never execute")
	}
	if sess.Status == dto.SessionAwaitingApproval {
		t.Fatal("forbidden tool should not create an approval request")
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
