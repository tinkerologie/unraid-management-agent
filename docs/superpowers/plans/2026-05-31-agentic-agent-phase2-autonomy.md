# Agentic Agent — Phase 2 (Autonomy) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers-extended-cc:subagent-driven-development (recommended) or superpowers-extended-cc:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the embedded agent autonomous: the cheap rule-based alerting/watchdog layer wakes the LLM agent on incidents (event-driven, debounced), and high-risk actions pause the session for human approval (pause/resume that survives restart), with a non-overridable forbid-list — all exposed over REST and MCP.

**Architecture:** Builds directly on Phase 1 (merged to `main`). The agent becomes "just another subscriber" on the existing typed pubsub bus: alerting Engine + watchdog Runner publish a `dto.AgentWakeEvent` to a new typed topic `agent_wake`; the agent's `trigger.go` subscribes (before collectors start), debounces/dedups by subsystem, and spawns autonomous sessions under concurrency/cooldown caps. The Phase-1 ReAct loop is refactored into a **resumable** form: when the policy gate hits an approval-required tier it persists the conversation transcript + a pending `ApprovalRequest` and returns; `ApproveAction` re-enters the loop. A background sweeper default-denies stale approvals. New REST (`/approve`, `/cancel`) and MCP (`agent_*`) surfaces drive it.

**Tech Stack:** Go 1.26, existing `domain.EventBus` typed pubsub (`domain.NewTopic`/`Publish`/`SubTopics`), `gorilla/mux`, the official `modelcontextprotocol/go-sdk` (`mcp.AddTool`). No new third-party dependencies.

**Reference spec:** `docs/superpowers/specs/2026-05-31-agentic-agent-design.md` (Phase 2 = "Autonomy")
**Builds on:** `docs/superpowers/plans/2026-05-31-agentic-agent-phase1-foundation.md` (merged, commit `926d784`)

**Phase-2 scope:** event-driven `agent_wake` (alerting + watchdog) with debounce/dedup/cooldown/concurrency; tiered **approval gate** with pause→resume that survives restart; approval **TTL default-deny** sweeper; non-overridable **forbid-list**; REST `/approve` + `/cancel`; MCP `agent_*` tools. **Deferred to Phase 3:** planner, episodic/semantic memory, recall, suggest-not-mutate learning, runbook reuse, multi-turn `/messages` chat continuation.

**Key existing facts (verified against `main`):**

- Phase-1 agent: `daemon/services/agent/` — `Service` (`service.go`), resumable-ish `runLoop` (`loop.go`), `Store` (JSON sessions), `tools.Registry`/`Tool{Name,Description,Schema,RiskTier,Invoke}`, `llm.Provider`/`Message{Role,Content,ToolCallID,ToolCalls}`/`ChatResponse`, `Broadcaster` iface, `bootstrap.go` (`LoadConfig`/`BuildService`).
- `dto.AgentSession{ID,Goal,Status,Steps,Answer,Error,TokensUsed,StartedAt,EndedAt}`; `dto.AgentToolCall`; `dto.AgentStep`; `RiskTier`(`RiskReadOnly|RiskLow|RiskHigh`); `AutonomyMode`(`ModeAuto|ModeApprove|ModeForbid`); statuses `SessionRunning|SessionCompleted|SessionFailed|SessionCancelled`.
- Typed pubsub: `domain.EventBus` with `Pub(msg,topics...)`, `Sub(topics...) chan any`, `SubTopics(...topicNamer) chan any`, `Unsub(ch,topics...)`; `domain.NewTopic[T]("name")`, `domain.Publish(bus,topic,data)`. Topics live in `daemon/constants/topics.go`. The hub is `ctx.Hub` (`*domain.EventBus`) in the orchestrator (`o.ctx.Hub`).
- Alerting `Engine` (`daemon/services/alerting/engine.go`): `NewEngine(store,provider)`, `Start(ctx)`, `evaluate()` dispatches firing `dto.AlertEvent`s (has `event.State=="firing"`, `event.Severity`, `event.RuleName`, `event.Message`). No hub field today.
- Watchdog `Runner` (`daemon/services/watchdog/runner.go`): `runCheck` sets `transitionedToUnhealthy`; has `check.Name`, `result.Error`. No hub field today.
- API server: `handlers_agent.go` (Phase-1 handlers + `SystemJSON/ArrayJSON/DockerJSON`), `server.go` (`agentSvc *agent.Service`, `SetAgent`, `BroadcastAgentEvent`, routes). Helpers `respondJSON`, `respondWithError`. `mux.Vars(r)["id"]`.
- MCP server (`daemon/services/mcp/server.go`): `Server` struct holds service refs; `Set*` methods; `register*Tools()` called from `Initialize()`; tools via `mcp.AddTool(s.mcpServer, &mcp.Tool{...}, handler)`; helpers `textResult`, `jsonResult`. No import cycle adding `agent` (agent does not import mcp).
- Orchestrator (`daemon/services/orchestrator.go`): HTTP-mode `Run` wires services after watchdog (`agentCfg := agent.LoadConfig("")` ... `if agentCfg.Enabled {...}` block ~line 125). `o.ctx.Hub` is the bus. Graceful-shutdown section closes controllers.

---

## File Structure

| File                                    | Responsibility                                                                                               | Action |
| --------------------------------------- | ------------------------------------------------------------------------------------------------------------ | ------ |
| `daemon/dto/agent.go`                   | Phase-2 DTOs: `AgentWakeEvent`, `ApprovalRequest`, `AgentMessage`, new status, session fields, config fields | Modify |
| `daemon/constants/topics.go`            | `TopicAgentWake` typed topic                                                                                 | Modify |
| `daemon/services/agent/loop.go`         | Refactor to resumable loop; approval pause; forbid-list                                                      | Modify |
| `daemon/services/agent/service.go`      | `ApproveAction`, `CancelSession`, hub/config fields, autonomous-session entrypoint                           | Modify |
| `daemon/services/agent/trigger.go`      | `agent_wake` subscription, debounce/dedup/cooldown/concurrency, TTL sweeper, `Start(ctx)`                    | Create |
| `daemon/services/agent/bootstrap.go`    | Pass hub into Service; defaults for new config fields                                                        | Modify |
| `daemon/services/alerting/engine.go`    | `SetEventBus` + publish `AgentWakeEvent` on firing                                                           | Modify |
| `daemon/services/watchdog/runner.go`    | `SetEventBus` + publish `AgentWakeEvent` on unhealthy transition                                             | Modify |
| `daemon/services/api/handlers_agent.go` | `/approve`, `/cancel` handlers                                                                               | Modify |
| `daemon/services/api/server.go`         | routes for approve/cancel                                                                                    | Modify |
| `daemon/services/mcp/server.go`         | `SetAgent` + `registerAgentTools()`                                                                          | Modify |
| `daemon/services/orchestrator.go`       | wire `SetEventBus`, `agentSvc.Start(ctx)` (before collectors), `mcpServer.SetAgent`                          | Modify |

---

## Task 1: Phase-2 DTOs, topic, and config fields

**Goal:** Add the data types and the typed pubsub topic Phase 2 needs, plus the new `AgentConfig` knobs, with safe defaults.

**Files:**

- Modify: `daemon/dto/agent.go`
- Modify: `daemon/constants/topics.go`
- Test: `daemon/dto/agent_test.go`

**Acceptance Criteria:**

- [ ] `dto.SessionAwaitingApproval` status constant exists.
- [ ] `dto.AgentWakeEvent`, `dto.ApprovalRequest`, `dto.AgentMessage`, `dto.AgentMsgToolCall` types exist with JSON tags.
- [ ] `dto.AgentSession` gains `PendingApproval *ApprovalRequest` and `Transcript []AgentMessage` (both `omitempty`).
- [ ] `dto.AgentConfig` gains `WakeDebounceSecs`, `WakeCooldownSecs`, `MaxConcurrentSessions`, `ApprovalTTLSecs int` and `ForbidList []string`; `DefaultAgentConfig()` sets sane non-zero defaults and a default forbid-list.
- [ ] `constants.TopicAgentWake` is a `domain.Topic[dto.AgentWakeEvent]` named `"agent_wake"`.

**Verify:** `go test ./daemon/dto/ -run TestAgent -v && go build ./daemon/constants/` → PASS

**Steps:**

- [ ] **Step 1: Extend the failing test** — append to `daemon/dto/agent_test.go`:

