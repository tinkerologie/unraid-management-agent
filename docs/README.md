# Unraid Management Agent Documentation

Complete documentation for the Unraid Management Agent plugin and API.

---

## 📚 Documentation Index

### API Documentation

- **[API Reference](api/API_REFERENCE.md)** - Complete API endpoint reference (46 endpoints)
- **[WebSocket Events Documentation](WEBSOCKET_EVENTS_DOCUMENTATION.md)** - Complete guide to WebSocket event system
- **[WebSocket Event Structure](WEBSOCKET_EVENT_STRUCTURE.md)** - Technical details of WebSocket event structure

### Development

- **[WARP](WARP.md)** - Development workflow and architecture reference

### Version History

- **[CHANGELOG](../CHANGELOG.md)** - Detailed version history and release notes

---

## 🚀 Quick Start

### Installation

1. Download the latest plugin package from releases
2. Install via Unraid Plugins page
3. Configure API port (default: 8043)
4. Start the service

### Basic Usage

```bash
# Health check
curl http://YOUR_UNRAID_IP:8043/api/v1/health

# Get system information
curl http://YOUR_UNRAID_IP:8043/api/v1/system

# Get array status
curl http://YOUR_UNRAID_IP:8043/api/v1/array

# Get disk information
curl http://YOUR_UNRAID_IP:8043/api/v1/disks
```

---

## 📖 API Endpoints Reference

### System & Health

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/v1/health` | GET | Health check endpoint |
| `/api/v1/system` | GET | System information (CPU, memory, uptime) |

### Array Management

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/v1/array` | GET | Array status and information |
| `/api/v1/array/start` | POST | Start the array |
| `/api/v1/array/stop` | POST | Stop the array |
| `/api/v1/array/parity-check/start` | POST | Start parity check |
| `/api/v1/array/parity-check/stop` | POST | Stop parity check |
| `/api/v1/array/parity-check/pause` | POST | Pause parity check |
| `/api/v1/array/parity-check/resume` | POST | Resume parity check |
| `/api/v1/array/parity-check/history` | GET | Get parity check history |

### Disks

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/v1/disks` | GET | List all disks |
| `/api/v1/disks/{id}` | GET | Get single disk by ID/device/name |

### Shares

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/v1/shares` | GET | List all shares |
| `/api/v1/shares/{name}/config` | GET | Get share configuration |
| `/api/v1/shares/{name}/config` | POST | Update share configuration |

### Docker

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/v1/docker` | GET | List all containers |
| `/api/v1/docker/{id}` | GET | Get single container by ID/name |
| `/api/v1/docker/{id}/start` | POST | Start container |
| `/api/v1/docker/{id}/stop` | POST | Stop container |
| `/api/v1/docker/{id}/restart` | POST | Restart container |
| `/api/v1/docker/{id}/pause` | POST | Pause container |
| `/api/v1/docker/{id}/unpause` | POST | Unpause container |

### Virtual Machines

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/v1/vm` | GET | List all VMs |
| `/api/v1/vm/{id}` | GET | Get single VM by ID/name |
| `/api/v1/vm/{id}/start` | POST | Start VM |
| `/api/v1/vm/{id}/stop` | POST | Stop VM |
| `/api/v1/vm/{id}/restart` | POST | Restart VM |
| `/api/v1/vm/{id}/pause` | POST | Pause VM |
| `/api/v1/vm/{id}/resume` | POST | Resume VM |
| `/api/v1/vm/{id}/hibernate` | POST | Hibernate VM |
| `/api/v1/vm/{id}/force-stop` | POST | Force stop VM |

### Hardware

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/v1/ups` | GET | UPS status and information |
| `/api/v1/gpu` | GET | GPU information and metrics |
| `/api/v1/network` | GET | Network interfaces and statistics |

### Configuration

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/v1/settings/system` | GET | Get system settings |
| `/api/v1/settings/system` | POST | Update system settings |
| `/api/v1/settings/docker` | GET | Get Docker settings |
| `/api/v1/settings/vm` | GET | Get VM Manager settings |
| `/api/v1/settings/disks` | GET | Get disk settings (spindown delay, etc.) |
| `/api/v1/network/{interface}/config` | GET | Get network interface configuration |

