# Claude Integration

Two ways to give Claude knowledge of and control over your Unraid server:

1. **Agent Skill** — a portable knowledge pack that teaches Claude how to use the
   agent's tools effectively. Works in Claude Code, Claude Desktop, and claude.ai.
2. **MCP connection** — the live tool/data connection to your running server.

You typically want **both**: the skill teaches _how_, the MCP connection provides
the _live tools_.

## 1. Install the Agent Skill

The skill lives at [`skills/unraid-management-agent/`](../../../skills/unraid-management-agent/)
and follows the open [Agent Skills standard](https://agentskills.io). Full
install instructions (npx installer, Claude Code plugin, claude.ai zip upload)
are in [`skills/README.md`](../../../skills/README.md).

Quick reference:

```bash
# Claude Code / Cursor / Copilot / Gemini CLI
npx skills add ruaan-deysel/unraid-management-agent
```

```text
# Claude Code plugin
/plugin marketplace add ruaan-deysel/unraid-management-agent
/plugin install unraid-management-agent-skills@unraid-management-agent-skills
```

For **claude.ai / Claude Desktop**, zip the skill folder and upload it under
Settings → Capabilities → Skills (see `skills/README.md`).

## 2. Connect Claude to your server (MCP)

- **Claude Code:**

  ```bash
  claude mcp add --transport http unraid http://<unraid-ip>:8043/mcp
  ```

- **Claude Desktop / claude.ai:** add a custom MCP connector with URL
  `http://<unraid-ip>:8043/mcp`.
- **Local (on the Unraid box):** use STDIO — `unraid-management-agent mcp-stdio`.

Details and per-client config: [`../mcp.md`](../mcp.md) and the skill's
[`references/connection.md`](../../../skills/unraid-management-agent/references/connection.md).

## Claude Projects

For a Claude Project, upload the skill's reference files as project knowledge so
the assistant has the tool catalog and workflows in context:

- `skills/unraid-management-agent/SKILL.md`
- `skills/unraid-management-agent/references/*.md`

Then add the MCP connector (above) so the project can actually call the tools.

## Troubleshooting

See the skill's [`references/connection.md`](../../../skills/unraid-management-agent/references/connection.md#troubleshooting)
for connection issues. The agent has no auth by default — keep it on a trusted
LAN or behind a VPN/reverse proxy.