```go
func TestDefaultAgentConfigPhase2Defaults(t *testing.T) {
 cfg := DefaultAgentConfig()
 if cfg.WakeDebounceSecs <= 0 || cfg.WakeCooldownSecs <= 0 {
  t.Error("wake debounce/cooldown must be positive")
 }
 if cfg.MaxConcurrentSessions <= 0 {
  t.Error("max concurrent sessions must be positive")
 }
 if cfg.ApprovalTTLSecs <= 0 {
  t.Error("approval TTL must be positive")
 }
 if len(cfg.ForbidList) == 0 {
  t.Error("forbid-list should have default irreversible-op entries")
 }
}

func TestAgentSessionPendingApprovalRoundTrips(t *testing.T) {
 s := AgentSession{
  ID: "sess-1", Status: SessionAwaitingApproval,
  PendingApproval: &ApprovalRequest{ActionID: "act-1", ToolName: "stop_array", RiskTier: RiskHigh},
  Transcript:      []AgentMessage{{Role: "user", Content: "fix it"}},
 }
 b, err := json.Marshal(s)
 if err != nil {
  t.Fatalf("marshal: %v", err)
 }
 var back AgentSession
 if err := json.Unmarshal(b, &back); err != nil {
  t.Fatalf("unmarshal: %v", err)
 }
 if back.PendingApproval == nil || back.PendingApproval.ToolName != "stop_array" {
  t.Fatalf("pending approval lost: %+v", back.PendingApproval)
 }
 if len(back.Transcript) != 1 || back.Transcript[0].Role != "user" {
  t.Fatalf("transcript lost: %+v", back.Transcript)
 }
}
```

(Ensure `encoding/json` is imported in the test file.)

- [ ] **Step 2: Run, confirm FAIL** — `go test ./daemon/dto/ -run TestAgent -v` (undefined symbols).

- [ ] **Step 3: Edit `daemon/dto/agent.go`.** Add the status constant to the `AgentSessionStatus` block:

```go
 SessionAwaitingApproval AgentSessionStatus = "awaiting_approval"
```

Add new types (anywhere after the existing types):

```go
// AgentWakeEvent is published on the agent_wake topic to trigger an autonomous
// investigation. Source is "alert" or "watchdog"; Subsystem is the dedup key.
type AgentWakeEvent struct {
 Source    string    `json:"source"`    // "alert" | "watchdog"
 Subsystem string    `json:"subsystem"` // dedup key, e.g. "disk", "docker:plex", rule name
 Severity  string    `json:"severity"`  // "info" | "warning" | "critical"
 Title     string    `json:"title"`
 Detail    string    `json:"detail"`
 At        time.Time `json:"at"`
}

// AgentMsgToolCall is a tool call recorded inside a persisted transcript message.
type AgentMsgToolCall struct {
 ID   string `json:"id"`
 Name string `json:"name"`
 Args string `json:"args"`
}

// AgentMessage is a persisted conversation turn used to resume a paused loop.
// It mirrors the agent's llm.Message so a session can resume after an approval
// pause or a daemon restart.
type AgentMessage struct {
 Role       string             `json:"role"` // system|user|assistant|tool
 Content    string             `json:"content,omitempty"`
 ToolCallID string             `json:"tool_call_id,omitempty"`
 ToolCalls  []AgentMsgToolCall `json:"tool_calls,omitempty"`
}

// ApprovalRequest describes a high-risk tool call paused awaiting a human decision.
type ApprovalRequest struct {
 ActionID    string    `json:"action_id"`
 ToolName    string    `json:"tool_name"`
 Args        string    `json:"args"`
 RiskTier    RiskTier  `json:"risk_tier"`
 Reason      string    `json:"reason"` // the agent's stated intent for this action
 RequestedAt time.Time `json:"requested_at"`
}
```

Add fields to `AgentSession` (after `EndedAt`):

```go
 PendingApproval *ApprovalRequest `json:"pending_approval,omitempty"`
 Transcript      []AgentMessage   `json:"transcript,omitempty"`
```

Add fields to `AgentConfig` (after `SessionDeadlineSecs`):

```go
 WakeDebounceSecs      int      `json:"wake_debounce_secs"`
 WakeCooldownSecs      int      `json:"wake_cooldown_secs"`
 MaxConcurrentSessions int      `json:"max_concurrent_sessions"`
 ApprovalTTLSecs       int      `json:"approval_ttl_secs"`
 ForbidList            []string `json:"forbid_list"`
```

In `DefaultAgentConfig()` add to the returned struct literal:

```go
  WakeDebounceSecs:      30,
  WakeCooldownSecs:      300,
  MaxConcurrentSessions: 2,
  ApprovalTTLSecs:       3600,
  ForbidList:            []string{"format_disk", "clear_parity", "disable_parity", "partition_disk", "delete_array_disk"},
```

- [ ] **Step 4: Edit `daemon/constants/topics.go`** — add inside the `var (...)` block:

```go
 // TopicAgentWake is published by alerting/watchdog to wake the autonomous agent
 // with a dto.AgentWakeEvent describing the triggering incident.
 TopicAgentWake = domain.NewTopic[dto.AgentWakeEvent]("agent_wake")
```

- [ ] **Step 5: Run** `go test ./daemon/dto/ -run TestAgent -v` (PASS) and `go build ./daemon/constants/ ./...`. gofmt/goimports/vet clean.

- [ ] **Step 6: Commit**

```bash
git add daemon/dto/agent.go daemon/dto/agent_test.go daemon/constants/topics.go
git commit -m "feat(agent): phase-2 DTOs (wake event, approval, transcript) + agent_wake topic + config knobs"
```

---

## Task 2: Resumable loop + approval pause + forbid-list

**Goal:** Refactor the ReAct loop to run from a session's persisted transcript and to PAUSE (persist + return) when a tool call needs approval; enforce a non-overridable forbid-list.

**Files:**

- Modify: `daemon/services/agent/loop.go`
- Modify: `daemon/services/agent/service.go` (add config-less helpers used by loop; transcript conversion)
- Test: `daemon/services/agent/loop_test.go`

**Acceptance Criteria:**

- [ ] A fresh session seeds `Transcript` with the user goal, then runs; on a final text answer it completes (existing behavior preserved).
- [ ] When the model calls a tool whose tier maps to `ModeApprove`, the session ends with `Status == SessionAwaitingApproval`, a populated `PendingApproval` (ActionID, ToolName, Args, RiskTier), and the assistant turn (incl. the tool_use) saved in `Transcript`; the tool is NOT invoked.
- [ ] A tool whose name is in `cfg.ForbidList` is refused with a clear message and the loop continues (never paused, never executed) — even if its tier were `ModeAuto`.
- [ ] Read-only + low-risk (`ModeAuto`) tools still auto-execute and the loop continues.
- [ ] Iteration/token/deadline caps still bound the loop across the whole session.

**Verify:** `go test ./daemon/services/agent/ -run Loop -v` → PASS

**Steps:**

- [ ] **Step 1: Add tests** to `daemon/services/agent/loop_test.go`:

```go
func TestLoopPausesForApproval(t *testing.T) {
 p := llm.NewMockProvider(
  &llm.ChatResponse{ToolCalls: []llm.ToolCall{{ID: "tu1", Name: "stop_array", Args: "{}"}}, OutputTokens: 3},
 )
 cfg := dto.DefaultAgentConfig()
 cfg.Enabled = true
 reg := tools.NewRegistry()
 called := false
 reg.Register(tools.Tool{Name: "stop_array", RiskTier: dto.RiskHigh,
  Invoke: func(_ context.Context, _ string) (string, error) { called = true; return "stopped", nil }})
 svc := NewService(cfg, p, reg, NewStore(t.TempDir()), &capturingBroadcaster{})

 sess, err := svc.StartSession(context.Background(), "stop the array")
 if err != nil {
  t.Fatalf("start: %v", err)
 }
 if sess.Status != dto.SessionAwaitingApproval {
  t.Fatalf("status=%q want awaiting_approval", sess.Status)
 }
 if sess.PendingApproval == nil || sess.PendingApproval.ToolName != "stop_array" {
  t.Fatalf("pending approval missing: %+v", sess.PendingApproval)
 }
 if called {
  t.Fatal("high-risk tool must NOT execute before approval")
 }
 if len(sess.Transcript) == 0 {
  t.Fatal("transcript must be persisted for resume")
 }
}

func TestLoopForbidListRefusesEvenIfAuto(t *testing.T) {
 p := llm.NewMockProvider(
  &llm.ChatResponse{ToolCalls: []llm.ToolCall{{ID: "f1", Name: "format_disk", Args: "{}"}}, OutputTokens: 2},
  &llm.ChatResponse{Text: "I won't do that.", OutputTokens: 2},
 )
 cfg := dto.DefaultAgentConfig()
 cfg.Enabled = true
 cfg.Autonomy[dto.RiskHigh] = dto.ModeAuto // even if mis-set to auto...
 reg := tools.NewRegistry()
 called := false
 reg.Register(tools.Tool{Name: "format_disk", RiskTier: dto.RiskHigh,
  Invoke: func(_ context.Context, _ string) (string, error) { called = true; return "", nil }})
 svc := NewService(cfg, p, reg, NewStore(t.TempDir()), &capturingBroadcaster{})

 sess, _ := svc.StartSession(context.Background(), "format disk1")
 if called {
  t.Fatal("forbid-list tool must never execute")
 }
 if sess.Status != dto.SessionCompleted {
  t.Fatalf("status=%q want completed (loop continues after refusal)", sess.Status)
 }
}
```

(The existing Phase-1 loop tests — tool-then-answer, iteration cap, disabled, token budget, provider error — must continue to pass unchanged.)

- [ ] **Step 2: Run, confirm FAIL** — `go test ./daemon/services/agent/ -run Loop -v`.

- [ ] **Step 3: Rewrite `runLoop` in `daemon/services/agent/loop.go`** to be transcript-driven and resumable. Replace the existing `runLoop` and `executeCall` with:

