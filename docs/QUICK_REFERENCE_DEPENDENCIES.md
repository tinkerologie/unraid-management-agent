# Quick Reference: Dependencies and Requirements

## TL;DR

| Question | Answer |
|----------|--------|
| **Minimum Unraid version?** | 6.9+ (recommended 7.x) |
| **CPU requirements?** | Any x86-64 (Intel/AMD) |
| **RAM requirements?** | Minimal (< 50 MB for agent) |
| **Requires other plugins?** | ❌ NO - completely independent |
| **Requires Dynamix plugins?** | ❌ NO |
| **Requires GPU drivers?** | ❌ NO (optional for GPU metrics) |
| **Collects data independently?** | ✅ YES - from system files/commands |
| **Works on older hardware?** | ✅ YES (except temps if no sensors) |

---

## Data Collection Methods

### What the Agent Collects (and How)

```
System Metrics
├─ CPU usage ..................... /proc/stat
├─ RAM usage ..................... /proc/meminfo
├─ Uptime ....................... /proc/uptime
├─ CPU model/cores .............. /proc/cpuinfo
└─ Temperatures ................. sensors or /sys/class/hwmon/

Array Status
├─ Array state .................. /var/local/emhttp/var.ini
├─ Parity info .................. /var/local/emhttp/disks.ini
└─ Capacity ..................... syscall.Statfs(/mnt/user)

Disk Information
├─ Disk list .................... /var/local/emhttp/disks.ini
├─ SMART data ................... smartctl command
├─ I/O stats .................... /proc/diskstats
└─ Temperatures ................. smartctl output

GPU Metrics (if available)
├─ NVIDIA ........................ nvidia-smi command
├─ AMD ........................... rocm-smi or radeontop
└─ Intel ......................... intel_gpu_top or hwmon

Docker/VMs (if available)
├─ Docker ....................... docker command
└─ VMs ........................... virsh command

UPS Status (if available)
├─ APC ........................... apcaccess command
└─ NUT ........................... upsc command
```

---

## Troubleshooting Missing Data

### Missing Temperatures?

**Most likely cause:** Hardware doesn't expose temperature sensors

```bash
# Check if sensors are available
sensors -u

# Check hwmon
ls /sys/class/hwmon/hwmon*/temp*_input

# If both return nothing → hardware limitation (not a plugin issue)
```

### Missing GPU Data?

**Cause:** GPU drivers/tools not installed

```bash
# NVIDIA
which nvidia-smi

# AMD
which rocm-smi radeontop

# Intel
which intel_gpu_top
```

### Missing Docker/VM Data?

**Cause:** Docker/libvirt not installed or not running

```bash
# Docker
docker ps

# VMs
virsh list
```

### Missing UPS Data?

**Cause:** UPS daemon not running

```bash
# APC
which apcaccess

# NUT
which upsc
```

---

## System Requirements Checklist

- [ ] Unraid 6.9 or later
- [ ] x86-64 CPU (Intel or AMD)
- [ ] Port 8043 available
- [ ] ~5-10 MB disk space
- [ ] Internet connection (for plugin download)

**Optional (for additional features):**
- [ ] lm-sensors (for temperature data)
- [ ] smartctl (for disk SMART data)
- [ ] Docker (for container monitoring)
- [ ] libvirt (for VM monitoring)
- [ ] GPU drivers (for GPU metrics)
- [ ] UPS daemon (for UPS monitoring)

---

## Plugin Dependencies

### ❌ NOT Required

- Dynamix System Information
- Dynamix System Temperature
- GPU Statistics
- Any Intel/NVIDIA/AMD driver plugins
- Any other Unraid plugins

### ✅ Why?

The agent collects data **directly** from:
- System files (`/proc/`, `/sys/`)
- Unraid config files (`/var/local/emhttp/`)
- System commands (`sensors`, `smartctl`, `docker`, `virsh`)
- Hardware interfaces (hwmon, DMI)

---

## Data Availability by Hardware

| Component | Always Available | Requires | Notes |
|-----------|------------------|----------|-------|
| CPU usage | ✅ | None | From /proc/stat |
| RAM usage | ✅ | None | From /proc/meminfo |
| CPU temps | ⚠️ | hwmon/sensors | Older hardware may not expose |
| MB temps | ⚠️ | hwmon/sensors | Older hardware may not expose |
| Disks | ✅ | None | From disks.ini |
| Disk temps | ⚠️ | smartctl | Requires SMART-capable drives |
| Array status | ✅ | None | From var.ini |
| Docker | ⚠️ | Docker installed | Optional feature |
| VMs | ⚠️ | libvirt installed | Optional feature |
| GPU | ⚠️ | GPU + drivers | Optional feature |
| UPS | ⚠️ | UPS daemon | Optional feature |

---

## Common Issues and Solutions

### Issue: No temperature data on older server

**Diagnosis:**
```bash
sensors -u  # Returns nothing?
ls /sys/class/hwmon/hwmon*/temp*_input  # Returns nothing?
```

**Solution:** Hardware doesn't expose sensors - not a plugin issue

### Issue: GPU metrics not showing

**Diagnosis:**
```bash
nvidia-smi  # Command not found?
rocm-smi    # Command not found?
```

**Solution:** Install GPU drivers or tools

### Issue: Docker/VM data missing

**Diagnosis:**
```bash
docker ps   # Error?
virsh list  # Error?
```

**Solution:** Install and start Docker/libvirt

### Issue: Agent not running

**Diagnosis:**
```bash
ps aux | grep unraid-management-agent
curl http://localhost:8043/api/v1/health
```

**Solution:** Check logs: `tail -f /var/log/unraid-management-agent.log`

---

## Key Insight

**The Unraid Management Agent is completely independent and does NOT rely on any other plugins.** It collects data directly from system sources. If data is missing, it's because:

1. The hardware doesn't support it (e.g., no temperature sensors)
2. The optional component isn't installed (e.g., Docker, GPU drivers)
3. The agent isn't running

It's **never** because of missing plugin dependencies.

