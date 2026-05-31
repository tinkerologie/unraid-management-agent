# Agentic Agent — Phase 1 (Foundation) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers-extended-cc:subagent-driven-development (recommended) or superpowers-extended-cc:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an embedded Agent Core to the daemon that runs an on-demand, bounded perceive→think→act loop with a pluggable LLM provider, executing read-only and low-risk tools automatically, exposed over REST + WebSocket — all disabled by default.

**Architecture:** A new `daemon/services/agent/` package subscribes to nothing yet (Phase 1 is on-demand only). It declares small interfaces it needs (`StateProvider` for cache reads, `Broadcaster` for WS streaming) so it never imports the `api` package — avoiding an import cycle, since `api` imports `agent` to wire it. The agent acts only through the existing `controllers` package and cache reads, never raw shell. The LLM provider is an interface with a mock (for tests) and an Anthropic implementation.

**Tech Stack:** Go 1.26, `net/http` (provider HTTP + REST), `gorilla/mux` (routing), `gorilla/websocket` (existing hub), JSON-on-disk persistence (matching `watchdog.Store`). No new third-party dependencies.

**Reference spec:** `docs/superpowers/specs/2026-05-31-agentic-agent-design.md`

**Scope (Phase 1 only):** provider interface + mock + Anthropic, tool registry with risk tiers (read-only + low-risk tools registered), bounded ReAct loop, session store, REST + WS surface, orchestrator wiring (disabled by default). **Deferred to later phases:** event-driven `agent_wake` triggers, the high-risk approval gate + pause/resume, memory/learning, MCP `agent_*` tools.

---

## Task 1: Agent domain types (`dto/agent.go`)

**Goal:** Define all DTOs and enums the agent uses, with secret-safe JSON tags and sensible defaults.

**Files:**

- Create: `daemon/dto/agent.go`
- Test: `daemon/dto/agent_test.go`

**Acceptance Criteria:**

- [ ] `AgentConfig` marshals to JSON **without** the API key field present.
- [ ] `DefaultAgentConfig()` returns `Enabled: false` and non-zero caps.
- [ ] `RiskTier` and `AutonomyMode` string constants round-trip through JSON.

**Verify:** `go test ./daemon/dto/ -run TestAgent -v` → PASS

**Steps:**

- [ ] **Step 1: Write the failing test**

```go
// daemon/dto/agent_test.go
package dto

import (
 "encoding/json"
 "strings"
 "testing"
)

func TestAgentConfigOmitsAPIKey(t *testing.T) {
 cfg := DefaultAgentConfig()
 cfg.APIKey = "secret-key-value"
 b, err := json.Marshal(cfg)
 if err != nil {
  t.Fatalf("marshal: %v", err)
 }
 if strings.Contains(string(b), "secret-key-value") {
  t.Fatalf("API key leaked into JSON: %s", b)
 }
}

func TestDefaultAgentConfigDefaults(t *testing.T) {
 cfg := DefaultAgentConfig()
 if cfg.Enabled {
  t.Error("agent must be disabled by default")
 }
 if cfg.MaxIterations <= 0 || cfg.MaxTokensPerSession <= 0 {
  t.Error("caps must be positive")
 }
 if cfg.Autonomy[RiskReadOnly] != ModeAuto {
  t.Errorf("read-only must default to auto, got %q", cfg.Autonomy[RiskReadOnly])
 }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./daemon/dto/ -run TestAgent -v`
Expected: FAIL (undefined: DefaultAgentConfig, RiskReadOnly, ModeAuto)

- [ ] **Step 3: Write the implementation**

```go
// daemon/dto/agent.go
package dto

import "time"

// RiskTier classifies how dangerous a tool's effect is.
type RiskTier string

const (
 RiskReadOnly RiskTier = "read_only" // never changes state
 RiskLow      RiskTier = "low"       // reversible, low blast radius (e.g. restart container)
 RiskHigh     RiskTier = "high"      // requires approval (e.g. stop array) — Phase 2
)

// AutonomyMode is how the policy gate treats a tier.
type AutonomyMode string

const (
 ModeAuto    AutonomyMode = "auto"    // execute without asking
 ModeApprove AutonomyMode = "approve" // require human approval (Phase 2)
 ModeForbid  AutonomyMode = "forbid"  // never execute
)

// AgentSessionStatus is the lifecycle state of a session.
type AgentSessionStatus string

const (
 SessionRunning   AgentSessionStatus = "running"
 SessionCompleted AgentSessionStatus = "completed"
 SessionFailed    AgentSessionStatus = "failed"
 SessionCancelled AgentSessionStatus = "cancelled"
)

// AgentConfig holds the agent's runtime configuration (persisted as JSON).
type AgentConfig struct {
 Enabled             bool                      `json:"enabled"`
 Provider            string                    `json:"provider"` // "anthropic" | "mock"
 Model               string                    `json:"model"`
 Endpoint            string                    `json:"endpoint,omitempty"`
 APIKey              string                    `json:"-"` // never serialized; from env/secret file
 Autonomy            map[RiskTier]AutonomyMode `json:"autonomy"`
 MaxIterations       int                       `json:"max_iterations"`
 MaxTokensPerSession int                       `json:"max_tokens_per_session"`
 SessionDeadlineSecs int                       `json:"session_deadline_secs"`
}

// DefaultAgentConfig returns safe defaults: disabled, conservative caps, tiered autonomy.
func DefaultAgentConfig() AgentConfig {
 return AgentConfig{
  Enabled:             false,
  Provider:            "anthropic",
  Model:               "claude-opus-4-8",
  Autonomy:            map[RiskTier]AutonomyMode{RiskReadOnly: ModeAuto, RiskLow: ModeAuto, RiskHigh: ModeApprove},
  MaxIterations:       12,
  MaxTokensPerSession: 60000,
  SessionDeadlineSecs: 180,
 }
}

// AgentToolCall records one tool invocation within a session.
type AgentToolCall struct {
 Name     string    `json:"name"`
 Args     string    `json:"args"` // raw JSON arguments
 RiskTier RiskTier  `json:"risk_tier"`
 Result   string    `json:"result"`
 Error    string    `json:"error,omitempty"`
 At       time.Time `json:"at"`
}

// AgentStep is one perceive→think→act iteration of the loop.
type AgentStep struct {
 Index     int             `json:"index"`
 Thought   string          `json:"thought,omitempty"`
 ToolCalls []AgentToolCall `json:"tool_calls,omitempty"`
 At        time.Time       `json:"at"`
}

// AgentSession is a full agent run (on-demand in Phase 1).
type AgentSession struct {
 ID         string             `json:"id"`
 Goal       string             `json:"goal"`
 Status     AgentSessionStatus `json:"status"`
 Steps      []AgentStep        `json:"steps"`
 Answer     string             `json:"answer,omitempty"`
 Error      string             `json:"error,omitempty"`
 TokensUsed int                `json:"tokens_used"`
 StartedAt  time.Time          `json:"started_at"`
 EndedAt    *time.Time         `json:"ended_at,omitempty"`
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./daemon/dto/ -run TestAgent -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add daemon/dto/agent.go daemon/dto/agent_test.go
git commit -m "feat(agent): domain types and config DTOs"
```

---

## Task 2: LLM provider interface + mock (`agent/llm/`)

**Goal:** Define the provider abstraction and a scriptable mock that drives the loop in tests with zero network calls.

**Files:**

