# Configuration Guide

Complete guide to configuring the Unraid Management Agent.

## Overview

The agent uses command-line arguments for configuration, managed automatically by the plugin system. Most settings can be adjusted without editing files.

## Configuration File Location

**Primary Config**: `/boot/config/plugins/unraid-management-agent/unraid-management-agent.cfg`

This file is created automatically after the first installation and persists across reboots (stored on USB flash).

## Command-Line Options

### Core Settings

| Option                     | Default  | Description                                                                                        |
| -------------------------- | -------- | -------------------------------------------------------------------------------------------------- |
| `--port`                   | `8043`   | HTTP API port                                                                                      |
| `--bind-address`           | -        | IP to bind the HTTP server to (empty = all). mDNS advertises it. Loopback rejected; invalid → all. |
| `--read-only`              | `false`  | Block state-changing MCP tools (AI agents read-only; REST API unaffected)                          |
| `--debug`                  | `false`  | Enable debug logging                                                                               |
| `--mqtt-enabled`           | `false`  | Enable MQTT publishing                                                                             |
| `--mqtt-broker`            | -        | MQTT broker address (e.g., `tcp://localhost:1883`)                                                 |
| `--mqtt-topic-prefix`      | `unraid` | MQTT topic prefix                                                                                  |
| `--mqtt-username`          | -        | MQTT username (optional)                                                                           |
| `--mqtt-password`          | -        | MQTT password (optional)                                                                           |
| `--discovery-enabled`      | `true`   | Advertise the agent via mDNS for auto-discovery                                                    |
| `--discovery-service-name` | -        | Override the advertised mDNS instance name                                                         |

### Collection Intervals

Control how often data is collected (in seconds):

| Collector          | Flag                      | Default | Min | Max   |
| ------------------ | ------------------------- | ------- | --- | ----- |
| System             | `--interval-system`       | 5s      | 1s  | 3600s |
| Array              | `--interval-array`        | 10s     | 5s  | 3600s |
| Disks              | `--interval-disk`         | 30s     | 10s | 3600s |
| Docker             | `--interval-docker`       | 10s     | 5s  | 3600s |
| VMs                | `--interval-vm`           | 10s     | 5s  | 3600s |
| UPS                | `--interval-ups`          | 10s     | 5s  | 3600s |
| NUT                | `--interval-nut`          | 10s     | 5s  | 3600s |
| GPU                | `--interval-gpu`          | 10s     | 5s  | 3600s |
| Shares             | `--interval-shares`       | 60s     | 30s | 3600s |
| Network            | `--interval-network`      | 15s     | 5s  | 3600s |
| Hardware           | `--interval-hardware`     | 60s     | 30s | 3600s |
| ZFS                | `--interval-zfs`          | 30s     | 10s | 3600s |
| Notifications      | `--interval-notification` | 30s     | 10s | 3600s |
| Registration       | `--interval-registration` | 300s    | 60s | 3600s |
| Unassigned Devices | `--interval-unassigned`   | 60s     | 30s | 3600s |

**Disable a collector**: Set interval to `0`

```bash
# Example: Disable GPU collector
--interval-gpu 0
```

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

## Auto-Discovery (zeroconf/mDNS)

The agent advertises itself on the local network using zeroconf (mDNS/DNS-SD),
so integrations such as the [Home Assistant integration](https://github.com/ruaan-deysel/ha-unraid-management-agent)
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

### Authentication (Future)

Authentication is planned for future versions. Current options:

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
