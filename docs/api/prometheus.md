# Prometheus Metrics

Collect agent metrics with Prometheus. Use Grafana to show them.

## Overview

The agent serves Prometheus metrics at `/metrics`. The available series depend on the collectors and their data.

**Endpoint**: `http://<unraid-ip>:8043/metrics`  
**Format**: Prometheus text format  
**Update Frequency**: Real-time (current cached values)

Set `metrics_enabled: false` to turn this endpoint off; it then returns 404. See
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

| Metric                           | Type  | Description                                                             |
| -------------------------------- | ----- | ----------------------------------------------------------------------- |
| `unraid_cpu_usage_percent`       | Gauge | CPU usage percentage (0-100)                                            |
| `unraid_cpu_temperature_celsius` | Gauge | CPU temperature in Celsius                                              |
| `unraid_memory_used_bytes`       | Gauge | Memory used in bytes                                                    |
| `unraid_memory_total_bytes`      | Gauge | Total memory in bytes                                                   |
| `unraid_memory_usage_percent`    | Gauge | Memory usage percentage (0-100)                                         |
| `unraid_uptime_seconds`          | Gauge | System uptime in seconds                                                |
| `unraid_cpu_power_watts`         | Gauge | CPU package power consumption in watts (Intel RAPL, 0 when unavailable) |
| `unraid_dram_power_watts`        | Gauge | DRAM power consumption in watts (Intel RAPL, 0 when unavailable)        |

**Labels**: `hostname`, `version`

### Array Metrics

| Metric                              | Type  | Description                                    |
| ----------------------------------- | ----- | ---------------------------------------------- |
| `unraid_array_state`                | Gauge | Array state (0=stopped, 1=started, 2=stopping) |
| `unraid_array_size_bytes`           | Gauge | Total array size in bytes                      |
| `unraid_array_free_bytes`           | Gauge | Free space in bytes                            |
| `unraid_array_used_percent`         | Gauge | Array usage percentage (0-100)                 |
| `unraid_array_parity_valid`         | Gauge | Parity valid (0=invalid, 1=valid)              |
| `unraid_array_parity_check_running` | Gauge | Parity check running (0=no, 1=yes)             |
| `unraid_array_parity_check_percent` | Gauge | Parity check progress (0-100)                  |

**Labels**: `hostname`

### Disk Metrics

| Metric                            | Type  | Description                              |
| --------------------------------- | ----- | ---------------------------------------- |
| `unraid_disk_size_bytes`          | Gauge | Disk size in bytes                       |
| `unraid_disk_free_bytes`          | Gauge | Free space in bytes                      |
| `unraid_disk_temperature_celsius` | Gauge | Disk temperature in Celsius              |
| `unraid_disk_standby`             | Gauge | Disk standby state (0=active, 1=standby) |
| `unraid_disk_smart_status`        | Gauge | SMART status (0=fail, 1=pass)            |

**Labels**: `hostname`, `disk_id`, `disk_name`, `device`

The agent omits a disk temperature series if the reading is unavailable. This can occur when the disk is in standby.
The agent removes an existing series if its reading becomes unavailable. A measured 0 °C remains a sample.

### Container Metrics (Docker)

| Metric                      | Type  | Description                  |
| --------------------------- | ----- | ---------------------------- |
| `unraid_containers_total`   | Gauge | Total number of containers   |
| `unraid_containers_running` | Gauge | Number of running containers |
| `unraid_containers_stopped` | Gauge | Number of stopped containers |
| `unraid_containers_paused`  | Gauge | Number of paused containers  |

**Labels**: `hostname`

### VM Metrics

| Metric               | Type  | Description           |
| -------------------- | ----- | --------------------- |
| `unraid_vms_total`   | Gauge | Total number of VMs   |
| `unraid_vms_running` | Gauge | Number of running VMs |
| `unraid_vms_stopped` | Gauge | Number of stopped VMs |

**Labels**: `hostname`

### GPU Metrics