```go
// transcriptToMessages converts the persisted transcript to llm messages.
func transcriptToMessages(t []dto.AgentMessage) []llm.Message {
 out := make([]llm.Message, 0, len(t))
 for _, m := range t {
  msg := llm.Message{Role: m.Role, Content: m.Content, ToolCallID: m.ToolCallID}
  for _, c := range m.ToolCalls {
   msg.ToolCalls = append(msg.ToolCalls, llm.ToolCall{ID: c.ID, Name: c.Name, Args: c.Args})
  }
  out = append(out, msg)
 }
 return out
}

func appendTranscript(sess *dto.AgentSession, m llm.Message) {
 rec := dto.AgentMessage{Role: m.Role, Content: m.Content, ToolCallID: m.ToolCallID}
 for _, c := range m.ToolCalls {
  rec.ToolCalls = append(rec.ToolCalls, dto.AgentMsgToolCall{ID: c.ID, Name: c.Name, Args: c.Args})
 }
 sess.Transcript = append(sess.Transcript, m2dto(m))
 _ = rec // (kept explicit for clarity; m2dto does the conversion)
}

func m2dto(m llm.Message) dto.AgentMessage {
 rec := dto.AgentMessage{Role: m.Role, Content: m.Content, ToolCallID: m.ToolCallID}
 for _, c := range m.ToolCalls {
  rec.ToolCalls = append(rec.ToolCalls, dto.AgentMsgToolCall{ID: c.ID, Name: c.Name, Args: c.Args})
 }
 return rec
}

// runLoop drives the bounded ReAct cycle from the session's current transcript.
// It returns when the model gives a final answer, a cap is hit, the provider
// errors, or a tool call requires approval (the session is left paused).
func (s *Service) runLoop(ctx context.Context, sess *dto.AgentSession) {
 defer func() {
  if r := recover(); r != nil {
   logger.Error("Agent: panic in session %s: %v", sess.ID, r)
   s.fail(sess, fmt.Sprintf("internal panic: %v", r))
  }
 }()

 deadline := time.Duration(s.cfg.SessionDeadlineSecs) * time.Second
 loopCtx := ctx
 if deadline > 0 {
  var cancel context.CancelFunc
  loopCtx, cancel = context.WithTimeout(ctx, deadline)
  defer cancel()
 }

 schemas := s.tools.Schemas()

 for len(sess.Steps) < s.cfg.MaxIterations {
  if s.cfg.MaxTokensPerSession > 0 && sess.TokensUsed >= s.cfg.MaxTokensPerSession {
   s.finish(sess, dto.SessionCompleted, "Stopped: token budget reached.")
   return
  }

  resp, err := s.provider.Chat(loopCtx, llm.ChatRequest{
   System:    systemPrompt,
   Messages:  transcriptToMessages(sess.Transcript),
   Tools:     schemas,
   MaxTokens: defaultLLMMaxOutputTokens,
  })
  if err != nil {
   if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
    s.fail(sess, fmt.Sprintf("session cancelled: %v", err))
   } else {
    s.fail(sess, fmt.Sprintf("provider error: %v", err))
   }
   return
  }
  sess.TokensUsed += resp.InputTokens + resp.OutputTokens

  step := dto.AgentStep{Index: len(sess.Steps), Thought: resp.Text, At: time.Now()}

  if len(resp.ToolCalls) == 0 {
   sess.Steps = append(sess.Steps, step)
   s.emit(sess, "step_completed", step)
   s.finish(sess, dto.SessionCompleted, resp.Text)
   return
  }

  // Persist the assistant turn (with tool_use) before acting.
  appendTranscript(sess, llm.Message{Role: "assistant", Content: resp.Text, ToolCalls: resp.ToolCalls})

  for _, call := range resp.ToolCalls {
   tool, ok := s.tools.Get(call.Name)
   tier := dto.RiskHigh
   if ok {
    tier = tool.RiskTier
   }

   // Forbid-list: never execute, never pause — refuse and continue.
   if s.isForbidden(call.Name) {
    rec := dto.AgentToolCall{Name: call.Name, Args: call.Args, RiskTier: tier,
     Error: "forbidden", Result: fmt.Sprintf("Action %q is on the forbidden list and will never be executed.", call.Name), At: time.Now()}
    step.ToolCalls = append(step.ToolCalls, rec)
    appendTranscript(sess, llm.Message{Role: "tool", ToolCallID: call.ID, Content: rec.Result})
    s.emit(sess, "tool_called", rec)
    continue
   }

   if !ok {
    rec := dto.AgentToolCall{Name: call.Name, Args: call.Args, Error: "unknown tool",
     Result: fmt.Sprintf("Error: tool %q does not exist.", call.Name), At: time.Now()}
    step.ToolCalls = append(step.ToolCalls, rec)
    appendTranscript(sess, llm.Message{Role: "tool", ToolCallID: call.ID, Content: rec.Result})
    s.emit(sess, "tool_called", rec)
    continue
   }

   mode := s.cfg.Autonomy[tier]
   if mode != dto.ModeAuto {
    // Pause for approval. Persist the step so far, set PendingApproval, return.
    sess.Steps = append(sess.Steps, step)
    sess.PendingApproval = &dto.ApprovalRequest{
     ActionID:    call.ID,
     ToolName:    call.Name,
     Args:        call.Args,
     RiskTier:    tier,
     Reason:      resp.Text,
     RequestedAt: time.Now(),
    }
    sess.Status = dto.SessionAwaitingApproval
    s.emit(sess, "approval_required", sess.PendingApproval)
    return
   }

   rec := s.invokeTool(loopCtx, tool, call)
   step.ToolCalls = append(step.ToolCalls, rec)
   appendTranscript(sess, llm.Message{Role: "tool", ToolCallID: call.ID, Content: rec.Result})
   s.emit(sess, "tool_called", rec)
  }
  sess.Steps = append(sess.Steps, step)
  s.emit(sess, "step_completed", step)
 }

 s.finish(sess, dto.SessionCompleted, "Stopped: reached maximum reasoning steps without a final answer.")
}

// invokeTool runs an auto-approved tool and returns the record.
func (s *Service) invokeTool(ctx context.Context, tool tools.Tool, call llm.ToolCall) dto.AgentToolCall {
 rec := dto.AgentToolCall{Name: call.Name, Args: call.Args, RiskTier: tool.RiskTier, At: time.Now()}
 out, err := tool.Invoke(ctx, call.Args)
 if err != nil {
  rec.Error = err.Error()
  rec.Result = "Error: " + err.Error()
  return rec
 }
 rec.Result = out
 return rec
}

// isForbidden reports whether a tool name is on the non-overridable forbid-list.
func (s *Service) isForbidden(name string) bool {
 for _, f := range s.cfg.ForbidList {
  if f == name {
   return true
  }
 }
 return false
}
```

Keep `systemPrompt`, `finish`, `fail`, `emit` as in Phase 1 but change their receiver param to `*dto.AgentSession` (they already take `*dto.AgentSession`). Remove the old `executeCall` (replaced by inline logic + `invokeTool`). Delete the now-unused `appendTranscript`'s `rec`/`_ = rec` lines if golangci complains — simplest: drop the unused `rec` and `_ = rec` and have `appendTranscript` call `m2dto` directly:

```go
func appendTranscript(sess *dto.AgentSession, m llm.Message) { sess.Transcript = append(sess.Transcript, m2dto(m)) }
```

- [ ] **Step 4: Update `StartSession` in `service.go`** to seed the transcript and call the new `runLoop(ctx, *sess)`:

```go
func (s *Service) StartSession(ctx context.Context, goal string) (dto.AgentSession, error) {
 if !s.Enabled() {
  return dto.AgentSession{}, errors.New("agent is disabled")
 }
 sess := dto.AgentSession{ID: s.nextID(), Goal: goal, Status: dto.SessionRunning, StartedAt: time.Now()}
 sess.Transcript = []dto.AgentMessage{{Role: "user", Content: goal}}
 s.emit(&sess, "session_started", nil)
 s.runLoop(ctx, &sess)
 s.store.Put(sess)
 if err := s.store.Save(); err != nil {
  logger.Warning("Agent: failed to persist session %s: %v", sess.ID, err)
 }
 return sess, nil
}
```

(`runLoop` no longer returns the session; it mutates `*sess`. Ensure `service.go` imports `errors`.)

- [ ] **Step 5: Run** `go test ./daemon/services/agent/ -run Loop -v` (PASS, incl. existing tests), gofmt/goimports/vet clean, `golangci-lint run ./daemon/services/agent/...` → 0 issues.

- [ ] **Step 6: Commit**

```bash
git add daemon/services/agent/loop.go daemon/services/agent/service.go daemon/services/agent/loop_test.go
git commit -m "feat(agent): resumable transcript-driven loop with approval pause + forbid-list"
```

---

## Task 3: ApproveAction / DenyAction / CancelSession (resume)

**Goal:** Resume a paused session: approve runs the pending tool (forbid-list re-checked) then continues the loop; deny records a denial and continues; cancel ends the session.

**Files:**

