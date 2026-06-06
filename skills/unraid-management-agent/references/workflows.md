# Workflows & Request → Tool Mapping

Map natural-language requests to the right MCP tool (or REST path). MCP tool
names shown; REST equivalents in parentheses where relevant.

## Monitoring

| User asks | Do this |
| --- | --- |
| "How's the server doing?" | `get_diagnostic_summary` or the `system_overview` prompt |
| "What's CPU/RAM right now?" | `get_system_info` |
| "Is the array healthy?" | `get_array_status`, then `explain_array_state` if confusing |
| "Show me my disks / SMART" | `list_disks`, then `get_disk_info` for one |
| "Which containers are running?" | `list_containers` (or `search_containers` to filter) |
| "Logs for a container" | `get_container_logs` |
| "How are my VMs?" | `list_vms` / `get_vm_info` |
| "UPS / GPU / ZFS status" | `get_ups_status` / `get_gpu_metrics` / `get_zfs_pools` |
| "Any alerts firing?" | `get_firing_alerts` |
| "How do I reach this server?" | `get_network_access_urls` |

## Container control

```
1. search_containers(query="plex")        # find it
2. get_container_info(...)                 # confirm current state
3. container_action(action="restart", ...) # act
```

`container_action` accepts: `start`, `stop`, `restart`, `pause`, `unpause`.

## VM control

```
1. search_vms(query="windows")
2. vm_action(action="start", ...)          # start/stop/restart/pause/resume/hibernate/force-stop
```

To protect a VM before risky changes: `create_vm_snapshot` → make changes →
`restore_vm_snapshot` (⚠️ confirm) if you need to roll back.

## Array & parity

- Start array: `array_action(action="start")`
- **Stop array** (⚠️ confirm — data goes offline): `array_action(action="stop", confirm=true)`
- Run a parity check: `parity_check_action`; pause/resume/stop with the matching tools.
- "When did parity last run?" → `get_parity_history`.

## Maintenance & diagnostics

| Goal | Tool/prompt |
| --- | --- |
| Health verdict per disk | `diagnose_disk_health` prompt |
| Find why it's slow | `diagnose_performance_issue` prompt |
| Maintenance checklist | `suggest_maintenance` prompt |
| Update containers | `check_container_updates` → `update_container` (one) or `update_all_containers` ⚠️ |
| Update plugins | `check_plugin_updates` → `update_plugin` / `update_all_plugins` ⚠️ |
| Reboot/shutdown | `system_reboot` / `system_shutdown` (⚠️ confirm) |

## Observability setup

- Create an alert: `create_alert_rule` with an expr-lang expression (e.g.
  `cpu_usage > 90`), or enable a curated one via `enable_alert_template`.
- Add a probe: `create_health_check` (HTTP/TCP/container), then `run_health_check`
  to test it.

## Power & tuning

- Quieter fans: `set_fan_profile(profile="quiet", ...)`; revert with `restore_fan_defaults`.
- Power saving: `set_cpu_governor(governor="powersave")`, `set_turbo_boost(enabled=false)`.
- Spin disks down to save power: `disk_spin_down`.

## Golden rules

1. Read before write.
2. Confirm anything ⚠️.
3. Prefer the narrowest tool.
4. Use prompts for analysis.
5. Never invent tool names — check `mcp-tools.md`.
