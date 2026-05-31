package llm

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenAIParsesToolCalls(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"choices":[{"message":{"role":"assistant","content":null,
				"tool_calls":[{"id":"call_1","type":"function","function":{"name":"get_system_info","arguments":"{}"}}]}}],
			"usage":{"prompt_tokens":12,"completion_tokens":5}
		}`))
	}))
	defer srv.Close()

	p := NewOpenAIProvider("k", "gpt-4o", srv.URL+"/v1/chat/completions")
	resp, err := p.Chat(context.Background(), ChatRequest{
		System:    "you are an agent",
		Messages:  []Message{{Role: "user", Content: "status?"}},
		Tools:     []ToolSchema{{Name: "get_system_info", Description: "sys", Schema: []byte(EmptyObjectSchema)}},
		MaxTokens: 100,
	})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if len(resp.ToolCalls) != 1 || resp.ToolCalls[0].Name != "get_system_info" || resp.ToolCalls[0].ID != "call_1" {
		t.Fatalf("tool call parse: %+v", resp.ToolCalls)
	}
	if resp.InputTokens != 12 || resp.OutputTokens != 5 {
		t.Fatalf("usage: %+v", resp)
	}
	// system message must be first, tools present
	if !strings.Contains(gotBody, `"role":"system"`) || !strings.Contains(gotBody, `"type":"function"`) {
		t.Fatalf("request body missing system/tools: %s", gotBody)
	}
}

func TestOpenAIToolResultRoundTrip(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf, _ := io.ReadAll(r.Body)
		gotBody = string(buf)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"done"}}],"usage":{}}`))
	}))
	defer srv.Close()
	p := NewOpenAIProvider("k", "m", srv.URL)
	_, err := p.Chat(context.Background(), ChatRequest{Messages: []Message{
		{Role: "assistant", ToolCalls: []ToolCall{{ID: "call_1", Name: "get_system_info", Args: "{}"}}},
		{Role: "tool", ToolCallID: "call_1", Content: "{\"host\":\"tower\"}"},
	}})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if !strings.Contains(gotBody, `"tool_calls"`) || !strings.Contains(gotBody, `"tool_call_id":"call_1"`) {
		t.Fatalf("round-trip body missing tool_calls/tool_call_id: %s", gotBody)
	}
}

func TestOpenAIErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"bad key"}}`))
	}))
	defer srv.Close()
	p := NewOpenAIProvider("k", "m", srv.URL)
	if _, err := p.Chat(context.Background(), ChatRequest{}); err == nil {
		t.Fatal("expected error on 401")
	}
}

func TestOpenAIEmptyChoicesErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[],"usage":{}}`))
	}))
	defer srv.Close()
	p := NewOpenAIProvider("k", "m", srv.URL)
	if _, err := p.Chat(context.Background(), ChatRequest{}); err == nil {
		t.Fatal("expected error when choices is empty")
	}
}
