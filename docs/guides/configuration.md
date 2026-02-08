# Configuration Guide

Complete guide to configuring the Unraid Management Agent.

## Overview

The agent uses command-line arguments for configuration, managed automatically by the plugin system. Most settings can be adjusted without editing files.

## Configuration File Location

**Primary Config**: `/boot/config/plugins/unraid-management-agent/unraid-management-agent.cfg`

This file is created automatically after the first installation and persists across reboots (stored on USB flash).

## Command-Line Options

### Core Settings

| Option | Default | Description |
|--------|---------|-------------|
| `--port` | `8043` | HTTP API port |
| `--debug` | `false` | Enable debug logging |
| `--mqtt-enabled` | `false` | Enable MQTT publishing |
| `--mqtt-broker` | - | MQTT broker address (e.g., `tcp://localhost:1883`) |
| `--mqtt-topic-prefix` | `unraid` | MQTT topic prefix |
| `--mqtt-username` | - | MQTT username (optional) |
| `--mqtt-password` | - | MQTT password (optional) |

### Collection Intervals

Control how often data is collected (in seconds):

| Collector | Flag | Default | Min | Max |
|-----------|------|---------|-----|-----|
| System | `--interval-system` | 5s | 1s | 3600s |
| Array | `--interval-array` | 10s | 5s | 3600s |
| Disks | `--interval-disk` | 30s | 10s | 3600s |
| Docker | `--interval-docker` | 10s | 5s | 3600s |
| VMs | `--interval-vm` | 10s | 5s | 3600s |
| UPS | `--interval-ups` | 10s | 5s | 3600s |
| NUT | `--interval-nut` | 10s | 5s | 3600s |
| GPU | `--interval-gpu` | 10s | 5s | 3600s |
| Shares | `--interval-shares` | 60s | 30s | 3600s |
| Network | `--interval-network` | 15s | 5s | 3600s |
| Hardware | `--interval-hardware` | 60s | 30s | 3600s |
| ZFS | `--interval-zfs` | 30s | 10s | 3600s |
| Notifications | `--interval-notification` | 30s | 10s | 3600s |
| Registration | `--interval-registration` | 300s | 60s | 3600s |
| Unassigned Devices | `--interval-unassigned` | 60s | 30s | 3600s |

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
