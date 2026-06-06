# MCP Tool Catalog

All **122 MCP tools** exposed by the Unraid Management Agent, grouped by purpose.

- **R** = read-only (`ReadOnlyHint: true`) — safe to call freely.
- **W** = write/control — changes the system.
- **⚠️** = destructive; **requires `confirm=true`** and explicit user approval.

Tool names are exact. Do not invent or alias them.

> Counts: 122 tools + 5 resources + 6 prompts. Resources and prompts are listed
> in `diagnostics.md`.

---

## System & Health (read)

| R/W | Tool | Purpose |
| --- | --- | --- |
| R | `run_self_test` | OS-resilience self-test: Unraid version, overall health, capabilities, per-subsystem source status (healthy/degraded/unavailable) |
| R | `get_system_info` | Hostname, CPU/RAM usage, temperatures, uptime |
| R | `get_health_status` | Quick health summary (API status, uptime, basics) |
| R | `get_diagnostic_summary` | Broad snapshot: health, array, recent alerts |
| R | `system_health_report` | Prioritised findings across array/disks/containers/alerts |
| R | `find_root_cause` | Correlates cached signals to a likely root cause |
| R | `get_hardware_info` | Motherboard, CPU, memory (DMI/dmidecode) |
| R | `get_temperatures` | All detected temperature sensors |
| R | `get_registration` | License/registration type and key status |
| R | `get_network_info` | Interfaces, IPs, speeds, traffic stats |
| R | `get_network_access_urls` | LAN/WAN/WireGuard/mDNS/IPv6 access URLs |

## Storage — Array, Disks, Shares (read)

| R/W | Tool | Purpose |
| --- | --- | --- |
| R | `get_array_status` | Array state, capacity, parity, disk assignments |
| R | `list_disks` | All disks (array, cache, unassigned) + health |
| R | `get_disk_info` | One disk's detail incl. SMART |
| R | `get_parity_history` | Past parity checks: dates, durations, speeds, errors |
| R | `list_shares` | All network shares + settings/usage |
| R | `get_share_config` | Allocation method, cache, disk inclusion for a share |
| R | `get_unassigned_devices` | USB/unassigned disks |
| R | `get_remote_shares` | SMB/NFS/ISO remote share mount status + usage |

## Storage — ZFS (read)

| R/W | Tool | Purpose |
| --- | --- | --- |
| R | `get_zfs_pools` | Pool health, capacity, config |
| R | `get_zfs_datasets` | Datasets, quotas, usage |
| R | `get_zfs_snapshots` | Snapshots across pools/datasets |
| R | `get_zfs_arc_stats` | ARC hit ratio + memory usage |

## Docker (read)

| R/W | Tool | Purpose |
| --- | --- | --- |
| R | `list_containers` | All containers: status, resources, config |
| R | `search_containers` | Find containers by name/image/state |
| R | `get_container_info` | One container's detail |
| R | `get_container_logs` | Container stdout/stderr (docker logs) |
| R | `get_docker_log` | Docker **daemon** log |
| R | `get_container_size` | Writable-layer + virtual size of a container |
| R | `get_docker_stats` | Aggregate CPU/memory across running containers |
| R | `list_docker_networks` | Docker networks: driver, scope, IPAM |
| R | `check_container_updates` | Check all containers for image updates |
| R | `check_container_update` | Check one container for an image update |
| R | `refresh_container_updates` | Force registry digest re-check (all) |

## Virtual Machines (read)

| R/W | Tool | Purpose |
| --- | --- | --- |
| R | `list_vms` | All VMs: status + config |
| R | `search_vms` | Find VMs by name/state |
| R | `get_vm_info` | One VM's detail |
| R | `list_vm_snapshots` | Snapshots for a VM |

## Power & Sensors (read)

| R/W | Tool | Purpose |
| --- | --- | --- |
| R | `get_ups_status` | UPS battery level, load, runtime |
| R | `get_nut_status` | NUT (Network UPS Tools) variables/metrics |
| R | `get_gpu_metrics` | GPU utilization, temp, memory |

## Logs & Processes (read)

| R/W | Tool | Purpose |
| --- | --- | --- |
| R | `list_log_files` | Available log files |
| R | `get_log_content` | Tail/last-N lines of a log file |
| R | `get_syslog` | System log shortcut |
| R | `list_processes` | Processes sorted by CPU or memory |
| R | `list_process_io` | Top processes by current disk I/O rate |

## Notifications (read)

| R/W | Tool | Purpose |
| --- | --- | --- |
| R | `get_notifications` | Alerts, warnings, info messages |
| R | `get_notifications_overview` | Counts by type/importance |

## Settings & Collectors (read)

| R/W | Tool | Purpose |
| --- | --- | --- |
| R | `get_system_settings` | Server name, timezone, security mode, date |
| R | `get_docker_settings` | Docker enabled state, image path, networking |
| R | `get_vm_settings` | VM Manager state, PCI/USB passthrough |
| R | `get_disk_settings` | Spindown delay, auto-start, spinup groups |
| R | `list_collectors` | All data collectors: status, intervals |
| R | `get_collector_status` | One collector's detail |
| R | `query_metric_history` | Buffered samples + stats for a metric series |

## Updates & Services (read/check)

| R/W | Tool | Purpose |
| --- | --- | --- |
| R | `get_os_update` | Cached Unraid OS update availability |
| R | `check_plugin_updates` | Cached plugin update status |
| R | `refresh_plugin_updates` | Force a plugin update check (all) |
| R | `get_mover_status` | Mover active state, schedule, last run |
| R | `get_service_status` | Status of one system service |
| R | `list_services` | All managed services + status |

