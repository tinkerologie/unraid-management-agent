# Unraid Management Agent - System Requirements and Dependencies

## 1. Minimum System Requirements

### Unraid OS Version
- **Minimum**: Unraid 6.9+
- **Recommended**: Unraid 7.x (tested and verified)
- **Architecture**: Linux/amd64 (x86-64)

### Hardware Requirements
- **CPU**: Any x86-64 processor (Intel or AMD)
  - No specific core count or generation required
  - Works with older CPUs (tested on Intel i7-6700K and i7-8700K)
- **RAM**: Minimal (< 50 MB for the agent itself)
  - No specific minimum; depends on your Unraid system
- **Storage**: ~5-10 MB for plugin installation
- **Network**: Port 8043 available (configurable)

### Hardware Dependencies
The agent does **NOT** require specific hardware. However, data availability depends on what hardware is present:

| Hardware | Data Availability | Notes |
|----------|-------------------|-------|
| **CPU** | Always available | CPU usage, cores, threads, model |
| **Temperature Sensors** | Optional | Requires hwmon sensors or `lm-sensors` |
| **Disks** | Always available | Requires `/var/local/emhttp/disks.ini` |
| **GPU** | Optional | Requires GPU drivers (nvidia-smi, rocm-smi, etc.) |
| **UPS** | Optional | Requires apcupsd or NUT daemon |
| **Docker** | Optional | Requires Docker installation |
| **VMs** | Optional | Requires libvirt/KVM |

---

## 2. Plugin Dependencies

### ✅ NO External Plugin Dependencies

**The Unraid Management Agent is completely independent and does NOT require any other Unraid plugins.**

Specifically:
- ❌ **Does NOT require** "Dynamix System Information" plugin
- ❌ **Does NOT require** "Dynamix System Temperature" plugin
- ❌ **Does NOT require** "GPU Statistics" plugin
- ❌ **Does NOT require** Intel/NVIDIA/AMD driver plugins

### Why No Dependencies?

The agent collects data **directly from system sources**:

1. **System files** (`/proc/`, `/sys/`)
2. **Unraid configuration files** (`/var/local/emhttp/`)
3. **System commands** (`sensors`, `smartctl`, `docker`, `virsh`, etc.)
4. **Hardware monitoring** (hwmon, DMI tables)

---

## 3. Data Collection Independence

### Direct Data Collection Methods

The agent collects data **independently** without relying on other plugins:

#### System Metrics (System Collector - 5s interval)
```
Data Source: /proc/stat, /proc/meminfo, /proc/uptime, /proc/cpuinfo
Methods:
  - CPU usage: Calculated from /proc/stat
  - RAM usage: Read from /proc/meminfo
  - Uptime: Read from /proc/uptime
  - CPU model/cores: Read from /proc/cpuinfo
```

#### Temperature Data (System Collector - 5s interval)
```
Data Source: /sys/class/hwmon/ or `sensors` command
Methods:
  1. Try `sensors -u` command (lm-sensors)
  2. Fallback: Read from /sys/class/hwmon/hwmon*/temp*_input
  3. Parse sensor names for CPU/MB temps
  
Sensor Name Matching:
  - CPU Temp: "core", "package", "cputin"
  - MB Temp: "mb_temp" (case-insensitive)
```

#### Array Status (Array Collector - 10s interval)
```
Data Source: /var/local/emhttp/var.ini, /var/local/emhttp/disks.ini
Methods:
  - Array state: Read mdState from var.ini
  - Parity info: Count parity disks from disks.ini
  - Capacity: Use syscall.Statfs on /mnt/user
```

#### Disk Information (Disk Collector - 30s interval)
```
Data Source: /var/local/emhttp/disks.ini, smartctl, /proc/diskstats
Methods:
  - Disk list: Parse disks.ini
  - SMART data: Execute `smartctl -a /dev/sdX`
  - I/O stats: Read from /proc/diskstats
  - Temperatures: Parse smartctl output
```

#### GPU Metrics (GPU Collector - 10s interval)
```
Data Source: nvidia-smi, rocm-smi, radeontop, intel_gpu_top
Methods:
  - NVIDIA: Execute `nvidia-smi --query-gpu=...`
  - AMD: Execute `radeontop` or `rocm-smi`
  - Intel: Execute `intel_gpu_top` or read hwmon
```

