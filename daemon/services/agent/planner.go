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
	text := strings.TrimSpace(resp.Text)
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(text, "```")
	text = strings.TrimSpace(text)
	var steps []dto.PlanStep
	if err := json.Unmarshal([]byte(text), &steps); err != nil {
		return nil
	}
	return steps
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
