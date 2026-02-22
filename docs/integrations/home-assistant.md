# Home Assistant Integration

Complete guide for integrating Unraid Management Agent with Home Assistant for smart home automation.

## Overview

The Unraid Management Agent enables deep Home Assistant integration via MQTT, allowing you to:

- **Monitor** server health, array status, and resource usage
- **Control** Docker containers and VMs from HA dashboards
- **Automate** actions based on server events (high CPU, parity errors, UPS status)
- **Alert** on critical conditions via HA notification systems

## Prerequisites

- **Home Assistant** 2024.1+ running
- **MQTT Broker** (Mosquitto recommended)
- **Unraid Management Agent** with MQTT enabled
- **MQTT Integration** installed in Home Assistant

## Quick Setup

### 1. Install MQTT Broker

Add Mosquitto via Home Assistant Add-ons:

1. **Settings** → **Add-ons** → **Add-on Store**
2. Search for "Mosquitto broker"
3. Click **Install** → **Start** → Enable **Start on boot**

### 2. Configure Unraid Agent

Edit `/boot/config/plugins/unraid-management-agent/unraid-management-agent.cfg`:

```bash
MQTT_ENABLED=true
MQTT_BROKER=tcp://homeassistant.local:1883
MQTT_USERNAME=unraid
MQTT_PASSWORD=your_secure_password
MQTT_TOPIC_PREFIX=unraid
```

Restart the agent:

```bash
/etc/rc.d/rc.unraid-management-agent restart
```

### 3. Configure Home Assistant

Add MQTT integration:

1. **Settings** → **Devices & Services** → **Add Integration**
2. Search for "MQTT"
3. Enter broker details:
   - **Broker**: `homeassistant.local` (or broker IP)
   - **Port**: `1883`
   - **Username**: `unraid`
   - **Password**: `your_secure_password`

## Sensors Configuration

### Basic Sensors

Add to `configuration.yaml`:

```yaml
mqtt:
  sensor:
    # System Sensors
    - name: "Unraid CPU Usage"
      unique_id: unraid_cpu_usage
      state_topic: "unraid/system"
      value_template: "{{ value_json.cpu_usage | round(1) }}"
      unit_of_measurement: "%"
      icon: mdi:cpu-64-bit

    - name: "Unraid RAM Usage"
      unique_id: unraid_ram_usage
      state_topic: "unraid/system"
      value_template: "{{ value_json.ram_usage | round(1) }}"
      unit_of_measurement: "%"
      icon: mdi:memory

    - name: "Unraid CPU Temperature"
      unique_id: unraid_cpu_temp
      state_topic: "unraid/system"
      value_template: "{{ value_json.cpu_temp | round(1) }}"
      unit_of_measurement: "°C"
      device_class: temperature
      state_class: measurement

    - name: "Unraid Uptime"
      unique_id: unraid_uptime
      state_topic: "unraid/system"
      value_template: "{{ (value_json.uptime / 86400) | round(1) }}"
      unit_of_measurement: "days"
      icon: mdi:clock-outline

    # Array Sensors
    - name: "Unraid Array State"
      unique_id: unraid_array_state
      state_topic: "unraid/array"
      value_template: "{{ value_json.state }}"
      icon: mdi:server

    - name: "Unraid Array Usage"
      unique_id: unraid_array_usage
      state_topic: "unraid/array"
      value_template: "{{ value_json.used_percent | round(1) }}"
      unit_of_measurement: "%"
      icon: mdi:harddisk
      state_class: measurement

    - name: "Unraid Array Free Space"
      unique_id: unraid_array_free
      state_topic: "unraid/array"
      value_template: "{{ (value_json.free / 1099511627776) | round(2) }}"
      unit_of_measurement: "TB"
      icon: mdi:database
      state_class: measurement

    - name: "Unraid Parity Status"
      unique_id: unraid_parity_status
      state_topic: "unraid/array"
      value_template: >
        {% if value_json.parity_valid %}
          Valid
        {% else %}
          Invalid
        {% endif %}
      icon: mdi:shield-check
```

### Advanced Sensors

