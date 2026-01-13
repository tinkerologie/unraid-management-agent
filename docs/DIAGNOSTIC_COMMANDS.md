# Diagnostic Commands for Unraid Management Agent

Use these commands to diagnose issues and verify data collection capabilities.

## System Information

### Check Unraid Version

```bash
cat /etc/unraid-version
# or
cat /var/local/emhttp/var.ini | grep version
```

### Check CPU Information

```bash
cat /proc/cpuinfo | head -20
# Shows: processor count, model name, cores, threads
```

### Check RAM

```bash
cat /proc/meminfo | head -5
# Shows: MemTotal, MemFree, MemAvailable
```

### Check Uptime

```bash
cat /proc/uptime
# Shows: uptime in seconds
```

---

## Temperature Sensors

### Check if lm-sensors is Installed

```bash
which sensors
# If not found: opkg install lm-sensors
```

### Read Sensors

```bash
sensors -u
# Shows all available temperature sensors
# If empty, hardware doesn't expose sensors
```

### Check hwmon Directly

```bash
ls -la /sys/class/hwmon/
# Lists available hwmon devices

# Read temperature files
cat /sys/class/hwmon/hwmon*/temp*_input 2>/dev/null
# Shows raw temperature values in millidegrees

# Find sensor names
for d in /sys/class/hwmon/hwmon*; do
  echo "=== $(basename $d) ==="
  cat $d/name 2>/dev/null
  ls $d/temp*_label 2>/dev/null | while read f; do
    echo "$(basename $f): $(cat $f)"
  done
done
```

### Check CPU Temperature Specifically

```bash
# Look for coretemp
for d in /sys/class/hwmon/hwmon*; do
  if grep -q coretemp $d/name 2>/dev/null; then
    echo "Found coretemp in $(basename $d)"
    cat $d/temp*_input 2>/dev/null
  fi
done
```

### Check Motherboard Temperature

```bash
# Look for MB_Temp or similar
sensors -u | grep -i "mb_temp\|motherboard"
```

---

## Disk Information

### Check Disks Configuration

```bash
cat /var/local/emhttp/disks.ini
# Shows all configured disks
```

### Check Array Status

```bash
cat /var/local/emhttp/var.ini | grep -E "mdState|mdResync"
# Shows array state and parity info
```

### Check Disk Space

```bash
df -h /mnt/user
# Shows array capacity and usage

# Or use syscall method (what agent uses)
stat -f /mnt/user
```

### Check SMART Data

```bash
which smartctl
# If not found: opkg install smartmontools

# Read SMART data for a disk
smartctl -a /dev/sda
# Shows temperature, power-on hours, SMART status
```

### Check Disk I/O Stats

```bash
cat /proc/diskstats
# Shows I/O statistics for all disks
```

---

## GPU Information

### Check NVIDIA GPU

```bash
which nvidia-smi
# If found, GPU is available

# Get GPU info
nvidia-smi --query-gpu=index,name,temperature.gpu,utilization.gpu,memory.used,memory.total --format=csv
```

### Check AMD GPU

```bash
which rocm-smi
# or
which radeontop

# Get AMD GPU info
rocm-smi
```

### Check Intel GPU

```bash
which intel_gpu_top
# or check hwmon for i915
ls /sys/class/drm/card*/device/hwmon/hwmon*/temp*_input 2>/dev/null
```

---

## Docker Information

### Check Docker Installation

```bash
which docker
# If not found, Docker is not installed

# Check Docker status
docker ps
# Lists running containers

# Check Docker stats
docker stats --no-stream
# Shows CPU, memory, network stats
```

---

## Virtual Machines

### Check libvirt Installation

```bash
which virsh
# If not found, libvirt is not installed

# List VMs
virsh list --all

# Get VM stats
virsh domstats --raw
```

---

## UPS Information

### Check APC UPS

```bash
which apcaccess
# If found, APC UPS monitoring is available

# Get UPS status
apcaccess
```

### Check NUT UPS

```bash
which upsc
# If found, NUT UPS monitoring is available

# Get UPS status
upsc ups@localhost
```

### Check UPS Daemon Status

```bash
# APC
ps aux | grep apcupsd

# NUT
ps aux | grep upsmon
```

---

## Agent Status

### Check if Agent is Running

```bash
ps aux | grep unraid-management-agent
# Should show the running agent process
```

### Check Agent Logs

```bash
tail -f /var/log/unraid-management-agent.log
# Shows real-time logs

# Check for errors
grep -i error /var/log/unraid-management-agent.log

# Check for temperature collection
grep -i temperature /var/log/unraid-management-agent.log
```

### Test API Endpoints

```bash
# Health check
curl http://localhost:8043/api/v1/health

# System info
curl http://localhost:8043/api/v1/system | jq .

# Check temperatures specifically
curl http://localhost:8043/api/v1/system | jq '.cpu_temp_celsius, .motherboard_temp_celsius'

# Array status
curl http://localhost:8043/api/v1/array | jq .

# Disks
curl http://localhost:8043/api/v1/disks | jq .

# Docker
curl http://localhost:8043/api/v1/docker | jq .

# VMs
curl http://localhost:8043/api/v1/vm | jq .

# GPU
curl http://localhost:8043/api/v1/gpu | jq .

# UPS
curl http://localhost:8043/api/v1/ups | jq .
```

---

## Comprehensive Diagnostic Script

```bash
#!/bin/bash

echo "=== Unraid Management Agent Diagnostics ==="
echo ""

echo "1. Unraid Version:"
cat /etc/unraid-version 2>/dev/null || echo "Not found"
echo ""

echo "2. Agent Status:"
ps aux | grep unraid-management-agent | grep -v grep || echo "Not running"
echo ""

echo "3. API Health:"
curl -s http://localhost:8043/api/v1/health || echo "API not responding"
echo ""

echo "4. Temperature Sensors:"
sensors -u 2>/dev/null | head -20 || echo "No sensors available"
echo ""

echo "5. hwmon Temperatures:"
cat /sys/class/hwmon/hwmon*/temp*_input 2>/dev/null | head -5 || echo "No hwmon temps"
echo ""

echo "6. Docker Status:"
docker ps 2>/dev/null | head -3 || echo "Docker not available"
echo ""

echo "7. VM Status:"
virsh list 2>/dev/null | head -3 || echo "libvirt not available"
echo ""

echo "8. GPU Status:"
nvidia-smi --query-gpu=name --format=csv 2>/dev/null || echo "NVIDIA GPU not available"
echo ""

echo "9. UPS Status:"
apcaccess 2>/dev/null | head -3 || echo "APC UPS not available"
echo ""

echo "10. Agent Logs (last 10 lines):"
tail -10 /var/log/unraid-management-agent.log 2>/dev/null || echo "Log file not found"
```

Save as `/tmp/diagnose.sh` and run:

```bash
bash /tmp/diagnose.sh
```

---

## Interpreting Results

### If temperatures are 0 or missing

1. Run: `sensors -u`
2. If empty → hardware doesn't expose sensors
3. If populated → check agent logs for parsing errors

### If Docker/VM data is missing

1. Run: `docker ps` or `virsh list`
2. If command not found → component not installed
3. If error → component not running

### If GPU data is missing

1. Run: `nvidia-smi` or `rocm-smi`
2. If command not found → drivers not installed
3. If error → GPU not detected

### If API is not responding

1. Check: `ps aux | grep unraid-management-agent`
2. Check logs: `tail -f /var/log/unraid-management-agent.log`
3. Check port: `netstat -tlnp | grep 8043`
