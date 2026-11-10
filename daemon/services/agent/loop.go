package agent

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/tinkerologie/unraid-management-agent/daemon/dto"
	"github.com/tinkerologie/unraid-management-agent/daemon/logger"
	"github.com/tinkerologie/unraid-management-agent/daemon/services/agent/llm"
	"github.com/tinkerologie/unraid-management-agent/daemon/services/agent/tools"
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
// An approval decision resumes the interrupted tool turn before the next model request.
// It returns when the model gives a final answer, a cap is hit, the provider
// errors, or a tool call requires approval (the session is left paused).
func (s *Service) runLoop(ctx context.Context, sess *dto.AgentSession, approval *bool) {
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

	if approval != nil {
		s.resumeToolTurn(loopCtx, sess, *approval)
		if sess.Status != dto.SessionRunning {
			return
		}
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

		appendTranscript(sess, llm.Message{Role: "assistant", Content: resp.Text, ToolCalls: resp.ToolCalls})
		sess.Steps = append(sess.Steps, step)
		turn := sess.Transcript[len(sess.Transcript)-1]
		s.runToolCalls(loopCtx, sess, &sess.Steps[len(sess.Steps)-1], turn.ToolCalls)
		if sess.PendingApproval != nil {
			return
		}
	}

	s.finish(sess, dto.SessionCompleted, "Stopped: reached maximum reasoning steps without a final answer.")
}

// runToolCalls finishes the current step or pauses at the next call that needs approval.
func (s *Service) runToolCalls(ctx context.Context, sess *dto.AgentSession, step *dto.AgentStep, calls []dto.AgentMsgToolCall) {
	for _, call := range calls {
		tool, rec := s.checkToolCall(call)
		if rec.Error == "" {
			mode := s.cfg.Autonomy[tool.RiskTier]
			if mode != dto.ModeAuto && !s.autoApprovedByPreference(call.Name) {
				sess.PendingApproval = &dto.ApprovalRequest{
					ActionID:    call.ID,
					ToolName:    call.Name,
					Args:        call.Args,
					RiskTier:    tool.RiskTier,
					Reason:      step.Thought,
					RequestedAt: time.Now(),
				}
				sess.Status = dto.SessionAwaitingApproval
				s.emit(sess, "approval_required", sess.PendingApproval)
				return
			}
			invokeTool(ctx, tool, &rec)
		}
		s.recordToolResult(sess, step, call.ID, rec)
	}
	s.emit(sess, "step_completed", *step)
}

// resumeToolTurn resolves the saved call, then processes the remaining calls in its turn.
func (s *Service) resumeToolTurn(ctx context.Context, sess *dto.AgentSession, approve bool) {
	pending := sess.PendingApproval
	sess.PendingApproval = nil
	sess.Status = dto.SessionRunning

	// Each completed call before the pause left a result immediately after the
	// assistant turn. The next call in that turn is the one awaiting approval.
	turnIndex := len(sess.Transcript) - 1
	for turnIndex >= 0 && sess.Transcript[turnIndex].Role == "tool" {
		turnIndex--
	}
	if turnIndex < 0 || sess.Transcript[turnIndex].Role != "assistant" || len(sess.Steps) == 0 {
		s.fail(sess, "pending approval has no assistant tool turn")
		return
	}
	calls := sess.Transcript[turnIndex].ToolCalls
	next := len(sess.Transcript) - turnIndex - 1
	if next >= len(calls) || calls[next].ID != pending.ActionID ||
		calls[next].Name != pending.ToolName || calls[next].Args != pending.Args {
		s.fail(sess, "pending approval does not match the next tool call")
		return
	}

	// The store's snapshots share slice backing arrays. Copy the step before
	// adding results so readers of the paused session keep their snapshot.
	sess.Transcript = slices.Clip(sess.Transcript)
	sess.Steps = slices.Clone(sess.Steps)
	step := &sess.Steps[len(sess.Steps)-1]
	step.ToolCalls = slices.Clip(step.ToolCalls)

	call := calls[next]
	rec := dto.AgentToolCall{
		Name: call.Name, Args: call.Args, RiskTier: pending.RiskTier,
		Result: "Action denied by operator.", Error: "denied", At: time.Now(),
	}
	if approve {
		var tool tools.Tool
		tool, rec = s.checkToolCall(call)
		if rec.Error == "" {
			invokeTool(ctx, tool, &rec)
		}
	}
	s.recordToolResult(sess, step, call.ID, rec)
	s.runToolCalls(ctx, sess, step, calls[next+1:])
}

// checkToolCall resolves the tool and checks policies that also apply to operator approval.
func (s *Service) checkToolCall(call dto.AgentMsgToolCall) (tools.Tool, dto.AgentToolCall) {
	tool, found := s.tools.Get(call.Name)
	rec := dto.AgentToolCall{Name: call.Name, Args: call.Args, RiskTier: tool.RiskTier, At: time.Now()}
	switch refusal := s.toolRefusal(call.Name, tool.RiskTier, found); {
	case refusal != "":
		if !found {
			rec.RiskTier = dto.RiskHigh
		}
		rec.Error = "forbidden"
		rec.Result = refusal
	case !found:
		rec.Error = "unknown tool"
		rec.Result = fmt.Sprintf("Error: tool %q does not exist.", call.Name)
	}
	return tool, rec
}

func invokeTool(ctx context.Context, tool tools.Tool, rec *dto.AgentToolCall) {
	out, err := tool.Invoke(ctx, rec.Args)
	if err != nil {
		rec.Error = err.Error()
		rec.Result = "Error: " + err.Error()
		return
	}
	rec.Result = out
}

func (s *Service) recordToolResult(sess *dto.AgentSession, step *dto.AgentStep, id string, rec dto.AgentToolCall) {
	step.ToolCalls = append(step.ToolCalls, rec)
	appendTranscript(sess, llm.Message{Role: "tool", ToolCallID: id, Content: rec.Result})
	s.emit(sess, "tool_called", rec)
}

// toolRefusal returns the reason a tool is blocked by policy, or an empty string.
func (s *Service) toolRefusal(name string, tier dto.RiskTier, registered bool) string {
	if slices.Contains(s.cfg.ForbidList, name) {
		return fmt.Sprintf("Action %q is on the forbidden list and will never be executed.", name)
	}
	if registered && s.cfg.Autonomy[tier] == dto.ModeForbid {
		return fmt.Sprintf("Action %q is forbidden by the autonomy policy for risk tier %q.", name, tier)
	}
	return ""
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