| Metric                           | Type  | Description                |
| -------------------------------- | ----- | -------------------------- |
| `unraid_gpu_utilization_percent` | Gauge | GPU utilization (0-100)    |
| `unraid_gpu_temperature_celsius` | Gauge | GPU temperature in Celsius |
| `unraid_gpu_memory_used_bytes`   | Gauge | GPU memory used in bytes   |
| `unraid_gpu_memory_total_bytes`  | Gauge | Total GPU memory in bytes  |
| `unraid_gpu_power_watts`         | Gauge | GPU power draw in watts    |

**Labels**: `hostname`, `gpu_index`, `gpu_name`

The agent omits GPU temperature, used memory, total memory, and power series if the readings are unavailable.
It removes existing series if their readings become unavailable. Measured zero values remain samples.

### UPS Metrics

| Metric                              | Type  | Description                      |
| ----------------------------------- | ----- | -------------------------------- |
| `unraid_ups_battery_charge_percent` | Gauge | Battery charge (0-100)           |
| `unraid_ups_load_percent`           | Gauge | UPS load (0-100)                 |
| `unraid_ups_time_left_seconds`      | Gauge | Estimated runtime in seconds     |
| `unraid_ups_online`                 | Gauge | UPS status (0=offline, 1=online) |

**Labels**: `hostname`, `ups_name`

The battery, load and runtime gauges are not exported when the UPS does not
report that value, rather than being exported as 0.

### Share Metrics

| Metric                             | Type  | Description                                           |
| ---------------------------------- | ----- | ----------------------------------------------------- |
| `unraid_shares_total`              | Gauge | Total number of shares                                |
| `unraid_share_used_bytes`          | Gauge | Measured share content in bytes; omitted when unknown |
| `unraid_share_storage_used_bytes`  | Gauge | Backing-storage used space in bytes                   |
| `unraid_share_storage_free_bytes`  | Gauge | Backing-storage free space in bytes                   |
| `unraid_share_storage_total_bytes` | Gauge | Backing-storage capacity in bytes                     |

**Labels**: `name` on per-share gauges; no labels on `unraid_shares_total`.

Measured usage is available only for cache-only shares on an assigned mounted ZFS pool whose members
are all known spinning. A measured empty share emits zero. Storage figures are shared by all shares
on the same backing storage, so summing these per-share gauges can double-count capacity.

### Service Metrics

| Metric                   | Type  | Description                             |
| ------------------------ | ----- | --------------------------------------- |
| `unraid_service_enabled` | Gauge | Service enabled (0=disabled, 1=enabled) |

**Labels**: `hostname`, `service` (docker, vm_manager)

## Example Queries

### PromQL Examples

```promql
# CPU usage over time
unraid_cpu_usage_percent{hostname="Tower"}

# Memory usage percentage
(unraid_memory_used_bytes / unraid_memory_total_bytes) * 100

# Disk temperatures above 45°C
unraid_disk_temperature_celsius > 45

# Total disk space used
sum(unraid_disk_size_bytes - unraid_disk_free_bytes)

# Running containers count
unraid_containers_running

# Array usage percentage
unraid_array_used_percent

# GPU temperature alert
unraid_gpu_temperature_celsius > 80
```

### Grafana Dashboard Panels

#### System Overview Panel

```promql
# CPU Usage
unraid_cpu_usage_percent{hostname="$hostname"}

# Memory Usage
(unraid_memory_used_bytes{hostname="$hostname"} / unraid_memory_total_bytes{hostname="$hostname"}) * 100

# Uptime (days)
unraid_uptime_seconds{hostname="$hostname"} / 86400
```

#### Disk Health Panel

```promql
# Hottest disk
max(unraid_disk_temperature_celsius{hostname="$hostname"})

# Disks in standby
sum(unraid_disk_standby{hostname="$hostname"})

# Failed SMART checks
count(unraid_disk_smart_status{hostname="$hostname"} == 0)
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

# Count metrics
curl -s http://localhost:8043/metrics | grep -c "^unraid_"
```

## Metric Updates

Metrics reflect current cached values from collectors:

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
**Metrics Count**: 41

## PSU metrics

`unraid_psu_state` and `unraid_psu_device_state` expose availability. Per-device sensor metrics
cover voltage, current, DC output watts, temperature, fan RPM and derived load. Missing readings
produce no samples. See [PSU telemetry](../guides/psu-telemetry.md) for exact series names.
