# AI & Automation Integrations

Ways to connect the Unraid Management Agent to AI assistants and automation
platforms. Pick by client.

## Integration methods

| Method                                                      | Best for                            | Provides                    | Setup                          |
| ----------------------------------------------------------- | ----------------------------------- | --------------------------- | ------------------------------ |
| **MCP** ([mcp.md](mcp.md))                                  | Claude, Cursor, Copilot, Gemini CLI | 121 tools, 5 res, 6 prompts | `http://<ip>:8043/mcp`         |
| **Agent Skill** ([claude/](claude/))                        | Claude, Cursor, Copilot, Gemini     | How-to knowledge pack       | `npx skills add …` / `/plugin` |
| **ChatGPT Actions** ([chatgpt/](chatgpt/))                  | ChatGPT Custom GPTs                 | ~30 REST endpoints          | Import `openapi-actions.yaml`  |
| **MQTT** ([mqtt.md](mqtt.md))                               | Home Assistant entities             | State topics + discovery    | Enable MQTT → broker           |
| **Home Assistant** ([home-assistant.md](home-assistant.md)) | HA dashboards/control               | REST + WebSocket            | Install HA integration         |
| **Grafana** ([grafana.md](grafana.md))                      | Dashboards, metrics                 | `/metrics` (Prometheus)     | Scrape `/metrics`              |

## Choosing

- **AI agent that speaks MCP (Claude, Cursor, Copilot, Gemini):** use the **MCP**
  connection for live tools, and install the **Agent Skill** so the agent knows
  how to use them well.
- **ChatGPT:** use **ChatGPT Actions** (it does not consume MCP/Skills).
- **Home Assistant:** use the **MQTT** integration and/or the dedicated HA
  integration. (The agent also advertises itself via mDNS as
  `_unraid-mgmt-agent._tcp.local.` for auto-discovery.)
- **Metrics/dashboards:** use **Prometheus/Grafana**.

## Real-time vs polling

- **MCP / WebSocket / MQTT:** push or cached — near real-time, low overhead.
- **REST / Prometheus:** request/scrape — fine for scripts and dashboards; don't
  hot-loop the REST API when a push channel is available.

## See also

- [Configuration guide](../guides/configuration.md) — flags, env vars, YAML config
- [REST API reference](../api/rest-api.md) — full HTTP surface
