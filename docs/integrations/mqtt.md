# MQTT Integration

Publish Unraid server events to MQTT brokers for IoT integration and Home Assistant automation.

## Overview

The Unraid Management Agent can publish system events to MQTT brokers in real-time, enabling:

- **Home Assistant** integration for smart home automation
- **IoT dashboards** like Node-RED
- **Custom automation** scripts
- **Multi-server monitoring** from a central MQTT broker

## Configuration

### Enable MQTT

Edit `/boot/config/plugins/unraid-management-agent/unraid-management-agent.cfg`:

```bash
# Enable MQTT publishing
MQTT_ENABLED=true

# MQTT broker address (required)
MQTT_BROKER=tcp://192.168.1.100:1883

# Optional: Authentication
MQTT_USERNAME=unraid_agent
MQTT_PASSWORD=your_secure_password

# Optional: Topic prefix (default: unraid)
MQTT_TOPIC_PREFIX=homelab/unraid
```

### Restart Service

```bash
/etc/rc.d/rc.unraid-management-agent restart
```

### Verify Connection

Check logs for MQTT connection:

```bash
tail -f /var/log/unraid-management-agent.log | grep MQTT
```

You should see: `MQTT client connected to tcp://...`

## MQTT Topics

The agent publishes to hierarchical topics under your configured prefix.

### Topic Structure

```
<prefix>/system          # System metrics (CPU, RAM, temps)
<prefix>/array           # Array status and capacity
<prefix>/disks           # All disk information
<prefix>/shares          # User share list
<prefix>/containers      # Docker container status
<prefix>/vms             # Virtual machine status
<prefix>/ups             # UPS status (if configured)
<prefix>/gpu             # GPU metrics (if available)
<prefix>/network         # Network interface info
<prefix>/notifications   # System notifications
```

### Message Format

All messages are published as JSON payloads with QoS 1 (at least once delivery).

#### System Topic Example

**Topic**: `unraid/system`

**Payload**:

```json
{
  "hostname": "Tower",
  "version": "7.2.0",
  "agent_version": "2025.11.0",
  "uptime": 345600,
  "cpu_usage": 15.3,
  "cpu_temp": 45.5,
  "cpu_model": "Intel Core i7-9700K",
  "cpu_cores": 8,
  "cpu_threads": 8,
  "ram_used": 8589934592,
  "ram_total": 34359738368,
  "ram_usage": 25.0,
  "timestamp": "2025-01-20T10:30:00Z"
}
```

#### Array Topic Example

**Topic**: `unraid/array`

**Payload**:

```json
{
  "state": "STARTED",
  "size": 12000000000000,
  "free": 8000000000000,
  "used_percent": 33.3,
  "num_disks": 6,
  "parity_valid": true,
  "parity_check_status": "idle",
  "timestamp": "2025-01-20T10:30:00Z"
}
```

#### Containers Topic Example

**Topic**: `unraid/containers`

**Payload**:

```json
[
  {
    "id": "abc123def456",
    "name": "plex",
    "state": "running",
    "status": "Up 2 hours",
    "image": "plexinc/pms-docker:latest",
    "cpu_percent": 5.2,
    "memory_usage": 2147483648,
    "timestamp": "2025-01-20T10:30:00Z"
  }
]
```

## Home Assistant Integration

### MQTT Sensor Configuration

Add to your `configuration.yaml`:

```yaml
mqtt:
  sensor:
    # CPU Usage
    - name: "Unraid CPU Usage"
      state_topic: "unraid/system"
      value_template: "{{ value_json.cpu_usage | round(1) }}"
      unit_of_measurement: "%"
      icon: mdi:cpu-64-bit
      
    # RAM Usage
    - name: "Unraid RAM Usage"
      state_topic: "unraid/system"
      value_template: "{{ value_json.ram_usage | round(1) }}"
      unit_of_measurement: "%"
      icon: mdi:memory
      
    # CPU Temperature
    - name: "Unraid CPU Temperature"
      state_topic: "unraid/system"
      value_template: "{{ value_json.cpu_temp | round(1) }}"
      unit_of_measurement: "°C"
      device_class: temperature
      
    # Array Status
    - name: "Unraid Array State"
      state_topic: "unraid/array"
      value_template: "{{ value_json.state }}"
      icon: mdi:server
      
    # Array Usage
    - name: "Unraid Array Usage"
      state_topic: "unraid/array"
      value_template: "{{ value_json.used_percent | round(1) }}"
      unit_of_measurement: "%"
      icon: mdi:harddisk
      
    # Parity Valid
    - name: "Unraid Parity Status"
      state_topic: "unraid/array"
      value_template: "{{ 'Valid' if value_json.parity_valid else 'Invalid' }}"
      icon: mdi:shield-check
```

### Home Assistant Automation Examples

#### Alert on High CPU

```yaml
automation:
  - alias: "Unraid High CPU Alert"
    trigger:
      - platform: numeric_state
        entity_id: sensor.unraid_cpu_usage
        above: 90
        for:
          minutes: 5
    action:
      - service: notify.mobile_app
        data:
          message: "Unraid CPU usage is {{ states('sensor.unraid_cpu_usage') }}%"
          title: "⚠️ Unraid Alert"
```

#### Alert on Parity Invalid

```yaml
automation:
  - alias: "Unraid Parity Invalid Alert"
    trigger:
      - platform: state
        entity_id: sensor.unraid_parity_status
        to: "Invalid"
    action:
      - service: notify.mobile_app
        data:
          message: "Unraid array parity is INVALID!"
          title: "🚨 Unraid Critical Alert"
```

