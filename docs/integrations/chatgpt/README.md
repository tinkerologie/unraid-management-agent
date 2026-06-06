# ChatGPT Custom GPT Integration

Give ChatGPT the ability to monitor and control your Unraid server by importing
the Unraid Management Agent's REST API as a Custom GPT **Action**.

> ChatGPT consumes REST **Actions** (OpenAPI), not the Agent Skills / MCP that
> Claude Code uses. For Claude and other MCP clients, see
> [`../claude/`](../claude/) and [`../mcp.md`](../mcp.md).

## Prerequisites

- The Unraid Management Agent plugin running on your Unraid server.
- A ChatGPT plan that supports building **Custom GPTs** (GPT Builder).
- Network reachability from ChatGPT to your server. The agent has **no
  authentication** and listens on HTTP, so the practical options are:
  - A reverse proxy exposing it over HTTPS on your domain (recommended), or
  - A tunnel (Cloudflare Tunnel, Tailscale Funnel) to reach the LAN service.
  - ChatGPT Actions cannot reach a private `192.168.x.x` address directly.

## Steps

1. **ChatGPT → Explore GPTs → Create → Configure → Create new action.**
2. **Import the schema.** Paste the contents of
   [`openapi-actions.yaml`](openapi-actions.yaml), or host it and use "Import from URL".
3. **Set the server URL.** Edit the `servers[0].url` in the schema to your
   reachable address, e.g. `https://unraid.example.com/api/v1`.
4. **Authentication:** choose **None** for a trusted/tunnelled deployment. If you
   put the agent behind a reverse proxy that adds auth (e.g. an API key header or
   basic auth), configure that here instead.
5. **Save** the action. ChatGPT will list the imported operations
   (`getSystemInfo`, `listContainers`, `startContainer`, …).

## Recommended GPT instructions

Paste into the GPT's **Instructions** field:

```text
You manage an Unraid server via the connected Actions.
- For status questions, call the relevant GET action (getSystemInfo, getArrayStatus,
  listContainers, listVMs, getGpuMetrics, getUpsStatus, getParityHistory, etc.).
- Before any state-changing action (start/stop/restart, update, reboot, shutdown,
  stop array, execute script), summarize exactly what will happen and ask the user
  to confirm. These are marked consequential and ChatGPT will also prompt.
- To act on a container or VM, first list/get it to resolve the correct id/name.
- Never call stopArray, rebootSystem, shutdownSystem, or executeUserScript without
  explicit user confirmation in the conversation.
- Report results concisely; surface errors verbatim.
```

## Example conversations

- "How's my server doing?" → `getSystemInfo` + `getHealthReport`
- "Restart the Plex container." → `listContainers` → confirm → `restartContainer`
- "Is a parity check overdue?" → `getParityHistory`
- "Stop the array." → GPT confirms (consequential) → `stopArray`

## Troubleshooting

| Symptom                           | Fix                                                                            |
| --------------------------------- | ------------------------------------------------------------------------------ |
| "Action not found" / not callable | Re-import the schema; ensure each `operationId` is unique                      |
| Network/timeout errors            | ChatGPT can't reach a LAN IP — use a public HTTPS reverse proxy or tunnel      |
| CORS errors                       | Set `--cors-origin` / `CORS_ORIGIN` on the agent (or handle CORS at the proxy) |
| Calls succeed but do nothing      | Check the `servers[0].url` includes the `/api/v1` base path                    |
| GPT acts without asking           | Confirm the operation has `x-openai-isConsequential: true` in the schema       |

## Notes

This schema is a curated subset (~30 endpoints) for reliable action selection.
The complete REST API (148 paths) is at `http://<your-unraid-ip>:8043/swagger/`;
add more operations to the schema as needed, keeping `operationId`s unique.
