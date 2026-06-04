# Embedded Agent Core

> **Status: Beta (Phase 3)** — Disabled by default. Opt-in required.
> Phase 3 adds episodic memory with semantic recall, a goal-decomposition planner,
> suggest-not-mutate learning (preference + runbook proposals), multi-turn chat
> (`SendMessage`), and new REST / MCP surfaces for memory and preferences.

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

### Phase 3 scope (current)

| Capability                      | Status    |
| ------------------------------- | --------- |
| Read-only tool execution        | Supported |
| Low-risk action execution       | Supported |
| High-risk approval gate         | Supported |
| Approval pause/resume (restart) | Supported |
| Event-driven triggers (alerts)  | Supported |
| Forbid-list (irreversible ops)  | Supported |
| MCP agent tools                 | Supported |
| Episodic memory + recall        | Supported |
| Goal-decomposition planner      | Supported |
| Suggest-not-mutate learning     | Supported |
| Multi-turn chat (SendMessage)   | Supported |

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
  "approval_ttl_secs": 3600,
  "memory_enabled": true,
  "max_incidents": 200,
  "recall_top_k": 3
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
  "forbid_list": [],
  "memory_enabled": true,
  "max_incidents": 200,
  "recall_top_k": 3
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
| `memory_enabled`          | bool              | `true`    | Enable episodic memory recording and recall across sessions                            |
| `max_incidents`           | int               | `200`     | Maximum number of incident records retained in `agent_memory.json`                     |
| `recall_top_k`            | int               | `3`       | Number of past incidents injected as context at the start of each session              |

The API key is **never stored in the config file**. It must be provided via the
`UMA_AGENT_API_KEY` environment variable.

---

## Memory and recall

When `memory_enabled` is `true` (the default), the agent records a structured
incident for every finished session into
`/boot/config/plugins/unraid-management-agent/agent_memory.json`.

### What gets recorded

Each incident stores:

| Field       | Description                                      |
| ----------- | ------------------------------------------------ |
| `id`        | Incident identifier (`inc-<session id>`)         |
| `signature` | Short stable fingerprint of the goal (for dedup) |
| `goal`      | Original operator goal text                      |
| `outcome`   | `completed`, `failed`, or `cancelled`            |
| `summary`   | One-sentence summary of what happened            |
| `actions`   | List of tool names called during the session     |
| `at`        | When the session concluded                       |

### How recall works

At the start of each new session the recall engine:

1. Scores every stored incident against the new goal using keyword / tag overlap.
2. Selects the top-K incidents (controlled by `recall_top_k`, default `3`).
3. Injects the matching incidents plus all active operator preferences into a
   "memory context" block that is prepended to the first LLM system message.

This allows the agent to reference past remediations — for example, if a container
restart repeatedly fails the agent can note that it tried before and skip to a
deeper investigation step.

### Where memory is stored

| File                | Purpose                                            |
| ------------------- | -------------------------------------------------- |
| `agent_memory.json` | Episodic incidents (up to `max_incidents` entries) |

Inspect and clear via:

```bash
# View current memory via REST
curl -s http://<unraid-ip>:8043/api/v1/agent/memory | jq .

# Clear manually (daemon re-creates the file on next session)
rm /boot/config/plugins/unraid-management-agent/agent_memory.json
```

---

## Planner

For operator-initiated sessions (not event-driven wakes), the agent runs one
additional LLM call before the ReAct loop to decompose the goal into a short
ordered plan.

The plan is stored on the session as `sess.Plan` — a list of `{intent, tool}`
steps — and a plain-English summary is injected into the transcript as the first
assistant turn so subsequent reasoning steps can reference it.

The planner call is **best-effort**: if it fails (LLM error, timeout) the session
continues as normal without a plan. The extra LLM call consumes tokens from the
session budget.

---

## Learning (suggest-not-mutate)

The agent can propose new preferences and runbooks during a session, but it can
never activate them by itself. All proposals are `PENDING` until an operator
explicitly confirms them.

### How proposals work

During a session the agent may call:

| Tool                 | What it does                                                            |
| -------------------- | ----------------------------------------------------------------------- |
| `propose_preference` | Records a pending preference (e.g. `auto_approve_tool` for a tool name) |
| `propose_runbook`    | Records a proposed remediation runbook in `agent_runbooks.json`         |

Both tools are read-only from the system perspective — they only write to the
proposal store; they never change any configuration or execute any action.

### Confirming a preference

Pending preferences are confirmed via REST or MCP:

```bash
# List pending preferences
curl -s http://<unraid-ip>:8043/api/v1/agent/memory | jq '.preferences[] | select(.status=="pending")'

# Confirm a preference by ID
curl -s -X POST http://<unraid-ip>:8043/api/v1/agent/preferences/01J3XK.../confirm
```

Or via MCP tool `agent_confirm_preference`:

```json
{
  "name": "agent_confirm_preference",
  "arguments": { "preference_id": "01J3XK..." }
}
```

### Effect of a confirmed `auto_approve_tool` preference

When a preference with `kind = "auto_approve_tool"` is confirmed, the
policy gate treats calls to that tool (identified by `subject`) as if the tier
were `auto`, bypassing the pause-for-approval step.

> **Important:** the forbid-list always wins. A confirmed `auto_approve_tool`
> preference for a forbidden tool has no effect.

### Runbook proposals

Proposed runbooks are persisted to
`/boot/config/plugins/unraid-management-agent/agent_runbooks.json` alongside the
built-in static runbook catalogue. They can be inspected, edited, or deleted
manually. The `list_runbooks` MCP tool includes proposed runbooks in its output.

