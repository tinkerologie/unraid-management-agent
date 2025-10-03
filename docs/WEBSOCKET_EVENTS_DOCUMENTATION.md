# WebSocket Events Documentation

## Overview

The Unraid Management Agent provides real-time updates via WebSocket connections. This document details all available events, their data structures, frequencies, and usage patterns.

**WebSocket Endpoint**: `ws://<host>:<port>/api/v1/ws`  
**Default Port**: 8043  
**Protocol**: WebSocket (ws://)

---

## Event Architecture

### Event Flow
```
Collector → Event Bus (pubsub) → API Server Cache → WebSocket Hub → Connected Clients
```

### Event Structure
All WebSocket events follow this structure:
```json
{
  "event": "update",
  "timestamp": "2025-10-02T14:02:59.850035377+10:00",
  "data": { ... }
}
```

### Event Identification
Events do NOT have a `type` field. Event types are identified by inspecting the `data` structure and checking for specific field combinations.

---

## Available Events (9 Types)

### 1. System Update (`system_update`)

**Frequency**: Every 5 seconds  
**Collector**: `SystemCollector`  
**Topic**: `system_update`

**Identification**: Contains `hostname` AND `cpu_usage_percent`

**Data Structure**:
```json
{
  "hostname": "Tower",
  "version": "1.0.0",
  "uptime_seconds": 86400,
  "cpu_usage_percent": 12.5,
  "cpu_model": "Intel(R) Core(TM) i7-8700 CPU @ 3.20GHz",
  "cpu_cores": 6,
  "cpu_threads": 12,
  "cpu_mhz": 3200.0,
  "cpu_temp_celsius": 38.0,
  "ram_usage_percent": 45.2,
  "ram_total_bytes": 17179869184,
  "ram_used_bytes": 7767482368,
  "ram_free_bytes": 9412386816,
  "ram_buffers_bytes": 0,
  "ram_cached_bytes": 0,
  "server_model": "System Product Name",
  "bios_version": "1234",
  "bios_date": "01/01/2020",
  "motherboard_temp_celsius": 35.0,
  "fans": [
    {
      "name": "CPU Fan",
      "rpm": 1200
    }
  ],
  "timestamp": "2025-10-02T14:02:59.789972296+10:00"
}
```

**Key Fields**:
- `hostname` - Server hostname
- `cpu_usage_percent` - Overall CPU usage (0-100)
- `cpu_temp_celsius` - CPU temperature
- `ram_usage_percent` - RAM usage (0-100)
- `motherboard_temp_celsius` - Motherboard temperature
- `fans[]` - Array of fan sensors with RPM

---

### 2. Array Status Update (`array_status_update`)

**Frequency**: Every 10 seconds  
**Collector**: `ArrayCollector`  
**Topic**: `array_status_update`

**Identification**: Contains `state` AND `parity_check_status` AND `num_disks`

**Data Structure**:
```json
{
  "state": "STARTED",
  "num_disks": 8,
  "num_data_disks": 6,
  "num_parity_disks": 2,
  "sync_percent": 100.0,
  "parity_valid": true,
  "parity_check_status": "idle",
  "parity_check_running": false,
  "parity_check_progress": 0.0,
  "timestamp": "2025-10-02T14:02:59.850035377+10:00"
}
```

**Key Fields**:
- `state` - Array state: "STARTED", "STOPPED", "STARTING", "STOPPING"
- `parity_valid` - Parity validation status
- `parity_check_running` - Whether parity check is active
- `parity_check_progress` - Parity check progress (0-100)

---

### 3. Disk List Update (`disk_list_update`)

**Frequency**: Every 30 seconds  
**Collector**: `DiskCollector`  
**Topic**: `disk_list_update`

**Identification**: Contains `device` AND `mount_point`

**Data Structure** (array of disks):
```json
[
  {
    "id": "disk1",
    "device": "sda",
    "name": "disk1",
    "status": "DISK_OK",
    "size_bytes": 4000787030016,
    "used_bytes": 2000000000000,
    "free_bytes": 2000787030016,
    "temperature_celsius": 35.0,
    "smart_status": "PASSED",
    "smart_errors": 0,
    "spindown_delay": 0,
    "filesystem": "xfs",
    "mount_point": "/mnt/disk1",
    "usage_percent": 50.0,
    "power_on_hours": 12345,
    "power_cycle_count": 100,
    "read_bytes": 1234567890,
    "write_bytes": 987654321,
    "read_ops": 5000000,
    "write_ops": 4500000,
    "io_utilization_percent": 5.0,
    "timestamp": "2025-10-02T14:02:59.850035377+10:00"
  }
]
```

**Key Fields**:
- `id` - Disk identifier (disk1, disk2, parity, cache)
- `temperature_celsius` - Disk temperature (0 if spun down)
- `smart_status` - SMART health status
- `smart_errors` - Count of SMART errors
- `usage_percent` - Disk usage percentage

---

### 4. Container List Update (`container_list_update`)

**Frequency**: Every 10 seconds  
**Collector**: `DockerCollector`  
**Topic**: `container_list_update`

**Identification**: Contains `image` AND `ports` AND (`id` OR `container_id`)

**Data Structure** (array of containers):
```json
[
  {
    "id": "abc123",
    "name": "nginx",
    "image": "nginx:latest",
    "state": "running",
    "status": "Up 2 hours",
    "cpu_percent": 0.5,
    "memory_usage_bytes": 52428800,
    "memory_limit_bytes": 2147483648,
    "network_rx_bytes": 1234567,
    "network_tx_bytes": 7654321,
    "ports": [
      {
        "container_port": 80,
        "host_port": 8080,
        "protocol": "tcp"
      }
    ],
    "timestamp": "2025-10-02T14:02:59.850035377+10:00"
  }
]
```

**Key Fields**:
- `state` - Container state: "running", "paused", "stopped", "exited"
- `cpu_percent` - CPU usage percentage
- `memory_usage_bytes` - Memory usage in bytes
- `ports[]` - Port mappings

---

### 5. VM List Update (`vm_list_update`)

**Frequency**: Every 10 seconds  
**Collector**: `VMCollector`  
**Topic**: `vm_list_update`

**Identification**: Contains `state` AND `vcpus`

**Data Structure** (array of VMs):
```json
[
  {
    "id": "1",
    "name": "Ubuntu",
    "state": "running",
    "vcpus": 4,
    "memory_allocated_bytes": 4294967296,
    "memory_used_bytes": 2147483648,
    "disk_path": "/mnt/user/domains/Ubuntu/vdisk1.img",
    "disk_size_bytes": 53687091200,
    "autostart": true,
    "persistent": true,
    "timestamp": "2025-10-02T14:02:59.850035377+10:00"
  }
]
```

**Key Fields**:
- `state` - VM state: "running", "paused", "shut off", "crashed"
- `vcpus` - Number of virtual CPUs
- `memory_allocated_bytes` - Allocated memory
- `autostart` - Whether VM starts automatically

---

### 6. UPS Status Update (`ups_status_update`)

**Frequency**: Every 10 seconds  
**Collector**: `UPSCollector`  
**Topic**: `ups_status_update`

**Identification**: Contains `connected` AND `battery_charge_percent`

**Data Structure**:
```json
{
  "connected": true,
  "model": "Back-UPS RS 1500G",
  "status": "ONLINE",
  "battery_charge_percent": 100.0,
  "battery_runtime_seconds": 3600,
  "load_percent": 25.0,
  "input_voltage": 120.0,
  "output_voltage": 120.0,
  "power_watts": 150.0,
  "timestamp": "2025-10-02T14:02:59.850035377+10:00"
}
```

**Key Fields**:
- `connected` - Whether UPS is connected
- `status` - UPS status: "ONLINE", "ONBATT", "LOWBATT"
- `battery_charge_percent` - Battery charge (0-100)
- `battery_runtime_seconds` - Estimated runtime
- `load_percent` - Load percentage

---

### 7. GPU Update (`gpu_update`)

**Frequency**: Every 10 seconds  
**Collector**: `GPUCollector`  
**Topic**: `gpu_metrics_update`

**Identification**: Contains `available` AND `driver_version` AND `utilization_gpu_percent`

**Data Structure** (array of GPUs):
```json
[
  {
    "available": true,
    "name": "Intel UHD Graphics 630",
    "driver_version": "6.12.24-Unraid",
    "temperature_celsius": 0,
    "cpu_temperature_celsius": 34,
    "utilization_gpu_percent": 0,
    "utilization_memory_percent": 0,
    "memory_total_bytes": 0,
    "memory_used_bytes": 0,
    "power_draw_watts": 0,
    "timestamp": "2025-10-02T14:02:59.789972296+10:00"
  }
]
```

**Key Fields**:
- `available` - Whether GPU is available
- `name` - GPU model name
- `driver_version` - GPU driver version
- `cpu_temperature_celsius` - CPU temp (for iGPUs)
- `utilization_gpu_percent` - GPU utilization
- `power_draw_watts` - Power consumption

---

### 8. Network List Update (`network_list_update`)

**Frequency**: Every 15 seconds  
**Collector**: `NetworkCollector`  
**Topic**: `network_list_update`

**Identification**: Contains `mac_address` AND `bytes_received`

**Data Structure** (array of interfaces):
```json
[
  {
    "name": "eth0",
    "mac_address": "00:11:22:33:44:55",
    "ip_address": "192.168.1.100",
    "speed_mbps": 1000,
    "state": "up",
    "bytes_received": 1234567890,
    "bytes_sent": 987654321,
    "packets_received": 5000000,
    "packets_sent": 4500000,
    "errors_received": 0,
    "errors_sent": 0,
    "timestamp": "2025-10-02T14:02:59.850035377+10:00"
  }
]
```

**Key Fields**:
- `name` - Interface name (eth0, br0, etc.)
- `state` - Interface state: "up", "down"
- `speed_mbps` - Link speed in Mbps
- `bytes_received` / `bytes_sent` - Traffic counters

---

### 9. Share List Update (`share_list_update`)

**Frequency**: Every 60 seconds  
**Collector**: `ShareCollector`  
**Topic**: `share_list_update`

**Identification**: Contains `name` AND `path` AND `size_bytes`

**Data Structure** (array of shares):
```json
[
  {
    "name": "appdata",
    "path": "/mnt/user/appdata",
    "size_bytes": 107374182400,
    "used_bytes": 53687091200,
    "free_bytes": 53687091200,
    "usage_percent": 50.0,
    "timestamp": "2025-10-02T14:02:59.850035377+10:00"
  }
]
```

**Key Fields**:
- `name` - Share name
- `path` - Mount path
- `size_bytes` - Total size
- `usage_percent` - Usage percentage

---

## Event Frequency Summary

| Event Type | Interval | Collector |
|------------|----------|-----------|
| system_update | 5s | SystemCollector |
| array_status_update | 10s | ArrayCollector |
| disk_list_update | 30s | DiskCollector |
| container_list_update | 10s | DockerCollector |
| vm_list_update | 10s | VMCollector |
| ups_status_update | 10s | UPSCollector |
| gpu_update | 10s | GPUCollector |
| network_list_update | 15s | NetworkCollector |
| share_list_update | 60s | ShareCollector |

---

## Connection Management

### WebSocket Settings
- **Ping Interval**: 30 seconds
- **Max Clients**: 10 concurrent connections
- **Buffer Size**: 256 messages
- **Read Deadline**: 60 seconds

### Reconnection Strategy
The Home Assistant integration uses exponential backoff for reconnections:
- Delays: 1s, 2s, 4s, 8s, 16s, 32s, 60s (max)
- Max retries: 10
- Automatic reconnection on disconnect

---

## Usage Examples

### Python WebSocket Client
```python
import asyncio
import aiohttp

async def monitor_events():
    async with aiohttp.ClientSession() as session:
        async with session.ws_connect('ws://192.168.1.100:8043/api/v1/ws') as ws:
            async for msg in ws:
                if msg.type == aiohttp.WSMsgType.TEXT:
                    data = msg.json()
                    event_type = identify_event_type(data['data'])
                    print(f"Event: {event_type}")
                    print(f"Data: {data['data']}")

def identify_event_type(data):
    if isinstance(data, list):
        data = data[0] if data else {}
    
    if "hostname" in data and "cpu_usage_percent" in data:
        return "system_update"
    elif "state" in data and "parity_check_status" in data:
        return "array_status_update"
    # ... (see websocket_client.py for full implementation)
    
    return "unknown"

asyncio.run(monitor_events())
```

### JavaScript WebSocket Client
```javascript
const ws = new WebSocket('ws://192.168.1.100:8043/api/v1/ws');

ws.onmessage = (event) => {
  const message = JSON.parse(event.data);
  const eventType = identifyEventType(message.data);
  console.log(`Event: ${eventType}`, message.data);
};

function identifyEventType(data) {
  if (Array.isArray(data)) data = data[0] || {};
  
  if (data.hostname && data.cpu_usage_percent) return 'system_update';
  if (data.state && data.parity_check_status) return 'array_status_update';
  // ... (add other checks)
  
  return 'unknown';
}
```

---

## Testing & Monitoring

### Test Script
A Python test script is available at `test_websocket.py`:
```bash
python test_websocket.py ws://192.168.1.100:8043/api/v1/ws 120
```

This will:
- Connect to the WebSocket
- Monitor events for 120 seconds
- Count event types
- Save examples to `websocket_test_results.json`

### Expected Event Counts (2 minutes)
- system_update: ~24 events (every 5s)
- array_status_update: ~12 events (every 10s)
- disk_list_update: ~4 events (every 30s)
- container_list_update: ~12 events (every 10s)
- vm_list_update: ~12 events (every 10s)
- ups_status_update: ~12 events (every 10s)
- gpu_update: ~12 events (every 10s)
- network_list_update: ~8 events (every 15s)
- share_list_update: ~2 events (every 60s)

---

## Status

✅ **All 9 event types documented**  
✅ **Event identification logic defined**  
✅ **Data structures documented**  
✅ **Frequencies documented**  
✅ **Usage examples provided**  
✅ **Testing procedures documented**

**Last Updated**: 2025-10-02  
**Version**: 1.1.0