- Modify: `daemon/services/agent/service.go`
- Test: `daemon/services/agent/service_test.go`

**Acceptance Criteria:**

- [ ] `ApproveAction(ctx, sessionID, actionID, approve bool)` returns the updated session + error.
- [ ] Approving the pending action: executes the pending tool, appends its result to the transcript, clears `PendingApproval`, and continues the loop to completion.
- [ ] Denying: does NOT execute the tool, appends a "denied by operator" tool result, clears `PendingApproval`, continues the loop.
- [ ] Approving a forbidden tool still refuses execution (forbid-list beats approval).
- [ ] Mismatched `actionID` or a non-awaiting session returns an error and does not mutate state.
- [ ] `CancelSession(sessionID)` sets status `SessionCancelled` and clears any pending approval.

**Verify:** `go test ./daemon/services/agent/ -run Approve -v` → PASS

**Steps:**

- [ ] **Step 1: Add tests** to `daemon/services/agent/service_test.go`:

```go
func pausedSvc(t *testing.T, toolCalled *bool) *Service {
 t.Helper()
 p := llm.NewMockProvider(
  &llm.ChatResponse{ToolCalls: []llm.ToolCall{{ID: "tu1", Name: "stop_array", Args: "{}"}}, OutputTokens: 2},
  &llm.ChatResponse{Text: "Array stopped, all done.", OutputTokens: 2}, // returned after resume
 )
 cfg := dto.DefaultAgentConfig()
 cfg.Enabled = true
 reg := tools.NewRegistry()
 reg.Register(tools.Tool{Name: "stop_array", RiskTier: dto.RiskHigh,
  Invoke: func(_ context.Context, _ string) (string, error) { *toolCalled = true; return "stopped", nil }})
 return NewService(cfg, p, reg, NewStore(t.TempDir()), &capturingBroadcaster{})
}

func TestApproveExecutesAndCompletes(t *testing.T) {
 called := false
 svc := pausedSvc(t, &called)
 sess, _ := svc.StartSession(context.Background(), "stop array")
 if sess.Status != dto.SessionAwaitingApproval {
  t.Fatalf("precondition: want awaiting_approval, got %q", sess.Status)
 }
 out, err := svc.ApproveAction(context.Background(), sess.ID, sess.PendingApproval.ActionID, true)
 if err != nil {
  t.Fatalf("approve: %v", err)
 }
 if !called {
  t.Fatal("approved tool should have executed")
 }
 if out.Status != dto.SessionCompleted || out.PendingApproval != nil {
  t.Fatalf("want completed + cleared approval, got %q / %+v", out.Status, out.PendingApproval)
 }
}

func TestDenyDoesNotExecute(t *testing.T) {
 called := false
 svc := pausedSvc(t, &called)
 sess, _ := svc.StartSession(context.Background(), "stop array")
 out, err := svc.ApproveAction(context.Background(), sess.ID, sess.PendingApproval.ActionID, false)
 if err != nil {
  t.Fatalf("deny: %v", err)
 }
 if called {
  t.Fatal("denied tool must not execute")
 }
 if out.Status != dto.SessionCompleted {
  t.Fatalf("want completed after deny+continue, got %q", out.Status)
 }
}

func TestApproveWrongActionIDErrors(t *testing.T) {
 called := false
 svc := pausedSvc(t, &called)
 sess, _ := svc.StartSession(context.Background(), "stop array")
 if _, err := svc.ApproveAction(context.Background(), sess.ID, "wrong-id", true); err == nil {
  t.Fatal("expected error on action_id mismatch")
 }
}

func TestCancelSession(t *testing.T) {
 called := false
 svc := pausedSvc(t, &called)
 sess, _ := svc.StartSession(context.Background(), "stop array")
 out, err := svc.CancelSession(sess.ID)
 if err != nil || out.Status != dto.SessionCancelled {
  t.Fatalf("cancel: status=%q err=%v", out.Status, err)
 }
}
```

- [ ] **Step 2: Run, confirm FAIL.**

- [ ] **Step 3: Implement in `service.go`:**

```go
// ApproveAction resolves a pending approval and resumes the session loop.
func (s *Service) ApproveAction(ctx context.Context, sessionID, actionID string, approve bool) (dto.AgentSession, error) {
 sess, ok := s.store.Get(sessionID)
 if !ok {
  return dto.AgentSession{}, fmt.Errorf("session %q not found", sessionID)
 }
 if sess.Status != dto.SessionAwaitingApproval || sess.PendingApproval == nil {
  return dto.AgentSession{}, fmt.Errorf("session %q is not awaiting approval", sessionID)
 }
 if sess.PendingApproval.ActionID != actionID {
  return dto.AgentSession{}, fmt.Errorf("action_id %q does not match the pending approval", actionID)
 }

 pending := sess.PendingApproval
 sess.PendingApproval = nil
 sess.Status = dto.SessionRunning

 var result string
 switch {
 case !approve:
  result = "Action denied by operator."
 case s.isForbidden(pending.ToolName):
  result = fmt.Sprintf("Action %q is on the forbidden list and cannot be executed even with approval.", pending.ToolName)
 default:
  tool, found := s.tools.Get(pending.ToolName)
  if !found {
   result = fmt.Sprintf("Error: tool %q no longer exists.", pending.ToolName)
  } else {
   rec := s.invokeTool(ctx, tool, llm.ToolCall{ID: pending.ActionID, Name: pending.ToolName, Args: pending.Args})
   result = rec.Result
   s.emit(&sess, "tool_called", rec)
  }
 }
 // Feed the (executed/denied) tool result back, then continue reasoning.
 appendTranscript(&sess, llm.Message{Role: "tool", ToolCallID: pending.ActionID, Content: result})
 s.runLoop(ctx, &sess)

 s.store.Put(sess)
 if err := s.store.Save(); err != nil {
  logger.Warning("Agent: failed to persist session %s: %v", sess.ID, err)
 }
 return sess, nil
}

// CancelSession marks a session cancelled and clears any pending approval.
func (s *Service) CancelSession(sessionID string) (dto.AgentSession, error) {
 sess, ok := s.store.Get(sessionID)
 if !ok {
  return dto.AgentSession{}, fmt.Errorf("session %q not found", sessionID)
 }
 now := time.Now()
 sess.Status = dto.SessionCancelled
 sess.PendingApproval = nil
 sess.EndedAt = &now
 s.emit(&sess, "session_cancelled", nil)
 s.store.Put(sess)
 if err := s.store.Save(); err != nil {
  logger.Warning("Agent: failed to persist session %s: %v", sess.ID, err)
 }
 return sess, nil
}
```

- [ ] **Step 4: Run** `go test ./daemon/services/agent/ -run 'Approve|Deny|Cancel' -v` (PASS) + full agent package, lint clean.

- [ ] **Step 5: Commit**

```bash
git add daemon/services/agent/service.go daemon/services/agent/service_test.go
git commit -m "feat(agent): approve/deny/cancel resume paused sessions"
```

---

## Task 4: Approval TTL sweeper (default-deny)

**Goal:** Expire sessions stuck in `awaiting_approval` past `ApprovalTTLSecs` by auto-denying (resume with a "approval timed out" result), so abandoned approvals don't linger.

**Files:**

- Modify: `daemon/services/agent/service.go` (add `SweepExpiredApprovals(ctx, now) int`)
- Test: `daemon/services/agent/service_test.go`

**Acceptance Criteria:**

- [ ] `SweepExpiredApprovals(ctx, now time.Time) int` auto-denies every `awaiting_approval` session whose `PendingApproval.RequestedAt` is older than `ApprovalTTLSecs`, returns the count swept.
- [ ] A swept session continues the loop with a timeout-denial result (does NOT execute the tool) and no longer has `PendingApproval`.
- [ ] Sessions within TTL are untouched.

**Verify:** `go test ./daemon/services/agent/ -run Sweep -v` → PASS

**Steps:**

- [ ] **Step 1: Add test:**

```go
func TestSweepExpiredApprovals(t *testing.T) {
 called := false
 svc := pausedSvc(t, &called)
 svc.cfg.ApprovalTTLSecs = 60
 sess, _ := svc.StartSession(context.Background(), "stop array")
 // Backdate the pending approval beyond TTL.
 sess.PendingApproval.RequestedAt = time.Now().Add(-2 * time.Hour)
 svc.store.Put(sess)

 n := svc.SweepExpiredApprovals(context.Background(), time.Now())
 if n != 1 {
  t.Fatalf("expected 1 swept, got %d", n)
 }
 if called {
  t.Fatal("expired approval must NOT execute the tool")
 }
 out, _ := svc.GetSession(sess.ID)
 if out.PendingApproval != nil || out.Status == dto.SessionAwaitingApproval {
  t.Fatalf("expired session should no longer await approval: %+v", out)
 }
}
```

- [ ] **Step 2: Run, confirm FAIL.**

- [ ] **Step 3: Implement in `service.go`:**