- Create: `daemon/services/agent/llm/provider.go`
- Create: `daemon/services/agent/llm/mock.go`
- Test: `daemon/services/agent/llm/mock_test.go`

**Acceptance Criteria:**

- [ ] `Provider` interface compiles with `Chat(ctx, ChatRequest) (*ChatResponse, error)`.
- [ ] `MockProvider` returns scripted responses in order and records the requests it received.
- [ ] A `ChatResponse` can carry either assistant text **or** tool calls.

**Verify:** `go test ./daemon/services/agent/llm/ -v` → PASS

**Steps:**

- [ ] **Step 1: Write the failing test**

```go
// daemon/services/agent/llm/mock_test.go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./daemon/services/agent/llm/ -v`
Expected: FAIL (package/symbols undefined)

- [ ] **Step 3: Write the implementation**

```go
// daemon/services/agent/llm/provider.go
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
 System   string
 Messages []Message
 Tools    []ToolSchema
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
```

```go
// daemon/services/agent/llm/mock.go
package llm

import (
 "context"
 "fmt"
 "sync"
)

// MockProvider replays a fixed script of responses for deterministic tests.
type MockProvider struct {
 mu       sync.Mutex
 script   []*ChatResponse
 idx      int
 requests []ChatRequest
}

// NewMockProvider builds a mock that returns the given responses in order.
func NewMockProvider(responses ...*ChatResponse) *MockProvider {
 return &MockProvider{script: responses}
}

// Name identifies the provider.
func (m *MockProvider) Name() string { return "mock" }

// Chat returns the next scripted response and records the request.
func (m *MockProvider) Chat(_ context.Context, req ChatRequest) (*ChatResponse, error) {
 m.mu.Lock()
 defer m.mu.Unlock()
 m.requests = append(m.requests, req)
 if m.idx >= len(m.script) {
  return nil, fmt.Errorf("mock provider exhausted after %d responses", len(m.script))
 }
 resp := m.script[m.idx]
 m.idx++
 return resp, nil
}

// Requests returns all requests received so far.
func (m *MockProvider) Requests() []ChatRequest {
 m.mu.Lock()
 defer m.mu.Unlock()
 out := make([]ChatRequest, len(m.requests))
 copy(out, m.requests)
 return out
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./daemon/services/agent/llm/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add daemon/services/agent/llm/provider.go daemon/services/agent/llm/mock.go daemon/services/agent/llm/mock_test.go
git commit -m "feat(agent): LLM provider interface + scriptable mock"
```

---

## Task 3: Anthropic provider (`agent/llm/anthropic.go`)

**Goal:** Implement `Provider` against the Anthropic Messages API with tool-use, tested via `httptest` (no real network).

**Files:**

- Create: `daemon/services/agent/llm/anthropic.go`
- Test: `daemon/services/agent/llm/anthropic_test.go`

**Acceptance Criteria:**

- [ ] `NewAnthropicProvider(apiKey, model, endpoint)` returns a `Provider`; empty endpoint defaults to the public API base.
- [ ] A `tool_use` content block in the API response maps to a `ToolCall`.
- [ ] A `text` content block maps to `ChatResponse.Text`; usage maps to token counts.
- [ ] Non-2xx responses return a wrapped error.

**Verify:** `go test ./daemon/services/agent/llm/ -run Anthropic -v` → PASS

**Steps:**

- [ ] **Step 1: Write the failing test**

```go
// daemon/services/agent/llm/anthropic_test.go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./daemon/services/agent/llm/ -run Anthropic -v`
Expected: FAIL (undefined: NewAnthropicProvider)

- [ ] **Step 3: Write the implementation**

```go
// daemon/services/agent/llm/anthropic.go
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
 Model     string              `json:"model"`
 MaxTokens int                 `json:"max_tokens"`
 System    string              `json:"system,omitempty"`
 Messages  []anthropicMessage  `json:"messages"`
 Tools     []anthropicReqTool  `json:"tools,omitempty"`
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
  // Tool results are sent as a user message with a tool_result content block.
  if m.Role == "tool" {
   body.Messages = append(body.Messages, anthropicMessage{
    Role: "user",
    Content: []map[string]any{{
     "type":        "tool_result",
     "tool_use_id": m.ToolCallID,
     "content":     m.Content,
    }},
   })
   continue
  }
  body.Messages = append(body.Messages, anthropicMessage{Role: m.Role, Content: m.Content})
 }
 for _, t := range req.Tools {
  schema := t.Schema
  if len(schema) == 0 {
   schema = []byte(`{"type":"object","properties":{}}`)
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

 raw, _ := io.ReadAll(resp.Body)
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./daemon/services/agent/llm/ -run Anthropic -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add daemon/services/agent/llm/anthropic.go daemon/services/agent/llm/anthropic_test.go
git commit -m "feat(agent): Anthropic Messages API provider with tool-use"
```

---

## Task 4: Tool registry + risk tiers (`agent/tools/`)

**Goal:** Define the tool registry and register Phase-1 tools (read-only cache reads + low-risk Docker actions), each tagged with a risk tier. The agent calls these instead of MCP/controllers directly, keeping one tiering table.

**Files:**

- Create: `daemon/services/agent/tools/registry.go`
- Create: `daemon/services/agent/tools/builtin.go`
- Test: `daemon/services/agent/tools/registry_test.go`

**Acceptance Criteria:**

- [ ] `Registry.Register` and `Registry.Get` work; `Registry.Schemas()` returns `llm.ToolSchema` for every tool.
- [ ] Read-only tools carry `RiskReadOnly`; `restart_container` carries `RiskLow`.
- [ ] `BuildDefault` wires read-only tools from a `StateProvider` and low-risk tools from a `DockerActor`, both small interfaces (no `api` import).
- [ ] Invoking `get_system_info` returns the JSON the `StateProvider` supplies.

**Verify:** `go test ./daemon/services/agent/tools/ -v` → PASS

**Steps:**

- [ ] **Step 1: Write the failing test**

```go
// daemon/services/agent/tools/registry_test.go
package tools

import (
 "context"
 "encoding/json"
 "testing"

 "github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

type fakeState struct{}

func (fakeState) SystemJSON() (any, bool) { return map[string]string{"host": "tower"}, true }
func (fakeState) ArrayJSON() (any, bool)  { return map[string]string{"state": "STARTED"}, true }
func (fakeState) DockerJSON() (any, bool) { return []string{"plex"}, true }

type fakeDocker struct{ restarted string }

func (f *fakeDocker) Restart(id string) error { f.restarted = id; return nil }

func TestBuildDefaultTiersAndInvoke(t *testing.T) {
 fd := &fakeDocker{}
 reg := BuildDefault(fakeState{}, fd)

 sys, ok := reg.Get("get_system_info")
 if !ok || sys.RiskTier != dto.RiskReadOnly {
  t.Fatalf("get_system_info missing or wrong tier: %+v", sys)
 }
 res, err := sys.Invoke(context.Background(), "{}")
 if err != nil || res == "" {
  t.Fatalf("invoke get_system_info: %q err=%v", res, err)
 }

 rc, ok := reg.Get("restart_container")
 if !ok || rc.RiskTier != dto.RiskLow {
  t.Fatalf("restart_container missing or wrong tier: %+v", rc)
 }
 if _, err := rc.Invoke(context.Background(), `{"container_id":"abc"}`); err != nil {
  t.Fatalf("invoke restart_container: %v", err)
 }
 if fd.restarted != "abc" {
  t.Fatalf("expected restart of abc, got %q", fd.restarted)
 }

 if len(reg.Schemas()) == 0 {
  t.Fatal("expected non-empty schemas")
 }
 _ = json.RawMessage(nil)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./daemon/services/agent/tools/ -v`
