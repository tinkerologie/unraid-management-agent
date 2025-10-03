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
- `409 Conflict` - Resource state conflict (e.g., starting already-started array)
- `500 Internal Server Error` - Server error

### Error Response Format

All errors follow this structure:

```json
{
  "success": false,
  "error_code": "ERROR_CODE_NAME",
  "message": "Human-readable error description",
  "details": {
    "additional": "context"
  },
  "timestamp": "2025-10-03T13:41:13+10:00"
}
```

### Common Error Codes

| Error Code | HTTP Status | Description | Resolution |
|------------|-------------|-------------|------------|
| `ARRAY_ALREADY_STARTED` | 409 | Array is already running | Check array status before starting |
| `ARRAY_ALREADY_STOPPED` | 409 | Array is already stopped | Check array status before stopping |
| `ARRAY_NOT_STARTED` | 400 | Array must be started for this operation | Start the array first |
| `PARITY_CHECK_RUNNING` | 409 | Parity check is already running | Stop current check before starting new one |
| `PARITY_CHECK_NOT_RUNNING` | 400 | No parity check is running | Start a parity check first |
| `DISK_NOT_FOUND` | 404 | Disk ID/name not found | Verify disk exists using GET /disks |
| `CONTAINER_NOT_FOUND` | 404 | Docker container not found | Verify container name/ID using GET /docker |
| `CONTAINER_ALREADY_RUNNING` | 409 | Container is already running | Check container state first |
| `CONTAINER_ALREADY_STOPPED` | 409 | Container is already stopped | Check container state first |
| `VM_NOT_FOUND` | 404 | Virtual machine not found | Verify VM name/ID using GET /vm |
| `VM_ALREADY_RUNNING` | 409 | VM is already running | Check VM state first |
| `VM_ALREADY_STOPPED` | 409 | VM is already stopped | Check VM state first |
| `SHARE_NOT_FOUND` | 404 | Share not found | Verify share exists using GET /shares |
| `NETWORK_INTERFACE_NOT_FOUND` | 404 | Network interface not found | Verify interface using GET /network |
| `VALIDATION_ERROR` | 400 | Invalid request parameters | Check request body format and values |
| `INTERNAL_ERROR` | 500 | Server error | Check server logs for details |

### Validation Error Example

```json
{
  "success": false,
  "error_code": "VALIDATION_ERROR",
  "message": "Invalid request parameters",
  "errors": [
    {
      "field": "allocator",
      "message": "Must be one of: highwater, mostfree, fillup",
      "received": "invalid_value"
    }
  ],
  "timestamp": "2025-10-03T13:41:13+10:00"
}
```

### Error Handling Best Practices

1. **Always check HTTP status code** - Don't rely solely on response body
2. **Handle specific error codes** - Different errors require different actions
3. **Implement retry logic** - For 500 errors, retry with exponential backoff
4. **Log error details** - Include error_code and timestamp for debugging
5. **Validate before sending** - Check parameters client-side to avoid validation errors

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

**Response (Success)**:
```json
{
  "success": true,
  "message": "Array stopped successfully",
  "timestamp": "2025-10-03T13:41:13+10:00"
}
```

**Response (Error - Array Already Stopped)**:
```json
{
  "success": false,
  "error_code": "ARRAY_ALREADY_STOPPED",
  "message": "Cannot stop array: array is already in STOPPED state",
  "details": {
    "current_state": "STOPPED"
  },
  "timestamp": "2025-10-03T13:41:13+10:00"
}
```

**Example**:
```bash
curl -X POST http://192.168.20.21:8043/api/v1/array/stop
```

---

### POST /array/parity-check/start

Start a parity check.

**Query Parameters**:

| Parameter | Type | Required | Description | Valid Values | Default |
|-----------|------|----------|-------------|--------------|---------|
| `correcting` | boolean | No | Whether to perform correcting parity check | `true`, `false` | `false` |