### WebSocket

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/v1/ws` | WebSocket | Real-time event stream |

**Total Endpoints**: 46

---

## 🔌 WebSocket Events

The WebSocket endpoint (`/api/v1/ws`) provides real-time updates for:

- System metrics (CPU, memory, temperature)
- Array status changes
- Disk status changes
- Docker container events
- VM state changes
- UPS status updates
- GPU metrics
- Network statistics

See [WebSocket Events Documentation](WEBSOCKET_EVENTS_DOCUMENTATION.md) for complete details.

---

## 📊 API Coverage

Current API coverage compared to Unraid Web UI:

| Category | Coverage | Status |
|----------|----------|--------|
| **Overall** | **60%** | 🟡 Partial |
| Monitoring | 85% | ✅ Good |
| Control Operations | 75% | ✅ Good |
| Configuration | 40% | 🟡 Partial |
| Administration | 0% | 🔴 None |

See [API Coverage Analysis](api/API_COVERAGE_ANALYSIS.md) for detailed breakdown.

---

## 🏗️ Architecture

### Components

- **API Server** - RESTful HTTP API with gorilla/mux router
- **WebSocket Server** - Real-time event streaming
- **Collectors** - Data collection services (system, disk, docker, vm, etc.)
- **Controllers** - Action controllers (array, docker, vm)
- **Orchestrator** - Service startup coordination
- **PubSub Hub** - Event distribution system

### Data Flow

```
Unraid System → Collectors → Cache → API Handlers → JSON Response
                    ↓
                PubSub Hub → WebSocket → Clients
```

---

## 🔧 Configuration

### Environment Variables

- `PORT` - API server port (default: 8043)
- `LOG_LEVEL` - Logging level (debug, info, warning, error)

### Configuration Files

- `/boot/config/plugins/unraid-management-agent/config.cfg` - Plugin configuration
- `/var/log/unraid-management-agent.log` - Application logs

---

## 🧪 Testing

### Manual Testing

```bash
# Test all endpoints
curl http://YOUR_UNRAID_IP:8043/api/v1/health
curl http://YOUR_UNRAID_IP:8043/api/v1/system
curl http://YOUR_UNRAID_IP:8043/api/v1/array
curl http://YOUR_UNRAID_IP:8043/api/v1/disks
curl http://YOUR_UNRAID_IP:8043/api/v1/docker
curl http://YOUR_UNRAID_IP:8043/api/v1/vm
curl http://YOUR_UNRAID_IP:8043/api/v1/settings/disks
```

### WebSocket Testing

See `tests/test_websocket.py` for WebSocket testing examples.

---

## 🐛 Troubleshooting

### Service Not Starting

```bash
# Check service status
ps aux | grep unraid-management-agent

# Check logs
tail -f /var/log/unraid-management-agent.log

# Restart service
killall unraid-management-agent
nohup /usr/local/emhttp/plugins/unraid-management-agent/unraid-management-agent --port 8043 boot > /dev/null 2>&1 &
```

### API Not Responding

```bash
# Check if port is listening
netstat -tlnp | grep 8043

# Test health endpoint
curl http://localhost:8043/api/v1/health

# Check firewall
iptables -L -n | grep 8043
```

### WebSocket Connection Issues

- Ensure WebSocket protocol is supported by client
- Check for proxy/firewall blocking WebSocket connections
- Verify correct WebSocket URL: `ws://YOUR_UNRAID_IP:8043/api/v1/ws`

---

## 📝 Contributing

### Development Workflow

1. Clone repository
2. Make changes
3. Build: `make build`
4. Test locally
5. Deploy to test server
6. Verify functionality
7. Commit changes
8. Create pull request

### Code Style

- Follow Go best practices
- Use meaningful variable names
- Add comments for complex logic
- Write tests for new features

---

## 📄 License

See LICENSE file for details.

---

## 🔗 Links

- **GitHub Repository**: [unraid-management-agent](https://github.com/ruaan-deysel/unraid-management-agent)
- **Unraid Forums**: [Plugin Discussion](https://forums.unraid.net/)
- **Home Assistant Integration**: Coming soon

---

## 📞 Support

For issues, questions, or feature requests:

1. Check existing documentation
2. Search GitHub issues
3. Create new issue with details
4. Include logs and error messages

---

**Last Updated**: 2025-10-03  
**Version**: 1.0.0  
**API Endpoints**: 46  
**WebSocket Events**: 9

