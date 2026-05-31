# Embedded Agent Core

> **Status: Beta (Phase 1)** — Disabled by default. Opt-in required.
> Phase 1 covers synchronous goal execution, read-only and low-risk actions,
> and a bounded ReAct reasoning loop. High-risk approval workflows and
> event-driven triggers are planned for later phases.

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

### Phase 1 scope

| Capability                      | Status            |
| ------------------------------- | ----------------- |
| Read-only tool execution        | Supported         |
| Low-risk action execution       | Supported         |
| High-risk action execution      | Refused (Phase 2) |
| Approval / pause-resume flow    | Phase 2           |
| Event-driven triggers (alerts)  | Phase 2           |
| Memory / cross-session learning | Phase 3           |
| MCP agent tools                 | Phase 3           |

### Safety model

Tools are assigned one of three **risk tiers**. The tier determines whether the agent
can execute a tool automatically or must refuse it:

| Tier        | Examples                                                        | Agent behaviour                                        |
| ----------- | --------------------------------------------------------------- | ------------------------------------------------------ |
| `read_only` | `get_system_info`, `get_array_status`, `list_docker_containers` | Auto-executes                                          |
| `low`       | `restart_container`                                             | Auto-executes                                          |
| `high`      | Array start/stop, system reboot                                 | Refused — returns error (approval workflow in Phase 2) |

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
    "high": "forbid"
  },
  "max_iterations": 12,
  "max_tokens_per_session": 60000,
  "session_deadline_secs": 180
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
    "high": "forbid"
  },
  "max_iterations": 12,
  "max_tokens_per_session": 60000,
  "session_deadline_secs": 180
}
```

| Field                    | Type              | Default   | Description                                                                   |
| ------------------------ | ----------------- | --------- | ----------------------------------------------------------------------------- |
| `enabled`                | bool              | `false`   | Master switch — set to `true` to activate the agent                           |
| `provider`               | string            | —         | LLM provider identifier. Currently only `"anthropic"` is supported            |
| `model`                  | string            | —         | Model name passed to the provider (e.g. `"claude-opus-4-8"`)                  |
| `endpoint`               | string            | `""`      | Optional base URL override for the provider API (leave empty for the default) |
| `autonomy`               | map[string]string | see above | Per-risk-tier behaviour: `"auto"`, `"approve"`, or `"forbid"`                 |
| `max_iterations`         | int               | `12`      | Hard cap on ReAct loop iterations per session                                 |
| `max_tokens_per_session` | int               | `60000`   | Cumulative token budget across all LLM calls in a session                     |
| `session_deadline_secs`  | int               | `180`     | Wall-clock time limit for a session (seconds)                                 |

The API key is **never stored in the config file**. It must be provided via the
`UMA_AGENT_API_KEY` environment variable.

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
curl -s -X POST http://192.168.20.21:8043/api/v1/agent/sessions \
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
curl -s http://192.168.20.21:8043/api/v1/agent/sessions
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
curl -s http://192.168.20.21:8043/api/v1/agent/sessions/01J3XKZP2V8N4DRHMGT5W6QA00
```

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

Connect to the WebSocket endpoint and filter by `event` prefix `agent_` to stream
live agent reasoning to a UI or monitoring tool:

```bash
# Using websocat (https://github.com/vi/websocat)
websocat ws://192.168.20.21:8043/ws | grep '"agent_'
```

---

## Sessions persistence

Completed sessions are persisted to
`/boot/config/plugins/unraid-management-agent/agent_sessions.json`. The file is
read on daemon startup so sessions survive restarts. There is currently no automatic
pruning — remove or truncate the file manually if it grows too large.

---

## Coming in later phases

- **Phase 2 — High-risk approval workflow:** The agent pauses before executing
  high-risk tools and publishes an `agent_awaiting_approval` WebSocket event.
  A REST endpoint (`POST /api/v1/agent/sessions/{id}/approve`) resumes the session.
- **Phase 2 — Event-driven triggers:** Firing alerts can automatically wake the
  agent with a pre-configured goal (e.g. auto-diagnose when a disk error alert fires).
- **Phase 3 — Memory and learning:** The agent retains facts across sessions so it
  can learn server-specific context over time.
- **Phase 3 — MCP agent tools:** External MCP clients will be able to invoke the
  embedded agent as a named tool, composing it with other MCP capabilities.

---

## Related documentation

- [MCP Integration](mcp.md)
- [REST API Reference](../api/API_REFERENCE.md)
- [WebSocket Events](../websocket/WEBSOCKET_EVENTS_DOCUMENTATION.md)
