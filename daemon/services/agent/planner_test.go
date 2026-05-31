package agent

import (
	"context"
	"testing"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/services/agent/llm"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/services/agent/memory"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/services/agent/tools"
)

func TestPlanParsesSteps(t *testing.T) {
	p := llm.NewMockProvider(
		&llm.ChatResponse{Text: `[{"intent":"check array","tool":"get_array_status"},{"intent":"answer"}]`, OutputTokens: 5},
		&llm.ChatResponse{Text: "done", OutputTokens: 1},
	)
	cfg := dto.DefaultAgentConfig()
	cfg.Enabled = true
	svc := NewService(cfg, p, tools.BuildDefault(fakeState{}, fakeDocker{}), NewStore(t.TempDir()), memory.NewStore(t.TempDir(), 0), &capturingBroadcaster{})
	sess, _ := svc.StartSession(context.Background(), "is the array ok?")
	if len(sess.Plan) != 2 || sess.Plan[0].Intent != "check array" {
		t.Fatalf("plan not populated: %+v", sess.Plan)
	}
}

func TestPlanGarbageIgnored(t *testing.T) {
	p := llm.NewMockProvider(
		&llm.ChatResponse{Text: "not json at all", OutputTokens: 2},
		&llm.ChatResponse{Text: "done", OutputTokens: 1},
	)
	cfg := dto.DefaultAgentConfig()
	cfg.Enabled = true
	svc := NewService(cfg, p, tools.BuildDefault(fakeState{}, fakeDocker{}), NewStore(t.TempDir()), memory.NewStore(t.TempDir(), 0), &capturingBroadcaster{})
	sess, _ := svc.StartSession(context.Background(), "anything")
	if len(sess.Plan) != 0 {
		t.Fatalf("garbage plan should be ignored, got %+v", sess.Plan)
	}
	if sess.Status != dto.SessionCompleted {
		t.Fatalf("session should still complete, got %q", sess.Status)
	}
}