## User Scripts (read)

| R/W | Tool | Purpose |
| --- | --- | --- |
| R | `list_user_scripts` | Available User Scripts plugin scripts |

---

## Control — Containers & VMs (write)

| R/W | Tool | Purpose |
| --- | --- | --- |
| W | `container_action` | start / stop / restart / pause / unpause a container |
| W | `vm_action` | start / stop / restart / pause / resume / hibernate / force-stop a VM |
| W | `update_container` | Update one container to latest image |
| W ⚠️ | `update_all_containers` | Update all containers with available updates |
| W | `create_vm_snapshot` | Snapshot a VM |
| W ⚠️ | `delete_vm_snapshot` | Delete a VM snapshot (irreversible) |
| W ⚠️ | `restore_vm_snapshot` | Revert a VM to a snapshot (irreversible) |
| W | `clone_vm` | Clone a VM (source must be off) |

## Control — Array & Parity (write)

| R/W | Tool | Purpose |
| --- | --- | --- |
| W ⚠️ | `array_action` | Start or **stop** the array (stop = data inaccessible) |
| W | `parity_check_action` | Start a parity check |
| W | `parity_check_stop` | Stop a running parity check |
| W | `parity_check_pause` | Pause a parity check |
| W | `parity_check_resume` | Resume a paused parity check |

## Control — Disks (write)

| R/W | Tool | Purpose |
| --- | --- | --- |
| W | `disk_spin_down` | Spin a disk down (saves power) |
| W | `disk_spin_up` | Spin a disk up from standby |

## Control — System & Services (write)

| R/W | Tool | Purpose |
| --- | --- | --- |
| W ⚠️ | `system_reboot` | Reboot the server |
| W ⚠️ | `system_shutdown` | Power off the server |
| W | `service_action` | start / stop / restart a system service |
| W ⚠️ | `execute_user_script` | Run a User Scripts script |
| W | `collector_action` | Enable/disable a collector at runtime |
| W | `update_collector_interval` | Change a collector's interval (5–86400s) |

## Control — Plugins & Remote Shares (write)

| R/W | Tool | Purpose |
| --- | --- | --- |
| W | `update_plugin` | Update one plugin |
| W ⚠️ | `update_all_plugins` | Update all plugins with available updates |
| W | `remote_share_action` | Mount/unmount an SMB/NFS remote share by source |

---

## Alerting

| R/W | Tool | Purpose |
| --- | --- | --- |
| R | `list_alert_rules` | All alert rules (expr, severity, channels, enabled) |
| R | `get_alert_rule` | One alert rule by ID |
| R | `get_alert_status` | Evaluation status of enabled rules (ok/pending/firing) |
| R | `get_firing_alerts` | Only currently firing rules |
| R | `get_alert_history` | Recent alert events (firing/resolved) |
| R | `list_alert_templates` | Curated, disabled-by-default rule templates |
| W | `create_alert_rule` | Create a rule from an expr-lang expression |
| W | `enable_alert_template` | Enable a curated template by ID |
| W ⚠️ | `delete_alert_rule` | Delete a rule (confirm=true) |

## Health Checks (Watchdog)

| R/W | Tool | Purpose |
| --- | --- | --- |
| R | `list_health_checks` | All probes (type, target, interval, enabled) |
| R | `get_health_check` | One probe by ID |
| R | `get_health_check_status` | Healthy/unhealthy state, failures, last check |
| R | `get_health_check_history` | Recent state-change events |
| W | `create_health_check` | Create an HTTP/TCP/container-state probe |
| W | `run_health_check` | Manually trigger a probe |
| W ⚠️ | `delete_health_check` | Delete a probe (confirm=true) |

## Remediation Runbooks

| R/W | Tool | Purpose |
| --- | --- | --- |
| R | `list_runbooks` | Reviewed remediation runbooks + step shapes |
| W ⚠️ | `run_runbook` | Run a runbook (dry-run unless confirm=true) |

## Autonomous Agent

| R/W | Tool | Purpose |
| --- | --- | --- |
| R | `agent_get_session` | One session (status, steps, pending approval) |
| R | `agent_list_sessions` | All sessions, newest first |
| R | `agent_get_memory` | Episodic incidents + learned preferences |
| W | `agent_start_session` | Start an autonomous investigate/remediate session |
| W | `agent_approve_action` | Approve/deny an awaited high-risk action |
| W | `agent_send_message` | Continue a finished session with a follow-up |
| W | `agent_confirm_preference` | Activate a pending learned preference |

## Fan Control

| R/W | Tool | Purpose |
| --- | --- | --- |
| R | `get_fan_status` | Fan speeds, modes, profiles, config |
| W | `set_fan_speed` | Set PWM speed for a fan (manual mode) |
| W | `set_fan_mode` | automatic (BIOS) or manual (software) |
| W | `set_fan_profile` | Assign a temp-curve profile (quiet/balanced/performance) |
| W | `create_fan_profile` | Create a custom temp-curve profile |
| W | `restore_fan_defaults` | Return all fans to automatic (safe) |

## CPU Control

| R/W | Tool | Purpose |
| --- | --- | --- |
| W | `set_cpu_governor` | Set scaling governor (performance/powersave/ondemand…) |

## System Tuning

| R/W | Tool | Purpose |
| --- | --- | --- |
| R | `get_tuning_status` | Turbo boost, disk cache (vm.dirty_*), inotify limits |
| W | `set_turbo_boost` | Enable/disable Intel Turbo / AMD Performance Boost |
| W | `set_disk_cache` | Set vm.dirty_* disk cache parameters |
| W | `set_inotify_limits` | Set inotify kernel limits (max_user_watches, …) |
