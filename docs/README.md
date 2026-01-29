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
- [WebSocket Events](api/websocket-events.md) - Real-time event streaming documentation
- [WebSocket Event Structure](api/websocket-structure.md) - Technical event structure details
- [Prometheus Metrics](api/prometheus.md) - Metrics endpoint documentation

### Integrations
- [Model Context Protocol (MCP)](integrations/mcp.md) - AI agent integration guide
- [MQTT Integration](integrations/mqtt.md) - MQTT broker integration
- [Grafana Dashboard](integrations/grafana.md) - Monitoring with Grafana
- [Home Assistant](integrations/home-assistant.md) - Home automation integration

### Development
- [Contributing Guide](development/contributing.md) - How to contribute to the project
- [Development Setup](development/setup.md) - Set up your development environment
- [Code Quality Standards](development/code-quality.md) - Pre-commit hooks and standards
- [Testing Guide](development/testing.md) - Writing and running tests
- [Architecture Overview](development/architecture.md) - System design and architecture

### Troubleshooting
- [Diagnostic Commands](troubleshooting/diagnostics.md) - Commands for diagnosing issues
- [Common Issues](troubleshooting/common-issues.md) - Solutions to frequently encountered problems
- [Hardware Compatibility](troubleshooting/hardware-compatibility.md) - Hardware-specific guidance

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

## 📝 Documentation Standards

All documentation follows these standards:

- **Clear Headers**: Use descriptive section headers
- **Code Examples**: Include practical, working examples
- **Links**: Use relative links within documentation
- **Formatting**: Follow Markdown best practices
- **Up-to-date**: Keep version numbers and features current

## 💬 Need Help?

- **Issues**: [GitHub Issues](https://github.com/ruaan-deysel/unraid-management-agent/issues)
- **Discussions**: [GitHub Discussions](https://github.com/ruaan-deysel/unraid-management-agent/discussions)
- **Community**: Unraid Community Forums

---

**Version**: 2025.11.0  
**Last Updated**: January 2026
