# Unraid Management Agent Documentation

Complete documentation for the Unraid Management Agent plugin and API.

---

## 📚 Documentation Index

### Getting Started

- **[Quick Start Guide](#-quick-start)** - Get up and running in minutes
- **[Installation](#installation)** - Install the plugin on your Unraid server

### API Reference

- **[API Reference](api/API_REFERENCE.md)** - Complete REST API documentation (49 endpoints)
- **[WebSocket Events](websocket/WEBSOCKET_EVENTS_DOCUMENTATION.md)** - Real-time event streaming guide
- **[WebSocket Event Structure](websocket/WEBSOCKET_EVENT_STRUCTURE.md)** - Technical event format details

### Integrations

- **[Grafana Integration](integrations/GRAFANA.md)** - Monitoring dashboards with Grafana
- **[Pre-built Dashboard](integrations/unraid-system-monitor-dashboard.json)** - Ready-to-import Grafana dashboard

### Operations & Maintenance

- **[System Requirements](SYSTEM_REQUIREMENTS_AND_DEPENDENCIES.md)** - Prerequisites and dependencies
- **[Diagnostic Commands](DIAGNOSTIC_COMMANDS.md)** - Troubleshooting commands
- **[Quick Reference](QUICK_REFERENCE_DEPENDENCIES.md)** - Dependency quick reference

### Development

- **[Contributing Guide](../CONTRIBUTING.md)** - How to contribute to the project
- **[Changelog](../CHANGELOG.md)** - Version history and release notes

---

## 🚀 Quick Start

### Installation

1. In the Unraid web UI, go to **Plugins** → **Install Plugin**
2. Paste the plugin URL:

   ```
   https://github.com/ruaan-deysel/unraid-management-agent/raw/main/unraid-management-agent.plg
   ```

3. Click **Install**
4. The service starts automatically on port **8043**

### Verify Installation

```bash
# Health check
curl http://YOUR_UNRAID_IP:8043/api/v1/health

# Expected response:
# {"status":"ok"}
```

### Basic API Examples

```bash
# Get system information
curl http://YOUR_UNRAID_IP:8043/api/v1/system

# Get array status
curl http://YOUR_UNRAID_IP:8043/api/v1/array

# List all disks
curl http://YOUR_UNRAID_IP:8043/api/v1/disks

# List Docker containers
curl http://YOUR_UNRAID_IP:8043/api/v1/docker

# Get log files
curl http://YOUR_UNRAID_IP:8043/api/v1/logs

# Read syslog (last 50 lines)
curl "http://YOUR_UNRAID_IP:8043/api/v1/logs/syslog?lines=50"
```

---

## 📖 API Endpoints Overview

### Monitoring Endpoints (GET)

| Category       | Endpoint                      | Description                       |
| -------------- | ----------------------------- | --------------------------------- |
| **System**     | `/health`                     | Health check                      |
|                | `/system`                     | CPU, memory, temperatures, uptime |
| **Array**      | `/array`                      | Array status, capacity, parity    |
|                | `/array/parity-check/history` | Parity check history              |
| **Storage**    | `/disks`                      | All disks with SMART data         |
|                | `/disks/{id}`                 | Single disk details               |
|                | `/shares`                     | User shares                       |
|                | `/shares/{name}/config`       | Share configuration               |
| **Containers** | `/docker`                     | All Docker containers             |
|                | `/docker/{id}`                | Container details                 |
| **VMs**        | `/vm`                         | All virtual machines              |
|                | `/vm/{id}`                    | VM details                        |
| **Hardware**   | `/ups`                        | UPS status                        |
|                | `/gpu`                        | GPU metrics                       |
|                | `/network`                    | Network interfaces                |
|                | `/hardware/full`              | Complete hardware info            |
| **Logs**       | `/logs`                       | Available log files               |
|                | `/logs/{filename}`            | Read log file content             |
| **Other**      | `/notifications`              | System notifications              |
|                | `/registration`               | License information               |
|                | `/user-scripts`               | User scripts list                 |
|                | `/unassigned`                 | Unassigned devices                |
| **ZFS**        | `/zfs/pools`                  | ZFS pools                         |
|                | `/zfs/datasets`               | ZFS datasets                      |
|                | `/zfs/snapshots`              | ZFS snapshots                     |
|                | `/zfs/arc`                    | ZFS ARC statistics                |

### Control Endpoints (POST)

| Category         | Endpoint                       | Description         |
| ---------------- | ------------------------------ | ------------------- |
| **System**       | `/system/reboot`               | Reboot server       |
|                  | `/system/shutdown`             | Shutdown server     |
| **Array**        | `/array/start`                 | Start array         |
|                  | `/array/stop`                  | Stop array          |
|                  | `/array/parity-check/start`    | Start parity check  |
|                  | `/array/parity-check/stop`     | Stop parity check   |
|                  | `/array/parity-check/pause`    | Pause parity check  |
|                  | `/array/parity-check/resume`   | Resume parity check |
| **Docker**       | `/docker/{id}/start`           | Start container     |
|                  | `/docker/{id}/stop`            | Stop container      |
|                  | `/docker/{id}/restart`         | Restart container   |
|                  | `/docker/{id}/pause`           | Pause container     |
|                  | `/docker/{id}/unpause`         | Unpause container   |
| **VMs**          | `/vm/{id}/start`               | Start VM            |
|                  | `/vm/{id}/stop`                | Stop VM             |
|                  | `/vm/{id}/restart`             | Restart VM          |
|                  | `/vm/{id}/pause`               | Pause VM            |
|                  | `/vm/{id}/resume`              | Resume VM           |
|                  | `/vm/{id}/hibernate`           | Hibernate VM        |
|                  | `/vm/{id}/force-stop`          | Force stop VM       |
| **User Scripts** | `/user-scripts/{name}/execute` | Execute user script |

### WebSocket

| Endpoint | Description                                    |
| -------- | ---------------------------------------------- |
| `/ws`    | Real-time event stream for all monitoring data |

**Total Endpoints: 49**

For complete API documentation with request/response examples, see **[API Reference](api/API_REFERENCE.md)**.

---

## 🔌 WebSocket Events

Connect to `/api/v1/ws` for real-time updates:

```javascript
const ws = new WebSocket("ws://YOUR_UNRAID_IP:8043/api/v1/ws");

ws.onmessage = (event) => {
  const data = JSON.parse(event.data);
  console.log("Event:", data.event, "Data:", data.data);
};
```

### Available Events

| Event                   | Description               | Update Interval |
| ----------------------- | ------------------------- | --------------- |
| `system_update`         | CPU, memory, temperatures | 5s              |
| `array_status_update`   | Array state, capacity     | 10s             |
| `disk_list_update`      | Disk status, SMART        | 30s             |
| `container_list_update` | Docker containers         | 10s             |
| `vm_list_update`        | Virtual machines          | 10s             |
| `network_list_update`   | Network statistics        | 10s             |
| `ups_status_update`     | UPS status                | 30s             |
| `gpu_metrics_update`    | GPU utilization           | 10s             |
| `notifications_update`  | System notifications      | 60s             |

See **[WebSocket Documentation](websocket/WEBSOCKET_EVENTS_DOCUMENTATION.md)** for complete details.

---

## 📊 Grafana Integration

Import the pre-built dashboard for instant monitoring:

1. Install the **Infinity** data source in Grafana
2. Import `docs/integrations/unraid-system-monitor-dashboard.json`
3. Configure the data source URL to your Unraid server

The dashboard includes:

- System metrics (CPU, RAM, temperatures)
- Array status and capacity gauges
- Disk health monitoring
- Docker container status
- VM state overview

See **[Grafana Integration Guide](integrations/GRAFANA.md)** for detailed setup.

---

## 🔧 Configuration

### Service Configuration

The service runs automatically after installation. Configuration file:

```
/boot/config/plugins/unraid-management-agent/config.cfg
```

### Default Settings

| Setting   | Default | Description                                 |
| --------- | ------- | ------------------------------------------- |
| Port      | 8043    | API server port                             |
| Log Level | info    | Logging verbosity (CLI only: `--log-level`) |

### Log File

```
/var/log/unraid-management-agent.log
```

---

## 🐛 Troubleshooting

### Service Status

```bash
# Check if service is running
ps aux | grep unraid-management-agent

# View logs
tail -f /var/log/unraid-management-agent.log

# Restart service
killall unraid-management-agent
nohup /usr/local/emhttp/plugins/unraid-management-agent/unraid-management-agent --port 8043 boot > /dev/null 2>&1 &
```

### API Not Responding

```bash
# Check port is listening
netstat -tlnp | grep 8043

# Test locally
curl http://localhost:8043/api/v1/health
```

### Common Issues

| Issue               | Solution                                          |
| ------------------- | ------------------------------------------------- |
| Port already in use | Change port in config or stop conflicting service |
| Permission denied   | Ensure plugin is properly installed               |
| Empty responses     | Check if required Unraid services are running     |

---

## 📞 Support

- **GitHub Issues**: [Report bugs or request features](https://github.com/ruaan-deysel/unraid-management-agent/issues)
- **Unraid Forums**: [Community discussion](https://forums.unraid.net/topic/178262-home-assistant-unraid-integration)
- **Documentation**: Check this docs folder first

---

## 📄 Version Information

| Item                 | Value             |
| -------------------- | ----------------- |
| **Current Version**  | 2025.11.26        |
| **API Endpoints**    | 49                |
| **WebSocket Events** | 9                 |
| **Minimum Unraid**   | 6.9.0             |
| **Last Updated**     | November 28, 2025 |

---

## 🔗 Quick Links

- [GitHub Repository](https://github.com/ruaan-deysel/unraid-management-agent)
- [Latest Release](https://github.com/ruaan-deysel/unraid-management-agent/releases/latest)
- [API Reference](api/API_REFERENCE.md)
- [Changelog](../CHANGELOG.md)
