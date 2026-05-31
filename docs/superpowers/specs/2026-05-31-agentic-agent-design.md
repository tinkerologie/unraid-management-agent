# Design: Making the Unraid Management Agent Agentic

**Date:** 2026-05-31
**Status:** Approved (design phase)
**Author:** brainstorming session

## Summary

Today the Unraid Management Agent is **agent-_ready_** but not itself **agentic**: collectors
publish to a pubsub bus, the API server caches the data, and REST / WebSocket / MCP surfaces
expose it. A rule-based `watchdog` performs deterministic remediation (`notify`,
`restart_container`, `webhook`) and the `alerting` engine maps `action://` channels to
controller operations. The 54+ MCP tools let _external_ AI agents drive the daemon, but there
is no autonomous reasoning loop _inside_ the daemon.

This design adds an embedded **Agent Core** service (`daemon/services/agent/`) that turns the
daemon into a self-driving operator: it perceives system state, reasons with a pluggable LLM,
plans multi-step remediations, acts through the existing validated controllers/MCP tools under a
tiered-by-risk approval model, and learns from past incidents. It wakes on alerts/health-check
failures (event-driven) and on operator request (on-demand), with zero idle token cost.

## Goals

- **Autonomous perceive → think → act loop** inside the daemon.
- **Goal-driven planning** — decompose a high-level goal into steps, execute, self-correct.
- **Natural-language operator chat** — investigate and answer/act conversationally.
- **Self-improving memory** — remember incidents, fixes, and approval preferences.
- **Pluggable LLM provider** — cloud (BYO key) or local (Ollama / OpenAI-compatible).
- **Tiered-by-risk autonomy** — read-only always auto, low-risk auto, high-risk needs approval.
- **Event-driven + on-demand triggers** — no idle token burn.
- **Surface:** new REST + WebSocket API **and** the agent exposed as MCP tools.

## Non-Goals (Out of Scope / Future)

- Native Unraid-notification approve/deny flow for high-risk actions (future).
- Dedicated Unraid web-UI plugin page (future; PHP/JS lives outside the Go daemon).
- Vector-database / embeddings-based memory recall (start with keyed keyword recall; the recall
  interface leaves room to add this later).
- A separate sidecar agent process or third-party agent framework dependency
  (rejected: fragments deployment; a hand-rolled bounded ReAct loop keeps the binary lean and
  auditable).
- Letting the agent silently auto-escalate its own autonomy (learning is suggest-not-mutate).

## Approved Decisions

| Decision | Choice |
| --- | --- |
| Architecture | **A — embedded Agent Core service**, reusing pubsub, controllers, and the MCP tool catalog. |
| Brain location | **Pluggable provider** behind one interface (cloud BYO + local). |
| Autonomy model | **Tiered by risk** (ReadOnly auto / LowRisk auto / HighRisk approval). |
| Triggers | **Event-driven** (alerting + watchdog wake the agent) **+ on-demand**. |
| Surface | **New REST + WebSocket** and **agent exposed as MCP tools**. |
| Reasoning | **Hand-rolled bounded ReAct loop** in Go (no heavy framework). |
| Tools | **Registry wraps existing MCP/controller calls** (single source of truth). |
| Runbooks | **Reuse `remediation/runbooks.go`** as the runbook source of truth. |
| Learning | **Suggest-not-mutate** — agent proposes preference/runbook changes; user confirms. |
| Array stop | **HighRisk (approval-required, reversible)**, not forbidden. |
| Forbid-list | Non-overridable circuit breaker for irreversible ops (disk format, parity clear/disable, partition changes) — agent may _describe_ but the gate _never_ executes, even with approval. |
| Default state | **Disabled by default** — daemon behaves exactly as today until opted in. |

## Architecture

A new internal service, `daemon/services/agent/`, is **just another subscriber** on the existing
pubsub bus and **just another consumer** of the existing controllers/MCP tools. The
collect → bus → cache → REST/WS/MCP pipeline is unchanged.

