# Unraid Management Agent

A Go-based plugin for Unraid that exposes comprehensive system monitoring and control via REST API and WebSockets.

## Features

### Real-time Monitoring
- **System Information**: CPU usage, RAM, temperatures, uptime, hostname
- **Array Status**: Array state, parity status, disk counts
- **Disk Information**: Per-disk metrics, SMART data, temperatures, space usage
- **Network Interfaces**: Interface status, bandwidth, IP addresses, MAC addresses
- **Docker Containers**: Container list, status, resource usage
- **Virtual Machines**: VM list, state, resource allocation
- **UPS Status**: Battery level, runtime, power state
- **GPU Metrics**: GPU utilization, memory, temperature
- **User Shares**: Share list, space usage, paths

### Control Operations
- **Docker**: Start, stop, restart, pause, unpause containers
- **Virtual Machines**: Start, stop, restart, pause, resume, hibernate VMs

### Communication Protocols
- **REST API**: HTTP endpoints for synchronous queries
- **WebSocket**: Real-time event streaming for live updates

## Architecture

### Event-Driven Design
The agent uses a pubsub event bus for decoupled, real-time data flow:

```
Collectors → Event Bus → API Server Cache → REST Endpoints
                        ↓
                 WebSocket Hub → Connected Clients
```

### Components

#### Collectors
Data collectors run independently at fixed intervals:
- **System Collector** (5s): CPU, RAM, temps, uptime
- **Array Collector** (10s): Array state and parity info
- **Disk Collector** (30s): Per-disk metrics
- **Network Collector** (15s): Interface status and statistics
- **Docker Collector** (10s): Container information
- **VM Collector** (10s): Virtual machine data
- **UPS Collector** (10s): UPS status
- **GPU Collector** (10s): GPU metrics
- **Share Collector** (60s): User share information

#### API Server
- Maintains in-memory cache of latest collector data
- Serves REST endpoints for instant responses
- Broadcasts events to WebSocket clients
- Implements CORS, logging, and recovery middleware

#### Orchestrator
Coordinates the entire application lifecycle:
- Initializes all collectors
- Starts API server subscriptions before collectors
- Manages graceful shutdown

## Installation

### Prerequisites
- Unraid 6.x or later
- Go 1.21+ (for building from source)

### From Release Package

1. Download the latest release package:
   ```bash
   wget https://github.com/ruaandeysel/unraid-management-agent/releases/latest/unraid-management-agent-2025.10.03.tgz
   ```

2. Extract and install:
   ```bash
   tar xzf unraid-management-agent-2025.10.03.tgz -C /
   ```

3. Start the service:
   ```bash
   /usr/local/emhttp/plugins/unraid-management-agent/unraid-management-agent boot
   ```

### Building from Source

```bash
# Clone the repository
git clone https://github.com/ruaandeysel/unraid-management-agent.git
cd unraid-management-agent

# Install dependencies
make deps

# Build for Unraid (Linux/amd64)
make release

# Create plugin package
make package
```

## Usage

### Starting the Agent

```bash
# Standard mode
./unraid-management-agent boot

# Debug mode (stdout logging)
./unraid-management-agent boot --debug

# Custom port
./unraid-management-agent boot --port 8043
```

### REST API Endpoints

Base URL: `http://localhost:8043/api/v1`

#### Monitoring Endpoints
- `GET /health` - Health check
- `GET /system` - System information
- `GET /array` - Array status
- `GET /disks` - List all disks
- `GET /disks/{id}` - Get specific disk info
- `GET /network` - Network interface list
- `GET /shares` - List user shares
- `GET /docker` - List Docker containers
- `GET /docker/{id}` - Get container details
- `GET /vm` - List virtual machines
- `GET /vm/{id}` - Get VM details
- `GET /ups` - UPS status
- `GET /gpu` - GPU metrics

#### Control Endpoints
- `POST /docker/{id}/start` - Start container
- `POST /docker/{id}/stop` - Stop container
- `POST /docker/{id}/restart` - Restart container
- `POST /docker/{id}/pause` - Pause container
- `POST /docker/{id}/unpause` - Unpause container
- `POST /vm/{id}/start` - Start VM
- `POST /vm/{id}/stop` - Stop VM
- `POST /vm/{id}/restart` - Restart VM
- `POST /vm/{id}/pause` - Pause VM
- `POST /vm/{id}/resume` - Resume VM
- `POST /vm/{id}/hibernate` - Hibernate VM
- `POST /vm/{id}/force-stop` - Force stop VM

### WebSocket Connection

Connect to `ws://localhost:8043/api/v1/ws` to receive real-time events:

```javascript
const ws = new WebSocket('ws://localhost:8043/api/v1/ws');

ws.onmessage = (event) => {
  const data = JSON.parse(event.data);
  console.log('Event received:', data);
};
```

### Example API Usage