**Response (Success)**:
```json
{
  "success": true,
  "message": "Parity check started successfully",
  "details": {
    "correcting": false
  },
  "timestamp": "2025-10-03T13:41:13+10:00"
}
```

**Response (Error - Parity Check Already Running)**:
```json
{
  "success": false,
  "error_code": "PARITY_CHECK_RUNNING",
  "message": "Cannot start parity check: a parity check is already running",
  "details": {
    "current_progress": 45.2
  },
  "timestamp": "2025-10-03T13:41:13+10:00"
}
```

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

**Response (Success)**:
```json
{
  "success": true,
  "message": "Parity check stopped successfully",
  "timestamp": "2025-10-03T13:41:13+10:00"
}
```

**Response (Error - No Parity Check Running)**:
```json
{
  "success": false,
  "error_code": "PARITY_CHECK_NOT_RUNNING",
  "message": "Cannot stop parity check: no parity check is running",
  "timestamp": "2025-10-03T13:41:13+10:00"
}
```

**Example**:
```bash
curl -X POST http://192.168.20.21:8043/api/v1/array/parity-check/stop
```

---

### POST /array/parity-check/pause

Pause the current parity check.

**Response (Success)**:
```json
{
  "success": true,
  "message": "Parity check paused successfully",
  "timestamp": "2025-10-03T13:41:13+10:00"
}
```

**Response (Error - No Parity Check Running)**:
```json
{
  "success": false,
  "error_code": "PARITY_CHECK_NOT_RUNNING",
  "message": "Cannot pause parity check: no parity check is running",
  "timestamp": "2025-10-03T13:41:13+10:00"
}
```

**Example**:
```bash
curl -X POST http://192.168.20.21:8043/api/v1/array/parity-check/pause
```

---

### POST /array/parity-check/resume

Resume a paused parity check.

**Response (Success)**:
```json
{
  "success": true,
  "message": "Parity check resumed successfully",
  "timestamp": "2025-10-03T13:41:13+10:00"
}
```

**Response (Error - No Paused Parity Check)**:
```json
{
  "success": false,
  "error_code": "PARITY_CHECK_NOT_PAUSED",
  "message": "Cannot resume parity check: no paused parity check found",
  "timestamp": "2025-10-03T13:41:13+10:00"
}
```

**Example**:
```bash
curl -X POST http://192.168.20.21:8043/api/v1/array/parity-check/resume
```

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

| Parameter | Type | Required | Description | Examples |
|-----------|------|----------|-------------|----------|
| `id` | string | Yes | Disk identifier | `sdb`, `parity`, `disk1`, `cache`, `WUH721816ALE6L4_2CGV0URP` |

**Supported ID Formats**:
- **Device name**: `sdb`, `sdc`, etc.
- **Disk name**: `parity`, `disk1`, `disk2`, `cache`, etc.
- **Disk ID**: Full disk ID like `WUH721816ALE6L4_2CGV0URP`

**Response (Success)**:
```json
{
  "id": "WUH721816ALE6L4_2CGV0URP",
  "device": "sdb",
  "name": "parity",
  "role": "parity",
  "size_bytes": 16000000000000,
  "used_bytes": 0,
  "free_bytes": 16000000000000,
  "temperature_celsius": 35,
  "spin_state": "standby",
  "serial_number": "2CGV0URP",
  "model": "WDC WUH721816ALE6L4",
  "filesystem": "xfs",
  "status": "DISK_OK",
  "timestamp": "2025-10-03T13:41:13+10:00"
}
```

**Response (Error - Disk Not Found)**:
```json
{
  "success": false,
  "error_code": "DISK_NOT_FOUND",
  "message": "Disk not found: sdb99",
  "timestamp": "2025-10-03T13:41:13+10:00"
}
```

**Example**:
```bash
# By device
curl http://192.168.20.21:8043/api/v1/disks/sdb

# By name
curl http://192.168.20.21:8043/api/v1/disks/parity

# By ID
curl http://192.168.20.21:8043/api/v1/disks/WUH721816ALE6L4_2CGV0URP
```

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

