# Prometheus Metrics

Collect agent metrics with Prometheus. Use Grafana to show them.

## Overview

The agent serves Prometheus metrics at `/metrics`. The available series depend on the collectors and their data.

**Endpoint**: `http://<unraid-ip>:8043/metrics`  
**Format**: Prometheus text format  
**Update Frequency**: Each scrape returns the current cached collector values

Set `metrics_enabled: false` to disable this endpoint. The agent then returns `404 Not Found`. See
[Endpoint toggles](../guides/configuration.md#endpoint-toggles-rest-only-mode).

## Scraping Configuration

### Prometheus Config

```yaml
scrape_configs:
  - job_name: "unraid"
    static_configs:
      - targets: ["unraid-server:8043"]
    scrape_interval: 15s
    scrape_timeout: 10s
    metrics_path: "/metrics"
```

If `API_TOKEN` is set, the scrape must send `Authorization: Bearer <token>`.

### Docker Compose (Prometheus)

```yaml
version: "3"
services:
  prometheus:
    image: prom/prometheus:latest
    volumes:
      - ./prometheus.yml:/etc/prometheus/prometheus.yml
      - prometheus-data:/prometheus
    ports:
      - "9090:9090"
    command:
      - "--config.file=/etc/prometheus/prometheus.yml"
volumes:
  prometheus-data:
```

## Available Metrics

### System Metrics

| Metric                           | Type  | Labels                           | Description                                                             |
| -------------------------------- | ----- | -------------------------------- | ----------------------------------------------------------------------- |
| `unraid_system_info`             | Gauge | hostname, version, agent_version | Unraid system information (always 1)                                    |
| `unraid_cpu_usage_percent`       | Gauge | -                                | CPU usage percentage (0-100)                                            |
| `unraid_cpu_temperature_celsius` | Gauge | -                                | CPU temperature in Celsius                                              |
| `unraid_memory_used_bytes`       | Gauge | -                                | Memory used in bytes                                                    |
| `unraid_memory_total_bytes`      | Gauge | -                                | Total memory in bytes                                                   |
| `unraid_memory_usage_percent`    | Gauge | -                                | Memory usage percentage (0-100)                                         |
| `unraid_system_uptime_seconds`   | Gauge | -                                | System uptime in seconds                                                |
| `unraid_cpu_power_watts`         | Gauge | -                                | CPU package power consumption in watts (Intel RAPL, 0 when unavailable) |
| `unraid_dram_power_watts`        | Gauge | -                                | DRAM power consumption in watts (Intel RAPL, 0 when unavailable)        |

Only `unraid_system_info` has the `hostname` label. The other families in this document do not have it.

### Array Metrics

| Metric                                           | Type  | Labels | Description                                                                     |
| ------------------------------------------------ | ----- | ------ | ------------------------------------------------------------------------------- |
| `unraid_array_state`                             | Gauge | -      | Array state (1=started, 0 for all other states)                                 |
| `unraid_array_total_bytes`                       | Gauge | -      | Total array capacity in bytes                                                   |
| `unraid_array_used_bytes`                        | Gauge | -      | Used array space in bytes                                                       |
| `unraid_array_free_bytes`                        | Gauge | -      | Free space in bytes                                                             |
| `unraid_array_usage_percent`                     | Gauge | -      | Array usage percentage (0-100)                                                  |
| `unraid_array_capacity_sample_timestamp_seconds` | Gauge | -      | Unix time of the last array capacity measurement; 0 when no sample is available |
| `unraid_parity_valid`                            | Gauge | -      | Parity valid (0=invalid, 1=valid)                                               |
| `unraid_parity_check_running`                    | Gauge | -      | 1 for any non-empty parity check status except `IDLE` or `idle`, else 0         |
| `unraid_parity_check_progress`                   | Gauge | -      | Parity check progress (0-100)                                                   |

### Disk Metrics

| Metric                            | Type  | Labels               | Description                                                   |
| --------------------------------- | ----- | -------------------- | ------------------------------------------------------------- |
| `unraid_disk_size_bytes`          | Gauge | disk, device         | Disk size in bytes                                            |
| `unraid_disk_used_bytes`          | Gauge | disk, device         | Used space in bytes                                           |
| `unraid_disk_free_bytes`          | Gauge | disk, device         | Free space in bytes                                           |
| `unraid_disk_temperature_celsius` | Gauge | disk, device, type   | Disk temperature in Celsius                                   |
| `unraid_disk_status`              | Gauge | disk, device, status | Disk status: 1 for `DISK_OK`, `OK` or an empty status, else 0 |
| `unraid_disk_standby`             | Gauge | disk, device         | Disk standby state (0=active, 1=standby)                      |
| `unraid_disk_smart_status`        | Gauge | disk, device         | SMART status: 0 only for `FAILED`, else 1                     |

The `disk` label is the disk name, for example `disk1` or `parity`. The `type` label is `Flash` for boot flash,
`SSD` for cache and pool disks, and `HDD` for all other disks.

The agent omits a disk temperature series if the reading is unavailable. This can occur when the disk is in standby.
The agent removes an existing series if its reading becomes unavailable. A measured 0 °C remains a sample.

### Container Metrics (Docker)

| Metric                             | Type  | Labels          | Description                                         |
| ---------------------------------- | ----- | --------------- | --------------------------------------------------- |
| `unraid_docker_container_state`    | Gauge | name, id, image | Container state (1=running, 0 for all other states) |
| `unraid_docker_containers_total`   | Gauge | -               | Total number of containers                          |
| `unraid_docker_containers_running` | Gauge | -               | Number of running containers                        |

The agent does not export stopped or paused container counts. Use `unraid_docker_container_state` for each container.

### VM Metrics

| Metric               | Type  | Labels     | Description                                  |
| -------------------- | ----- | ---------- | -------------------------------------------- |
| `unraid_vm_state`    | Gauge | name, uuid | VM state (1=running, 0 for all other states) |
| `unraid_vms_total`   | Gauge | -          | Total number of VMs                          |
| `unraid_vms_running` | Gauge | -          | Number of running VMs                        |

The agent does not export a stopped VM count. Use `unraid_vm_state` for each VM.

### GPU Metrics

| Metric                                  | Type  | Labels    | Description                              |
| --------------------------------------- | ----- | --------- | ---------------------------------------- |
| `unraid_gpu_utilization_percent`        | Gauge | gpu, name | GPU utilization (0-100)                  |
| `unraid_gpu_temperature_celsius`        | Gauge | gpu, name | GPU temperature in Celsius               |
| `unraid_gpu_memory_temperature_celsius` | Gauge | gpu, name | GPU memory (VRAM) temperature in Celsius |
| `unraid_gpu_memory_used_bytes`          | Gauge | gpu, name | GPU memory used in bytes                 |
| `unraid_gpu_memory_total_bytes`         | Gauge | gpu, name | Total GPU memory in bytes                |
| `unraid_gpu_power_watts`                | Gauge | gpu, name | GPU power draw in watts                  |

The `gpu` label is the position of the GPU in the GPU list, starting at `0`. The `name` label is the GPU name.

The agent omits GPU temperature, memory temperature, used memory, total memory, and power series if the readings
are unavailable.
It removes existing series if their readings become unavailable. Measured zero values remain samples.

### UPS Metrics

| Metric                              | Type  | Labels      | Description                                             |
| ----------------------------------- | ----- | ----------- | ------------------------------------------------------- |
| `unraid_ups_battery_charge_percent` | Gauge | name        | Battery charge (0-100)                                  |
| `unraid_ups_load_percent`           | Gauge | name        | UPS load (0-100)                                        |
| `unraid_ups_runtime_seconds`        | Gauge | name        | Estimated runtime in seconds                            |
| `unraid_ups_status`                 | Gauge | name, model | UPS status (1=`ONLINE` or `OL`, 0 for all other states) |

The `name` label is always `ups`.

The battery, load and runtime gauges are not exported when the UPS does not
report that value, rather than being exported as 0.

### Share Metrics

| Metric                             | Type  | Labels | Description                                           |
| ---------------------------------- | ----- | ------ | ----------------------------------------------------- |
| `unraid_shares_total`              | Gauge | -      | Total number of shares                                |
| `unraid_share_used_bytes`          | Gauge | name   | Measured share content in bytes; omitted when unknown |
| `unraid_share_storage_used_bytes`  | Gauge | name   | Backing-storage used space in bytes                   |
| `unraid_share_storage_free_bytes`  | Gauge | name   | Backing-storage free space in bytes                   |
| `unraid_share_storage_total_bytes` | Gauge | name   | Backing-storage capacity in bytes                     |

Measured usage is available only for cache-only shares on an assigned mounted ZFS pool whose members
are all known spinning. A measured empty share emits zero. Storage figures are shared by all shares
on the same backing storage, so summing these per-share gauges can double-count capacity.

### Service Metrics

| Metric                   | Type  | Labels  | Description                             |
| ------------------------ | ----- | ------- | --------------------------------------- |
| `unraid_service_enabled` | Gauge | service | Service enabled (0=disabled, 1=enabled) |
| `unraid_service_running` | Gauge | service | Service running (0=stopped, 1=running)  |

The `service` label is `smb`, `nfs`, `afp`, `ftp`, `ssh`, `telnet`, `avahi`, `netbios`, `wsd`, `wireguard`,
`upnp`, `ntp` or `syslog`.

### Fan Metrics

| Metric                   | Type  | Labels       | Description                           |
| ------------------------ | ----- | ------------ | ------------------------------------- |
| `unraid_fan_rpm`         | Gauge | fan_id, name | Fan speed in revolutions per minute   |
| `unraid_fan_pwm_percent` | Gauge | fan_id, name | Fan PWM duty cycle percentage (0-100) |

The `fan_id` label is the fan ID from the fan control status, for example `hwmon0_fan1`.
The `name` label is the fan name.

### Subsystem Health Metrics

| Metric                            | Type  | Labels    | Description                                                                         |
| --------------------------------- | ----- | --------- | ----------------------------------------------------------------------------------- |
| `unraid_subsystem_status`         | Gauge | subsystem | Data-source health per subsystem (0=healthy or disabled, 1=degraded, 2=unavailable) |
| `unraid_degraded_subsystem_count` | Gauge | -         | Number of subsystems whose data source is degraded or unavailable                   |

A subsystem that is disabled on purpose, for example Docker or the VM manager, has the value 0.
It does not increase `unraid_degraded_subsystem_count`.

### MCP Metrics

| Metric                        | Type    | Labels | Description                                                   |
| ----------------------------- | ------- | ------ | ------------------------------------------------------------- |
| `mcp_connection_events_total` | Counter | event  | Total MCP connection and handshake events by bounded category |

The `event` label is `initialize_ok`, `initialize_error`, `auth_rejected`, `origin_rejected`, `not_found` or
`tools_list_ok`. The agent ignores all other event names. Each of the six series starts at 0.

This counter counts events. It does not show the number of current MCP connections. Use `rate()` for queries:

```promql
# MCP initialize requests per second, by result
sum by (event) (rate(mcp_connection_events_total{event=~"initialize_ok|initialize_error"}[5m]))

# Rejected MCP requests per second
sum(rate(mcp_connection_events_total{event=~"auth_rejected|origin_rejected"}[5m]))
```

## Example Queries

### PromQL Examples

```promql
# CPU usage over time
unraid_cpu_usage_percent

# Memory usage percentage
(unraid_memory_used_bytes / unraid_memory_total_bytes) * 100

# Disk temperatures above 45°C
unraid_disk_temperature_celsius > 45

# Total disk space used
sum(unraid_disk_size_bytes - unraid_disk_free_bytes)

# Running containers count
unraid_docker_containers_running

# Array usage percentage
unraid_array_usage_percent

# GPU temperature alert
unraid_gpu_temperature_celsius > 80
```

### Grafana Dashboard Panels

The series have no `hostname` label. To select one server, use a target label such as `instance`.

#### System Overview Panel

```promql
# CPU Usage
unraid_cpu_usage_percent

# Memory Usage
(unraid_memory_used_bytes / unraid_memory_total_bytes) * 100

# Uptime (days)
unraid_system_uptime_seconds / 86400
```

#### Disk Health Panel

```promql
# Hottest disk
max(unraid_disk_temperature_celsius)

# Disks in standby
sum(unraid_disk_standby)

# Failed SMART checks
count(unraid_disk_smart_status == 0)
```

## Grafana Integration

See [Grafana Dashboard Guide](../integrations/grafana.md) for:

- Pre-built dashboard JSON
- Dashboard setup instructions
- Alert configuration
- Panel examples

## Testing

### Check Metrics Endpoint

```bash
# Fetch all metrics
curl http://localhost:8043/metrics

# Filter specific metric
curl http://localhost:8043/metrics | grep unraid_cpu

# Format with promtool (if installed)
curl http://localhost:8043/metrics | promtool check metrics
```

### Validate Metric Format

```bash
# Check for errors
curl -s http://localhost:8043/metrics | grep "# HELP\|# TYPE"

# Count sample lines
curl -s http://localhost:8043/metrics | grep -c "^unraid_"
```

## Metric Updates

Each scrape returns the current cached values from the collectors. The default collector intervals are:

- **Fast metrics** (15s): CPU, memory, temperatures
- **Standard metrics** (30s): Array, disks, containers
- **Moderate metrics** (60s): UPS, GPU

Scrape interval should match or exceed collector intervals for accurate data.

## Troubleshooting

### No Metrics Returned

```bash
# Check endpoint accessibility
curl http://localhost:8043/metrics

# Verify service is running
ps aux | grep unraid-management-agent

# Check logs
tail -f /var/log/unraid-management-agent.log
```

### Missing Metrics

Some metrics only appear when relevant:

- **GPU metrics**: Only if GPU detected
- **UPS metrics**: Only if UPS configured
- **Disk metrics**: Only for discovered disks

### Stale Metrics

Check collector intervals in plugin settings. Increase scrape frequency or decrease collector intervals.

## Next Steps

- [Grafana Dashboard](../integrations/grafana.md) - Set up visualization
- [REST API Reference](rest-api.md) - API endpoints
- [Configuration](../guides/configuration.md) - Adjust intervals

---

**Last Updated**: January 2026  
**Metrics Count**: 62 application families (61 `unraid_` families and one `mcp_` family)

The endpoint also exposes the Go runtime (`go_*`) and process (`process_*`) families. The count does not include them.

## PSU metrics

| Metric                           | Type  | Labels         | Description                                                        |
| -------------------------------- | ----- | -------------- | ------------------------------------------------------------------ |
| `unraid_psu_state`               | Gauge | state          | PSU snapshot state (always 1); the `state` label carries the state |
| `unraid_psu_device_state`        | Gauge | device, state  | PSU device state (always 1); the `state` label carries the state   |
| `unraid_psu_voltage_volts`       | Gauge | device, sensor | PSU measured voltage in volts                                      |
| `unraid_psu_current_amperes`     | Gauge | device, sensor | PSU measured current in amperes                                    |
| `unraid_psu_power_watts`         | Gauge | device, sensor | PSU power in watts; output channels are DC, not wall power         |
| `unraid_psu_temperature_celsius` | Gauge | device, sensor | PSU temperature in Celsius                                         |
| `unraid_psu_fan_rpm`             | Gauge | device, sensor | PSU fan revolutions per minute                                     |
| `unraid_psu_load_percent`        | Gauge | device, sensor | PSU DC output load derived from rated capacity                     |
| `unraid_psu_efficiency_percent`  | Gauge | device, sensor | PSU efficiency when a backend supplies it                          |

`unraid_psu_state` and `unraid_psu_device_state` expose availability. The `device` label is the PSU device ID.
The `sensor` label is the sensor name, for example `output_power` or `12v_voltage`.
Missing readings produce no samples. The current Corsair backend does not supply an efficiency sample.
See [PSU telemetry](../guides/psu-telemetry.md) for the state values and the measurement meaning.
