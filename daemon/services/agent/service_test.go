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
