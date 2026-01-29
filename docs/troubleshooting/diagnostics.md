# Diagnostic Commands

Essential commands for diagnosing issues with the Unraid Management Agent.

## Service Status

### Check if Service is Running

```bash
# Process check
ps aux | grep unraid-management-agent

# Expected output shows running process:
# root ... /usr/local/emhttp/plugins/unraid-management-agent/unraid-management-agent boot
```

### View Service Logs

```bash
# Real-time log monitoring
tail -f /var/log/unraid-management-agent.log

# Last 100 lines
tail -100 /var/log/unraid-management-agent.log

# Search for errors
grep -i error /var/log/unraid-management-agent.log

# Search for specific collector
grep "Docker collector" /var/log/unraid-management-agent.log
```

### Service Control

```bash
# Stop service
/usr/local/emhttp/plugins/unraid-management-agent/scripts/stop

# Start service
/usr/local/emhttp/plugins/unraid-management-agent/scripts/start

# Restart service
/usr/local/emhttp/plugins/unraid-management-agent/scripts/stop
/usr/local/emhttp/plugins/unraid-management-agent/scripts/start

# Start in debug mode (manual)
/usr/local/emhttp/plugins/unraid-management-agent/unraid-management-agent boot --debug
```

## Network Diagnostics

### Check Port Status

```bash
# Check if port 8043 is listening
netstat -tulpn | grep 8043

# Check what process is using port 8043
lsof -i :8043

# Check all listening ports
netstat -tulpn | grep LISTEN
```

### Test API Connectivity

```bash
# Local connection
curl http://localhost:8043/api/v1/health

# From another machine
curl http://UNRAID_IP:8043/api/v1/health

# Check response time
time curl http://localhost:8043/api/v1/system

# Verbose connection test
curl -v http://localhost:8043/api/v1/health
```

### Test WebSocket

```bash
# Using websocat (if installed)
websocat ws://localhost:8043/api/v1/ws

# Using wscat (if installed)
wscat -c ws://localhost:8043/api/v1/ws
```

## Data Collection Diagnostics

### Verify System Access

```bash
# Check /proc access
cat /proc/stat | head -5
cat /proc/meminfo | head -5
cat /proc/uptime

# Check /sys access
ls -l /sys/class/hwmon/
ls -l /sys/class/net/

# Check Unraid config files
cat /var/local/emhttp/var.ini | head -10
```

### Docker Diagnostics

```bash
# Check Docker socket access
ls -l /var/run/docker.sock

# Test Docker connection
docker info

# List containers
docker ps -a

# Check Docker API version
docker version
```

### VM/Libvirt Diagnostics

```bash
# Check libvirt socket
ls -l /var/run/libvirt/libvirt-sock

# List VMs via virsh
virsh list --all

# Check libvirtd status
ps aux | grep libvirtd
```

### Disk/SMART Diagnostics

```bash
# Check smartctl availability
which smartctl

# Test SMART on a disk
smartctl -i /dev/sda
smartctl -H /dev/sda

# Check disk stats
cat /proc/diskstats
```

### Temperature Sensor Diagnostics

```bash
# Check sensors command
which sensors

# List all sensors
sensors

# Raw sensor output
sensors -u

# Check hwmon devices
ls -la /sys/class/hwmon/
cat /sys/class/hwmon/hwmon*/name
```

### GPU Diagnostics

```bash
# Check nvidia-smi
which nvidia-smi

# Test GPU detection
nvidia-smi

# GPU detailed info
nvidia-smi -q
```

### UPS Diagnostics

```bash
# Check apcupsd
which apcaccess
apcaccess status

# Check NUT
which upsc
upsc -l
upsc ups@localhost
```

## API Response Testing

### Test All Endpoints

```bash
# Create test script
cat << 'EOF' > /tmp/test-api.sh
#!/bin/bash
BASE_URL="http://localhost:8043/api/v1"

echo "Testing endpoints..."
curl -s $BASE_URL/health && echo "✓ Health"
curl -s $BASE_URL/system > /dev/null && echo "✓ System"
curl -s $BASE_URL/array > /dev/null && echo "✓ Array"
curl -s $BASE_URL/disks > /dev/null && echo "✓ Disks"
curl -s $BASE_URL/docker > /dev/null && echo "✓ Docker"
curl -s $BASE_URL/vm > /dev/null && echo "✓ VMs"
curl -s $BASE_URL/network > /dev/null && echo "✓ Network"
curl -s $BASE_URL/shares > /dev/null && echo "✓ Shares"
curl -s $BASE_URL/ups > /dev/null && echo "✓ UPS"
curl -s $BASE_URL/gpu > /dev/null && echo "✓ GPU"
echo "Done!"
EOF

chmod +x /tmp/test-api.sh
/tmp/test-api.sh
```

### Collector Status Check

