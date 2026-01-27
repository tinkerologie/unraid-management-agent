[![GitHub Release](https://img.shields.io/github/v/release/ruaan-deysel/unraid-management-agent?label=latest%20release)](https://github.com/ruaan-deysel/unraid-management-agent/releases/latest)
[![GitHub Last Commit](https://img.shields.io/github/last-commit/ruaan-deysel/unraid-management-agent)](https://github.com/ruaan-deysel/unraid-management-agent/commits/main)
[![GitHub go.mod Go version](https://img.shields.io/github/go-mod/go-version/ruaan-deysel/unraid-management-agent)](https://github.com/ruaan-deysel/unraid-management-agent)
[![GitHub issues](https://img.shields.io/github/issues/ruaan-deysel/unraid-management-agent)](https://github.com/ruaan-deysel/unraid-management-agent/issues)
[![License](https://img.shields.io/github/license/ruaan-deysel/unraid-management-agent)](./LICENSE)
[![Ask DeepWiki](https://deepwiki.com/badge.svg)](https://deepwiki.com/ruaan-deysel/unraid-management-agent)

# Unraid Management Agent

A Go-based plugin for Unraid that exposes comprehensive system monitoring and control via REST API and WebSockets.

## ⚠️ Important: Third-Party Plugin Notice

**This is a community-developed third-party plugin and is NOT an official Unraid product.**

### Relationship to Official Unraid API

- **Official Unraid API**: Unraid OS 7.2+ includes an official **GraphQL-based API** as part of the core operating system. This is the official API provided and supported by Lime Technology (the creators of Unraid).

- **This Plugin**: The Unraid Management Agent is a **separate, independent third-party plugin** that provides a **REST API and WebSocket interface** for system monitoring and control. It is developed and maintained by the community, not by Lime Technology.

### Key Differences

| Feature          | Official Unraid API        | This Plugin (Unraid Management Agent)         |
| ---------------- | -------------------------- | --------------------------------------------- |
| **Developer**    | Lime Technology (Official) | Community (Third-Party)                       |
| **API Type**     | GraphQL                    | REST API + WebSocket                          |
| **Availability** | Built into Unraid OS 7.2+  | Separate plugin installation required         |
| **Support**      | Official Unraid support    | Community support                             |
| **Purpose**      | Official system API        | Alternative/complementary monitoring solution |

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
- **Docker Containers**: Container list, status, resource usage (via Docker SDK)
- **Virtual Machines**: VM list, state, resource allocation (via libvirt API)
- **UPS Status**: Battery level, runtime, power state
- **GPU Metrics**: GPU utilization, memory, temperature
- **User Shares**: Share list, space usage, paths
- **ZFS Pools/Datasets**: ZFS pool health, datasets, snapshots, ARC stats
- **Notifications**: System alerts, warnings, and info messages

### Control Operations

- **Docker**: Start, stop, restart, pause, unpause containers
- **Virtual Machines**: Start, stop, restart, pause, resume, hibernate, force-stop VMs
- **Array**: Start, stop array operations
- **Parity**: Start, stop, pause, resume parity checks
- **Disk**: Spin up, spin down individual disks
- **User Scripts**: Execute User Scripts plugin scripts

### Communication Protocols

- **REST API**: HTTP endpoints for synchronous queries
- **WebSocket**: Real-time event streaming for live updates
- **Prometheus**: Native `/metrics` endpoint for Grafana/monitoring integration (41 metrics)
- **MQTT**: Event publishing to MQTT brokers for IoT integration and Home Assistant
- **MCP (Model Context Protocol)**: AI agent integration for LLM-powered monitoring and control (54 tools)

## Architecture

### Event-Driven Design

The agent uses a pubsub event bus for decoupled, real-time data flow:

```
Collectors → Event Bus → API Server Cache → REST Endpoints
                        ↓                  ↓
                 WebSocket Hub      MCP Server → AI Agents
                        ↓
                 Connected Clients
```

### Native API Integration

For optimal performance, collectors use native Go libraries instead of shell commands:

| Component  | Library                              | Description                          |
| ---------- | ------------------------------------ | ------------------------------------ |
| **Docker** | `github.com/moby/moby/client`        | Docker Engine SDK for container data |
| **VMs**    | `github.com/digitalocean/go-libvirt` | Native libvirt bindings for VM data  |
| **System** | Direct `/proc`, `/sys` access        | Kernel interfaces for metrics        |

### Components

#### Collectors

Data collectors run at configurable intervals (defaults shown):

- **System Collector** (15s): CPU, RAM, temps, uptime
- **Array Collector** (30s): Array state and parity info
- **Disk Collector** (30s): Per-disk metrics
- **Network Collector** (30s): Interface status and statistics
- **Docker Collector** (30s): Container information
- **VM Collector** (30s): Virtual machine data
- **UPS Collector** (60s): UPS status
- **GPU Collector** (60s): GPU metrics
- **Share Collector** (60s): User share information
- **Hardware Collector** (5m): Hardware info (rarely changes)
- **Registration Collector** (5m): License info (rarely changes)

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

## System Requirements and Dependencies

**Important:** The Unraid Management Agent has **NO external plugin dependencies**. It collects data directly from system sources.

For detailed information, see:

- **[System Requirements & Dependencies](docs/SYSTEM_REQUIREMENTS_AND_DEPENDENCIES.md)** - Complete requirements and data collection methods
- **[Quick Reference](docs/QUICK_REFERENCE_DEPENDENCIES.md)** - TL;DR version
- **[Diagnostic Commands](docs/DIAGNOSTIC_COMMANDS.md)** - Troubleshooting guide

### Quick Prerequisites

- Unraid 6.9+ (tested on Unraid 7.x)
- Port 8043 available (configurable)
- No other plugins required

### Via Community Applications (Recommended)

**Coming Soon**: This plugin will be available in the Unraid Community Applications store.

For now, you can install manually using the plugin URL:

1. Open your Unraid Web UI
2. Navigate to **Plugins** → **Install Plugin**
3. Enter the plugin URL:

   ```
   https://raw.githubusercontent.com/ruaan-deysel/unraid-management-agent/main/unraid-management-agent.plg
   ```

4. Click **Install**
5. The plugin will automatically:
   - Download and extract the package
   - Create default configuration
   - Start the service on port 8043

### Manual Installation from Release Package

1. Download the latest release package:

   ```bash
   wget https://github.com/ruaan-deysel/unraid-management-agent/releases/download/v2025.11.0/unraid-management-agent-2025.11.0.tgz
   ```

2. Extract and install:

   ```bash
   tar xzf unraid-management-agent-2025.11.0.tgz -C /
   ```

3. Start the service:

   ```bash
   /usr/local/emhttp/plugins/unraid-management-agent/scripts/start
   ```

### Building from Source

```bash
# Clone the repository
git clone https://github.com/ruaan-deysel/unraid-management-agent.git
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
- **Docker and VM Configurations**: Different Docker API versions, libvirt socket configurations, and virtualization setups

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
- `GET /logs` - List log files or get log content
- `GET /logs/{filename}` - Get specific log file by name

#### Settings & Configuration Endpoints

- `GET /settings/disk-thresholds` - Global disk temperature warning/critical thresholds
- `GET /settings/mover` - Mover schedule, thresholds, and running status
- `GET /settings/services` - Docker and VM Manager enabled/disabled status
- `GET /settings/network-services` - Network services status (SMB, NFS, FTP, SSH, VPN, etc.)
- `GET /array/parity-check/schedule` - Parity check schedule configuration
- `GET /plugins` - List installed plugins with versions and update status
- `GET /updates` - OS and plugin update availability
- `GET /system/flash` - USB flash boot drive health statistics

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
const ws = new WebSocket("ws://localhost:8043/api/v1/ws");

ws.onmessage = (event) => {
  const data = JSON.parse(event.data);
  console.log("Event received:", data);
};
```

### Prometheus Metrics

The agent exposes **41 metrics** in Prometheus format at `/metrics`:

```bash
# Scrape metrics
curl http://localhost:8043/metrics
```

Available metrics include:

- **Array**: state, capacity, usage, parity status
- **CPU**: usage, temperature
- **Memory**: total, used, usage percentage
- **Disks**: size, free space, temperature, SMART status, standby state
- **Docker**: container counts, states
- **VMs**: VM counts, states
- **GPU**: utilization, temperature, memory, power
- **UPS**: battery charge, load, runtime, status
- **Shares**: share counts, usage
- **Services**: service states
- **Parity**: parity validity, check progress
- **System**: uptime, info labels

For Grafana integration, see [docs/integrations/GRAFANA.md](docs/integrations/GRAFANA.md).

### MQTT Publishing

Publish system events to MQTT brokers for IoT integration and Home Assistant:

```bash
# Enable MQTT publishing
./unraid-management-agent boot \
  --mqtt-enabled \
  --mqtt-broker "192.168.1.100" \
  --mqtt-port 1883 \
  --mqtt-topic-prefix "unraid/tower"

# With authentication
./unraid-management-agent boot \
  --mqtt-enabled \
  --mqtt-broker "mqtt.example.com" \
  --mqtt-port 1883 \
  --mqtt-username "unraid" \
  --mqtt-password "secret" \
  --mqtt-use-tls \
  --mqtt-topic-prefix "homelab/unraid"

# Home Assistant auto-discovery
./unraid-management-agent boot \
  --mqtt-enabled \
  --mqtt-broker "192.168.1.100" \
  --mqtt-home-assistant
```

**Published Events:**

- System metrics (CPU, RAM, temperatures)
- Array status and capacity
- Disk information and health
- Container and VM states
- Share usage
- UPS status
- GPU metrics
- Network statistics
- Notifications

**Configuration via REST API:**

```bash
# Check MQTT status
curl http://localhost:8043/api/v1/mqtt/status

# Test MQTT connection
curl -X POST http://localhost:8043/api/v1/mqtt/test

# Publish custom message
curl -X POST http://localhost:8043/api/v1/mqtt/publish \
  -H "Content-Type: application/json" \
  -d '{"topic":"custom/topic","payload":{"message":"hello"},"retained":false}'
```

### MCP (Model Context Protocol)

The agent includes an MCP endpoint for AI agent integration at `POST /mcp`:

```bash
# List available MCP tools
curl -X POST http://localhost:8043/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"tools/list","id":1}'

# Get system info via MCP
curl -X POST http://localhost:8043/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"tools/call","params":{"name":"get_system_info","arguments":{}},"id":1}'
```

For full MCP documentation including all 54 tools, 5 resources, and 3 prompts, see [docs/MCP_INTEGRATION.md](docs/MCP_INTEGRATION.md).

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

### Dev Container (VS Code)

- Prereqs: Docker (or compatible), VS Code with Dev Containers extension.
- Open the repo in VS Code and run `Dev Containers: Reopen in Container`.
- Builds image and container named `unraid-management-agent-dev` via `.devcontainer/docker-compose.yml`.
- Tooling baked in: Go 1.25, Node.js 20, GitHub CLI (`gh`), Copilot CLI, make, gcc, jq.
- VS Code extensions auto-installed: Go, Makefile Tools, Prettier, GitHub (PRs, Codespaces, Actions, Copilot, Copilot Chat, RemoteHub, theme), and Claude Dev.
- Post-create runs `go mod download`; run `make test` to verify after attach.

## Configuration

### Settings Page

Configure the plugin through the Unraid web UI:

1. Navigate to **Settings** → **Unraid Management Agent**
2. Adjust settings as needed:
   - **Port**: API server port (default: 8043)
   - **Collection Intervals**: How often each data type is collected

3. Click **Apply** to save changes (service restarts automatically)

### Collection Intervals

The settings page provides dropdown menus to configure how often data is collected. Default values are optimized for low power consumption:

| Category     | Collectors                                           | Default    |
| ------------ | ---------------------------------------------------- | ---------- |
| **Fast**     | System Metrics                                       | 15 seconds |
| **Standard** | Array, Disk, Docker, VM, Network, ZFS, Notifications | 30 seconds |
| **Moderate** | UPS, GPU, Shares, Unassigned Devices                 | 1 minute   |
| **Slow**     | Hardware Info, License Info                          | 5 minutes  |

**⚡ Power Note:** Lower intervals provide faster updates but increase CPU usage and power consumption. On Intel systems with many Docker containers, aggressive intervals (5-10s) can increase idle power by 15-20W.

### Advanced: Manual Configuration

For automation or headless setups, you can edit the config file directly:

```
/boot/config/plugins/unraid-management-agent/config.cfg
```

Changes require a service restart:

```bash
/usr/local/emhttp/plugins/unraid-management-agent/scripts/stop
/usr/local/emhttp/plugins/unraid-management-agent/scripts/start
```

### Logging

The agent uses log rotation with the following settings:

- **Location**: `/var/log/unraid-management-agent.log`
- **Max Size**: 5 MB per file
- **Backups**: 1 backup file (older backups are automatically deleted)
- **Age-based Retention**: 1 day (backup files older than 1 day are deleted)
- **Log Levels**: DEBUG, INFO, WARNING, ERROR (configurable via `--log-level` CLI flag)
- **Default Level**: INFO
- **Auto Cleanup**: On startup, old rotated log files from previous versions are automatically removed

In debug mode (`--debug` or `--log-level debug`), logs are written to stdout for immediate visibility.

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

### Code Quality Standards

This project enforces **zero tolerance** for code quality issues:

- ✅ No linting warnings or errors
- ✅ No security vulnerabilities (medium+ severity)
- ✅ All tests must pass with race detection
- ✅ Code must be properly formatted (gofmt/goimports)

We use **pre-commit hooks** to automatically enforce these standards.

#### Quick Setup

```bash
# Automated setup (recommended)
./scripts/setup-pre-commit.sh

# Or manual setup
pip install pre-commit
make pre-commit-install
```

Pre-commit will automatically run checks before each commit. See [docs/PRE_COMMIT_HOOKS.md](docs/PRE_COMMIT_HOOKS.md) for detailed documentation.

#### Available Commands

```bash
make pre-commit-run   # Run all pre-commit checks
make lint             # Run golangci-lint only
make security-check   # Run security scans
make test-coverage    # Run tests with coverage
```

### How to Contribute

#### General Contributions

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/your-feature-name`)
3. Commit your changes with descriptive messages
4. Add tests for new functionality
5. Ensure all checks pass: `make pre-commit-run && make test-coverage`
6. Submit a pull request with a clear description of your changes

**Important**: Pre-commit hooks will block commits that don't meet quality standards. Fix all issues before committing.

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

MIT License - see [LICENSE](LICENSE) file for details

## Changelog

See [CHANGELOG.md](CHANGELOG.md) for detailed version history and release notes.
