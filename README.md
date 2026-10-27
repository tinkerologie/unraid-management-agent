# Unraid Management Agent

[![CI](https://github.com/tinkerologie/unraid-management-agent/actions/workflows/ci.yml/badge.svg)](https://github.com/tinkerologie/unraid-management-agent/actions/workflows/ci.yml)
[![Latest release](https://img.shields.io/github/v/release/tinkerologie/unraid-management-agent?label=release)](https://github.com/tinkerologie/unraid-management-agent/releases/latest)
[![Codecov](https://codecov.io/gh/tinkerologie/unraid-management-agent/branch/main/graph/badge.svg)](https://codecov.io/gh/tinkerologie/unraid-management-agent)
[![License: MIT](https://img.shields.io/github/license/tinkerologie/unraid-management-agent)](LICENSE)

Monitor and control your Unraid server over a REST API, WebSocket events, Prometheus metrics, MQTT and
the Model Context Protocol (MCP), from a single lightweight Go plugin.

The agent reads system state directly from Unraid, `/proc`, `/sys`, the Docker Engine API and libvirt,
caches it, and serves it to dashboards, Home Assistant, monitoring stacks and AI assistants. It can also
act: start and stop containers, VMs, the array and parity checks, spin disks, and more.

> [!NOTE]
> This is a community plugin, not an official Lime Technology product. Unraid 7.2 and later ship an
> official GraphQL API; this agent is an independent REST, WebSocket, MQTT and MCP alternative. The two
> run side by side without conflict.

## Contents

- [Features](#features)
- [Requirements](#requirements)
- [Installation](#installation)
- [Quick start](#quick-start)
- [Security and privacy](#security-and-privacy)
- [Configuration](#configuration)
- [Integrations](#integrations)
- [Documentation](#documentation)
- [Troubleshooting and support](#troubleshooting-and-support)
- [Development](#development)
- [Contributing](#contributing)
- [License](#license)

## Features

**Monitoring**

- System: CPU, memory, temperatures, fans (with their labels), uptime and hardware (DMI) details
- Array and parity: array state, capacity, parity status, schedules and check history
- Disks: per-disk usage, temperatures, SMART health and spin state, without waking spun-down disks
- Docker containers and VMs: state, resource use and image update checks (Docker Engine SDK and libvirt)
- Shares, unassigned devices, network interfaces, notifications, mover, plugins and OS updates
- ZFS: pools, datasets, snapshots, ARC, scrub progress and the SMART health of pool member drives
- UPS (apcupsd or NUT): charge, load, runtime, power, input/output/battery voltage, line frequency,
  transfers and self-test result
- GPUs (Intel, AMD, NVIDIA), network cards with SFP/QSFP optic diagnostics, and SAS storage topology
  (controllers, enclosures, drive paths)

**Hardware auto-detection**: hardware-dependent collectors (network cards and optics, SAS storage
topology, GPU, UPS, NUT and ZFS) run only when the hardware is present, and the settings page hides
what your server does not have. See [Hardware Auto-Detection](docs/guides/configuration.md#hardware-auto-detection).

**Control**

- Docker containers: start, stop, restart, pause, unpause and update
- VMs: start, stop, restart, pause, resume, hibernate, force stop, snapshots and clones
- Array and parity checks, disk spin up and down, User Scripts, notifications, fan control and CPU tuning
- Server reboot and shutdown

**Operations**

- Alert rules and scheduled health checks with notifications
- One-click self-test and a redacted diagnostics bundle for bug reports
- mDNS (zeroconf) advertising, so clients such as Home Assistant find the agent on your network
- An optional, opt-in embedded AI operator ([Agent Core](docs/integrations/agent.md), beta)

## Requirements

- Unraid OS 7.2 or later (amd64)
- TCP port 8043 free on the server (configurable)
- No other plugins. Optional features use what is already installed: apcupsd or the NUT plugin for UPS
  data, `storcli` or `sg_ses` for SAS topology, and the GPU drivers for GPU metrics.

See [System Requirements](docs/guides/system-requirements.md) for details.

## Installation

**Community Applications (recommended):** in the Unraid web UI, open **Apps**, search for
**Unraid Management Agent** and click **Install**.

**Plugin URL:** open **Plugins → Install Plugin** and paste:

```text
https://raw.githubusercontent.com/tinkerologie/unraid-management-agent/main/unraid-management-agent.plg
```

The service starts on port 8043. Open **Settings → Unraid Management Agent** to configure it. See the
[Installation Guide](docs/guides/installation.md) for upgrades and removal.

## Quick start

Replace `tower.local` with your server's address.

1. Check that the agent is up (this endpoint never needs a token):

   ```bash
   curl http://tower.local:8043/api/v1/health
   ```

2. Generate an **API token** on the settings page and click **Apply**. Read it into a variable with a
   silent prompt (paste it, then press Enter), so it stays out of your shell history:

   ```bash
   read -rs UMA_TOKEN && export UMA_TOKEN
   ```

3. Read data and run an action:

   ```bash
   curl -H "Authorization: Bearer $UMA_TOKEN" http://tower.local:8043/api/v1/system
   curl -H "Authorization: Bearer $UMA_TOKEN" http://tower.local:8043/api/v1/docker
   curl -X POST -H "Authorization: Bearer $UMA_TOKEN" http://tower.local:8043/api/v1/docker/nginx/restart
   ```

4. Explore every endpoint in the interactive API docs at `http://tower.local:8043/swagger/`.

| Interface        | Address                            |
| ---------------- | ---------------------------------- |
| REST API         | `http://tower.local:8043/api/v1`   |
| WebSocket events | `ws://tower.local:8043/api/v1/ws`  |
| Prometheus       | `http://tower.local:8043/metrics`  |
| MCP              | `http://tower.local:8043/mcp`      |
| Swagger UI       | `http://tower.local:8043/swagger/` |

More examples: [Quick Start Guide](docs/guides/quick-start.md).

## Security and privacy

> [!WARNING]
> The API listens on all interfaces on port 8043 over plain HTTP. Set an API token, and enable HTTPS
> before the token crosses a network you do not trust: over plain HTTP anyone on the path can read it.
> Never forward the port to the internet.

- **Authentication.** Without a token, anyone on your network can read monitoring data, and on a
  fresh install every control action (REST and MCP) is refused. With a token, every request must send
  `Authorization: Bearer <token>`, except `/api/v1/health`, the Swagger UI and the optional MCP
  connect URL (`/mcp/<secret>`), where the secret in the URL is the credential. Treat that URL like a
  password. Installs upgraded from a version without a token keep control actions working
  (`ALLOW_UNAUTHENTICATED_WRITES`) until you set one.
- **What access allows.** The REST API and MCP can reboot or shut down the server, control containers,
  VMs and the array, spin disks and run User Scripts as `/bin/bash`. MCP read-only mode and per-tool
  policies restrict MCP clients only, not the REST API.
- **Encryption.** Serve HTTPS by setting a certificate and key (`TLS_CERT_FILE`, `TLS_KEY_FILE`).
- **WAN IP lookup.** Only when you request network access URLs, the agent asks `api.ipify.org`,
  `ifconfig.me` and `icanhazip.com` for your public IP. Turn **WAN IP Lookup** off to make no external
  requests.

Details: [Configuration → Security](docs/guides/configuration.md#security) and [SECURITY.md](SECURITY.md).

## Configuration

Most settings live on the plugin settings page (**Settings → Unraid Management Agent**) and are saved
to `/boot/config/plugins/unraid-management-agent/config.cfg`. Clicking **Apply** restarts the service.

| Setting                  | Purpose                                                             |
| ------------------------ | ------------------------------------------------------------------- |
| Port and bind address    | Where the API listens (default `8043` on all interfaces)            |
| API token                | Requires a bearer token and enables control actions                 |
| Read-only mode           | Blocks every state-changing MCP tool                                |
| MCP tool policies        | Hide, allow or restrict individual MCP tools                        |
| MQTT                     | Broker connection and Home Assistant MQTT discovery                 |
| Collection intervals     | How often each collector runs; **Auto** for hardware-dependent ones |
| Show all collectors      | Reveals settings rows for hardware the agent did not detect         |
| WAN IP lookup, log level | Privacy and logging                                                 |

Longer intervals use less power. Every option, the `config.yml` file and command-line flags are covered
in the [Configuration Guide](docs/guides/configuration.md).

## Integrations

| Integration                          | How                                                                                                                                                                                 |
| ------------------------------------ | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Home Assistant                       | The [Home Assistant integration](https://github.com/tinkerologie/ha-unraid-management-agent) (HACS, with dashboard cards), or [MQTT discovery](docs/integrations/home-assistant.md) |
| Prometheus and Grafana               | Scrape `/metrics`; see [Prometheus metrics](docs/api/prometheus.md) and [Grafana](docs/integrations/grafana.md)                                                                     |
| Claude, Cursor and other MCP clients | Connect to `/mcp`; see [MCP](docs/integrations/mcp.md) and the [Agent Skill](docs/integrations/claude/README.md)                                                                    |
| ChatGPT                              | [Custom GPT Actions](docs/integrations/chatgpt/README.md) over the REST API                                                                                                         |
| MQTT                                 | Publish state to any broker; see [MQTT](docs/integrations/mqtt.md)                                                                                                                  |

## Documentation

| Topic                                               | What it covers                                       |
| --------------------------------------------------- | ---------------------------------------------------- |
| [Documentation index](docs/README.md)               | Every guide and reference                            |
| [Configuration Guide](docs/guides/configuration.md) | Settings, intervals, hardware detection, security    |
| [REST API](docs/api/rest-api.md)                    | Endpoints, parameters and response examples          |
| [WebSocket events](docs/api/websocket-events.md)    | Real-time event types and payloads                   |
| [Diagnostics API](docs/api/diagnostics.md)          | Self-test, detected capabilities, diagnostics bundle |
| [Integrations](docs/integrations/README.md)         | MCP, Claude, ChatGPT, Home Assistant, MQTT, Grafana  |
| [CHANGELOG](CHANGELOG.md)                           | What changed in each release                         |

## Troubleshooting and support

- Logs: `/var/log/unraid-management-agent.log`. Turn on **Debug Logging** in the settings for more
  detail.
- **Run Self-Test** on the settings page checks every data source; **Download Diagnostics** creates a
  redacted bundle to attach to a bug report.
- See [Diagnostic Commands](docs/troubleshooting/diagnostics.md) for command-line checks.
- Ask on the [Unraid forum support thread](https://forums.unraid.net/topic/196675-plugin-unraid-management-agent)
  or [open an issue](https://github.com/tinkerologie/unraid-management-agent/issues/new/choose).

## Development

Requires Go (the version in [`go.mod`](go.mod)) and `make`.

```bash
git clone https://github.com/tinkerologie/unraid-management-agent.git
cd unraid-management-agent
make deps            # install dependencies and tools
make local           # build for this machine
make test            # run the tests with the race detector
make pre-commit-run  # lint, format and security checks
make package         # build the Unraid plugin bundle
make test-packaging  # bundle, migration, checksum and release-note regression tests (uses sudo)
```

Packaging tests include exact changelog extraction (`scripts/test-release-notes.sh`) and safe manifest
`CHANGES` replacement (`scripts/test-plg-changes.sh`). Install optional `xmllint` locally for XML validation
(`brew install libxml2` on macOS, `apt-get install libxml2-utils` on Debian/Ubuntu); CI requires it.
Each release needs a non-empty `## [VERSION] - DATE` changelog section. Release automation generates the
manifest's current-version notes from that section and keeps the full history in `CHANGELOG.md`.

<details>
<summary>Deploy to an Unraid server for testing</summary>

The [Ansible playbook](ansible/README.md) builds the plugin, deploys it to a server and verifies the
endpoints:

```bash
cp ansible/inventory.yml.example ansible/inventory.yml   # set the server address and credentials
ansible-playbook -i ansible/inventory.yml ansible/deploy.yml --tags build,deploy,verify
```

</details>

Architecture, conventions and the AI-assistant workflow are described in [AGENTS.md](AGENTS.md).

## Contributing

Contributions are welcome, especially fixes for hardware the maintainer cannot test: disk controllers,
GPUs, UPS models and network cards. Every change starts with an issue, and pull requests follow the
template and pass the quality checks. Read [CONTRIBUTING.md](CONTRIBUTING.md) before you start.

When you report a hardware problem, include your Unraid version, the hardware involved and a
diagnostics bundle from the settings page.

## License

[MIT](LICENSE)
