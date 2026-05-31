package agent

import (
	"context"
	"testing"
	"time"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/services/agent/llm"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/services/agent/tools"
)

func wakeSvc(t *testing.T) *Service {
	t.Helper()
	p := llm.NewMockProvider() // empty script: autonomous session will fail-fast on first Chat; that's fine for admission tests
	cfg := dto.DefaultAgentConfig()
	cfg.Enabled = true
	reg := tools.BuildDefault(fakeState{}, fakeDocker{})
	return NewService(cfg, p, reg, NewStore(t.TempDir()), &capturingBroadcaster{})
}

func TestHandleWakeDedupsBySubsystem(t *testing.T) {
	svc := wakeSvc(t)
	ev := dto.AgentWakeEvent{Source: "alert", Subsystem: "disk", Title: "disk hot", At: time.Now()}
	if !svc.handleWake(context.Background(), ev) {
		t.Fatal("first wake should spawn")
	}
	if svc.handleWake(context.Background(), ev) {
		t.Fatal("duplicate wake within debounce window should be skipped")
	}
}

func TestHandleWakeRespectsConcurrencyCap(t *testing.T) {
	svc := wakeSvc(t)
	svc.cfg.MaxConcurrentSessions = 0
	if svc.handleWake(context.Background(), dto.AgentWakeEvent{Subsystem: "x", At: time.Now()}) {
		t.Fatal("should not spawn when concurrency cap is 0")
	}
}

func TestStartNoOpWhenNoHub(t *testing.T) {
	svc := wakeSvc(t) // no SetEventBus called
	// Start must return promptly (no hub). Use a cancellable ctx as a safety net.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	done := make(chan struct{})
	go func() { svc.Start(ctx); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Start should return immediately when no hub is wired")
	}
}
