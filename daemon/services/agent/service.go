package agent

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/services/agent/llm"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/services/agent/memory"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/services/agent/tools"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/services/remediation"
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
	memory   *memory.Store
	runbooks *remediation.RunbookStore
	bc       Broadcaster

	mu      sync.Mutex
	seq     int
	prefSeq int

	hub        *domain.EventBus
	wakeMu     sync.Mutex
	lastWake   map[string]time.Time
	activeAuto int
}

// NewService constructs the agent service.
func NewService(cfg dto.AgentConfig, provider llm.Provider, reg *tools.Registry, store *Store, mem *memory.Store, bc Broadcaster) *Service {
	s := &Service{cfg: cfg, provider: provider, tools: reg, store: store, memory: mem, bc: bc, lastWake: map[string]time.Time{}}
	// Resume the session counter past any persisted IDs so a restart does not
	// reuse "sess-1" and overwrite an existing session.
	for _, sess := range store.List() {
		if n, ok := parseSessionSeq(sess.ID); ok && n > s.seq {
			s.seq = n
		}
	}
	return s
}

// parseSessionSeq extracts N from an ID of the form "sess-N".
func parseSessionSeq(id string) (int, bool) {
	const prefix = "sess-"
	if !strings.HasPrefix(id, prefix) {
		return 0, false
	}
	n, err := strconv.Atoi(strings.TrimPrefix(id, prefix))
	if err != nil {
		return 0, false
	}
	return n, true
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

// SetRunbookStore wires the persistent runbook store for runbook proposals.
func (s *Service) SetRunbookStore(rs *remediation.RunbookStore) { s.runbooks = rs }

// nextPrefSeq returns a monotonically increasing preference counter.
func (s *Service) nextPrefSeq() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prefSeq++
	return s.prefSeq
}

// ConfirmPreference activates a pending learned preference.
func (s *Service) ConfirmPreference(id string) error {
	if s.memory == nil {
		return fmt.Errorf("memory disabled")
	}
	if err := s.memory.ConfirmPreference(id); err != nil {
		return err
	}
	return s.memory.Save()
}

// autoApprovedByPreference reports whether an active auto_approve_tool preference
// covers the named tool. (Forbidden tools are filtered earlier in the loop.)
func (s *Service) autoApprovedByPreference(toolName string) bool {
	if s.memory == nil {
		return false
	}
	for _, p := range s.memory.ActivePreferences() {
		if p.Kind == "auto_approve_tool" && p.Subject == toolName {
			return true
		}
	}
	return false
}

// GetSession returns a stored session by ID.
func (s *Service) GetSession(id string) (dto.AgentSession, bool) { return s.store.Get(id) }

// ListSessions returns all sessions newest-first.
func (s *Service) ListSessions() []dto.AgentSession { return s.store.List() }

// StartSession runs a new agent session until it completes, fails, or pauses
// awaiting approval.
func (s *Service) StartSession(ctx context.Context, goal string) (dto.AgentSession, error) {
	if !s.Enabled() {
		return dto.AgentSession{}, errors.New("agent is disabled")
	}
	sess := dto.AgentSession{ID: s.nextID(), Goal: goal, Status: dto.SessionRunning, StartedAt: time.Now()}
	sess.Transcript = []dto.AgentMessage{{Role: "user", Content: goal}}
	s.injectRecall(&sess)
	if steps := s.plan(ctx, goal); len(steps) > 0 {
		sess.Plan = steps
		appendTranscript(&sess, llm.Message{Role: "system", Content: planSummary(steps)})
	}
	s.emit(&sess, "session_started", nil)
	s.runLoop(ctx, &sess)
	s.finalize(&sess)
	s.store.Put(sess)
	if err := s.store.Save(); err != nil {
		logger.Warning("Agent: failed to persist session %s: %v", sess.ID, err)
	}
	return sess, nil
}

// ApproveAction resolves a pending approval and resumes the session loop.
func (s *Service) ApproveAction(ctx context.Context, sessionID, actionID string, approve bool) (dto.AgentSession, error) {
	sess, ok := s.store.Get(sessionID)
	if !ok {
		return dto.AgentSession{}, fmt.Errorf("session %q not found", sessionID)
	}
	if sess.Status != dto.SessionAwaitingApproval || sess.PendingApproval == nil {
		return dto.AgentSession{}, fmt.Errorf("session %q is not awaiting approval", sessionID)
	}
	if sess.PendingApproval.ActionID != actionID {
		return dto.AgentSession{}, fmt.Errorf("action_id %q does not match the pending approval", actionID)
	}

	pending := sess.PendingApproval
	sess.PendingApproval = nil
	sess.Status = dto.SessionRunning

	var result string
	switch {
	case !approve:
		result = "Action denied by operator."
	case s.isForbidden(pending.ToolName):
		result = fmt.Sprintf("Action %q is on the forbidden list and cannot be executed even with approval.", pending.ToolName)
	default:
		tool, found := s.tools.Get(pending.ToolName)
		if !found {
			result = fmt.Sprintf("Error: tool %q no longer exists.", pending.ToolName)
		} else {
			rec := s.invokeTool(ctx, tool, llm.ToolCall{ID: pending.ActionID, Name: pending.ToolName, Args: pending.Args})
			result = rec.Result
			s.emit(&sess, "tool_called", rec)
		}
	}
	appendTranscript(&sess, llm.Message{Role: "tool", ToolCallID: pending.ActionID, Content: result})
	s.runLoop(ctx, &sess)
	s.finalize(&sess)

	s.store.Put(sess)
	if err := s.store.Save(); err != nil {
		logger.Warning("Agent: failed to persist session %s: %v", sess.ID, err)
	}
	return sess, nil
}

