package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const defaultAnthropicEndpoint = "https://api.anthropic.com/v1/messages"
const anthropicVersion = "2023-06-01"

// AnthropicProvider implements Provider against the Anthropic Messages API.
type AnthropicProvider struct {
	apiKey   string
	model    string
	endpoint string
	client   *http.Client
}

// NewAnthropicProvider creates a provider. Empty endpoint uses the public API base.
func NewAnthropicProvider(apiKey, model, endpoint string) *AnthropicProvider {
	if endpoint == "" {
		endpoint = defaultAnthropicEndpoint
	}
	return &AnthropicProvider{
		apiKey:   apiKey,
		model:    model,
		endpoint: endpoint,
		client:   &http.Client{Timeout: 120 * time.Second},
	}
}

// Name identifies the provider.
func (a *AnthropicProvider) Name() string { return "anthropic" }

type anthropicReqTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

type anthropicReq struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	System    string             `json:"system,omitempty"`
	Messages  []anthropicMessage `json:"messages"`
	Tools     []anthropicReqTool `json:"tools,omitempty"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type anthropicResp struct {
	Content []struct {
		Type  string          `json:"type"`
		Text  string          `json:"text"`
		ID    string          `json:"id"`
		Name  string          `json:"name"`
		Input json.RawMessage `json:"input"`
	} `json:"content"`
	Usage struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

// Chat sends the request to the Anthropic API and maps the reply.
func (a *AnthropicProvider) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 4096
	}

	body := anthropicReq{Model: a.model, MaxTokens: maxTokens, System: req.System}
	for _, m := range req.Messages {
		switch m.Role {
		case "tool":
			// Tool results are sent as a user message with a tool_result content block.
			body.Messages = append(body.Messages, anthropicMessage{
				Role: "user",
				Content: []map[string]any{{
					"type":        "tool_result",
					"tool_use_id": m.ToolCallID,
					"content":     m.Content,
				}},
			})
		case "assistant":
			if len(m.ToolCalls) > 0 {
				blocks := make([]map[string]any, 0, len(m.ToolCalls)+1)
				if m.Content != "" {
					blocks = append(blocks, map[string]any{"type": "text", "text": m.Content})
				}
				for _, c := range m.ToolCalls {
					input := json.RawMessage(c.Args)
					if len(input) == 0 || !json.Valid(input) {
						input = json.RawMessage(`{}`)
					}
					blocks = append(blocks, map[string]any{
						"type":  "tool_use",
						"id":    c.ID,
						"name":  c.Name,
						"input": input,
					})
				}
				body.Messages = append(body.Messages, anthropicMessage{Role: "assistant", Content: blocks})
			} else {
				body.Messages = append(body.Messages, anthropicMessage{Role: "assistant", Content: m.Content})
			}
		default: // system, user
			body.Messages = append(body.Messages, anthropicMessage{Role: m.Role, Content: m.Content})
		}
	}
	for _, t := range req.Tools {
		schema := t.Schema
		if len(schema) == 0 {
			schema = []byte(EmptyObjectSchema)
		}
		body.Tools = append(body.Tools, anthropicReqTool{
			Name: t.Name, Description: t.Description, InputSchema: schema,
		})
	}

	buf, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal anthropic request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.endpoint, bytes.NewReader(buf))
	if err != nil {
		return nil, fmt.Errorf("build anthropic request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", a.apiKey)
	httpReq.Header.Set("anthropic-version", anthropicVersion)

	resp, err := a.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("anthropic request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read anthropic response body: %w", err)
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("anthropic API status %d: %s", resp.StatusCode, string(raw))
	}

	var parsed anthropicResp
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("decode anthropic response: %w", err)
	}

	out := &ChatResponse{InputTokens: parsed.Usage.InputTokens, OutputTokens: parsed.Usage.OutputTokens}
	for _, c := range parsed.Content {
		switch c.Type {
		case "text":
			out.Text += c.Text
		case "tool_use":
			out.ToolCalls = append(out.ToolCalls, ToolCall{ID: c.ID, Name: c.Name, Args: string(c.Input)})
		}
	}
	return out, nil
}