```
                  ┌─────────── existing (unchanged) ───────────┐
 collectors ──▶ pubsub Hub ──▶ apiServer cache ──▶ REST / WS / MCP
                    │                                    ▲
   alerting ────────┤ publishes AlertEvent               │ reuses 54+ MCP
   watchdog ────────┘ to "agent_wake" topic              │ tool definitions
                    │                                     │
                    ▼                                     │
        ┌───────────────────── daemon/services/agent ─────────────────────┐
        │  trigger.go    subscribes wake topics + on-demand entrypoint     │
        │  llm/          provider interface (cloud BYO + local Ollama)     │
        │  tools/        registry: wraps MCP/controller calls + risk tier  │
        │  loop.go       ReAct session: perceive→think→act→observe         │
        │  planner.go    goal decomposition + step tracking                │
        │  policy.go     tiered approval gate (auto / approve / forbid)    │
        │  memory/       episodic incidents + semantic prefs (JSON store)  │
        │  session.go    session state, transcript, status                 │
        │  config.go     provider, tiers, limits, forbid-list              │
        │  store.go      NewStore("") persistence (sessions, memory, cfg)  │
        └─────────────────────────────────────────────────────────────────┘
                    │                         │
                    ▼                         ▼
        new REST: /api/v1/agent/*    new MCP tools: agent_*    new WS topic: agent_stream
```

**Key principle:** the agent never bypasses the safety layer. It acts **only** through existing
controllers/MCP tools (which already perform input validation via `lib.Validate*` and execute via
`lib.ExecCommand`). The agent is a reasoning layer on top, not a new execution path.

## Components

### LLM Provider (`agent/llm/`)

One small interface keeps cloud and local interchangeable:

```go
type Provider interface {
    Name() string
    // Chat sends messages + available tool schemas, returns either assistant
    // text or tool-call requests. Streaming delivered via a channel.
    Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error)
}
```

- Implementations: `anthropic.go`, `openai.go` (covers OpenAI-compatible gateways), `ollama.go`.
- Selected by config. API keys use `json:"-"` (like the MQTT password) and load from env or a
  secret file; never logged.
- A provider that fails to initialize disables the agent gracefully with a logged warning,
  mirroring the fan/CPU controller pattern.

### Tool Registry (`agent/tools/`)

Wraps the existing MCP/controller surface instead of duplicating handlers. Each tool carries the
metadata the LLM and policy gate need:

```go
type RiskTier int // ReadOnly | LowRisk | HighRisk

type Tool struct {
    Name        string          // e.g. "restart_container"
    Description string
    Schema      json.RawMessage // JSON schema reused from the MCP tool definition
    RiskTier    RiskTier
    Invoke      func(ctx context.Context, args json.RawMessage) (Result, error)
}
```

Tier assignment lives in **one auditable file**. Examples:

| Tier | Examples |
| --- | --- |
| ReadOnly (always auto) | `get_system`, `query_metric_history`, `system_health_report` |
| LowRisk (auto by default) | `restart_container`, `start_vm` |
| HighRisk (approval) | `stop_array`, `force_stop_vm`, delete/destroy operations |
| Forbidden (never, even w/ approval) | disk format, parity clear/disable, partition changes |

### Agent Loop (`loop.go`)

A bounded **ReAct** cycle per session:

1. **Perceive** — read current state cheaply from the apiServer cache + recent alert/event
   context. No new collection is triggered.
2. **Think** — provider call with system prompt + available tool schemas + recalled memory +
   running transcript.
3. **Act** — if the model requests a tool, the **policy gate** decides: execute now, or emit an
   approval request and pause the session.
4. **Observe** — feed the tool result back into the transcript; repeat.

**Hard stops:** `MaxIterations`, `MaxTokensPerSession`, and a wall-clock `SessionDeadline` — a
confused model can neither loop forever nor run up an unbounded bill.

### Planner (`planner.go`)

For goal-driven requests (e.g. "free up 200 GB on the cache pool"), the first think-step produces
a short ordered plan (each step: intent, expected tool, success check). The loop executes steps and
**re-plans** when a step's observation contradicts the plan. The plan is stored on the session so
the operator (and WS stream) can watch progress, and so the agent can resume after an approval
pause.

### Memory (`agent/memory/`)

Two JSON-on-disk stores following the `NewStore("")` pattern (persisted under the agent config dir,
surviving restarts):

