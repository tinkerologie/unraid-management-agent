# Quick Start Guide

Get up and running with the Unraid Management Agent in under 5 minutes.

## Installation

### Via Plugin URL (Recommended)

1. Open Unraid Web UI
2. Navigate to **Plugins** → **Install Plugin**
3. Paste plugin URL:
   ```
   https://raw.githubusercontent.com/ruaan-deysel/unraid-management-agent/main/unraid-management-agent.plg
   ```
4. Click **Install**
5. Wait for installation to complete
6. Service starts automatically on port 8043

## Verify Installation

### Check Service Status

```bash
# Check if service is running
ps aux | grep unraid-management-agent

# Check logs
tail -f /var/log/unraid-management-agent.log
```

### Test API

```bash
# Health check
curl http://localhost:8043/api/v1/health

# System information
curl http://localhost:8043/api/v1/system

# Array status
curl http://localhost:8043/api/v1/array
```

Expected response:
```json
{
  "status": "ok"
}
```

## First API Calls

### Get System Information

```bash
curl http://localhost:8043/api/v1/system | jq
```

### List Docker Containers

```bash
curl http://localhost:8043/api/v1/docker | jq
```

### View Network Interfaces

```bash
curl http://localhost:8043/api/v1/network | jq
```

### Check Disk Health

```bash
curl http://localhost:8043/api/v1/disks | jq
```

## WebSocket Connection

### JavaScript Example

```javascript
const ws = new WebSocket('ws://YOUR_UNRAID_IP:8043/api/v1/ws');

ws.onopen = () => {
  console.log('Connected to Unraid Management Agent');
};

ws.onmessage = (event) => {
  const data = JSON.parse(event.data);
  console.log('Event received:', data.event, data.data);
};

ws.onerror = (error) => {
  console.error('WebSocket error:', error);
};
```

### Python Example

```python
import websocket
import json

def on_message(ws, message):
    data = json.loads(message)
    print(f"Event: {data['event']}")
    print(f"Data: {data['data']}")

def on_open(ws):
    print("Connected to Unraid Management Agent")

ws = websocket.WebSocketApp(
    "ws://YOUR_UNRAID_IP:8043/api/v1/ws",
    on_message=on_message,
    on_open=on_open
)

ws.run_forever()
```

## Common Operations

### Start/Stop Docker Container

```bash
# Start container
curl -X POST http://localhost:8043/api/v1/docker/nginx/start

# Stop container
curl -X POST http://localhost:8043/api/v1/docker/nginx/stop

# Restart container
curl -X POST http://localhost:8043/api/v1/docker/nginx/restart
```

### Control Virtual Machines

```bash
# Start VM
curl -X POST http://localhost:8043/api/v1/vm/Ubuntu/start

# Stop VM
curl -X POST http://localhost:8043/api/v1/vm/Ubuntu/stop

# Pause VM
curl -X POST http://localhost:8043/api/v1/vm/Ubuntu/pause
```

### Array Operations

```bash
# Start array
curl -X POST http://localhost:8043/api/v1/array/start

# Stop array
curl -X POST http://localhost:8043/api/v1/array/stop

# Start parity check
curl -X POST http://localhost:8043/api/v1/array/parity-check/start
```

## Prometheus Metrics

Access metrics for Grafana:

```bash
curl http://localhost:8043/metrics
```

## Configuration

### Change Port

1. Navigate to **Settings** → **Unraid Management Agent**
2. Change **Port** setting
3. Click **Apply** (service restarts automatically)

### Adjust Collection Intervals

1. Navigate to **Settings** → **Unraid Management Agent**
2. Adjust intervals for each collector type
3. Click **Apply**

Lower intervals = more frequent updates but higher CPU/power usage.

## Troubleshooting

### No Response from API

```bash
# Check service status
ps aux | grep unraid-management-agent

# View logs
tail -100 /var/log/unraid-management-agent.log

# Restart service
/usr/local/emhttp/plugins/unraid-management-agent/scripts/stop
/usr/local/emhttp/plugins/unraid-management-agent/scripts/start
```

### Empty Data Returned

- Wait 30-60 seconds after install for first data collection
- Check collector intervals in settings
- Verify permissions: `ls -l /var/run/docker.sock`

### Port Already in Use

```bash
# Check what's using port 8043
netstat -tulpn | grep 8043

# Change port in plugin settings
```

## Next Steps

- [REST API Reference](../api/rest-api.md) - Complete API documentation
- [WebSocket Events](../api/websocket-events.md) - Real-time events guide
- [Integrations](../integrations/mcp.md) - Connect with other tools
- [Configuration Guide](configuration.md) - Advanced settings

## Need Help?

- **Issues**: [GitHub Issues](https://github.com/ruaan-deysel/unraid-management-agent/issues)
- **Documentation**: [Full Documentation](../README.md)
- **Community**: Unraid Community Forums

---

**Last Updated**: January 2026