#### Docker/VMs (Docker & VM Collectors - 10s interval)
```
Data Source: Docker daemon, libvirt
Methods:
  - Docker: Execute `docker ps`, `docker stats`
  - VMs: Execute `virsh list`, `virsh domstats`
```

#### UPS Status (UPS Collector - 10s interval)
```
Data Source: apcupsd or NUT daemon
Methods:
  - APC: Execute `apcaccess` command
  - NUT: Execute `upsc` command
```

---

## 4. Troubleshooting: Missing Temperature Data

### Scenario: Temperatures Not Displayed in API

If a friend's Unraid server shows no temperature data in the API responses, here are the diagnostic steps:

### Step 1: Check if Sensors are Available

```bash
# SSH into the Unraid server
ssh root@<unraid-ip>

# Check if lm-sensors is installed
which sensors
# If not found, install: opkg install lm-sensors

# Try to read sensors
sensors -u
# If no output, sensors may not be configured for your hardware
```

### Step 2: Check hwmon Directly

```bash
# Check if hwmon devices exist
ls -la /sys/class/hwmon/

# Try to read temperature files
cat /sys/class/hwmon/hwmon*/temp*_input 2>/dev/null

# If no output, your hardware doesn't expose temperature sensors
```

### Step 3: Check Agent Logs

```bash
# View agent logs
tail -f /var/log/unraid-management-agent.log

# Look for messages like:
# "Failed to get temperatures"
# "No GPUs detected"
# "Intel GPU collection failed"
```

### Step 4: Test API Response

```bash
# Check system endpoint
curl http://<unraid-ip>:8043/api/v1/system | jq '.cpu_temp_celsius, .motherboard_temp_celsius'

# If both are 0 or null, temperatures are not available
```

### Root Causes and Solutions

| Cause | Likelihood | Solution |
|-------|------------|----------|
| **Hardware doesn't expose sensors** | ⭐⭐⭐⭐⭐ (Very High) | Older/incompatible hardware - no fix available |
| **lm-sensors not installed** | ⭐⭐⭐ (Medium) | Install: `opkg install lm-sensors` |
| **Sensors not configured** | ⭐⭐⭐ (Medium) | Run: `sensors-detect` to configure |
| **Older Unraid OS version** | ⭐⭐ (Low) | Upgrade to Unraid 7.x |
| **Agent not running** | ⭐ (Very Low) | Check: `ps aux \| grep unraid-management-agent` |
| **Permission issues** | ⭐ (Very Low) | Agent runs as root, should have access |

### Most Common Reason: Hardware Compatibility

**The most likely cause is that the older hardware doesn't expose temperature sensors via hwmon or lm-sensors.**

This is **NOT a plugin dependency issue** - it's a hardware limitation. Many older motherboards and CPUs don't have temperature sensors that are accessible via standard Linux interfaces.

### Verification

To confirm if it's a hardware issue:

```bash
# If this returns nothing, your hardware doesn't expose temps
sensors 2>/dev/null || echo "No sensors available"

# If this returns nothing, hwmon is not available
ls /sys/class/hwmon/hwmon*/temp*_input 2>/dev/null || echo "No hwmon temps"
```

If both commands return nothing, **the hardware simply doesn't expose temperature data**, and no plugin can fix this.

---

## 5. Summary

### Key Takeaways

✅ **No plugin dependencies** - The agent is completely independent  
✅ **Direct data collection** - Reads from system files and commands  
✅ **Minimal requirements** - Works on any Unraid 6.9+ system  
✅ **Hardware-agnostic** - Adapts to available hardware  
⚠️ **Hardware limitations** - Some data (temps, GPU) depends on hardware support

### For Your Friend's System

If temperatures are missing:
1. Check if `sensors` command works
2. Check if `/sys/class/hwmon/` has temperature files
3. If both are empty, the hardware doesn't expose temperature sensors
4. This is **NOT** a plugin dependency issue - it's a hardware limitation

The agent will continue to work perfectly for all other metrics (CPU usage, RAM, array status, disks, Docker, VMs, etc.) even if temperature sensors are unavailable.

