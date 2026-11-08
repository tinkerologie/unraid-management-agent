# Documentation

Welcome to the Unraid Management Agent documentation. This directory contains comprehensive guides for installation, API usage, integration, and development.

## 📚 Table of Contents

### Getting Started

- [System Requirements & Dependencies](guides/system-requirements.md) - Prerequisites and compatibility information
- [Installation Guide](guides/installation.md) - Step-by-step installation instructions
- [Quick Start](guides/quick-start.md) - Get up and running in minutes
- [Configuration](guides/configuration.md) - Configure the agent for your needs

### API Documentation

- [REST API Reference](api/rest-api.md) - Complete REST API endpoint documentation
- [Diagnostics API Reference](api/diagnostics.md) - Diagnostic bundles, self-test and detected capabilities endpoints
- [WebSocket Events](api/websocket-events.md) - Real-time event streaming documentation
- [WebSocket Event Structure](api/websocket-structure.md) - Technical event structure details
- [Prometheus Metrics](api/prometheus.md) - Metrics endpoint documentation

### Integrations

- [Model Context Protocol (MCP)](integrations/mcp.md) - AI agent integration guide
- [MQTT Integration](integrations/mqtt.md) - MQTT broker integration
- [Grafana Dashboard](integrations/grafana.md) - Monitoring with Grafana
- [Home Assistant](integrations/home-assistant.md) - Home automation integration

### Development

- [Contributing Guide](../CONTRIBUTING.md) - Contribution steps and project checks
- [Code Quality Standards](development/code-quality.md) - Setup, tests, and packaging
- [Architecture Decisions](adr/README.md) - Decisions and links to the source
- [Glossary](../GLOSSARY.md) - Project terms

### Troubleshooting

- [Diagnostic Commands](troubleshooting/diagnostics.md) - Check the agent and prepare a report

### Reference

- [Changelog](../CHANGELOG.md) - Version history and release notes
- [Security Policy](../SECURITY.md) - Security practices and reporting
- [License](../LICENSE) - MIT License details

## 🚀 Quick Links

- **REST API Base URL**: `http://<unraid-ip>:8043/api/v1`
- **WebSocket Endpoint**: `ws://<unraid-ip>:8043/api/v1/ws`
- **MCP Endpoint**: `http://<unraid-ip>:8043/mcp`
- **Prometheus Metrics**: `http://<unraid-ip>:8043/metrics`

## 📖 Documentation Organization

This documentation is organized into the following sections:

### Getting Started

Essential information for new users including installation, configuration, and first steps.

### API Documentation

Complete technical reference for all API interfaces (REST, WebSocket, Prometheus).

### Integrations

Guides for connecting the agent with external systems (MQTT, Grafana, Home Assistant, AI agents).

### Development

Resources for developers contributing to or extending the project.

### Troubleshooting

Diagnostic tools and solutions for common problems.

## 🤝 Contributing to Documentation

Found an error or want to improve the docs? Contributions are welcome!

1. Edit the relevant markdown file
2. Follow the existing formatting style
3. Submit a pull request with your changes

## Documentation Standards

Use [ASD-STE100 Issue 9](https://www.asd-ste100.org/STE_downloads.html) for maintained technical prose.

- Use each approved word with its approved meaning and part of speech.
- Use active voice. Give one instruction per step. Put a condition before its instruction.
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
For generated Swagger prose, edit the source annotations. Run `make swagger`. Check the generated schema for contract changes.

## 💬 Need Help?

- **Issues**: [GitHub Issues](https://github.com/tinkerologie/unraid-management-agent/issues)
- **Discussions**: [GitHub Discussions](https://github.com/tinkerologie/unraid-management-agent/discussions)
- **Community**: Unraid Community Forums

---

**Version**: 2025.11.0  
**Last Updated**: February 2026
