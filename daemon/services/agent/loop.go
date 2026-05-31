package agent

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/services/agent/llm"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/services/agent/tools"
)

const systemPrompt = `You are the Unraid Management Agent's autonomous operator. ` +
	`Investigate the user's goal using the provided tools, then give a concise answer. ` +
	`Only call tools that exist. When you have enough information, reply with a final text answer and no tool calls.`

const defaultLLMMaxOutputTokens = 4096 // minimum safe for the Anthropic Claude family

// transcriptToMessages converts the persisted transcript to llm messages.
func transcriptToMessages(t []dto.AgentMessage) []llm.Message {
	out := make([]llm.Message, 0, len(t))
	for _, m := range t {
		msg := llm.Message{Role: m.Role, Content: m.Content, ToolCallID: m.ToolCallID}
		for _, c := range m.ToolCalls {
			msg.ToolCalls = append(msg.ToolCalls, llm.ToolCall{ID: c.ID, Name: c.Name, Args: c.Args})
		}
		out = append(out, msg)
	}
	return out
}

// m2dto converts an llm message to its persisted form.
func m2dto(m llm.Message) dto.AgentMessage {
	rec := dto.AgentMessage{Role: m.Role, Content: m.Content, ToolCallID: m.ToolCallID}
	for _, c := range m.ToolCalls {
		rec.ToolCalls = append(rec.ToolCalls, dto.AgentMsgToolCall{ID: c.ID, Name: c.Name, Args: c.Args})
	}
	return rec
}

// appendTranscript records an llm message on the session for resume.
func appendTranscript(sess *dto.AgentSession, m llm.Message) {
	sess.Transcript = append(sess.Transcript, m2dto(m))
}

// runLoop drives the bounded ReAct cycle from the session's current transcript.
// It returns when the model gives a final answer, a cap is hit, the provider
// errors, or a tool call requires approval (the session is left paused).
func (s *Service) runLoop(ctx context.Context, sess *dto.AgentSession) {
	defer func() {
		if r := recover(); r != nil {
			logger.Error("Agent: panic in session %s: %v", sess.ID, r)
			s.fail(sess, fmt.Sprintf("internal panic: %v", r))
		}
	}()

	deadline := time.Duration(s.cfg.SessionDeadlineSecs) * time.Second
	loopCtx := ctx
	if deadline > 0 {
		var cancel context.CancelFunc
		loopCtx, cancel = context.WithTimeout(ctx, deadline)
		defer cancel()
	}

	schemas := s.tools.Schemas()

	for len(sess.Steps) < s.cfg.MaxIterations {
		if s.cfg.MaxTokensPerSession > 0 && sess.TokensUsed >= s.cfg.MaxTokensPerSession {
			s.finish(sess, dto.SessionCompleted, "Stopped: token budget reached.")
			return
		}

		resp, err := s.provider.Chat(loopCtx, llm.ChatRequest{
			System:    systemPrompt,
			Messages:  transcriptToMessages(sess.Transcript),
			Tools:     schemas,
			MaxTokens: defaultLLMMaxOutputTokens,
		})
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				s.fail(sess, fmt.Sprintf("session cancelled: %v", err))
			} else {
				s.fail(sess, fmt.Sprintf("provider error: %v", err))
			}
			return
		}
		sess.TokensUsed += resp.InputTokens + resp.OutputTokens

		step := dto.AgentStep{Index: len(sess.Steps), Thought: resp.Text, At: time.Now()}

		if len(resp.ToolCalls) == 0 {
			sess.Steps = append(sess.Steps, step)
			s.emit(sess, "step_completed", step)
			s.finish(sess, dto.SessionCompleted, resp.Text)
			return
		}

		// Persist the assistant turn (with tool_use) before acting.
		appendTranscript(sess, llm.Message{Role: "assistant", Content: resp.Text, ToolCalls: resp.ToolCalls})

		for _, call := range resp.ToolCalls {
			tool, ok := s.tools.Get(call.Name)
			tier := dto.RiskHigh
			if ok {
				tier = tool.RiskTier
			}

			if s.isForbidden(call.Name) {
				rec := dto.AgentToolCall{Name: call.Name, Args: call.Args, RiskTier: tier,
					Error: "forbidden", Result: fmt.Sprintf("Action %q is on the forbidden list and will never be executed.", call.Name), At: time.Now()}
				step.ToolCalls = append(step.ToolCalls, rec)
				appendTranscript(sess, llm.Message{Role: "tool", ToolCallID: call.ID, Content: rec.Result})
				s.emit(sess, "tool_called", rec)
				continue
			}

			if !ok {
				rec := dto.AgentToolCall{Name: call.Name, Args: call.Args, Error: "unknown tool",
					Result: fmt.Sprintf("Error: tool %q does not exist.", call.Name), At: time.Now()}
				step.ToolCalls = append(step.ToolCalls, rec)
				appendTranscript(sess, llm.Message{Role: "tool", ToolCallID: call.ID, Content: rec.Result})
				s.emit(sess, "tool_called", rec)
				continue
			}

			mode := s.cfg.Autonomy[tier]
			if mode != dto.ModeAuto && s.autoApprovedByPreference(call.Name) {
				mode = dto.ModeAuto
			}
			if mode != dto.ModeAuto {
				// NOTE: the pending call's tool_use is in the assistant turn but has NO
				// matching tool result yet. ApproveAction (Task 3) MUST append the tool
				// result for this call.ID before calling runLoop again.
				sess.Steps = append(sess.Steps, step)
				sess.PendingApproval = &dto.ApprovalRequest{
					ActionID:    call.ID,
					ToolName:    call.Name,
					Args:        call.Args,
					RiskTier:    tier,
					Reason:      resp.Text,
					RequestedAt: time.Now(),
				}
				sess.Status = dto.SessionAwaitingApproval
				s.emit(sess, "approval_required", sess.PendingApproval)
				return
			}

			rec := s.invokeTool(loopCtx, tool, call)
			step.ToolCalls = append(step.ToolCalls, rec)
			appendTranscript(sess, llm.Message{Role: "tool", ToolCallID: call.ID, Content: rec.Result})
			s.emit(sess, "tool_called", rec)
		}
		sess.Steps = append(sess.Steps, step)
		s.emit(sess, "step_completed", step)
	}

	s.finish(sess, dto.SessionCompleted, "Stopped: reached maximum reasoning steps without a final answer.")
}

// invokeTool runs an auto-approved tool and returns the record.
func (s *Service) invokeTool(ctx context.Context, tool tools.Tool, call llm.ToolCall) dto.AgentToolCall {
	rec := dto.AgentToolCall{Name: call.Name, Args: call.Args, RiskTier: tool.RiskTier, At: time.Now()}
	out, err := tool.Invoke(ctx, call.Args)
	if err != nil {
		rec.Error = err.Error()
		rec.Result = "Error: " + err.Error()
		return rec
	}
	rec.Result = out
	return rec
}

// isForbidden reports whether a tool name is on the non-overridable forbid-list.
func (s *Service) isForbidden(name string) bool {
	for _, f := range s.cfg.ForbidList {
		if f == name {
			return true
		}
	}
	return false
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
