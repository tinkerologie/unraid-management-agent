# Documentation

These guides describe how to install, configure, and use the Unraid Management Agent.

## Setup

- [System requirements](guides/system-requirements.md)
- [Installation and removal](guides/installation.md)
- [Quick start](guides/quick-start.md)
- [Configuration](guides/configuration.md)

## API reference

- [REST API](api/rest-api.md)
- [Diagnostics API](api/diagnostics.md)
- [WebSocket events](api/websocket-events.md)
- [WebSocket event structure](api/websocket-structure.md)
- [Prometheus metrics](api/prometheus.md)

## Integrations

- [Integration index](integrations/README.md)
- [Model Context Protocol (MCP)](integrations/mcp.md)
- [ChatGPT through Secure MCP Tunnel](integrations/mcp.md#chatgpt-through-secure-mcp-tunnel)
- [ChatGPT Custom GPT Actions](integrations/chatgpt/README.md)
- [Home Assistant](integrations/home-assistant.md)
- [MQTT](integrations/mqtt.md)
- [Grafana](integrations/grafana.md)

## Troubleshooting

- [Diagnostic commands](troubleshooting/diagnostics.md)
- [Security policy](../SECURITY.md)
- [GitHub issues](https://github.com/tinkerologie/unraid-management-agent/issues)

## Development

- [Contribution guide](../CONTRIBUTING.md)
- [Build and test procedures](development/code-quality.md)
- [Architecture decisions](adr/README.md)
- [Project glossary](../GLOSSARY.md)
- [Changelog](../CHANGELOG.md)
- [License](../LICENSE)

## Endpoint addresses

These examples use the default port. The host name `tower.local` represents the Unraid server address.

| Interface     | Address                                 |
| ------------- | --------------------------------------- |
| REST API      | `http://tower.local:8043/api/v1`        |
| WebSocket     | `ws://tower.local:8043/api/v1/ws`       |
| Stateful MCP  | `http://tower.local:8043/mcp`           |
| Stateless MCP | `http://tower.local:8043/mcp-stateless` |
| Prometheus    | `http://tower.local:8043/metrics`       |
| Swagger UI    | `http://tower.local:8043/swagger/`      |

## Documentation Standards

Use [ASD-STE100 Issue 9](https://www.asd-ste100.org/STE_downloads.html) for maintained technical prose.

- Use each approved word with its approved meaning and part of speech.
- Use active voice. Start instructions with a command verb. Give one instruction per step.
- Put a condition before its instruction. Keep notes separate from instructions.
- Use at most 20 words per procedural sentence and 25 per descriptive sentence.
- Use the [project glossary](../GLOSSARY.md) for technical terms. For example, use _collector_, _source health_, and _hardware gating_ consistently.
- Keep exact interface labels, API and configuration identifiers, command examples, error messages, and product names.
- Preserve historical release notes, attributed excerpts, and required governance text.

### Issue 9 review scope

The [documentation review](development/documentation-review.md) lists the files, sources, and checks for #326.

| Area                        | Files                                                                              |
| --------------------------- | ---------------------------------------------------------------------------------- |
| Project overview and policy | `README.md`, `docs/README.md`, `CONTRIBUTING.md`, `SECURITY.md`                    |
| Setup and operation         | `docs/guides/*.md`, `docs/troubleshooting/diagnostics.md`                          |
| API and integration         | `docs/api/*.md`, `docs/integrations/*.md`, Claude and ChatGPT guides               |
| Domain and architecture     | `GLOSSARY.md`, `docs/adr/*.md`                                                     |
| Build and deployment        | `docs/development/code-quality.md`, `ansible/README.md`                            |
| Generated API reference     | `daemon/docs/swagger.go`, API handler annotations, and the generated Swagger files |

Keep dated design notes, issue forms, templates, and agent instructions in their original roles.
For changes to generated Swagger prose:

1. Edit the source annotations.
2. Run `make swagger`.
3. Compare the generated schema with the previous schema.
