# ChatGPT Custom GPT Integration

Import the agent's REST API as an **Action** to let a custom GPT monitor or control your Unraid server.

> ChatGPT consumes REST **Actions** (OpenAPI), not the Agent Skills / MCP that
> Claude Code uses. For Claude and other MCP clients, see
> [`../claude/`](../claude/) and [`../mcp.md`](../mcp.md).

## Prerequisites

- The Unraid Management Agent plugin running on your Unraid server.
- A ChatGPT plan that supports building **Custom GPTs** (GPT Builder).
- An HTTPS URL that ChatGPT can reach. A reverse proxy or tunnel can supply the URL.
- An agent API token. Set the token before making the endpoint reachable to external clients.
  See [Authentication](../../guides/configuration.md#authentication).

## Steps

1. **ChatGPT → Explore GPTs → Create → Configure → Create new action.**
2. **Import the schema.** Paste the contents of
   [`openapi-actions.yaml`](openapi-actions.yaml), or host it and use "Import from URL".
3. **Set the server URL.** Edit the `servers[0].url` in the schema to your
   reachable address, e.g. `https://unraid.example.com/api/v1`.
4. In the GPT editor, set **Authentication Type** to **API Key**.
5. Set **Auth Type** to **Bearer**.
6. Enter the raw agent token in **API Key**. The action sends `Authorization: Bearer <token>`.
   See [GPT Action authentication](https://developers.openai.com/api/docs/actions/authentication).
7. **Save** the action. ChatGPT will list the imported operations
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
- Before you control a container, list containers to get its 12- or 64-character hexadecimal ID.
- Before you control a VM, list VMs to get its name.
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

| Symptom                           | Fix                                                                       |
| --------------------------------- | ------------------------------------------------------------------------- |
| "Action not found" / not callable | Re-import the schema; ensure each `operationId` is unique                 |
| Network/timeout errors            | ChatGPT can't reach a LAN IP — use a public HTTPS reverse proxy or tunnel |
| CORS errors                       | Check the origin rules at the reverse proxy                               |
| Calls succeed but do nothing      | Check the `servers[0].url` includes the `/api/v1` base path               |
| GPT acts without asking           | Confirm the operation has `x-openai-isConsequential: true` in the schema  |

## Notes

This schema is a curated subset (~30 endpoints) for reliable action selection.
The agent serves its generated REST reference at `http://<your-unraid-ip>:8043/swagger/`;
add more operations to the schema as needed, keeping `operationId`s unique.