#### Container Stopped Alert

```yaml
automation:
  - alias: "Plex Container Stopped"
    trigger:
      - platform: mqtt
        topic: "unraid/containers"
    condition:
      - condition: template
        value_template: >
          {% set containers = trigger.payload_json %}
          {% set plex = containers | selectattr('name', 'eq', 'plex') | list | first %}
          {{ plex.state != 'running' }}
    action:
      - service: notify.mobile_app
        data:
          message: "Plex container has stopped!"
          title: "⚠️ Unraid Container Alert"
```

## Node-RED Integration

### MQTT In Node

Configure an MQTT In node:

- **Server**: Your MQTT broker
- **Topic**: `unraid/#` (subscribe to all topics)
- **QoS**: 1
- **Output**: Parsed JSON object

### Example Flow

```json
[
  {
    "id": "mqtt_in",
    "type": "mqtt in",
    "topic": "unraid/system",
    "qos": "1",
    "broker": "mqtt_broker"
  },
  {
    "id": "cpu_check",
    "type": "function",
    "func": "if (msg.payload.cpu_usage > 80) {\n  msg.payload = {\n    title: 'High CPU Usage',\n    message: `CPU at ${msg.payload.cpu_usage}%`\n  };\n  return msg;\n}"
  },
  {
    "id": "notification",
    "type": "pushover",
    "title": "{{payload.title}}",
    "message": "{{payload.message}}"
  }
]
```

## Testing MQTT

### Subscribe to All Topics

```bash
# Using mosquitto_sub
mosquitto_sub -h localhost -t "unraid/#" -v

# With authentication
mosquitto_sub -h localhost -u username -P password -t "unraid/#" -v
```

### Publish Test Message (API)

```bash
curl -X POST http://localhost:8043/api/v1/mqtt/publish \
  -H "Content-Type: application/json" \
  -d '{
    "topic": "unraid/test",
    "payload": {"message": "test"},
    "retained": false
  }'
```

## Advanced Configuration

### TLS/SSL

For secure MQTT connections:

```bash
# Use ssl:// instead of tcp://
MQTT_BROKER=ssl://mqtt.example.com:8883
```

**Note**: TLS certificate validation is performed. Ensure valid certificates or configure your broker accordingly.

### QoS and Retained Messages

- **QoS**: Messages are published with QoS 1 (at least once delivery)
- **Retained**: Messages are NOT retained by default (latest state only)

To enable retained messages for persistent state, this would require a code modification.

### Custom Topic Prefix

Use hierarchical topics for multi-server setups:

```bash
# Server 1
MQTT_TOPIC_PREFIX=homelab/tower1

# Server 2
MQTT_TOPIC_PREFIX=homelab/tower2
```

Topics become:

- `homelab/tower1/system`
- `homelab/tower2/system`

## Troubleshooting

### Connection Failed

```bash
# Check logs
tail -f /var/log/unraid-management-agent.log | grep MQTT

# Test broker connectivity
mosquitto_sub -h BROKER_IP -p 1883 -t "test"

# Test with authentication
mosquitto_pub -h BROKER_IP -u username -P password -t "test" -m "hello"
```

### No Messages Received

1. Verify MQTT is enabled in config
2. Check broker is running: `ps aux | grep mosquitto`
3. Verify topic subscription: `mosquitto_sub -t "unraid/#" -v`
4. Check firewall rules on broker

### Authentication Errors

```bash
# Verify credentials
mosquitto_pub -h BROKER_IP -u username -P password -t "test" -m "test"

# Check broker logs
tail -f /var/log/mosquitto/mosquitto.log
```

## MQTT Broker Setup

### Mosquitto on Unraid (Docker)

```yaml
version: '3'
services:
  mosquitto:
    image: eclipse-mosquitto:latest
    container_name: mosquitto
    restart: unless-stopped
    ports:
      - "1883:1883"
      - "9001:9001"
    volumes:
      - ./config:/mosquitto/config
      - ./data:/mosquitto/data
      - ./log:/mosquitto/log
```

Create `config/mosquitto.conf`:

```conf
listener 1883
allow_anonymous true
persistence true
persistence_location /mosquitto/data/
log_dest file /mosquitto/log/mosquitto.log
```

With authentication:

```conf
listener 1883
allow_anonymous false
password_file /mosquitto/config/passwd
```

Create password file:

```bash
docker exec mosquitto mosquitto_passwd -c /mosquitto/config/passwd username
```

## Performance Considerations

### Message Rate

With default collection intervals:

- **Fast topics** (5-10s): system, array, containers, vms
- **Moderate topics** (30-60s): disks, shares, network
- **Total message rate**: ~10-15 messages/minute

### Bandwidth

Typical bandwidth usage:

- **Average message size**: 200-500 bytes
- **Peak bandwidth**: <1 KB/s
- **Daily data**: <50 MB

### Broker Load

Impact on MQTT broker:

- **CPU**: Negligible
- **Memory**: ~5-10 MB per client
- **Connections**: 1 persistent connection

## Next Steps

- [Home Assistant Integration](home-assistant.md) - Complete HA setup
- [Grafana Integration](grafana.md) - Monitoring dashboards  
- [REST API Reference](../api/rest-api.md) - Control via API
- [Configuration Guide](../guides/configuration.md) - Customize settings

---

**Last Updated**: January 2026  
**MQTT Protocol Version**: 3.1.1  
**Topics Published**: 10+
