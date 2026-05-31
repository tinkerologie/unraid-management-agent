package agent

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/services/agent/llm"
)

const systemPrompt = `You are the Unraid Management Agent's autonomous operator. ` +
	`Investigate the user's goal using the provided tools, then give a concise answer. ` +
	`Only call tools that exist. When you have enough information, reply with a final text answer and no tool calls.`

const defaultLLMMaxOutputTokens = 4096 // minimum safe for the Anthropic Claude family

// runLoop executes the bounded ReAct cycle and returns the finished session.
func (s *Service) runLoop(ctx context.Context, id, goal string) (sess dto.AgentSession) {
	sess = dto.AgentSession{ID: id, Goal: goal, Status: dto.SessionRunning, StartedAt: time.Now()}
	defer func() {
		if r := recover(); r != nil {
			logger.Error("Agent: panic in session %s: %v", id, r)
			s.fail(&sess, fmt.Sprintf("internal panic: %v", r))
		}
	}()
	s.emit(&sess, "session_started", nil)

	deadline := time.Duration(s.cfg.SessionDeadlineSecs) * time.Second
	loopCtx := ctx
	if deadline > 0 {
		var cancel context.CancelFunc
		loopCtx, cancel = context.WithTimeout(ctx, deadline)
		defer cancel()
	}

	messages := []llm.Message{{Role: "user", Content: goal}}
	schemas := s.tools.Schemas()

	for i := 0; i < s.cfg.MaxIterations; i++ {
		if s.cfg.MaxTokensPerSession > 0 && sess.TokensUsed >= s.cfg.MaxTokensPerSession {
			s.finish(&sess, dto.SessionCompleted, "Stopped: token budget reached.")
			return sess
		}

		resp, err := s.provider.Chat(loopCtx, llm.ChatRequest{
			System: systemPrompt, Messages: messages, Tools: schemas,
			MaxTokens: defaultLLMMaxOutputTokens,
		})
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				s.fail(&sess, fmt.Sprintf("session cancelled: %v", err))
			} else {
				s.fail(&sess, fmt.Sprintf("provider error: %v", err))
			}
			return sess
		}
		sess.TokensUsed += resp.InputTokens + resp.OutputTokens

		step := dto.AgentStep{Index: i, Thought: resp.Text, At: time.Now()}

		// No tool calls => final answer.
		if len(resp.ToolCalls) == 0 {
			sess.Steps = append(sess.Steps, step)
			s.emit(&sess, "step_completed", step)
			s.finish(&sess, dto.SessionCompleted, resp.Text)
			return sess
		}

		// Record the assistant's tool-call turn so the provider keeps context.
		messages = append(messages, llm.Message{Role: "assistant", Content: resp.Text})

		for _, call := range resp.ToolCalls {
			rec := s.executeCall(loopCtx, call)
			step.ToolCalls = append(step.ToolCalls, rec)
			messages = append(messages, llm.Message{Role: "tool", ToolCallID: call.ID, Content: rec.Result})
			s.emit(&sess, "tool_called", rec)
		}
		sess.Steps = append(sess.Steps, step)
		s.emit(&sess, "step_completed", step)
	}

	s.finish(&sess, dto.SessionCompleted, "Stopped: reached maximum reasoning steps without a final answer.")
	return sess
}

// executeCall runs one tool call under the tiered policy and returns a record.
func (s *Service) executeCall(ctx context.Context, call llm.ToolCall) dto.AgentToolCall {
	rec := dto.AgentToolCall{Name: call.Name, Args: call.Args, At: time.Now()}

	tool, ok := s.tools.Get(call.Name)
	if !ok {
		rec.Error = "unknown tool"
		rec.Result = fmt.Sprintf("Error: tool %q does not exist.", call.Name)
		return rec
	}
	rec.RiskTier = tool.RiskTier

	// Phase-1 policy: read-only and low-risk auto-execute; anything else is refused.
	mode := s.cfg.Autonomy[tool.RiskTier]
	if mode != dto.ModeAuto {
		rec.Error = "requires approval"
		rec.Result = fmt.Sprintf("Action %q (risk=%s) requires approval, which is not available yet. Skipped.", call.Name, tool.RiskTier)
		return rec
	}

	out, err := tool.Invoke(ctx, call.Args)
	if err != nil {
		rec.Error = err.Error()
		rec.Result = "Error: " + err.Error()
		return rec
	}
	rec.Result = out
	return rec
}

func (s *Service) finish(sess *dto.AgentSession, status dto.AgentSessionStatus, answer string) {
	now := time.Now()
	sess.Status = status
	sess.Answer = answer
	sess.EndedAt = &now
	s.emit(sess, "session_completed", nil)
}

func (s *Service) fail(sess *dto.AgentSession, msg string) {
	now := time.Now()
	sess.Status = dto.SessionFailed
	sess.Error = msg
	sess.EndedAt = &now
	logger.Error("Agent: session %s failed: %s", sess.ID, msg)
	s.emit(sess, "session_failed", nil)
}

// emit broadcasts a WS event and tolerates a nil broadcaster.
func (s *Service) emit(sess *dto.AgentSession, event string, data any) {
	if s.bc == nil {
		return
	}
	payload := map[string]any{"session_id": sess.ID, "status": sess.Status}
	if data != nil {
		payload["detail"] = data
	}
	s.bc.BroadcastAgentEvent(dto.WSEvent{Event: "agent_" + event, Timestamp: time.Now(), Data: payload})
}