```go
// SweepExpiredApprovals auto-denies awaiting-approval sessions older than the TTL.
// Returns the number of sessions swept.
func (s *Service) SweepExpiredApprovals(ctx context.Context, now time.Time) int {
 if s.cfg.ApprovalTTLSecs <= 0 {
  return 0
 }
 ttl := time.Duration(s.cfg.ApprovalTTLSecs) * time.Second
 swept := 0
 for _, sess := range s.store.List() {
  if sess.Status != dto.SessionAwaitingApproval || sess.PendingApproval == nil {
   continue
  }
  if now.Sub(sess.PendingApproval.RequestedAt) < ttl {
   continue
  }
  logger.Warning("Agent: approval for session %s timed out after %s; auto-denying", sess.ID, ttl)
  if _, err := s.ApproveAction(ctx, sess.ID, sess.PendingApproval.ActionID, false); err != nil {
   logger.Error("Agent: failed to auto-deny session %s: %v", sess.ID, err)
   continue
  }
  swept++
 }
 return swept
}
```

- [ ] **Step 4: Run** `go test ./daemon/services/agent/ -run Sweep -v` (PASS), lint clean.

- [ ] **Step 5: Commit**

```bash
git add daemon/services/agent/service.go daemon/services/agent/service_test.go
git commit -m "feat(agent): approval TTL sweeper (default-deny stale approvals)"
```

---

## Task 5: Trigger — agent_wake subscription, debounce/dedup/cooldown/concurrency, Start loop

**Goal:** Add `trigger.go` with `Service.Start(ctx)` that subscribes to `agent_wake`, dedups/debounces wakes by subsystem, respects cooldown + max-concurrent, spawns autonomous sessions, and periodically sweeps expired approvals.

**Files:**

- Create: `daemon/services/agent/trigger.go`
- Modify: `daemon/services/agent/service.go` (add `hub *domain.EventBus` field + setter; concurrency tracking; `startAutonomousSession`)
- Test: `daemon/services/agent/trigger_test.go`

**Acceptance Criteria:**

- [ ] `Service.SetEventBus(*domain.EventBus)` stores the hub; `Start(ctx)` returns immediately if disabled or no hub.
- [ ] `handleWake(ev)` dedups: two wakes with the same `Subsystem` within `WakeDebounceSecs` spawn only ONE session.
- [ ] A subsystem in cooldown (`WakeCooldownSecs` since last wake) is skipped.
- [ ] No new autonomous session is spawned when `MaxConcurrentSessions` autonomous sessions are already running.
- [ ] A spawned autonomous session derives its goal from the wake event and runs the loop (verified with a mock provider; may end completed or awaiting_approval).

**Verify:** `go test ./daemon/services/agent/ -run Trigger -v` → PASS

**Steps:**

- [ ] **Step 1: Add tests** to `daemon/services/agent/trigger_test.go` — drive `handleWake` directly (no real bus) for determinism:

```go
package agent

import (
 "context"
 "testing"
 "time"

 "github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
 "github.com/ruaan-deysel/unraid-management-agent/daemon/services/agent/llm"
 "github.com/ruaan-deysel/unraid-management-agent/daemon/services/agent/tools"
)

func wakeSvc(t *testing.T) *Service {
 t.Helper()
 // Provider always answers immediately so autonomous sessions complete fast.
 p := llm.NewMockProvider()
 cfg := dto.DefaultAgentConfig()
 cfg.Enabled = true
 reg := tools.BuildDefault(fakeState{}, fakeDocker{})
 return NewService(cfg, p, reg, NewStore(t.TempDir()), &capturingBroadcaster{})
}

func TestHandleWakeDedupsBySubsystem(t *testing.T) {
 svc := wakeSvc(t)
 ev := dto.AgentWakeEvent{Source: "alert", Subsystem: "disk", Title: "disk hot", At: time.Now()}
 spawned1 := svc.handleWake(context.Background(), ev)
 spawned2 := svc.handleWake(context.Background(), ev) // within debounce window
 if !spawned1 {
  t.Fatal("first wake should spawn")
 }
 if spawned2 {
  t.Fatal("duplicate wake within debounce window should be skipped")
 }
}

func TestHandleWakeRespectsConcurrencyCap(t *testing.T) {
 svc := wakeSvc(t)
 svc.cfg.MaxConcurrentSessions = 0 // no capacity
 if svc.handleWake(context.Background(), dto.AgentWakeEvent{Subsystem: "x", At: time.Now()}) {
  t.Fatal("should not spawn when concurrency cap is 0")
 }
}
```

(`handleWake` runs the session synchronously in the test path and returns whether it spawned — see implementation note.)

- [ ] **Step 2: Run, confirm FAIL.**

- [ ] **Step 3: Add hub/concurrency fields to `service.go`** (in the `Service` struct and `NewService` leave them zero):

```go
 hub        *domain.EventBus
 wakeMu     sync.Mutex
 lastWake   map[string]time.Time // subsystem -> last wake time
 activeAuto int                  // currently running autonomous sessions
```

Initialize `lastWake` in `NewService`: `s.lastWake = map[string]time.Time{}` (add to the constructor before returning). Add import `"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"`.

```go
// SetEventBus wires the pubsub hub so the agent can receive wake events.
func (s *Service) SetEventBus(hub *domain.EventBus) { s.hub = hub }

// startAutonomousSession runs an investigation triggered by a wake event.
// Synchronous; callers decide whether to run it in a goroutine.
func (s *Service) startAutonomousSession(ctx context.Context, ev dto.AgentWakeEvent) {
 goal := fmt.Sprintf("An incident was detected (source=%s, severity=%s): %s. %s\n"+
  "Investigate using read-only tools and, within policy, remediate it.",
  ev.Source, ev.Severity, ev.Title, ev.Detail)
 sess := dto.AgentSession{ID: s.nextID(), Goal: goal, Status: dto.SessionRunning, StartedAt: time.Now()}
 sess.Transcript = []dto.AgentMessage{{Role: "user", Content: goal}}
 s.emit(&sess, "session_started", nil)
 s.runLoop(ctx, &sess)
 s.store.Put(sess)
 if err := s.store.Save(); err != nil {
  logger.Warning("Agent: failed to persist autonomous session %s: %v", sess.ID, err)
 }
 s.wakeMu.Lock()
 s.activeAuto--
 s.wakeMu.Unlock()
}
```

- [ ] **Step 4: Create `daemon/services/agent/trigger.go`:**

```go
package agent

import (
 "context"
 "time"

 "github.com/ruaan-deysel/unraid-management-agent/daemon/constants"
 "github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
 "github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
)

// sweepInterval is how often Start checks for expired approvals.
const sweepInterval = 30 * time.Second

// Start subscribes to agent_wake events and runs until ctx is cancelled.
// It is a no-op when the agent is disabled or no event bus is wired.
func (s *Service) Start(ctx context.Context) {
 if !s.Enabled() || s.hub == nil {
  logger.Info("Agent: autonomous triggers not started (disabled or no event bus)")
  return
 }
 ch := s.hub.SubTopics(constants.TopicAgentWake)
 defer s.hub.Unsub(ch, constants.TopicAgentWake.Name)

 ticker := time.NewTicker(sweepInterval)
 defer ticker.Stop()
 logger.Success("Agent: autonomous trigger listening on %q", constants.TopicAgentWake.Name)

 for {
  select {
  case <-ctx.Done():
   logger.Info("Agent: autonomous trigger stopped")
   return
  case <-ticker.C:
   func() {
    defer func() {
     if r := recover(); r != nil {
      logger.LogPanicWithStack("Agent sweeper", r)
     }
    }()
    s.SweepExpiredApprovals(ctx, time.Now())
   }()
  case msg := <-ch:
   ev, ok := msg.(dto.AgentWakeEvent)
   if !ok {
    continue
   }
   func() {
    defer func() {
     if r := recover(); r != nil {
      logger.LogPanicWithStack("Agent wake handler", r)
     }
    }()
    s.handleWake(ctx, ev)
   }()
  }
 }
}

// handleWake applies dedup/debounce/cooldown/concurrency policy and, if admitted,
// runs an autonomous session. Returns true if a session was spawned.
// In production the spawned session runs in a goroutine; the return value reflects
// whether admission succeeded (used by tests, which run it synchronously).
func (s *Service) handleWake(ctx context.Context, ev dto.AgentWakeEvent) bool {
 now := time.Now()
 debounce := time.Duration(s.cfg.WakeDebounceSecs) * time.Second
 cooldown := time.Duration(s.cfg.WakeCooldownSecs) * time.Second

 s.wakeMu.Lock()
 last, seen := s.lastWake[ev.Subsystem]
 if seen && now.Sub(last) < debounce {
  s.wakeMu.Unlock()
  logger.Debug("Agent: wake for %q debounced", ev.Subsystem)
  return false
 }
 if seen && now.Sub(last) < cooldown {
  s.wakeMu.Unlock()
  logger.Debug("Agent: wake for %q in cooldown", ev.Subsystem)
  return false
 }
 if s.activeAuto >= s.cfg.MaxConcurrentSessions {
  s.wakeMu.Unlock()
  logger.Warning("Agent: wake for %q dropped — %d autonomous sessions already running (cap=%d)",
   ev.Subsystem, s.activeAuto, s.cfg.MaxConcurrentSessions)
  return false
 }
 s.lastWake[ev.Subsystem] = now
 s.activeAuto++
 s.wakeMu.Unlock()

 logger.Info("Agent: waking on %s incident (%s)", ev.Subsystem, ev.Title)
 s.startAutonomousSession(ctx, ev)
 return true
}
```

