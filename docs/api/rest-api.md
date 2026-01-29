# API Reference Guide

Complete reference for all Unraid Management Agent API endpoints.

**Base URL**: `http://YOUR_UNRAID_IP:8043/api/v1`  
**Version**: 1.0.0  
**Total Endpoints**: 57

---

## Table of Contents

- [Authentication](#authentication)
- [Response Format](#response-format)
- [Error Handling](#error-handling)
- [Code Examples](#code-examples)
  - [Python Examples](#python-examples)
  - [JavaScript Examples](#javascript-examples)
  - [TypeScript Examples](#typescript-examples)
- [System & Health](#system--health)
- [Array Management](#array-management)
- [Disks](#disks)
- [Shares](#shares)
- [Docker Containers](#docker-containers)
- [Virtual Machines](#virtual-machines)
- [Hardware](#hardware)
- [Log Files](#log-files)
- [Configuration](#configuration)
- [WebSocket](#websocket)
- [Prometheus Metrics](#prometheus-metrics)
- [Security Best Practices](#security-best-practices)
- [Rate Limiting](#rate-limiting)
- [Best Practices](#best-practices)

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

| Error Code                    | HTTP Status | Description                              | Resolution                                 |
| ----------------------------- | ----------- | ---------------------------------------- | ------------------------------------------ |
| `ARRAY_ALREADY_STARTED`       | 409         | Array is already running                 | Check array status before starting         |
| `ARRAY_ALREADY_STOPPED`       | 409         | Array is already stopped                 | Check array status before stopping         |
| `ARRAY_NOT_STARTED`           | 400         | Array must be started for this operation | Start the array first                      |
| `PARITY_CHECK_RUNNING`        | 409         | Parity check is already running          | Stop current check before starting new one |
| `PARITY_CHECK_NOT_RUNNING`    | 400         | No parity check is running               | Start a parity check first                 |
| `DISK_NOT_FOUND`              | 404         | Disk ID/name not found                   | Verify disk exists using GET /disks        |
| `CONTAINER_NOT_FOUND`         | 404         | Docker container not found               | Verify container name/ID using GET /docker |
| `CONTAINER_ALREADY_RUNNING`   | 409         | Container is already running             | Check container state first                |
| `CONTAINER_ALREADY_STOPPED`   | 409         | Container is already stopped             | Check container state first                |
| `VM_NOT_FOUND`                | 404         | Virtual machine not found                | Verify VM name/ID using GET /vm            |
| `VM_ALREADY_RUNNING`          | 409         | VM is already running                    | Check VM state first                       |
| `VM_ALREADY_STOPPED`          | 409         | VM is already stopped                    | Check VM state first                       |
| `SHARE_NOT_FOUND`             | 404         | Share not found                          | Verify share exists using GET /shares      |
| `NETWORK_INTERFACE_NOT_FOUND` | 404         | Network interface not found              | Verify interface using GET /network        |
| `VALIDATION_ERROR`            | 400         | Invalid request parameters               | Check request body format and values       |
| `INTERNAL_ERROR`              | 500         | Server error                             | Check server logs for details              |

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

## Code Examples

This section provides practical code examples in multiple programming languages for integrating with the Unraid Management Agent API.

### Python Examples

#### Installation

```bash
pip install requests
```

#### Basic Usage - Get System Information

```python
import requests

# Configuration
UNRAID_HOST = "192.168.20.21"
UNRAID_PORT = 8043
BASE_URL = f"http://{UNRAID_HOST}:{UNRAID_PORT}/api/v1"

# Get system information
response = requests.get(f"{BASE_URL}/system")
system_info = response.json()

print(f"Hostname: {system_info['hostname']}")
print(f"CPU Usage: {system_info['cpu_usage_percent']}%")
print(f"Memory Usage: {system_info['memory_usage_percent']}%")
print(f"Uptime: {system_info['uptime_seconds']} seconds")
```

#### Health Check

```python
import requests

def check_health(base_url):
    """Check if the API is healthy."""
    try:
        response = requests.get(f"{base_url}/health", timeout=5)
        response.raise_for_status()
        return response.json().get('status') == 'ok'
    except requests.exceptions.RequestException as e:
        print(f"Health check failed: {e}")
        return False

# Usage
if check_health(BASE_URL):
    print("API is healthy")
else:
    print("API is not responding")
```

#### Array Management - Start/Stop Array

```python
import requests

def start_array(base_url):
    """Start the Unraid array."""
    try:
        response = requests.post(f"{base_url}/array/start", timeout=30)
        response.raise_for_status()
        result = response.json()
        print(f"Success: {result['message']}")
        return True
    except requests.exceptions.HTTPError as e:
        if e.response.status_code == 409:
            error = e.response.json()
            print(f"Conflict: {error['message']}")
        else:
            print(f"Error: {e}")
        return False

def stop_array(base_url):
    """Stop the Unraid array."""
    try:
        response = requests.post(f"{base_url}/array/stop", timeout=30)
        response.raise_for_status()
        result = response.json()
        print(f"Success: {result['message']}")
        return True
    except requests.exceptions.HTTPError as e:
        if e.response.status_code == 409:
            error = e.response.json()
            print(f"Conflict: {error['message']}")
        else:
            print(f"Error: {e}")
        return False

# Usage
start_array(BASE_URL)
```

#### Get Array Status

```python
import requests

def get_array_status(base_url):
    """Get current array status."""
    response = requests.get(f"{base_url}/array")
    array_info = response.json()

    print(f"Array State: {array_info['state']}")
    print(f"Total Disks: {array_info['num_disks']}")
    print(f"Data Disks: {array_info['num_data_disks']}")
    print(f"Parity Disks: {array_info['num_parity_disks']}")
    print(f"Usage: {array_info['used_percent']:.1f}%")
    print(f"Total: {array_info['total_bytes'] / (1024**4):.2f} TB")
    print(f"Free: {array_info['free_bytes'] / (1024**4):.2f} TB")

    return array_info

# Usage
array_status = get_array_status(BASE_URL)
```

#### Disk Operations - List and Get Disk

```python
import requests

def list_disks(base_url):
    """List all disks."""
    response = requests.get(f"{base_url}/disks")
    disks = response.json()

    for disk in disks:
        print(f"Disk: {disk['name']} ({disk['device']})")
        print(f"  Role: {disk['role']}")
        print(f"  Size: {disk['size_bytes'] / (1024**3):.1f} GB")
        print(f"  Temperature: {disk['temperature_celsius']}°C")
        print(f"  Spin State: {disk['spin_state']}")
        print()

    return disks

def get_disk(base_url, disk_id):
    """Get specific disk information."""
    try:
        response = requests.get(f"{base_url}/disks/{disk_id}")
        response.raise_for_status()
        return response.json()
    except requests.exceptions.HTTPError as e:
        if e.response.status_code == 404:
            print(f"Disk not found: {disk_id}")
        return None

# Usage
all_disks = list_disks(BASE_URL)
parity_disk = get_disk(BASE_URL, "parity")
```

---

### JavaScript Examples

#### Using Fetch API

```javascript
// Configuration
const UNRAID_HOST = "192.168.20.21";
const UNRAID_PORT = 8043;
const BASE_URL = `http://${UNRAID_HOST}:${UNRAID_PORT}/api/v1`;

// Get system information
async function getSystemInfo() {
  try {
    const response = await fetch(`${BASE_URL}/system`);

    if (!response.ok) {
      throw new Error(`HTTP error! status: ${response.status}`);
    }

    const systemInfo = await response.json();

    console.log(`Hostname: ${systemInfo.hostname}`);
    console.log(`CPU Usage: ${systemInfo.cpu_usage_percent}%`);
    console.log(`RAM Usage: ${systemInfo.ram_usage_percent}%`);
    console.log(`CPU Temp: ${systemInfo.cpu_temp_celsius}°C`);

    return systemInfo;
  } catch (error) {
    console.error("Error fetching system info:", error);
    throw error;
  }
}

// Health check
async function checkHealth() {
  try {
    const response = await fetch(`${BASE_URL}/health`);
    const data = await response.json();
    return data.status === "ok";
  } catch (error) {
    console.error("Health check failed:", error);
    return false;
  }
}

// Start array
async function startArray() {
  try {
    const response = await fetch(`${BASE_URL}/array/start`, {
      method: "POST",
    });

    if (!response.ok) {
      const error = await response.json();
      if (response.status === 409) {
        console.log(`Conflict: ${error.message}`);
      } else {
        throw new Error(error.message);
      }
      return false;
    }

    const result = await response.json();
    console.log(`Success: ${result.message}`);
    return true;
  } catch (error) {
    console.error("Error starting array:", error);
    return false;
  }
}

// Usage
getSystemInfo();
checkHealth().then((healthy) => {
  console.log(`API is ${healthy ? "healthy" : "not responding"}`);
});
```

#### Using Axios

```javascript
const axios = require("axios");

// Configuration
const BASE_URL = "http://192.168.20.21:8043/api/v1";

// Create axios instance with default config
const api = axios.create({
  baseURL: BASE_URL,
  timeout: 5000,
  headers: {
    "Content-Type": "application/json",
  },
});

// Get system information
async function getSystemInfo() {
  try {
    const { data } = await api.get("/system");
    console.log(`CPU Usage: ${data.cpu_usage_percent}%`);
    return data;
  } catch (error) {
    console.error("Error:", error.message);
    throw error;
  }
}

// Start array with error handling
async function startArray() {
  try {
    const { data } = await api.post("/array/start");
    console.log(data.message);
    return true;
  } catch (error) {
    if (error.response) {
      // Server responded with error
      const { status, data } = error.response;
      if (status === 409) {
        console.log(`Conflict: ${data.message}`);
      } else {
        console.error(`Error ${status}: ${data.message}`);
      }
    } else if (error.request) {
      // Request made but no response
      console.error("No response from server");
    } else {
      console.error("Error:", error.message);
    }
    return false;
  }
}

// List all disks
async function listDisks() {
  try {
    const { data } = await api.get("/disks");
    data.forEach((disk) => {
      console.log(`${disk.name}: ${disk.spin_state}`);
    });
    return data;
  } catch (error) {
    console.error("Error listing disks:", error.message);
    return [];
  }
}

// Usage
getSystemInfo();
startArray();
listDisks();
```

---

### TypeScript Examples

#### Type Definitions

```typescript
// Type definitions for API responses
interface SystemInfo {
  hostname: string;
  uptime_seconds: number;
  cpu_usage_percent: number;
  memory_total_bytes: number;
  memory_used_bytes: number;
  memory_usage_percent: number;
  load_average: number[];
  timestamp: string;
}

interface ArrayStatus {
  state: "STARTED" | "STOPPED" | "STARTING" | "STOPPING";
  total_disks: number;
  data_disks: number;
  parity_disks: number;
  cache_disks: number;
  size_bytes: number;
  used_bytes: number;
  free_bytes: number;
  usage_percent: number;
  timestamp: string;
}

interface DiskInfo {
  id: string;
  device: string;
  name: string;
  role:
    | "parity"
    | "parity2"
    | "data"
    | "cache"
    | "pool"
    | "docker_vdisk"
    | "log";
  size_bytes: number;
  used_bytes: number;
  free_bytes: number;
  temperature_celsius: number;
  spin_state: "active" | "standby" | "idle";
  serial_number: string;
  model: string;
  filesystem: string;
  status: string;
  timestamp: string;
}

interface ContainerInfo {
  id: string;
  name: string;
  image: string;
  state: "running" | "stopped" | "paused";
  status: string;
  cpu_usage_percent: number;
  memory_usage_bytes: number;
  network_rx_bytes: number;
  network_tx_bytes: number;
  timestamp: string;
}

interface VMInfo {
  id: string;
  name: string;
  state: "running" | "stopped" | "paused";
  cpu_count: number;
  memory_bytes: number;
  timestamp: string;
}

interface APIError {
  success: false;
  error_code: string;
  message: string;
  details?: Record<string, any>;
  timestamp: string;
}

interface APISuccess {
  success: true;
  message: string;
  timestamp: string;
}
```

#### API Client Class

```typescript
class UnraidAPIClient {
  private baseURL: string;
  private timeout: number;

  constructor(host: string, port: number = 8043, timeout: number = 5000) {
    this.baseURL = `http://${host}:${port}/api/v1`;
    this.timeout = timeout;
  }

  private async request<T>(
    endpoint: string,
    method: "GET" | "POST" = "GET",
    body?: any,
  ): Promise<T> {
    const controller = new AbortController();
    const timeoutId = setTimeout(() => controller.abort(), this.timeout);

    try {
      const response = await fetch(`${this.baseURL}${endpoint}`, {
        method,
        headers: body ? { "Content-Type": "application/json" } : {},
        body: body ? JSON.stringify(body) : undefined,
        signal: controller.signal,
      });

      clearTimeout(timeoutId);

      if (!response.ok) {
        const error: APIError = await response.json();
        throw new Error(`${error.error_code}: ${error.message}`);
      }

      return await response.json();
    } catch (error) {
      clearTimeout(timeoutId);
      throw error;
    }
  }

  async getSystemInfo(): Promise<SystemInfo> {
    return this.request<SystemInfo>("/system");
  }

  async getArrayStatus(): Promise<ArrayStatus> {
    return this.request<ArrayStatus>("/array");
  }

  async startArray(): Promise<APISuccess> {
    return this.request<APISuccess>("/array/start", "POST");
  }

  async stopArray(): Promise<APISuccess> {
    return this.request<APISuccess>("/array/stop", "POST");
  }

  async listDisks(): Promise<DiskInfo[]> {
    return this.request<DiskInfo[]>("/disks");
  }

  async getDisk(id: string): Promise<DiskInfo> {
    return this.request<DiskInfo>(`/disks/${id}`);
  }

  async listContainers(): Promise<ContainerInfo[]> {
    return this.request<ContainerInfo[]>("/docker");
  }

  async getContainer(id: string): Promise<ContainerInfo> {
    return this.request<ContainerInfo>(`/docker/${id}`);
  }

  async startContainer(id: string): Promise<APISuccess> {
    return this.request<APISuccess>(`/docker/${id}/start`, "POST");
  }

  async stopContainer(id: string): Promise<APISuccess> {
    return this.request<APISuccess>(`/docker/${id}/stop`, "POST");
  }

  async listVMs(): Promise<VMInfo[]> {
    return this.request<VMInfo[]>("/vm");
  }

  async getVM(id: string): Promise<VMInfo> {
    return this.request<VMInfo>(`/vm/${id}`);
  }

  async startVM(id: string): Promise<APISuccess> {
    return this.request<APISuccess>(`/vm/${id}/start`, "POST");
  }

  async stopVM(id: string): Promise<APISuccess> {
    return this.request<APISuccess>(`/vm/${id}/stop`, "POST");
  }
}
```

#### Usage Example

```typescript
// Create client instance
const client = new UnraidAPIClient("192.168.20.21");

// Get system information
async function displaySystemInfo() {
  try {
    const systemInfo = await client.getSystemInfo();
    console.log(`Hostname: ${systemInfo.hostname}`);
    console.log(`CPU Usage: ${systemInfo.cpu_usage_percent}%`);
    console.log(`RAM Usage: ${systemInfo.ram_usage_percent}%`);
    console.log(`CPU Temp: ${systemInfo.cpu_temp_celsius}°C`);
  } catch (error) {
    console.error("Error:", error);
  }
}

// Manage array
async function manageArray() {
  try {
    const status = await client.getArrayStatus();

    if (status.state === "STOPPED") {
      console.log("Starting array...");
      await client.startArray();
    } else {
      console.log(`Array is ${status.state}`);
    }
  } catch (error) {
    console.error("Error managing array:", error);
  }
}

// List disks with type safety
async function listDisks() {
  try {
    const disks = await client.listDisks();

    disks.forEach((disk: DiskInfo) => {
      console.log(
        `${disk.name}: ${disk.spin_state} (${disk.temperature_celsius}°C)`,
      );
    });
  } catch (error) {
    console.error("Error listing disks:", error);
  }
}

// Docker container management
async function manageContainer(containerName: string) {
  try {
    const container = await client.getContainer(containerName);

    if (container.state === "stopped") {
      console.log(`Starting ${containerName}...`);
      await client.startContainer(containerName);
    } else {
      console.log(`${containerName} is ${container.state}`);
    }
  } catch (error) {
    console.error("Error managing container:", error);
  }
}

// VM management
async function manageVM(vmName: string) {
  try {
    const vm = await client.getVM(vmName);

    if (vm.state === "stopped") {
      console.log(`Starting ${vmName}...`);
      await client.startVM(vmName);
    } else {
      console.log(`${vmName} is ${vm.state}`);
    }
  } catch (error) {
    console.error("Error managing VM:", error);
  }
}

// Usage
displaySystemInfo();
manageArray();
listDisks();
manageContainer("plex");
manageVM("windows-10");
```

#### Error Handling with Type Guards

```typescript
function isAPIError(response: any): response is APIError {
  return response.success === false && "error_code" in response;
}

async function safeAPICall<T>(apiCall: () => Promise<T>): Promise<T | null> {
  try {
    return await apiCall();
  } catch (error) {
    if (error instanceof Error) {
      console.error("API Error:", error.message);
    }
    return null;
  }
}

// Usage with type safety
const systemInfo = await safeAPICall(() => client.getSystemInfo());
if (systemInfo) {
  console.log(`CPU: ${systemInfo.cpu_usage_percent}%`);
}
```

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

Get system information including CPU, memory, temperatures, and uptime.

**Response**:

```json
{
  "hostname": "Cube",
  "version": "7.2.0",
  "agent_version": "2025.11.23",
  "uptime_seconds": 698918,
  "cpu_usage_percent": 0,
  "cpu_model": "Intel(R) Core(TM) i7-8700K CPU @ 3.70GHz",
  "cpu_cores": 1,
  "cpu_threads": 12,
  "cpu_mhz": 800.113,
  "cpu_per_core_usage": {
    "cpu0": 0.5,
    "cpu1": 0.3
  },
  "cpu_temp_celsius": 36,
  "ram_usage_percent": 41.82,
  "ram_total_bytes": 33328332800,
  "ram_used_bytes": 13937836032,
  "ram_free_bytes": 19390496768,
  "ram_buffers_bytes": 1073741824,
  "ram_cached_bytes": 8589934592,
  "server_model": "System Product Name",
  "bios_version": "1.0",
  "bios_date": "01/01/2020",
  "motherboard_temp_celsius": 36,
  "hvm_enabled": true,
  "iommu_enabled": true,
  "openssl_version": "OpenSSL 3.0.0",
  "kernel_version": "6.1.0-unRAID",
  "fans": [
    {
      "name": "CPU Fan",
      "rpm": 1200
    }
  ],
  "timestamp": "2025-11-17T14:39:17+10:00"
}
```

**Field Descriptions**:

- `hostname`: Server hostname
- `version`: Unraid OS version
- `agent_version`: Management Agent version
- `uptime_seconds`: System uptime in seconds
- `cpu_usage_percent`: Overall CPU usage percentage
- `cpu_model`: CPU model name
- `cpu_cores`: Number of physical CPU cores
- `cpu_threads`: Number of CPU threads (with hyperthreading)
- `cpu_mhz`: Current CPU frequency in MHz
- `cpu_per_core_usage`: Per-core CPU usage map (optional)
- `cpu_temp_celsius`: CPU temperature in Celsius
- `ram_usage_percent`: RAM usage percentage
- `ram_total_bytes`: Total RAM in bytes
- `ram_used_bytes`: Used RAM in bytes
- `ram_free_bytes`: Free RAM in bytes
- `ram_buffers_bytes`: RAM used for buffers in bytes
- `ram_cached_bytes`: RAM used for cache in bytes
- `server_model`: Server/motherboard model
- `bios_version`: BIOS version
- `bios_date`: BIOS date
- `motherboard_temp_celsius`: Motherboard temperature in Celsius
- `hvm_enabled`: Hardware virtualization enabled
- `iommu_enabled`: IOMMU enabled for PCIe passthrough
- `openssl_version`: OpenSSL version (optional)
- `kernel_version`: Linux kernel version (optional)
- `fans`: Array of fan information (optional)

**Example**:

```bash
curl http://192.168.20.21:8043/api/v1/system
```

**Note**: All size fields are in **bytes**, all temperatures are in **Celsius**, and all percentages use the `_percent` suffix.

---

### POST /system/reboot

Initiate a server reboot.

**⚠️ Warning**: This is a destructive operation. The server will reboot immediately. Ensure all critical operations are complete before calling this endpoint.

**Response**:

```json
{
  "success": true,
  "message": "System reboot initiated"
}
```

**Error Response** (if reboot command fails):

```json
{
  "success": false,
  "message": "Failed to initiate reboot: <error details>"
}
```

**Example**:

```bash
curl -X POST http://192.168.20.21:8043/api/v1/system/reboot
```

**Use Cases**:

- Home Assistant integration for automated server power management
- Remote server administration
- Post-maintenance reboot automation

---

### POST /system/shutdown

Initiate a server shutdown (power off).

**⚠️ Warning**: This is a destructive operation. The server will shut down immediately. You will need physical access or out-of-band management (IPMI/iLO/iDRAC) to power the server back on.

**Response**:

```json
{
  "success": true,
  "message": "System shutdown initiated"
}
```

**Error Response** (if shutdown command fails):

```json
{
  "success": false,
  "message": "Failed to initiate shutdown: <error details>"
}
```

**Example**:

```bash
curl -X POST http://192.168.20.21:8043/api/v1/system/shutdown
```

**Use Cases**:

- Home Assistant integration for scheduled server shutdowns
- Power-saving automation (shutdown during off-peak hours)
- Emergency shutdown via remote access

---

## Array Management

### GET /array

Get array status and information.

**Response**:

```json
{
  "state": "STARTED",
  "used_percent": 31.27,
  "free_bytes": 28864055205888,
  "total_bytes": 41996310249472,
  "parity_valid": true,
  "parity_check_status": "idle",
  "parity_check_progress": 0,
  "num_disks": 5,
  "num_data_disks": 1,
  "num_parity_disks": 2,
  "timestamp": "2025-11-17T14:39:17+10:00"
}
```

**Field Descriptions**:

- `state`: Array state (`STARTED`, `STOPPED`, `STARTING`, `STOPPING`)
- `used_percent`: Percentage of array capacity used
- `free_bytes`: Free space in bytes
- `total_bytes`: Total array capacity in bytes
- `parity_valid`: Whether parity is valid
- `parity_check_status`: Parity check status (`idle`, `running`, `paused`)
- `parity_check_progress`: Parity check progress percentage (0-100)
- `num_disks`: Total number of disks in array
- `num_data_disks`: Number of data disks
- `num_parity_disks`: Number of parity disks (0, 1, or 2)

**Note**: Cache disks are shown in the `/api/v1/disks` endpoint with `role: "cache"` or `role: "pool"`.

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

| Parameter    | Type    | Required | Description                                | Valid Values    | Default |
| ------------ | ------- | -------- | ------------------------------------------ | --------------- | ------- |
| `correcting` | boolean | No       | Whether to perform correcting parity check | `true`, `false` | `false` |

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
    "status": "DISK_OK",
    "size_bytes": 8000450304000,
    "used_bytes": 0,
    "free_bytes": 8000450304000,
    "temperature_celsius": 0,
    "smart_status": "PASSED",
    "smart_errors": 0,
    "spindown_delay": 0,
    "filesystem": "xfs",
    "serial_number": "2CGV0URP",
    "model": "WDC WUH721816ALE6L4",
    "role": "parity",
    "spin_state": "standby",
    "smart_attributes": {
      "5": {
        "id": 5,
        "name": "Reallocated_Sector_Ct",
        "value": 100,
        "worst": 100,
        "threshold": 10,
        "raw_value": "0"
      }
    },
    "power_on_hours": 12345,
    "power_cycle_count": 100,
    "mount_point": "/mnt/disk1",
    "usage_percent": 50.5,
    "timestamp": "2025-11-17T14:39:17+10:00"
  }
]
```

**Field Descriptions**:

- `id`: Unique disk identifier (model_serial)
- `device`: Linux device name (e.g., `sdb`, `sdc`)
- `name`: Unraid disk name (e.g., `parity`, `disk1`, `cache`)
- `status`: Disk status (`DISK_OK`, `DISK_DSBL`, etc.)
- `size_bytes`: Total disk capacity in bytes
- `used_bytes`: Used space in bytes
- `free_bytes`: Free space in bytes
- `temperature_celsius`: Disk temperature in Celsius (0 if spun down)
- `smart_status`: SMART health status (`PASSED`, `FAILED`)
- `smart_errors`: Number of SMART errors
- `spindown_delay`: Spindown delay in minutes (0 = never)
- `filesystem`: Filesystem type (`xfs`, `btrfs`, etc.)
- `serial_number`: Disk serial number
- `model`: Disk model name
- `role`: Disk role (`parity`, `parity2`, `data`, `cache`, `pool`)
- `spin_state`: Current spin state (`active`, `standby`, `unknown`)
- `smart_attributes`: SMART attribute details (optional)
- `power_on_hours`: Total power-on hours (optional)
- `power_cycle_count`: Number of power cycles (optional)
- `mount_point`: Mount point path (optional)
- `usage_percent`: Disk usage percentage (optional)

**Note**: Temperature of 0°C typically indicates the disk is in standby/spun down state.

---

### GET /disks/{id}

Get a single disk by ID, device name, or disk name.

**Path Parameters**:

| Parameter | Type   | Required | Description     | Examples                                                      |
| --------- | ------ | -------- | --------------- | ------------------------------------------------------------- |
| `id`      | string | Yes      | Disk identifier | `sdb`, `parity`, `disk1`, `cache`, `WUH721816ALE6L4_2CGV0URP` |

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

| Parameter | Type   | Required | Description | Examples                      |
| --------- | ------ | -------- | ----------- | ----------------------------- |
| `name`    | string | Yes      | Share name  | `appdata`, `media`, `backups` |

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

| Parameter | Type   | Required | Description | Examples                      |
| --------- | ------ | -------- | ----------- | ----------------------------- |
| `name`    | string | Yes      | Share name  | `appdata`, `media`, `backups` |

**Request Body Parameters**:

| Parameter   | Type   | Required | Description                | Valid Values                      | Default       |
| ----------- | ------ | -------- | -------------------------- | --------------------------------- | ------------- |
| `allocator` | string | No       | Allocation method          | `highwater`, `mostfree`, `fillup` | Current value |
| `floor`     | string | No       | Minimum free space (bytes) | Numeric string (e.g., `50000000`) | `0`           |
| `use_cache` | string | No       | Cache usage policy         | `yes`, `no`, `only`, `prefer`     | Current value |
| `export`    | string | No       | Export protocol            | `e` (SMB), `n` (NFS), `-` (none)  | Current value |
| `security`  | string | No       | Security mode              | `public`, `secure`, `private`     | Current value |

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

| Parameter | Type   | Required | Description          | Examples                          |
| --------- | ------ | -------- | -------------------- | --------------------------------- |
| `id`      | string | Yes      | Container ID or name | `jackett`, `plex`, `fedcb3e1ba1f` |

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

| Parameter | Type   | Required | Description          | Examples                          |
| --------- | ------ | -------- | -------------------- | --------------------------------- |
| `id`      | string | Yes      | Container ID or name | `jackett`, `plex`, `fedcb3e1ba1f` |

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

| Parameter | Type   | Required | Description          | Examples                          |
| --------- | ------ | -------- | -------------------- | --------------------------------- |
| `id`      | string | Yes      | Container ID or name | `jackett`, `plex`, `fedcb3e1ba1f` |

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

| Parameter | Type   | Required | Description          | Examples                          |
| --------- | ------ | -------- | -------------------- | --------------------------------- |
| `id`      | string | Yes      | Container ID or name | `jackett`, `plex`, `fedcb3e1ba1f` |

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

| Parameter | Type   | Required | Description          | Examples                          |
| --------- | ------ | -------- | -------------------- | --------------------------------- |
| `id`      | string | Yes      | Container ID or name | `jackett`, `plex`, `fedcb3e1ba1f` |

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

| Parameter | Type   | Required | Description          | Examples                          |
| --------- | ------ | -------- | -------------------- | --------------------------------- |
| `id`      | string | Yes      | Container ID or name | `jackett`, `plex`, `fedcb3e1ba1f` |

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

| Parameter | Type   | Required | Description   | Examples                      |
| --------- | ------ | -------- | ------------- | ----------------------------- |
| `id`      | string | Yes      | VM ID or name | `windows-10`, `ubuntu-server` |

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

| Parameter | Type   | Required | Description   | Examples                      |
| --------- | ------ | -------- | ------------- | ----------------------------- |
| `id`      | string | Yes      | VM ID or name | `windows-10`, `ubuntu-server` |

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

| Parameter | Type   | Required | Description   | Examples                      |
| --------- | ------ | -------- | ------------- | ----------------------------- |
| `id`      | string | Yes      | VM ID or name | `windows-10`, `ubuntu-server` |

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

| Parameter | Type   | Required | Description   | Examples                      |
| --------- | ------ | -------- | ------------- | ----------------------------- |
| `id`      | string | Yes      | VM ID or name | `windows-10`, `ubuntu-server` |

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

| Parameter | Type   | Required | Description   | Examples                      |
| --------- | ------ | -------- | ------------- | ----------------------------- |
| `id`      | string | Yes      | VM ID or name | `windows-10`, `ubuntu-server` |

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

| Parameter | Type   | Required | Description   | Examples                      |
| --------- | ------ | -------- | ------------- | ----------------------------- |
| `id`      | string | Yes      | VM ID or name | `windows-10`, `ubuntu-server` |

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

| Parameter | Type   | Required | Description   | Examples                      |
| --------- | ------ | -------- | ------------- | ----------------------------- |
| `id`      | string | Yes      | VM ID or name | `windows-10`, `ubuntu-server` |

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

| Parameter | Type   | Required | Description   | Examples                      |
| --------- | ------ | -------- | ------------- | ----------------------------- |
| `id`      | string | Yes      | VM ID or name | `windows-10`, `ubuntu-server` |

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

### GET /network/access-urls

Get all methods to access the Unraid server, including LAN IP, mDNS hostname, WireGuard VPN IPs, WAN IP, and IPv6 addresses.

**Response**:

```json
{
  "urls": [
    {
      "type": "lan",
      "name": "LAN (br0)",
      "ipv4": "http://192.168.20.21"
    },
    {
      "type": "mdns",
      "name": "mDNS",
      "ipv4": "http://cube.local"
    },
    {
      "type": "wireguard",
      "name": "VPN (wg0)",
      "ipv4": "http://10.0.0.1"
    },
    {
      "type": "wan",
      "name": "Remote Access (WAN)",
      "ipv4": "http://203.0.113.45"
    },
    {
      "type": "ipv6",
      "name": "IPv6 (br0)",
      "ipv6": "http://[2001:db8::1]"
    }
  ],
  "timestamp": "2025-10-03T13:41:13+10:00"
}
```

**URL Types**:

| Type        | Description                                                |
| ----------- | ---------------------------------------------------------- |
| `lan`       | Local Area Network IPv4 addresses                          |
| `mdns`      | mDNS hostname (hostname.local) for Bonjour/Avahi discovery |
| `wireguard` | WireGuard VPN interface addresses                          |
| `wan`       | Public WAN IP (if accessible)                              |
| `ipv6`      | IPv6 global unicast addresses                              |
| `other`     | Other access methods                                       |

**Use Cases**:

- Display all connection methods in a dashboard
- Auto-discover server addresses for mobile apps
- Generate connection links automatically
- Verify remote access configuration

---

## Collector Management

The agent runs multiple collectors that gather data at configurable intervals. These endpoints allow runtime management of collectors without restarting the agent.

### GET /collectors/status

Get status of all collectors including enabled state, interval, and runtime information.

**Response**:

```json
{
  "collectors": [
    {
      "name": "system",
      "enabled": true,
      "interval_seconds": 15,
      "status": "running",
      "last_run": "2026-01-07T12:05:10.959716559+10:00",
      "error_count": 0,
      "required": true
    },
    {
      "name": "docker",
      "enabled": true,
      "interval_seconds": 30,
      "status": "running",
      "last_run": "2026-01-07T12:05:10.959676097+10:00",
      "error_count": 0,
      "required": false
    },
    {
      "name": "gpu",
      "enabled": false,
      "interval_seconds": 60,
      "status": "stopped",
      "error_count": 0,
      "required": false
    }
  ],
  "total": 15,
  "enabled_count": 11,
  "disabled_count": 4,
  "timestamp": "2026-01-07T12:05:25+10:00"
}
```

**Status Values**:

| Status       | Description                                 |
| ------------ | ------------------------------------------- |
| `running`    | Collector is actively collecting data       |
| `stopped`    | Collector has been disabled at runtime      |
| `disabled`   | Collector was disabled at startup           |
| `registered` | Collector is registered but not yet started |

---

### GET /collectors/{name}

Get status of a specific collector.

**Path Parameters**:

- `name` - Collector name (system, array, disk, docker, vm, ups, nut, gpu, shares, network, hardware, zfs, notification, registration, unassigned)

**Response**:

```json
{
  "success": true,
  "message": "collector gpu status retrieved",
  "collector": {
    "name": "gpu",
    "enabled": true,
    "interval_seconds": 60,
    "status": "running",
    "last_run": "2026-01-07T12:05:10.959614279+10:00",
    "error_count": 0,
    "required": false
  },
  "timestamp": "2026-01-07T12:05:31+10:00"
}
```

**Error Response (404)**:

```json
{
  "success": false,
  "message": "unknown collector: invalid_name",
  "timestamp": "2026-01-07T12:05:31+10:00"
}
```

---

### POST /collectors/{name}/enable

Enable a collector at runtime. The collector will start immediately using its configured interval.

**Path Parameters**:

- `name` - Collector name

**Response**:

```json
{
  "success": true,
  "message": "gpu collector enabled",
  "collector": {
    "name": "gpu",
    "enabled": true,
    "interval_seconds": 60,
    "status": "running",
    "last_run": "2026-01-07T12:05:35+10:00",
    "error_count": 0,
    "required": false
  },
  "timestamp": "2026-01-07T12:05:35+10:00"
}
```

**Notes**:

- Enabling an already-enabled collector is a no-op (returns success)
- If interval was 0, a default interval is used

---

### POST /collectors/{name}/disable

Disable a collector at runtime. The collector will stop gracefully.

**Path Parameters**:

- `name` - Collector name

**Response**:

```json
{
  "success": true,
  "message": "gpu collector disabled",
  "collector": {
    "name": "gpu",
    "enabled": false,
    "interval_seconds": 60,
    "status": "stopped",
    "last_run": "2026-01-07T12:05:10+10:00",
    "error_count": 0,
    "required": false
  },
  "timestamp": "2026-01-07T12:05:31+10:00"
}
```

**Error Response (400)** - Required Collector:

```json
{
  "success": false,
  "message": "cannot disable system collector (always required)",
  "timestamp": "2026-01-07T12:05:47+10:00"
}
```

**Notes**:

- The `system` collector cannot be disabled (it's required)
- Disabling an already-disabled collector is a no-op (returns success)

---

### PATCH /collectors/{name}/interval

Update the collection interval for a collector. If the collector is running, it will be restarted with the new interval.

**Path Parameters**:

- `name` - Collector name

**Request Body**:

```json
{
  "interval": 120
}
```

**Response**:

```json
{
  "success": true,
  "message": "docker collector interval updated to 120 seconds",
  "collector": {
    "name": "docker",
    "enabled": true,
    "interval_seconds": 120,
    "status": "running",
    "last_run": "2026-01-07T12:05:40+10:00",
    "error_count": 0,
    "required": false
  },
  "timestamp": "2026-01-07T12:05:40+10:00"
}
```

**Validation**:

- Interval must be between 5 and 3600 seconds

**Error Response (400)** - Invalid Interval:

```json
{
  "success": false,
  "message": "invalid interval: must be between 5 and 3600 seconds",
  "timestamp": "2026-01-07T12:05:47+10:00"
}
```

---

### WebSocket Event: collector_state_change

When a collector is enabled or disabled, a WebSocket event is broadcast to all connected clients.

**Event Payload**:

```json
{
  "event": "collector_state_change",
  "collector": "gpu",
  "enabled": true,
  "status": "running",
  "interval": 60,
  "timestamp": "2026-01-07T12:05:35+10:00"
}
```

---

## Log Files

### GET /logs

List available log files or retrieve log content.

**Query Parameters**:

- `path` (optional) - Full path to log file. If omitted, lists all available log files.
- `lines` (optional) - Number of lines to return (tail behavior if start not specified)
- `start` (optional) - Starting line number for range retrieval

**List All Logs Response**:

```json
{
  "logs": [
    {
      "name": "syslog",
      "path": "/var/log/syslog",
      "size_bytes": 1048576,
      "modified_at": "2025-11-28T12:00:00+10:00"
    },
    {
      "name": "docker.log",
      "path": "/var/log/docker.log",
      "size_bytes": 524288,
      "modified_at": "2025-11-28T11:30:00+10:00"
    }
  ]
}
```

**Get Log Content Response** (with `?path=/var/log/syslog&lines=100`):

```json
{
  "path": "/var/log/syslog",
  "content": "Nov 28 12:00:00 Cube ...\n...",
  "lines": ["Nov 28 12:00:00 Cube ...", "..."],
  "total_lines": 10000,
  "lines_returned": 100,
  "start_line": 9900,
  "end_line": 10000
}
```

**Available Log Files**:

- `/var/log/syslog` - System log
- `/var/log/dmesg` - Kernel ring buffer
- `/var/log/messages` - System messages
- `/var/log/cron` - Cron job logs
- `/var/log/docker.log` - Docker daemon logs
- `/var/log/libvirt/libvirtd.log` - Libvirt daemon logs
- `/var/log/unraid-management-agent.log` - This plugin's logs
- `/var/log/graphql-api.log` - Unraid GraphQL API logs
- `/var/log/nginx/error.log` - Nginx error logs
- `/var/log/nginx/access.log` - Nginx access logs
- `/var/log/apcupsd.events` - UPS events
- And 20+ more system and application logs

---

### GET /logs/{filename}

Retrieve a specific log file by filename.

**Path Parameters**:

- `filename` - The name of the log file (e.g., `syslog`, `docker.log`)

**Query Parameters**:

- `lines` (optional) - Number of lines to return
- `start` (optional) - Starting line number

**Response**:

```json
{
  "path": "/var/log/syslog",
  "content": "Nov 28 12:00:00 Cube ...\n...",
  "lines": ["Nov 28 12:00:00 Cube ...", "..."],
  "total_lines": 10000,
  "lines_returned": 50,
  "start_line": 9950,
  "end_line": 10000
}
```

**Error Response** (invalid filename):

```json
{
  "success": false,
  "message": "Invalid filename",
  "timestamp": "2025-11-28T12:00:00+10:00"
}
```

**Error Response** (not found):

```json
{
  "success": false,
  "message": "Log file not found: unknown.log",
  "timestamp": "2025-11-28T12:00:00+10:00"
}
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

| Parameter     | Type   | Required | Description        | Examples                           |
| ------------- | ------ | -------- | ------------------ | ---------------------------------- |
| `description` | string | No       | Server description | `Home Server`, `Production Server` |
| `server_name` | string | No       | Server hostname    | `Tower`, `Cube`                    |

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

### GET /settings/disk-thresholds

Get global disk temperature warning and critical thresholds.

**Response**:

```json
{
  "hot_threshold": 45,
  "max_threshold": 55,
  "hot_ssd_threshold": 50,
  "max_ssd_threshold": 60,
  "timestamp": "2025-10-03T13:41:13+10:00"
}
```

**Fields**:

| Field               | Type | Description                   |
| ------------------- | ---- | ----------------------------- |
| `hot_threshold`     | int  | HDD warning temperature (°C)  |
| `max_threshold`     | int  | HDD critical temperature (°C) |
| `hot_ssd_threshold` | int  | SSD warning temperature (°C)  |
| `max_ssd_threshold` | int  | SSD critical temperature (°C) |

**Use Case**: Monitoring tools (Grafana, Home Assistant) can use these thresholds to set appropriate alert levels for disk temperature monitoring.

---

### GET /settings/mover

Get mover schedule, thresholds, and current running status.

**Response**:

```json
{
  "schedule": "0 3 * * *",
  "enabled": true,
  "running": false,
  "fill_up_threshold": 70,
  "age_days": 0,
  "action_on_share_fill": "move_all",
  "timestamp": "2025-10-03T13:41:13+10:00"
}
```

**Fields**:

| Field                  | Type   | Description                                          |
| ---------------------- | ------ | ---------------------------------------------------- |
| `schedule`             | string | Cron-style schedule expression                       |
| `enabled`              | bool   | Whether scheduled mover runs are enabled             |
| `running`              | bool   | Whether mover is currently running                   |
| `fill_up_threshold`    | int    | Percentage threshold to trigger cache->array moves   |
| `age_days`             | int    | Move files older than this many days (0 = all files) |
| `action_on_share_fill` | string | Action when share fills up                           |

---

### GET /settings/services

Get Docker and VM Manager service status.

**Response**:

```json
{
  "docker_enabled": true,
  "vm_enabled": true,
  "timestamp": "2025-10-03T13:41:13+10:00"
}
```

**Use Case**: Determine whether to display Docker or VM-related UI elements, or to skip queries for disabled services.

---

### GET /settings/network-services

Get comprehensive status of all Unraid network services.

**Response**:

```json
{
  "smb": {
    "name": "SMB",
    "enabled": true,
    "running": true,
    "port": 445,
    "description": "Windows/SMB file sharing"
  },
  "nfs": {
    "name": "NFS",
    "enabled": false,
    "running": false,
    "port": 2049,
    "description": "NFS file sharing"
  },
  "afp": {
    "name": "AFP",
    "enabled": false,
    "running": false,
    "port": 548,
    "description": "Apple Filing Protocol (legacy)"
  },
  "ftp": {
    "name": "FTP",
    "enabled": false,
    "running": false,
    "port": 21,
    "description": "FTP file transfer"
  },
  "ssh": {
    "name": "SSH",
    "enabled": true,
    "running": true,
    "port": 22,
    "description": "Secure Shell remote access"
  },
  "telnet": {
    "name": "Telnet",
    "enabled": false,
    "running": false,
    "port": 23,
    "description": "Telnet remote access (insecure)"
  },
  "avahi": {
    "name": "Avahi",
    "enabled": true,
    "running": true,
    "port": 5353,
    "description": "mDNS/DNS-SD service discovery"
  },
  "netbios": {
    "name": "NetBIOS",
    "enabled": true,
    "running": true,
    "port": 137,
    "description": "NetBIOS name service"
  },
  "wsd": {
    "name": "WSD",
    "enabled": true,
    "running": false,
    "port": 3702,
    "description": "Web Services Discovery for Windows"
  },
  "wireguard": {
    "name": "WireGuard",
    "enabled": false,
    "running": false,
    "port": 51820,
    "description": "WireGuard VPN"
  },
  "upnp": {
    "name": "UPnP",
    "enabled": false,
    "running": false,
    "port": 1900,
    "description": "UPnP/IGD port forwarding"
  },
  "ntp": {
    "name": "NTP",
    "enabled": true,
    "running": false,
    "port": 123,
    "description": "Network Time Protocol"
  },
  "syslog_server": {
    "name": "Syslog",
    "enabled": false,
    "running": true,
    "port": 514,
    "description": "Syslog daemon"
  },
  "total_services": 13,
  "enabled_services": 6,
  "running_services": 5,
  "timestamp": "2025-10-03T13:41:13+10:00"
}
```

**Fields (per service)**:

| Field         | Type   | Description                          |
| ------------- | ------ | ------------------------------------ |
| `name`        | string | Service display name                 |
| `enabled`     | bool   | Whether service is enabled in config |
| `running`     | bool   | Whether service is currently running |
| `port`        | int    | Primary network port (if applicable) |
| `description` | string | Human-readable service description   |

**Summary Fields**:

| Field              | Type | Description                          |
| ------------------ | ---- | ------------------------------------ |
| `total_services`   | int  | Total number of monitored services   |
| `enabled_services` | int  | Number of services enabled in config |
| `running_services` | int  | Number of services currently running |

**Use Case**: Monitor network service availability, build status dashboards, detect service outages.

---

### GET /array/parity-check/schedule

Get parity check schedule configuration.

**Response**:

```json
{
  "enabled": true,
  "frequency": "monthly",
  "day_of_week": 0,
  "day_of_month": 1,
  "hour": 3,
  "minute": 0,
  "write_corrections": true,
  "timestamp": "2025-10-03T13:41:13+10:00"
}
```

**Fields**:

| Field               | Type   | Description                                                |
| ------------------- | ------ | ---------------------------------------------------------- |
| `enabled`           | bool   | Whether scheduled parity checks are enabled                |
| `frequency`         | string | Schedule frequency: "daily", "weekly", "monthly", "custom" |
| `day_of_week`       | int    | Day of week for weekly checks (0 = Sunday)                 |
| `day_of_month`      | int    | Day of month for monthly checks                            |
| `hour`              | int    | Hour to start check (24-hour format)                       |
| `minute`            | int    | Minute to start check                                      |
| `write_corrections` | bool   | Whether to automatically write corrections                 |

---

### GET /plugins

Get list of installed plugins with version and update information.

**Response**:

```json
{
  "plugins": [
    {
      "name": "unassigned.devices",
      "version": "2024.08.09",
      "author": "dlandon",
      "icon": "/plugins/unassigned.devices/images/unassigned.devices.png",
      "support_url": "https://forums.unraid.net/topic/...",
      "update_available": true,
      "update_version": "2024.10.15"
    }
  ],
  "total_plugins": 12,
  "plugin_updates_count": 3,
  "timestamp": "2025-10-03T13:41:13+10:00"
}
```

---

### GET /updates

Get OS and plugin update availability.

**Response**:

```json
{
  "os_update_available": true,
  "current_version": "6.12.10",
  "available_version": "6.12.11",
  "plugin_updates_count": 3,
  "plugins_with_updates": [
    "unassigned.devices",
    "dynamix.docker.manager",
    "user.scripts"
  ],
  "timestamp": "2025-10-03T13:41:13+10:00"
}
```

---

### GET /system/flash

Get USB flash boot drive health information.

**Response**:

```json
{
  "device": "/dev/sda1",
  "mount_point": "/boot",
  "filesystem": "vfat",
  "total_bytes": 32019734528,
  "used_bytes": 1073741824,
  "free_bytes": 30945992704,
  "used_percent": 3.4,
  "timestamp": "2025-10-03T13:41:13+10:00"
}
```

**Use Case**: Monitor flash drive space usage to prevent boot issues from a full flash drive.

---

### GET /network/{interface}/config

Get network interface configuration.

**Path Parameters**:

| Parameter   | Type   | Required | Description            | Examples                       |
| ----------- | ------ | -------- | ---------------------- | ------------------------------ |
| `interface` | string | Yes      | Network interface name | `eth0`, `eth1`, `bond0`, `br0` |

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

## Prometheus Metrics

### GET /metrics

Prometheus exposition format endpoint for native Grafana integration.

**URL**: `http://YOUR_UNRAID_IP:8043/metrics`

**Note**: This endpoint is at the root level (`/metrics`), not under `/api/v1/`.

**Response Format**: `text/plain; version=0.0.4; charset=utf-8` (Prometheus exposition format)

#### Available Metrics

| Category     | Metric                              | Type  | Labels                     | Description                        |
| ------------ | ----------------------------------- | ----- | -------------------------- | ---------------------------------- |
| **System**   | `unraid_system_info`                | Gauge | version, hostname          | Unraid system information          |
|              | `unraid_system_uptime_seconds`      | Gauge | -                          | System uptime in seconds           |
|              | `unraid_cpu_usage_percent`          | Gauge | -                          | Current CPU usage percentage       |
|              | `unraid_cpu_temperature_celsius`    | Gauge | -                          | CPU temperature                    |
|              | `unraid_memory_total_bytes`         | Gauge | -                          | Total system memory                |
|              | `unraid_memory_used_bytes`          | Gauge | -                          | Used system memory                 |
|              | `unraid_memory_free_bytes`          | Gauge | -                          | Free system memory                 |
|              | `unraid_memory_usage_percent`       | Gauge | -                          | Memory usage percentage            |
| **Array**    | `unraid_array_state`                | Gauge | state                      | Array state (1=started, 0=stopped) |
|              | `unraid_array_total_bytes`          | Gauge | -                          | Total array capacity               |
|              | `unraid_array_used_bytes`           | Gauge | -                          | Used array space                   |
|              | `unraid_array_free_bytes`           | Gauge | -                          | Free array space                   |
|              | `unraid_parity_check_running`       | Gauge | -                          | Whether parity check is running    |
|              | `unraid_parity_check_progress`      | Gauge | -                          | Parity check progress (0-100)      |
|              | `unraid_parity_check_errors`        | Gauge | -                          | Number of parity errors found      |
| **Disks**    | `unraid_disk_temperature_celsius`   | Gauge | name, device, type         | Disk temperature                   |
|              | `unraid_disk_size_bytes`            | Gauge | name, device, type         | Disk total size                    |
|              | `unraid_disk_used_bytes`            | Gauge | name, device, type         | Disk used space                    |
|              | `unraid_disk_status`                | Gauge | name, device, type, status | Disk status (1=healthy, 0=problem) |
|              | `unraid_disk_standby`               | Gauge | name, device, type         | Disk standby state                 |
|              | `unraid_disk_smart_status`          | Gauge | name, device, type         | SMART status (1=passed, 0=failed)  |
| **Docker**   | `unraid_docker_container_state`     | Gauge | name, id, image            | Container state (1=running)        |
|              | `unraid_docker_containers_total`    | Gauge | -                          | Total number of containers         |
|              | `unraid_docker_containers_running`  | Gauge | -                          | Number of running containers       |
| **VMs**      | `unraid_vm_state`                   | Gauge | name, id                   | VM state (1=running, 2=paused)     |
|              | `unraid_vms_total`                  | Gauge | -                          | Total number of VMs                |
|              | `unraid_vms_running`                | Gauge | -                          | Number of running VMs              |
| **UPS**      | `unraid_ups_status`                 | Gauge | status                     | UPS status (1=online)              |
|              | `unraid_ups_battery_charge_percent` | Gauge | -                          | Battery charge percentage          |
|              | `unraid_ups_load_percent`           | Gauge | -                          | UPS load percentage                |
|              | `unraid_ups_runtime_seconds`        | Gauge | -                          | Remaining runtime                  |
| **Shares**   | `unraid_share_used_bytes`           | Gauge | name                       | Share used space                   |
|              | `unraid_shares_total`               | Gauge | -                          | Total number of shares             |
| **Services** | `unraid_service_enabled`            | Gauge | service                    | Service enabled state              |
|              | `unraid_service_running`            | Gauge | service                    | Service running state              |
| **GPU**      | `unraid_gpu_temperature_celsius`    | Gauge | name, index                | GPU temperature                    |
|              | `unraid_gpu_utilization_percent`    | Gauge | name, index                | GPU utilization                    |
|              | `unraid_gpu_memory_used_bytes`      | Gauge | name, index                | GPU memory used                    |
|              | `unraid_gpu_memory_total_bytes`     | Gauge | name, index                | GPU memory total                   |
|              | `unraid_gpu_power_watts`            | Gauge | name, index                | GPU power draw                     |

#### Example Response

```
# HELP unraid_system_info Unraid system information
# TYPE unraid_system_info gauge
unraid_system_info{hostname="Tower",version="7.1.0"} 1
# HELP unraid_cpu_usage_percent Current CPU usage percentage
# TYPE unraid_cpu_usage_percent gauge
unraid_cpu_usage_percent 12.5
# HELP unraid_array_state Array state (1=started, 0=stopped)
# TYPE unraid_array_state gauge
unraid_array_state{state="started"} 1
# HELP unraid_disk_temperature_celsius Disk temperature in Celsius
# TYPE unraid_disk_temperature_celsius gauge
unraid_disk_temperature_celsius{device="sda",name="disk1",type="data"} 35
unraid_disk_temperature_celsius{device="sdb",name="disk2",type="data"} 36
unraid_disk_temperature_celsius{device="sdc",name="parity",type="parity"} 38
# HELP unraid_docker_container_state Container state (1=running, 0=stopped)
# TYPE unraid_docker_container_state gauge
unraid_docker_container_state{id="abc123",image="linuxserver/plex",name="plex"} 1
unraid_docker_container_state{id="def456",image="linuxserver/sonarr",name="sonarr"} 0
```

#### Grafana Integration

1. Add Prometheus data source in Grafana pointing to `http://YOUR_UNRAID_IP:9090`
2. Configure Prometheus to scrape the Unraid endpoint:

```yaml
# prometheus.yml
scrape_configs:
  - job_name: "unraid"
    static_configs:
      - targets: ["YOUR_UNRAID_IP:8043"]
    metrics_path: /metrics
    scrape_interval: 15s
```

1. Import or create dashboards using the available metrics

#### Example Grafana Queries

```promql
# CPU usage over time
rate(unraid_cpu_usage_percent[5m])

# Disk temperatures by disk
unraid_disk_temperature_celsius

# Container status
unraid_docker_container_state{name="plex"}

# Array usage percentage
(unraid_array_used_bytes / unraid_array_total_bytes) * 100

# Running containers count
unraid_docker_containers_running

# UPS battery level
unraid_ups_battery_charge_percent
```

---

## Security Best Practices

### Network Security

**⚠️ IMPORTANT**: Do NOT expose the API directly to the internet without proper security measures.

#### Recommended Security Options

**Option 1: VPN Access (Most Secure)**

- Use WireGuard or OpenVPN to create a secure tunnel
- Access API only through VPN connection
- No direct internet exposure
- Best for remote access scenarios

**Option 2: Reverse Proxy with SSL/TLS**

- Use nginx, Caddy, or Traefik as reverse proxy
- Terminate SSL at the proxy
- Add authentication layer
- Enable rate limiting

**Option 3: Firewall Rules**

- Restrict access to trusted IP addresses
- Use iptables or UFW
- Block all other traffic
- Good for local network with specific remote IPs

---

### SSL/TLS Setup with nginx

#### nginx Configuration

```nginx
# /etc/nginx/sites-available/unraid-api

# Redirect HTTP to HTTPS
server {
    listen 80;
    server_name unraid-api.example.com;
    return 301 https://$server_name$request_uri;
}

# HTTPS server
server {
    listen 443 ssl http2;
    server_name unraid-api.example.com;

    # SSL certificates (use Let's Encrypt)
    ssl_certificate /etc/letsencrypt/live/unraid-api.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/unraid-api.example.com/privkey.pem;

    # SSL configuration
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers HIGH:!aNULL:!MD5;
    ssl_prefer_server_ciphers on;
    ssl_session_cache shared:SSL:10m;
    ssl_session_timeout 10m;

    # Security headers
    add_header Strict-Transport-Security "max-age=31536000; includeSubDomains" always;
    add_header X-Frame-Options "SAMEORIGIN" always;
    add_header X-Content-Type-Options "nosniff" always;
    add_header X-XSS-Protection "1; mode=block" always;

    # API proxy
    location /api/ {
        proxy_pass http://localhost:8043/api/;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # Timeouts
        proxy_connect_timeout 30s;
        proxy_send_timeout 30s;
        proxy_read_timeout 30s;
    }

    # WebSocket proxy
    location /api/v1/ws {
        proxy_pass http://localhost:8043/api/v1/ws;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # WebSocket timeouts
        proxy_connect_timeout 7d;
        proxy_send_timeout 7d;
        proxy_read_timeout 7d;
    }
}
```

#### Enable Configuration

```bash
# Create symbolic link
sudo ln -s /etc/nginx/sites-available/unraid-api /etc/nginx/sites-enabled/

# Test configuration
sudo nginx -t

# Reload nginx
sudo systemctl reload nginx
```

---

### CORS Configuration

If accessing the API from web applications, configure CORS headers:

#### nginx CORS Configuration

```nginx
location /api/ {
    # CORS headers
    if ($request_method = 'OPTIONS') {
        add_header Access-Control-Allow-Origin "https://your-app.example.com" always;
        add_header Access-Control-Allow-Methods "GET, POST, OPTIONS" always;
        add_header Access-Control-Allow-Headers "Content-Type, Authorization" always;
        add_header Access-Control-Max-Age 3600 always;
        add_header Content-Length 0;
        add_header Content-Type text/plain;
        return 204;
    }

    add_header Access-Control-Allow-Origin "https://your-app.example.com" always;
    add_header Access-Control-Allow-Methods "GET, POST, OPTIONS" always;
    add_header Access-Control-Allow-Headers "Content-Type, Authorization" always;

    proxy_pass http://localhost:8043/api/;
    # ... rest of proxy configuration
}
```

#### Application-Level CORS (JavaScript)

```javascript
// Configure axios with credentials
const api = axios.create({
  baseURL: "https://unraid-api.example.com/api/v1",
  withCredentials: true,
  headers: {
    "Content-Type": "application/json",
  },
});
```

---

### Rate Limiting Implementation

Protect your API from abuse with rate limiting:

#### nginx Rate Limiting

```nginx
# Define rate limit zone (10 requests per second per IP)
limit_req_zone $binary_remote_addr zone=api_limit:10m rate=10r/s;

# Define connection limit (max 10 concurrent connections per IP)
limit_conn_zone $binary_remote_addr zone=api_conn:10m;

server {
    # ... SSL configuration ...

    location /api/ {
        # Apply rate limiting
        limit_req zone=api_limit burst=20 nodelay;
        limit_conn api_conn 10;

        # Custom error responses
        limit_req_status 429;
        limit_conn_status 429;

        proxy_pass http://localhost:8043/api/;
        # ... rest of proxy configuration
    }
}
```

#### Rate Limit Response

When rate limit is exceeded, nginx returns:

```
HTTP/1.1 429 Too Many Requests
```

---

### Authentication Roadmap

**Current Status**: No authentication required

**Future Plans**:

- API key authentication
- JWT token-based authentication
- OAuth 2.0 support
- Role-based access control (RBAC)

**Interim Solution**: Use reverse proxy authentication:

```nginx
location /api/ {
    # Basic authentication
    auth_basic "Unraid API";
    auth_basic_user_file /etc/nginx/.htpasswd;

    proxy_pass http://localhost:8043/api/;
    # ... rest of proxy configuration
}
```

Create password file:

```bash
sudo htpasswd -c /etc/nginx/.htpasswd username
```

---

### IP Whitelisting

Restrict access to specific IP addresses:

#### nginx IP Whitelisting

```nginx
# Define allowed IPs
geo $allowed_ip {
    default 0;
    192.168.1.0/24 1;      # Local network
    10.0.0.0/8 1;          # VPN network
    203.0.113.10 1;        # Specific remote IP
}

server {
    # ... SSL configuration ...

    location /api/ {
        # Check if IP is allowed
        if ($allowed_ip = 0) {
            return 403;
        }

        proxy_pass http://localhost:8043/api/;
        # ... rest of proxy configuration
    }
}
```

#### Firewall Rules (iptables)

```bash
# Allow local network
sudo iptables -A INPUT -p tcp --dport 8043 -s 192.168.1.0/24 -j ACCEPT

# Allow specific remote IP
sudo iptables -A INPUT -p tcp --dport 8043 -s 203.0.113.10 -j ACCEPT

# Block all other traffic
sudo iptables -A INPUT -p tcp --dport 8043 -j DROP

# Save rules
sudo iptables-save > /etc/iptables/rules.v4
```

---

### Security Checklist

Before deploying to production:

- [ ] **SSL/TLS enabled** - Use valid certificates (Let's Encrypt recommended)
- [ ] **Reverse proxy configured** - nginx, Caddy, or Traefik
- [ ] **Rate limiting enabled** - Protect against abuse
- [ ] **IP whitelisting** - Restrict to known IPs if possible
- [ ] **Authentication added** - At minimum, basic auth via reverse proxy
- [ ] **CORS configured** - Only allow trusted origins
- [ ] **Firewall rules** - Block direct access to port 8043
- [ ] **Monitoring enabled** - Track API usage and errors
- [ ] **Logs reviewed** - Check for suspicious activity
- [ ] **Backups configured** - Regular backups of Unraid configuration

---

## Rate Limiting

### Current Implementation

Currently, there is no rate limiting implemented in the API itself. Use responsibly to avoid overloading the Unraid server.

### Recommended Limits

**For Production Use**:

- **GET requests**: 60 requests per minute per IP
- **POST requests**: 20 requests per minute per IP
- **WebSocket connections**: 5 concurrent connections per IP
- **Burst allowance**: 2x normal rate for short periods

**Implementation**: Use reverse proxy rate limiting (see [Security Best Practices](#security-best-practices))

### Client-Side Rate Limiting

Implement client-side throttling to avoid hitting limits:

#### Python Example

```python
import time
from functools import wraps

def rate_limit(calls_per_second=2):
    """Decorator to rate limit function calls."""
    min_interval = 1.0 / calls_per_second
    last_called = [0.0]

    def decorator(func):
        @wraps(func)
        def wrapper(*args, **kwargs):
            elapsed = time.time() - last_called[0]
            wait_time = min_interval - elapsed
            if wait_time > 0:
                time.sleep(wait_time)
            result = func(*args, **kwargs)
            last_called[0] = time.time()
            return result
        return wrapper
    return decorator

@rate_limit(calls_per_second=2)
def get_system_info(base_url):
    response = requests.get(f"{base_url}/system")
    return response.json()

# Usage - automatically rate limited to 2 calls per second
for i in range(10):
    info = get_system_info(BASE_URL)
    print(f"CPU: {info['cpu_usage_percent']}%")
```

#### JavaScript Example

```javascript
class RateLimiter {
  constructor(callsPerSecond = 2) {
    this.minInterval = 1000 / callsPerSecond;
    this.lastCalled = 0;
  }

  async throttle() {
    const now = Date.now();
    const elapsed = now - this.lastCalled;
    const waitTime = this.minInterval - elapsed;

    if (waitTime > 0) {
      await new Promise((resolve) => setTimeout(resolve, waitTime));
    }

    this.lastCalled = Date.now();
  }

  async execute(fn) {
    await this.throttle();
    return fn();
  }
}

// Usage
const limiter = new RateLimiter(2); // 2 calls per second

async function getSystemInfo() {
  return limiter.execute(async () => {
    const response = await fetch(`${BASE_URL}/system`);
    return response.json();
  });
}

// Automatically rate limited
for (let i = 0; i < 10; i++) {
  const info = await getSystemInfo();
  console.log(`CPU: ${info.cpu_usage_percent}%`);
}
```

---

## Best Practices

### 1. Use WebSocket for Real-Time Data

WebSocket connections are more efficient than polling for real-time updates.

**❌ Bad: Polling**

```python
import time
import requests

# Inefficient polling
while True:
    response = requests.get(f"{BASE_URL}/system")
    system_info = response.json()
    print(f"CPU: {system_info['cpu_usage_percent']}%")
    time.sleep(5)  # Poll every 5 seconds
```

**✅ Good: WebSocket**

```python
import websocket
import json

def on_message(ws, message):
    event = json.loads(message)
    if 'cpu_usage_percent' in event.get('data', {}):
        print(f"CPU: {event['data']['cpu_usage_percent']}%")

ws = websocket.WebSocketApp(
    "ws://192.168.20.21:8043/api/v1/ws",
    on_message=on_message
)
ws.run_forever()
```

---

### 2. Implement Retry Logic with Exponential Backoff

Handle transient failures gracefully with retry logic.

#### Python Example

```python
import time
import requests
from functools import wraps

def retry_with_backoff(max_retries=3, base_delay=1, max_delay=60):
    """Decorator for retry logic with exponential backoff."""
    def decorator(func):
        @wraps(func)
        def wrapper(*args, **kwargs):
            retries = 0
            while retries < max_retries:
                try:
                    return func(*args, **kwargs)
                except requests.exceptions.RequestException as e:
                    retries += 1
                    if retries >= max_retries:
                        raise

                    # Exponential backoff: 1s, 2s, 4s, 8s, ...
                    delay = min(base_delay * (2 ** (retries - 1)), max_delay)
                    print(f"Request failed: {e}. Retrying in {delay}s... ({retries}/{max_retries})")
                    time.sleep(delay)

            return None
        return wrapper
    return decorator

@retry_with_backoff(max_retries=3, base_delay=1)
def get_system_info(base_url):
    response = requests.get(f"{base_url}/system", timeout=5)
    response.raise_for_status()
    return response.json()

# Usage - automatically retries on failure
try:
    info = get_system_info(BASE_URL)
    print(f"CPU: {info['cpu_usage_percent']}%")
except Exception as e:
    print(f"Failed after retries: {e}")
```

#### JavaScript Example

```javascript
async function retryWithBackoff(fn, maxRetries = 3, baseDelay = 1000) {
  let retries = 0;

  while (retries < maxRetries) {
    try {
      return await fn();
    } catch (error) {
      retries++;

      if (retries >= maxRetries) {
        throw error;
      }

      // Exponential backoff: 1s, 2s, 4s, 8s, ...
      const delay = Math.min(baseDelay * Math.pow(2, retries - 1), 60000);
      console.log(
        `Request failed: ${error.message}. Retrying in ${delay}ms... (${retries}/${maxRetries})`,
      );

      await new Promise((resolve) => setTimeout(resolve, delay));
    }
  }
}

// Usage
async function getSystemInfo() {
  return retryWithBackoff(
    async () => {
      const response = await fetch(`${BASE_URL}/system`);
      if (!response.ok) {
        throw new Error(`HTTP ${response.status}`);
      }
      return response.json();
    },
    3,
    1000,
  );
}

try {
  const info = await getSystemInfo();
  console.log(`CPU: ${info.cpu_usage_percent}%`);
} catch (error) {
  console.error("Failed after retries:", error);
}
```

---

### 3. Cache Responses Appropriately

Cache frequently accessed data to reduce API calls and improve performance.

#### Python Caching Example

```python
import time
import requests
from functools import lru_cache

class UnraidAPIClient:
    def __init__(self, base_url):
        self.base_url = base_url
        self._cache = {}
        self._cache_ttl = {}

    def _get_cached(self, key, ttl_seconds):
        """Get cached value if not expired."""
        if key in self._cache:
            if time.time() - self._cache_ttl[key] < ttl_seconds:
                return self._cache[key]
        return None

    def _set_cache(self, key, value):
        """Set cached value with timestamp."""
        self._cache[key] = value
        self._cache_ttl[key] = time.time()

    def get_system_info(self, use_cache=True, ttl=5):
        """Get system info with 5-second cache."""
        cache_key = 'system_info'

        if use_cache:
            cached = self._get_cached(cache_key, ttl)
            if cached:
                return cached

        response = requests.get(f"{self.base_url}/system")
        data = response.json()
        self._set_cache(cache_key, data)
        return data

    def get_disk_settings(self, use_cache=True, ttl=300):
        """Get disk settings with 5-minute cache (rarely changes)."""
        cache_key = 'disk_settings'

        if use_cache:
            cached = self._get_cached(cache_key, ttl)
            if cached:
                return cached

        response = requests.get(f"{self.base_url}/settings/disks")
        data = response.json()
        self._set_cache(cache_key, data)
        return data

# Usage
client = UnraidAPIClient(BASE_URL)

# First call - fetches from API
info1 = client.get_system_info()

# Second call within 5 seconds - returns cached value
info2 = client.get_system_info()

# Force fresh data
info3 = client.get_system_info(use_cache=False)
```

#### Recommended Cache TTLs

| Endpoint           | Recommended TTL | Reason                             |
| ------------------ | --------------- | ---------------------------------- |
| `/health`          | 5 seconds       | Fast-changing, health checks       |
| `/system`          | 5-10 seconds    | Frequently updated metrics         |
| `/array`           | 10 seconds      | Array state changes occasionally   |
| `/disks`           | 30 seconds      | Disk metrics update slowly         |
| `/docker`          | 10 seconds      | Container states change frequently |
| `/vm`              | 10 seconds      | VM states change frequently        |
| `/settings/disks`  | 5 minutes       | Configuration rarely changes       |
| `/settings/system` | 5 minutes       | Configuration rarely changes       |
| `/shares`          | 1 minute        | Share data updates slowly          |

---

### 4. Set Appropriate Timeouts

Different operations require different timeout values.

#### Recommended Timeouts

| Operation Type               | Timeout       | Reason                    |
| ---------------------------- | ------------- | ------------------------- |
| Health checks                | 2-5 seconds   | Should be fast            |
| GET requests                 | 5-10 seconds  | Data retrieval            |
| POST operations (start/stop) | 30-60 seconds | May take time to complete |
| Parity check operations      | 60 seconds    | Long-running operations   |
| WebSocket connection         | No timeout    | Persistent connection     |

#### Python Example

```python
import requests

# Short timeout for health checks
try:
    response = requests.get(f"{BASE_URL}/health", timeout=2)
except requests.exceptions.Timeout:
    print("Health check timed out")

# Standard timeout for GET requests
response = requests.get(f"{BASE_URL}/system", timeout=5)

# Longer timeout for POST operations
response = requests.post(f"{BASE_URL}/array/start", timeout=30)

# Very long timeout for parity check
response = requests.post(f"{BASE_URL}/array/parity-check/start", timeout=60)
```

#### JavaScript Example

```javascript
// Create axios instance with default timeout
const api = axios.create({
  baseURL: BASE_URL,
  timeout: 5000, // 5 seconds default
});

// Override timeout for specific operations
async function startArray() {
  const { data } = await api.post(
    "/array/start",
    {},
    {
      timeout: 30000, // 30 seconds for array start
    },
  );
  return data;
}
```

---

### 5. Respect Disk Spindown Delay

Avoid waking spun-down disks unnecessarily by checking spindown settings.

#### Python Example

```python
import requests

def should_query_disk_smart(base_url, disk_id):
    """Check if we should query SMART data (might wake disk)."""
    # Get disk settings
    settings = requests.get(f"{base_url}/settings/disks").json()
    spindown_delay = settings.get('spindown_delay_minutes', 0)

    if spindown_delay == 0:
        # Spindown disabled, safe to query
        return True

    # Get current disk state
    disk = requests.get(f"{base_url}/disks/{disk_id}").json()
    spin_state = disk.get('spin_state', 'active')

    if spin_state == 'standby':
        print(f"Disk {disk_id} is in standby. Skipping SMART query to avoid wake.")
        return False

    return True

# Usage
if should_query_disk_smart(BASE_URL, 'disk1'):
    # Safe to query SMART data
    disk = requests.get(f"{BASE_URL}/disks/disk1").json()
    print(f"Temperature: {disk['temperature_celsius']}°C")
else:
    print("Skipping query to preserve disk spindown")
```

---

### 6. Use Specific Endpoints

Use specific resource endpoints instead of listing all resources when possible.

**❌ Bad: List all then filter**

```python
# Inefficient - fetches all disks
disks = requests.get(f"{BASE_URL}/disks").json()
parity_disk = next(d for d in disks if d['name'] == 'parity')
```

**✅ Good: Direct access**

```python
# Efficient - fetches only the disk you need
parity_disk = requests.get(f"{BASE_URL}/disks/parity").json()
```

---

### 7. Connection Pooling

Reuse HTTP connections for better performance.

#### Python Example

```python
import requests

# Create session for connection pooling
session = requests.Session()

# Configure connection pool
adapter = requests.adapters.HTTPAdapter(
    pool_connections=10,
    pool_maxsize=20,
    max_retries=3
)
session.mount('http://', adapter)

# Reuse session for all requests
def get_system_info():
    return session.get(f"{BASE_URL}/system").json()

def get_array_status():
    return session.get(f"{BASE_URL}/array").json()

# All requests reuse connections
for i in range(100):
    info = get_system_info()
    array = get_array_status()
```

#### JavaScript Example

```javascript
// axios automatically pools connections
const api = axios.create({
  baseURL: BASE_URL,
  timeout: 5000,
  // Connection pooling is automatic
  maxRedirects: 5,
  httpAgent: new http.Agent({ keepAlive: true }),
  httpsAgent: new https.Agent({ keepAlive: true }),
});
```

---

### 8. Batch Operations

When performing multiple operations, batch them efficiently.

#### Python Example

```python
import asyncio
import aiohttp

async def fetch_all_resources(base_url):
    """Fetch multiple resources concurrently."""
    async with aiohttp.ClientSession() as session:
        # Create tasks for concurrent requests
        tasks = [
            session.get(f"{base_url}/system"),
            session.get(f"{base_url}/array"),
            session.get(f"{base_url}/disks"),
            session.get(f"{base_url}/docker"),
            session.get(f"{base_url}/vm")
        ]

        # Execute all requests concurrently
        responses = await asyncio.gather(*tasks)

        # Parse all responses
        results = {
            'system': await responses[0].json(),
            'array': await responses[1].json(),
            'disks': await responses[2].json(),
            'docker': await responses[3].json(),
            'vm': await responses[4].json()
        }

        return results

# Usage
results = asyncio.run(fetch_all_resources(BASE_URL))
print(f"CPU: {results['system']['cpu_usage_percent']}%")
print(f"Array: {results['array']['state']}")
```

---

### 9. Error Handling Best Practices

Always handle errors gracefully and provide meaningful feedback.

#### Python Example

```python
import requests

def safe_api_call(url, operation_name):
    """Make API call with comprehensive error handling."""
    try:
        response = requests.get(url, timeout=5)
        response.raise_for_status()
        return response.json()

    except requests.exceptions.Timeout:
        print(f"{operation_name}: Request timed out")
        return None

    except requests.exceptions.ConnectionError:
        print(f"{operation_name}: Could not connect to server")
        return None

    except requests.exceptions.HTTPError as e:
        if e.response.status_code == 404:
            print(f"{operation_name}: Resource not found")
        elif e.response.status_code == 409:
            error = e.response.json()
            print(f"{operation_name}: Conflict - {error.get('message')}")
        else:
            print(f"{operation_name}: HTTP error {e.response.status_code}")
        return None

    except Exception as e:
        print(f"{operation_name}: Unexpected error - {e}")
        return None

# Usage
system_info = safe_api_call(f"{BASE_URL}/system", "Get System Info")
if system_info:
    print(f"CPU: {system_info['cpu_usage_percent']}%")
```

---

### 10. Monitoring and Logging

Implement proper logging for debugging and monitoring.

#### Python Example

```python
import logging
import requests

# Configure logging
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(name)s - %(levelname)s - %(message)s'
)
logger = logging.getLogger('unraid_api')

class UnraidAPIClient:
    def __init__(self, base_url):
        self.base_url = base_url

    def get_system_info(self):
        logger.info("Fetching system information")
        try:
            response = requests.get(f"{self.base_url}/system", timeout=5)
            response.raise_for_status()
            data = response.json()
            logger.info(f"System info retrieved: CPU {data['cpu_usage_percent']}%")
            return data
        except Exception as e:
            logger.error(f"Failed to get system info: {e}")
            raise

# Usage
client = UnraidAPIClient(BASE_URL)
info = client.get_system_info()
```

---

### Summary of Best Practices

1. ✅ **Use WebSocket** for real-time data instead of polling
2. ✅ **Implement retry logic** with exponential backoff for transient failures
3. ✅ **Cache responses** with appropriate TTLs to reduce API calls
4. ✅ **Set timeouts** based on operation type (2s for health, 30s for operations)
5. ✅ **Respect spindown** - Check disk state before querying SMART data
6. ✅ **Use specific endpoints** - `/disks/{id}` instead of `/disks` when possible
7. ✅ **Connection pooling** - Reuse HTTP connections for better performance
8. ✅ **Batch operations** - Fetch multiple resources concurrently
9. ✅ **Handle errors** - Comprehensive error handling with meaningful messages
10. ✅ **Monitor and log** - Implement proper logging for debugging

---

**Last Updated**: 2025-10-03
**API Version**: 1.0.0
