# REST API (for non-MCP clients)

For clients that cannot speak MCP (ChatGPT Custom GPTs, shell scripts, other
integrations). MCP-capable agents should prefer the tools in `mcp-tools.md`.

- **Base path:** `http://<unraid-ip>:8043/api/v1`
- **Auth:** none by default (trusted LAN / VPN).
- **Full spec:** `http://<unraid-ip>:8043/swagger/` (148 documented paths). The
  curated subset for ChatGPT Actions is in `docs/integrations/chatgpt/openapi-actions.yaml`.
- **Conventions:** Docker endpoints use the container id/name as `{id}`; VM
  endpoints use the VM name as `{name}`. Control endpoints are `POST`.

## Monitoring (GET)

| Path | Returns |
| --- | --- |
| `/health` | Liveness check |
| `/health/report` | Aggregated health report |
| `/system` | System info (CPU, RAM, uptime, temps) |
| `/array` | Array status |
| `/disks`, `/disks/{id}` | All disks / one disk (SMART) |
| `/shares` | Network shares |
| `/docker`, `/docker/{id}` | Containers / one container |
| `/docker/{id}/logs`, `/docker/{id}/size` | Container logs / size |
| `/docker/stats`, `/docker/networks`, `/docker/updates` | Aggregate stats / networks / update status |
| `/vm`, `/vm/{id}` | VMs / one VM |
| `/vm/{name}/snapshots` | VM snapshots |
| `/gpu`, `/ups`, `/nut` | GPU / UPS / NUT status |
| `/network/access-urls` | Access URLs (LAN/WAN/WireGuard/mDNS/IPv6) |
| `/notifications`, `/notifications/overview` | Notifications / counts |
| `/array/parity-check/history`, `/array/parity-check/schedule` | Parity history / schedule |
| `/settings/system`, `/settings/docker`, `/settings/vm`, `/settings/disks` | Settings |
| `/system/flash` | USB flash drive health |
| `/user-scripts` | User Scripts list |
| `/healthchecks`, `/healthchecks/status`, `/healthchecks/history` | Health checks |

## Control (POST) — confirm destructive actions with the user first

| Path | Effect |
| --- | --- |
| `/docker/{id}/start` `/stop` `/restart` `/pause` `/unpause` | Container lifecycle |
| `/docker/{id}/update`, `/docker/update-all` | Update one / all containers |
| `/vm/{name}/start` `/stop` `/restart` `/pause` `/resume` `/hibernate` `/force-stop` | VM lifecycle |
| `/vm/{name}/snapshot`, `/vm/{name}/clone` | Create snapshot / clone VM |
| `/vm/{name}/snapshots/{snapshot_name}/restore` (POST), `…/{snapshot_name}` (DELETE) | Restore / delete snapshot ⚠️ |
| `/array/start`, `/array/stop` ⚠️ | Start / stop array |
| `/array/parity-check/start` `/stop` `/pause` `/resume` | Parity check control |
| `/system/reboot` ⚠️, `/system/shutdown` ⚠️ | Reboot / power off |
| `/user-scripts/{name}/execute` ⚠️ | Run a user script |
| `/notifications/{id}/archive`, `/notifications/archive/all` | Archive notifications |

⚠️ = high-impact: confirm with the user before calling.

## Live data

Use the WebSocket at `ws://<unraid-ip>:8043/api/v1/ws` for push updates instead
of polling these GET endpoints in a loop.
