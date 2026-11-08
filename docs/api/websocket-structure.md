# WebSocket Event Structure Documentation

## Overview

Use this reference to read the WebSocket envelope and payload fields.

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

The agent wraps each WebSocket event in a `WSEvent` envelope:

```go
// daemon/dto/websocket.go
type WSEvent struct {
    Event     string      `json:"event"`
    Timestamp time.Time   `json:"timestamp"`
    Data      interface{} `json:"data"`
}
```

### Envelope Fields

| Field       | Type              | Required | Description                             |
| ----------- | ----------------- | -------- | --------------------------------------- |
| `event`     | string            | Yes      | Topic name or `update` fallback         |
| `timestamp` | string (ISO 8601) | Yes      | Server timestamp when event was created |
| `data`      | object/array/null | Yes      | Event payload                           |

### Example Envelope

```json
{
  "event": "system_update",
  "timestamp": "2025-10-02T14:02:59.850035377+10:00",
  "data": {
    "hostname": "Tower",
    "cpu_usage_percent": 12.5,
    ...
  }
}
```

### Important Notes

1. The envelope has no `type` field.
2. Read `event` first. If it is `update`, read `data.event` when present.
3. Read `timestamp` as RFC3339Nano.
4. Check the event definition for the shape of `data`.
5. **Subscriptions drive collection**: In Dynamic collection mode a client's topic filter
   (`{"subscribe": ["container_list_update", ...]}`) decides which collectors run at their active
   rate. A client that never subscribes receives every topic, so it keeps every collector active;
   subscribe to the topics you use to let the others idle. See the
   [Configuration Guide](../guides/configuration.md#collection-mode).

---

## Data Type Definitions

### Primitive Types

| Type      | Go Type        | JSON Type | Description                 |
| --------- | -------------- | --------- | --------------------------- |
| String    | `string`       | string    | UTF-8 encoded text          |
| Integer   | `int`, `int64` | number    | Whole numbers               |
| Float     | `float64`      | number    | Decimal numbers             |
| Boolean   | `bool`         | boolean   | true/false values           |
| Timestamp | `time.Time`    | string    | ISO 8601 formatted datetime |

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

Read the envelope's `event` field first. Use the payload only for the generic `update` fallback:

```
1. Use the event field if it names a known topic.
2. For update, check for a known event name in the payload.
3. Skip unknown event names.
```

### Identification Rules

For example, `system_update` identifies a system snapshot. `share_list_update` identifies a share snapshot,
even when its payload is null. The collector state event has `event: "update"` and
`data.event: "collector_state_change"`. See [producer fixtures](../../contracts/README.md) for wire examples.

### Reference Implementation

```python
def identify_event_type(envelope):
    """Read the topic or the subtype of a generic event."""
    topic = envelope.get("event")
    payload = envelope.get("data")
    if topic == "update" and isinstance(payload, dict):
        topic = payload.get("event")
    if isinstance(topic, str) and topic and topic != "update":
        return topic
    return "unknown"
```

---

## Field Specifications

### System Update Fields

| Field                      | Type    | Unit    | Range  | Nullable | Description     |
| -------------------------- | ------- | ------- | ------ | -------- | --------------- |
| `hostname`                 | string  | -       | -      | No       | Server hostname |
| `version`                  | string  | -       | -      | No       | Agent version   |
| `uptime_seconds`           | int64   | seconds | ≥0     | No       | System uptime   |
| `cpu_usage_percent`        | float64 | percent | 0-100  | No       | CPU usage       |
| `cpu_model`                | string  | -       | -      | No       | CPU model name  |
| `cpu_cores`                | int     | count   | ≥1     | No       | Physical cores  |
| `cpu_threads`              | int     | count   | ≥1     | No       | Logical threads |
| `cpu_mhz`                  | float64 | MHz     | ≥0     | No       | CPU frequency   |
| `cpu_temp_celsius`         | float64 | °C      | -273-∞ | Yes      | CPU temperature |
| `ram_usage_percent`        | float64 | percent | 0-100  | No       | RAM usage       |
| `ram_total_bytes`          | int64   | bytes   | ≥0     | No       | Total RAM       |
| `ram_used_bytes`           | int64   | bytes   | ≥0     | No       | Used RAM        |
| `ram_free_bytes`           | int64   | bytes   | ≥0     | No       | Free RAM        |
| `ram_buffers_bytes`        | int64   | bytes   | ≥0     | No       | Buffer RAM      |
| `ram_cached_bytes`         | int64   | bytes   | ≥0     | No       | Cached RAM      |
| `server_model`             | string  | -       | -      | No       | Server model    |
| `bios_version`             | string  | -       | -      | No       | BIOS version    |
| `bios_date`                | string  | -       | -      | No       | BIOS date       |
| `motherboard_temp_celsius` | float64 | °C      | -273-∞ | Yes      | MB temperature  |
| `fans`                     | array   | -       | -      | Yes      | Fan sensors     |
| `timestamp`                | string  | -       | -      | No       | Event timestamp |

### Array Status Fields

| Field                   | Type    | Unit    | Range | Nullable | Description     |
| ----------------------- | ------- | ------- | ----- | -------- | --------------- |
| `state`                 | string  | -       | enum  | No       | Array state     |
| `num_disks`             | int     | count   | ≥0    | No       | Total disks     |
| `num_data_disks`        | int     | count   | ≥0    | No       | Data disks      |
| `num_parity_disks`      | int     | count   | 0-2   | No       | Parity disks    |
| `sync_percent`          | float64 | percent | 0-100 | No       | Sync progress   |
| `parity_valid`          | bool    | -       | -     | No       | Parity valid    |
| `parity_check_status`   | string  | -       | enum  | No       | Check status    |
| `parity_check_running`  | bool    | -       | -     | No       | Check active    |
| `parity_check_progress` | float64 | percent | 0-100 | No       | Check progress  |
| `timestamp`             | string  | -       | -     | No       | Event timestamp |

### Disk Info Fields

| Field                    | Type    | Unit    | Range | Nullable | Description     |
| ------------------------ | ------- | ------- | ----- | -------- | --------------- |
| `id`                     | string  | -       | -     | No       | Disk identifier |
| `device`                 | string  | -       | -     | No       | Device name     |
| `name`                   | string  | -       | -     | No       | Disk name       |
| `status`                 | string  | -       | enum  | No       | Disk status     |
| `size_bytes`             | int64   | bytes   | ≥0    | No       | Total size      |
| `used_bytes`             | int64   | bytes   | ≥0    | No       | Used space      |
| `free_bytes`             | int64   | bytes   | ≥0    | No       | Free space      |
| `temperature_celsius`    | float64 | °C      | 0-∞   | Yes      | Temperature     |
| `smart_status`           | string  | -       | enum  | No       | SMART status    |
| `smart_errors`           | int     | count   | ≥0    | No       | Error count     |
| `smart_exit_status`      | int     | bitmask | 0-255 | Yes      | smartctl exit   |
| `spindown_delay`         | int     | minutes | ≥0    | No       | Spindown delay  |
| `filesystem`             | string  | -       | -     | No       | FS type         |
| `mount_point`            | string  | -       | -     | No       | Mount path      |
| `usage_percent`          | float64 | percent | 0-100 | No       | Usage percent   |
| `power_on_hours`         | int64   | hours   | ≥0    | No       | Power-on time   |
| `power_cycle_count`      | int     | count   | ≥0    | No       | Power cycles    |
| `read_bytes`             | int64   | bytes   | ≥0    | No       | Bytes read      |
| `write_bytes`            | int64   | bytes   | ≥0    | No       | Bytes written   |
| `read_ops`               | int64   | count   | ≥0    | No       | Read ops        |
| `write_ops`              | int64   | count   | ≥0    | No       | Write ops       |
| `io_utilization_percent` | float64 | percent | 0-100 | No       | I/O usage       |
| `timestamp`              | string  | -       | -     | No       | Event timestamp |

### Container Info Fields

| Field                | Type    | Unit    | Range | Nullable | Description     |
| -------------------- | ------- | ------- | ----- | -------- | --------------- |
| `id`                 | string  | -       | -     | No       | Container ID    |
| `name`               | string  | -       | -     | No       | Container name  |
| `image`              | string  | -       | -     | No       | Image name      |
| `state`              | string  | -       | enum  | No       | Container state |
| `status`             | string  | -       | -     | No       | Status text     |
| `cpu_percent`        | float64 | percent | 0-∞   | No       | CPU usage       |
| `memory_usage_bytes` | int64   | bytes   | ≥0    | No       | Memory used     |
| `memory_limit_bytes` | int64   | bytes   | ≥0    | No       | Memory limit    |
| `network_rx_bytes`   | int64   | bytes   | ≥0    | No       | RX bytes        |
| `network_tx_bytes`   | int64   | bytes   | ≥0    | No       | TX bytes        |
| `ports`              | array   | -       | -     | No       | Port mappings   |
| `timestamp`          | string  | -       | -     | No       | Event timestamp |

### VM Info Fields

| Field                    | Type   | Unit  | Range | Nullable | Description                                             |
| ------------------------ | ------ | ----- | ----- | -------- | ------------------------------------------------------- |
| `id`                     | string | -     | -     | No       | VM ID                                                   |
| `name`                   | string | -     | -     | No       | VM name                                                 |
| `state`                  | string | -     | enum  | No       | VM state                                                |
| `cpu_count`              | int    | count | ≥1    | No       | Virtual CPUs                                            |
| `memory_allocated_bytes` | uint64 | bytes | ≥0    | No       | Maximum configured RAM                                  |
| `memory_used_bytes`      | uint64 | bytes | ≥0    | No       | Guest usage, else QEMU RSS, else 0; 0 when not running  |
| `disk_path`              | string | -     | -     | No       | Primary disk source; empty when unavailable             |
| `disk_size_bytes`        | uint64 | bytes | ≥0    | No       | Primary disk guest-visible capacity; 0 when unavailable |
| `autostart`              | bool   | -     | -     | No       | Auto-start                                              |
| `persistent`             | bool   | -     | -     | No       | Persistent                                              |
| `timestamp`              | string | -     | -     | No       | Event timestamp                                         |

`memory_allocated_bytes` is the maximum configured memory. `memory_used_bytes` is 0 for non-running VMs.
For running VMs it uses guest `available - unused` from libvirt balloon stats when both values are present
and valid, otherwise host QEMU RSS (including QEMU overhead, which can exceed allocation), otherwise 0.
Guest stats require a balloon driver providing stats; the collector does not configure the balloon stats period.
Allocated memory is never used as usage. A running VM can report 0 when stats are unavailable or usage is zero.

The primary disk is the first `device="disk"` entry with a source in domain XML order; CD-ROMs and floppies
are skipped. `disk_path` is its source path or identifier (storage volumes are resolved to paths), or empty
when unavailable. `disk_size_bytes` is its guest-visible virtual capacity in bytes, collected best effort
for all VM states; 0 means unavailable. It is not host allocation or physical space used.

### UPS Status Fields

| Field                                | Type    | Unit    | Range | Nullable | Description                                 |
| ------------------------------------ | ------- | ------- | ----- | -------- | ------------------------------------------- |
| `connected`                          | bool    | -       | -     | No       | UPS connected                               |
| `device_name`                        | string  | -       | -     | No       | NUT device (omitted for apcupsd)            |
| `status`                             | string  | -       | enum  | No       | UPS status                                  |
| `load_percent`                       | float64 | percent | 0-100 | Yes      | Load percent                                |
| `battery_charge_percent`             | float64 | percent | 0-100 | Yes      | Battery charge                              |
| `runtime_left_seconds`               | int     | seconds | ≥0    | Yes      | Runtime est.                                |
| `power_watts`                        | float64 | watts   | ≥0    | Yes      | Power draw (reported or derived)            |
| `nominal_power_watts`                | float64 | watts   | ≥0    | Yes      | Nominal power                               |
| `input_voltage`                      | float64 | volts   | ≥0    | Yes      | Input (line) voltage                        |
| `output_voltage`                     | float64 | volts   | ≥0    | Yes      | Output voltage                              |
| `battery_voltage`                    | float64 | volts   | ≥0    | Yes      | Battery voltage                             |
| `line_frequency`                     | float64 | hertz   | ≥0    | Yes      | Input line frequency                        |
| `temperature_celsius`                | float64 | °C      | -     | Yes      | Internal UPS temperature                    |
| `time_on_battery_seconds`            | int     | seconds | ≥0    | Yes      | On battery, current outage                  |
| `cumulative_time_on_battery_seconds` | int     | seconds | ≥0    | Yes      | Total on battery (apcupsd only)             |
| `transfer_count`                     | int     | count   | ≥0    | Yes      | Transfers to battery                        |
| `self_test`                          | string  | -       | -     | No       | Raw self-test result (omitted when unknown) |
| `serial_number`                      | string  | -       | -     | No       | Serial number (omitted when unknown)        |
| `model`                              | string  | -       | -     | No       | UPS model                                   |
| `timestamp`                          | string  | -       | -     | No       | Event timestamp                             |

Nullable fields are `null` when the UPS does not report the value.

### GPU Metrics Fields

| Field                        | Type    | Unit    | Range | Nullable | Description     |
| ---------------------------- | ------- | ------- | ----- | -------- | --------------- |
| `available`                  | bool    | -       | -     | No       | GPU available   |
| `name`                       | string  | -       | -     | No       | GPU name        |
| `driver_version`             | string  | -       | -     | No       | Driver version  |
| `temperature_celsius`        | float64 | °C      | 0-∞   | Yes      | GPU temp        |
| `cpu_temperature_celsius`    | float64 | °C      | 0-∞   | Yes      | CPU temp (iGPU) |
| `utilization_gpu_percent`    | float64 | percent | 0-100 | No       | GPU usage       |
| `utilization_memory_percent` | float64 | percent | 0-100 | No       | VRAM usage      |
| `memory_total_bytes`         | int64   | bytes   | ≥0    | No       | Total VRAM      |
| `memory_used_bytes`          | int64   | bytes   | ≥0    | No       | Used VRAM       |
| `power_draw_watts`           | float64 | watts   | ≥0    | No       | Power draw      |
| `timestamp`                  | string  | -       | -     | No       | Event timestamp |

### Network Info Fields

| Field              | Type   | Unit  | Range | Nullable | Description     |
| ------------------ | ------ | ----- | ----- | -------- | --------------- |
| `name`             | string | -     | -     | No       | Interface name  |
| `mac_address`      | string | -     | -     | No       | MAC address     |
| `ip_address`       | string | -     | -     | No       | IP address      |
| `speed_mbps`       | int    | Mbps  | ≥0    | No       | Link speed      |
| `state`            | string | -     | enum  | No       | Interface state |
| `bytes_received`   | int64  | bytes | ≥0    | No       | RX bytes        |
| `bytes_sent`       | int64  | bytes | ≥0    | No       | TX bytes        |
| `packets_received` | int64  | count | ≥0    | No       | RX packets      |
| `packets_sent`     | int64  | count | ≥0    | No       | TX packets      |
| `errors_received`  | int64  | count | ≥0    | No       | RX errors       |
| `errors_sent`      | int64  | count | ≥0    | No       | TX errors       |
| `timestamp`        | string | -     | -     | No       | Event timestamp |

### Share Info Fields

| Field                   | Type    | Unit    | Range | Nullable | Description                                                                     |
| ----------------------- | ------- | ------- | ----- | -------- | ------------------------------------------------------------------------------- |
| `name`                  | string  | -       | -     | No       | Share name                                                                      |
| `path`                  | string  | -       | -     | No       | Share path                                                                      |
| `used_bytes`            | uint64  | bytes   | ≥0    | Omitted  | Measured share content, including mounted descendants visible beneath the share |
| `storage_used_bytes`    | uint64  | bytes   | ≥0    | No       | Backing-storage used space                                                      |
| `storage_free_bytes`    | uint64  | bytes   | ≥0    | No       | Backing-storage free space                                                      |
| `storage_total_bytes`   | uint64  | bytes   | ≥0    | No       | Backing-storage capacity                                                        |
| `storage_usage_percent` | float64 | percent | 0-100 | No       | Backing-storage usage                                                           |
| `free_bytes`            | uint64  | bytes   | ≥0    | No       | Deprecated alias of `storage_free_bytes`                                        |
| `total_bytes`           | uint64  | bytes   | ≥0    | No       | Deprecated alias of `storage_total_bytes`                                       |
| `usage_percent`         | float64 | percent | 0-100 | No       | Deprecated alias of `storage_usage_percent`                                     |
| `timestamp`             | string  | -       | -     | No       | Event timestamp                                                                 |

`used_bytes` is omitted when unknown, and `0` for a measured empty share. Measurement requires a cache-only
share on its assigned mounted ZFS pool with all members known spinning. Descendants count only when mounted
at or beneath the share dataset's mountpoint; unknown mount metadata leaves usage unknown.
Storage figures are shared by all shares on the same backing storage.

---

## Validation Rules

### General Rules

1. **Required Fields**: All non-nullable fields must be present, except fields
   documented as omitted (for example `device_name`, `self_test` and
   `serial_number`): these may be absent, but are never `null` when present
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

| Go Type        | JSON Type   | Notes               |
| -------------- | ----------- | ------------------- |
| `string`       | string      | UTF-8 encoded       |
| `int`, `int64` | number      | Integer values      |
| `float64`      | number      | Decimal values      |
| `bool`         | boolean     | true/false          |
| `time.Time`    | string      | RFC3339Nano format  |
| `[]T`          | array       | Array of type T     |
| `*T`           | object/null | Pointer can be null |

### JSON to Python Type Mapping

| JSON Type      | Python Type | Notes          |
| -------------- | ----------- | -------------- |
| string         | str         | Unicode string |
| number (int)   | int         | Integer        |
| number (float) | float       | Float          |
| boolean        | bool        | True/False     |
| array          | list        | List of items  |
| object         | dict        | Dictionary     |
| null           | None        | None value     |

### JSON to JavaScript Type Mapping

| JSON Type | JavaScript Type | Notes              |
| --------- | --------------- | ------------------ |
| string    | string          | String             |
| number    | number          | Number (int/float) |
| boolean   | boolean         | true/false         |
| array     | Array           | Array              |
| object    | Object          | Object             |
| null      | null            | null value         |

---

## Implementation Examples

### TypeScript Interface Definitions

```typescript
interface WSEvent {
  event: string;
  timestamp: string;
  data:
    | SystemInfo
    | ArrayStatus
    | DiskInfo[]
    | ContainerInfo[]
    | VMInfo[]
    | UPSStatus
    | GPUMetrics[]
    | NetworkInfo[]
    | ShareInfo[];
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

        # Parse timestamp
        try:
            timestamp = datetime.fromisoformat(raw_event['timestamp'].replace('Z', '+00:00'))
        except ValueError as e:
            logger.error(f"Invalid timestamp format: {e}")
            return None

        # Keep the wire envelope intact. Resolve its type when dispatching.
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
event_type = identify_event_type(data)
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

class TestEventParsing(unittest.TestCase):
    def test_system_topic(self):
        event = {"event": "system_update", "data": {"hostname": "Tower"}}
        self.assertEqual(identify_event_type(event), "system_update")

    def test_null_share_payload(self):
        event = {"event": "share_list_update", "data": None}
        self.assertEqual(identify_event_type(event), "share_list_update")

    def test_collector_state_subtype(self):
        event = {"event": "update", "data": {"event": "collector_state_change"}}
        self.assertEqual(identify_event_type(event), "collector_state_change")

    def test_missing_topic(self):
        self.assertEqual(identify_event_type({"data": {}}), "unknown")

```

### Integration Test Example

The server sends no initial snapshot. Set `event_timeout` in seconds.
Use a value greater than the [system collector interval](../guides/configuration.md#collection-intervals) on the test server.

```python
async def test_websocket_event_flow(event_timeout=30):
    """Test receipt of a WebSocket event."""
    async with aiohttp.ClientSession() as session:
        async with session.ws_connect('ws://localhost:8043/api/v1/ws') as ws:
            # Wait for first event
            msg = await ws.receive(timeout=event_timeout)
            assert msg.type == aiohttp.WSMsgType.TEXT

            # Parse event
            event = json.loads(msg.data)
            assert 'event' in event
            assert 'timestamp' in event
            assert 'data' in event

            # Identify event type
            event_type = identify_event_type(event)
            assert event_type != 'unknown'

            print(f"✅ Received valid {event_type} event")
```

---

## Reference Implementation

See the [WebSocket server](../../daemon/services/api/websocket.go) and [producer fixtures](../../contracts/README.md).
For the Home Assistant client, see the [integration repository](https://github.com/tinkerologie/ha-unraid-management-agent).

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
