# Embedded Agent Core

> **Status: Beta (Phase 2)** — Disabled by default. Opt-in required.
> Phase 2 adds event-driven autonomy (alert/health-check triggers), a full
> approval gate with pause/resume surviving restarts, a non-overridable forbid-list,
> and OpenAI-compatible LLM providers (OpenRouter, Gemini).

The Unraid Management Agent includes an embedded autonomous operator ("Agent Core")
that can reason about your Unraid server and take actions on your behalf using a
large language model (LLM). Unlike the [MCP integration](mcp.md) — which turns your
server into a tool for an external AI client — the Agent Core runs **inside the daemon**
and drives its own reasoning loop without an external orchestrator.

## Overview

The Agent Core implements a bounded **ReAct** (Reason + Act) loop:

1. The operator receives a natural-language goal (e.g. "Check whether any containers
   have stopped unexpectedly and restart them if safe to do so").
2. It reasons step-by-step using an LLM, calling registered tools to gather data
   and take actions.
3. It returns a final answer once the goal is satisfied, the iteration cap is reached,
   or the token or time budget is exhausted.

Sessions can also start **automatically** when an alert fires or a health check fails
(see [Autonomy / triggers](#autonomy--triggers) below).

### Phase 2 scope

| Capability                      | Status    |
| ------------------------------- | --------- |
| Read-only tool execution        | Supported |
| Low-risk action execution       | Supported |
| High-risk approval gate         | Supported |
| Approval pause/resume (restart) | Supported |
| Event-driven triggers (alerts)  | Supported |
| Forbid-list (irreversible ops)  | Supported |
| MCP agent tools                 | Supported |
| Memory / cross-session learning | Phase 3   |
| Planner / runbook reuse         | Phase 3   |
| Multi-turn chat                 | Phase 3   |

### Safety model

Tools are assigned one of three **risk tiers**. The tier determines whether the agent
can execute a tool automatically or must refuse it:

| Tier        | Examples                                                        | Agent behaviour                                             |
| ----------- | --------------------------------------------------------------- | ----------------------------------------------------------- |
| `read_only` | `get_system_info`, `get_array_status`, `list_docker_containers` | Auto-executes                                               |
| `low`       | `restart_container`                                             | Auto-executes                                               |
| `high`      | Array start/stop, system reboot                                 | Pauses session — operator must approve via REST or MCP tool |

The `autonomy` map in `agent_config.json` (see below) lets you override the default
behaviour per tier (`"auto"`, `"approve"`, or `"forbid"`).

### Safety caps

Every session is bounded by three hard limits:

| Config field             | Default | Description                                         |
| ------------------------ | ------- | --------------------------------------------------- |
| `max_iterations`         | 12      | Maximum ReAct loop iterations before the loop stops |
| `max_tokens_per_session` | 60000   | Cumulative token budget for the entire session      |
| `session_deadline_secs`  | 180     | Wall-clock deadline in seconds (3 minutes)          |

The loop stops as soon as **any** cap is reached and returns the best answer found so far.

---

## Enabling the Agent

### 1. Create the configuration file

Create `/boot/config/plugins/unraid-management-agent/agent_config.json`:

```json
{
  "enabled": true,
  "provider": "anthropic",
  "model": "claude-opus-4-8",
  "autonomy": {
    "read_only": "auto",
    "low": "auto",
    "high": "approve"
  },
  "max_iterations": 12,
  "max_tokens_per_session": 60000,
  "session_deadline_secs": 180,
  "wake_debounce_secs": 30,
  "wake_cooldown_secs": 300,
  "max_concurrent_sessions": 2,
  "approval_ttl_secs": 3600
}
```

> **API key:** Never put your API key in `agent_config.json`. The daemon reads it
> from the environment variable `UMA_AGENT_API_KEY` at startup.

### 2. Set the API key environment variable

Add `UMA_AGENT_API_KEY=<your-key>` to the daemon's environment before it starts.
On Unraid the typical approach is to add the export to
`/boot/config/go/environment` or the plugin's startup script, depending on your
setup.

### 3. Restart the daemon

```bash
/etc/rc.d/rc.unraid-management-agent restart
```

After restart, call `GET /api/v1/agent/sessions` to confirm the agent is enabled
(a 200 response with an empty list means it is running; a 503 means it is still
disabled or misconfigured).

---

## Configuration reference

```json
{
  "enabled": false,
  "provider": "anthropic",
  "model": "claude-opus-4-8",
  "endpoint": "",
  "autonomy": {
    "read_only": "auto",
    "low": "auto",
    "high": "approve"
  },
  "max_iterations": 12,
  "max_tokens_per_session": 60000,
  "session_deadline_secs": 180,
  "wake_debounce_secs": 30,
  "wake_cooldown_secs": 300,
  "max_concurrent_sessions": 2,
  "approval_ttl_secs": 3600,
  "forbid_list": []
}
```

| Field                     | Type              | Default   | Description                                                                            |
| ------------------------- | ----------------- | --------- | -------------------------------------------------------------------------------------- |
| `enabled`                 | bool              | `false`   | Master switch — set to `true` to activate the agent                                    |
| `provider`                | string            | —         | LLM provider: `"anthropic"`, `"openai"`, `"openrouter"`, or `"gemini"`                 |
| `model`                   | string            | —         | Model name passed to the provider (e.g. `"claude-opus-4-8"`)                           |
| `endpoint`                | string            | `""`      | Optional base URL override for the provider API (leave empty for the provider default) |
| `autonomy`                | map[string]string | see above | Per-risk-tier behaviour: `"auto"`, `"approve"`, or `"forbid"`                          |
| `max_iterations`          | int               | `12`      | Hard cap on ReAct loop iterations per session                                          |
| `max_tokens_per_session`  | int               | `60000`   | Cumulative token budget across all LLM calls in a session                              |
| `session_deadline_secs`   | int               | `180`     | Wall-clock time limit for a session (seconds)                                          |
| `wake_debounce_secs`      | int               | `30`      | Minimum gap between wake events for the same subsystem before a new session is started |
| `wake_cooldown_secs`      | int               | `300`     | Minimum time (seconds) between autonomous sessions for the same subsystem              |
| `max_concurrent_sessions` | int               | `2`       | Maximum number of autonomous sessions that may run simultaneously                      |
| `approval_ttl_secs`       | int               | `3600`    | Seconds before an unresolved pending approval is automatically denied                  |
| `forbid_list`             | []string          | `[]`      | Additional tool names to block unconditionally (merged with the built-in forbid-list)  |

The API key is **never stored in the config file**. It must be provided via the
`UMA_AGENT_API_KEY` environment variable.

---

## Autonomy / triggers

The agent can wake itself automatically without a user-initiated REST call.

### How wakes work

1. When an alert fires or a health check fails, the subsystem (alerting Engine or
   watchdog Runner) publishes a `dto.AgentWakeEvent` to the internal `agent_wake`
   pub-sub topic.
2. The agent subscribes to this topic at daemon startup (before collectors start).
3. On receiving a wake event, the agent applies three throttle layers before spawning
   a session:

| Throttle            | Config field              | Default | What it does                                                              |
| ------------------- | ------------------------- | ------- | ------------------------------------------------------------------------- |
| **Debounce**        | `wake_debounce_secs`      | `30 s`  | Ignores duplicate wake events for the same subsystem within the window    |
| **Cooldown**        | `wake_cooldown_secs`      | `300 s` | Prevents a new session for the same subsystem until the cooldown expires  |
| **Concurrency cap** | `max_concurrent_sessions` | `2`     | Drops a wake event if the number of running autonomous sessions is at cap |

If all three checks pass, the agent starts a new session with a pre-configured goal
derived from the wake event (e.g. "An alert fired for subsystem 'docker'. Investigate
and remediate if safe to do so.").

> **Note:** event-driven sessions use the same ReAct loop, safety tiers, and approval
> gate as manually started sessions. No special configuration is needed to enable
> triggers beyond enabling the agent itself (`"enabled": true`).

---

## Approval workflow

When a session needs to call a `high`-risk tool and the `autonomy.high` tier is set
to `"approve"` (the default in Phase 2), the session **pauses** with status
`awaiting_approval`.

### What the session exposes while paused

```json
{
  "id": "01J3XKZP2V8N4DRHMGT5W6QA00",
  "status": "awaiting_approval",
  "pending_approval": {
    "action_id": "act_01J3XKZP2V8N4DRHMGT5W6QA01",
    "tool": "start_array",
    "args": {},
    "risk_tier": "high",
    "reason": "The agent needs to start the array to complete the goal."
  }
}
```

The session transcript is persisted to disk so it survives a daemon restart — when
the daemon comes back up, an approval or denial will resume the loop from exactly
where it left off.

### Approving or denying via REST

```bash
# Approve
curl -s -X POST http://<unraid-ip>:8043/api/v1/agent/sessions/01J3XKZP2V8N4DRHMGT5W6QA00/approve \
  -H "Content-Type: application/json" \
  -d '{"action_id": "act_01J3XKZP2V8N4DRHMGT5W6QA01", "approve": true}'

# Deny
curl -s -X POST http://<unraid-ip>:8043/api/v1/agent/sessions/01J3XKZP2V8N4DRHMGT5W6QA00/approve \
  -H "Content-Type: application/json" \
  -d '{"action_id": "act_01J3XKZP2V8N4DRHMGT5W6QA01", "approve": false}'
```

On approval the session resumes and the tool executes. On denial the agent receives a
"denied" error for that tool call and continues its reasoning loop (it may propose
an alternative approach or surface a final answer without executing the action).

### Approving or denying via MCP

External MCP clients can use the `agent_approve_action` tool:

```json
{
  "name": "agent_approve_action",
  "arguments": {
    "session_id": "01J3XKZP2V8N4DRHMGT5W6QA00",
    "action_id": "act_01J3XKZP2V8N4DRHMGT5W6QA01",
    "approve": true
  }
}
```

### Approval TTL (auto-deny)

If an approval is not resolved within `approval_ttl_secs` (default `3600`, i.e.
1 hour), a background sweeper automatically denies it. The session then resumes with
a denial and winds down gracefully.

### Cancelling a session

To abandon a session entirely (whether running or awaiting approval):

```bash
curl -s -X POST http://<unraid-ip>:8043/api/v1/agent/sessions/01J3XKZP2V8N4DRHMGT5W6QA00/cancel
```

The session transitions to status `cancelled` and any pending approvals are cleared.

---

## Forbid-list

Some operations are irreversible at the hardware or filesystem level. The agent will
**never** execute them, even if the operator explicitly approves:

| Forbidden tool      | Why                                             |
| ------------------- | ----------------------------------------------- |
| `format_disk`       | Destroys all data on the target disk            |
| `clear_parity`      | Invalidates array parity with no recovery path  |
| `disable_parity`    | Removes parity protection from the array        |
| `partition_disk`    | Rewrites the partition table — cannot be undone |
| `delete_array_disk` | Removes a disk from the array configuration     |

These are enforced in the gate layer, not the autonomy config, so they cannot be
overridden by setting `autonomy.high = "auto"`. The agent will describe what it
would have done and explain why it cannot proceed.

You can extend the forbid-list with additional tool names via the `forbid_list` field
in `agent_config.json` (merged with the built-in defaults at startup).

---

## LLM providers

The `provider` field in `agent_config.json` selects the LLM backend. All providers
read the API key from the `UMA_AGENT_API_KEY` environment variable.

| `provider`   | Default endpoint                                                           | Notes                                           |
| ------------ | -------------------------------------------------------------------------- | ----------------------------------------------- |
| `anthropic`  | Anthropic API (default)                                                    | Uses the Messages API                           |
| `openai`     | `https://api.openai.com/v1/chat/completions`                               | Any OpenAI-compatible endpoint                  |
| `openrouter` | `https://openrouter.ai/api/v1/chat/completions`                            | Routes to many providers; free models available |
| `gemini`     | `https://generativelanguage.googleapis.com/v1beta/openai/chat/completions` | Gemini via its OpenAI-compatible API            |

Override `endpoint` to point to a self-hosted or proxy endpoint. Example using
OpenRouter with a free model:

```json
{
  "enabled": true,
  "provider": "openrouter",
  "model": "openai/gpt-oss-20b:free",
  "autonomy": {
    "read_only": "auto",
    "low": "auto",
    "high": "approve"
  },
  "max_iterations": 12,
  "max_tokens_per_session": 60000,
  "session_deadline_secs": 180,
  "wake_debounce_secs": 30,
  "wake_cooldown_secs": 300,
  "max_concurrent_sessions": 2,
  "approval_ttl_secs": 3600
}
```

> Set `UMA_AGENT_API_KEY` to your OpenRouter API key. The `endpoint` field can be
> omitted — the default OpenRouter URL is used automatically.

---

## REST API reference

All agent endpoints live under `/api/v1/agent`. When the agent is disabled they
return HTTP **503** with `{"success": false, "message": "agent is disabled"}`.

### POST /api/v1/agent/sessions

Start a new agent session synchronously. The request blocks until the session
completes (goal reached, iteration cap, token budget, or deadline).

**Request body:**

```json
{ "goal": "Check whether any containers have exited and restart them." }
```

**Example:**

```bash
curl -s -X POST http://<unraid-ip>:8043/api/v1/agent/sessions \
  -H "Content-Type: application/json" \
  -d '{"goal": "Check whether any containers have exited and restart them."}'
```

**Example response (200 OK):**

```json
{
  "id": "01J3XKZP2V8N4DRHMGT5W6QA00",
  "goal": "Check whether any containers have exited and restart them.",
  "status": "completed",
  "started_at": "2026-06-01T14:22:01Z",
  "completed_at": "2026-06-01T14:22:18Z",
  "iterations": 4,
  "total_tokens": 3210,
  "final_answer": "All containers are running. No restarts were needed.",
  "steps": [
    {
      "iteration": 1,
      "thought": "I need to list the Docker containers to check their state.",
      "tool_name": "list_docker_containers",
      "tool_input": {},
      "tool_output": "...",
      "tokens_used": 820
    }
  ]
}
```

**When disabled (503):**

```json
{
  "success": false,
  "message": "agent is disabled"
}
```

---

### GET /api/v1/agent/sessions

List all persisted sessions, newest first.

**Example:**

```bash
curl -s http://<unraid-ip>:8043/api/v1/agent/sessions
```

**Example response (200 OK):**

```json
[
  {
    "id": "01J3XKZP2V8N4DRHMGT5W6QA00",
    "goal": "Check whether any containers have exited and restart them.",
    "status": "completed",
    "started_at": "2026-06-01T14:22:01Z",
    "completed_at": "2026-06-01T14:22:18Z",
    "iterations": 4,
    "total_tokens": 3210,
    "final_answer": "All containers are running. No restarts were needed."
  }
]
```

---

### GET /api/v1/agent/sessions/{id}

Retrieve a single session by its ULID. Returns **404** if not found.

**Example:**

```bash
curl -s http://<unraid-ip>:8043/api/v1/agent/sessions/01J3XKZP2V8N4DRHMGT5W6QA00
```

---

### POST /api/v1/agent/sessions/{id}/approve

Approve or deny a pending high-risk tool call. The session must be in status
`awaiting_approval`. Returns the updated session on success, **400** on invalid
request or wrong state, **503** when the agent is disabled.

**Request body:**

```json
{ "action_id": "<pending action ID>", "approve": true }
```

**Example (approve):**

```bash
curl -s -X POST http://<unraid-ip>:8043/api/v1/agent/sessions/01J3XKZP2V8N4DRHMGT5W6QA00/approve \
  -H "Content-Type: application/json" \
  -d '{"action_id": "act_01J3XKZP2V8N4DRHMGT5W6QA01", "approve": true}'
```

---

### POST /api/v1/agent/sessions/{id}/cancel

Cancel a running or awaiting-approval session. Idempotent — cancelling an already
completed session returns **400**. Returns the updated session on success.

**Example:**

```bash
curl -s -X POST http://<unraid-ip>:8043/api/v1/agent/sessions/01J3XKZP2V8N4DRHMGT5W6QA00/cancel
```

---

## MCP tools

The following MCP tools are exposed under the `agent_*` namespace for external AI
agents and MCP clients:

| Tool                   | Description                                                       |
| ---------------------- | ----------------------------------------------------------------- |
| `agent_start_session`  | Start a new agent session with a natural-language goal            |
| `agent_get_session`    | Retrieve a session by ID (includes full step transcript)          |
| `agent_list_sessions`  | List all persisted sessions, newest first                         |
| `agent_approve_action` | Approve or deny a pending high-risk action in an awaiting session |

---

## WebSocket events

Agent activity is broadcast on the `agent_stream` topic via the `/ws` WebSocket
endpoint. Each message is a standard `dto.WSEvent`:

```json
{
  "event": "<event_type>",
  "timestamp": "2026-06-01T14:22:01Z",
  "data": { ... }
}
```

| Event type                | Fired when                                         | `data` payload                                                          |
| ------------------------- | -------------------------------------------------- | ----------------------------------------------------------------------- |
| `agent_session_started`   | A new session begins                               | `{id, goal, started_at}`                                                |
| `agent_tool_called`       | The loop is about to call a tool                   | `{session_id, iteration, tool_name, tool_input}`                        |
| `agent_step_completed`    | One ReAct iteration finishes                       | `{session_id, iteration, thought, tool_name, tool_output, tokens_used}` |
| `agent_session_completed` | The session ends successfully with a final answer  | `{id, final_answer, iterations, total_tokens, completed_at}`            |
| `agent_session_failed`    | The session terminates due to an error or hard cap | `{id, error, iterations, total_tokens, completed_at}`                   |
| `agent_approval_required` | The session pauses waiting for operator approval   | `{session_id, action_id, tool, args, risk_tier, reason}`                |
| `agent_session_cancelled` | An operator or TTL sweeper cancelled the session   | `{id, cancelled_at}`                                                    |

Connect to the WebSocket endpoint and filter by `event` prefix `agent_` to stream
live agent reasoning to a UI or monitoring tool:

```bash
# Using websocat (https://github.com/vi/websocat)
websocat ws://<unraid-ip>:8043/ws | grep '"agent_'
```

---

## Sessions persistence

Completed sessions are persisted to
`/boot/config/plugins/unraid-management-agent/agent_sessions.json`. The file is
read on daemon startup so sessions survive restarts. There is currently no automatic
pruning — remove or truncate the file manually if it grows too large.

---

## Coming in later phases

- **Phase 3 — Memory and learning:** The agent retains facts across sessions so it
  can learn server-specific context over time (e.g. "disk 3 runs hot after array
  starts").
- **Phase 3 — Planner and runbook reuse:** Higher-level planning step that selects
  and parameterises a runbook before entering the ReAct loop, reducing token usage
  for known remediation patterns.
- **Phase 3 — Multi-turn chat:** Interactive back-and-forth with the agent within a
  long-lived session, beyond single-goal execution.

---

## Related documentation

- [MCP Integration](mcp.md)
- [REST API Reference](../api/API_REFERENCE.md)
- [WebSocket Events](../websocket/WEBSOCKET_EVENTS_DOCUMENTATION.md)
