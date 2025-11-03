# Unraid Management Agent

A Go-based plugin for Unraid that exposes comprehensive system monitoring and control via REST API and WebSockets.

## ⚠️ Important: Third-Party Plugin Notice

**This is a community-developed third-party plugin and is NOT an official Unraid product.**

### Relationship to Official Unraid API

- **Official Unraid API**: Unraid OS 7.2+ includes an official **GraphQL-based API** as part of the core operating system. This is the official API provided and supported by Lime Technology (the creators of Unraid).

- **This Plugin**: The Unraid Management Agent is a **separate, independent third-party plugin** that provides a **REST API and WebSocket interface** for system monitoring and control. It is developed and maintained by the community, not by Lime Technology.

### Key Differences

| Feature | Official Unraid API | This Plugin (Unraid Management Agent) |
|---------|---------------------|----------------------------------------|
| **Developer** | Lime Technology (Official) | Community (Third-Party) |
| **API Type** | GraphQL | REST API + WebSocket |
| **Availability** | Built into Unraid OS 7.2+ | Separate plugin installation required |
| **Support** | Official Unraid support | Community support |
| **Purpose** | Official system API | Alternative/complementary monitoring solution |

### When to Use This Plugin

You might choose this plugin if you:

- ✅ **Prefer REST API**: You want a traditional REST API instead of GraphQL
- ✅ **Need WebSocket Support**: You require real-time event streaming via WebSockets
- ✅ **Want Specific Features**: This plugin offers specific monitoring and control features tailored to community needs
- ✅ **Compatibility**: You need an API solution that works alongside or independently of the official API

### Coexistence with Official API

This plugin **can coexist** with the official Unraid API. They operate independently and do not conflict with each other. You can use both simultaneously if your use case requires it.

### Official Unraid API Documentation