```bash
# Check collectors status (if endpoint exists)
curl http://localhost:8043/api/v1/collectors/status | jq

# Check specific collector
curl http://localhost:8043/api/v1/collectors/system | jq
```

## Performance Diagnostics

### Memory Usage

```bash
# Check agent memory usage
ps aux | grep unraid-management-agent | grep -v grep

# Detailed memory info
pmap $(pgrep -f unraid-management-agent)
```

### CPU Usage

```bash
# Real-time CPU monitoring
top -b -n 1 | grep unraid-management-agent

# CPU usage over time (run for 10 seconds)
top -b -d 1 -n 10 | grep unraid-management-agent
```

### Response Times

```bash
# Measure endpoint response times
for endpoint in health system array disks docker vm; do
  echo -n "$endpoint: "
  time curl -s http://localhost:8043/api/v1/$endpoint > /dev/null
done
```

## Configuration Diagnostics

### Check Configuration File

```bash
# View current config
cat /boot/config/plugins/unraid-management-agent/config.cfg

# Check file permissions
ls -l /boot/config/plugins/unraid-management-agent/

# Verify config syntax (if tool available)
cat /boot/config/plugins/unraid-management-agent/config.cfg | grep -v ^# | grep .
```

### Check Binary Info

```bash
# Binary location
which unraid-management-agent

# Binary version
/usr/local/emhttp/plugins/unraid-management-agent/unraid-management-agent --version

# Binary permissions
ls -l /usr/local/emhttp/plugins/unraid-management-agent/unraid-management-agent
```

## Log Analysis

### Find Errors

```bash
# Recent errors
grep -i error /var/log/unraid-management-agent.log | tail -20

# Count error types
grep -i error /var/log/unraid-management-agent.log | cut -d: -f2 | sort | uniq -c

# Errors in last hour
grep -i error /var/log/unraid-management-agent.log | grep "$(date +'%Y-%m-%d %H')"
```

### Find Warnings

```bash
# All warnings
grep -i warning /var/log/unraid-management-agent.log

# Recent warnings
grep -i warning /var/log/unraid-management-agent.log | tail -10
```

### Collector Activity

```bash
# See which collectors are running
grep "Starting.*collector" /var/log/unraid-management-agent.log

# Check collector errors
grep "collector.*failed\|collector.*error" /var/log/unraid-management-agent.log

# Last collection times
grep "Collecting" /var/log/unraid-management-agent.log | tail -20
```

## System Information

### Unraid Version

```bash
# Unraid version
cat /etc/unraid-version

# Kernel version
uname -r

# System architecture
uname -m
```

### Plugin Version

```bash
# Plugin version from binary
/usr/local/emhttp/plugins/unraid-management-agent/unraid-management-agent --version

# Plugin version from file
cat /usr/local/emhttp/plugins/unraid-management-agent/VERSION
```

## Export Diagnostics

### Create Diagnostic Bundle

```bash
# Create comprehensive diagnostic output
cat << 'EOF' > /tmp/diagnostic-bundle.sh
#!/bin/bash
OUTPUT="/tmp/unraid-agent-diagnostics.txt"
echo "Unraid Management Agent Diagnostics" > $OUTPUT
echo "Generated: $(date)" >> $OUTPUT
echo "=================================" >> $OUTPUT

echo -e "\n=== System Info ===" >> $OUTPUT
uname -a >> $OUTPUT
cat /etc/unraid-version >> $OUTPUT

echo -e "\n=== Service Status ===" >> $OUTPUT
ps aux | grep unraid-management-agent >> $OUTPUT

echo -e "\n=== Port Status ===" >> $OUTPUT
netstat -tulpn | grep 8043 >> $OUTPUT

echo -e "\n=== Last 50 Log Lines ===" >> $OUTPUT
tail -50 /var/log/unraid-management-agent.log >> $OUTPUT

echo -e "\n=== Configuration ===" >> $OUTPUT
cat /boot/config/plugins/unraid-management-agent/config.cfg >> $OUTPUT

echo -e "\n=== API Health Check ===" >> $OUTPUT
curl -s http://localhost:8043/api/v1/health >> $OUTPUT 2>&1

echo -e "\n=== Collectors Status ===" >> $OUTPUT
curl -s http://localhost:8043/api/v1/collectors/status >> $OUTPUT 2>&1

echo "Diagnostics saved to: $OUTPUT"
cat $OUTPUT
EOF

chmod +x /tmp/diagnostic-bundle.sh
/tmp/diagnostic-bundle.sh
```

## Next Steps

- [Common Issues](common-issues.md) - Solutions to frequent problems
- [Hardware Compatibility](hardware-compatibility.md) - Hardware-specific guidance
- [System Requirements](../guides/system-requirements.md) - Verify requirements

---

**Last Updated**: January 2026