```yaml
mqtt:
  sensor:
    # UPS Status (if configured)
    - name: "UPS Battery Level"
      unique_id: ups_battery
      state_topic: "unraid/ups"
      value_template: "{{ value_json.battery_charge }}"
      unit_of_measurement: "%"
      device_class: battery

    - name: "UPS Runtime"
      unique_id: ups_runtime
      state_topic: "unraid/ups"
      value_template: "{{ (value_json.runtime / 60) | round(0) }}"
      unit_of_measurement: "minutes"
      icon: mdi:clock-outline

    - name: "UPS Load"
      unique_id: ups_load
      state_topic: "unraid/ups"
      value_template: "{{ value_json.load }}"
      unit_of_measurement: "%"
      icon: mdi:gauge

    # Docker Container Counts
    - name: "Unraid Containers Running"
      unique_id: unraid_containers_running
      state_topic: "unraid/containers"
      value_template: >
        {{ value_json | selectattr('state', 'eq', 'running') | list | count }}
      icon: mdi:docker

    # VM Counts
    - name: "Unraid VMs Running"
      unique_id: unraid_vms_running
      state_topic: "unraid/vms"
      value_template: >
        {{ value_json | selectattr('state', 'eq', 'running') | list | count }}
      icon: mdi:desktop-tower-monitor
```

### Binary Sensors

```yaml
mqtt:
  binary_sensor:
    # Array Started
    - name: "Unraid Array Started"
      unique_id: unraid_array_started
      state_topic: "unraid/array"
      value_template: >
        {{ value_json.state == 'STARTED' }}
      device_class: running

    # Parity Check Active
    - name: "Unraid Parity Check Running"
      unique_id: unraid_parity_check
      state_topic: "unraid/array"
      value_template: >
        {{ value_json.parity_check_status == 'running' }}
      device_class: running

    # UPS Online
    - name: "UPS Online"
      unique_id: ups_online
      state_topic: "unraid/ups"
      value_template: >
        {{ value_json.status == 'ONLINE' }}
      device_class: power
```

## Switches Configuration

### Docker Container Control

```yaml
mqtt:
  switch:
    - name: "Plex Container"
      unique_id: plex_container
      state_topic: "unraid/containers"
      value_template: >
        {% set containers = value_json %}
        {% set plex = containers | selectattr('name', 'eq', 'plex') | list %}
        {{ plex[0].state == 'running' if plex else 'unknown' }}
      command_topic: "unraid/containers/plex/command"
      payload_on: "start"
      payload_off: "stop"
      icon: mdi:plex
```

