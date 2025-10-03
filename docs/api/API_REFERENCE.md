# API Reference Guide

Complete reference for all Unraid Management Agent API endpoints.

**Base URL**: `http://YOUR_UNRAID_IP:8043/api/v1`  
**Version**: 1.0.0  
**Total Endpoints**: 46

---

## Table of Contents

- [Authentication](#authentication)
- [Response Format](#response-format)
- [Error Handling](#error-handling)
- [System & Health](#system--health)
- [Array Management](#array-management)
- [Disks](#disks)
- [Shares](#shares)
- [Docker Containers](#docker-containers)
- [Virtual Machines](#virtual-machines)
- [Hardware](#hardware)
- [Configuration](#configuration)
- [WebSocket](#websocket)

---

## Authentication

Currently, the API does not require authentication. All endpoints are accessible without credentials.

**Security Note**: Ensure the API is only accessible on trusted networks or implement network-level security (firewall, VPN, etc.).

---

## Response Format

### Success Response

```json
{
  "data": { ... },
  "timestamp": "2025-10-03T13:41:13.631962129+10:00"
}
```

### Error Response

```json
{
  "success": false,
  "message": "Error description",
  "timestamp": "2025-10-03T13:41:13.631962129+10:00"
}
```

---

## Error Handling

### HTTP Status Codes

- `200 OK` - Request successful
- `400 Bad Request` - Invalid request parameters
- `404 Not Found` - Resource not found
- `500 Internal Server Error` - Server error

### Error Messages

All errors include a descriptive message in the response body.

---

## System & Health

### GET /health

Health check endpoint.

**Response**:
```json
{
  "status": "ok"
}
```

**Example**:
```bash
curl http://192.168.20.21:8043/api/v1/health
```

---

### GET /system

Get system information including CPU, memory, and uptime.

**Response**:
```json
{
  "hostname": "Cube",
  "uptime_seconds": 1234567,
  "cpu_usage_percent": 15.5,
  "memory_total_bytes": 17179869184,
  "memory_used_bytes": 8589934592,
  "memory_usage_percent": 50.0,
  "load_average": [1.5, 1.2, 1.0],
  "timestamp": "2025-10-03T13:41:13+10:00"
}
```

**Example**:
```bash
curl http://192.168.20.21:8043/api/v1/system
```

---

## Array Management

### GET /array

Get array status and information.

**Response**:
```json
{
  "state": "STARTED",
  "total_disks": 5,
  "data_disks": 3,
  "parity_disks": 1,
  "cache_disks": 1,
  "size_bytes": 16000000000000,
  "used_bytes": 8000000000000,
  "free_bytes": 8000000000000,
  "usage_percent": 50.0,
  "timestamp": "2025-10-03T13:41:13+10:00"
}
```

---

### POST /array/start

Start the Unraid array.

**Response**:
```json
{
  "success": true,
  "message": "Array started successfully",
  "timestamp": "2025-10-03T13:41:13+10:00"
}
```

**Example**:
```bash
curl -X POST http://192.168.20.21:8043/api/v1/array/start
```

---

### POST /array/stop

Stop the Unraid array.

**Response**:
```json
{
  "success": true,
  "message": "Array stopped successfully",
  "timestamp": "2025-10-03T13:41:13+10:00"
}
```

---

### POST /array/parity-check/start

Start a parity check.

**Query Parameters**:
- `correcting` (optional) - Set to `true` for correcting parity check, `false` for read-only check

**Example**:
```bash
# Read-only parity check
curl -X POST http://192.168.20.21:8043/api/v1/array/parity-check/start

# Correcting parity check
curl -X POST "http://192.168.20.21:8043/api/v1/array/parity-check/start?correcting=true"
```

---

### POST /array/parity-check/stop

Stop the current parity check.

---

### POST /array/parity-check/pause

Pause the current parity check.

---

### POST /array/parity-check/resume

Resume a paused parity check.

---

### GET /array/parity-check/history

Get parity check history.

**Response**:
```json
{
  "records": [
    {
      "action": "Parity-Check",
      "date": "2025-06-30T10:29:12+10:00",
      "duration_seconds": 131131,
      "speed_mbps": 123.4,
      "status": "OK",
      "errors": 0,
      "size_bytes": 16000000000000
    }
  ],
  "timestamp": "2025-10-03T13:41:13+10:00"
}
```

---

## Disks

### GET /disks

List all disks in the system.

**Response**:
```json
[
  {
    "id": "WUH721816ALE6L4_2CGV0URP",
    "device": "sdb",
    "name": "parity",
    "role": "parity",
    "size_bytes": 16000000000000,
    "used_bytes": 0,
    "free_bytes": 16000000000000,
    "temperature_celsius": 0,
    "spin_state": "standby",
    "serial_number": "2CGV0URP",
    "model": "WDC WUH721816ALE6L4",
    "filesystem": "xfs",
    "status": "DISK_OK",
    "timestamp": "2025-10-03T13:41:13+10:00"
  }
]
```

---

### GET /disks/{id}

Get a single disk by ID, device name, or disk name.

**Path Parameters**:
- `id` - Disk ID, device (e.g., `sdb`), or name (e.g., `parity`, `disk1`, `cache`)

**Example**:
```bash
# By device
curl http://192.168.20.21:8043/api/v1/disks/sdb

# By name
curl http://192.168.20.21:8043/api/v1/disks/parity

# By ID
curl http://192.168.20.21:8043/api/v1/disks/WUH721816ALE6L4_2CGV0URP
```

**Response**: Same as single disk object from `/disks` endpoint

---

## Shares

### GET /shares

List all user shares.

**Response**:
```json
[
  {
    "name": "appdata",
    "size_bytes": 100000000000,
    "used_bytes": 50000000000,
    "free_bytes": 50000000000,
    "usage_percent": 50.0,
    "timestamp": "2025-10-03T13:41:13+10:00"
  }
]
```

---

### GET /shares/{name}/config

Get share configuration.

**Path Parameters**:
- `name` - Share name

**Response**:
```json
{
  "name": "appdata",
  "allocator": "highwater",
  "floor": "50000000",
  "use_cache": "only",
  "export": "e",
  "security": "public",
  "timestamp": "2025-10-03T13:41:13+10:00"
}
```

---

### POST /shares/{name}/config

Update share configuration.

**Request Body**:
```json
{
  "allocator": "highwater",
  "floor": "50000000",
  "use_cache": "only"
}
```

**Response**:
```json
{
  "success": true,
  "message": "Share configuration updated successfully",
  "timestamp": "2025-10-03T13:41:13+10:00"
}
```

---

## Docker Containers

### GET /docker

List all Docker containers.

**Response**:
```json
[
  {
    "id": "fedcb3e1ba1f",
    "name": "jackett",
    "image": "linuxserver/jackett:latest",
    "state": "running",
    "status": "Up 9 hours",
    "cpu_usage_percent": 0.5,
    "memory_usage_bytes": 104857600,
    "network_rx_bytes": 1000000,
    "network_tx_bytes": 500000,
    "timestamp": "2025-10-03T13:41:13+10:00"
  }
]
```

---

### GET /docker/{id}

Get a single container by ID or name.

**Path Parameters**:
- `id` - Container ID or name

**Example**:
```bash
curl http://192.168.20.21:8043/api/v1/docker/jackett
```

---

### POST /docker/{id}/start

Start a Docker container.

---

### POST /docker/{id}/stop

Stop a Docker container.

---

### POST /docker/{id}/restart

Restart a Docker container.

---

### POST /docker/{id}/pause

Pause a Docker container.

---

### POST /docker/{id}/unpause

Unpause a Docker container.

---

## Virtual Machines

### GET /vm

List all virtual machines.

**Response**:
```json
[
  {
    "id": "windows-10",
    "name": "Windows 10",
    "state": "running",
    "cpu_count": 4,
    "memory_bytes": 8589934592,
    "timestamp": "2025-10-03T13:41:13+10:00"
  }
]
```

---

### GET /vm/{id}

Get a single VM by ID or name.

---

### POST /vm/{id}/start

Start a virtual machine.

---

### POST /vm/{id}/stop

Stop a virtual machine (graceful shutdown).

---

### POST /vm/{id}/restart

Restart a virtual machine.

---

### POST /vm/{id}/pause

Pause a virtual machine.

---

### POST /vm/{id}/resume

Resume a paused virtual machine.

---

### POST /vm/{id}/hibernate

Hibernate a virtual machine.

---

### POST /vm/{id}/force-stop

Force stop a virtual machine (immediate shutdown).

---

## Hardware

### GET /ups

Get UPS status and information.

**Response**:
```json
{
  "status": "ONLINE",
  "battery_charge_percent": 100,
  "battery_runtime_seconds": 3600,
  "load_percent": 25,
  "input_voltage": 230,
  "output_voltage": 230,
  "timestamp": "2025-10-03T13:41:13+10:00"
}
```

---

### GET /gpu

Get GPU information and metrics.

**Response**:
```json
[
  {
    "id": "0",
    "name": "Intel UHD Graphics 770",
    "vendor": "Intel",
    "temperature_celsius": 45,
    "usage_percent": 10,
    "memory_total_bytes": 8589934592,
    "memory_used_bytes": 1073741824,
    "timestamp": "2025-10-03T13:41:13+10:00"
  }
]
```

---

### GET /network

Get network interfaces and statistics.

**Response**:
```json
[
  {
    "interface": "eth0",
    "ip_address": "192.168.20.21",
    "mac_address": "00:11:22:33:44:55",
    "rx_bytes": 1000000000,
    "tx_bytes": 500000000,
    "rx_packets": 1000000,
    "tx_packets": 500000,
    "timestamp": "2025-10-03T13:41:13+10:00"
  }
]
```

---

## Configuration

### GET /settings/system

Get system settings.

**Response**:
```json
{
  "server_name": "Cube",
  "description": "Home Server",
  "security_mode": "user",
  "timestamp": "2025-10-03T13:41:13+10:00"
}
```

---

### POST /settings/system

Update system settings.

**Request Body**:
```json
{
  "description": "Updated description"
}
```

---

### GET /settings/docker

Get Docker settings.

**Response**:
```json
{
  "enabled": true,
  "image_path": "/mnt/cache/system/docker.img",
  "custom_networks": ["eth1"],
  "timestamp": "2025-10-03T13:41:13+10:00"
}
```

---

### GET /settings/vm

Get VM Manager settings.

**Response**:
```json
{
  "enabled": true,
  "pci_devices": ["0000:00:02.0"],
  "usb_devices": [],
  "timestamp": "2025-10-03T13:41:13+10:00"
}
```

---

### GET /settings/disks

Get disk settings including spindown delay.

**Response**:
```json
{
  "spindown_delay_minutes": 30,
  "start_array": true,
  "spinup_groups": false,
  "shutdown_timeout_seconds": 90,
  "default_filesystem": "xfs",
  "timestamp": "2025-10-03T13:41:13+10:00"
}
```

**Use Case**: Home Assistant can use `spindown_delay_minutes` to avoid waking disks with SMART queries.

---

### GET /network/{interface}/config

Get network interface configuration.

**Path Parameters**:
- `interface` - Interface name (e.g., `eth0`, `bond0`)

---

## WebSocket

### WebSocket /ws

Real-time event stream.

**URL**: `ws://YOUR_UNRAID_IP:8043/api/v1/ws`

**Events**:
- `system` - System metrics updates
- `array` - Array status changes
- `disk` - Disk status changes
- `docker` - Docker container events
- `vm` - VM state changes
- `ups` - UPS status updates
- `gpu` - GPU metrics updates
- `network` - Network statistics updates

**Example Event**:
```json
{
  "type": "system",
  "data": {
    "cpu_usage_percent": 15.5,
    "memory_usage_percent": 50.0
  },
  "timestamp": "2025-10-03T13:41:13+10:00"
}
```

See [WebSocket Events Documentation](../WEBSOCKET_EVENTS_DOCUMENTATION.md) for complete details.

---

## Rate Limiting

Currently, there is no rate limiting implemented. Use responsibly to avoid overloading the Unraid server.

---

## Best Practices

1. **Use WebSocket for real-time data** - More efficient than polling
2. **Cache responses** - Reduce API calls by caching data
3. **Handle errors gracefully** - Always check response status
4. **Respect spindown delay** - Use `/settings/disks` to avoid waking disks
5. **Use specific endpoints** - Use `/disks/{id}` instead of `/disks` when possible

---

**Last Updated**: 2025-10-03  
**API Version**: 1.0.0

