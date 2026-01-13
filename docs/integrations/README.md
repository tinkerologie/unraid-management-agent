# Unraid Management Agent - Integrations

This directory contains integration guides and resources for connecting the Unraid Management Agent with third-party monitoring and visualization tools.

## Available Integrations

### Grafana

**[Grafana Integration Guide](./GRAFANA.md)** - Complete guide for integrating with Grafana for monitoring and dashboards.

**[Pre-built Dashboard](./unraid-system-monitor-dashboard.json)** - Production-ready Grafana dashboard JSON file.

#### Quick Import

To quickly get started with Grafana monitoring:

1. Download `unraid-system-monitor-dashboard.json`
2. In Grafana, go to **Dashboards** → **Import**
3. Upload the JSON file
4. Select your Infinity data source
5. Click **Import**

The dashboard includes 16 panels covering:

- System metrics (CPU, RAM, temperatures)
- Array status and capacity
- Disk information
- Docker container status
- Virtual machine status

#### Dashboard Features

- ✅ Uses correct API field names (`cpu_usage_percent`, `ram_usage_percent`, etc.)
- ✅ Proper thresholds and color coding
- ✅ 30-second refresh interval (configurable)
- ✅ Data source variable for easy configuration
- ✅ Production-ready with no additional setup required

For detailed setup instructions, see the [Grafana Integration Guide](./GRAFANA.md).

## Future Integrations

Additional integration guides will be added for:

- Prometheus/Alertmanager
- InfluxDB
- Home Assistant
- Datadog
- Other monitoring platforms

## Contributing

If you've created an integration with another platform, please consider contributing a guide!

1. Create a new markdown file in this directory (e.g., `PROMETHEUS.md`)
2. Follow the same structure as the Grafana guide
3. Include example configurations and screenshots
4. Submit a pull request

## Support

For questions or issues with integrations:

- **GitHub Issues**: [Report an issue](https://github.com/ruaan-deysel/unraid-management-agent/issues)
- **Documentation**: [Main README](../README.md)
- **API Reference**: [API Documentation](../api/API_REFERENCE.md)