Expected: FAIL (undefined symbols)

- [ ] **Step 3: Write the implementation**

```go
// daemon/services/agent/tools/registry.go
// Package tools is the agent's action space: a registry of risk-tiered tools.
package tools

import (
 "context"
 "sort"

 "github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
 "github.com/ruaan-deysel/unraid-management-agent/daemon/services/agent/llm"
)

// Tool is one risk-tiered action the agent can take.
type Tool struct {
 Name        string
 Description string
 Schema      []byte // JSON Schema for arguments
 RiskTier    dto.RiskTier
 Invoke      func(ctx context.Context, argsJSON string) (string, error)
}

// Registry holds the available tools by name.
type Registry struct {
 tools map[string]Tool
}

// NewRegistry creates an empty registry.
func NewRegistry() *Registry { return &Registry{tools: map[string]Tool{}} }

// Register adds or replaces a tool.
func (r *Registry) Register(t Tool) { r.tools[t.Name] = t }

// Get returns a tool by name.
func (r *Registry) Get(name string) (Tool, bool) { t, ok := r.tools[name]; return t, ok }

// Schemas returns the LLM-facing schema for every tool, name-sorted for determinism.
func (r *Registry) Schemas() []llm.ToolSchema {
 out := make([]llm.ToolSchema, 0, len(r.tools))
 for _, t := range r.tools {
  schema := t.Schema
  if len(schema) == 0 {
   schema = []byte(`{"type":"object","properties":{}}`)
  }
  out = append(out, llm.ToolSchema{Name: t.Name, Description: t.Description, Schema: schema})
 }
 sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
 return out
}
```

```go
// daemon/services/agent/tools/builtin.go
package tools

import (
 "context"
 "encoding/json"
 "fmt"

 "github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
 "github.com/ruaan-deysel/unraid-management-agent/daemon/lib"
)

// StateProvider supplies read-only cache snapshots. Satisfied by the API server.
// Each method returns the value and whether it is currently available.
type StateProvider interface {
 SystemJSON() (any, bool)
 ArrayJSON() (any, bool)
 DockerJSON() (any, bool)
}

// DockerActor performs low-risk container actions. Satisfied by controllers.DockerController.
type DockerActor interface {
 Restart(id string) error
}

func marshalState(v any, ok bool, label string) (string, error) {
 if !ok {
  return label + " not available yet", nil
 }
 b, err := json.Marshal(v)
 if err != nil {
  return "", fmt.Errorf("marshal %s: %w", label, err)
 }
 return string(b), nil
}

// BuildDefault wires the Phase-1 tool set: read-only reads + low-risk Docker restart.
func BuildDefault(state StateProvider, docker DockerActor) *Registry {
 r := NewRegistry()

 r.Register(Tool{
  Name: "get_system_info", RiskTier: dto.RiskReadOnly,
  Description: "Get current system info: CPU, RAM, temperatures, uptime.",
  Invoke: func(_ context.Context, _ string) (string, error) {
   v, ok := state.SystemJSON()
   return marshalState(v, ok, "system info")
  },
 })
 r.Register(Tool{
  Name: "get_array_status", RiskTier: dto.RiskReadOnly,
  Description: "Get the Unraid array status (state, capacity, parity).",
  Invoke: func(_ context.Context, _ string) (string, error) {
   v, ok := state.ArrayJSON()
   return marshalState(v, ok, "array status")
  },
 })
 r.Register(Tool{
  Name: "list_docker_containers", RiskTier: dto.RiskReadOnly,
  Description: "List Docker containers and their current state.",
  Invoke: func(_ context.Context, _ string) (string, error) {
   v, ok := state.DockerJSON()
   return marshalState(v, ok, "docker containers")
  },
 })

 r.Register(Tool{
  Name: "restart_container", RiskTier: dto.RiskLow,
  Description: "Restart a Docker container by ID. Low-risk, reversible.",
  Schema:      []byte(`{"type":"object","properties":{"container_id":{"type":"string"}},"required":["container_id"]}`),
  Invoke: func(_ context.Context, argsJSON string) (string, error) {
   var a struct {
    ContainerID string `json:"container_id"`
   }
   if err := json.Unmarshal([]byte(argsJSON), &a); err != nil {
    return "", fmt.Errorf("parse args: %w", err)
   }
   if err := lib.ValidateContainerID(a.ContainerID); err != nil {
    return "", err
   }
   if err := docker.Restart(a.ContainerID); err != nil {
    return "", err
   }
   return fmt.Sprintf("Container %s restarted.", a.ContainerID), nil
  },
 })

 return r
}
```

> Note: `lib.ValidateContainerID` already exists (`daemon/lib/validation.go`). If a container ID shorter than 12 hex chars must be accepted from the model, the validation error is surfaced to the agent as the tool result — do **not** weaken validation.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./daemon/services/agent/tools/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add daemon/services/agent/tools/
git commit -m "feat(agent): risk-tiered tool registry with read-only + low-risk tools"
```

---

## Task 5: Session store (`agent/store.go`)

**Goal:** In-memory session map with JSON persistence, following the `watchdog.Store` pattern (default dir `/boot/config/plugins/unraid-management-agent`).

**Files:**

- Create: `daemon/services/agent/store.go`
- Test: `daemon/services/agent/store_test.go`

**Acceptance Criteria:**

- [ ] `NewStore("")` uses the default config dir; `NewStore(dir)` uses the given dir.
- [ ] `Put`/`Get`/`List` are concurrency-safe; `List` returns newest-first.
- [ ] `Save` then `Load` round-trips sessions to/from JSON on disk.

**Verify:** `go test ./daemon/services/agent/ -run Store -v` → PASS

**Steps:**

- [ ] **Step 1: Write the failing test**

```go
// daemon/services/agent/store_test.go
package agent