```bash
# Get system information
curl http://localhost:8043/api/v1/system

# Get network interfaces
curl http://localhost:8043/api/v1/network

# List all disks
curl http://localhost:8043/api/v1/disks

# Start a Docker container
curl -X POST http://localhost:8043/api/v1/docker/nginx/start

# Stop a VM
curl -X POST http://localhost:8043/api/v1/vm/Ubuntu/stop
```

## Development

### Project Structure

```
daemon/
├── cmd/              # CLI commands
├── common/           # Constants (intervals, paths)
├── domain/           # Core types (Context, Config)
├── dto/              # Data transfer objects
├── lib/              # Utilities (shell execution)
├── logger/           # Logging wrapper
└── services/
    ├── api/          # HTTP server, handlers, WebSocket
    ├── collectors/   # Data collection subsystems
    └── controllers/  # Control operations

meta/                 # Unraid plugin metadata
scripts/              # Test and deployment scripts
tests/                # Unit and integration tests
```

### Building and Testing

```bash
# Install dependencies
make deps

# Build for local development
make local

# Run tests
make test

# Generate coverage report
make test-coverage

# Run specific test
go test -v ./daemon/services/api/handlers_test.go

# Clean build artifacts
make clean
```

## Configuration

### Collection Intervals

Defined in `daemon/common/const.go`:

- System: 5 seconds
- Array: 10 seconds
- Disk: 30 seconds
- Network: 15 seconds
- Docker: 10 seconds
- VM: 10 seconds
- UPS: 10 seconds
- GPU: 10 seconds
- Shares: 60 seconds

### Logging

The agent uses structured logging with automatic log rotation:

- **Location**: `/var/log/unraid-management-agent.log`
- **Max Size**: 10 MB per file
- **Retention**: 10 backup files
- **Max Age**: 28 days

In debug mode (`--debug`), logs are written to stdout for immediate visibility.

## Troubleshooting

### No Data Returned

If endpoints return empty or default data:

1. Check that the agent is running: `ps aux | grep unraid-management-agent`
2. Review logs: `tail -f /var/log/unraid-management-agent.log`
3. Verify collection intervals haven't expired
4. Ensure proper permissions for system file access

### Collectors Not Running

1. Enable debug mode: `./unraid-management-agent boot --debug`
2. Check for panic recovery messages in logs
3. Verify event bus subscriptions are initialized before collectors start

### WebSocket Connection Issues

1. Verify WebSocket hub is running: check logs for "API server subscriptions started"
2. Test REST endpoints first to isolate API vs WebSocket issues
3. Check browser console for connection errors

## API Response Examples

### System Information
```json
{
  "hostname": "Tower",
  "version": "2025.10.03",
  "cpu_usage": 12.5,
  "ram_usage": 45.2,
  "temperature": 42.0,
  "uptime": 86400,
  "timestamp": "2025-10-02T08:00:00Z"
}
```

### Network Interfaces
```json
[
  {
    "name": "eth0",
    "mac_address": "00:11:22:33:44:55",
    "ip_address": "192.168.1.100",
    "speed_mbps": 1000,
    "state": "up",
    "bytes_received": 1234567890,
    "bytes_sent": 987654321,
    "packets_received": 5000000,
    "packets_sent": 4500000,
    "errors_received": 0,
    "errors_sent": 0,
    "timestamp": "2025-10-02T08:00:00Z"
  }
]
```

### Array Status
```json
{
  "state": "STARTED",
  "num_disks": 8,
  "num_data_disks": 6,
  "num_parity_disks": 2,
  "sync_percent": 100.0,
  "parity_valid": true,
  "timestamp": "2025-10-02T08:00:00Z"
}
```

## Contributing

Contributions are welcome! Please:

1. Fork the repository
2. Create a feature branch
3. Commit your changes with descriptive messages
4. Add tests for new functionality
5. Ensure all tests pass: `make test`
6. Submit a pull request

## License

MIT License - see LICENSE file for details

## Documentation

Comprehensive documentation is available in the `docs/` directory:

- **[Documentation Index](docs/README.md)** - Complete documentation overview
- **[API Reference](docs/api/API_REFERENCE.md)** - Detailed API endpoint reference (46 endpoints)
- **[API Coverage Analysis](docs/api/API_COVERAGE_ANALYSIS.md)** - API coverage vs Unraid Web UI
- **[WebSocket Events](docs/WEBSOCKET_EVENTS_DOCUMENTATION.md)** - WebSocket event system guide
- **[Changelog](CHANGELOG.md)** - Version history and release notes

## Support

For issues, questions, or feature requests:
- Open an issue on GitHub
- Check existing documentation in the `docs/` directory
- Review the [Documentation Index](docs/README.md) for comprehensive guides

## Roadmap

### Planned Enhancements
- Enhanced system info collector (CPU model, BIOS info, per-core usage)
- Detailed disk metrics (I/O statistics, per-disk performance metrics)
- User management collector
- Network statistics trending
- Alerting and notification system
- Historical data storage
- Plugin management endpoints

## Changelog

See [CHANGELOG.md](CHANGELOG.md) for detailed version history and release notes.