> Implementation note for the engineer: `handleWake` calls `startAutonomousSession` synchronously here so the unit tests are deterministic. In production this is fine because `Start` processes wakes one at a time off the channel and the concurrency cap bounds work; if you later want true concurrency, wrap the `startAutonomousSession` call in `go func(){...}()`. Keep it synchronous for this task (the cap + debounce already bound load, and the mock-provider tests rely on synchronous completion). `activeAuto` is decremented inside `startAutonomousSession`.

- [ ] **Step 5: Run** `go test ./daemon/services/agent/ -run Trigger -v` (PASS) + full agent package + lint clean.

- [ ] **Step 6: Commit**

```bash
git add daemon/services/agent/trigger.go daemon/services/agent/service.go daemon/services/agent/trigger_test.go
git commit -m "feat(agent): event-driven wake trigger with debounce/dedup/cooldown/concurrency + approval sweeper"
```

---

## Task 6: Publish agent_wake from alerting + watchdog; orchestrator wiring

**Goal:** Make the alerting Engine and watchdog Runner publish `AgentWakeEvent` on incident transitions, and wire the agent's `Start(ctx)` + event bus in the orchestrator (subscribing before collectors start).

**Files:**

- Modify: `daemon/services/alerting/engine.go`
- Modify: `daemon/services/watchdog/runner.go`
- Modify: `daemon/services/agent/bootstrap.go`
- Modify: `daemon/services/orchestrator.go`
- Test: `daemon/services/alerting/engine_test.go`, `daemon/services/watchdog/runner_test.go`

**Acceptance Criteria:**

- [ ] `alerting.Engine.SetEventBus(*domain.EventBus)` exists; on a firing alert, the engine publishes a `dto.AgentWakeEvent{Source:"alert", Subsystem:<rule>, Severity, Title, Detail}` to `TopicAgentWake` (verified by subscribing a test channel).
- [ ] `watchdog.Runner.SetEventBus(*domain.EventBus)` exists; on a check transitioning to unhealthy, it publishes `dto.AgentWakeEvent{Source:"watchdog", Subsystem:<check name>, Severity:"warning", ...}`.
- [ ] Publishing is a no-op when no bus is set (existing tests with `NewEngine`/`NewRunner` still pass).
- [ ] Orchestrator: when the agent is enabled, calls `alertEngine.SetEventBus(o.ctx.Hub)`, `watchdogRunner.SetEventBus(o.ctx.Hub)`, `agentSvc.SetEventBus(o.ctx.Hub)`, `mcpServer.SetAgent(agentSvc)`, and launches `agentSvc.Start(ctx)` in a recovered goroutine **before** collectors start.
- [ ] `go build ./...` succeeds; full suite passes.

**Verify:** `go test ./daemon/services/alerting/ ./daemon/services/watchdog/ -run EventBus -v && go build ./...` → PASS

**Steps:**

- [ ] **Step 1: Tests.** In `engine_test.go`:

```go
func TestEngineSetEventBusPublishesWake(t *testing.T) {
 bus := domain.NewEventBus(8)
 ch := bus.SubTopics(constants.TopicAgentWake)
 e := NewEngine(NewStore(t.TempDir()), nil) // provider nil ok; we call publish directly
 e.SetEventBus(bus)
 e.publishWake(dto.AlertEvent{RuleName: "High CPU", Severity: "warning", Message: "cpu 95%", State: "firing"})
 select {
 case msg := <-ch:
  ev := msg.(dto.AgentWakeEvent)
  if ev.Source != "alert" || ev.Subsystem != "High CPU" {
   t.Fatalf("unexpected wake: %+v", ev)
  }
 case <-time.After(time.Second):
  t.Fatal("no wake published")
 }
}
```

In `runner_test.go`:

```go
func TestRunnerSetEventBusPublishesWake(t *testing.T) {
 bus := domain.NewEventBus(8)
 ch := bus.SubTopics(constants.TopicAgentWake)
 r := NewRunner(NewStore(t.TempDir()))
 r.SetEventBus(bus)
 r.publishWake(dto.HealthCheck{Name: "Plex HTTP"}, ProbeResult{Healthy: false, Error: "timeout"})
 select {
 case msg := <-ch:
  ev := msg.(dto.AgentWakeEvent)
  if ev.Source != "watchdog" || ev.Subsystem != "Plex HTTP" {
   t.Fatalf("unexpected wake: %+v", ev)
  }
 case <-time.After(time.Second):
  t.Fatal("no wake published")
 }
}
```

(Add imports for `domain`, `constants`, `dto`, `time` as needed.)

- [ ] **Step 2: Run, confirm FAIL.**

- [ ] **Step 3: Edit `alerting/engine.go`.** Add field `hub *domain.EventBus` to `Engine`; add imports `domain`, `constants`. Add:

```go
// SetEventBus wires the pubsub hub so firing alerts can wake the agent.
func (e *Engine) SetEventBus(hub *domain.EventBus) { e.hub = hub }

// publishWake emits an AgentWakeEvent for a firing alert (no-op if no hub).
func (e *Engine) publishWake(event dto.AlertEvent) {
 if e.hub == nil || event.State != "firing" {
  return
 }
 domain.Publish(e.hub, constants.TopicAgentWake, dto.AgentWakeEvent{
  Source:    "alert",
  Subsystem: event.RuleName,
  Severity:  event.Severity,
  Title:     event.RuleName,
  Detail:    event.Message,
  At:        time.Now(),
 })
}
```

In `evaluate()`, right after `e.dispatcher.Dispatch(result.Rule, event)`, add: `e.publishWake(event)`.

- [ ] **Step 4: Edit `watchdog/runner.go`.** Add field `hub *domain.EventBus` to `Runner`; add imports `domain`, `constants`. Add:

```go
// SetEventBus wires the pubsub hub so unhealthy transitions can wake the agent.
func (r *Runner) SetEventBus(hub *domain.EventBus) { r.hub = hub }

// publishWake emits an AgentWakeEvent for an unhealthy check (no-op if no hub).
func (r *Runner) publishWake(check dto.HealthCheck, result ProbeResult) {
 if r.hub == nil {
  return
 }
 domain.Publish(r.hub, constants.TopicAgentWake, dto.AgentWakeEvent{
  Source:    "watchdog",
  Subsystem: check.Name,
  Severity:  "warning",
  Title:     "Health check failed: " + check.Name,
  Detail:    result.Error,
  At:        time.Now(),
 })
}
```

In `runCheck`, inside the `if transitionedToUnhealthy {` block (after the existing logging/remediation), add: `r.publishWake(check, result)`.