---

## Multi-turn chat

After a session finishes (status `completed` or `failed`), the operator can
continue the conversation by sending a follow-up message. The agent appends the
message to the existing transcript and re-runs the ReAct loop.

### Continuing a session via REST

```bash
curl -s -X POST http://<unraid-ip>:8043/api/v1/agent/sessions/01J3XKZP2V8N4DRHMGT5W6QA00/messages \
  -H "Content-Type: application/json" \
  -d '{"message": "Can you also check whether the disk temperatures are normal?"}'
```

**Example response (200 OK):**

```json
{
  "id": "01J3XKZP2V8N4DRHMGT5W6QA00",
  "goal": "Check whether any containers have exited and restart them.",
  "status": "completed",
  "iterations": 7,
  "total_tokens": 5840,
  "final_answer": "All containers are running. Disk temperatures are within normal range (max 38 °C).",
  "steps": ["..."]
}
```

The session `id` is unchanged — each `SendMessage` call adds turns to the same
session object. You can retrieve the full transcript at any time with
`GET /api/v1/agent/sessions/{id}`.

### Continuing a session via MCP

```json
{
  "name": "agent_send_message",
  "arguments": {
    "session_id": "01J3XKZP2V8N4DRHMGT5W6QA00",
    "message": "Can you also check disk temperatures?"
  }
}
```

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

### POST /api/v1/agent/sessions/{id}/messages

Continue a completed or failed session with a follow-up operator message. The
agent appends the message to the existing conversation history and re-runs the
ReAct loop. Returns the updated session on success, **400** on invalid request
or wrong session state, **503** when the agent is disabled.

**Request body:**

```json
{ "message": "Also check disk temperatures." }
```

**Example:**

```bash
curl -s -X POST http://<unraid-ip>:8043/api/v1/agent/sessions/01J3XKZP2V8N4DRHMGT5W6QA00/messages \
  -H "Content-Type: application/json" \
  -d '{"message": "Also check disk temperatures."}'
```

**Example response (200 OK):**

```json
{
  "id": "01J3XKZP2V8N4DRHMGT5W6QA00",
  "goal": "Check whether any containers have exited and restart them.",
  "status": "completed",
  "iterations": 7,
  "total_tokens": 5840,
  "final_answer": "All containers are running. Disk temperatures are within normal range.",
  "steps": ["..."]
}
```

---

### GET /api/v1/agent/memory

Return the agent's in-memory store: episodic incidents and learned preferences.
Always returns **200** (even when the memory store is empty). Returns **503**
when the agent is disabled.

**Example:**

```bash
curl -s http://<unraid-ip>:8043/api/v1/agent/memory | jq .
```

**Example response (200 OK):**

```json
{
  "incidents": [
    {
      "id": "inc-01J3XM...",
      "signature": "check-containers-restart",
      "goal": "Check whether any containers have exited and restart them.",
      "outcome": "completed",
      "summary": "Found plex container stopped; restarted successfully.",
      "actions": ["list_docker_containers", "restart_container"],
      "at": "2026-06-01T14:22:18Z"
    }
  ],
  "preferences": [
    {
      "id": "pref-1",
      "kind": "auto_approve_tool",
      "subject": "restart_container",
      "status": "active",
      "note": "",
      "at": "2026-06-01T15:00:00Z"
    }
  ]
}
```

---

### POST /api/v1/agent/preferences/{id}/confirm

Activate a pending learned preference by its ID. Idempotent — confirming an
already-confirmed preference is a no-op. Returns **400** if the preference is
not found or already in a terminal state, **503** when the agent is disabled.

**Example:**

```bash
curl -s -X POST http://<unraid-ip>:8043/api/v1/agent/preferences/01J3XN.../confirm
```

**Example response (200 OK):**

```json
{
  "success": true,
  "message": "preference confirmed",
  "timestamp": "2026-06-01T15:00:00Z"
}
```

---

## MCP tools

The following MCP tools are exposed under the `agent_*` namespace for external AI
agents and MCP clients:

| Tool                       | Description                                                       |
| -------------------------- | ----------------------------------------------------------------- |
| `agent_start_session`      | Start a new agent session with a natural-language goal            |
| `agent_get_session`        | Retrieve a session by ID (includes full step transcript)          |
| `agent_list_sessions`      | List all persisted sessions, newest first                         |
| `agent_approve_action`     | Approve or deny a pending high-risk action in an awaiting session |
| `agent_send_message`       | Continue a finished session with a follow-up operator message     |
| `agent_get_memory`         | Retrieve the agent's episodic incidents and learned preferences   |
| `agent_confirm_preference` | Activate a pending learned preference by ID                       |

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

## Future work

The planned three-phase roadmap for the Agent Core is now complete. Potential future
enhancements include:

- **Embedding-based recall:** replace keyword / tag matching with vector similarity
  search for more accurate incident retrieval on large memory stores.
- **GUI page:** a dedicated Unraid UI page for viewing sessions, memory, and
  confirming proposals without using the REST API or a terminal.
- **Native notification approvals:** allow high-risk approval requests to be resolved
  directly from an Unraid notification toast or mobile push notification.

---

## Related documentation

- [MCP Integration](mcp.md)
- [REST API Reference](../api/API_REFERENCE.md)
- [WebSocket Events](../websocket/WEBSOCKET_EVENTS_DOCUMENTATION.md)