- **Episodic — incident log.** Each completed session writes a compact record: trigger, system
  snapshot at the time, plan, actions taken, approvals granted/denied, and outcome
  (resolved / failed / escalated).
- **Semantic — preferences & learned runbooks.**
  - _User preferences_ distilled from approval patterns (e.g. "always approve restarting `plex`",
    "denied stopping the array at night"), stored as structured rules the policy gate reads.
  - _Runbooks_ — on a successful resolution, the agent distills "symptom → diagnosis → fix" into a
    runbook keyed by symptom signature, **proposed into the existing `remediation/runbooks.go`
    store** rather than a parallel system.

**Recall** is deliberately simple (no vector DB): each new session uses the trigger's signature
(alert type + affected subsystem) for a keyed lookup over episodic + runbook stores; top matches are
injected into the system prompt as "relevant past incidents." Keyword/tag match — cheap,
deterministic, debuggable. The recall interface allows swapping in embeddings later.

**Learning is suggest-not-mutate:** the agent never silently rewrites its own autonomy rules. A
learned preference (e.g. "auto-approve plex restarts") is _proposed_ and only takes effect after the
user confirms it via the same approval mechanism used for actions. The agent cannot quietly escalate
its own privileges.

## Triggers (`trigger.go`)

Two entrypoints; zero idle token burn.

