package agent

import (
	"fmt"
	"strings"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/services/agent/llm"
)

// signatureFor derives a coarse recall key from a goal/incident description.
func signatureFor(goal string) string {
	g := strings.TrimSpace(goal)
	if i := strings.IndexAny(g, ".\n"); i > 0 {
		g = g[:i]
	}
	if len(g) > 120 {
		g = g[:120]
	}
	return g
}

// recallContext builds a system-message body summarizing relevant past incidents
// and active preferences. Returns "" when there is nothing useful to inject.
func (s *Service) recallContext(sig string) string {
	if s.memory == nil || !s.cfg.MemoryEnabled {
		return ""
	}
	var b strings.Builder
	if hits := s.memory.Recall(sig, s.cfg.RecallTopK); len(hits) > 0 {
		b.WriteString("Relevant past incidents (most similar first):\n")
		for _, h := range hits {
			fmt.Fprintf(&b, "- [%s] %s — %s\n", h.Outcome, h.Signature, h.Summary)
		}
	}
	if prefs := s.memory.ActivePreferences(); len(prefs) > 0 {
		b.WriteString("Operator preferences in effect:\n")
		for _, p := range prefs {
			fmt.Fprintf(&b, "- %s: %s (%s)\n", p.Kind, p.Subject, p.Note)
		}
	}
	return strings.TrimSpace(b.String())
}

// injectRecall appends a recalled-context system message to a fresh session, if any.
func (s *Service) injectRecall(sess *dto.AgentSession) {
	ctxText := s.recallContext(signatureFor(sess.Goal))
	if ctxText == "" {
		return
	}
	appendTranscript(sess, llm.Message{Role: "system", Content: ctxText})
}

// finalize writes an episodic incident for a terminal session and persists memory.
func (s *Service) finalize(sess *dto.AgentSession) {
	if s.memory == nil || !s.cfg.MemoryEnabled {
		return
	}
	switch sess.Status {
	case dto.SessionCompleted, dto.SessionFailed:
	default:
		return
	}
	summary := sess.Answer
	if summary == "" {
		summary = sess.Error
	}
	var actions []string
	for _, st := range sess.Steps {
		for _, tc := range st.ToolCalls {
			actions = append(actions, tc.Name)
		}
	}
	s.memory.AddIncident(dto.AgentIncident{
		ID:        "inc-" + sess.ID,
		Signature: signatureFor(sess.Goal),
		Goal:      sess.Goal,
		Outcome:   string(sess.Status),
		Summary:   summary,
		Actions:   actions,
		At:        sess.StartedAt,
	})
	if err := s.memory.Save(); err != nil {
		logger.Warning("Agent: memory save failed for session %s: %v", sess.ID, err)
	}
}