// SweepExpiredApprovals auto-denies awaiting-approval sessions older than the TTL.
// Returns the number of sessions swept. A non-positive ApprovalTTLSecs disables it.
func (s *Service) SweepExpiredApprovals(ctx context.Context, now time.Time) int {
	if s.cfg.ApprovalTTLSecs <= 0 {
		return 0
	}
	ttl := time.Duration(s.cfg.ApprovalTTLSecs) * time.Second
	swept := 0
	for _, sess := range s.store.List() {
		if sess.Status != dto.SessionAwaitingApproval || sess.PendingApproval == nil {
			continue
		}
		if now.Sub(sess.PendingApproval.RequestedAt) < ttl {
			continue
		}
		logger.Warning("Agent: approval for session %s timed out after %s; auto-denying", sess.ID, ttl)
		if _, err := s.ApproveAction(ctx, sess.ID, sess.PendingApproval.ActionID, false); err != nil {
			logger.Error("Agent: failed to auto-deny session %s: %v", sess.ID, err)
			continue
		}
		swept++
	}
	return swept
}

// SetEventBus wires the pubsub hub so the agent can receive wake events.
func (s *Service) SetEventBus(hub *domain.EventBus) { s.hub = hub }

// startAutonomousSession runs an investigation triggered by a wake event (synchronous).
func (s *Service) startAutonomousSession(ctx context.Context, ev dto.AgentWakeEvent) {
	goal := fmt.Sprintf("An incident was detected (source=%s, severity=%s): %s. %s\n"+
		"Investigate using read-only tools and, within policy, remediate it.",
		ev.Source, ev.Severity, ev.Title, ev.Detail)
	sess := dto.AgentSession{ID: s.nextID(), Goal: goal, Status: dto.SessionRunning, StartedAt: time.Now()}
	sess.Transcript = []dto.AgentMessage{{Role: "user", Content: goal}}
	s.injectRecall(&sess)
	s.emit(&sess, "session_started", nil)
	s.runLoop(ctx, &sess)
	s.finalize(&sess)
	s.store.Put(sess)
	if err := s.store.Save(); err != nil {
		logger.Warning("Agent: failed to persist autonomous session %s: %v", sess.ID, err)
	}
	s.wakeMu.Lock()
	s.activeAuto--
	s.wakeMu.Unlock()
}

// SendMessage continues a finished session with a follow-up operator message.
func (s *Service) SendMessage(ctx context.Context, sessionID, message string) (dto.AgentSession, error) {
	if strings.TrimSpace(message) == "" {
		return dto.AgentSession{}, errors.New("message must not be empty")
	}
	sess, ok := s.store.Get(sessionID)
	if !ok {
		return dto.AgentSession{}, fmt.Errorf("session %q not found", sessionID)
	}
	if sess.Status != dto.SessionCompleted && sess.Status != dto.SessionFailed {
		return dto.AgentSession{}, fmt.Errorf("session %q cannot be continued in state %q", sessionID, sess.Status)
	}
	sess.Status = dto.SessionRunning
	sess.Answer = ""
	sess.Error = ""
	sess.EndedAt = nil
	appendTranscript(&sess, llm.Message{Role: "user", Content: message})
	s.emit(&sess, "message_received", nil)
	s.runLoop(ctx, &sess)
	s.finalize(&sess)
	s.store.Put(sess)
	if err := s.store.Save(); err != nil {
		logger.Warning("Agent: failed to persist session %s: %v", sess.ID, err)
	}
	return sess, nil
}

// CancelSession marks a session cancelled and clears any pending approval.
func (s *Service) CancelSession(sessionID string) (dto.AgentSession, error) {
	sess, ok := s.store.Get(sessionID)
	if !ok {
		return dto.AgentSession{}, fmt.Errorf("session %q not found", sessionID)
	}
	if sess.Status == dto.SessionCompleted || sess.Status == dto.SessionFailed || sess.Status == dto.SessionCancelled {
		return sess, fmt.Errorf("cannot cancel session %q in terminal state %q", sessionID, sess.Status)
	}
	now := time.Now()
	sess.Status = dto.SessionCancelled
	sess.PendingApproval = nil
	sess.EndedAt = &now
	s.emit(&sess, "session_cancelled", nil)
	s.store.Put(sess)
	if err := s.store.Save(); err != nil {
		logger.Warning("Agent: failed to persist session %s: %v", sess.ID, err)
	}
	return sess, nil
}