1. **Event-driven (autonomous).** The cheap rule-based layer remains the always-on tripwire. The
   alerting engine publishes its `AlertEvent` to a new pubsub topic **`agent_wake`** (a few lines in
   `engine.go`/`dispatcher.go`); the watchdog publishes on a check transitioning to unhealthy. The
   agent subscribes to `agent_wake` **before collectors start** (honoring the critical init-order
   rule in `orchestrator.go`).
   - **Debounce + dedup:** waking sessions are keyed by subsystem so a burst of related alerts wakes
     **one** investigation.
   - **Concurrency cap** (e.g. max 2 live autonomous sessions) and a per-trigger **cooldown**
     (mirroring the watchdog's existing `RemediationCooldown`) bound cost.
   - An autonomous wake starts a session whose initial goal is "investigate and, within policy,
     remediate this alert."
2. **On-demand.** A REST/WS/MCP entrypoint where the operator starts a session with a question or
   goal. Never debounced.

Both paths converge on the same `loop.go`.

## Approval Gate / Tiered Autonomy (`policy.go`)

```
tool requested ──▶ resolve RiskTier ──▶ consult policy + learned prefs
   ReadOnly  ─────────────────────────────────────────▶ execute
   LowRisk   ──▶ (config: auto?) ──yes──────────────────▶ execute
                                  ──no───┐
   HighRisk  ─────────────────────────── ├─▶ emit ApprovalRequest, PAUSE session
                                          │      └─ user approves via REST/MCP ─▶ resume + execute
                                          └─ user denies / timeout ─▶ record, agent re-plans or stops
   Forbidden ─────────────────────────────────────────▶ refuse (describe only; never execute)
```

- A session needing approval **persists and pauses** — it survives a daemon restart and resumes
  when the decision arrives, or expires after a configurable TTL (**default deny**).
- Every decision (auto or human) is written to the episodic log — the audit trail and the raw
  material for learned preferences.
- The **forbid-list** is a non-overridable circuit breaker independent of the LLM: even a user who
  _could_ approve cannot make the gate execute an irreversible op.

## API / MCP / WebSocket Surface

### REST (`api/handlers.go` + `setupRoutes()`), under `/api/v1/agent`

| Method & path | Purpose |
| --- | --- |
| `POST /agent/sessions` | Start a session (`{goal}` or `{question}`). Returns session ID. |
| `GET  /agent/sessions` | List sessions (status, trigger, started). |
| `GET  /agent/sessions/{id}` | Full session: plan, transcript, actions, pending approval. |
| `POST /agent/sessions/{id}/messages` | Continue the conversation (operator chat). |
| `POST /agent/sessions/{id}/approve` | Approve/deny a pending action (`{action_id, decision, remember?}`). |
| `POST /agent/sessions/{id}/cancel` | Stop a running/paused session. |
| `GET  /agent/config` / `PUT /agent/config` | Provider, tier policy, limits (secrets write-only). |
| `GET  /agent/memory` | Browse episodic incidents + learned prefs/runbooks. |

- Reads use `RLock/RUnlock` on the agent cache; control endpoints return `dto.Response`.
- New DTOs (`daemon/dto/`): `AgentSession`, `AgentStep`, `AgentAction`, `ApprovalRequest`,
  `AgentConfig`, `AgentMemoryEntry`.

### WebSocket

New event topic **`agent_stream`** broadcasting `dto.WSEvent`s as the agent reasons/acts
(`step_started`, `tool_called`, `approval_required`, `session_completed`) — the "watch it think"
live feed, reusing the existing hub.

### MCP tools (`mcp/server.go`, same `mcp.AddTool` pattern)

`agent_start_session`, `agent_get_session`, `agent_send_message`, `agent_approve_action`,
`agent_list_sessions`, `agent_get_memory`. This lets an external Claude/Cursor client **drive the
internal agent** — the two layers compose rather than compete.

**Approvals are via REST/MCP only** in this scope (native-notification and GUI-page approvals are
future work).

## Config & Secrets (`agent/config.go`)

Persisted JSON like the watchdog/alert stores.

```go
type AgentConfig struct {
    Enabled               bool              // off by default — opt-in
    Provider              string            // "anthropic" | "openai" | "ollama"
    Model                 string
    Endpoint              string            // ollama / openai-compatible gateways
    APIKey                string `json:"-"` // never serialized/logged; env or secret file
    Autonomy              map[RiskTier]Mode // ReadOnly→auto, LowRisk→auto, HighRisk→approve
    MaxIterations         int
    MaxTokensPerSession   int
    SessionDeadline       time.Duration
    MaxConcurrentSessions int
    WakeCooldown          time.Duration
    ForbidList            []string          // non-overridable irreversible ops
}
```

Disabled by default: the daemon behaves exactly as today until a user opts in and configures a
provider.

## Safety & Cost Guardrails (consolidated)

- Per-session iteration / token / wall-clock caps.
- Wake debounce + dedup + cooldown + concurrency cap.
- Tiered approval gate + non-overridable forbid-list.
- Suggest-not-mutate learning (no silent privilege escalation).
- Agent acts only through validated controllers/MCP tools.
- Every action audited to the episodic log.
- All LLM prompts/responses logged at debug level for post-hoc review.

## Testing

Table-driven, per project conventions. A **`mockProvider`** implementing `llm.Provider` returns
scripted tool-call sequences so the entire loop is testable with zero network/tokens.

Coverage targets:

- Policy-gate tiering, including forbid-list and learned-preference interaction.
- Debounce / dedup / cooldown / concurrency cap.
- Approval pause → resume → persist-across-restart.
- Max-iteration / max-token termination.
- Planner re-plan on observation contradiction.
- Memory recall keying (signature → relevant incidents).
- Security cases: confirm the agent cannot bypass input validation (it routes through existing
  validated controllers).

## Orchestrator Wiring

In `orchestrator.go`, after the watchdog block and **before** collectors start:

1. Construct the agent service (load `AgentConfig`; if disabled or provider init fails, log and
   skip — no behavior change).
2. `apiServer.SetAgent(agentSvc)` and `mcpServer.SetAgent(agentSvc)`.
3. Subscribe to wake topics (`agent_wake`) **before** collectors publish, per the init-order rule.
4. `wg.Go(func() { defer recover-with-stack; agentSvc.Start(ctx) })`.

Graceful shutdown: cancel running sessions, flush episodic/memory stores, then return — added to the
existing ordered shutdown sequence.

## Phasing

Each phase is shippable behind the `Enabled` flag and maps to its own implementation plan.

1. **Foundation** — provider interface + one provider, tool registry + tiers, bounded ReAct loop,
   sessions, REST + WS, `mockProvider` tests. _On-demand chat works; read-only + low-risk auto._
2. **Autonomy** — `agent_wake` from alerting/watchdog, debounce/dedup, approval gate with
   pause/resume, MCP agent tools.
3. **Planning & memory** — planner/re-plan, episodic + semantic stores, recall injection,
   suggest-not-mutate learning, runbook reuse.

## Open Questions

None blocking. Provider-specific details (exact model defaults, token accounting per provider) are
deferred to the Foundation implementation plan.