- [ ] **Step 5: Edit `agent/bootstrap.go`** — `BuildService` already returns the `*Service`; the orchestrator wires the bus separately via `SetEventBus`, so no signature change is needed here. (If you prefer, add `hub` as a param; the plan wires it in the orchestrator to avoid touching the bootstrap tests — do NOT change `BuildService`'s signature.)

- [ ] **Step 6: Edit `orchestrator.go`** — in the HTTP-mode agent block (`if agentCfg.Enabled { ... }`), after `apiServer.SetAgent(agentSvc)` add:

```go
   agentSvc.SetEventBus(o.ctx.Hub)
   alertEngine.SetEventBus(o.ctx.Hub)
   watchdogRunner.SetEventBus(o.ctx.Hub)
   mcpServer.SetAgent(agentSvc)
   wg.Go(func() {
    defer func() {
     if r := recover(); r != nil {
      logger.LogPanicWithStack("Agent trigger goroutine", r)
     }
    }()
    agentSvc.Start(ctx)
   })
```

This block runs after the watchdog/alerting are constructed and BEFORE `o.collectorManager.StartAll()` (verify ordering — the agent block already sits before collectors in Phase 1). `mcpServer` is in scope (constructed earlier in `Run`). Use the same `wg`/recovery pattern as the other goroutines.

- [ ] **Step 7: Run** `go test ./daemon/services/alerting/ ./daemon/services/watchdog/ -run EventBus -v` (PASS), `go build ./...`, `go test ./daemon/services/...` (full), lint clean.

- [ ] **Step 8: Commit**

```bash
git add daemon/services/alerting/engine.go daemon/services/watchdog/runner.go daemon/services/orchestrator.go daemon/services/alerting/engine_test.go daemon/services/watchdog/runner_test.go
git commit -m "feat(agent): alerting+watchdog publish agent_wake; orchestrator starts autonomous trigger"
```

---

## Task 7: REST approve/cancel endpoints

**Goal:** Expose `POST /api/v1/agent/sessions/{id}/approve` and `/cancel` following the Phase-1 handler pattern.

**Files:**

- Modify: `daemon/services/api/handlers_agent.go`
- Modify: `daemon/services/api/server.go` (routes)
- Test: `daemon/services/api/handlers_agent_test.go`

**Acceptance Criteria:**

- [ ] `POST /api/v1/agent/sessions/{id}/approve` with `{"action_id":"...","approve":true|false}` resumes the session and returns the updated session JSON; 503 when agent disabled; 400 on missing/invalid body; 404/409-style error surfaced from the service as 400 with message when not awaiting/ID mismatch.
- [ ] `POST /api/v1/agent/sessions/{id}/cancel` cancels and returns the session.
- [ ] Existing agent endpoint tests still pass.

**Verify:** `go test ./daemon/services/api/ -run Agent -v` → PASS

**Steps:**

- [ ] **Step 1: Add tests** to `handlers_agent_test.go` (reuse the Phase-1 `newAgentServer` helper, but its mock must script a tool-call-then-answer so a session can reach `awaiting_approval`). Add a helper that builds a server whose agent pauses, then approve via HTTP:

```go
func TestAgentApproveEndpoint(t *testing.T) {
 s := NewServer(&domain.Context{Hub: domain.NewEventBus(10)})
 cfg := dto.DefaultAgentConfig()
 cfg.Enabled = true
 p := llm.NewMockProvider(
  &llm.ChatResponse{ToolCalls: []llm.ToolCall{{ID: "tu1", Name: "stop_array", Args: "{}"}}, OutputTokens: 2},
  &llm.ChatResponse{Text: "done", OutputTokens: 1},
 )
 reg := tools.NewRegistry()
 reg.Register(tools.Tool{Name: "stop_array", RiskTier: dto.RiskHigh,
  Invoke: func(_ context.Context, _ string) (string, error) { return "stopped", nil }})
 svc := agent.NewService(cfg, p, reg, agent.NewStore(t.TempDir()), s)
 s.SetAgent(svc)

 // Start a session that pauses for approval.
 start := httptest.NewRequest(http.MethodPost, "/api/v1/agent/sessions", strings.NewReader(`{"goal":"stop array"}`))
 sr := httptest.NewRecorder()
 s.GetRouter().ServeHTTP(sr, start)
 var started dto.AgentSession
 _ = json.Unmarshal(sr.Body.Bytes(), &started)
 if started.Status != dto.SessionAwaitingApproval {
  t.Fatalf("expected awaiting_approval, got %q", started.Status)
 }

 body := `{"action_id":"` + started.PendingApproval.ActionID + `","approve":true}`
 ar := httptest.NewRequest(http.MethodPost, "/api/v1/agent/sessions/"+started.ID+"/approve", strings.NewReader(body))
 rr := httptest.NewRecorder()
 s.GetRouter().ServeHTTP(rr, ar)
 if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"completed"`) {
  t.Fatalf("approve failed: code=%d body=%s", rr.Code, rr.Body.String())
 }
}
```

(Add imports: `encoding/json`, `github.com/.../daemon/domain`.)

- [ ] **Step 2: Run, confirm FAIL.**

- [ ] **Step 3: Add handlers** to `handlers_agent.go`:

```go
// handleAgentApprove resolves a pending approval and resumes the session.
func (s *Server) handleAgentApprove(w http.ResponseWriter, r *http.Request) {
 if s.agentSvc == nil || !s.agentSvc.Enabled() {
  respondJSON(w, http.StatusServiceUnavailable, dto.Response{Success: false, Message: "agent is disabled", Timestamp: time.Now()})
  return
 }
 id := mux.Vars(r)["id"]
 var body struct {
  ActionID string `json:"action_id"`
  Approve  bool   `json:"approve"`
 }
 if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ActionID == "" {
  respondWithError(w, http.StatusBadRequest, "request body must include 'action_id' and 'approve'")
  return
 }
 sess, err := s.agentSvc.ApproveAction(r.Context(), id, body.ActionID, body.Approve)
 if err != nil {
  respondWithError(w, http.StatusBadRequest, err.Error())
  return
 }
 respondJSON(w, http.StatusOK, sess)
}

// handleAgentCancel cancels a session.
func (s *Server) handleAgentCancel(w http.ResponseWriter, r *http.Request) {
 if s.agentSvc == nil || !s.agentSvc.Enabled() {
  respondJSON(w, http.StatusServiceUnavailable, dto.Response{Success: false, Message: "agent is disabled", Timestamp: time.Now()})
  return
 }
 sess, err := s.agentSvc.CancelSession(mux.Vars(r)["id"])
 if err != nil {
  respondWithError(w, http.StatusBadRequest, err.Error())
  return
 }
 respondJSON(w, http.StatusOK, sess)
}
```

- [ ] **Step 4: Add routes** in `server.go` `setupRoutes()` next to the Phase-1 agent routes:

```go
 api.HandleFunc("/agent/sessions/{id}/approve", s.handleAgentApprove).Methods("POST")
 api.HandleFunc("/agent/sessions/{id}/cancel", s.handleAgentCancel).Methods("POST")
