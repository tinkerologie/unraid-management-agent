# Grafana Dashboard

Set up monitoring and visualization for your Unraid server using Grafana and Prometheus.

## Overview

This guide shows you how to create a comprehensive Grafana dashboard for the Unraid Management Agent using the Prometheus metrics endpoint.

## Prerequisites

- Unraid Management Agent installed and running
- Prometheus server (can run in Docker on Unraid)
- Grafana instance (can run in Docker on Unraid)

## Quick Setup

### 1. Deploy Prometheus (Docker)

Create `docker-compose.yml`:

```yaml
version: '3.8'

services:
  prometheus:
    image: prom/prometheus:latest
    container_name: prometheus
    restart: unless-stopped
    volumes:
      - ./prometheus.yml:/etc/prometheus/prometheus.yml:ro
      - prometheus-data:/prometheus
    ports:
      - "9090:9090"
    command:
      - '--config.file=/etc/prometheus/prometheus.yml'
      - '--storage.tsdb.path=/prometheus'
      - '--storage.tsdb.retention.time=30d'
    networks:
      - monitoring

  grafana:
    image: grafana/grafana:latest
    container_name: grafana
    restart: unless-stopped
    volumes:
      - grafana-data:/var/lib/grafana
    ports:
      - "3000:3000"
    environment:
      - GF_SECURITY_ADMIN_PASSWORD=admin
      - GF_INSTALL_PLUGINS=
    networks:
      - monitoring
    depends_on:
      - prometheus

volumes:
  prometheus-data:
  grafana-data:

networks:
  monitoring:
```

Create `prometheus.yml`:

```yaml
global:
  scrape_interval: 15s
  evaluation_interval: 15s

scrape_configs:
  - job_name: 'unraid'
    static_configs:
      - targets: ['YOUR_UNRAID_IP:8043']
    metrics_path: '/metrics'
```

Deploy:

```bash
docker-compose up -d
```

### 2. Configure Grafana

1. Open Grafana: `http://YOUR_SERVER_IP:3000`
2. Login (default: admin/admin)
3. Add Prometheus data source:
   - **Configuration** → **Data Sources** → **Add data source**
   - Select **Prometheus**
   - URL: `http://prometheus:9090`
   - Click **Save & Test**

### 3. Import Dashboard

**Coming Soon**: Pre-built dashboard JSON

For now, create panels manually using the queries below.

## Dashboard Panels

### System Overview

#### CPU Usage Panel
- **Type**: Graph/Time series
- **Query**: `unraid_cpu_usage_percent{hostname="$hostname"}`
- **Unit**: Percent (0-100)
- **Thresholds**: Warning: 80, Critical: 90

#### Memory Usage Panel
- **Type**: Graph/Time series
- **Query**: `(unraid_memory_used_bytes{hostname="$hostname"} / unraid_memory_total_bytes{hostname="$hostname"}) * 100`
- **Unit**: Percent (0-100)

#### CPU Temperature Panel
- **Type**: Gauge
- **Query**: `unraid_cpu_temperature_celsius{hostname="$hostname"}`
- **Unit**: Celsius
- **Thresholds**: Warning: 70, Critical: 85

#### Uptime Panel
- **Type**: Stat
- **Query**: `unraid_uptime_seconds{hostname="$hostname"} / 86400`
- **Unit**: Days
- **Decimals**: 2

### Array & Storage

#### Array Usage Panel
- **Type**: Gauge
- **Query**: `unraid_array_used_percent{hostname="$hostname"}`
- **Unit**: Percent (0-100)
- **Thresholds**: Warning: 80, Critical: 90

#### Array Capacity Panel
- **Type**: Stat
- **Query A** (Used): `(unraid_array_size_bytes{hostname="$hostname"} - unraid_array_free_bytes{hostname="$hostname"}) / 1e12`
- **Query B** (Total): `unraid_array_size_bytes{hostname="$hostname"} / 1e12`
- **Unit**: TB

#### Parity Status Panel
- **Type**: Stat
- **Query**: `unraid_array_parity_valid{hostname="$hostname"}`
- **Value Mappings**: 0=Invalid, 1=Valid
- **Thresholds**: 0=Red, 1=Green

#### Disk Temperatures Panel
- **Type**: Bar gauge
- **Query**: `unraid_disk_temperature_celsius{hostname="$hostname"}`
- **Legend**: `{{disk_name}}`
- **Unit**: Celsius
- **Thresholds**: Warning: 45, Critical: 55

### Docker Containers

#### Container Status Panel
- **Type**: Stat
- **Query A** (Running): `unraid_containers_running{hostname="$hostname"}`
- **Query B** (Stopped): `unraid_containers_stopped{hostname="$hostname"}`
- **Query C** (Total): `unraid_containers_total{hostname="$hostname"}`

#### Container States Pie Chart
- **Type**: Pie chart
- **Query A** (Running): `unraid_containers_running{hostname="$hostname"}`
- **Query B** (Stopped): `unraid_containers_stopped{hostname="$hostname"}`
- **Query C** (Paused): `unraid_containers_paused{hostname="$hostname"}`

### Virtual Machines

