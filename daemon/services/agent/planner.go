package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/services/agent/llm"
)

const plannerPrompt = `You are planning how to achieve an operator's goal on an Unraid server. ` +
	`Reply with ONLY a JSON array of 1-6 steps, each {"intent": "...", "tool": "<tool name or empty>"}. ` +
	`No prose, no code fences — just the JSON array.`

// plan asks the provider for a short ordered plan. Best-effort: returns nil on
// any error or parse failure (the session proceeds without a plan).
func (s *Service) plan(ctx context.Context, goal string) []dto.PlanStep {
	resp, err := s.provider.Chat(ctx, llm.ChatRequest{
		System:    plannerPrompt,
		Messages:  []llm.Message{{Role: "user", Content: goal}},
		MaxTokens: 512,
	})
	if err != nil || resp == nil {
		return nil
	}
	var steps []dto.PlanStep
	if err := json.Unmarshal([]byte(extractJSONArray(resp.Text)), &steps); err != nil {
		return nil
	}
	return steps
}

// extractJSONArray pulls a JSON array out of an LLM reply, tolerating ```json
// fences and surrounding prose by taking the span from the first '[' to the
// last ']'. Returns the trimmed input unchanged when no array is found.
func extractJSONArray(text string) string {
	t := strings.TrimSpace(text)
	t = strings.TrimPrefix(t, "```json")
	t = strings.TrimPrefix(t, "```")
	t = strings.TrimSuffix(t, "```")
	t = strings.TrimSpace(t)
	start := strings.IndexByte(t, '[')
	end := strings.LastIndexByte(t, ']')
	if start >= 0 && end > start {
		return t[start : end+1]
	}
	return t
}

// planSummary renders a plan as a compact system message.
func planSummary(steps []dto.PlanStep) string {
	var b strings.Builder
	b.WriteString("Your plan for this goal:\n")
	for i, st := range steps {
		fmt.Fprintf(&b, "%d. %s", i+1, st.Intent)
		if st.Tool != "" {
			fmt.Fprintf(&b, " (tool: %s)", st.Tool)
		}
		b.WriteByte('\n')
	}
	return strings.TrimSpace(b.String())
}
