package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAnthropicParsesToolUse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"content":[{"type":"tool_use","id":"toolu_1","name":"get_system_info","input":{}}],
			"usage":{"input_tokens":11,"output_tokens":4}
		}`))
	}))
	defer srv.Close()

	p := NewAnthropicProvider("test-key", "claude-opus-4-8", srv.URL+"/v1/messages")
	resp, err := p.Chat(context.Background(), ChatRequest{Messages: []Message{{Role: "user", Content: "hi"}}, MaxTokens: 100})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if len(resp.ToolCalls) != 1 || resp.ToolCalls[0].Name != "get_system_info" {
		t.Fatalf("expected tool call, got %+v", resp)
	}
	if resp.InputTokens != 11 || resp.OutputTokens != 4 {
		t.Fatalf("token usage wrong: %+v", resp)
	}
}

func TestAnthropicErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"bad key"}}`))
	}))
	defer srv.Close()
	p := NewAnthropicProvider("k", "m", srv.URL+"/v1/messages")
	if _, err := p.Chat(context.Background(), ChatRequest{MaxTokens: 10}); err == nil {
		t.Fatal("expected error on 401")
	}
}
