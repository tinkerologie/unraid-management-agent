# Design Proposal: Zero-Config MCP Onboarding (`ha-mcp` Style) & Streamable HTTP `GET /mcp` Fix

- **Tracking Issues:** [#165](https://github.com/ruaan-deysel/unraid-management-agent/issues/165), [#166](https://github.com/ruaan-deysel/unraid-management-agent/issues/166)
- **Status:** Implemented

---

## 1. Executive Summary

Unraid Management Agent (UMA) ships a complete embedded Model Context Protocol (MCP) server inside the single Go daemon binary (`POST /mcp` and `GET /mcp`, `modelcontextprotocol/go-sdk`, 132 tools, 5 resources, 6 prompts). Unlike `homeassistant-ai/ha-mcp`—which runs as an external Python bridge against Home Assistant's REST/WebSocket API—UMA **already runs natively on the Unraid server** and starts automatically with the plugin.

This design adopts the best onboarding ergonomics from `ha-mcp` while eliminating external sidecars:

1. **Single-URL Secret Path (`/mcp/<connect-secret>`)**: Follows `ha-mcp`'s private path-based authentication (`MCP_SECRET_PATH` in `ha-mcp`, `MCP_CONNECT_SECRET` in UMA) so clients that only accept a single URL (Claude.ai connectors, Cursor UI, VS Code UI, Claude Desktop via `mcp-remote`) can authenticate without custom `Authorization` headers.
2. **One-Click WebGUI Snippet Generator**: Generates a cryptographically random 256-bit (43-character `base64url`) secret directly in **Settings -> Utilities -> Unraid Management Agent** and renders copy-ready configs for Claude Code, VS Code, Cursor, OpenAI Codex CLI, Gemini CLI, and Claude Desktop.
3. **Streamable HTTP `GET /mcp` Compliance (#166)**:
   - Sessionless `GET /mcp` immediately returns a framed `405 Method Not Allowed` with `Allow: POST, DELETE` and `Content-Length` so clients probing for SSE or standalone streams fall back to `POST /mcp` in `< 5 ms` instead of hanging until `ReadTimeout`/`WriteTimeout` (30 s).
   - Valid-session `GET /mcp` (with `Mcp-Session-Id`) clears per-request read/write deadlines via `http.ResponseController` and flushes the initial `: ok\n\n` SSE frame immediately through `statusRecorder.Flush()` / `Unwrap()`.
4. **LAN Discovery & Connection Diagnostics**:
   - Extends `_unraid-agent._tcp` mDNS TXT metadata with `mcp_path=/mcp`, `mcp_transport=streamable-http`, and `mcp_auth=bearer|none` (never broadcasting secrets).
   - Adds structured connection lifecycle logging (with `/mcp/<redacted>` path masking) and the `mcp_connection_events_total{event}` Prometheus counter.

---

## 2. Onboarding Baseline vs. Target Comparison

| Metric | Before (Manual Setup) | After (`ha-mcp` Style Onboarding) |
| --- | --- | --- |
| **User steps to connect** | 5–6 steps (look up LAN IP, verify port `8043`, generate/copy `API_TOKEN`, look up client-specific JSON/CLI syntax, manually craft `Authorization: Bearer` header, test connection) | **2 steps** (click **Generate** -> click **Copy Snippet** in the Unraid WebGUI, then paste/run in the MCP client) |
| **User-supplied fields** | 3 (`host`, `port`, `API_TOKEN` + header syntax) | **0** (all pre-filled from active server bind IP, TLS scheme, port, and generated secret) |
| **Time to first `tools/list`** | ~3–5 minutes | **< 20 seconds** |
| **Clients without custom header UI** | Required `npx mcp-remote --header ...` workaround | Native single-URL connection via `http(s)://<host>:<port>/mcp/<connect-secret>` |
| **`GET /mcp` probe latency (#166)** | 30,000 ms (hung until server `WriteTimeout` or client timeout) | **< 5 ms** (`405 Method Not Allowed` with `Allow: POST, DELETE` or immediate `: ok` SSE flush with session) |

---

## 3. Architecture & Changes Across Subsystems

### 3.1 Daemon (`daemon/`)

- **Configuration (`domain/config.go`, `domain/fileconfig.go`, `main.go`)**:
  - Added `MCPConnectSecret` (`--mcp-connect-secret` / `MCP_CONNECT_SECRET` / `mcp_connect_secret` in `config.yml`).
  - Validated at startup via `lib.ValidateMCPConnectSecret`: requires 32–256 characters matching `^[A-Za-z0-9_-]+$`. Invalid values log a warning and disable the secret path without failing daemon startup.
- **Routing & Auth Middleware (`services/api/server.go`, `services/api/middleware.go`)**:
  - `RegisterMCPRoutes` registers exact routes `/mcp`, `/mcp/`, and (when valid) `/mcp/<secret>`.
  - `authMiddlewareWithMCPSecret` allows `/mcp/<secret>` via constant-time SHA-256 digest comparison (`subtle.ConstantTimeCompare`) even when `API_TOKEN` is enabled, while plain `/mcp` continues to require `Authorization: Bearer <API_TOKEN>`.
  - Any unconfigured or non-matching `/mcp/<other>` path returns `404 Not Found` and increments `mcp_connection_events_total{event="not_found"}`.
  - `redactMCPPath` masks any `/mcp/<segment>` path to `/mcp/<redacted>` across all HTTP request logs and warnings so secrets never leak to `/var/log/unraid-management-agent.log` or diagnostics bundles.
- **Streamable HTTP Transport (`services/mcp/server.go`, `services/api/middleware.go`)**:
  - `statusRecorder` implements `http.Flusher` and `Unwrap() http.ResponseWriter` so `http.NewResponseController(w)` can reach the underlying connection.
  - `GetHTTPHandler()` wraps the SDK `StreamableHTTPHandler`:
    - `GET` without `Mcp-Session-Id` returns HTTP `405 Method Not Allowed` (`Allow: POST, DELETE`, `Content-Length`, and `Connection: keep-alive` on HTTP/1.x).
    - `GET` with `Mcp-Session-Id` clears `ReadDeadline` and `WriteDeadline` via `http.NewResponseController(w)` so long-lived server-to-client SSE notification streams are not terminated after 30 seconds.
- **Discovery (`services/discovery/service.go`)**:
  - Advertises `mcp_path=/mcp`, `mcp_transport=streamable-http`, and `mcp_auth=bearer|none` in `_unraid-agent._tcp` mDNS TXT records.
- **Observability (`services/api/metrics.go`, `services/mcp/server.go`)**:
  - Exports `mcp_connection_events_total{event="initialize_ok|initialize_error|auth_rejected|origin_rejected|not_found|tools_list_ok"}` at `/metrics`.

### 3.2 Unraid Plugin UI (`meta/plugin/`)

- **`meta/plugin/include/update.php`, `scripts/apply`, `scripts/start`**:
  - Persists, single-quotes, and clears `MCP_CONNECT_SECRET` in `/boot/config/plugins/unraid-management-agent/config.cfg` and exports it to the daemon.
- **`meta/plugin/unraid-management-agent.page`**:
  - Derives `http`/`https` (`ws`/`wss`) from `TLS_CERT_FILE` and `TLS_KEY_FILE`.
  - Adds **Generate / Regenerate / Show / Hide / Copy Connect URL / Clear** controls and live client snippet generator for Claude Code, VS Code, Cursor, Codex CLI, Gemini CLI, and Claude Desktop.

---

## 4. Decisions on Open Questions (#165)

1. **Does an Unraid server on a typical home LAN have HTTPS / DNS needed for Streamable HTTP clients, or do some clients refuse plain `http://`?**
   - **Decision**: Local desktop and CLI clients (Claude Code, VS Code, Cursor, Codex CLI, Gemini CLI, and `mcp-remote`) allow `http://<lan-ip>:8043/mcp` on RFC1918 LAN addresses and `.local` mDNS hostnames. However, cloud-hosted connectors (`claude.ai` web connectors and ChatGPT web connectors) require a publicly reachable HTTPS endpoint. The WebGUI and documentation explicitly distinguish local LAN HTTP/HTTPS usage from cloud connectors (which require Cloudflare Tunnel or Tailscale Funnel).
2. **How should authentication work if UMA has bearer/API key auth enabled?**
   - **Decision**: Adopt `ha-mcp`'s private path secret pattern (`/mcp/<MCP_CONNECT_SECRET>`) alongside standard `Authorization: Bearer <API_TOKEN>` on `/mcp`. Users with clients that support headers can use `Bearer <API_TOKEN>` on `/mcp`; users with URL-only clients can use `/mcp/<MCP_CONNECT_SECRET>`. Both can be active simultaneously, and rotating `MCP_CONNECT_SECRET` (followed by clicking **Apply** to restart the daemon) invalidates the old URL (`404 Not Found`).
3. **Should UMA ship an optional thin `npx unraid-mcp` launcher?**
   - **Decision**: Not required as a separate npm package to maintain. Every modern MCP client either supports Streamable HTTP natively (Claude Code, VS Code, Cursor, Codex CLI, Gemini CLI) or supports the standard `npx -y mcp-remote <url>` bridge (Claude Desktop `claude_desktop_config.json`), which the WebGUI generates out-of-the-box.
4. **Which MCP spec version (`2024-11-05` vs `2025-03-26` vs `2025-06-18`) does `daemon/services/mcp/` advertise, and does it match Tier-1 clients?**
   - **Decision**: UMA uses `github.com/modelcontextprotocol/go-sdk` v1.8.0, which negotiates protocol versions per session across `2026-07-28`, `2025-11-25`, `2025-06-18`, `2025-03-26`, and `2024-11-05`. Startup logs and Ansible verification tests explicitly verify per-session protocol negotiation.
5. **Can we emit a one-click Setup Wizard card in the Unraid WebGUI under Settings -> Management Agent?**
   - **Decision**: Yes—implemented directly inside `meta/plugin/unraid-management-agent.page` under **AI Agent Access (MCP)** (`#uma-mcp-onboarding`), with instant client snippet generation and one-click copy buttons.
