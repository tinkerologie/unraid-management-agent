package llm

import (
	"context"
	"testing"
)

func TestMockProviderReplaysScript(t *testing.T) {
	m := NewMockProvider(
		&ChatResponse{ToolCalls: []ToolCall{{ID: "1", Name: "get_system_info", Args: "{}"}}, InputTokens: 10, OutputTokens: 5},
		&ChatResponse{Text: "All good.", InputTokens: 12, OutputTokens: 8},
	)
	r1, err := m.Chat(context.Background(), ChatRequest{Messages: []Message{{Role: "user", Content: "status?"}}})
	if err != nil {
		t.Fatalf("chat1: %v", err)
	}
	if len(r1.ToolCalls) != 1 || r1.ToolCalls[0].Name != "get_system_info" {
		t.Fatalf("expected tool call, got %+v", r1)
	}
	r2, _ := m.Chat(context.Background(), ChatRequest{})
	if r2.Text != "All good." {
		t.Fatalf("expected final text, got %q", r2.Text)
	}
	if len(m.Requests()) != 2 {
		t.Fatalf("expected 2 recorded requests, got %d", len(m.Requests()))
	}
}
