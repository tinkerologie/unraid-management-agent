# Installation Guide

Install the Unraid Management Agent on your Unraid server.

## Overview

The Unraid Management Agent runs as an Unraid plugin. The plugin gives you a REST API, WebSocket events, MQTT, Prometheus metrics, and Model Context Protocol (MCP) tools.

## Prerequisites

- Unraid OS 7.2 or later on Linux/amd64
- Network access to the plugin download
- Community Applications for the **Apps** installation method

## Installation Methods

### Method 1: Community Applications (Recommended)

1. Open **Apps** in the Unraid web UI.

2. Search for **Unraid Management Agent**.

3. Click **Install**.

4. Wait until the installation is complete.

5. Check that the service is running:

   ```bash
   ps aux | grep unraid-management-agent
   ```

6. Check the health endpoint:

   ```bash
   curl http://localhost:8043/api/v1/health
   ```

### Method 2: Manual Installation

1. **Download the .plg file** from the repository:

   ```bash
   wget https://raw.githubusercontent.com/tinkerologie/unraid-management-agent/main/unraid-management-agent.plg -O /boot/config/plugins/unraid-management-agent.plg
   ```

2. **Install via command line**:

   ```bash
   /usr/local/emhttp/plugins/dynamix.plugin.manager/scripts/plugin install /boot/config/plugins/unraid-management-agent.plg
   ```

3. If the service is stopped, start it:

   ```bash
   /usr/local/emhttp/plugins/unraid-management-agent/scripts/start
   ```

### Method 3: Development Build

For developers or testing:

```bash
# Clone repository
git clone https://github.com/tinkerologie/unraid-management-agent.git
cd unraid-management-agent

# Build for Unraid (Linux/amd64)
make release

# Copy to Unraid
scp build/unraid-management-agent root@YOUR_UNRAID_IP:/usr/local/bin/

# Set permissions
ssh root@YOUR_UNRAID_IP "chmod +x /usr/local/bin/unraid-management-agent"

# Test
ssh root@YOUR_UNRAID_IP "/usr/local/bin/unraid-management-agent boot --help"
```

## Post-Installation

### 1. Verify Service Status

```bash
# Check if running
ps aux | grep unraid-management-agent

# Check logs
tail -f /var/log/unraid-management-agent.log

# Test API endpoint
curl http://localhost:8043/api/v1/health
```

Expected response:

```json
{
  "status": "ok"
}
```

### 2. Configuration

The plugin starts with these defaults:

- **Port**: 8043
- **Debug Mode**: Off
- **Collection Intervals**: See [Configuration Guide](configuration.md)

Open **Settings → Utilities → Unraid Management Agent** to change the settings. The plugin saves them in `/boot/config/plugins/unraid-management-agent/config.cfg`.

### 3. Network Access

The API listens on **port 8043**. To access from other machines:

```bash
# Test from another device
curl http://YOUR_UNRAID_IP:8043/api/v1/health
```

If a firewall blocks access, permit connections from the clients that use the API.

### 4. Security Considerations

- Keep the API on a trusted network. Use a VPN for remote access.
- Set an API token for network control. The agent permits reads without a token by default and blocks network writes.
- Use HTTPS when you send a token across an untrusted network. See [Authentication](configuration.md#authentication).

## Verification

### Quick Test

If you set an API token, send the bearer header on protected requests. See
[Authentication](configuration.md#authentication). The health endpoint is open.

```bash
# System info
curl http://localhost:8043/api/v1/system

# Array status
curl http://localhost:8043/api/v1/array

# Docker containers
curl http://localhost:8043/api/v1/docker

# Prometheus metrics
curl http://localhost:8043/metrics

# Swagger UI
open http://localhost:8043/swagger/
```

### WebSocket Test

```bash
# Install wscat (if not installed)
npm install -g wscat

# Connect to WebSocket
wscat -c ws://localhost:8043/api/v1/ws

# You should receive real-time events
```

## Updating

### Via Community Applications

1. **Plugins** → **Check for Updates**
2. If update available, click **Update**
3. Plugin will restart automatically

### Manual Update

```bash
# Download latest .plg
wget https://raw.githubusercontent.com/tinkerologie/unraid-management-agent/main/unraid-management-agent.plg -O /boot/config/plugins/unraid-management-agent.plg

# Remove old version
/usr/local/emhttp/plugins/dynamix.plugin.manager/scripts/plugin remove unraid-management-agent

# Reinstall
/usr/local/emhttp/plugins/dynamix.plugin.manager/scripts/plugin install /boot/config/plugins/unraid-management-agent.plg
```

## Uninstallation

### Via Web UI

1. **Plugins** → **Installed Plugins**
2. Find "Unraid Management Agent"
3. Click **Uninstall**

### Manual Removal

```bash
# Stop service
/usr/local/emhttp/plugins/unraid-management-agent/scripts/stop

# Remove plugin
/usr/local/emhttp/plugins/dynamix.plugin.manager/scripts/plugin remove unraid-management-agent

# Remove config (optional)
rm -rf /boot/config/plugins/unraid-management-agent
```

## Troubleshooting

### Service Won't Start

```bash
# Check logs
tail -f /var/log/unraid-management-agent.log

# Start the installed plugin
/usr/local/emhttp/plugins/unraid-management-agent/scripts/start
```

### Port Already in Use

```bash
# Check what's using port 8043
lsof -i :8043

# Change port (if needed)
# Edit /boot/config/plugins/unraid-management-agent/config.cfg
```

### API Not Responding

```bash
# Verify service is running
ps aux | grep unraid-management-agent

# Check network connectivity
curl -v http://localhost:8043/api/v1/health

# Review logs for errors
grep ERROR /var/log/unraid-management-agent.log
```

### Permission Errors

```bash
# Ensure executable permissions
chmod +x /usr/local/emhttp/plugins/unraid-management-agent/unraid-management-agent

# Check log file permissions
ls -la /var/log/unraid-management-agent.log
```

## Next Steps

- [Configuration Guide](configuration.md) - Configure collection intervals and features
- [Quick Start Guide](quick-start.md) - Learn API basics
- [REST API Reference](../api/rest-api.md) - Complete API documentation
- [WebSocket Events](../api/websocket-events.md) - Real-time monitoring

## Support

- **GitHub Issues**: <https://github.com/tinkerologie/unraid-management-agent/issues>
- **Documentation**: <https://github.com/tinkerologie/unraid-management-agent/tree/main/docs>
- **Community**: Unraid Forums
