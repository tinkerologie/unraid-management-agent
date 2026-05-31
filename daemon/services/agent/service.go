package agent

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/services/agent/llm"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/services/agent/tools"
)

// Broadcaster streams agent events to WebSocket clients. Satisfied by the API server.
type Broadcaster interface {
	BroadcastAgentEvent(event dto.WSEvent)
}

// Service is the agent facade used by the API layer.
type Service struct {
	cfg      dto.AgentConfig
	provider llm.Provider
	tools    *tools.Registry
	store    *Store
	bc       Broadcaster

	mu  sync.Mutex
	seq int
}

// NewService constructs the agent service.
func NewService(cfg dto.AgentConfig, provider llm.Provider, reg *tools.Registry, store *Store, bc Broadcaster) *Service {
	return &Service{cfg: cfg, provider: provider, tools: reg, store: store, bc: bc}
}

// Enabled reports whether the agent is configured to run.
func (s *Service) Enabled() bool { return s.cfg.Enabled && s.provider != nil }

// nextID returns a monotonically increasing session ID (deterministic, no clock dependency).
func (s *Service) nextID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seq++
	return fmt.Sprintf("sess-%d", s.seq)
}

// GetSession returns a stored session by ID.
func (s *Service) GetSession(id string) (dto.AgentSession, bool) { return s.store.Get(id) }

// ListSessions returns all sessions newest-first.
func (s *Service) ListSessions() []dto.AgentSession { return s.store.List() }

// StartSession runs a new agent session to completion (synchronous in Phase 1).
func (s *Service) StartSession(ctx context.Context, goal string) (dto.AgentSession, error) {
	if !s.Enabled() {
		return dto.AgentSession{}, errors.New("agent is disabled")
	}
	sess := s.runLoop(ctx, s.nextID(), goal)
	s.store.Put(sess)
	if err := s.store.Save(); err != nil {
		logger.Warning("Agent: failed to persist session %s: %v", sess.ID, err)
	}
	return sess, nil
}