| Parameter | Type | Required | Description | Examples |
|-----------|------|----------|-------------|----------|
| `name` | string | Yes | Share name | `appdata`, `media`, `backups` |

**Response (Success)**:
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

**Response (Error - Share Not Found)**:
```json
{
  "success": false,
  "error_code": "SHARE_NOT_FOUND",
  "message": "Share not found: invalid_share",
  "timestamp": "2025-10-03T13:41:13+10:00"
}
```

**Example**:
```bash
curl http://192.168.20.21:8043/api/v1/shares/appdata/config
```

---

### POST /shares/{name}/config

Update share configuration.

**Path Parameters**:

| Parameter | Type | Required | Description | Examples |
|-----------|------|----------|-------------|----------|
| `name` | string | Yes | Share name | `appdata`, `media`, `backups` |

**Request Body Parameters**:

| Parameter | Type | Required | Description | Valid Values | Default |
|-----------|------|----------|-------------|--------------|---------|
| `allocator` | string | No | Allocation method | `highwater`, `mostfree`, `fillup` | Current value |
| `floor` | string | No | Minimum free space (bytes) | Numeric string (e.g., `50000000`) | `0` |
| `use_cache` | string | No | Cache usage policy | `yes`, `no`, `only`, `prefer` | Current value |
| `export` | string | No | Export protocol | `e` (SMB), `n` (NFS), `-` (none) | Current value |
| `security` | string | No | Security mode | `public`, `secure`, `private` | Current value |

**Validation Rules**:
- `allocator`: Must be one of the valid values
- `floor`: Must be a valid numeric string
- `use_cache`: Must be one of the valid values
- At least one parameter must be provided

**Request Body Example**:
```json
{
  "allocator": "highwater",
  "floor": "50000000",
  "use_cache": "only"
}
```

**Response (Success)**:
```json
{
  "success": true,
  "message": "Share configuration updated successfully",
  "backup_created": "/boot/config/shares/appdata.cfg.bak",
  "timestamp": "2025-10-03T13:41:13+10:00"
}
```

**Response (Error - Validation Error)**:
```json
{
  "success": false,
  "error_code": "VALIDATION_ERROR",
  "message": "Invalid request parameters",
  "errors": [
    {
      "field": "allocator",
      "message": "Must be one of: highwater, mostfree, fillup",
      "received": "invalid_value"
    }
  ],
  "timestamp": "2025-10-03T13:41:13+10:00"
}
```

