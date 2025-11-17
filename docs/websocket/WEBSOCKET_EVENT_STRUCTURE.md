# WebSocket Event Structure Documentation

## Overview

This document provides detailed technical documentation of the WebSocket event structure used by the Unraid Management Agent. It covers the envelope format, data type definitions, field specifications, and validation rules.

**Target Audience**: Developers integrating with the WebSocket API  
**Version**: 1.1.0  
**Last Updated**: 2025-10-02

---

## Table of Contents

1. [Event Envelope Structure](#event-envelope-structure)
2. [Data Type Definitions](#data-type-definitions)
3. [Event Identification Logic](#event-identification-logic)
4. [Field Specifications](#field-specifications)
5. [Validation Rules](#validation-rules)
6. [Type Mappings](#type-mappings)
7. [Implementation Examples](#implementation-examples)

---

## Event Envelope Structure

### Base Event Format

All WebSocket events are wrapped in a standard envelope defined by the `WSEvent` DTO:

```go
// daemon/dto/websocket.go
type WSEvent struct {
    Event     string      `json:"event"`
    Timestamp time.Time   `json:"timestamp"`
    Data      interface{} `json:"data"`
}
```

### Envelope Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `event` | string | Yes | Always "update" for all event types |
| `timestamp` | string (ISO 8601) | Yes | Server timestamp when event was created |
| `data` | object/array | Yes | Event-specific payload (varies by type) |

### Example Envelope

```json
{
  "event": "update",
  "timestamp": "2025-10-02T14:02:59.850035377+10:00",
  "data": {
    "hostname": "Tower",
    "cpu_usage_percent": 12.5,
    ...
  }
}
```

### Important Notes

1. **No Type Field**: Events do NOT include a `type` field in the envelope
2. **Type Identification**: Event types are determined by inspecting the `data` structure
3. **Timestamp Format**: RFC3339Nano format with timezone offset
4. **Data Variability**: The `data` field can be an object or array depending on event type

---

## Data Type Definitions

### Primitive Types

| Type | Go Type | JSON Type | Description |
|------|---------|-----------|-------------|
| String | `string` | string | UTF-8 encoded text |
| Integer | `int`, `int64` | number | Whole numbers |
| Float | `float64` | number | Decimal numbers |
| Boolean | `bool` | boolean | true/false values |
| Timestamp | `time.Time` | string | ISO 8601 formatted datetime |

### Complex Types

#### Array Types
- **Disk List**: `[]dto.DiskInfo`
- **Container List**: `[]dto.ContainerInfo`
- **VM List**: `[]dto.VMInfo`
- **GPU List**: `[]*dto.GPUMetrics`
- **Network List**: `[]dto.NetworkInfo`
- **Share List**: `[]dto.ShareInfo`

#### Object Types
- **System Info**: `dto.SystemInfo`
- **Array Status**: `dto.ArrayStatus`
- **UPS Status**: `dto.UPSStatus`

---

## Event Identification Logic

### Identification Algorithm

Since events lack a `type` field, clients must inspect the `data` structure to determine the event type:

```
1. Check if data is an array or object
2. If array, inspect first element (or return "empty_list" if empty)
3. Check for unique field combinations that identify each event type
4. Return event type or "unknown" if no match
```

### Identification Rules

| Event Type | Identification Rule |
|------------|---------------------|
| `system_update` | Has `hostname` AND `cpu_usage_percent` |
| `array_status_update` | Has `state` AND `parity_check_status` AND `num_disks` |
| `ups_status_update` | Has `connected` AND `battery_charge_percent` |
| `gpu_update` | Has `available` AND `driver_version` AND `utilization_gpu_percent` |
| `network_list_update` | Has `mac_address` AND `bytes_received` |
| `container_list_update` | Has `image` AND `ports` |
| `vm_list_update` | Has `state` AND `vcpus` |
| `disk_list_update` | Has `device` AND `mount_point` |
| `share_list_update` | Has `name` AND `path` AND `size_bytes` |

### Reference Implementation

```python
def identify_event_type(data):
    """Identify event type from data structure."""
    # Handle arrays - check first element
    if isinstance(data, list):
        if not data:
            return "empty_list"
        data = data[0]
    
    # Must be a dict to identify
    if not isinstance(data, dict):
        return "unknown"
    
    # System update
    if "hostname" in data and "cpu_usage_percent" in data:
        return "system_update"
    
    # Array status
    if "state" in data and "parity_check_status" in data and "num_disks" in data:
        return "array_status_update"
    
    # UPS status
    if "connected" in data and "battery_charge_percent" in data:
        return "ups_status_update"
    
    # GPU metrics
    if "available" in data and "driver_version" in data and "utilization_gpu_percent" in data:
        return "gpu_update"
    
    # Network interface
    if "mac_address" in data and "bytes_received" in data:
        return "network_list_update"
    
    # Container
    if "image" in data and "ports" in data:
        return "container_list_update"
    
    # VM
    if "state" in data and "vcpus" in data:
        return "vm_list_update"
    
    # Disk
    if "device" in data and "mount_point" in data:
        return "disk_list_update"
    
    # Share
    if "name" in data and "path" AND "size_bytes" in data:
        return "share_list_update"
    
    return "unknown"
```

---

## Field Specifications

### System Update Fields

| Field | Type | Unit | Range | Nullable | Description |
|-------|------|------|-------|----------|-------------|
| `hostname` | string | - | - | No | Server hostname |
| `version` | string | - | - | No | Agent version |
| `uptime_seconds` | int64 | seconds | ≥0 | No | System uptime |
| `cpu_usage_percent` | float64 | percent | 0-100 | No | CPU usage |
| `cpu_model` | string | - | - | No | CPU model name |
| `cpu_cores` | int | count | ≥1 | No | Physical cores |
| `cpu_threads` | int | count | ≥1 | No | Logical threads |
| `cpu_mhz` | float64 | MHz | ≥0 | No | CPU frequency |
| `cpu_temp_celsius` | float64 | °C | -273-∞ | Yes | CPU temperature |
| `ram_usage_percent` | float64 | percent | 0-100 | No | RAM usage |
| `ram_total_bytes` | int64 | bytes | ≥0 | No | Total RAM |
| `ram_used_bytes` | int64 | bytes | ≥0 | No | Used RAM |
| `ram_free_bytes` | int64 | bytes | ≥0 | No | Free RAM |
| `ram_buffers_bytes` | int64 | bytes | ≥0 | No | Buffer RAM |
| `ram_cached_bytes` | int64 | bytes | ≥0 | No | Cached RAM |
| `server_model` | string | - | - | No | Server model |
| `bios_version` | string | - | - | No | BIOS version |
| `bios_date` | string | - | - | No | BIOS date |
| `motherboard_temp_celsius` | float64 | °C | -273-∞ | Yes | MB temperature |
| `fans` | array | - | - | Yes | Fan sensors |
| `timestamp` | string | - | - | No | Event timestamp |

### Array Status Fields

| Field | Type | Unit | Range | Nullable | Description |
|-------|------|------|-------|----------|-------------|
| `state` | string | - | enum | No | Array state |
| `num_disks` | int | count | ≥0 | No | Total disks |
| `num_data_disks` | int | count | ≥0 | No | Data disks |
| `num_parity_disks` | int | count | 0-2 | No | Parity disks |
| `sync_percent` | float64 | percent | 0-100 | No | Sync progress |
| `parity_valid` | bool | - | - | No | Parity valid |
| `parity_check_status` | string | - | enum | No | Check status |
| `parity_check_running` | bool | - | - | No | Check active |
| `parity_check_progress` | float64 | percent | 0-100 | No | Check progress |
| `timestamp` | string | - | - | No | Event timestamp |

### Disk Info Fields

| Field | Type | Unit | Range | Nullable | Description |
|-------|------|------|-------|----------|-------------|
| `id` | string | - | - | No | Disk identifier |
| `device` | string | - | - | No | Device name |
| `name` | string | - | - | No | Disk name |
| `status` | string | - | enum | No | Disk status |
| `size_bytes` | int64 | bytes | ≥0 | No | Total size |
| `used_bytes` | int64 | bytes | ≥0 | No | Used space |
| `free_bytes` | int64 | bytes | ≥0 | No | Free space |
| `temperature_celsius` | float64 | °C | 0-∞ | Yes | Temperature |
| `smart_status` | string | - | enum | No | SMART status |
| `smart_errors` | int | count | ≥0 | No | Error count |
| `spindown_delay` | int | minutes | ≥0 | No | Spindown delay |
| `filesystem` | string | - | - | No | FS type |
| `mount_point` | string | - | - | No | Mount path |
| `usage_percent` | float64 | percent | 0-100 | No | Usage percent |
| `power_on_hours` | int64 | hours | ≥0 | No | Power-on time |
| `power_cycle_count` | int | count | ≥0 | No | Power cycles |
| `read_bytes` | int64 | bytes | ≥0 | No | Bytes read |
| `write_bytes` | int64 | bytes | ≥0 | No | Bytes written |
| `read_ops` | int64 | count | ≥0 | No | Read ops |
| `write_ops` | int64 | count | ≥0 | No | Write ops |
| `io_utilization_percent` | float64 | percent | 0-100 | No | I/O usage |
| `timestamp` | string | - | - | No | Event timestamp |

### Container Info Fields

| Field | Type | Unit | Range | Nullable | Description |
|-------|------|------|-------|----------|-------------|
| `id` | string | - | - | No | Container ID |
| `name` | string | - | - | No | Container name |
| `image` | string | - | - | No | Image name |
| `state` | string | - | enum | No | Container state |
| `status` | string | - | - | No | Status text |
| `cpu_percent` | float64 | percent | 0-∞ | No | CPU usage |
| `memory_usage_bytes` | int64 | bytes | ≥0 | No | Memory used |
| `memory_limit_bytes` | int64 | bytes | ≥0 | No | Memory limit |
| `network_rx_bytes` | int64 | bytes | ≥0 | No | RX bytes |
| `network_tx_bytes` | int64 | bytes | ≥0 | No | TX bytes |
| `ports` | array | - | - | No | Port mappings |
| `timestamp` | string | - | - | No | Event timestamp |

### VM Info Fields

| Field | Type | Unit | Range | Nullable | Description |
|-------|------|------|-------|----------|-------------|
| `id` | string | - | - | No | VM ID |
| `name` | string | - | - | No | VM name |
| `state` | string | - | enum | No | VM state |
| `vcpus` | int | count | ≥1 | No | Virtual CPUs |
| `memory_allocated_bytes` | int64 | bytes | ≥0 | No | Allocated RAM |
| `memory_used_bytes` | int64 | bytes | ≥0 | No | Used RAM |
| `disk_path` | string | - | - | No | Disk path |
| `disk_size_bytes` | int64 | bytes | ≥0 | No | Disk size |
| `autostart` | bool | - | - | No | Auto-start |
| `persistent` | bool | - | - | No | Persistent |
| `timestamp` | string | - | - | No | Event timestamp |

### UPS Status Fields

| Field | Type | Unit | Range | Nullable | Description |
|-------|------|------|-------|----------|-------------|
| `connected` | bool | - | - | No | UPS connected |
| `model` | string | - | - | No | UPS model |
| `status` | string | - | enum | No | UPS status |
| `battery_charge_percent` | float64 | percent | 0-100 | No | Battery charge |
| `battery_runtime_seconds` | int | seconds | ≥0 | No | Runtime est. |
| `load_percent` | float64 | percent | 0-100 | No | Load percent |
| `input_voltage` | float64 | volts | ≥0 | No | Input voltage |
| `output_voltage` | float64 | volts | ≥0 | No | Output voltage |
| `power_watts` | float64 | watts | ≥0 | No | Power draw |
| `timestamp` | string | - | - | No | Event timestamp |

### GPU Metrics Fields

| Field | Type | Unit | Range | Nullable | Description |
|-------|------|------|-------|----------|-------------|
| `available` | bool | - | - | No | GPU available |
| `name` | string | - | - | No | GPU name |
| `driver_version` | string | - | - | No | Driver version |
| `temperature_celsius` | float64 | °C | 0-∞ | Yes | GPU temp |
| `cpu_temperature_celsius` | float64 | °C | 0-∞ | Yes | CPU temp (iGPU) |
| `utilization_gpu_percent` | float64 | percent | 0-100 | No | GPU usage |
| `utilization_memory_percent` | float64 | percent | 0-100 | No | VRAM usage |
| `memory_total_bytes` | int64 | bytes | ≥0 | No | Total VRAM |
| `memory_used_bytes` | int64 | bytes | ≥0 | No | Used VRAM |
| `power_draw_watts` | float64 | watts | ≥0 | No | Power draw |
| `timestamp` | string | - | - | No | Event timestamp |

### Network Info Fields

| Field | Type | Unit | Range | Nullable | Description |
|-------|------|------|-------|----------|-------------|
| `name` | string | - | - | No | Interface name |
| `mac_address` | string | - | - | No | MAC address |
| `ip_address` | string | - | - | No | IP address |
| `speed_mbps` | int | Mbps | ≥0 | No | Link speed |
| `state` | string | - | enum | No | Interface state |
| `bytes_received` | int64 | bytes | ≥0 | No | RX bytes |
| `bytes_sent` | int64 | bytes | ≥0 | No | TX bytes |
| `packets_received` | int64 | count | ≥0 | No | RX packets |
| `packets_sent` | int64 | count | ≥0 | No | TX packets |
| `errors_received` | int64 | count | ≥0 | No | RX errors |
| `errors_sent` | int64 | count | ≥0 | No | TX errors |
| `timestamp` | string | - | - | No | Event timestamp |

### Share Info Fields

| Field | Type | Unit | Range | Nullable | Description |
|-------|------|------|-------|----------|-------------|
| `name` | string | - | - | No | Share name |
| `path` | string | - | - | No | Share path |
| `size_bytes` | int64 | bytes | ≥0 | No | Total size |
| `used_bytes` | int64 | bytes | ≥0 | No | Used space |
| `free_bytes` | int64 | bytes | ≥0 | No | Free space |
| `usage_percent` | float64 | percent | 0-100 | No | Usage percent |
| `timestamp` | string | - | - | No | Event timestamp |

---

## Validation Rules

### General Rules

1. **Required Fields**: All non-nullable fields must be present
2. **Type Safety**: Fields must match specified types
3. **Range Validation**: Numeric fields must be within specified ranges
4. **Enum Validation**: String enums must match allowed values

### Enum Values

#### Array State
- `STARTED` - Array is running
- `STOPPED` - Array is stopped
- `STARTING` - Array is starting
- `STOPPING` - Array is stopping

#### Parity Check Status
- `idle` - No check running
- `running` - Check in progress
- `paused` - Check paused
- `completed` - Check completed
- `error` - Check failed

#### Disk Status
- `DISK_OK` - Disk healthy
- `DISK_DSBL` - Disk disabled
- `DISK_NP` - Disk not present
- `DISK_INVALID` - Disk invalid

#### SMART Status
- `PASSED` - SMART passed
- `FAILED` - SMART failed
- `UNKNOWN` - Status unknown

#### Container State
- `running` - Container running
- `paused` - Container paused
- `stopped` - Container stopped
- `exited` - Container exited
- `created` - Container created
- `restarting` - Container restarting

#### VM State
- `running` - VM running
- `paused` - VM paused
- `shut off` - VM stopped
- `crashed` - VM crashed
- `pmsuspended` - VM suspended

#### UPS Status
- `ONLINE` - On line power
- `ONBATT` - On battery
- `LOWBATT` - Low battery
- `REPLACEBATT` - Replace battery

#### Network State
- `up` - Interface up
- `down` - Interface down

---

## Type Mappings

### Go to JSON Type Mapping

| Go Type | JSON Type | Notes |
|---------|-----------|-------|
| `string` | string | UTF-8 encoded |
| `int`, `int64` | number | Integer values |
| `float64` | number | Decimal values |
| `bool` | boolean | true/false |
| `time.Time` | string | RFC3339Nano format |
| `[]T` | array | Array of type T |
| `*T` | object/null | Pointer can be null |

### JSON to Python Type Mapping

| JSON Type | Python Type | Notes |
|-----------|-------------|-------|
| string | str | Unicode string |
| number (int) | int | Integer |
| number (float) | float | Float |
| boolean | bool | True/False |
| array | list | List of items |
| object | dict | Dictionary |
| null | None | None value |

### JSON to JavaScript Type Mapping

| JSON Type | JavaScript Type | Notes |
|-----------|-----------------|-------|
| string | string | String |
| number | number | Number (int/float) |
| boolean | boolean | true/false |
| array | Array | Array |
| object | Object | Object |
| null | null | null value |

---

## Implementation Examples

### TypeScript Interface Definitions

```typescript
interface WSEvent {
  event: string;
  timestamp: string;
  data: SystemInfo | ArrayStatus | DiskInfo[] | ContainerInfo[] | VMInfo[] | 
        UPSStatus | GPUMetrics[] | NetworkInfo[] | ShareInfo[];
}

interface SystemInfo {
  hostname: string;
  version: string;
  uptime_seconds: number;
  cpu_usage_percent: number;
  cpu_model: string;
  cpu_cores: number;
  cpu_threads: number;
  cpu_mhz: number;
  cpu_temp_celsius?: number;
  ram_usage_percent: number;
  ram_total_bytes: number;
  ram_used_bytes: number;
  ram_free_bytes: number;
  ram_buffers_bytes: number;
  ram_cached_bytes: number;
  server_model: string;
  bios_version: string;
  bios_date: string;
  motherboard_temp_celsius?: number;
  fans?: FanInfo[];
  timestamp: string;
}

interface FanInfo {
  name: string;
  rpm: number;
}

// ... (additional interfaces for other event types)
```

---

### Python Dataclass Definitions

```python
from dataclasses import dataclass
from typing import Optional, List
from datetime import datetime

@dataclass
class WSEvent:
    event: str
    timestamp: str
    data: dict | list

@dataclass
class SystemInfo:
    hostname: str
    version: str
    uptime_seconds: int
    cpu_usage_percent: float
    cpu_model: str
    cpu_cores: int
    cpu_threads: int
    cpu_mhz: float
    cpu_temp_celsius: Optional[float]
    ram_usage_percent: float
    ram_total_bytes: int
    ram_used_bytes: int
    ram_free_bytes: int
    ram_buffers_bytes: int
    ram_cached_bytes: int
    server_model: str
    bios_version: str
    bios_date: str
    motherboard_temp_celsius: Optional[float]
    fans: Optional[List['FanInfo']]
    timestamp: str

@dataclass
class FanInfo:
    name: str
    rpm: int

# ... (additional dataclasses for other event types)
```

### Go Struct Validation

```go
// Example validation function
func ValidateSystemInfo(info *dto.SystemInfo) error {
    if info.Hostname == "" {
        return errors.New("hostname is required")
    }
    if info.CPUUsagePercent < 0 || info.CPUUsagePercent > 100 {
        return errors.New("cpu_usage_percent must be between 0 and 100")
    }
    if info.CPUCores < 1 {
        return errors.New("cpu_cores must be at least 1")
    }
    if info.RAMUsagePercent < 0 || info.RAMUsagePercent > 100 {
        return errors.New("ram_usage_percent must be between 0 and 100")
    }
    return nil
}
```

---

## Best Practices

### Client Implementation

1. **Always Check Data Type**: Verify if `data` is an object or array before processing
2. **Handle Missing Fields**: Use optional/nullable types for fields that may be absent
3. **Validate Enums**: Check enum values against allowed list before using
4. **Parse Timestamps**: Convert ISO 8601 strings to native datetime objects
5. **Handle Unknown Events**: Gracefully handle events that don't match any known type

### Error Handling

```python
def parse_event(raw_event: dict) -> Optional[WSEvent]:
    """Parse raw WebSocket event with error handling."""
    try:
        # Validate required fields
        if 'event' not in raw_event or 'timestamp' not in raw_event or 'data' not in raw_event:
            logger.error("Missing required fields in event")
            return None

        # Validate event type
        if raw_event['event'] != 'update':
            logger.warning(f"Unknown event type: {raw_event['event']}")

        # Parse timestamp
        try:
            timestamp = datetime.fromisoformat(raw_event['timestamp'].replace('Z', '+00:00'))
        except ValueError as e:
            logger.error(f"Invalid timestamp format: {e}")
            return None

        # Identify event type
        event_type = identify_event_type(raw_event['data'])
        if event_type == 'unknown':
            logger.warning("Could not identify event type")

        return WSEvent(
            event=raw_event['event'],
            timestamp=raw_event['timestamp'],
            data=raw_event['data']
        )

    except Exception as e:
        logger.error(f"Failed to parse event: {e}")
        return None
```

### Performance Optimization

1. **Cache Event Type**: Store identified event type to avoid re-identification
2. **Use Efficient Parsing**: Use streaming JSON parsers for large payloads
3. **Batch Processing**: Process multiple events in batches when possible
4. **Selective Parsing**: Only parse fields you need, not entire structure

---

## Common Pitfalls

### 1. Assuming Type Field Exists

❌ **Wrong**:
```python
event_type = data['type']  # This field doesn't exist!
```

✅ **Correct**:
```python
event_type = identify_event_type(data['data'])
```

### 2. Not Checking Array vs Object

❌ **Wrong**:
```python
hostname = data['data']['hostname']  # Fails if data is array!
```

✅ **Correct**:
```python
if isinstance(data['data'], dict):
    hostname = data['data'].get('hostname')
elif isinstance(data['data'], list) and data['data']:
    hostname = data['data'][0].get('hostname')
```

### 3. Ignoring Nullable Fields

❌ **Wrong**:
```python
temp = data['cpu_temp_celsius']  # May be null!
if temp > 50:
    alert()
```

✅ **Correct**:
```python
temp = data.get('cpu_temp_celsius')
if temp is not None and temp > 50:
    alert()
```

### 4. Not Validating Enums

❌ **Wrong**:
```python
if state == 'RUNNING':  # Wrong case!
    process()
```

✅ **Correct**:
```python
VALID_STATES = ['STARTED', 'STOPPED', 'STARTING', 'STOPPING']
if state in VALID_STATES:
    process()
```

### 5. Hardcoding Event Frequencies

❌ **Wrong**:
```python
# Assuming events arrive exactly every 5 seconds
time.sleep(5)
expect_event()
```

✅ **Correct**:
```python
# Events may arrive slightly off schedule
# Use event-driven processing, not timing assumptions
async for event in websocket:
    process_event(event)
```

---

## Versioning and Compatibility

### Current Version: 1.1.0

**Breaking Changes from 1.0.0**:
- None (fully backward compatible)

**New Fields in 1.1.0**:
- `motherboard_temp_celsius` in SystemInfo
- `fans[]` array in SystemInfo
- Additional disk metrics (power_on_hours, power_cycle_count, etc.)

### Forward Compatibility

Clients should be designed to handle:
1. **New Fields**: Ignore unknown fields gracefully
2. **New Event Types**: Handle unknown events without crashing
3. **Field Type Changes**: Validate types before using
4. **Deprecated Fields**: Continue supporting old fields during transition

### Backward Compatibility

The server maintains backward compatibility by:
1. Never removing required fields
2. Adding new fields as optional
3. Maintaining existing field types
4. Preserving enum values

---

## Testing and Validation

### Unit Test Example

```python
import unittest
from datetime import datetime

class TestEventParsing(unittest.TestCase):

    def test_system_update_identification(self):
        """Test system update event identification."""
        data = {
            "hostname": "Tower",
            "cpu_usage_percent": 12.5,
            "ram_usage_percent": 45.2
        }
        event_type = identify_event_type(data)
        self.assertEqual(event_type, "system_update")

    def test_array_event_identification(self):
        """Test array event identification."""
        data = [{
            "device": "sda",
            "mount_point": "/mnt/disk1",
            "size_bytes": 4000000000000
        }]
        event_type = identify_event_type(data)
        self.assertEqual(event_type, "disk_list_update")

    def test_empty_array_handling(self):
        """Test empty array handling."""
        data = []
        event_type = identify_event_type(data)
        self.assertEqual(event_type, "empty_list")

    def test_unknown_event_handling(self):
        """Test unknown event handling."""
        data = {"unknown_field": "value"}
        event_type = identify_event_type(data)
        self.assertEqual(event_type, "unknown")
```

### Integration Test Example

```python
async def test_websocket_event_flow():
    """Test complete WebSocket event flow."""
    async with aiohttp.ClientSession() as session:
        async with session.ws_connect('ws://localhost:8043/api/v1/ws') as ws:
            # Wait for first event
            msg = await ws.receive()
            assert msg.type == aiohttp.WSMsgType.TEXT

            # Parse event
            event = json.loads(msg.data)
            assert 'event' in event
            assert 'timestamp' in event
            assert 'data' in event
            assert event['event'] == 'update'

            # Identify event type
            event_type = identify_event_type(event['data'])
            assert event_type != 'unknown'

            print(f"✅ Received valid {event_type} event")
```

---

## Reference Implementation

Complete reference implementations are available in:

1. **Python**: `ha-unraid-management-agent/custom_components/unraid_management_agent/websocket_client.py`
2. **Go (Server)**: `daemon/services/api/websocket.go`
3. **Test Script**: `test_websocket.py`
4. **Multi-Client Test**: `test_multiple_connections.py`

---

## Status

✅ **Event envelope structure documented**
✅ **Data type definitions provided**
✅ **Event identification logic specified**
✅ **Field specifications detailed**
✅ **Validation rules defined**
✅ **Type mappings documented**
✅ **Implementation examples provided**
✅ **Best practices documented**
✅ **Common pitfalls identified**
✅ **Versioning strategy defined**
✅ **Testing examples provided**

**Version**: 1.1.0
**Last Updated**: 2025-10-02

