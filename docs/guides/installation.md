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

1. Check the service process:

   ```bash
   pidof unraid-management-agent
   ```

2. Read the last 100 lines of the agent log:

   ```bash
   tail -n 100 /var/log/unraid-management-agent.log
   ```

3. Send a request to the health endpoint:

   ```bash
   curl --fail --silent --show-error http://localhost:8043/api/v1/health
   ```

The health endpoint returns this response:

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

To change the settings, open **Settings → Utilities → Unraid Management Agent**.

The plugin saves its settings in `/boot/config/plugins/unraid-management-agent/config.cfg`.

### 3. Network Access

The default API port is `8043`.

From another machine, send a request to the health endpoint:

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

Use the [quick start examples](quick-start.md#first-api-calls) to read system, disk, network, and container data.
For a list of available operations, open `http://localhost:8043/swagger/` in a browser on the server.

### WebSocket Test

This example uses an agent without an API token. If you set a token, send the bearer header.
See [Authentication](configuration.md#authentication).

```bash
# Install wscat (if not installed)
npm install -g wscat

# Connect to WebSocket
wscat -c ws://localhost:8043/api/v1/ws

# You should receive real-time events
```

## Updating

### Via Community Applications

1. Open **Plugins**.
2. Click **Check for Updates**.
3. If an update is available, click **Update**.

The plugin restarts a running service after the update.
The update procedure keeps the plugin settings.

## Uninstallation

> [!CAUTION]
> Back up the plugin settings before you uninstall the plugin.
> Both procedures below delete `/boot/config/plugins/unraid-management-agent`.
> The backup can contain credentials. Keep it private.

### Via the Unraid web UI

1. Open **Plugins → Installed Plugins**.
2. Find **Unraid Management Agent**.
3. Click **Uninstall**.

### Manual Removal

1. Stop the service:

   ```bash
   /usr/local/emhttp/plugins/unraid-management-agent/scripts/stop
   ```

2. Remove the plugin:

   ```bash
   /usr/local/emhttp/plugins/dynamix.plugin.manager/scripts/plugin remove unraid-management-agent
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
