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

const defaultOpenAIEndpoint = "https://api.openai.com/v1/chat/completions"

// OpenAIProvider implements Provider against any OpenAI-compatible Chat
// Completions API (OpenAI, OpenRouter, Gemini's OpenAI-compatible endpoint).
type OpenAIProvider struct {
	apiKey   string
	model    string
	endpoint string
	client   *http.Client
}

// NewOpenAIProvider creates a provider. Empty endpoint uses the public OpenAI base.
func NewOpenAIProvider(apiKey, model, endpoint string) *OpenAIProvider {
	if endpoint == "" {
		endpoint = defaultOpenAIEndpoint
	}
	return &OpenAIProvider{
		apiKey:   apiKey,
		model:    model,
		endpoint: endpoint,
		client:   &http.Client{Timeout: 120 * time.Second},
	}
}

// Name identifies the provider.
func (o *OpenAIProvider) Name() string { return "openai" }

type openAIFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type openAIReqToolCall struct {
	ID       string         `json:"id"`
	Type     string         `json:"type"`
	Function openAIFunction `json:"function"`
}

type openAIReqMessage struct {
	Role       string              `json:"role"`
	Content    any                 `json:"content"`
	ToolCallID string              `json:"tool_call_id,omitempty"`
	ToolCalls  []openAIReqToolCall `json:"tool_calls,omitempty"`
}

type openAIReqToolFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type openAIReqTool struct {
	Type     string                `json:"type"`
	Function openAIReqToolFunction `json:"function"`
}

type openAIReq struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens,omitempty"`
	Messages  []openAIReqMessage `json:"messages"`
	Tools     []openAIReqTool    `json:"tools,omitempty"`
}

type openAIResp struct {
	Choices []struct {
		Message struct {
			Content   string `json:"content"`
			ToolCalls []struct {
				ID       string         `json:"id"`
				Type     string         `json:"type"`
				Function openAIFunction `json:"function"`
			} `json:"tool_calls"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}

// Chat sends the request to the OpenAI-compatible API and maps the reply.
func (o *OpenAIProvider) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	body := openAIReq{Model: o.model}
	if req.MaxTokens > 0 {
		body.MaxTokens = req.MaxTokens
	}

	if req.System != "" {
		body.Messages = append(body.Messages, openAIReqMessage{Role: "system", Content: req.System})
	}
	for _, m := range req.Messages {
		switch m.Role {
		case "tool":
			body.Messages = append(body.Messages, openAIReqMessage{
				Role:       "tool",
				ToolCallID: m.ToolCallID,
				Content:    m.Content,
			})
		case "assistant":
			if len(m.ToolCalls) > 0 {
				calls := make([]openAIReqToolCall, 0, len(m.ToolCalls))
				for _, c := range m.ToolCalls {
					args := c.Args
					if args == "" {
						args = "{}"
					}
					calls = append(calls, openAIReqToolCall{
						ID:       c.ID,
						Type:     "function",
						Function: openAIFunction{Name: c.Name, Arguments: args},
					})
				}
				msg := openAIReqMessage{Role: "assistant", ToolCalls: calls}
				if m.Content != "" {
					msg.Content = m.Content
				}
				body.Messages = append(body.Messages, msg)
			} else {
				body.Messages = append(body.Messages, openAIReqMessage{Role: "assistant", Content: m.Content})
			}
		default: // user (and any other plain role)
			body.Messages = append(body.Messages, openAIReqMessage{Role: m.Role, Content: m.Content})
		}
	}
	for _, t := range req.Tools {
		schema := t.Schema
		if len(schema) == 0 {
			schema = []byte(EmptyObjectSchema)
		}
		body.Tools = append(body.Tools, openAIReqTool{
			Type: "function",
			Function: openAIReqToolFunction{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  schema,
			},
		})
	}

	buf, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal openai request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, o.endpoint, bytes.NewReader(buf))
	if err != nil {
		return nil, fmt.Errorf("build openai request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+o.apiKey)

	resp, err := o.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("openai request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read openai response body: %w", err)
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("openai API status %d: %s", resp.StatusCode, string(raw))
	}

	var parsed openAIResp
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("decode openai response: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return nil, fmt.Errorf("openai response contained no choices: %s", string(raw))
	}

	out := &ChatResponse{InputTokens: parsed.Usage.PromptTokens, OutputTokens: parsed.Usage.CompletionTokens}
	if len(parsed.Choices) > 0 {
		msg := parsed.Choices[0].Message
		out.Text = msg.Content
		for _, c := range msg.ToolCalls {
			out.ToolCalls = append(out.ToolCalls, ToolCall{ID: c.ID, Name: c.Function.Name, Args: c.Function.Arguments})
		}
	}
	return out, nil
}