For information about the official Unraid GraphQL API, please refer to:
- [Unraid Official Documentation](https://docs.unraid.net/)
- Unraid OS 7.2+ release notes and API documentation

---

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
- Unraid 7.x or later
- Go 1.21+ (for building from source)

### From Release Package

1. Download the latest release package:
   ```bash
   wget https://github.com/domalab/unraid-management-agent/releases/latest/unraid-management-agent-2025.11.0.tgz
   ```

2. Extract and install:
   ```bash
   tar xzf unraid-management-agent-2025.11.0.tgz -C /
   ```

3. Start the service:
   ```bash
   /usr/local/emhttp/plugins/unraid-management-agent/unraid-management-agent boot
   ```

### Building from Source

```bash
# Clone the repository
git clone https://github.com/domalab/unraid-management-agent.git
cd unraid-management-agent

# Install dependencies
make deps

# Build for Unraid (Linux/amd64)
make release

# Create plugin package
make package
```

## System Compatibility

**Important Notice:** This plugin was developed and tested on a specific Unraid system configuration. While we strive for broad compatibility, **there is a possibility that the plugin may not function correctly on all hardware configurations** due to variations in:

- **CPU Architectures**: Different CPU models, instruction sets, and architectures (Intel vs AMD, different generations)
- **Disk Controllers and Storage Devices**: Various RAID controllers, HBA cards, SAS/SATA controllers, NVMe configurations
- **GPU Models and Drivers**: Different GPU vendors (NVIDIA, AMD, Intel), driver versions, and passthrough configurations
- **Network Interfaces**: Various network cards, bonding configurations, VLANs, and bridge setups
- **UPS Models and Monitoring Tools**: Different UPS brands, monitoring software (apcupsd, nut), and communication protocols
- **Docker and VM Configurations**: Different Docker versions, libvirt configurations, and virtualization setups

### What This Means for You

- ✅ **If it works on your system**: Great! The plugin should continue to work reliably.
- ⚠️ **If you encounter issues**: This is likely due to hardware/configuration differences. See the [Contributing](#contributing) section below for how you can help improve compatibility.
- 🔧 **Debugging across different hardware**: As a single maintainer, it's challenging to test and debug across all possible hardware configurations. Community contributions are essential for broader compatibility.

### Tested Configuration

This plugin has been developed and tested on the following configuration:

- **Unraid Version**: 7.x
- **Plugin Version**: 2025.11.0
- **Architecture**: Linux/amd64
- **Primary Testing**: REST API endpoints, WebSocket events, Docker/VM control operations

**Note:** This configuration represents the primary development and testing environment. Your mileage may vary on different hardware setups.

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

The agent uses log rotation with the following settings:

- **Location**: `/var/log/unraid-management-agent.log`
- **Max Size**: 5 MB per file
- **Backups**: None (only current log is kept)
- **Age-based Retention**: None (logs are rotated based on size only)

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

Contributions are welcome and greatly appreciated! This project benefits from community involvement, especially for improving hardware compatibility across different Unraid configurations.

### How to Contribute

#### General Contributions

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/your-feature-name`)
3. Commit your changes with descriptive messages
4. Add tests for new functionality
5. Ensure all tests pass: `make test`
6. Submit a pull request with a clear description of your changes

#### Hardware Compatibility Contributions

**Encountered compatibility issues on your system?** You can help improve the plugin for everyone!

If the plugin doesn't work correctly on your hardware configuration:

1. **Fork the Repository**: Create your own fork to work on fixes
2. **Identify the Issue**: Determine which component is failing (disk detection, GPU metrics, UPS monitoring, etc.)
3. **Make Necessary Changes**: Modify the code to support your hardware configuration
   - Update collectors in `daemon/services/collectors/` for data collection issues
   - Update parsers in `daemon/lib/` for command output parsing issues
   - Add fallback logic for different hardware variations
4. **Test Thoroughly**: Ensure your changes work on your system and don't break existing functionality
   - Run the full test suite: `make test`
   - Test all affected API endpoints
   - Verify WebSocket events are working correctly
5. **Document Your Changes**: In your pull request, include:
   - **Hardware Configuration**: CPU model, disk controllers, GPU model, UPS model, etc.
   - **Issue Description**: What wasn't working and why
   - **Solution Implemented**: How your changes fix the issue
   - **Testing Performed**: What you tested and the results
   - **Unraid Version**: Your Unraid version number

**Example PR Description:**
```
Hardware: AMD Ryzen 9 5950X, LSI 9300-8i HBA, NVIDIA RTX 3080
Issue: GPU temperature not detected due to different nvidia-smi output format
Solution: Added parsing for alternative nvidia-smi XML format
Testing: Verified GPU metrics endpoint returns correct data, all tests pass
Unraid Version: 7.2
```

### Why Community Contributions Matter

As a single maintainer, it's challenging to:
- Test across all possible hardware configurations
- Debug issues on systems I don't have access to
- Support every variation of disk controllers, GPUs, UPS models, etc.

**Your contributions help make this plugin work for everyone!** Even small fixes for specific hardware configurations are valuable and appreciated.

### Areas Where Contributions Are Especially Helpful

- 🔧 **Hardware-Specific Fixes**: Support for different disk controllers, GPU models, UPS brands
- 📊 **Data Collection Improvements**: Better parsing of system commands for different hardware
- 🧪 **Testing**: Testing on different Unraid versions and hardware configurations
- 📝 **Documentation**: Improving docs, adding examples, documenting edge cases
- 🐛 **Bug Fixes**: Fixing issues you encounter on your system
- ✨ **New Features**: Adding support for additional hardware or metrics

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
- Check existing documentation in the `docs/` directory
- Review the [Documentation Index](docs/README.md) for comprehensive guides

## Roadmap

### Recently Implemented ✅

The following features from the original roadmap have been successfully implemented:

- **Enhanced System Info Collector** (v2025.11.0)
  - CPU model detection and specifications
  - BIOS version and release date information
  - Per-core CPU usage monitoring
  - Server model identification

- **Detailed Disk Metrics** (v2025.11.0)
  - I/O statistics (read/write operations and bytes)
  - Per-disk performance metrics
  - IO utilization percentage
  - Disk spin state detection

### Planned Enhancements

The following features are planned for future releases:

- **User Management Collector**
  - User account listing and information
  - User permissions and group membership
  - Active user sessions monitoring

- **Network Statistics Trending**
  - Historical network bandwidth tracking
  - Time-series data for network metrics
  - Bandwidth usage trends and analysis

- **Alerting and Notification System**
  - Configurable alert rules and thresholds
  - Notification delivery (email, webhook, etc.)
  - Alert history and acknowledgment
  - Integration with monitoring platforms

- **Historical Data Storage**
  - Time-series database integration
  - Long-term metrics retention
  - Historical data querying and analysis
  - Data export capabilities

- **Plugin Management Endpoints**
  - Plugin installation and removal
  - Plugin listing and status
  - Plugin configuration management
  - Plugin update notifications

## Changelog

See [CHANGELOG.md](CHANGELOG.md) for detailed version history and release notes.
