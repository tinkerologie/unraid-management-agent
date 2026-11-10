# ChatGPT Custom GPT Integration

Import the agent's REST API as an **Action** to let a custom GPT monitor or control your Unraid server.

> To connect a ChatGPT custom MCP server, use the
> [stateless MCP endpoint through Secure MCP Tunnel](../mcp.md#chatgpt-through-secure-mcp-tunnel).

## Prerequisites

- The Unraid Management Agent plugin running on your Unraid server.
- A ChatGPT plan that supports building **Custom GPTs** (GPT Builder).
- An HTTPS URL that ChatGPT can reach. A reverse proxy or tunnel can supply the URL.
- An agent API token. Set the token before making the endpoint reachable to external clients.
  See [Authentication](../../guides/configuration.md#authentication).

## Steps

1. In ChatGPT, open **Explore GPTs → Create → Configure → Create new action**.
2. Paste the contents of [`openapi-actions.yaml`](openapi-actions.yaml) into the schema field.
3. Set `servers[0].url` to your HTTPS address, for example `https://unraid.example.com/api/v1`.
4. Set **Authentication Type** to **API Key**.
5. Set **Auth Type** to **Bearer**.
6. Enter the agent token in **API Key**.
7. Save the action.

ChatGPT lists the imported operations, such as `getSystemInfo` and `listContainers`.
The action sends the token in an `Authorization: Bearer <token>` header.
See [GPT Action authentication](https://developers.openai.com/api/docs/actions/authentication).

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
| "Action not found" / not callable | Check for duplicate `operationId` values. Then import the schema again.   |
| Network/timeout errors            | ChatGPT can't reach a LAN IP — use a public HTTPS reverse proxy or tunnel |
| CORS errors                       | Check the origin rules at the reverse proxy                               |
| Calls succeed but do nothing      | Check the `servers[0].url` includes the `/api/v1` base path               |
| GPT acts without asking           | Confirm the operation has `x-openai-isConsequential: true` in the schema  |

## Notes

The Actions schema contains a subset of the REST operations.
The agent serves the full REST reference at `http://<your-unraid-ip>:8043/swagger/`.

If you add an operation, give it a unique `operationId`.
