# Unraid Management Agent - Development Container

This document describes the development container setup for the unraid-management-agent project.

## What's Included

### Base Tools

- **Go 1.25.0** - Go programming language (matches deployment target)
- **Node.js 20** - JavaScript runtime (for Copilot CLI)
- **GitHub CLI (gh)** - GitHub command-line interface
- **Git** - Version control
- **Make** - Build automation
- **Python 3** - Python runtime with pip

### Development Tools

- **pre-commit** - Git hooks framework for code quality checks
- **sshpass** - Non-interactive SSH password authentication (for Unraid deployment testing)
- **shellcheck** - Bash/shell script linter
- **yamllint** - YAML file linter
- **jq** - JSON processor
- **php-cli** - PHP command-line interface (for plugin page validation)

### MQTT Testing

- **Mosquitto MQTT Broker** - Full MQTT broker for testing (runs in separate container)
- **mosquitto-clients** - MQTT client tools (mosquitto_sub, mosquitto_pub)

### VS Code Extensions

- Go - Go language support
- GitHub Copilot - AI-powered coding assistance
- GitHub Copilot Chat - AI conversation interface
- Make Tools - Makefile support
- Prettier - Code formatter
- Pull Requests and Issues - GitHub integration
- GitHub Actions - Actions workflow support
- Remote Hub - GitHub remote browsing

## Getting Started

### Rebuild the Container

When you rebuild the container, all tools are automatically installed:

```bash
# If using VS Code Dev Containers or GitHub Codespaces:
# - Use Command Palette (Ctrl+Shift+P)
# - Select "Dev Containers: Rebuild Container"
# - Wait for the build to complete (2-3 minutes)

# OR manually rebuild:
docker-compose -f .devcontainer/docker-compose.yml build --no-cache
```

### MQTT Testing

The devcontainer includes a Mosquitto MQTT broker for testing MQTT functionality:

#### Start the MQTT Broker

```bash
# The broker starts automatically when you rebuild/restart the container
# To manually verify it's running:
docker ps | grep mqtt
```

#### Test MQTT Connectivity

```bash
# Check broker status
./scripts/mqtt-test.sh check

# Subscribe to all MQTT topics (press Ctrl+C to stop)
./scripts/mqtt-test.sh sub

# Monitor broker system stats
./scripts/mqtt-test.sh monitor

# Publish a test message
./scripts/mqtt-test.sh pub "test/topic" "hello world"
```

#### Connect the Agent to Local MQTT Broker

```bash
# Start the agent with MQTT enabled (pointing to local broker)
./unraid-management-agent boot \
  --debug \
  --port 8043 \
  --mqtt-enabled \
  --mqtt-broker tcp://mosquitto:1883 \
  --mqtt-port 1883
```

#### Verify MQTT Messages

In another terminal, subscribe to MQTT topics:

```bash
# Watch system updates
mosquitto_sub -h mosquitto -t "unraid/system_update" -v

# Watch all topics
mosquitto_sub -h mosquitto -t "unraid/#" -v
```

## Common Tasks

### Run Pre-commit Checks

```bash
# Run all hooks on all files
pre-commit run --all-files

# Run specific hook
pre-commit run go-fmt --all-files
```

### Build the Project

```bash
# Build for Linux/amd64 (Unraid)
make release

# Build for current architecture
make local

# Run all tests
make test
```

### Deploy to Unraid Server

```bash
# Set up SSH credentials in config.sh
cp scripts/config.sh.example scripts/config.sh
# Edit config.sh with your Unraid server details

# Deploy the plugin
./scripts/deploy-plugin.sh
```

### Check for Dependency Updates

```bash
# List available updates
go list -m -u all

# Update specific dependency
go get -u github.com/package/name

# Update all direct dependencies
go get -u ./...

# Clean up
go mod tidy
```

## Troubleshooting

### "sshpass not found"

This shouldn't happen if the container was rebuilt after the Dockerfile update. If you encounter this:

```bash
# Install manually
apt-get update && apt-get install -y sshpass
```

### "pre-commit not found"

This shouldn't happen if the container was rebuilt. If you encounter this:

```bash
# Install manually
pip3 install pre-commit
```

### "mosquitto_sub/mosquitto_pub not found"

If the MQTT client tools aren't available:

```bash
# Install manually
apt-get update && apt-get install -y mosquitto-clients
```

### MQTT Broker Connection Refused

Verify the broker is running:

```bash
# Check if mosquitto container is running
docker ps | grep mosquitto

# View broker logs
docker logs unraid-mqtt-broker

# Manually start the broker (if stopped)
docker-compose -f .devcontainer/docker-compose.yml up mosquitto -d
```

### MQTT Broker Port 1883 Already in Use

If port 1883 is already in use on your system:

1. Edit `.devcontainer/docker-compose.yml`
2. Change the port mapping: `"1884:1883"` instead of `"1883:1883"`
3. Rebuild the container
4. Connect with: `mosquitto_sub -h localhost -p 1884 -t "unraid/#"`

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│ Development Container (unraid-management-agent-dev)         │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│ Tools: Go, Node.js, Python, Git, Make                       │
│ Linters: golangci-lint, shellcheck, yamllint               │
│ Git Hooks: pre-commit framework                             │
│ SSH: sshpass for remote Unraid deployment                   │
│                                                              │
│ Network: unraid-dev-network                                 │
└────────────────────────┬────────────────────────────────────┘
                         │
        ┌────────────────┴────────────────┐
        │                                 │
        ▼                                 ▼
┌───────────────────┐          ┌──────────────────────┐
│ MQTT Broker       │          │ Unraid Server        │
│ (mosquitto:1883)  │          │ (192.168.20.21:8043) │
│                   │          │                      │
│ - System broker   │          │ Running agent        │
│ - Data logging    │          │ with MQTT enabled    │
│ - Testing topics  │          │                      │
└───────────────────┘          └──────────────────────┘
```

## Environment Variables

### In devcontainer.json

- `GOOS=linux` - Target operating system (always Linux for Unraid)
- `GOARCH=amd64` - Target architecture (always amd64)
- `GOPATH=/go` - Go workspace directory
- `GOBIN=/go/bin` - Go binaries installation directory

### For MQTT Testing (scripts/mqtt-test.sh)

- `MQTT_BROKER` - Broker hostname (default: `mosquitto`)
- `MQTT_PORT` - Broker port (default: `1883`)
- `MQTT_TOPIC` - Topic pattern to test (default: `unraid/#`)

## References

- [devcontainer.json documentation](https://containers.dev/implementers/json_reference/)
- [Docker Compose documentation](https://docs.docker.com/compose/)
- [Mosquitto MQTT Broker](https://mosquitto.org/)
- [Eclipse Paho MQTT Go Client](https://github.com/eclipse/paho.mqtt.golang)
- [unraid-management-agent MCP Integration](../docs/MCP_INTEGRATION.md)