**Note**: This requires a custom MQTT command handler. See [REST API Control](#rest-api-control-recommended) for the recommended approach.

## REST API Control (Recommended)

Instead of MQTT commands, use Home Assistant's RESTful switches for better control:

```yaml
switch:
  # Docker Container Control
  - platform: rest
    name: "Plex Container"
    resource: "http://unraid-server:8043/api/v1/docker/plex/start"
    body_on: ""
    body_off: ""
    is_on_template: >
      {{ state_attr('sensor.unraid_containers', 'plex_state') == 'running' }}

  # VM Control
  - platform: rest
    name: "Windows VM"
    resource: "http://unraid-server:8043/api/v1/vm/Windows10/start"
    body_on: ""
    body_off: ""
    is_on_template: >
      {{ state_attr('sensor.unraid_vms', 'Windows10_state') == 'running' }}
```

### RESTful Commands

Add to `configuration.yaml`:

```yaml
rest_command:
  # Container Control
  plex_start:
    url: "http://unraid-server:8043/api/v1/docker/plex/start"
    method: POST

  plex_stop:
    url: "http://unraid-server:8043/api/v1/docker/plex/stop"
    method: POST

  plex_restart:
    url: "http://unraid-server:8043/api/v1/docker/plex/restart"
    method: POST

  # VM Control
  windows_start:
    url: "http://unraid-server:8043/api/v1/vm/Windows10/start"
    method: POST

  windows_stop:
    url: "http://unraid-server:8043/api/v1/vm/Windows10/stop"
    method: POST

  # Array Control
  array_start:
    url: "http://unraid-server:8043/api/v1/array/start"
    method: POST

  array_stop:
    url: "http://unraid-server:8043/api/v1/array/stop"
    method: POST
```

## Automations

### Critical Alerts

#### High CPU Usage

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
      - service: notify.mobile_app_phone
        data:
          title: "⚠️ Unraid Alert"
          message: "CPU usage is {{ states('sensor.unraid_cpu_usage') }}%"
          data:
            priority: high
```

#### Parity Check Failed

```yaml
automation:
  - alias: "Unraid Parity Invalid"
    trigger:
      - platform: state
        entity_id: sensor.unraid_parity_status
        to: "Invalid"
    action:
      - service: notify.mobile_app_phone
        data:
          title: "🚨 CRITICAL: Unraid Parity Invalid"
          message: "Array parity is invalid! Check disk health immediately."
          data:
            priority: critical
            color: red
```

#### Array Stopped Unexpectedly

```yaml
automation:
  - alias: "Unraid Array Stopped"
    trigger:
      - platform: state
        entity_id: binary_sensor.unraid_array_started
        to: "off"
    condition:
      - condition: time
        after: "07:00:00"
        before: "22:00:00"
    action:
      - service: notify.mobile_app_phone
        data:
          title: "⚠️ Unraid Array Stopped"
          message: "The array has stopped unexpectedly."
```

#### UPS Battery Low

```yaml
automation:
  - alias: "UPS Battery Low"
    trigger:
      - platform: numeric_state
        entity_id: sensor.ups_battery_level
        below: 50
    action:
      - service: notify.mobile_app_phone
        data:
          title: "🔋 UPS Battery Low"
          message: "UPS battery at {{ states('sensor.ups_battery_level') }}%"
```

### Proactive Automations

#### Start Plex Before Movie Night

```yaml
automation:
  - alias: "Start Plex for Movie Night"
    trigger:
      - platform: time
        at: "18:30:00"
    condition:
      - condition: time
        weekday:
          - fri
          - sat
    action:
      - service: rest_command.plex_start
```

#### Stop Idle Containers

```yaml
automation:
  - alias: "Stop Idle Containers at Night"
    trigger:
      - platform: time
        at: "02:00:00"
    action:
      - service: rest_command.plex_stop
      - service: rest_command.sonarr_stop
      - delay:
          minutes: 1
      - service: rest_command.radarr_stop
```

## Lovelace Dashboard

### Basic Server Card

```yaml
type: entities
title: Unraid Server
entities:
  - entity: sensor.unraid_cpu_usage
    name: CPU Usage
  - entity: sensor.unraid_ram_usage
    name: RAM Usage
  - entity: sensor.unraid_cpu_temperature
    name: CPU Temperature
  - entity: sensor.unraid_uptime
    name: Uptime
  - entity: sensor.unraid_array_usage
    name: Array Usage
  - entity: sensor.unraid_parity_status
    name: Parity Status
```

### Advanced Dashboard

```yaml
views:
  - title: Unraid
    path: unraid
    icon: mdi:server
    cards:
      # System Overview
      - type: vertical-stack
        cards:
          - type: custom:mini-graph-card
            name: CPU Usage
            entities:
              - entity: sensor.unraid_cpu_usage
            line_color: blue
            hours_to_show: 6
            points_per_hour: 10

          - type: custom:mini-graph-card
            name: RAM Usage
            entities:
              - entity: sensor.unraid_ram_usage
            line_color: green
            hours_to_show: 6

          - type: custom:mini-graph-card
            name: CPU Temperature
            entities:
              - entity: sensor.unraid_cpu_temperature
            line_color: red
            hours_to_show: 6

      # Array Status
      - type: entities
        title: Array Status
        entities:
          - entity: binary_sensor.unraid_array_started
          - entity: sensor.unraid_array_usage
          - entity: sensor.unraid_array_free_space
          - entity: sensor.unraid_parity_status
          - entity: binary_sensor.unraid_parity_check_running

      # Docker Containers
      - type: entities
        title: Docker Containers
        entities:
          - entity: sensor.unraid_containers_running
          - type: section
          - entity: switch.plex_container
          - entity: switch.sonarr_container
          - entity: switch.radarr_container

      # UPS Status
      - type: entities
        title: UPS
        entities:
          - entity: binary_sensor.ups_online
          - entity: sensor.ups_battery_level
          - entity: sensor.ups_runtime
          - entity: sensor.ups_load
```

### Button Card Example

```yaml
type: custom:button-card
entity: sensor.unraid_array_state
name: Unraid Array
icon: mdi:server
show_state: true
state:
  - value: STARTED
    color: green
  - value: STOPPED
    color: red
tap_action:
  action: more-info
```

## Notifications

### Mobile App Notifications

```yaml
automation:
  - alias: "Unraid Critical Notification"
    trigger:
      - platform: state
        entity_id: sensor.unraid_parity_status
        to: "Invalid"
    action:
      - service: notify.mobile_app_phone
        data:
          title: "🚨 Unraid Critical"
          message: "Parity is invalid!"
          data:
            ttl: 0
            priority: high
            channel: Critical
            group: unraid
            tag: parity
            actions:
              - action: "VIEW"
                title: "View Dashboard"
                uri: "/lovelace/unraid"
```

### Persistent Notification

```yaml
automation:
  - alias: "Unraid Parity Check Started"
    trigger:
      - platform: state
        entity_id: binary_sensor.unraid_parity_check_running
        to: "on"
    action:
      - service: persistent_notification.create
        data:
          title: "Unraid Parity Check"
          message: "Parity check has started. This may take several hours."
          notification_id: unraid_parity_check
```

## Advanced Features

### Template Sensors

Create derived sensors:

```yaml
template:
  - sensor:
      - name: "Unraid Health Score"
        unique_id: unraid_health_score
        state: >
          {% set cpu = states('sensor.unraid_cpu_usage') | float(0) %}
          {% set ram = states('sensor.unraid_ram_usage') | float(0) %}
          {% set temp = states('sensor.unraid_cpu_temperature') | float(0) %}
          {% set parity = 'Valid' if is_state('sensor.unraid_parity_status', 'Valid') else 'Invalid' %}

          {% set score = 100 %}
          {% set score = score - (10 if cpu > 80 else 0) %}
          {% set score = score - (10 if ram > 90 else 0) %}
          {% set score = score - (15 if temp > 75 else 0) %}
          {% set score = score - (50 if parity == 'Invalid' else 0) %}

          {{ score | int }}
        unit_of_measurement: "%"
        icon: mdi:heart-pulse
```

### Conditional Cards

Show cards only when relevant:

```yaml
type: conditional
conditions:
  - entity: binary_sensor.ups_online
    state: "off"
card:
  type: alert
  title: "⚠️ UPS Power Loss!"
  message: "Server is running on battery backup."
  severity: warning
```

## Troubleshooting

### Sensors Not Updating

1. **Check MQTT connection**:

   ```bash
   # In Home Assistant, check logs
   ha su logs
   ```

2. **Verify MQTT messages**:

   ```bash
   # Subscribe to all Unraid topics
   mosquitto_sub -h localhost -t "unraid/#" -v
   ```

3. **Check sensor configuration**:
   - Developer Tools → States
   - Search for "unraid"
   - Verify entities exist

### State Template Errors

If you see `TemplateError` in logs:

- Add default values: `| float(0)` or `| default('unknown')`
- Check JSON path: `value_json.field_name`
- Test templates in Developer Tools → Template

### Control Commands Not Working

For RESTful commands:

1. **Test endpoint directly**:

   ```bash
   curl -X POST http://unraid-server:8043/api/v1/docker/plex/start
   ```

2. **Check Home Assistant logs** for errors
3. **Verify network connectivity** from HA to Unraid

## Next Steps

- [MQTT Integration Guide](mqtt.md) - Configure MQTT publishing
- [REST API Reference](../api/rest-api.md) - Full API documentation
- [Grafana Dashboards](grafana.md) - Advanced monitoring
- [Troubleshooting](../troubleshooting/diagnostics.md) - Solve common issues

---

**Last Updated**: January 2026  
**Home Assistant Tested**: 2024.1+  
**MQTT Protocol**: 3.1.1
