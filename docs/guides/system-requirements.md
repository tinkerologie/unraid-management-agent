# System Requirements & Dependencies

## Overview

The agent uses Go libraries and Linux kernel interfaces to collect system data. Core monitoring needs no other Unraid plugins.

## Minimum Requirements

### Operating System

- **Unraid OS**: 7.2 or later
- **Architecture**: Linux/amd64 (x86-64-v1)

### Hardware

- **CPU**: Any x64 processor (Intel or AMD)
- **Memory and storage**: Allow space for the plugin files, logs, and collector data. Use varies with the system and active collectors.
- **Network**: 1 available TCP port (default: 8043)

### Tested Configurations

- **Primary Testing**: Unraid 7.x
- **Architecture**: Linux/amd64
- **Plugin Version**: 2025.11.0+

## Data Collection Methods

### Core System Data (No Dependencies)

The agent collects most data using direct system access:

| Data Source     | Method                          | Dependencies                  |
| --------------- | ------------------------------- | ----------------------------- |
| **CPU/RAM**     | `/proc/stat`, `/proc/meminfo`   | None (kernel)                 |
| **Disks**       | `/proc/diskstats`, `smartctl`   | smartmontools (pre-installed) |
| **Array**       | `/var/local/emhttp/var.ini`     | None (Unraid core)            |
| **Network**     | `/sys/class/net/*`, `ethtool`   | ethtool (pre-installed)       |
| **Temperature** | `sensors`, `/sys/class/hwmon/*` | lm-sensors (optional)         |

### Container & VM Data (Native APIs)

| Service    | API Method        | Library                              | Status             |
| ---------- | ----------------- | ------------------------------------ | ------------------ |
| **Docker** | Docker Engine API | `github.com/moby/moby/client`        | Built-in Go SDK    |
| **VMs**    | libvirt protocol  | `github.com/digitalocean/go-libvirt` | Native Go bindings |

### Optional Features

These features require additional Unraid plugins or hardware:

| Feature                | Requirement               | Notes                           |
| ---------------------- | ------------------------- | ------------------------------- |
| **GPU Metrics**        | NVIDIA GPU + drivers      | Uses `nvidia-smi` command       |
| **UPS Status**         | apcupsd or NUT plugin     | Reads UPS daemon status         |
| **ZFS**                | ZFS support enabled       | Uses `zfs` and `zpool` commands |
| **User Scripts**       | User Scripts plugin       | Runs scripts with the plugin    |
| **Unassigned Devices** | Unassigned Devices plugin | Reads plugin config files       |

## Port Requirements

### Default Port

- **API/WebSocket/MCP**: TCP 8043 (configurable)

### Port Conflicts

If port 8043 is in use:

1. Check running services: `netstat -tulpn | grep 8043`
2. Change port in plugin settings
3. Restart the agent service

## Permissions

The agent runs with standard Unraid plugin permissions and requires:

- **Read Access**: System files (`/proc`, `/sys`, `/var`)
- **Execute Access**: System commands (`smartctl`, `docker`, `ethtool`)
- **Socket Access**: Docker socket, libvirt socket (if VMs enabled)

## Network Configuration

### Firewall Considerations

- The agent listens on all interfaces by default. Set a bind address to limit access.
- Use firewall rules to restrict access to trusted clients.
- Set an API token for network control. Without a token, the agent permits reads and blocks network writes by default.

### Recommended Security

1. **Internal Network Only**: Do not expose to public internet
2. **VPN Access**: Use WireGuard/OpenVPN for remote access
3. **Reverse Proxy**: Use nginx/Traefik with auth if needed

## Performance Impact

### Resource Usage

Collection costs depend on the hardware and enabled collectors. Short Manual intervals can increase CPU and power use.
Dynamic mode adjusts the cadence from demand, change events, and alert floors.
See [Collection Mode](configuration.md#collection-mode) for the two modes.

## Hardware Compatibility

### Known Compatible Hardware

The plugin was developed and tested on:

- **CPU**: Modern Intel/AMD x64 processors
- **Storage**: Standard SATA, NVMe, SAS controllers
- **Network**: Most Ethernet adapters
- **GPU**: NVIDIA GPUs with drivers installed

### Potential Compatibility Issues

The plugin may not work correctly with:

- **Exotic disk controllers**: Uncommon RAID/HBA cards
- **Non-standard sensors**: Custom temperature monitoring
- **Unusual network configs**: Complex bonding/VLAN setups
- **Old hardware**: Pre-2010 systems may have parsing issues

### If You Encounter Issues

Hardware variations can cause compatibility problems. See:

- [Diagnostic Commands](../troubleshooting/diagnostics.md)
- [Contributing Guide](../../CONTRIBUTING.md) - Help improve compatibility

## Diagnostic Tools

To verify system compatibility:

```bash
# Check core utilities
which docker smartctl ethtool sensors

# Verify Docker API access
docker info

# Check libvirt socket (if VMs enabled)
ls -l /var/run/libvirt/libvirt-sock

# Test temperature sensors
sensors

# Check GPU detection
nvidia-smi
```

See [Diagnostic Commands](../troubleshooting/diagnostics.md) for complete troubleshooting guide.

## FAQ

**Q: Do I need to install any other plugins?**  
A: No. The agent has no plugin dependencies.

**Q: Does this conflict with the official Unraid API?**  
A: No. They can coexist. This plugin is a third-party alternative.

**Q: What happens if I don't have a GPU/UPS?**  
A: Those endpoints return empty/unavailable. No errors occur.

**Q: Can I disable certain collectors?**  
A: Yes. In Manual mode, set the collector interval to 0. In Dynamic mode, use `UNRAID_DISABLE_COLLECTORS` or the collector control API.

**Q: Does this work on ARM systems?**  
A: No. Currently only Linux/amd64 (x86_64) is supported.

## Next Steps

- [Installation Guide](installation.md) - Install the plugin
- [Configuration](configuration.md) - Configure settings
- [Quick Start](quick-start.md) - Get started quickly

---

**Last Updated**: January 2026  
**Plugin Version**: 2025.11.0+