import (
 "testing"
 "time"

 "github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

func TestStoreRoundTrip(t *testing.T) {
 dir := t.TempDir()
 s := NewStore(dir)
 s.Put(dto.AgentSession{ID: "a", Goal: "g1", Status: dto.SessionCompleted, StartedAt: time.Now()})
 s.Put(dto.AgentSession{ID: "b", Goal: "g2", Status: dto.SessionRunning, StartedAt: time.Now().Add(time.Second)})

 if err := s.Save(); err != nil {
  t.Fatalf("save: %v", err)
 }
 s2 := NewStore(dir)
 if err := s2.Load(); err != nil {
  t.Fatalf("load: %v", err)
 }
 got, ok := s2.Get("a")
 if !ok || got.Goal != "g1" {
  t.Fatalf("reload missing session a: %+v ok=%v", got, ok)
 }
 list := s2.List()
 if len(list) != 2 || list[0].ID != "b" {
  t.Fatalf("expected newest-first list, got %+v", list)
 }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./daemon/services/agent/ -run Store -v`
Expected: FAIL (undefined: NewStore)

- [ ] **Step 3: Write the implementation**

```go
// daemon/services/agent/store.go
package agent

import (
 "encoding/json"
 "fmt"
 "os"
 "path/filepath"
 "sort"
 "sync"

 "github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
 "github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
)

const (
 // DefaultConfigDir matches the watchdog/alert stores.
 DefaultConfigDir = "/boot/config/plugins/unraid-management-agent"
 // SessionsFile is the on-disk session log filename.
 SessionsFile = "agent_sessions.json"
 // MaxStoredSessions bounds the persisted session history.
 MaxStoredSessions = 200
)

// Store persists agent sessions to a JSON file and serves them from memory.
type Store struct {
 mu       sync.RWMutex
 sessions map[string]dto.AgentSession
 filePath string
}

// NewStore creates a session store. Empty dir uses DefaultConfigDir.
func NewStore(configDir string) *Store {
 if configDir == "" {
  configDir = DefaultConfigDir
 }
 return &Store{
  sessions: make(map[string]dto.AgentSession),
  filePath: filepath.Join(configDir, SessionsFile),
 }
}

// Put inserts or updates a session.
func (s *Store) Put(sess dto.AgentSession) {
 s.mu.Lock()
 defer s.mu.Unlock()
 s.sessions[sess.ID] = sess
}

// Get returns a session by ID.
func (s *Store) Get(id string) (dto.AgentSession, bool) {
 s.mu.RLock()
 defer s.mu.RUnlock()
 v, ok := s.sessions[id]
 return v, ok
}

// List returns all sessions, newest StartedAt first.
func (s *Store) List() []dto.AgentSession {
 s.mu.RLock()
 defer s.mu.RUnlock()
 out := make([]dto.AgentSession, 0, len(s.sessions))
 for _, v := range s.sessions {
  out = append(out, v)
 }
 sort.Slice(out, func(i, j int) bool { return out[i].StartedAt.After(out[j].StartedAt) })
 return out
}

// Save writes the most recent MaxStoredSessions sessions to disk.
func (s *Store) Save() error {
 list := s.List()
 if len(list) > MaxStoredSessions {
  list = list[:MaxStoredSessions]
 }
 if err := os.MkdirAll(filepath.Dir(s.filePath), 0o755); err != nil {
  return fmt.Errorf("creating agent config dir: %w", err)
 }
 data, err := json.MarshalIndent(list, "", "  ")
 if err != nil {
  return fmt.Errorf("marshal sessions: %w", err)
 }
 if err := os.WriteFile(s.filePath, data, 0o600); err != nil {
  return fmt.Errorf("writing sessions: %w", err)
 }
 return nil
}

// Load reads sessions from disk. A missing file is not an error.
func (s *Store) Load() error {
 data, err := os.ReadFile(s.filePath)
 if err != nil {
  if os.IsNotExist(err) {
   logger.Info("Agent: no session file yet, starting empty")
   return nil
  }
  return fmt.Errorf("reading sessions: %w", err)
 }
 var list []dto.AgentSession
 if err := json.Unmarshal(data, &list); err != nil {
  return fmt.Errorf("unmarshal sessions: %w", err)
 }
 s.mu.Lock()
 defer s.mu.Unlock()
 for _, sess := range list {
  s.sessions[sess.ID] = sess
 }
 return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./daemon/services/agent/ -run Store -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add daemon/services/agent/store.go daemon/services/agent/store_test.go
git commit -m "feat(agent): JSON-persisted session store"
```

---

## Task 6: Agent loop + service (`agent/loop.go`, `agent/service.go`)

**Goal:** Implement the bounded ReAct loop and the `Service` facade used by the API. Read-only and low-risk tools auto-execute; the loop stops on a text answer, the iteration cap, or the token cap.

**Files:**

- Create: `daemon/services/agent/service.go`
- Create: `daemon/services/agent/loop.go`
- Test: `daemon/services/agent/loop_test.go`

**Acceptance Criteria:**

- [ ] `Service.StartSession(ctx, goal)` runs the loop synchronously and returns a completed `AgentSession`.
- [ ] Given a mock scripted to call `get_system_info` then answer, the session ends `completed`, records one tool call, and `Answer` is set.
- [ ] The loop stops with status `completed` and a truncation note if `MaxIterations` is hit without a final answer.
- [ ] `TokensUsed` accumulates input+output tokens across steps.
- [ ] Each step broadcasts a `dto.WSEvent` via the injected `Broadcaster`.

**Verify:** `go test ./daemon/services/agent/ -run Loop -v` → PASS

**Steps:**

- [ ] **Step 1: Write the failing test**

```go
// daemon/services/agent/loop_test.go
package agent

import (
 "context"
 "testing"

 "github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
 "github.com/ruaan-deysel/unraid-management-agent/daemon/services/agent/llm"
 "github.com/ruaan-deysel/unraid-management-agent/daemon/services/agent/tools"
)

type fakeState struct{}

func (fakeState) SystemJSON() (any, bool) { return map[string]string{"host": "tower"}, true }
func (fakeState) ArrayJSON() (any, bool)  { return map[string]string{"state": "STARTED"}, true }
func (fakeState) DockerJSON() (any, bool) { return []string{"plex"}, true }

type fakeDocker struct{}

func (fakeDocker) Restart(string) error { return nil }

type capturingBroadcaster struct{ events int }

func (c *capturingBroadcaster) BroadcastAgentEvent(dto.WSEvent) { c.events++ }

func newTestService(p llm.Provider) (*Service, *capturingBroadcaster) {
 cfg := dto.DefaultAgentConfig()
 cfg.Enabled = true
 reg := tools.BuildDefault(fakeState{}, fakeDocker{})
 bc := &capturingBroadcaster{}
 return NewService(cfg, p, reg, NewStore(testDir()), bc), bc
}

func testDir() string { return "" } // overridden below via t.TempDir in real test

func TestLoopToolThenAnswer(t *testing.T) {
 p := llm.NewMockProvider(
  &llm.ChatResponse{ToolCalls: []llm.ToolCall{{ID: "1", Name: "get_system_info", Args: "{}"}}, InputTokens: 10, OutputTokens: 5},
  &llm.ChatResponse{Text: "Host is tower, all healthy.", InputTokens: 8, OutputTokens: 7},
 )
 cfg := dto.DefaultAgentConfig()
 cfg.Enabled = true
 reg := tools.BuildDefault(fakeState{}, fakeDocker{})
 bc := &capturingBroadcaster{}
 svc := NewService(cfg, p, reg, NewStore(t.TempDir()), bc)

 sess, err := svc.StartSession(context.Background(), "is my system healthy?")
 if err != nil {
  t.Fatalf("start: %v", err)
 }
 if sess.Status != dto.SessionCompleted {
  t.Fatalf("status=%q err=%q", sess.Status, sess.Error)
 }
 if sess.Answer == "" {
  t.Fatal("expected an answer")
 }
 if sess.TokensUsed != 30 {
  t.Fatalf("tokens=%d want 30", sess.TokensUsed)
 }
 gotCalls := 0
 for _, s := range sess.Steps {
  gotCalls += len(s.ToolCalls)
 }
 if gotCalls != 1 {
  t.Fatalf("expected 1 tool call, got %d", gotCalls)
 }
 if bc.events == 0 {
  t.Fatal("expected broadcast events")
 }
}

func TestLoopHitsIterationCap(t *testing.T) {
 // Always asks for a tool, never answers.
 loop := []*llm.ChatResponse{}
 for i := 0; i < 50; i++ {
  loop = append(loop, &llm.ChatResponse{ToolCalls: []llm.ToolCall{{ID: "x", Name: "get_system_info", Args: "{}"}}, OutputTokens: 1})
 }
 p := llm.NewMockProvider(loop...)
 cfg := dto.DefaultAgentConfig()
 cfg.Enabled = true
 cfg.MaxIterations = 3
 svc := NewService(cfg, p, tools.BuildDefault(fakeState{}, fakeDocker{}), NewStore(t.TempDir()), &capturingBroadcaster{})

 sess, err := svc.StartSession(context.Background(), "loop forever")
 if err != nil {
  t.Fatalf("start: %v", err)
 }
 if len(sess.Steps) > 3 {
  t.Fatalf("expected <=3 steps, got %d", len(sess.Steps))
 }
}
```

> Delete the placeholder `newTestService`/`testDir` helpers before running — they exist only to show the construction shape; the real tests build the service inline with `t.TempDir()`. (Remove the unused helpers so the package compiles.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./daemon/services/agent/ -run Loop -v`
Expected: FAIL (undefined: NewService, Service)

- [ ] **Step 3: Write the implementation**

```go
// daemon/services/agent/service.go
package agent

import (
 "context"
 "fmt"
 "sync"

 "github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
 "github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
 "github.com/ruaan-deysel/unraid-management-agent/daemon/services/agent/llm"
 "github.com/ruaan-deysel/unraid-management-agent/daemon/services/agent/tools"
)

// Broadcaster streams agent events to WebSocket clients. Satisfied by the API server.
type Broadcaster interface {
 BroadcastAgentEvent(event dto.WSEvent)
}

// Service is the agent facade used by the API layer.
type Service struct {
 cfg      dto.AgentConfig
 provider llm.Provider
 tools    *tools.Registry
 store    *Store
 bc       Broadcaster

 mu  sync.Mutex
 seq int
}

// NewService constructs the agent service.
func NewService(cfg dto.AgentConfig, provider llm.Provider, reg *tools.Registry, store *Store, bc Broadcaster) *Service {
 return &Service{cfg: cfg, provider: provider, tools: reg, store: store, bc: bc}
}

// Enabled reports whether the agent is configured to run.
func (s *Service) Enabled() bool { return s.cfg.Enabled && s.provider != nil }

// nextID returns a monotonically increasing session ID (deterministic, no clock dependency).
func (s *Service) nextID() string {
 s.mu.Lock()
 defer s.mu.Unlock()
 s.seq++
 return fmt.Sprintf("sess-%d", s.seq)
}

// GetSession returns a stored session by ID.
func (s *Service) GetSession(id string) (dto.AgentSession, bool) { return s.store.Get(id) }

// ListSessions returns all sessions newest-first.
func (s *Service) ListSessions() []dto.AgentSession { return s.store.List() }

// StartSession runs a new agent session to completion (synchronous in Phase 1).
func (s *Service) StartSession(ctx context.Context, goal string) (dto.AgentSession, error) {
 if !s.Enabled() {
  return dto.AgentSession{}, fmt.Errorf("agent is disabled")
 }
 sess := s.runLoop(ctx, s.nextID(), goal)
 s.store.Put(sess)
 if err := s.store.Save(); err != nil {
  logger.Warning("Agent: failed to persist session %s: %v", sess.ID, err)
 }
 return sess, nil
}
```

```go
// daemon/services/agent/loop.go
package agent

import (
 "context"
 "fmt"
 "time"

 "github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
 "github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
 "github.com/ruaan-deysel/unraid-management-agent/daemon/services/agent/llm"
)

const systemPrompt = `You are the Unraid Management Agent's autonomous operator. ` +
 `Investigate the user's goal using the provided tools, then give a concise answer. ` +
 `Only call tools that exist. When you have enough information, reply with a final text answer and no tool calls.`

// runLoop executes the bounded ReAct cycle and returns the finished session.
func (s *Service) runLoop(ctx context.Context, id, goal string) dto.AgentSession {
 sess := dto.AgentSession{ID: id, Goal: goal, Status: dto.SessionRunning, StartedAt: time.Now()}
 s.emit(&sess, "session_started", nil)

 deadline := time.Duration(s.cfg.SessionDeadlineSecs) * time.Second
 loopCtx := ctx
 if deadline > 0 {
  var cancel context.CancelFunc
  loopCtx, cancel = context.WithTimeout(ctx, deadline)
  defer cancel()
 }

 messages := []llm.Message{{Role: "user", Content: goal}}
 schemas := s.tools.Schemas()

 for i := 0; i < s.cfg.MaxIterations; i++ {
  if sess.TokensUsed >= s.cfg.MaxTokensPerSession {
   s.finish(&sess, dto.SessionCompleted, "Stopped: token budget reached.")
   return sess
  }

  resp, err := s.provider.Chat(loopCtx, llm.ChatRequest{
   System: systemPrompt, Messages: messages, Tools: schemas,
   MaxTokens: 4096,
  })
  if err != nil {
   s.fail(&sess, fmt.Sprintf("provider error: %v", err))
   return sess
  }
  sess.TokensUsed += resp.InputTokens + resp.OutputTokens

  step := dto.AgentStep{Index: i, Thought: resp.Text, At: time.Now()}

  // No tool calls => final answer.
  if len(resp.ToolCalls) == 0 {
   sess.Steps = append(sess.Steps, step)
   s.emit(&sess, "step_completed", step)
   s.finish(&sess, dto.SessionCompleted, resp.Text)
   return sess
  }

  // Record the assistant's tool-call turn so the provider keeps context.
  messages = append(messages, llm.Message{Role: "assistant", Content: resp.Text})

  for _, call := range resp.ToolCalls {
   rec := s.executeCall(loopCtx, call)
   step.ToolCalls = append(step.ToolCalls, rec)
   messages = append(messages, llm.Message{Role: "tool", ToolCallID: call.ID, Content: rec.Result})
   s.emit(&sess, "tool_called", rec)
  }
  sess.Steps = append(sess.Steps, step)
  s.emit(&sess, "step_completed", step)
 }

 s.finish(&sess, dto.SessionCompleted, "Stopped: reached maximum reasoning steps without a final answer.")
 return sess
}

// executeCall runs one tool call under the tiered policy and returns a record.
func (s *Service) executeCall(ctx context.Context, call llm.ToolCall) dto.AgentToolCall {
 rec := dto.AgentToolCall{Name: call.Name, Args: call.Args, At: time.Now()}

 tool, ok := s.tools.Get(call.Name)
 if !ok {
  rec.Error = "unknown tool"
  rec.Result = fmt.Sprintf("Error: tool %q does not exist.", call.Name)
  return rec
 }
 rec.RiskTier = tool.RiskTier

 // Phase-1 policy: read-only and low-risk auto-execute; anything else is refused.
 mode := s.cfg.Autonomy[tool.RiskTier]
 if mode != dto.ModeAuto {
  rec.Error = "requires approval"
  rec.Result = fmt.Sprintf("Action %q (risk=%s) requires approval, which is not available yet. Skipped.", call.Name, tool.RiskTier)
  return rec
 }

 out, err := tool.Invoke(ctx, call.Args)
 if err != nil {
  rec.Error = err.Error()
  rec.Result = "Error: " + err.Error()
  return rec
 }
 rec.Result = out
 return rec
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./daemon/services/agent/ -run Loop -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add daemon/services/agent/service.go daemon/services/agent/loop.go daemon/services/agent/loop_test.go
git commit -m "feat(agent): bounded ReAct loop + service facade"
```

---

## Task 7: REST + WebSocket surface (`api/handlers_agent.go`)

**Goal:** Expose the agent over REST, add `SetAgent` wiring + the `BroadcastAgentEvent` method (so the API server satisfies `agent.Broadcaster`), and route the endpoints.

**Files:**

- Create: `daemon/services/api/handlers_agent.go`
- Modify: `daemon/services/api/server.go` (add `agentSvc` field, `SetAgent`, `BroadcastAgentEvent`, routes)
- Test: `daemon/services/api/handlers_agent_test.go`

**Acceptance Criteria:**

- [ ] `POST /api/v1/agent/sessions` with `{"goal":"..."}` returns the completed session as JSON.
- [ ] `GET /api/v1/agent/sessions` lists sessions; `GET /api/v1/agent/sessions/{id}` returns one or 404.
- [ ] When the agent is unset/disabled, control endpoints return HTTP 503 with `dto.Response{Success:false}`.
- [ ] `BroadcastAgentEvent` forwards to `wsHub.Broadcast("agent_stream", event)`.

**Verify:** `go test ./daemon/services/api/ -run Agent -v` → PASS

**Steps:**

- [ ] **Step 1: Write the failing test**

```go
// daemon/services/api/handlers_agent_test.go
package api

import (
 "net/http"
 "net/http/httptest"
 "strings"
 "testing"

 "github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
 "github.com/ruaan-deysel/unraid-management-agent/daemon/services/agent"
 "github.com/ruaan-deysel/unraid-management-agent/daemon/services/agent/llm"
 "github.com/ruaan-deysel/unraid-management-agent/daemon/services/agent/tools"
)

type agentTestState struct{}

func (agentTestState) SystemJSON() (any, bool) { return map[string]string{"host": "tower"}, true }
func (agentTestState) ArrayJSON() (any, bool)  { return map[string]string{"state": "STARTED"}, true }
func (agentTestState) DockerJSON() (any, bool) { return []string{}, true }

type agentTestDocker struct{}

func (agentTestDocker) Restart(string) error { return nil }

func newAgentServer(t *testing.T) *Server {
 t.Helper()
 s := NewServer(testContext()) // testContext() helper already used in api tests
 cfg := dto.DefaultAgentConfig()
 cfg.Enabled = true
 p := llm.NewMockProvider(&llm.ChatResponse{Text: "Healthy.", OutputTokens: 3})
 reg := tools.BuildDefault(agentTestState{}, agentTestDocker{})
 svc := agent.NewService(cfg, p, reg, agent.NewStore(t.TempDir()), s)
 s.SetAgent(svc)
 return s
}

func TestAgentStartSession(t *testing.T) {
 s := newAgentServer(t)
 req := httptest.NewRequest(http.MethodPost, "/api/v1/agent/sessions", strings.NewReader(`{"goal":"status?"}`))
 rr := httptest.NewRecorder()
 s.GetRouter().ServeHTTP(rr, req)
 if rr.Code != http.StatusOK {
  t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
 }
 if !strings.Contains(rr.Body.String(), "Healthy.") {
  t.Fatalf("expected answer in body, got %s", rr.Body.String())
 }
}

func TestAgentDisabledReturns503(t *testing.T) {
 s := NewServer(testContext())
 req := httptest.NewRequest(http.MethodPost, "/api/v1/agent/sessions", strings.NewReader(`{"goal":"x"}`))
 rr := httptest.NewRecorder()
 s.GetRouter().ServeHTTP(rr, req)
 if rr.Code != http.StatusServiceUnavailable {
  t.Fatalf("expected 503, got %d", rr.Code)
 }
}
```

> If `testContext()` does not already exist in the api test package, reuse the context-construction helper the other `api` tests use (grep `func testContext` / how `server_test.go` builds a `*domain.Context`). Match whatever the existing tests do.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./daemon/services/api/ -run Agent -v`
Expected: FAIL (undefined: SetAgent)

- [ ] **Step 3: Add the server field, wiring, broadcaster, and routes**

In `daemon/services/api/server.go`, add to the `Server` struct (after `tuningController`):

```go
 agentSvc *agent.Service
```

Add the import `"github.com/ruaan-deysel/unraid-management-agent/daemon/services/agent"` to the server.go import block.

Add these methods (near the other `Set*` methods, ~line 683):

```go
// SetAgent wires the agent service into the API server.
func (s *Server) SetAgent(svc *agent.Service) {
 s.agentSvc = svc
}

// BroadcastAgentEvent implements agent.Broadcaster: streams agent events to WS clients
// subscribed to the "agent_stream" topic.
func (s *Server) BroadcastAgentEvent(event dto.WSEvent) {
 if s.wsHub != nil {
  s.wsHub.Broadcast("agent_stream", event)
 }
}
```

In `setupRoutes()` (after the watchdog/alert routes block), add:

```go
 // Agent (Phase 1: on-demand sessions)
 api.HandleFunc("/agent/sessions", s.handleAgentStartSession).Methods("POST")
 api.HandleFunc("/agent/sessions", s.handleAgentListSessions).Methods("GET")
 api.HandleFunc("/agent/sessions/{id}", s.handleAgentGetSession).Methods("GET")
```

- [ ] **Step 4: Write the handlers**

```go
// daemon/services/api/handlers_agent.go
package api

import (
 "encoding/json"
 "net/http"
 "time"

 "github.com/gorilla/mux"

 "github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

// handleAgentStartSession starts a new on-demand agent session and returns the result.
func (s *Server) handleAgentStartSession(w http.ResponseWriter, r *http.Request) {
 if s.agentSvc == nil || !s.agentSvc.Enabled() {
  respondJSON(w, http.StatusServiceUnavailable, dto.Response{
   Success: false, Message: "agent is disabled", Timestamp: time.Now(),
  })
  return
 }
 var body struct {
  Goal string `json:"goal"`
 }
 if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Goal == "" {
  respondWithError(w, http.StatusBadRequest, "request body must include a non-empty 'goal'")
  return
 }
 sess, err := s.agentSvc.StartSession(r.Context(), body.Goal)
 if err != nil {
  respondWithError(w, http.StatusInternalServerError, err.Error())
  return
 }
 respondJSON(w, http.StatusOK, sess)
}

// handleAgentListSessions returns all agent sessions, newest-first.
func (s *Server) handleAgentListSessions(w http.ResponseWriter, _ *http.Request) {
 if s.agentSvc == nil {
  respondJSON(w, http.StatusServiceUnavailable, dto.Response{
   Success: false, Message: "agent is disabled", Timestamp: time.Now(),
  })
  return
 }
 respondJSON(w, http.StatusOK, s.agentSvc.ListSessions())
}

// handleAgentGetSession returns a single agent session by ID.
func (s *Server) handleAgentGetSession(w http.ResponseWriter, r *http.Request) {
 if s.agentSvc == nil {
  respondJSON(w, http.StatusServiceUnavailable, dto.Response{
   Success: false, Message: "agent is disabled", Timestamp: time.Now(),
  })
  return
 }
 id := mux.Vars(r)["id"]
 sess, ok := s.agentSvc.GetSession(id)
 if !ok {
  respondWithError(w, http.StatusNotFound, "session not found")
  return
 }
 respondJSON(w, http.StatusOK, sess)
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./daemon/services/api/ -run Agent -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add daemon/services/api/handlers_agent.go daemon/services/api/handlers_agent_test.go daemon/services/api/server.go
git commit -m "feat(agent): REST endpoints + WS broadcaster for agent sessions"
```

---

## Task 8: Orchestrator wiring + config loading

**Goal:** Construct the agent service in `orchestrator.go` (disabled by default), wire it into the API server, and ensure the whole daemon builds and all tests pass. No behavior change unless the user opts in.

**Files:**

- Modify: `daemon/services/orchestrator.go` (HTTP-mode `Run`, after the watchdog block ~line 123)
- Create: `daemon/services/agent/bootstrap.go` (config load + provider selection helper)
- Test: `daemon/services/agent/bootstrap_test.go`

**Acceptance Criteria:**

- [ ] `LoadConfig(dir)` returns `DefaultAgentConfig()` when no config file exists.
- [ ] `BuildService` returns `(nil, nil)` when config is disabled (so the orchestrator skips wiring cleanly).
- [ ] `BuildService` reads the API key from env var `UMA_AGENT_API_KEY` when set.
- [ ] `go build ./...` succeeds and `make test` passes.

**Verify:** `go test ./daemon/services/agent/ -run Bootstrap -v && go build ./...` → PASS

**Steps:**

- [ ] **Step 1: Write the failing test**

```go
// daemon/services/agent/bootstrap_test.go
package agent

import (
 "testing"

 "github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

func TestLoadConfigDefaultsWhenMissing(t *testing.T) {
 cfg := LoadConfig(t.TempDir())
 if cfg.Enabled {
  t.Fatal("expected disabled default config")
 }
}

func TestBuildServiceNilWhenDisabled(t *testing.T) {
 cfg := dto.DefaultAgentConfig() // Enabled=false
 svc, err := BuildService(cfg, t.TempDir(), nil, nil, nil)
 if err != nil {
  t.Fatalf("unexpected err: %v", err)
 }
 if svc != nil {
  t.Fatal("expected nil service when disabled")
 }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./daemon/services/agent/ -run Bootstrap -v`
Expected: FAIL (undefined: LoadConfig, BuildService)

- [ ] **Step 3: Write the bootstrap helper**

```go
// daemon/services/agent/bootstrap.go
package agent

import (
 "encoding/json"
 "fmt"
 "os"
 "path/filepath"

 "github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
 "github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
 "github.com/ruaan-deysel/unraid-management-agent/daemon/services/agent/llm"
 "github.com/ruaan-deysel/unraid-management-agent/daemon/services/agent/tools"
)

// AgentConfigFile is the on-disk agent configuration filename.
const AgentConfigFile = "agent_config.json"

// APIKeyEnv is the environment variable that supplies the LLM API key.
const APIKeyEnv = "UMA_AGENT_API_KEY"

// LoadConfig reads agent_config.json, falling back to safe defaults. Empty dir uses DefaultConfigDir.
func LoadConfig(configDir string) dto.AgentConfig {
 if configDir == "" {
  configDir = DefaultConfigDir
 }
 cfg := dto.DefaultAgentConfig()
 data, err := os.ReadFile(filepath.Join(configDir, AgentConfigFile))
 if err != nil {
  return cfg
 }
 if err := json.Unmarshal(data, &cfg); err != nil {
  logger.Warning("Agent: invalid config file, using defaults: %v", err)
  return dto.DefaultAgentConfig()
 }
 return cfg
}

// BuildService assembles the agent service from config. Returns (nil, nil) when disabled.
// The API key is sourced from the environment (never persisted).
func BuildService(cfg dto.AgentConfig, configDir string, state tools.StateProvider, docker tools.DockerActor, bc Broadcaster) (*Service, error) {
 if !cfg.Enabled {
  return nil, nil
 }
 if key := os.Getenv(APIKeyEnv); key != "" {
  cfg.APIKey = key
 }

 var provider llm.Provider
 switch cfg.Provider {
 case "anthropic":
  if cfg.APIKey == "" {
   return nil, fmt.Errorf("agent enabled but %s is not set", APIKeyEnv)
  }
  provider = llm.NewAnthropicProvider(cfg.APIKey, cfg.Model, cfg.Endpoint)
 default:
  return nil, fmt.Errorf("unsupported agent provider %q", cfg.Provider)
 }

 store := NewStore(configDir)
 if err := store.Load(); err != nil {
  logger.Warning("Agent: failed to load sessions: %v", err)
 }
 reg := tools.BuildDefault(state, docker)
 return NewService(cfg, provider, reg, store, bc), nil
}
```

- [ ] **Step 4: Wire into the orchestrator**

In `daemon/services/orchestrator.go`, after the watchdog block (after `logger.Success("Watchdog started")`, ~line 123), add. The API server must satisfy `tools.StateProvider` and `agent.Broadcaster`; pass a Docker controller for low-risk actions:

```go
 // Initialize agent (disabled by default; opt-in via agent_config.json + UMA_AGENT_API_KEY)
 agentCfg := agent.LoadConfig("")
 agentDocker := controllers.NewDockerController()
 agentSvc, err := agent.BuildService(agentCfg, "", apiServer, agentDocker, apiServer)
 if err != nil {
  logger.Warning("Agent disabled: %v", err)
 } else if agentSvc != nil {
  apiServer.SetAgent(agentSvc)
  logger.Success("Agent service started (provider=%s, model=%s)", agentCfg.Provider, agentCfg.Model)
 }
```

Add `"github.com/ruaan-deysel/unraid-management-agent/daemon/services/agent"` to the orchestrator imports (the `controllers` package is already imported).

> **StateProvider on the API server:** the `api.Server` must implement `SystemJSON`/`ArrayJSON`/`DockerJSON`. Add these thin methods in `handlers_agent.go` (or `server.go`) that read the embedded `CacheStore` and return `(value, ok)`. Use the same cache accessors the MCP server uses (`GetSystemCache`/`GetArrayCache`/`GetDockerCache` or their `*CacheStore` equivalents — grep `func (.*CacheStore) Get` to find exact names). Example:

```go
// In daemon/services/api/handlers_agent.go
func (s *Server) SystemJSON() (any, bool) { v := s.GetSystemCache(); return v, v != nil }
func (s *Server) ArrayJSON() (any, bool)  { v := s.GetArrayCache(); return v, v != nil }
func (s *Server) DockerJSON() (any, bool) { v := s.GetDockerCache(); return v, v != nil }
```

> Confirm the exact cache getter names by grepping `daemon/services/api/cache_store.go`; if they differ (e.g. `Containers()`), use the real names. Do not add new caching — only expose what already exists.

- [ ] **Step 5: Build and run the full test suite**

Run: `go build ./... && go test ./daemon/services/agent/... ./daemon/services/api/ -v`
Expected: build succeeds; agent + api tests PASS

- [ ] **Step 6: Commit**

```bash
git add daemon/services/agent/bootstrap.go daemon/services/agent/bootstrap_test.go daemon/services/orchestrator.go daemon/services/api/handlers_agent.go
git commit -m "feat(agent): orchestrator wiring + config loading (disabled by default)"
```

---

## Task 9: Docs, CHANGELOG, CodeRabbit review, and on-Unraid verification

**Goal:** **USER-ORDERED GATE — NON-SKIPPABLE.** This task was requested by the user in the current conversation. It MUST NOT be closed by walking around it, by declaring it "verified inline", or by substituting a cheaper check. Close only after every item in `acceptanceCriteria` has been re-validated independently, with output captured.

Document the new feature, run the CodeRabbit CLI review and address feedback, then build/test and deploy to real Unraid hardware and verify the agent endpoints work and the plugin is stable.

**Files:**

- Modify: `CHANGELOG.md` (new dated entry under Added)
- Modify: `docs/integrations/` (new `agent.md` describing config, env var, REST endpoints) and `AGENTS.md` if a new service dir warrants a mention
- Create: `docs/integrations/agent.md`

**Acceptance Criteria:**

- [ ] `CHANGELOG.md` has a new `## [YYYY.MM.DD]` entry describing the Agent Core (Phase 1) under **Added**.
- [ ] `docs/integrations/agent.md` documents: enabling via `agent_config.json`, the `UMA_AGENT_API_KEY` env var, risk tiers, and the three REST endpoints.
- [ ] `make pre-commit-run` passes (lint + security) with zero errors.
- [ ] `make test` passes (full suite, race detector).
- [ ] CodeRabbit CLI review has been run on the branch diff and all actionable findings are either fixed or explicitly noted as out-of-scope with reasoning.
- [ ] Plugin builds for Unraid and deploys via Ansible; the daemon starts cleanly (no panics in `/var/log/unraid-management-agent.log`).
- [ ] On Unraid: with the agent **disabled** (default), `GET /api/v1/agent/sessions` returns HTTP 503 and all existing endpoints behave exactly as before (no regression).
- [ ] On Unraid: with the agent **enabled** + a real API key, `POST /api/v1/agent/sessions {"goal":"is my array healthy?"}` returns a completed session whose answer references real array state.

**Verify:**

- `make pre-commit-run && make test` → all pass
- `coderabbit review` (CLI) on the branch → findings addressed
- `ansible-playbook -i ansible/inventory.yml ansible/deploy.yml --tags build,deploy,verify` → deploy + endpoint verification succeeds
- Manual curl on Unraid host (disabled): `curl -s -o /dev/null -w "%{http_code}" -X POST http://<unraid-ip>:8043/api/v1/agent/sessions -d '{"goal":"x"}'` → `503`
- Manual curl on Unraid host (enabled): `curl -s -X POST http://<unraid-ip>:8043/api/v1/agent/sessions -d '{"goal":"is my array healthy?"}'` → JSON with `"status":"completed"` and a non-empty `"answer"`

**Steps:**

- [ ] **Step 1: Update CHANGELOG + write docs**

Add a dated entry to `CHANGELOG.md` under a new version header, e.g.:

```markdown
## [YYYY.MM.DD]

### Added

- **Agent Core (Phase 1):** embedded autonomous operator with a pluggable LLM provider
  (Anthropic), a risk-tiered tool registry (read-only + low-risk auto-execute), a bounded
  ReAct loop, JSON-persisted sessions, and REST endpoints (`/api/v1/agent/sessions`) plus a
  WebSocket `agent_stream` event feed. Disabled by default; opt-in via `agent_config.json`
  and the `UMA_AGENT_API_KEY` environment variable.
```

Write `docs/integrations/agent.md` covering enable steps, env var, risk tiers, and the REST/WS surface (mirror the structure of `docs/integrations/mcp.md`).

- [ ] **Step 2: Lint, security, and full test suite**

Run: `make pre-commit-run`
Expected: all hooks pass (gofmt, goimports, golangci-lint, gosec, govulncheck).

Run: `make test`
Expected: full suite passes with `-race`.

- [ ] **Step 3: CodeRabbit CLI review**

Run the CodeRabbit CLI review on the branch diff (e.g. `coderabbit review` or the project's configured invocation; the `coderabbit:code-review` skill may be used). Capture findings, fix actionable items, and re-run until clean. Record any deliberately-deferred items in the PR description with rationale.

- [ ] **Step 4: Build + deploy + verify on Unraid**

Run: `ansible-playbook -i ansible/inventory.yml ansible/deploy.yml --tags build,deploy,verify`
Expected: build succeeds, plugin deploys, and the built-in endpoint verification passes.

Then tail the log on the Unraid host and confirm no panics:

```bash
ssh root@<unraid-ip> 'tail -n 50 /var/log/unraid-management-agent.log'
```

- [ ] **Step 5: Manual endpoint verification (disabled, then enabled)**

Disabled (default) — expect 503 and no regression:

```bash
curl -s -o /dev/null -w "%{http_code}\n" -X POST http://<unraid-ip>:8043/api/v1/agent/sessions -d '{"goal":"x"}'
curl -s -o /dev/null -w "%{http_code}\n" http://<unraid-ip>:8043/api/v1/system
```

Expected: `503` for agent, `200` for system.

Enable on the host (write `agent_config.json` with `"enabled":true`, set `UMA_AGENT_API_KEY`, restart the daemon), then:

```bash
curl -s -X POST http://<unraid-ip>:8043/api/v1/agent/sessions -d '{"goal":"is my array healthy?"}' | jq '{status, answer}'
```

Expected: `status` = `completed`, `answer` non-empty and referencing real array state.

- [ ] **Step 6: Commit**

```bash
git add CHANGELOG.md docs/integrations/agent.md AGENTS.md
git commit -m "docs(agent): document Agent Core Phase 1; verified on Unraid"
```

```json:metadata
{"files": ["CHANGELOG.md", "docs/integrations/agent.md"], "verifyCommand": "make pre-commit-run && make test && ansible-playbook -i ansible/inventory.yml ansible/deploy.yml --tags build,deploy,verify", "acceptanceCriteria": ["CHANGELOG has dated Agent Core entry", "docs/integrations/agent.md documents enable steps + env var + endpoints", "make pre-commit-run passes", "make test passes", "CodeRabbit CLI review run and findings addressed", "plugin builds + deploys to Unraid with no panics in log", "agent-disabled: POST /api/v1/agent/sessions returns 503 and existing endpoints unregressed", "agent-enabled: POST session returns completed status with answer referencing real array state"], "userGate": true, "tags": ["user-gate"], "gateScope": "all"}
```

---

## Self-Review Notes

- **Spec coverage (Phase 1 scope):** provider interface + mock (T2) + Anthropic (T3); risk-tiered registry (T4); session store (T5); bounded ReAct loop with iteration/token/deadline caps + tiered auto-execute (T6); REST + WS surface (T7); orchestrator wiring disabled-by-default + config/secret handling (T8); docs + CodeRabbit + Unraid verification (T9). Deferred items (`agent_wake` triggers, high-risk approval/pause-resume, memory/learning, MCP `agent_*` tools) are explicitly out of Phase 1 per the spec phasing.
- **Type consistency:** `dto.RiskTier`/`dto.AutonomyMode`/`dto.AgentSession` defined in T1 are used unchanged in T4/T5/T6/T7. `llm.Provider`/`ChatRequest`/`ChatResponse`/`ToolCall` from T2 are used by T3/T6. `tools.StateProvider`/`tools.DockerActor`/`Registry`/`Tool` from T4 are used by T6/T8. `agent.Broadcaster`/`Service`/`Store` are consistent across T5–T8. `BroadcastAgentEvent` name matches between the API server (T7) and the `Broadcaster` interface (T6).
- **Import-cycle check:** `agent` and `agent/tools` import only `dto`, `lib`, `logger`, `controllers`, and `agent/llm` — never `api`. `api` imports `agent`. No cycle.
- **Placeholders:** the only deliberately-illustrative code is the `newTestService`/`testDir` helper shape in T6, flagged for removal, and the cache-getter names in T8 flagged to confirm by grep against `cache_store.go`. These require a one-line lookup, not invention.