```

- [ ] **Step 5: Run** `go test ./daemon/services/api/ -run Agent -v` (PASS) + whole api package, lint clean.

- [ ] **Step 6: Commit**

```bash
git add daemon/services/api/handlers_agent.go daemon/services/api/server.go daemon/services/api/handlers_agent_test.go
git commit -m "feat(agent): REST approve/cancel endpoints"
```

---

## Task 8: MCP agent\_\* tools

**Goal:** Expose the agent over MCP so external AI clients (and the Unraid UI) can start/inspect/drive autonomous sessions and approvals.

**Files:**

- Modify: `daemon/services/mcp/server.go`
- Test: `daemon/services/mcp/server_test.go`

**Acceptance Criteria:**

- [ ] `mcp.Server` gains `agentSvc *agent.Service` + `SetAgent(*agent.Service)`; `Initialize()` calls `registerAgentTools()`.
- [ ] Tools registered: `agent_start_session` (arg `goal`), `agent_get_session` (arg `session_id`), `agent_list_sessions`, `agent_approve_action` (args `session_id`,`action_id`,`approve`).
- [ ] Each tool returns a clear text/JSON result and a graceful message when the agent is unset/disabled (no panic).
- [ ] `go build ./...` + MCP package tests pass.

**Verify:** `go test ./daemon/services/mcp/ -run Agent -v && go build ./...` → PASS

**Steps:**

- [ ] **Step 1: Add a test** to `daemon/services/mcp/server_test.go` that constructs the MCP server, calls `SetAgent` with a mock-backed `agent.Service`, runs `Initialize()`, and asserts the agent tools are registered and `agent_start_session` returns a completed/awaiting session. Follow the existing MCP test pattern in that file for how tools are invoked (mirror an existing `register*Tools` test). Minimal assertion if direct tool invocation is awkward: assert `Initialize()` succeeds with `SetAgent` set and that listing tools includes `agent_start_session` (use whatever list/exec helper the existing tests use).

- [ ] **Step 2: Run, confirm FAIL.**

- [ ] **Step 3: Add to `server.go`** — field `agentSvc *agent.Service` in the `Server` struct; import `"github.com/ruaan-deysel/unraid-management-agent/daemon/services/agent"`; setter:

```go
// SetAgent wires the agent service for MCP agent tools.
func (s *Server) SetAgent(svc *agent.Service) { s.agentSvc = svc }
```

Call `s.registerAgentTools()` in `Initialize()` (after `registerWatchdogTools()`). Implement:

```go
// registerAgentTools registers tools that drive the embedded autonomous agent.
func (s *Server) registerAgentTools() {
 type startArgs struct {
  Goal string `json:"goal" jsonschema:"the goal or question for the agent"`
 }
 mcp.AddTool(s.mcpServer, &mcp.Tool{
  Name:        "agent_start_session",
  Description: "Start an autonomous agent session to investigate/remediate a goal. Returns the session (may be 'awaiting_approval' if a high-risk action is proposed).",
 }, func(ctx context.Context, _ *mcp.CallToolRequest, args startArgs) (*mcp.CallToolResult, any, error) {
  if s.agentSvc == nil || !s.agentSvc.Enabled() {
   return textResult("Agent is disabled."), nil, nil
  }
  if args.Goal == "" {
   return textResult("Error: 'goal' is required."), nil, nil
  }
  sess, err := s.agentSvc.StartSession(ctx, args.Goal)
  if err != nil {
   return textResult("Error: " + err.Error()), nil, nil
  }
  return jsonResult(sess)
 })

 type idArgs struct {
  SessionID string `json:"session_id" jsonschema:"the session id"`
 }
 mcp.AddTool(s.mcpServer, &mcp.Tool{
  Name:        "agent_get_session",
  Description: "Get a single agent session (status, steps, pending approval, answer).",
  Annotations: &mcp.ToolAnnotations{ReadOnlyHint: ptr(true)},
 }, func(_ context.Context, _ *mcp.CallToolRequest, args idArgs) (*mcp.CallToolResult, any, error) {
  if s.agentSvc == nil {
   return textResult("Agent is disabled."), nil, nil
  }
  sess, ok := s.agentSvc.GetSession(args.SessionID)
  if !ok {
   return textResult(fmt.Sprintf("Session %q not found.", args.SessionID)), nil, nil
  }
  return jsonResult(sess)
 })

 mcp.AddTool(s.mcpServer, &mcp.Tool{
  Name:        "agent_list_sessions",
  Description: "List all agent sessions, newest first.",
  Annotations: &mcp.ToolAnnotations{ReadOnlyHint: ptr(true)},
 }, func(_ context.Context, _ *mcp.CallToolRequest, _ dto.MCPEmptyArgs) (*mcp.CallToolResult, any, error) {
  if s.agentSvc == nil {
   return textResult("Agent is disabled."), nil, nil
  }
  return jsonResult(s.agentSvc.ListSessions())
 })

 type approveArgs struct {
  SessionID string `json:"session_id" jsonschema:"the session id"`
  ActionID  string `json:"action_id" jsonschema:"the pending approval action id"`
  Approve   bool   `json:"approve" jsonschema:"true to approve, false to deny"`
 }
 mcp.AddTool(s.mcpServer, &mcp.Tool{
  Name:        "agent_approve_action",
  Description: "Approve or deny a high-risk action a session is awaiting, then resume it.",
 }, func(ctx context.Context, _ *mcp.CallToolRequest, args approveArgs) (*mcp.CallToolResult, any, error) {
  if s.agentSvc == nil || !s.agentSvc.Enabled() {
   return textResult("Agent is disabled."), nil, nil
  }
  sess, err := s.agentSvc.ApproveAction(ctx, args.SessionID, args.ActionID, args.Approve)
  if err != nil {
   return textResult("Error: " + err.Error()), nil, nil
  }
  return jsonResult(sess)
 })
}
```

> Confirm `dto.MCPEmptyArgs` exists (used by existing read-only MCP tools) and the `ptr(...)`/`textResult`/`jsonResult` helpers are in the mcp package (they are, per Phase-1 inspection). Match the exact `jsonschema` tag style used by neighboring tools in `server.go`; if the SDK version uses a different arg-schema mechanism, mirror an existing tool that takes arguments (e.g. a control tool) verbatim.

- [ ] **Step 4: Run** `go test ./daemon/services/mcp/ -run Agent -v` (PASS) + `go build ./...`, lint clean.

- [ ] **Step 5: Commit**

```bash
git add daemon/services/mcp/server.go daemon/services/mcp/server_test.go
git commit -m "feat(agent): MCP agent_* tools (start/get/list/approve)"
```

---

## Task 9: Docs, CHANGELOG, CodeRabbit review, and on-Unraid verification

**Goal:** **USER-ORDERED GATE — NON-SKIPPABLE.** This task was requested by the user in the current conversation. It MUST NOT be closed by walking around it, by declaring it "verified inline", or by substituting a cheaper check. Close only after every item in `acceptanceCriteria` has been re-validated independently, with output captured.

Document Phase 2, run the CodeRabbit CLI review and address findings, then build/test and deploy to the Unraid server and verify the autonomy + approval surfaces work and the plugin is stable.

**Files:**

- Modify: `CHANGELOG.md`, `docs/integrations/agent.md`
- Modify: `daemon/services/mcp/server.go` docs if needed; regenerate Swagger (`make swagger`) for the new REST endpoints

**Acceptance Criteria:**

- [ ] `CHANGELOG.md` has a Phase-2 entry under **Added** (agent_wake triggers, approval gate + pause/resume, forbid-list, TTL sweeper, REST approve/cancel, MCP agent tools).
- [ ] `docs/integrations/agent.md` documents: the autonomy trigger model + debounce/cooldown/concurrency config, the approval workflow (REST `/approve` + `/cancel`, MCP `agent_approve_action`), the forbid-list, and the new config fields.
- [ ] `make pre-commit-run` passes (lint + security), zero errors.
- [ ] `make test` passes (full suite, race detector).
- [ ] CodeRabbit CLI review has been run on the branch diff and all actionable findings are fixed or explicitly noted out-of-scope with reasoning.
- [ ] Plugin builds + deploys to Unraid via Ansible (`build,deploy,verify`); daemon starts cleanly (no panics in `/var/log/unraid-management-agent.log`); existing verify suite passes (no regression).
- [ ] On Unraid with the agent **disabled** (default): no `agent_wake` subscription is active and all existing endpoints behave as before.
- [ ] On Unraid with the agent **enabled** + a real API key: a watchdog/alert incident wakes an autonomous session (visible via `GET /api/v1/agent/sessions`), and a high-risk action produces an `awaiting_approval` session that `POST /agent/sessions/{id}/approve` resumes to completion.

**Verify:**

- `make pre-commit-run && make test` → all pass
- `coderabbit review --agent --base-commit <phase2-base>` → findings addressed
- `ansible-playbook -i ansible/inventory.yml ansible/deploy.yml --tags build,deploy,verify` → deploy + verification succeed
- Manual on Unraid (enabled): trigger a failing health check (or alert), then `curl -s http://<unraid-ip>:8043/api/v1/agent/sessions | jq '.[0]|{status,goal}'` shows an autonomous session; for an awaiting-approval session, `curl -s -X POST http://<unraid-ip>:8043/api/v1/agent/sessions/<id>/approve -d '{"action_id":"<id>","approve":true}' | jq '{status}'` → `completed`.

**Steps:**

- [ ] **Step 1** — Update `CHANGELOG.md` (new bullet under the current version's Added) and expand `docs/integrations/agent.md` with the autonomy + approval sections and the new config fields. Run `make swagger` to regenerate REST docs for `/approve` + `/cancel` (add Swagger annotations to the two new handlers first, matching neighboring handlers).
- [ ] **Step 2** — `make pre-commit-run` (clear stale golangci cache first if paths look wrong: `golangci-lint cache clean`); fix any real issues. `make test`.
- [ ] **Step 3** — Run the CodeRabbit CLI review on the branch diff; fix actionable findings; re-run until clean or only style/deferred remain. Record deferrals with rationale.
- [ ] **Step 4** — Deploy: `ansible-playbook -i ansible/inventory.yml ansible/deploy.yml --tags build,deploy,verify`; tail the log for panics; confirm the verify suite passes.
- [ ] **Step 5** — Enabled-path checks on hardware (set `agent_config.json` enabled + `UMA_AGENT_API_KEY`, restart): confirm an incident wakes a session and an approval resumes to completion. (If no API key is available, this sub-item is the operator-deferred one — note it explicitly.)
- [ ] **Step 6** — Commit docs.

```json:metadata
{"files": ["CHANGELOG.md", "docs/integrations/agent.md"], "verifyCommand": "make pre-commit-run && make test && ansible-playbook -i ansible/inventory.yml ansible/deploy.yml --tags build,deploy,verify", "acceptanceCriteria": ["CHANGELOG Phase-2 entry under Added", "agent.md documents triggers + approval workflow + forbid-list + new config", "make pre-commit-run passes", "make test passes", "CodeRabbit CLI review run and findings addressed", "plugin builds + deploys to Unraid with no panics; verify suite passes (no regression)", "agent-disabled: existing endpoints unchanged, no agent_wake subscription", "agent-enabled: incident wakes a session; high-risk -> awaiting_approval -> approve resumes to completed"], "userGate": true, "tags": ["user-gate"], "gateScope": "all"}
```

---

## Self-Review Notes

- **Spec coverage (Phase 2 = Autonomy):** event-driven `agent_wake` from alerting+watchdog (Task 6) with debounce/dedup/cooldown/concurrency (Task 5); tiered approval gate with pause→resume surviving restart via persisted `Transcript` (Tasks 1–3); approval TTL default-deny (Task 4); non-overridable forbid-list (Task 2, re-checked on approval Task 3); REST approve/cancel (Task 7); MCP agent tools (Task 8); docs+review+hardware verify (Task 9). Deferred Phase-3 items (planner, memory, learning, runbooks, multi-turn chat) are explicitly out of scope.
- **Type consistency:** `dto.AgentWakeEvent`/`ApprovalRequest`/`AgentMessage`/`AgentMsgToolCall`/`SessionAwaitingApproval` (Task 1) are used unchanged in Tasks 2–8. `constants.TopicAgentWake` (Task 1) is the single topic used by alerting/watchdog publishers (Task 6) and the agent subscriber (Task 5). `Service.SetEventBus`/`Start`/`handleWake`/`startAutonomousSession`/`ApproveAction`/`CancelSession`/`SweepExpiredApprovals`/`isForbidden`/`invokeTool` are introduced once and reused consistently. `runLoop` is changed to `func (s *Service) runLoop(ctx, *dto.AgentSession)` (no return) — `StartSession`, `ApproveAction`, and `startAutonomousSession` all call this same signature.
- **Import-cycle check:** `agent` imports `domain`, `constants`, `dto`, `llm`, `tools`, `logger`, `controllers` — never `api`/`mcp`/`alerting`/`watchdog`. `alerting`/`watchdog` import `domain`/`constants`/`dto` (already do). `mcp` imports `agent` (agent doesn't import mcp). `api` imports `agent` (already, Phase 1). No cycles.
- **Resume-across-restart:** the persisted `Transcript` + `PendingApproval` on `AgentSession` (saved by `Store.Save`) is what a future session load resumes from; `ApproveAction` reconstructs `llm.Message`s via `transcriptToMessages`. This satisfies the spec's "survives a daemon restart" requirement without persisting `llm` types directly.
- **Placeholders:** none. The only "match the existing pattern" references point at already-committed code (`handlers_agent.go`, neighboring MCP tools), not at other plan tasks.
