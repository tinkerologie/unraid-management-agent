# Unraid Management Agent

[![CI](https://github.com/tinkerologie/unraid-management-agent/actions/workflows/ci.yml/badge.svg)](https://github.com/tinkerologie/unraid-management-agent/actions/workflows/ci.yml)
[![Latest release](https://img.shields.io/github/v/release/tinkerologie/unraid-management-agent?label=release)](https://github.com/tinkerologie/unraid-management-agent/releases/latest)
[![Codecov](https://codecov.io/gh/tinkerologie/unraid-management-agent/branch/main/graph/badge.svg)](https://codecov.io/gh/tinkerologie/unraid-management-agent)
[![MIT license](https://img.shields.io/github/license/tinkerologie/unraid-management-agent)](LICENSE)

The Unraid Management Agent is a community plugin that monitors and controls an Unraid server.
It provides a REST API, WebSocket events, Prometheus metrics, MQTT, and Model Context Protocol (MCP) tools.
The plugin runs independently of the Unraid GraphQL API.

[Installation](#installation) · [Quick start](#quick-start) · [Security](#security-and-privacy) ·
[Integrations](#integrations) · [Documentation](#documentation)

## Requirements

- Unraid OS 7.2 or later on an x86-64 server.
- An available TCP port. The default port is `8043`.

Core monitoring requires no other plugins. Some collectors require specific hardware or an installed service.
See [System requirements](docs/guides/system-requirements.md).

## Installation

### Install from Community Applications

1. Open **Apps** in the Unraid web UI.
2. Search for **Unraid Management Agent**.
3. Click **Install**.

### Install from a URL

1. Open **Plugins → Install Plugin**.
2. Paste this URL:

   ```text
   https://raw.githubusercontent.com/tinkerologie/unraid-management-agent/main/unraid-management-agent.plg
   ```

3. Click **Install**.

For update and removal procedures, see the [installation guide](docs/guides/installation.md).

## Quick start

### Check the service

Replace `tower.local` with your server address in these examples. The health endpoint does not require an API token.

1. Send a request to the health endpoint:

   ```bash
   curl --fail --silent --show-error http://tower.local:8043/api/v1/health
   ```

2. Make sure that the response contains `"status":"ok"`.

The health response shows that the agent accepts HTTP requests. It does not report collector status.

### Set an API token

If the agent has an API token, use that token. The steps below apply to a new token.

1. Open **Settings → Utilities → Unraid Management Agent**.
2. Under **API Token**, click **Generate**.
3. Click **Copy**.
4. Click **Apply**.

### Read system information

> [!WARNING]
> Do not send an API token over an untrusted network without HTTPS.
> A device on the network can read the token in an HTTP request.

1. In a Bash terminal, run this command:

   ```bash
   read -rs UMA_TOKEN
   ```

2. Paste the token.
3. Press Enter.
4. Send the request:

   ```bash
   curl --fail --silent --show-error \
     -H "Authorization: Bearer ${UMA_TOKEN}" \
     http://tower.local:8043/api/v1/system
   ```

5. Remove the token from the shell variable:

   ```bash
   unset UMA_TOKEN
   ```

The [quick start guide](docs/guides/quick-start.md) gives more read and control examples.

## Features

| Area              | Functions                                                                                   |
| ----------------- | ------------------------------------------------------------------------------------------- |
| System            | CPU, memory, temperatures, fan sensors, uptime, and hardware information                    |
| Storage           | Array and parity status, disk usage and SMART health, shares, ZFS, and SAS storage topology |
| Docker and VMs    | Status, resource use, container image updates, and control actions                          |
| Network and power | Network interfaces, SFP and QSFP diagnostics, GPU metrics, and UPS status                   |
| Operations        | Alert rules, scheduled health checks, notifications, and diagnostics                        |
| Discovery         | mDNS records for network clients such as Home Assistant                                     |

The agent detects hardware for optional collectors. See [Hardware auto-detection](docs/guides/configuration.md#hardware-auto-detection).

Control actions include disk spin control, parity checks, User Scripts, fan control, and CPU tuning.
The agent can also reboot or shut down the server. [Agent Core](docs/integrations/agent.md) is an optional beta feature.

## Security and privacy

> [!WARNING]
> Do not forward the agent port to the internet.
> The agent listens on all interfaces over HTTP by default.

- **API token.** When you set a token, protected requests require `Authorization: Bearer <token>`.
- **Health and Swagger.** The health endpoint and Swagger UI do not require a token.
- **MCP connect URL.** The optional `/mcp/<connect-secret>` URL contains its credential. Keep that URL private.
- **Network write lock.** A new installation blocks network control without a token.
  Some upgrades keep **Allow Write Access Without a Token** enabled.
- **MCP tool access.** **Read-Only Mode** blocks state-changing MCP tools. Tool policies control access to individual tools.
  The REST API uses separate access rules.
- **HTTPS.** The settings `TLS_CERT_FILE` and `TLS_KEY_FILE` enable HTTPS.
- **WAN IP lookup.** The agent can ask external services for the public IP address when a client requests network access URLs.
  **WAN IP Lookup** controls these requests.

See [Security settings](docs/guides/configuration.md#security) and the [security policy](SECURITY.md).

## Configuration

The plugin stores its settings in `/boot/config/plugins/unraid-management-agent/config.cfg`.
**Apply** saves the settings and restarts a running service.

| Setting               | Purpose                                                              |
| --------------------- | -------------------------------------------------------------------- |
| Port and bind address | Set the HTTP port and listener address.                              |
| API token             | Require a bearer token for protected requests.                       |
| Collection mode       | Use automatic scheduling or set intervals manually.                  |
| Endpoints             | Enable or disable HTTP MCP, Prometheus metrics, or WebSocket events. |
| Debug Logging         | Add diagnostic information to the agent log.                         |

See the [configuration guide](docs/guides/configuration.md) for settings and defaults.

## Integrations

| Client or interface                   | Guide                                                                                               |
| ------------------------------------- | --------------------------------------------------------------------------------------------------- |
| Home Assistant                        | [Home Assistant integration and MQTT discovery](docs/integrations/home-assistant.md)                |
| Prometheus and Grafana                | [Prometheus metrics](docs/api/prometheus.md) and [Grafana](docs/integrations/grafana.md)            |
| Claude, Cursor, and other MCP clients | [Stateful MCP at `/mcp`](docs/integrations/mcp.md)                                                  |
| ChatGPT custom MCP server             | [Secure MCP Tunnel at `/mcp-stateless`](docs/integrations/mcp.md#chatgpt-through-secure-mcp-tunnel) |
| ChatGPT Custom GPT                    | [REST Actions](docs/integrations/chatgpt/README.md)                                                 |
| MQTT clients                          | [MQTT topics and setup](docs/integrations/mqtt.md)                                                  |

`/mcp` uses sessions and supports protocol versions through `2025-11-25`.
`/mcp-stateless` also supports `2026-07-28` discovery without a session.
Both routes use the same MCP tool policies.

## Documentation

- [Documentation index](docs/README.md)
- [REST API reference](docs/api/rest-api.md)
- [WebSocket events](docs/api/websocket-events.md)
- [Diagnostics API](docs/api/diagnostics.md)
- [Changelog](CHANGELOG.md)

The Swagger UI is at `/swagger/` on the agent HTTP port.

## Troubleshooting and support

The agent log is at `/var/log/unraid-management-agent.log`.
**Run Self-Test** on the settings page checks source health.
**Download Diagnostics** saves a redacted ZIP file.

1. Open **Settings → Utilities → Unraid Management Agent**.
2. Click **Run Self-Test**.
3. Click **Download Diagnostics**.
4. Examine the ZIP for private information.
5. Report the problem on the [Unraid forum](https://forums.unraid.net/topic/196675-plugin-unraid-management-agent)
   or [open a GitHub issue](https://github.com/tinkerologie/unraid-management-agent/issues/new/choose).

See [Diagnostic commands](docs/troubleshooting/diagnostics.md) for checks on the server.

## Development

A local build requires the Go version in [`go.mod`](go.mod) and `make`.

1. Clone the repository:

   ```bash
   git clone https://github.com/tinkerologie/unraid-management-agent.git
   ```

2. Change to the repository directory:

   ```bash
   cd unraid-management-agent
   ```

3. Build the agent:

   ```bash
   make local
   ```

4. Run the Go tests:

   ```bash
   make test
   ```

5. Run the repository checks:

   ```bash
   make pre-commit-run
   ```

The [development guide](docs/development/code-quality.md) describes packaging and test commands.
The [Ansible guide](ansible/README.md) describes deployment to an Unraid test server.

## Contributing

See the [contribution guide](CONTRIBUTING.md) for the issue and pull request process.
The [documentation standards](docs/README.md#documentation-standards) apply ASD-STE100 Issue 9 to maintained technical prose.

## License

[MIT](LICENSE)
