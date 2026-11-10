# Quick Start Guide

Install the agent and make your first API requests.

## Installation

The [installation guide](installation.md) describes the recommended Community Applications method.

### Via Plugin URL

1. Open the Unraid web UI.
2. Open **Plugins → Install Plugin**.
3. Paste this URL:

   ```
   https://raw.githubusercontent.com/tinkerologie/unraid-management-agent/main/unraid-management-agent.plg
   ```

4. Click **Install**.
5. Wait until the installation is complete.

The service uses port `8043` by default.

## Verify Installation

### Check Service Status

```bash
# Check if service is running
ps aux | grep unraid-management-agent

# Check logs
tail -f /var/log/unraid-management-agent.log
```

### Test API

1. Send a request to the health endpoint:

   ```bash
   curl --fail --silent --show-error http://localhost:8043/api/v1/health
   ```

2. Make sure that the response contains `"status":"ok"`.

The health endpoint does not require an API token. If you set a token, protected requests require it.
See [Authentication](configuration.md#authentication).

## First API Calls

Run these examples on the Unraid server. If the agent listens on a different address, replace `localhost`.

### Set the request header

> [!WARNING]
> Do not send an API token over an untrusted network without HTTPS.
> A device on the network can read the token in an HTTP request.

If you set an API token, use these steps to read it into a Bash variable.

1. Run this command in Bash:

   ```bash
   read -rs UMA_TOKEN
   ```

2. Paste the API token.
3. Press Enter.

The examples that follow use this token. If the agent has no API token, omit the bearer header for read requests.

### Get System Information

```bash
curl -H "Authorization: Bearer ${UMA_TOKEN}" http://localhost:8043/api/v1/system
```

### List Docker Containers

```bash
curl -H "Authorization: Bearer ${UMA_TOKEN}" http://localhost:8043/api/v1/docker
```

### View Network Interfaces

```bash
curl -H "Authorization: Bearer ${UMA_TOKEN}" http://localhost:8043/api/v1/network
```

### Check Disk Health

```bash
curl -H "Authorization: Bearer ${UMA_TOKEN}" http://localhost:8043/api/v1/disks
```

## WebSocket Connection

These examples use an agent without an API token. For an authenticated connection, see
[Authentication](configuration.md#authentication).

### JavaScript Example

```javascript
const ws = new WebSocket("ws://YOUR_UNRAID_IP:8043/api/v1/ws");

ws.onopen = () => {
  console.log("Connected to Unraid Management Agent");
};

ws.onmessage = (event) => {
  const data = JSON.parse(event.data);
  console.log("Event received:", data.event, data.data);
};

ws.onerror = (error) => {
  console.error("WebSocket error:", error);
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

Before you send a control request, set an API token. Use the token from [Set the request header](#set-the-request-header).

### Start/Stop Docker Container

Replace the example container ID with the ID from the Docker list.
Use a hexadecimal ID with 12 or 64 characters.

```bash
# Start container
curl -X POST -H "Authorization: Bearer ${UMA_TOKEN}" http://localhost:8043/api/v1/docker/fedcb3e1ba1f/start

# Stop container
curl -X POST -H "Authorization: Bearer ${UMA_TOKEN}" http://localhost:8043/api/v1/docker/fedcb3e1ba1f/stop

# Restart container
curl -X POST -H "Authorization: Bearer ${UMA_TOKEN}" http://localhost:8043/api/v1/docker/fedcb3e1ba1f/restart
```

### Control Virtual Machines

```bash
# Start VM
curl -X POST -H "Authorization: Bearer ${UMA_TOKEN}" http://localhost:8043/api/v1/vm/Ubuntu/start

# Stop VM
curl -X POST -H "Authorization: Bearer ${UMA_TOKEN}" http://localhost:8043/api/v1/vm/Ubuntu/stop

# Pause VM
curl -X POST -H "Authorization: Bearer ${UMA_TOKEN}" http://localhost:8043/api/v1/vm/Ubuntu/pause
```

### Array Operations

```bash
# Start array
curl -X POST -H "Authorization: Bearer ${UMA_TOKEN}" http://localhost:8043/api/v1/array/start

# Stop array
curl -X POST -H "Authorization: Bearer ${UMA_TOKEN}" http://localhost:8043/api/v1/array/stop

# Start parity check
curl -X POST -H "Authorization: Bearer ${UMA_TOKEN}" http://localhost:8043/api/v1/array/parity-check/start
```

## Prometheus Metrics

Read the metrics that the agent provides for Prometheus and Grafana:

```bash
curl -H "Authorization: Bearer ${UMA_TOKEN}" http://localhost:8043/metrics
```

## Configuration

### Change Port

1. Open **Settings → Utilities → Unraid Management Agent**.
2. Change **API Port**.
3. Click **Apply**.

The plugin restarts a running service.

### Adjust Collection Intervals

1. Open **Settings → Utilities → Unraid Management Agent**.
2. If you want to set intervals, turn off **Dynamic collection**.
3. Set the collector intervals.
4. Click **Apply**.

Shorter intervals increase the update rate. They can also increase CPU and power use.

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

1. Wait for the collector to complete its first cycle.
2. Check the collector interval in the settings.
3. For Docker data, check the socket permissions with `ls -l /var/run/docker.sock`.

### Port Already in Use

```bash
# Check what's using port 8043
netstat -tulpn | grep 8043

# Change port in plugin settings
```

## Next Steps

- [REST API reference](../api/rest-api.md)
- [WebSocket events](../api/websocket-events.md)
- [Integrations](../integrations/README.md)
- [Configuration guide](configuration.md)

## Need Help?

- **Issues**: [GitHub Issues](https://github.com/tinkerologie/unraid-management-agent/issues)
- **Documentation**: [Full Documentation](../README.md)
- **Community**: Unraid Community Forums