#### VM Status Panel
- **Type**: Stat
- **Query A** (Running): `unraid_vms_running{hostname="$hostname"}`
- **Query B** (Stopped): `unraid_vms_stopped{hostname="$hostname"}`
- **Query C** (Total): `unraid_vms_total{hostname="$hostname"}`

### GPU Monitoring

#### GPU Utilization Panel
- **Type**: Graph
- **Query**: `unraid_gpu_utilization_percent{hostname="$hostname"}`
- **Legend**: `{{gpu_name}}`
- **Unit**: Percent (0-100)

#### GPU Temperature Panel
- **Type**: Gauge
- **Query**: `unraid_gpu_temperature_celsius{hostname="$hostname"}`
- **Unit**: Celsius
- **Thresholds**: Warning: 75, Critical: 85

#### GPU Memory Panel
- **Type**: Graph
- **Query**: `(unraid_gpu_memory_used_bytes{hostname="$hostname"} / unraid_gpu_memory_total_bytes{hostname="$hostname"}) * 100`
- **Unit**: Percent (0-100)

### UPS Monitoring

#### Battery Charge Panel
- **Type**: Gauge
- **Query**: `unraid_ups_battery_charge_percent{hostname="$hostname"}`
- **Unit**: Percent (0-100)
- **Thresholds**: Critical: 20, Warning: 40, OK: 80

#### UPS Load Panel
- **Type**: Gauge
- **Query**: `unraid_ups_load_percent{hostname="$hostname"}`
- **Unit**: Percent (0-100)

#### Runtime Remaining Panel
- **Type**: Stat
- **Query**: `unraid_ups_time_left_seconds{hostname="$hostname"} / 60`
- **Unit**: Minutes

## Alert Rules

### Prometheus Alert Rules

Create `alerts.yml`:

```yaml
groups:
  - name: unraid_alerts
    interval: 30s
    rules:
      # High CPU usage
      - alert: HighCPUUsage
        expr: unraid_cpu_usage_percent > 90
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High CPU usage on {{ $labels.hostname }}"
          description: "CPU usage is {{ $value }}%"

      # High memory usage
      - alert: HighMemoryUsage
        expr: (unraid_memory_used_bytes / unraid_memory_total_bytes) * 100 > 90
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High memory usage on {{ $labels.hostname }}"
          description: "Memory usage is {{ $value }}%"

      # Array nearly full
      - alert: ArrayNearlyFull
        expr: unraid_array_used_percent > 90
        for: 10m
        labels:
          severity: warning
        annotations:
          summary: "Array nearly full on {{ $labels.hostname }}"
          description: "Array is {{ $value }}% full"

      # Parity invalid
      - alert: ParityInvalid
        expr: unraid_array_parity_valid == 0
        for: 1m
        labels:
          severity: critical
        annotations:
          summary: "Parity invalid on {{ $labels.hostname }}"
          description: "Array parity is invalid"

      # Disk temperature high
      - alert: DiskTemperatureHigh
        expr: unraid_disk_temperature_celsius > 55
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High disk temperature on {{ $labels.hostname }}"
          description: "Disk {{ $labels.disk_name }} is {{ $value }}°C"

      # SMART failure
      - alert: SMARTFailure
        expr: unraid_disk_smart_status == 0
        for: 1m
        labels:
          severity: critical
        annotations:
          summary: "SMART failure on {{ $labels.hostname }}"
          description: "Disk {{ $labels.disk_name }} failed SMART check"

      # UPS battery low
      - alert: UPSBatteryLow
        expr: unraid_ups_battery_charge_percent < 30
        for: 2m
        labels:
          severity: warning
        annotations:
          summary: "UPS battery low on {{ $labels.hostname }}"
          description: "UPS battery is {{ $value }}%"
```

Add to `prometheus.yml`:

```yaml
rule_files:
  - 'alerts.yml'
```

## Variables

Create dashboard variables for dynamic filtering:

### Hostname Variable
- **Name**: hostname
- **Type**: Query
- **Query**: `label_values(unraid_cpu_usage_percent, hostname)`
- **Multi-value**: No
- **Include All**: No

## Tips & Best Practices

### Performance
- Set appropriate scrape intervals (15-30s recommended)
- Use recording rules for complex queries
- Limit retention time to manage storage

### Organization
- Group related panels in rows
- Use consistent color schemes
- Add descriptive panel titles

### Alerts
- Set appropriate thresholds for your hardware
- Use "for" duration to avoid false positives
- Configure notification channels (email, Slack, etc.)

## Example Dashboard JSON

**Coming Soon**: Full dashboard JSON export will be added to the repository.

For now, use the panel configurations above to build your custom dashboard.

## Troubleshooting

### No Data in Panels

1. Verify Prometheus is scraping:
   ```bash
   # Check Prometheus targets
   curl http://prometheus:9090/api/v1/targets
   ```

2. Check metrics endpoint:
   ```bash
   curl http://UNRAID_IP:8043/metrics
   ```

3. Verify Grafana data source connection

### Missing Metrics

- Check collector intervals in plugin settings
- Verify hardware is detected (GPU, UPS, etc.)
- Review agent logs for errors

## Next Steps

- [Prometheus Metrics Reference](prometheus.md) - Complete metrics list
- [REST API Reference](rest-api.md) - API documentation
- [Configuration Guide](../guides/configuration.md) - Adjust settings

---

**Last Updated**: January 2026
