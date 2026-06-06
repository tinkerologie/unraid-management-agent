# Unraid Management Agent — Agent Skills

An **Agent Skill** is a portable Markdown knowledge pack that teaches AI coding
agents best practices for a specific technology, following the open
[Agent Skills standard](https://agentskills.io/specification).

This directory provides a skill for the **Unraid Management Agent** — teaching
agents how to monitor and control an Unraid server through the agent's MCP
server (121 tools, 5 resources, 6 diagnostic prompts) and REST API.

## Included Skill

**[`unraid-management-agent`](unraid-management-agent/):** connection setup,
the full MCP tool catalog with read/write (destructive) flags, the diagnostic
prompts and real-time resources, the REST API surface for non-MCP clients, and
request → tool workflows.

| File                               | Purpose                                                    |
| ---------------------------------- | ---------------------------------------------------------- |
| `unraid-management-agent/SKILL.md` | Decision workflow, anti-patterns, routing                  |
| `references/connection.md`         | Connect any client via MCP HTTP/STDIO; troubleshooting     |
| `references/mcp-tools.md`          | All 121 MCP tools by category (R/W + ⚠️ destructive flags) |
| `references/diagnostics.md`        | 6 diagnostic prompts + 5 real-time resources               |
| `references/rest-api.md`           | REST API surface for non-MCP clients                       |
| `references/workflows.md`          | Natural-language request → tool/endpoint mappings          |

## Installation

### Agent Skills installer (Claude Code, Cursor, Copilot, VS Code, Gemini CLI)

Requires [Node.js 18+](https://nodejs.org/).

```bash
npx skills add ruaan-deysel/unraid-management-agent
```

To update later: `npx skills update`

### Claude Code plugin

Run each command separately inside Claude Code:

```text
/plugin marketplace add ruaan-deysel/unraid-management-agent
```

```text
/plugin install unraid-management-agent-skills@unraid-management-agent-skills
```

Run `/reload-plugins` or restart Claude Code for the skill to take effect.

### Claude Desktop / claude.ai

1. Clone or download this repository.
2. Zip the skill folder:

   ```bash
   cd skills && zip -r unraid-management-agent.zip unraid-management-agent/
   ```

3. Upload the `.zip`:
   - **Claude Desktop:** Settings → Capabilities → Skills → Upload skill
   - **claude.ai:** [claude.ai/customize/skills](https://claude.ai/customize/skills) → Upload

## Connect the agent itself

The skill teaches agents _how to use_ the Unraid Management Agent — you still
need the agent running on your Unraid server and your AI client pointed at it.
See [`unraid-management-agent/references/connection.md`](unraid-management-agent/references/connection.md)
and the [MCP integration guide](../docs/integrations/mcp.md).

## ChatGPT

ChatGPT consumes REST **Actions**, not Agent Skills. Use the OpenAPI schema and
guide under [`docs/integrations/chatgpt/`](../docs/integrations/chatgpt/).

## License

MIT — see the repository [LICENSE](../LICENSE).
