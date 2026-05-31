// Package llm defines the pluggable LLM provider abstraction for the agent.
package llm

import "context"

// Message is one turn in the conversation. Role is "system" | "user" | "assistant" | "tool".
type Message struct {
	Role       string // system|user|assistant|tool
	Content    string
	ToolCallID string // set when Role == "tool": which call this result answers
}

// ToolSchema describes a tool the model may call.
type ToolSchema struct {
	Name        string
	Description string
	Schema      []byte // JSON Schema for the arguments object
}

// ToolCall is the model's request to invoke a tool.
type ToolCall struct {
	ID   string
	Name string
	Args string // raw JSON arguments
}

// ChatRequest is a single completion request.
type ChatRequest struct {
	System    string
	Messages  []Message
	Tools     []ToolSchema
	MaxTokens int
}

// ChatResponse is the model's reply: either Text (final) or ToolCalls (act).
type ChatResponse struct {
	Text         string
	ToolCalls    []ToolCall
	InputTokens  int
	OutputTokens int
}

// Provider is implemented by each LLM backend (anthropic, mock, ...).
type Provider interface {
	Name() string
	Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error)
}
