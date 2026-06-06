# Diagnostic Prompts & Real-Time Resources

The agent ships **6 MCP prompts** and **5 MCP resources** alongside its tools.

## Diagnostic Prompts

Prompts orchestrate the right tools for a common investigation. Prefer them over
hand-assembling tool chains — they encode the expert workflow.

| Prompt | What it analyses |
| --- | --- |
| `diagnose_disk_health` | Walks SMART data, temperatures, error rates, and power-on hours to produce a plain-English health verdict per disk |
| `diagnose_performance_issue` | Correlates CPU, RAM, Docker resource usage, and VM count to identify performance bottlenecks |
| `suggest_maintenance` | Reviews parity history, disk ages, array errors, and temperatures to generate a prioritized maintenance checklist |
| `explain_array_state` | Translates raw array status into human-readable context with recommended actions |
| `system_overview` | Produces a comprehensive overview of overall system status |
| `troubleshoot_issue` | Guides troubleshooting of common Unraid issues |

**When to use which:**

- "Is my disk failing?" / "Check disk health" → `diagnose_disk_health`
- "Why is my server slow?" → `diagnose_performance_issue`
- "What maintenance should I do?" → `suggest_maintenance`
- "Why won't my array start?" / "What does this array state mean?" → `explain_array_state`
- "Give me a status report" → `system_overview`
- "Something's wrong, help me debug" → `troubleshoot_issue`

## Real-Time Resources

MCP resources expose the agent's live cache as readable resources. Read these
(or the WebSocket stream) instead of polling REST for changing values.

| Resource | Content |
| --- | --- |
| `system-info` | Real-time Unraid system information |
| `array-status` | Real-time array status |
| `docker-containers` | Real-time container list and status |
| `virtual-machines` | Real-time VM list and status |
| `disk-status` | Real-time disk information and health |

A resource read returns the same cached value a corresponding `get_*` / `list_*`
tool would; use whichever your client supports best.
