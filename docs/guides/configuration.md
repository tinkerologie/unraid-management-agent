# Configuration Guide

Complete guide to configuring the Unraid Management Agent.

## Overview

The agent uses command-line arguments for configuration, managed automatically by the plugin system. Most settings can be adjusted without editing files.

## Configuration File Location

**Primary Config**: `/boot/config/plugins/unraid-management-agent/unraid-management-agent.cfg`

This file is created automatically after the first installation and persists across reboots (stored on USB flash).

## Command-Line Options

### Core Settings

| Option                           | Default  | Description                                                                                                                                                            |
| -------------------------------- | -------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `--port`                         | `8043`   | HTTP API port                                                                                                                                                          |
| `--bind-address`                 | -        | IP to bind the HTTP server to (empty = all). mDNS advertises it. Loopback rejected; invalid → all.                                                                     |
| `--read-only`                    | `false`  | Block state-changing MCP tools (AI agents read-only; REST API unaffected)                                                                                              |
| `--tool-policy`                  | -        | Comma-separated per-tool MCP policies (`tool=policy`), e.g. `system_reboot=ask,container_action=read_only`. If `tool_policy.json` exists on disk, it takes precedence. |
| `--api-token`                    | -        | Require `Authorization: Bearer <token>` on the HTTP API and `/mcp` (empty = unauthenticated reads, network writes refused)                                             |
| `--allow-unauthenticated-writes` | `false`  | Allow network writes without an API token (not recommended; `ALLOW_UNAUTHENTICATED_WRITES`)                                                                            |
| `--wan-ip-lookup`                | `true`   | Ask api.ipify.org, ifconfig.me and icanhazip.com for the WAN IP on request (`WAN_IP_LOOKUP`; `false` uses only Unraid's `WANIP`)                                       |
| `--vm-guest-agent`               | -        | Comma-separated VMs to read through the QEMU guest agent (`VM_GUEST_AGENT`); see [VM guest agent](#vm-guest-agent)                                                     |
| `--mcp-connect-secret`           | -        | Optional 32–256 char (`[A-Za-z0-9_-]`) secret enabling header-free MCP onboarding at `/mcp/<secret>` (`MCP_CONNECT_SECRET`)                                            |
| `--debug`                        | `false`  | Enable debug logging                                                                                                                                                   |
| `--mqtt-enabled`                 | `false`  | Enable MQTT publishing                                                                                                                                                 |
| `--mqtt-broker`                  | -        | MQTT broker address (e.g., `tcp://localhost:1883`)                                                                                                                     |
| `--mqtt-topic-prefix`            | `unraid` | MQTT topic prefix                                                                                                                                                      |
| `--mqtt-username`                | -        | MQTT username (optional)                                                                                                                                               |
| `--mqtt-password`                | -        | MQTT password (optional)                                                                                                                                               |
| `--discovery-enabled`            | `true`   | Advertise the agent via mDNS for auto-discovery                                                                                                                        |
| `--discovery-service-name`       | -        | Override the advertised mDNS instance name                                                                                                                             |

### Collection Mode

`--collection-mode` (env `COLLECTION_MODE`, `collection_mode` in `config.yml`) chooses who
decides when collectors run. Precedence is the usual: flag, then environment, then
`config.yml`, then the default. An invalid `config.yml` value is logged and ignored.

| Mode      | Behaviour                                                                                                              |
| --------- | ---------------------------------------------------------------------------------------------------------------------- |
| `dynamic` | The agent decides, from change events, active consumers and safety floors. `INTERVAL_*` values are kept but not used.  |
| `manual`  | Every collector runs at its configured `INTERVAL_*` value, as before. Default of the daemon binary outside the plugin. |

**Plugin installs.** Fresh installs start in `dynamic`. On upgrade the plugin runs a one-time
migration: if every `INTERVAL_*` value is still a shipped default (or `auto`), the install
moves to `dynamic`; if any value was changed, it stays on `manual` so the tuning is kept. The
"Dynamic collection" toggle at the top of the Collection Intervals section switches modes. In
Dynamic mode the interval settings under it are hidden; saved intervals survive the switch and
apply again in Manual mode. `GET /api/v1/collectors/status` shows what each collector does right
now.

**What Dynamic mode does.** Each collector has a built-in _active_ and _idle_ interval:

- **Active** while something consumes its data: a WebSocket client subscribed to one of its
  topics (a client without a topic filter receives, and so activates, every topic), the MQTT
  client while connected (for the collectors MQTT publishes), or a REST or MCP read of its data
  in the last 5 minutes. Clients subscribed only to other topics do not speed it up.
- **Idle** otherwise. Event-driven collectors poll at a slow safety-net rate instead, because
  events can be missed. Low power mode multiplies idle and safety-net intervals by 4.
- **Change events** run a collector right away: Docker container events (`docker`), libvirt
  domain lifecycle events (`vm`), file changes under `/var/local/emhttp` and `/boot/config`
  (`array`, `disk`, `shares`, `notification`, `mover`, `unassigned`, `plugin_update`) and kernel
  hotplug uevents (`unassigned`, `hardware`). Rewrites of `disks.ini` and `shares.ini` that only
  update I/O counters, temperatures or space usage, which emhttp writes every few seconds, do
  not count as changes; the polls pick those values up.
- **Refresh on read.** For collectors that follow demand (not the fixed-schedule ones listed
  below), a REST, MCP or `/metrics` read whose data is older than the collector's
  staleness limit (its active interval) triggers one collection, shared by concurrent reads, and
  waits up to 2 seconds for it. If the collection fails or is late, the read returns the cached
  data; REST responses carry `X-Cache-Updated-At` and, in that case, `X-Cache-Stale: true`. A
  `/metrics` scrape refreshes stale data but does not count as demand, so the scrape interval
  drives the rate.
- **Safety floors.** While an enabled alert rule reads a collector's data, that collector never
  runs slower than its Manual default. The built-in rules need no collector data, so they set
  no floors. Watchdog health checks and fan control read their own sources and are unaffected.
- Slow-changing collectors (`hardware`, `registration`, `tuning`, `docker_update`,
  `plugin_update`, `os_update`) keep long fixed schedules. Reads of their data never trigger a
  collection and never set `X-Cache-Stale`; their data is as old as their last scheduled run.

| Collector           | Active | Idle / safety net | Events                                                |
| ------------------- | ------ | ----------------- | ----------------------------------------------------- |
| `system`            | 15s    | 60s               |                                                       |
| `array`             | 60s    | 600s              | `var.ini`, `disks.ini`                                |
| `disk`              | 300s   | 1800s             | `var.ini`, `disks.ini`                                |
| `docker`            | 30s    | 300s              | Docker events                                         |
| `vm`                | 60s    | 600s              | libvirt lifecycle                                     |
| `ups`, `nut`, `gpu` | 60s    | 300s              |                                                       |
| `shares`            | 60s    | 600s              | `shares.ini`                                          |
| `network`           | 60s    | 300s              |                                                       |
| `zfs`               | 300s   | 900s              |                                                       |
| `notification`      | 30s    | 300s              | notification folders                                  |
| `unassigned`        | 60s    | 600s              | `disks.ini`, Unassigned Devices config, block uevents |
| `fancontrol`, `nic` | 60s    | 300s              |                                                       |
| `docker_networks`   | 60s    | 600s              |                                                       |
| `mover`             | 30s    | 300s              | `var.ini`                                             |
| `storage_topology`  | 300s   | 900s              |                                                       |
| `hardware`          | 600s   | 600s              | device uevents                                        |
| `registration`      | 600s   | 600s              |                                                       |
| `tuning`            | 120s   | 120s              |                                                       |
| `docker_update`     | 6h     | 6h                |                                                       |
| `plugin_update`     | 1h     | 1h                | `*.plg` installs                                      |
| `os_update`         | 24h    | 24h               |                                                       |

In Dynamic mode `UNRAID_DISABLE_COLLECTORS` and the runtime enable/disable endpoints still turn
collectors off, hardware detection still gates the hardware collectors, and
`PATCH /collectors/{name}/interval` returns `409 Conflict`. `GET /collectors/status` reports
each collector's `mode`, `effective_interval_seconds` and `reason`.

### Collection Intervals

Control how often data is collected in Manual mode (in seconds):

| Collector          | Flag                          | Default                         | Min | Max    |
| ------------------ | ----------------------------- | ------------------------------- | --- | ------ |
| System             | `--interval-system`           | 15s                             | 1s  | 3600s  |
| Array              | `--interval-array`            | 60s                             | 5s  | 3600s  |
| Disks              | `--interval-disk`             | 300s                            | 10s | 3600s  |
| Docker             | `--interval-docker`           | 30s                             | 5s  | 3600s  |
| VMs                | `--interval-vm`               | 60s                             | 5s  | 3600s  |
| UPS                | `--interval-ups`              | 60s                             | 5s  | 3600s  |
| NUT                | `--interval-nut`              | 0s (disabled)                   | 5s  | 3600s  |
| GPU                | `--interval-gpu`              | 60s                             | 5s  | 3600s  |
| Shares             | `--interval-shares`           | 60s                             | 30s | 3600s  |
| Network            | `--interval-network`          | 60s                             | 5s  | 3600s  |
| Hardware           | `--interval-hardware`         | 600s                            | 30s | 3600s  |
| ZFS                | `--interval-zfs`              | 300s                            | 10s | 3600s  |
| Notifications      | `--interval-notification`     | 30s                             | 10s | 3600s  |
| Registration       | `--interval-registration`     | 600s                            | 60s | 3600s  |
| Unassigned Devices | `--interval-unassigned`       | 60s                             | 30s | 3600s  |
| Storage Topology   | `--interval-storage-topology` | 0s (disabled; 300s recommended) | 0s  | 86400s |
| Network Cards      | `--interval-nic`              | 0s (disabled; 60s recommended)  | 0s  | 86400s |

The plugin starts these hardware-dependent collectors in automatic mode by
default; see [Hardware Auto-Detection](#hardware-auto-detection).

**Disable a collector**: Set interval to `0`

```bash
# Example: Disable GPU collector
--interval-gpu 0
```

### Hardware Auto-Detection

The agent detects which optional hardware the server has and runs the matching
collectors only when that hardware exists
([#261](https://github.com/tinkerologie/unraid-management-agent/issues/261)).
Detection is read-only: it reads sysfs, procfs and config files, runs no
commands, never wakes spun-down disks, and the NIC check reads at most 16
module EEPROMs (only when no NIC temperature sensor is found).

| Capability                  | Collector          | Detected when                                                                                                                       |
| --------------------------- | ------------------ | ----------------------------------------------------------------------------------------------------------------------------------- |
| `hardware.nic_optics`       | `nic`              | A wired PCI network card has a NIC-owned hwmon temperature sensor, or a port has a plugged SFP/QSFP-class module                    |
| `hardware.storage_topology` | `storage_topology` | `/sys/class/enclosure` has an entry, a SCSI generic device is an SES enclosure, or a SAS HBA or Broadcom/LSI RAID controller exists |
| `hardware.gpu`              | `gpu`              | A DRM card is driven by `i915`, `xe`, `amdgpu` or `nvidia`, or `/dev/nvidia<N>` exists (GPUs bound to `vfio-pci` do not count)      |
| `hardware.ups`              | `ups`              | apcupsd is running or enabled in Unraid's UPS settings, or NUT is detected                                                          |
| `hardware.nut`              | `nut`              | The NUT plugin is installed and enabled or running                                                                                  |
| `hardware.zfs`              | `zfs`              | The ZFS module is loaded and at least one pool is imported                                                                          |
| `hardware.fan_control`      | —                  | A hwmon chip has a `pwmN` output with `pwmN_enable`, or an IPMI device exists (reported only)                                       |

Plain onboard NICs without sensors or optics (for example Intel I219-V and
I211) and Wi-Fi cards (for example Intel 3168NGW) do not count as
"Network Cards & Optics". An empty SFP cage on a card without temperature
sensors is not detected until a module is inserted, because sysfs does not
expose cages.

Each class is `present`, `absent` or `unknown` (a read error or timeout).
Results are re-checked at startup, every 15 minutes, and on demand with
`GET /api/v1/capabilities?refresh=true` (at most every 30 seconds) or the
**Re-check hardware** button on the plugin settings page (shown in Manual collection mode).

**Absent hardware always wins.** While a class is absent, its collector is off
and its settings row is hidden, whatever interval is saved. The saved value is
kept and applies again as soon as the hardware is detected:

| Saved setting        | Hardware present                            | Hardware absent |
| -------------------- | ------------------------------------------- | --------------- |
| **Auto** (default)   | Runs every 300 s (1200 s in low power mode) | Off, row hidden |
| An explicit interval | Runs at that interval                       | Off, row hidden |
| **Disabled** (`0`)   | Off                                         | Off, row hidden |

- Hardware that was never detected turns its collector off at once. Hardware
  that was detected and then disappears turns it off after two consecutive
  `absent` results, so a single flaky check does not stop it.
- An `unknown` result changes nothing.
- The plugin passes **Auto** collectors to the agent as
  `UNRAID_AUTO_COLLECTORS` (`--auto-collectors`, `auto_collectors` in
  `config.yml`).
- A collector in `UNRAID_DISABLE_COLLECTORS` is never turned on by detection.
- Enabling, disabling or changing a collector's interval at runtime (REST or
  MCP) takes it out of hardware control until the agent restarts.
  `GET /api/v1/collectors/status` reports `"hardware_gated": true` while a
  collector follows detection and `"auto": true` while it is in automatic mode.

**Show all collectors** on the settings page only reveals hidden rows; it
never turns a collector on. If hardware detection is unavailable (for example
the agent is stopped), the page shows every row.

**Upgrades.** On the first install of this release, each hardware-dependent
interval that is missing or still equals a previously shipped default
(NIC `60` or `0`, Storage Topology `300` or `0`, GPU `60`, UPS `60`, NUT `0`,
ZFS `0` or `30`) is switched to **Auto**. Any other value is kept and applies
while the hardware is present. To keep a fixed interval, choose it (or
**Disabled**) for that collector and click **Apply**.

## Configuration Methods

### Method 1: Plugin Settings UI (Coming Soon)

Future versions will include a web UI for configuration.

### Method 2: Edit Config File

1. **Stop the service**:

   ```bash
   /etc/rc.d/rc.unraid-management-agent stop
   ```

2. **Edit config**:

   ```bash
   nano /boot/config/plugins/unraid-management-agent/unraid-management-agent.cfg
   ```

3. **Example configuration**:

   ```bash
   # Unraid Management Agent Configuration
   PORT=8043
   DEBUG=false

   # Bind the API server to a specific IP (empty = all interfaces).
   # Handy on multi-VLAN systems so Home Assistant connects via the right network.
   BIND_ADDRESS=192.168.40.10

   # Block all state-changing MCP tools (AI agents can only read)
   READ_ONLY=false

   # Per-tool MCP access policies (comma-separated tool=policy pairs)
   # Options: default, hidden, read_only, allow, ask
   TOOL_POLICY=""

   # Require "Authorization: Bearer <token>" on the API and /mcp.
   # Empty (the default) leaves the API unauthenticated.
   API_TOKEN=

   # MQTT Settings
   MQTT_ENABLED=true
   MQTT_BROKER=tcp://mqtt.local:1883
   MQTT_USERNAME=unraid
   MQTT_PASSWORD=your_password
   MQTT_TOPIC_PREFIX=unraid

   # Collection Intervals (seconds)
   INTERVAL_SYSTEM=5
   INTERVAL_ARRAY=10
   INTERVAL_DISK=30
   INTERVAL_DOCKER=10
   INTERVAL_VM=10
   INTERVAL_UPS=10
   INTERVAL_GPU=10
   INTERVAL_SHARES=60
   ```

4. **Start the service**:

   ```bash
   /etc/rc.d/rc.unraid-management-agent start
   ```

### Method 3: Command Line (Testing)

For temporary changes or testing:

```bash
# Stop service
/etc/rc.d/rc.unraid-management-agent stop

# Run with custom settings
/usr/local/bin/unraid-management-agent boot \
  --port 8043 \
  --debug \
  --interval-system 10 \
  --interval-disk 60
```

## Runtime Collector Control

Collectors can be enabled/disabled at runtime without restarting the service (added in v2025.11.0):

### Via REST API

```bash
# List all collectors with status
curl http://localhost:8043/api/v1/collectors/status

# Enable a collector
curl -X POST http://localhost:8043/api/v1/collectors/gpu/enable

# Disable a collector
curl -X POST http://localhost:8043/api/v1/collectors/gpu/disable

# Update collector interval
curl -X PATCH http://localhost:8043/api/v1/collectors/gpu/interval \
  -H "Content-Type: application/json" \
  -d '{"interval": 30}'
```

### Via MCP (AI Agents)

```python
# Using MCP tools
tools.list_collectors()
tools.collector_action(collector_name="gpu", action="enable")
tools.update_collector_interval(collector_name="gpu", interval=30)
```

## Common Configuration Scenarios

### Scenario 1: Low-Power Server

Reduce collection frequency to save CPU:

```bash
INTERVAL_SYSTEM=15
INTERVAL_ARRAY=30
INTERVAL_DISK=120
INTERVAL_DOCKER=30
INTERVAL_VM=30
INTERVAL_SHARES=300
```

### Scenario 2: High-Frequency Monitoring

For real-time dashboards:

```bash
INTERVAL_SYSTEM=1
INTERVAL_ARRAY=5
INTERVAL_DISK=10
INTERVAL_DOCKER=5
INTERVAL_VM=5
```

### Scenario 3: Disable Unused Collectors

```bash
# No GPU
INTERVAL_GPU=0

# No VMs
INTERVAL_VM=0

# No UPS
INTERVAL_UPS=0

# No ZFS
INTERVAL_ZFS=0
```

### Scenario 4: MQTT Integration

Enable MQTT for Home Assistant:

```bash
MQTT_ENABLED=true
MQTT_BROKER=tcp://homeassistant.local:1883
MQTT_USERNAME=mqtt_user
MQTT_PASSWORD=secure_password
MQTT_TOPIC_PREFIX=unraid/tower
```

## MQTT Configuration

### Basic Setup

```bash
MQTT_ENABLED=true
MQTT_BROKER=tcp://192.168.1.100:1883
```

### With Authentication

```bash
MQTT_ENABLED=true
MQTT_BROKER=tcp://mqtt.example.com:1883
MQTT_USERNAME=unraid_agent
MQTT_PASSWORD=your_secure_password
```

### Custom Topic Prefix

```bash
MQTT_TOPIC_PREFIX=homelab/unraid
```

Topics will be:

- `homelab/unraid/system`
- `homelab/unraid/array`
- `homelab/unraid/containers`
- etc.

### TLS/SSL (if broker requires)

```bash
MQTT_BROKER=ssl://mqtt.example.com:8883
```

## VM Guest Agent

The agent can read data from inside VMs through the QEMU guest agent, so a VM
needs no monitoring software of its own (such as LibreHardwareMonitor).
Each listed VM needs the guest agent installed and running
(`qemu-guest-agent` on Linux, the virtio-win guest tools on Windows).

Every `interval` seconds (default 30, minimum 10) the agent reads:

- host name, OS name and filesystem usage
- NVIDIA GPUs with `nvidia-smi`: temperature, utilization, memory, power, fan and clock
- on Windows, physical disks with `Get-StorageReliabilityCounter`: temperature, maximum
  temperature and wear

Custom sensors run a command inside the VM on their own interval (default 30, minimum
5 seconds); the trimmed standard output becomes the sensor value. Commands run without
a shell, as SYSTEM on Windows or root on Linux, and time out after 30 seconds.

A sensor with `json: true` must print a flat JSON object. Each key becomes its own sensor
named `<name>_<key>`: the key is lowercased, any character other than a letter, digit or
underscore becomes `_`, and the result must still be 1-64 characters (otherwise that key is
skipped with a log message). Numbers are written in plain decimal (`98.4`, `82`), strings
as they are, booleans as `true`/`false` and `null` as a `null` value; nested objects and
arrays are skipped. Sensors are listed in config order, then keys sorted. Output that is not
a JSON object counts as a failed run: the keys seen so far stay listed with `null` values.

`attributes` (only with `json: true`) maps a JSON key to the key whose sensor gets it as an
attribute instead of a sensor of its own; `attributes: {graph: avg}` reports `frames_graph`'s
value as attribute `graph` of sensor `frames_avg`. Both sides are normalised like sensor
keys. An attribute whose key is missing or fails is `null`; if the target key is missing, the
attributes are left out for that run. Empty names, or a target that is itself an attribute
key, skip the sensor with a log message.

Results are at `GET /api/v1/vm/guest` and in the `vm_guest_update` WebSocket event.
While a VM is off, or its guest agent is not answering, its GPUs, disks, filesystems
and sensors stay listed with `null` readings, so a consumer can display them as unknown
rather than unavailable.

Configure it in `config.yml`:

```yaml
vm_guest_agent:
  vms: ["Windows 11"]
  interval: 30
  sensors:
    - vm: "Windows 11"
      name: fps # 1-64 lowercase letters, digits or underscores
      interval: 5
      command:
        - C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe
        - -NoProfile
        - -ExecutionPolicy
        - Bypass
        - -File
        - C:\ProgramData\Scripts\fps.ps1
    # frames.ps1 prints {"avg":98.4,"low5":91,"low1":82,"graph":"82|10"} -> sensors
    # frames_avg (with attribute graph), frames_low5 and frames_low1
    - vm: "Windows 11"
      name: frames
      json: true
      attributes:
        graph: avg
      interval: 5
      command:
        - C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe
        - -NoProfile
        - -File
        - C:\ProgramData\Scripts\frames.ps1
```

`--vm-guest-agent` / `VM_GUEST_AGENT` (comma-separated VM names) replaces the `vms`
list; custom sensors are set only in `config.yml`, and a sensor whose VM is not in
the list is skipped.

## Auto-Discovery (zeroconf/mDNS)

The agent advertises itself on the local network using zeroconf (mDNS/DNS-SD),
so integrations such as the [Home Assistant integration](https://github.com/tinkerologie/ha-unraid-management-agent)
can auto-discover the server instead of requiring a manually entered IP and port.

This is **enabled by default** and requires no configuration. The agent
publishes a DNS-SD service of type `_unraid-mgmt-agent._tcp.local.` with TXT
records describing the version, API base path, and server name:

| TXT record | Example      | Purpose                         |
| ---------- | ------------ | ------------------------------- |
| `version`  | `2026.06.01` | Agent version                   |
| `path`     | `/api/v1`    | REST API base path              |
| `name`     | `tower`      | Server hostname / friendly name |

### Settings

```bash
# Disable advertising (e.g. on isolated networks where mDNS is blocked)
DISCOVERY_ENABLED=false

# Optionally override the advertised instance name (defaults to the hostname)
DISCOVERY_SERVICE_NAME=Main Unraid
```

### Advertised address

The agent advertises one IP address, chosen from one LAN interface, and sends
its announcements on that interface only. Every answer carries only that
address. The address is chosen in this order:

1. `BIND_ADDRESS`, when set to a specific IP on a LAN interface.
2. Otherwise the source address of the default route, when it belongs to a LAN
   interface.
3. Otherwise the first IPv4 address of `br0`, then `bond*`, then `eth*`, then
   any other LAN interface.

Docker (`docker0`, `br-*`, `veth*`), LXC, LXD, Incus and Podman bridges,
CNI and Kubernetes overlays, libvirt (`virbr*`, `vnet*`, `tap*`), Unraid shim
(`shim-*`) and VPN or overlay interfaces (`tun*`, `wg*`, `tailscale*`, `zt*`)
are never used. If no LAN interface has an address yet, for example early in
boot, the agent logs a warning and retries every 30 seconds.

A `BIND_ADDRESS` that is not on a LAN interface, such as a Docker bridge IP
or an address the host does not have, does not fall back to steps 2 and 3.
The agent logs the warning and keeps retrying until that address appears on
a LAN interface.

### Notes

- Advertising is **best-effort**: if registration fails (for example, when
  multicast is blocked), the agent logs a warning and continues normally.
- The agent coexists with Unraid's existing `avahi-daemon`; both respond only
  for their own service types.
- mDNS only works within a single broadcast domain (subnet). For discovery
  across VLANs/subnets, configure an mDNS reflector/repeater on your router.

## OS-Resilience & Self-Diagnostics

The agent continuously checks that each Unraid data source it reads is healthy.
If an OS update moves a path, changes a file format, or removes a binary, the
affected subsystem is flagged **degraded** (or **unavailable**) instead of
silently returning empty/wrong data — and the agent keeps serving whatever valid
data it still has.

Surfaces:

- **Self-test endpoint:** `GET /api/v1/diagnostics/self-test` → detected Unraid
  version, `overall_state`, probed capabilities, and per-subsystem source status.
- **MCP tool:** `run_self_test` (read-only) returns the same payload for AI agents.
- **Inline flag:** affected subsystem responses include a `source_status` field
  (omitted entirely when healthy, so healthy responses are unchanged).
- **Prometheus:** `unraid_subsystem_status{subsystem="…"}` (0=healthy, 1=degraded,
  2=unavailable) and `unraid_degraded_subsystem_count`.
- **Alert:** a built-in, enabled-by-default rule `subsystem_degraded` raises a
  warning notification the moment any source becomes degraded (you can disable or
  edit it like any alert rule).

No configuration is required — this is always on and self-contained (no
dependency on the official Unraid API).

## Performance Tuning

### CPU Impact

Collection intervals affect CPU usage:

- **1-5 second intervals**: ~2-5% CPU usage
- **10-30 second intervals**: ~1-2% CPU usage (recommended)
- **60+ second intervals**: <1% CPU usage

### Memory Usage

Typical memory footprint:

- Base: ~20-30 MB
- Per collector: ~5-10 MB
- WebSocket clients: ~1 MB per connection

### Network Bandwidth

With default intervals:

- REST API: Negligible (on-demand)
- WebSocket: ~5-10 KB/s per client
- MQTT: ~2-5 KB/s
- Prometheus scraping: ~10-20 KB per scrape

## Logging

### Log Levels

```bash
# Minimal logging
DEBUG=false

# Detailed logging (for troubleshooting)
DEBUG=true
```

### Log Location

**File**: `/var/log/unraid-management-agent.log`

**Rotation**: Automatic at 5 MB

### View Logs

```bash
# Tail logs
tail -f /var/log/unraid-management-agent.log

# View recent errors
grep ERROR /var/log/unraid-management-agent.log

# View debug messages (if enabled)
grep DEBUG /var/log/unraid-management-agent.log
```

## Security

### Network Security

#### WAN IP lookup

When network access URLs are requested (`GET /api/v1/network/access-urls` or the MCP tool
`get_network_access_urls`), the agent asks `api.ipify.org`, `ifconfig.me` and `icanhazip.com`, in that order,
for the server's public (WAN) IP and stops at the first answer. Those services see the server's public IP
address. Nothing is looked up at startup or on a schedule, and the result is not cached. If all three fail,
the `WANIP=` value from Unraid's `/boot/config/network.cfg` is used.

To make no external requests, turn the lookup off. The WAN entry then uses only the `WANIP=` value, or is
left out if none is set.

| Method                     | Setting                                         |
| -------------------------- | ----------------------------------------------- |
| Plugin settings page       | **WAN IP Lookup** (off)                         |
| Config file (`config.cfg`) | `WAN_IP_LOOKUP="false"`                         |
| YAML config (`config.yml`) | `wan_ip_lookup: false`                          |
| CLI flag / env var         | `--wan-ip-lookup=false` / `WAN_IP_LOOKUP=false` |

**Recommendations**:

- Keep port 8043 on internal network only
- Use WireGuard VPN for remote access
- Use reverse proxy with authentication (nginx, Traefik)

### HTTPS / TLS

By default the agent serves plain HTTP. You can have it serve HTTPS natively
(including the `/mcp` endpoint) by pointing it at a PEM certificate/key pair.
TLS is enabled only when **both** files are configured. If only one is set, or
either file is missing, unreadable, or not a valid certificate/key pair, the
agent logs a warning and falls back to plain HTTP so a stale path can never make
it unreachable.

| Setting          | CLI flag          | Env var         | Config key      |
| ---------------- | ----------------- | --------------- | --------------- |
| Certificate file | `--tls-cert-file` | `TLS_CERT_FILE` | `tls_cert_file` |
| Private key file | `--tls-key-file`  | `TLS_KEY_FILE`  | `tls_key_file`  |

Both paths must be absolute. The cert and key may live in separate files or in a
single combined PEM bundle — Go reads the certificate from `tls_cert_file` and
the private key from `tls_key_file`, so pointing **both** at one bundle that
contains the cert and key is valid. Example (`config.yml`) using Unraid's
combined bundle for both:

```yaml
tls_cert_file: /boot/config/ssl/certs/certificate_bundle.pem
tls_key_file: /boot/config/ssl/certs/certificate_bundle.pem
```

Point these at a **publicly-trusted** certificate — for example Unraid's own
`*.myunraid.net` Let's Encrypt certificate, which Unraid maintains as a combined
cert+key bundle at `/boot/config/ssl/certs/certificate_bundle.pem` (the same
path used in the example above for both keys). Self-signed certificates work for
browsers and `curl -k`, but are **not** accepted by hosted clients such as
Claude Desktop.

> [!NOTE]
> Claude Desktop / claude.ai "Custom Connectors" are reached from Anthropic's
> cloud, so native HTTPS alone is not enough — the endpoint must also be
> reachable from the public internet (port-forward or tunnel) with a trusted
> cert. For LAN-only use, the `mcp-remote` bridge needs no TLS at all. See the
> [Claude integration guide](../integrations/claude/README.md#2-connect-claude-to-your-server-mcp).

### Authentication

Set an API token to require `Authorization: Bearer <token>`. When the token is
empty (the default) network clients can read data without authentication, but
every state-changing request is refused:

- REST `POST`, `PUT`, `PATCH` and `DELETE` requests return `403 Forbidden` with
  `{"error": "Write access is disabled because no API token is configured. Set API_TOKEN in the plugin settings (or the API_TOKEN environment variable) and send it as 'Authorization: Bearer <token>'."}`
- HTTP MCP (`/mcp` and `/mcp/<connect-secret>`) treats every write tool as
  read-only. STDIO MCP (`mcp-stdio`) runs locally and is not affected.

`GET`, `HEAD` and `OPTIONS` requests, WebSocket subscriptions and MCP read tools
keep working, so monitoring clients need no token.

#### Allow write access without a token (not recommended)

`ALLOW_UNAUTHENTICATED_WRITES` restores the behaviour of earlier releases: with
no token, anyone who can reach the agent can control the server. It has no
effect once a token is set.

| Setting                      | CLI flag                         | Env var                        | Config key                                                                            |
| ---------------------------- | -------------------------------- | ------------------------------ | ------------------------------------------------------------------------------------- |
| Allow writes without a token | `--allow-unauthenticated-writes` | `ALLOW_UNAUTHENTICATED_WRITES` | `allow_unauthenticated_writes` (YAML) / `ALLOW_UNAUTHENTICATED_WRITES` (`config.cfg`) |

Plugin installs that were upgraded from an earlier release without a token get
`ALLOW_UNAUTHENTICATED_WRITES="true"` once, so their existing clients keep
working, and the settings page shows a warning. Set a token, add it to your
clients, then turn the option off. Fresh installs start with it off. Daemons
started directly (CLI, environment or YAML) are not migrated: set `API_TOKEN`,
or the opt-out, to keep write access.

#### Plugin Settings Page (Recommended)

The easiest way to configure authentication is through the Unraid WebGUI:

1. Go to **Settings** $\rightarrow$ **Unraid Management Agent**.
2. Scroll to the **Authentication** section.
3. Click **Generate** to create a secure 64-character token via the browser's cryptographic API.
4. Click **Copy** to copy the token to your clipboard.
5. Click **Apply** at the bottom of the page. The plugin saves the token and restarts the service automatically.

Clicking **Clear** and then **Apply** removes the token. Network access then
becomes read-only again, unless **Allow Write Access Without a Token**
(`ALLOW_UNAUTHENTICATED_WRITES`) is on, in which case anyone on the network can
use control actions (see above).

#### Manual Configuration (SSH / config.cfg)

Prefer the config key or the `API_TOKEN` environment variable over the
`--api-token` flag: command-line arguments are visible to any user who can read
the process list.

| Setting   | CLI flag      | Env var     | Config key  |
| --------- | ------------- | ----------- | ----------- |
| API token | `--api-token` | `API_TOKEN` | `api_token` |

```bash
# Generate a token and enable authentication
API_TOKEN=$(head -c 32 /dev/urandom | base64)
echo "$API_TOKEN"
```

Add it to `config.cfg`:

```bash
API_TOKEN='paste-the-generated-token-here'
```

**Use single quotes.** The plugin launcher reads `config.cfg` with `source`,
which expands `$`, `` ` ``, and `\` inside a double-quoted value — so
`API_TOKEN="tok$en"` reaches the daemon truncated, or empty. Single quotes stop
that. The launcher compares the sourced value against the raw file contents and
refuses to start on a mismatch, so this fails loudly rather than leaving
authentication quietly disabled, but quoting it correctly avoids the problem in
the first place.

Tokens generated the way shown above contain only `A-Za-z0-9+/=` and are always
safe. If you choose your own, the quoting rules are:

| Token contains                            | Use                                                                                                 |
| ----------------------------------------- | --------------------------------------------------------------------------------------------------- |
| none of `'`, `"`, `$`, `` ` ``, `\`       | either quote style                                                                                  |
| `"`, `$`, `` ` `` or `\` — but no `'`     | single quotes — `API_TOKEN='to$ken'`                                                                |
| `'` — but none of `"`, `$`, `` ` ``, `\`  | double quotes — `API_TOKEN="a'b"`                                                                   |
| `'` **and** any of `"`, `$`, `` ` ``, `\` | not expressible in `config.cfg` — use the `API_TOKEN` environment variable, or generate a new token |

Single quotes cannot contain a single quote, and double quotes cannot contain a
double quote, which is what the last row reflects. The usual shell escape
`API_TOKEN='a'\''b'` does not help: it is rejected too, because the launcher
compares against the raw line rather than re-implementing shell quoting. All of
these fail at startup with an explanation rather than starting with a corrupted
credential.

Then pass it on every request:

```bash
curl -H "Authorization: Bearer $API_TOKEN" https://your-unraid-ip:8043/api/v1/system
```

Use `https://` wherever the agent has TLS configured (see
[HTTPS / TLS](#https--tls)) — a bearer token sent over plain `http://` travels in
clear text and is readable by anything on the path. On a trusted LAN with TLS
disabled, substitute `http://`.

The token is never sanitised or rewritten: the value you configure is the value
compared. Via the `API_TOKEN` environment variable or `--api-token`, any
character is safe to use; via `config.cfg` the quoting table above applies.
Anything that would make the effective token differ from the configured one is
a startup error rather than a silent change: control characters, leading or
trailing whitespace (the server trims what a client sends, per normal HTTP
header handling, so a padded token would authenticate as its trimmed form), and
a whitespace-only value, which would otherwise fall back to no authentication
at all.

**What is protected**: every REST endpoint, `/mcp`, and `/metrics`.

**What is not**:

- `/api/v1/health` — so external uptime monitoring keeps working without
  embedding a credential. It reports liveness only and exposes no system data.
- `/swagger/` — the docs UI is a browser page that cannot attach an
  `Authorization` header when fetching its own `doc.json`, so requiring one
  would leave it permanently broken rather than merely gated. It serves the API
  schema and no system data.

Requests without a valid token receive `401 Unauthorized` with a
`WWW-Authenticate: Bearer` challenge. The token is compared in constant time,
and because the field name contains "token" it is automatically redacted from
diagnostics output.

> [!NOTE]
> The token is sent as a plain header, so pair it with HTTPS (see
> [HTTPS / TLS](#https--tls)) on any network you do not fully trust. Over plain
> HTTP the token is readable by anything on the path.

Complementary options, still worth combining with a token:

1. **Reverse Proxy**: nginx with basic auth
2. **VPN Only**: WireGuard/Tailscale
3. **Firewall Rules**: Restrict source IPs

### Example: nginx Reverse Proxy

```nginx
server {
    listen 443 ssl;
    server_name unraid-api.local;

    ssl_certificate /path/to/cert.pem;
    ssl_certificate_key /path/to/key.pem;

    auth_basic "Unraid API";
    auth_basic_user_file /path/to/.htpasswd;

    location / {
        proxy_pass http://localhost:8043;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
    }
}
```

## Troubleshooting

### Changes Not Applied

```bash
# Restart service
/etc/rc.d/rc.unraid-management-agent restart

# Verify config
cat /boot/config/plugins/unraid-management-agent/unraid-management-agent.cfg
```

### Invalid Configuration

```bash
# Check logs for errors
tail -f /var/log/unraid-management-agent.log

# Test with defaults
/usr/local/bin/unraid-management-agent boot
```

### Collector Not Running

```bash
# Check collector status
curl http://localhost:8043/api/v1/collectors/status

# Enable via API
curl -X POST http://localhost:8043/api/v1/collectors/COLLECTOR_NAME/enable
```

## Advanced Configuration

### Environment Variables

Alternative to config file:

```bash
export UMA_PORT=8043
export UMA_DEBUG=true
export UMA_INTERVAL_SYSTEM=5
/usr/local/bin/unraid-management-agent boot
```

### Multiple Instances (Not Supported)

Running multiple instances is not recommended and may cause conflicts.

## Next Steps

- [Quick Start Guide](quick-start.md) - Test your configuration
- [REST API Reference](../api/rest-api.md) - API endpoints
- [MQTT Integration](../integrations/mqtt.md) - Home Assistant setup
- [Troubleshooting](../troubleshooting/diagnostics.md) - Debug issues

---

**Last Updated**: January 2026  
**Configuration Version**: v2025.11.0

## PSU collection

Use `INTERVAL_PSU`, `--interval-psu` or YAML `intervals.psu` for Manual cadence. The plugin
defaults PSU Telemetry to Automatic. Dynamic mode manages cadence and explicit disable is
respected in both modes. See [PSU telemetry](psu-telemetry.md).
