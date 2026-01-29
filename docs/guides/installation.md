# Installation Guide

Complete installation instructions for the Unraid Management Agent.

## Overview

The Unraid Management Agent is installed as a **Community Applications** plugin and provides a REST API, WebSocket events, MQTT publishing, Prometheus metrics, and Model Context Protocol (MCP) integration for monitoring and controlling your Unraid server.

## Prerequisites

- Unraid OS 6.8.0 or newer (6.12.0+ recommended)
- Community Applications plugin installed
- Network connectivity
- Basic understanding of REST APIs (optional)

## Installation Methods

### Method 1: Community Applications (Recommended)

1. **Open Unraid Web UI** → **Plugins** → **Community Applications**

2. **Search** for "Unraid Management Agent"

3. **Click Install** and wait for completion

4. **Verify Installation**:
   ```bash
   ps aux | grep unraid-management-agent
   ```

5. **Access API**:
   ```bash
   curl http://localhost:8043/api/v1/health
   ```

### Method 2: Manual Installation

1. **Download the .plg file** from GitHub releases:
   ```bash
   wget https://github.com/ruaan-deysel/unraid-management-agent/releases/latest/download/unraid-management-agent.plg
   ```

2. **Install via command line**:
   ```bash
   /usr/local/emhttp/plugins/dynamix.plugin.manager/scripts/plugin install /boot/config/plugins/unraid-management-agent.plg
   ```

3. **Start the service**:
   ```bash
   /etc/rc.d/rc.unraid-management-agent start
   ```

### Method 3: Development Build

For developers or testing:

```bash
# Clone repository
git clone https://github.com/ruaan-deysel/unraid-management-agent.git
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

The plugin is configured via command-line arguments (managed by the plugin system). Default settings:

- **Port**: 8043
- **Debug Mode**: Off
- **Collection Intervals**: See [Configuration Guide](configuration.md)

To customize, edit `/boot/config/plugins/unraid-management-agent/unraid-management-agent.cfg` (created after first install).

### 3. Network Access

The API listens on **port 8043**. To access from other machines:

```bash
# Test from another device
curl http://YOUR_UNRAID_IP:8043/api/v1/health
```

**Firewall**: Port 8043 should be accessible by default on Unraid.

### 4. Security Considerations

- The API currently has **no authentication**
- Recommended for **internal networks only**
- Use **VPN** (WireGuard) or **reverse proxy** with authentication for external access
- Future versions will include authentication options

## Verification

### Quick Test

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
wget https://github.com/ruaan-deysel/unraid-management-agent/releases/latest/download/unraid-management-agent.plg -O /boot/config/plugins/unraid-management-agent.plg

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
/etc/rc.d/rc.unraid-management-agent stop

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

# Try manual start
/usr/local/bin/unraid-management-agent boot --debug --port 8043
```

### Port Already in Use

```bash
# Check what's using port 8043
lsof -i :8043

# Change port (if needed)
# Edit /boot/config/plugins/unraid-management-agent/unraid-management-agent.cfg
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
chmod +x /usr/local/bin/unraid-management-agent

# Check log file permissions
ls -la /var/log/unraid-management-agent.log
```

## Next Steps

- [Configuration Guide](configuration.md) - Configure collection intervals and features
- [Quick Start Guide](quick-start.md) - Learn API basics
- [REST API Reference](../api/rest-api.md) - Complete API documentation
- [WebSocket Events](../api/websocket-events.md) - Real-time monitoring

## Support

- **GitHub Issues**: https://github.com/ruaan-deysel/unraid-management-agent/issues
- **Documentation**: https://github.com/ruaan-deysel/unraid-management-agent/tree/main/docs
- **Community**: Unraid Forums

---

**Last Updated**: January 2026  
**Plugin Version**: 2025.11.0+