**Example**:
```bash
# Update share configuration
curl -X POST http://192.168.20.21:8043/api/v1/shares/appdata/config \
  -H "Content-Type: application/json" \
  -d '{
    "allocator": "highwater",
    "floor": "50000000",
    "use_cache": "only"
  }'
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

| Parameter | Type | Required | Description | Examples |
|-----------|------|----------|-------------|----------|
| `id` | string | Yes | Container ID or name | `jackett`, `plex`, `fedcb3e1ba1f` |

**Response (Success)**:
```json
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
```

**Response (Error - Container Not Found)**:
```json
{
  "success": false,
  "error_code": "CONTAINER_NOT_FOUND",
  "message": "Container not found: invalid_container",
  "timestamp": "2025-10-03T13:41:13+10:00"
}
```

**Example**:
```bash
curl http://192.168.20.21:8043/api/v1/docker/jackett
```

---

### POST /docker/{id}/start

Start a Docker container.

**Path Parameters**:

| Parameter | Type | Required | Description | Examples |
|-----------|------|----------|-------------|----------|
| `id` | string | Yes | Container ID or name | `jackett`, `plex`, `fedcb3e1ba1f` |

**Response (Success)**:
```json
{
  "success": true,
  "message": "Container started successfully",
  "container_id": "fedcb3e1ba1f",
  "container_name": "jackett",
  "timestamp": "2025-10-03T13:41:13+10:00"
}
```

**Response (Error - Container Already Running)**:
```json
{
  "success": false,
  "error_code": "CONTAINER_ALREADY_RUNNING",
  "message": "Container is already running: jackett",
  "timestamp": "2025-10-03T13:41:13+10:00"
}
```

**Example**:
```bash
curl -X POST http://192.168.20.21:8043/api/v1/docker/jackett/start
```

---

### POST /docker/{id}/stop

Stop a Docker container.

**Path Parameters**:

| Parameter | Type | Required | Description | Examples |
|-----------|------|----------|-------------|----------|
| `id` | string | Yes | Container ID or name | `jackett`, `plex`, `fedcb3e1ba1f` |

**Response (Success)**:
```json
{
  "success": true,
  "message": "Container stopped successfully",
  "container_id": "fedcb3e1ba1f",
  "container_name": "jackett",
  "timestamp": "2025-10-03T13:41:13+10:00"
}
```

**Response (Error - Container Already Stopped)**:
```json
{
  "success": false,
  "error_code": "CONTAINER_ALREADY_STOPPED",
  "message": "Container is already stopped: jackett",
  "timestamp": "2025-10-03T13:41:13+10:00"
}
```

**Example**:
```bash
curl -X POST http://192.168.20.21:8043/api/v1/docker/jackett/stop
```

---

### POST /docker/{id}/restart

Restart a Docker container.

**Path Parameters**:

| Parameter | Type | Required | Description | Examples |
|-----------|------|----------|-------------|----------|
| `id` | string | Yes | Container ID or name | `jackett`, `plex`, `fedcb3e1ba1f` |

**Response (Success)**:
```json
{
  "success": true,
  "message": "Container restarted successfully",
  "container_id": "fedcb3e1ba1f",
  "container_name": "jackett",
  "timestamp": "2025-10-03T13:41:13+10:00"
}
```

**Example**:
```bash
curl -X POST http://192.168.20.21:8043/api/v1/docker/jackett/restart
```

---

### POST /docker/{id}/pause

Pause a Docker container.

**Path Parameters**:

| Parameter | Type | Required | Description | Examples |
|-----------|------|----------|-------------|----------|
| `id` | string | Yes | Container ID or name | `jackett`, `plex`, `fedcb3e1ba1f` |

**Response (Success)**:
```json
{
  "success": true,
  "message": "Container paused successfully",
  "container_id": "fedcb3e1ba1f",
  "container_name": "jackett",
  "timestamp": "2025-10-03T13:41:13+10:00"
}
```

**Example**:
```bash
curl -X POST http://192.168.20.21:8043/api/v1/docker/jackett/pause
```

---

### POST /docker/{id}/unpause

Unpause a Docker container.

**Path Parameters**:

| Parameter | Type | Required | Description | Examples |
|-----------|------|----------|-------------|----------|
| `id` | string | Yes | Container ID or name | `jackett`, `plex`, `fedcb3e1ba1f` |

**Response (Success)**:
```json
{
  "success": true,
  "message": "Container unpaused successfully",
  "container_id": "fedcb3e1ba1f",
  "container_name": "jackett",
  "timestamp": "2025-10-03T13:41:13+10:00"
}
```

**Example**:
```bash
curl -X POST http://192.168.20.21:8043/api/v1/docker/jackett/unpause
```

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

**Path Parameters**:

| Parameter | Type | Required | Description | Examples |
|-----------|------|----------|-------------|----------|
| `id` | string | Yes | VM ID or name | `windows-10`, `ubuntu-server` |

**Response (Success)**:
```json
{
  "id": "windows-10",
  "name": "Windows 10",
  "state": "running",
  "cpu_count": 4,
  "memory_bytes": 8589934592,
  "timestamp": "2025-10-03T13:41:13+10:00"
}
```

**Response (Error - VM Not Found)**:
```json
{
  "success": false,
  "error_code": "VM_NOT_FOUND",
  "message": "Virtual machine not found: invalid_vm",
  "timestamp": "2025-10-03T13:41:13+10:00"
}
```

**Example**:
```bash
curl http://192.168.20.21:8043/api/v1/vm/windows-10
```

---

### POST /vm/{id}/start

Start a virtual machine.

**Path Parameters**:

| Parameter | Type | Required | Description | Examples |
|-----------|------|----------|-------------|----------|
| `id` | string | Yes | VM ID or name | `windows-10`, `ubuntu-server` |

**Response (Success)**:
```json
{
  "success": true,
  "message": "Virtual machine started successfully",
  "vm_id": "windows-10",
  "vm_name": "Windows 10",
  "timestamp": "2025-10-03T13:41:13+10:00"
}
```

**Response (Error - VM Already Running)**:
```json
{
  "success": false,
  "error_code": "VM_ALREADY_RUNNING",
  "message": "Virtual machine is already running: Windows 10",
  "timestamp": "2025-10-03T13:41:13+10:00"
}
```

**Example**:
```bash
curl -X POST http://192.168.20.21:8043/api/v1/vm/windows-10/start
```

---

### POST /vm/{id}/stop

Stop a virtual machine (graceful shutdown).

**Path Parameters**:

| Parameter | Type | Required | Description | Examples |
|-----------|------|----------|-------------|----------|
| `id` | string | Yes | VM ID or name | `windows-10`, `ubuntu-server` |

**Response (Success)**:
```json
{
  "success": true,
  "message": "Virtual machine stopped successfully",
  "vm_id": "windows-10",
  "vm_name": "Windows 10",
  "timestamp": "2025-10-03T13:41:13+10:00"
}
```

**Response (Error - VM Already Stopped)**:
```json
{
  "success": false,
  "error_code": "VM_ALREADY_STOPPED",
  "message": "Virtual machine is already stopped: Windows 10",
  "timestamp": "2025-10-03T13:41:13+10:00"
}
```

**Example**:
```bash
curl -X POST http://192.168.20.21:8043/api/v1/vm/windows-10/stop
```

---

### POST /vm/{id}/restart

Restart a virtual machine.

**Path Parameters**:

| Parameter | Type | Required | Description | Examples |
|-----------|------|----------|-------------|----------|
| `id` | string | Yes | VM ID or name | `windows-10`, `ubuntu-server` |

**Response (Success)**:
```json
{
  "success": true,
  "message": "Virtual machine restarted successfully",
  "vm_id": "windows-10",
  "vm_name": "Windows 10",
  "timestamp": "2025-10-03T13:41:13+10:00"
}
```

**Example**:
```bash
curl -X POST http://192.168.20.21:8043/api/v1/vm/windows-10/restart
```

---

### POST /vm/{id}/pause

Pause a virtual machine.

**Path Parameters**:

| Parameter | Type | Required | Description | Examples |
|-----------|------|----------|-------------|----------|
| `id` | string | Yes | VM ID or name | `windows-10`, `ubuntu-server` |

**Response (Success)**:
```json
{
  "success": true,
  "message": "Virtual machine paused successfully",
  "vm_id": "windows-10",
  "vm_name": "Windows 10",
  "timestamp": "2025-10-03T13:41:13+10:00"
}
```

**Example**:
```bash
curl -X POST http://192.168.20.21:8043/api/v1/vm/windows-10/pause
```

---

### POST /vm/{id}/resume

Resume a paused virtual machine.

**Path Parameters**:

| Parameter | Type | Required | Description | Examples |
|-----------|------|----------|-------------|----------|
| `id` | string | Yes | VM ID or name | `windows-10`, `ubuntu-server` |

**Response (Success)**:
```json
{
  "success": true,
  "message": "Virtual machine resumed successfully",
  "vm_id": "windows-10",
  "vm_name": "Windows 10",
  "timestamp": "2025-10-03T13:41:13+10:00"
}
```

**Example**:
```bash
curl -X POST http://192.168.20.21:8043/api/v1/vm/windows-10/resume
```

---

### POST /vm/{id}/hibernate

Hibernate a virtual machine.

**Path Parameters**:

| Parameter | Type | Required | Description | Examples |
|-----------|------|----------|-------------|----------|
| `id` | string | Yes | VM ID or name | `windows-10`, `ubuntu-server` |

**Response (Success)**:
```json
{
  "success": true,
  "message": "Virtual machine hibernated successfully",
  "vm_id": "windows-10",
  "vm_name": "Windows 10",
  "timestamp": "2025-10-03T13:41:13+10:00"
}
```

**Example**:
```bash
curl -X POST http://192.168.20.21:8043/api/v1/vm/windows-10/hibernate
```

---

### POST /vm/{id}/force-stop

Force stop a virtual machine (immediate shutdown).

**Path Parameters**:

| Parameter | Type | Required | Description | Examples |
|-----------|------|----------|-------------|----------|
| `id` | string | Yes | VM ID or name | `windows-10`, `ubuntu-server` |

**Response (Success)**:
```json
{
  "success": true,
  "message": "Virtual machine force stopped successfully",
  "vm_id": "windows-10",
  "vm_name": "Windows 10",
  "timestamp": "2025-10-03T13:41:13+10:00"
}
```

**Warning**: Force stop does not allow the guest OS to shut down gracefully. Use regular stop for graceful shutdown.

**Example**:
```bash
curl -X POST http://192.168.20.21:8043/api/v1/vm/windows-10/force-stop
```

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

**Request Body Parameters**:

| Parameter | Type | Required | Description | Examples |
|-----------|------|----------|-------------|----------|
| `description` | string | No | Server description | `Home Server`, `Production Server` |
| `server_name` | string | No | Server hostname | `Tower`, `Cube` |

**Validation Rules**:
- At least one parameter must be provided
- `server_name`: Must be a valid hostname (alphanumeric, hyphens allowed)
- `description`: Maximum 255 characters

**Request Body Example**:
```json
{
  "description": "Updated description",
  "server_name": "Cube"
}
```

**Response (Success)**:
```json
{
  "success": true,
  "message": "System settings updated successfully",
  "backup_created": "/boot/config/ident.cfg.bak",
  "timestamp": "2025-10-03T13:41:13+10:00"
}
```

**Response (Error - Validation Error)**:
```json
{
  "success": false,
  "error_code": "VALIDATION_ERROR",
  "message": "Invalid request parameters",
  "errors": [
    {
      "field": "server_name",
      "message": "Invalid hostname format",
      "received": "invalid name!"
    }
  ],
  "timestamp": "2025-10-03T13:41:13+10:00"
}
```

**Example**:
```bash
# Update system settings
curl -X POST http://192.168.20.21:8043/api/v1/settings/system \
  -H "Content-Type: application/json" \
  -d '{
    "description": "Updated description",
    "server_name": "Cube"
  }'
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

| Parameter | Type | Required | Description | Examples |
|-----------|------|----------|-------------|----------|
| `interface` | string | Yes | Network interface name | `eth0`, `eth1`, `bond0`, `br0` |

**Response (Success)**:
```json
{
  "interface": "eth0",
  "ip_address": "192.168.20.21",
  "netmask": "255.255.255.0",
  "gateway": "192.168.20.1",
  "dns_servers": ["8.8.8.8", "8.8.4.4"],
  "dhcp_enabled": false,
  "mtu": 1500,
  "timestamp": "2025-10-03T13:41:13+10:00"
}
```

**Response (Error - Interface Not Found)**:
```json
{
  "success": false,
  "error_code": "NETWORK_INTERFACE_NOT_FOUND",
  "message": "Network interface not found: eth99",
  "timestamp": "2025-10-03T13:41:13+10:00"
}
```

**Example**:
```bash
curl http://192.168.20.21:8043/api/v1/network/eth0/config
```

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

