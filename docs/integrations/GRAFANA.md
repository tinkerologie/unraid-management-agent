# Grafana Integration Guide

Complete guide to integrating the Unraid Management Agent with Grafana for monitoring and dashboards.

**Version**: 2025.11.26  
**Last Updated**: 2025-11-28

---

## Table of Contents

- [Quick Start](#quick-start)
- [Overview](#overview)
- [Prerequisites](#prerequisites)
- [Installation](#installation)
- [Data Source Configuration](#data-source-configuration)
- [Dashboard Creation](#dashboard-creation)
- [Panel Examples](#panel-examples)
- [WebSocket Integration](#websocket-integration)
- [Best Practices](#best-practices)
- [Troubleshooting](#troubleshooting)
- [Sample Dashboard JSON](#sample-dashboard-json)

---

## Quick Start

**Want to get started immediately?** Import the pre-built dashboard:

📥 **[unraid-system-monitor-dashboard.json](./unraid-system-monitor-dashboard.json)**

This production-ready dashboard includes:

- ✅ **16 comprehensive panels** covering all aspects of Unraid monitoring
- ✅ **System metrics**: CPU usage, RAM usage, CPU temperature, motherboard temperature
- ✅ **Array status**: State, capacity, parity validation
- ✅ **Disk information**: Temperatures, sizes, roles, spin states
- ✅ **Docker containers**: Status, CPU, memory usage
- ✅ **Virtual machines**: Status, vCPUs, memory allocation
- ✅ **Correct API field names**: Uses `cpu_usage_percent`, `ram_usage_percent`, `cpu_temp_celsius`, etc.
- ✅ **Proper thresholds**: Green/yellow/red color coding for all metrics
- ✅ **30-second refresh**: Configurable refresh interval (10s, 30s, 1m, 5m, 15m, 30m, 1h)

### Import Instructions

1. In Grafana, navigate to **Dashboards** → **Import**
2. Click **Upload JSON file**
3. Select `unraid-system-monitor-dashboard.json`
4. When prompted, select your **Infinity data source** (configured in [Data Source Configuration](#data-source-configuration))
5. Click **Import**

The dashboard will be immediately ready to use with no additional configuration required!

---

## Overview

The Unraid Management Agent provides a REST API and WebSocket interface that can be consumed by Grafana for real-time monitoring and visualization of your Unraid server.

### Features

- **Real-time Metrics**: CPU, RAM, disk temperatures, array status
- **Historical Data**: Track trends over time using Grafana's time-series capabilities
- **Custom Dashboards**: Create personalized dashboards for your specific needs
- **Alerting**: Set up alerts based on thresholds (CPU temp, disk usage, etc.)
- **WebSocket Support**: Real-time data streaming for live dashboards

### Architecture

```
Unraid Server → Management Agent API → Grafana → Dashboard
                      ↓
                  WebSocket → Live Updates
```

---

## Prerequisites

### Required Software

1. **Grafana** (v9.0 or later recommended)
   - Install on a separate server or Docker container
   - Can run on the Unraid server itself via Docker

2. **Grafana Plugins**:
   - **Infinity Data Source** (recommended) - For REST API queries
   - **JSON API Data Source** (alternative) - For JSON endpoints
   - **WebSocket Data Source** (optional) - For real-time streaming

3. **Unraid Management Agent**:
   - Version 2025.11.23 or later
   - Running on port 8043 (default)

### Network Requirements

- Grafana must be able to reach the Unraid server on port 8043
- If running Grafana in Docker on Unraid, use `http://YOUR_UNRAID_IP:8043`
- For external Grafana instances, ensure firewall allows access

---

## Installation

### Step 1: Install Grafana

#### Option A: Docker on Unraid

```bash
# Create Grafana container via Unraid Docker UI
# Or use docker run:
docker run -d \
  --name=grafana \
  -p 3000:3000 \
  -v /mnt/user/appdata/grafana:/var/lib/grafana \
  --restart unless-stopped \
  grafana/grafana:latest
```

#### Option B: Standalone Server

Follow the official Grafana installation guide for your platform:
https://grafana.com/docs/grafana/latest/setup-grafana/installation/

### Step 2: Install Required Plugins

#### Install Infinity Data Source Plugin

```bash
# Via Grafana CLI
grafana-cli plugins install yesoreyeram-infinity-datasource

# Or via Docker environment variable
docker run -d \
  --name=grafana \
  -p 3000:3000 \
  -e "GF_INSTALL_PLUGINS=yesoreyeram-infinity-datasource" \
  grafana/grafana:latest
```

#### Install JSON API Data Source (Alternative)

```bash
grafana-cli plugins install simpod-json-datasource
```

### Step 3: Access Grafana

1. Open browser to `http://YOUR_GRAFANA_IP:3000`
2. Default credentials: `admin` / `admin`
3. Change password when prompted

---

## Data Source Configuration

### Configure Infinity Data Source

1. **Navigate to Data Sources**:
   - Click **Configuration** (gear icon) → **Data Sources**
   - Click **Add data source**
   - Search for **Infinity**
   - Click **Select**

2. **Configure Connection**:
   - **Name**: `Unraid Management Agent`
   - **URL**: `http://YOUR_UNRAID_IP:8043/api/v1`
   - **Auth**: None (API doesn't require authentication)
   - **Allowed hosts**: Add your Unraid IP

3. **Test Connection**:
   - Click **Save & Test**
   - Should show "Data source is working"

### Alternative: JSON API Data Source

1. Add JSON API data source
2. **URL**: `http://YOUR_UNRAID_IP:8043/api/v1`
3. **Access**: Server (default)
4. Save & Test

---

## Dashboard Creation

### Create Your First Dashboard

1. **Create New Dashboard**:
   - Click **+** → **Dashboard**
   - Click **Add new panel**

2. **Configure Panel**:
   - Select **Infinity** as data source
   - Choose visualization type (Stat, Gauge, Time series, etc.)
   - Configure query (see examples below)

3. **Save Dashboard**:
   - Click **Save dashboard** (disk icon)
   - Enter name: "Unraid System Monitor"
   - Click **Save**

---

## Panel Examples

### Example 1: CPU Usage Gauge

**Panel Type**: Gauge

**Query Configuration**:
- **Type**: JSON
- **Parser**: Backend
- **Source**: URL
- **URL**: `/system`
- **Method**: GET

**Data Transformation**:
- **Fields**: Select `cpu_usage_percent`
- **Display Name**: CPU Usage
- **Unit**: Percent (0-100)

**Thresholds**:
- Green: 0-60
- Yellow: 60-80
- Red: 80-100

### Example 2: RAM Usage Stat

**Panel Type**: Stat

**Query Configuration**:
- **Type**: JSON
- **Source**: URL
- **URL**: `/system`
- **Method**: GET

**Data Transformation**:
```
Fields to extract:
- ram_usage_percent (display as percentage)
- ram_used_bytes (convert to GB)
- ram_total_bytes (convert to GB)
```

**Calculation** (using Grafana transformations):
```
Add field from calculation:
- Mode: Binary operation
- Operation: ram_used_bytes / 1073741824
- Alias: RAM Used (GB)

Add field from calculation:
- Mode: Binary operation
- Operation: ram_total_bytes / 1073741824
- Alias: RAM Total (GB)
```

**Display**:
- **Value**: `ram_usage_percent`
- **Unit**: Percent (0-100)
- **Decimals**: 1

### Example 3: CPU Temperature Gauge

**Panel Type**: Gauge

**Query Configuration**:
- **Type**: JSON
- **Source**: URL
- **URL**: `/system`
- **Method**: GET
- **Field**: `cpu_temp_celsius`

**Thresholds**:
- Green: 0-60°C
- Yellow: 60-75°C
- Red: 75-100°C

**Display**:
- **Unit**: Temperature (°C)
- **Min**: 0
- **Max**: 100

### Example 4: Array Status Overview

**Panel Type**: Stat (multiple stats)

**Query Configuration**:
- **Type**: JSON
- **Source**: URL
- **URL**: `/array`
- **Method**: GET

**Fields to Display**:
1. **Array State**:
   - Field: `state`
   - Display: Text
   - Color mode: Value
   - Mappings:
     - `STARTED` → Green
     - `STOPPED` → Red
     - `STARTING` → Yellow

2. **Array Usage**:
   - Field: `used_percent`
   - Unit: Percent (0-100)
   - Thresholds: 0-70 (green), 70-90 (yellow), 90-100 (red)

3. **Parity Valid**:
   - Field: `parity_valid`
   - Display: Boolean
   - Mappings:
     - `true` → ✓ Valid (Green)
     - `false` → ✗ Invalid (Red)

### Example 5: Disk Temperatures Table

**Panel Type**: Table

**Query Configuration**:
- **Type**: JSON
- **Source**: URL
- **URL**: `/disks`
- **Method**: GET
- **Format**: Table

**Columns to Display**:
- `name` → Disk Name
- `device` → Device
- `role` → Role
- `temperature_celsius` → Temperature (°C)
- `spin_state` → Spin State
- `size_bytes` → Size (convert to TB)

**Transformation**:
```
Organize fields:
- name
- device
- role
- temperature_celsius
- spin_state
- size_bytes

Add field from calculation:
- Mode: Binary operation
- Operation: size_bytes / 1099511627776
- Alias: Size (TB)
- Decimals: 2
```

**Conditional Formatting**:
- Temperature > 45°C → Yellow
- Temperature > 55°C → Red
- Spin State = "standby" → Gray

### Example 6: Docker Container Status

**Panel Type**: Table

**Query Configuration**:
- **Type**: JSON
- **Source**: URL
- **URL**: `/docker`
- **Method**: GET

**Columns**:
- `name` → Container Name
- `state` → Status
- `cpu_percent` → CPU %
- `memory_usage_bytes` → Memory (convert to MB)

**Transformation**:
```
Add field from calculation:
- Mode: Binary operation
- Operation: memory_usage_bytes / 1048576
- Alias: Memory (MB)
- Decimals: 0
```

**Conditional Formatting**:
- State = "running" → Green
- State = "stopped" → Red
- State = "paused" → Yellow

### Example 7: Array Capacity Time Series

**Panel Type**: Time series

**Query Configuration**:
- **Type**: JSON
- **Source**: URL
- **URL**: `/array`
- **Method**: GET
- **Refresh**: 30s

**Fields**:
- `total_bytes` → Total Capacity
- `free_bytes` → Free Space
- Calculate: `used_bytes = total_bytes - free_bytes`

**Transformation**:
```
Add field from calculation:
- Mode: Binary operation
- Operation: total_bytes / 1099511627776
- Alias: Total (TB)

Add field from calculation:
- Mode: Binary operation
- Operation: free_bytes / 1099511627776
- Alias: Free (TB)

Add field from calculation:
- Mode: Binary operation
- Operation: (total_bytes - free_bytes) / 1099511627776
- Alias: Used (TB)
```

**Display**:
- **Unit**: Data (IEC)
- **Legend**: Show
- **Tooltip**: All series

---

## WebSocket Integration

### Real-Time Data Streaming

For live dashboards with instant updates, use WebSocket connections.

#### Install WebSocket Data Source Plugin

```bash
grafana-cli plugins install golioth-websocket-datasource
```

#### Configure WebSocket Data Source

1. **Add WebSocket Data Source**:
   - Name: `Unraid WebSocket`
   - URL: `ws://YOUR_UNRAID_IP:8043/api/v1/ws`

2. **Configure Panel**:
   - Select WebSocket data source
   - Message format: JSON
   - Parse incoming messages

#### WebSocket Message Format

The WebSocket sends events in this format:

```json
{
  "event": "system_update",
  "data": {
    "hostname": "Cube",
    "cpu_usage_percent": 15.5,
    "ram_usage_percent": 41.82,
    "cpu_temp_celsius": 36,
    "timestamp": "2025-11-17T14:39:17+10:00"
  },
  "timestamp": "2025-11-17T14:39:17+10:00"
}
```

#### Event Types

- `system_update` - System metrics (every 5s)
- `array_status_update` - Array status (every 10s)
- `disk_list_update` - Disk information (every 30s)
- `container_list_update` - Docker containers (every 10s)
- `vm_list_update` - Virtual machines (every 10s)
- `network_list_update` - Network interfaces (every 15s)
- `ups_status_update` - UPS status (every 10s)
- `gpu_metrics_update` - GPU metrics (every 10s)
- `share_list_update` - User shares (every 60s)

---

## Best Practices

### Polling Intervals

**Recommended refresh rates**:
- **System metrics** (CPU, RAM, temps): 5-10 seconds
- **Array status**: 30 seconds
- **Disk information**: 1-5 minutes
- **Docker containers**: 10-30 seconds
- **Network stats**: 10-30 seconds

**Why not faster?**
- Reduces load on Unraid server
- Most metrics don't change that rapidly
- Grafana can interpolate between data points

### Data Retention

Configure Grafana to store historical data:

1. **Use a database backend** (PostgreSQL recommended)
2. **Configure retention policies**:
   - High-resolution data: 7 days
   - Aggregated data: 90 days
   - Long-term trends: 1 year

### Performance Optimization

1. **Limit concurrent queries**: Use query caching
2. **Use variables**: Create dashboard variables for dynamic filtering
3. **Aggregate data**: Use Grafana transformations to reduce data points
4. **Disable auto-refresh**: On complex dashboards when not actively monitoring

### Security Considerations

1. **Network isolation**: Keep Grafana and Unraid on trusted network
2. **Firewall rules**: Restrict access to port 8043
3. **VPN access**: Use VPN for remote access to Grafana
4. **Read-only dashboards**: Share dashboards as read-only links

---

## Troubleshooting

### Connection Issues

**Problem**: "Data source is not working"

**Solutions**:
1. Verify Unraid Management Agent is running:
   ```bash
   ps aux | grep unraid-management-agent
   ```

2. Test API endpoint manually:
   ```bash
   curl http://YOUR_UNRAID_IP:8043/api/v1/health
   ```

3. Check firewall rules:
   ```bash
   iptables -L -n | grep 8043
   ```

4. Verify Grafana can reach Unraid:
   ```bash
   # From Grafana container/server
   curl http://YOUR_UNRAID_IP:8043/api/v1/health
   ```

### No Data Displayed

**Problem**: Panel shows "No data"

**Solutions**:
1. Check query syntax in panel editor
2. Verify field names match API response
3. Check time range (use "Last 5 minutes" for testing)
4. Enable query inspector to see raw response

### WebSocket Not Connecting

**Problem**: WebSocket connection fails

**Solutions**:
1. Verify WebSocket URL: `ws://` not `http://`
2. Check browser console for errors
3. Test WebSocket manually using a WebSocket client
4. Ensure no proxy is blocking WebSocket connections

### Incorrect Values

**Problem**: Values don't match Unraid UI

**Solutions**:
1. Verify field names (use `_bytes` not `_gb`, `_celsius` not `_temp`)
2. Check unit conversions (bytes to GB: divide by 1073741824)
3. Ensure transformations are applied correctly
4. Compare raw API response with Grafana display

## Sample Dashboard JSON

### Complete Unraid System Monitor Dashboard

Below is a complete Grafana dashboard JSON that you can import directly into Grafana.

**To Import**:
1. Copy the JSON below
2. In Grafana, click **+** → **Import**
3. Paste JSON
4. Select your Infinity data source
5. Click **Import**

```json
{
  "dashboard": {
    "title": "Unraid System Monitor",
    "tags": ["unraid", "system", "monitoring"],
    "timezone": "browser",
    "refresh": "30s",
    "time": {
      "from": "now-6h",
      "to": "now"
    },
    "panels": [
      {
        "id": 1,
        "title": "CPU Usage",
        "type": "gauge",
        "gridPos": {"h": 8, "w": 6, "x": 0, "y": 0},
        "targets": [
          {
            "datasource": "Unraid Management Agent",
            "type": "json",
            "url": "/system",
            "method": "GET",
            "parser": "backend",
            "root_selector": "cpu_usage_percent"
          }
        ],
        "fieldConfig": {
          "defaults": {
            "unit": "percent",
            "min": 0,
            "max": 100,
            "thresholds": {
              "mode": "absolute",
              "steps": [
                {"value": 0, "color": "green"},
                {"value": 60, "color": "yellow"},
                {"value": 80, "color": "red"}
              ]
            }
          }
        }
      },
      {
        "id": 2,
        "title": "RAM Usage",
        "type": "gauge",
        "gridPos": {"h": 8, "w": 6, "x": 6, "y": 0},
        "targets": [
          {
            "datasource": "Unraid Management Agent",
            "type": "json",
            "url": "/system",
            "method": "GET",
            "parser": "backend",
            "root_selector": "ram_usage_percent"
          }
        ],
        "fieldConfig": {
          "defaults": {
            "unit": "percent",
            "min": 0,
            "max": 100,
            "thresholds": {
              "mode": "absolute",
              "steps": [
                {"value": 0, "color": "green"},
                {"value": 70, "color": "yellow"},
                {"value": 90, "color": "red"}
              ]
            }
          }
        }
      },
      {
        "id": 3,
        "title": "CPU Temperature",
        "type": "gauge",
        "gridPos": {"h": 8, "w": 6, "x": 12, "y": 0},
        "targets": [
          {
            "datasource": "Unraid Management Agent",
            "type": "json",
            "url": "/system",
            "method": "GET",
            "parser": "backend",
            "root_selector": "cpu_temp_celsius"
          }
        ],
        "fieldConfig": {
          "defaults": {
            "unit": "celsius",
            "min": 0,
            "max": 100,
            "thresholds": {
              "mode": "absolute",
              "steps": [
                {"value": 0, "color": "green"},
                {"value": 60, "color": "yellow"},
                {"value": 75, "color": "red"}
              ]
            }
          }
        }
      },
      {
        "id": 4,
        "title": "Array Status",
        "type": "stat",
        "gridPos": {"h": 8, "w": 6, "x": 18, "y": 0},
        "targets": [
          {
            "datasource": "Unraid Management Agent",
            "type": "json",
            "url": "/array",
            "method": "GET",
            "parser": "backend"
          }
        ],
        "fieldConfig": {
          "defaults": {
            "mappings": [
              {
                "type": "value",
                "options": {
                  "STARTED": {"text": "✓ STARTED", "color": "green"},
                  "STOPPED": {"text": "✗ STOPPED", "color": "red"}
                }
              }
            ]
          },
          "overrides": [
            {
              "matcher": {"id": "byName", "options": "state"},
              "properties": [{"id": "displayName", "value": "Array State"}]
            }
          ]
        }
      },
      {
        "id": 5,
        "title": "Disk Temperatures",
        "type": "table",
        "gridPos": {"h": 12, "w": 24, "x": 0, "y": 8},
        "targets": [
          {
            "datasource": "Unraid Management Agent",
            "type": "json",
            "url": "/disks",
            "method": "GET",
            "parser": "backend"
          }
        ],
        "fieldConfig": {
          "overrides": [
            {
              "matcher": {"id": "byName", "options": "temperature_celsius"},
              "properties": [
                {"id": "displayName", "value": "Temperature (°C)"},
                {
                  "id": "thresholds",
                  "value": {
                    "mode": "absolute",
                    "steps": [
                      {"value": 0, "color": "blue"},
                      {"value": 35, "color": "green"},
                      {"value": 45, "color": "yellow"},
                      {"value": 55, "color": "red"}
                    ]
                  }
                },
                {"id": "custom.cellOptions", "value": {"type": "color-background"}}
              ]
            }
          ]
        }
      }
    ]
  }
}
```

### Dashboard Variables

Add variables for dynamic filtering:

**Variable: disk_role**
- **Type**: Query
- **Data source**: Unraid Management Agent
- **Query**: `/disks`
- **Regex**: Extract `role` field
- **Multi-value**: Yes
- **Include All**: Yes

**Variable: container_state**
- **Type**: Query
- **Data source**: Unraid Management Agent
- **Query**: `/docker`
- **Regex**: Extract `state` field
- **Multi-value**: Yes
- **Include All**: Yes

---

## Advanced Queries

### Calculate Array Used Space in TB

```
Transformation: Add field from calculation
- Mode: Binary operation
- Operation: (total_bytes - free_bytes) / 1099511627776
- Alias: Used (TB)
- Decimals: 2
```

### Filter Running Containers Only

```
Transformation: Filter data by values
- Field: state
- Match: running
```

### Calculate Disk Usage Percentage

```
Transformation: Add field from calculation
- Mode: Binary operation
- Operation: (used_bytes / size_bytes) * 100
- Alias: Usage %
- Decimals: 1
```

### Convert Bytes to Human-Readable Format

| Bytes | Division | Unit |
|-------|----------|------|
| `size_bytes` | ÷ 1024 | KB |
| `size_bytes` | ÷ 1048576 | MB |
| `size_bytes` | ÷ 1073741824 | GB |
| `size_bytes` | ÷ 1099511627776 | TB |

---

## Additional Resources

### Official Documentation

- **Grafana Documentation**: https://grafana.com/docs/
- **Infinity Plugin**: https://grafana.com/grafana/plugins/yesoreyeram-infinity-datasource/
- **Unraid Management Agent API**: [API Reference](../api/API_REFERENCE.md)

### Community Dashboards

Share your dashboards with the community:
1. Export dashboard JSON
2. Create GitHub Gist
3. Share link in Unraid forums

### Example Queries

See the [API Reference](../api/API_REFERENCE.md) for complete endpoint documentation and response formats.

---

## Summary

You now have a complete Grafana integration for monitoring your Unraid server! Key takeaways:

✅ **Use Infinity Data Source** for REST API queries
✅ **Refresh every 30-60 seconds** for most metrics
✅ **Convert bytes to GB/TB** using transformations
✅ **Use correct field names** (`_bytes`, `_celsius`, `_percent`)
✅ **Set up thresholds** for visual alerts
✅ **Consider WebSocket** for real-time dashboards

**Next Steps**:
1. Import the sample dashboard
2. Customize panels for your needs
3. Set up alerting rules
4. Share your dashboard with the community!

---

**Last Updated**: 2025-11-17
**Version**: 2025.11.23
**Feedback**: Report issues on GitHub


