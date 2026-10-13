#### Unraid Management Agent

This plugin provides comprehensive system monitoring and control capabilities through REST API and WebSocket interfaces, enabling integration with Home Assistant and other automation platforms.

##### Security and privacy

- The API listens on all network interfaces on port 8043 over plain HTTP. Without an API token, anyone on your
  network can read its data and control actions are refused; upgrades from earlier versions keep them allowed
  until you set a token (once a token is set, every request must send it) ([Authentication](../../docs/guides/configuration.md#authentication)).
- With access, the API and MCP can reboot or shut down the server, control containers, VMs and the array, and run
  User Scripts. MCP read-only mode and tool policies do not restrict the REST API
  ([Core Settings](../../docs/guides/configuration.md#core-settings),
  [Network Security](../../docs/guides/configuration.md#network-security)).
- When you request network access URLs, the agent looks up your WAN IP from `api.ipify.org`, `ifconfig.me` and
  `icanhazip.com`. No setting disables this yet
  ([#252](https://github.com/tinkerologie/unraid-management-agent/issues/252),
  [Security](../../docs/guides/configuration.md#security)).
