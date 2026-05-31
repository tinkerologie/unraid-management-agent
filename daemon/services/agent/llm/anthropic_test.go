package llm

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestAnthropicToolUseRoundTrip(t *testing.T) {
	var secondBody []byte
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 2 {
			secondBody, _ = io.ReadAll(r.Body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"done"}],"usage":{"input_tokens":1,"output_tokens":1}}`))
	}))
	defer srv.Close()
	p := NewAnthropicProvider("k", "m", srv.URL+"/v1/messages")
	// First call (not strictly needed to inspect) then a second call carrying the assistant tool_use + tool_result:
	msgs := []Message{
		{Role: "user", Content: "check"},
		{Role: "assistant", Content: "", ToolCalls: []ToolCall{{ID: "tu_1", Name: "get_system_info", Args: "{}"}}},
		{Role: "tool", ToolCallID: "tu_1", Content: "{\"host\":\"tower\"}"},
	}
	_, _ = p.Chat(context.Background(), ChatRequest{Messages: []Message{{Role: "user", Content: "first"}}})
	if _, err := p.Chat(context.Background(), ChatRequest{Messages: msgs}); err != nil {
		t.Fatalf("chat2: %v", err)
	}
	s := string(secondBody)
	if !strings.Contains(s, `"type":"tool_use"`) || !strings.Contains(s, `"tu_1"`) {
		t.Fatalf("missing tool_use block: %s", s)
	}
	if !strings.Contains(s, `"type":"tool_result"`) || !strings.Contains(s, `"tool_use_id":"tu_1"`) {
		t.Fatalf("missing matching tool_result: %s", s)
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
