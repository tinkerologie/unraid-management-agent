// Package mqtt provides MQTT client functionality for the Unraid Management Agent.
package mqtt

import (
	"fmt"
	"strings"
	"sync"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/services/controllers"
)

// haEntityOpts holds configuration for a single HA MQTT discovery entity.
type haEntityOpts struct {
	entityType     string // sensor, binary_sensor, switch, button
	stateTopic     string
	commandTopic   string // for switch and button entity types
	id             string
	name           string
	unit           string
	icon           string
	template       string
	deviceClass    string
	stateClass     string
	entityCategory string
	payloadOn      string // for binary_sensor and switch
	payloadOff     string // for binary_sensor and switch
	payloadPress   string // for button
	stateOn        string // for switch (value that means ON)
	stateOff       string // for switch (value that means OFF)
	optimistic     bool   // for switch (no state feedback)
}

// discoveryTracker tracks published per-item HA discovery entities
// so that removed items can have their discovery configs cleaned up.
type discoveryTracker struct {
	mu       sync.Mutex
	entities map[string]map[string]bool // category -> set of entity IDs
}

func newDiscoveryTracker() *discoveryTracker {
	return &discoveryTracker{
		entities: make(map[string]map[string]bool),
	}
}

// update records the current set of entity IDs for a category and returns
// any IDs that were previously registered but are no longer present.
func (t *discoveryTracker) update(category string, currentIDs []string) []string {
	t.mu.Lock()
	defer t.mu.Unlock()

	prev := t.entities[category]
	next := make(map[string]bool, len(currentIDs))
	for _, id := range currentIDs {
		next[id] = true
	}
	t.entities[category] = next

	var removed []string
	for id := range prev {
		if !next[id] {
			removed = append(removed, id)
		}
	}
	return removed
}

// publishHAEntity publishes a single Home Assistant discovery config.
func (c *Client) publishHAEntity(opts haEntityOpts) {
	hostID := strings.ReplaceAll(c.hostname, " ", "_")

	discoveryTopic := fmt.Sprintf("%s/%s/%s/%s/config",
		c.config.HADiscoveryPrefix,
		opts.entityType,
		hostID,
		opts.id,
	)

	config := map[string]any{
		"name":                  opts.name,
		"unique_id":             fmt.Sprintf("unraid_%s_%s", hostID, opts.id),
		"availability_topic":    c.buildTopic("availability"),
		"payload_available":     "online",
		"payload_not_available": "offline",
		"icon":                  opts.icon,
		"device":                c.deviceInfo,
	}

	// state_topic is used by sensor, binary_sensor, and switch (not button)
	if opts.entityType != "button" && opts.stateTopic != "" {
		config["state_topic"] = opts.stateTopic
	}

	// value_template for sensors and binary sensors
	if opts.template != "" && (opts.entityType == "sensor" || opts.entityType == "binary_sensor") {
		config["value_template"] = opts.template
	}

	if opts.unit != "" {
		config["unit_of_measurement"] = opts.unit
	}
	if opts.deviceClass != "" {
		config["device_class"] = opts.deviceClass
	}
	if opts.stateClass != "" {
		config["state_class"] = opts.stateClass
	}
	if opts.entityCategory != "" {
		config["entity_category"] = opts.entityCategory
	}

	// binary_sensor payloads
	if opts.entityType == "binary_sensor" {
		on := opts.payloadOn
		if on == "" {
			on = "ON"
		}
		off := opts.payloadOff
		if off == "" {
			off = "OFF"
		}
		config["payload_on"] = on
		config["payload_off"] = off
	}

	// switch-specific config
	if opts.entityType == "switch" {
		config["command_topic"] = opts.commandTopic
		on := opts.payloadOn
		if on == "" {
			on = "ON"
		}
		off := opts.payloadOff
		if off == "" {
			off = "OFF"
		}
		config["payload_on"] = on
		config["payload_off"] = off

		if opts.template != "" {
			config["value_template"] = opts.template
		}
		if opts.stateOn != "" {
			config["state_on"] = opts.stateOn
		}
		if opts.stateOff != "" {
			config["state_off"] = opts.stateOff
		}
		if opts.optimistic {
			config["optimistic"] = true
		}
	}

	// button-specific config
	if opts.entityType == "button" {
		config["command_topic"] = opts.commandTopic
		press := opts.payloadPress
		if press == "" {
			press = "PRESS"
		}
		config["payload_press"] = press
	}

	if err := c.publishJSON(discoveryTopic, config); err != nil {
		logger.Warning("MQTT: Failed to publish HA discovery for %s: %v", opts.id, err)
	}
}

// removeHAEntity removes a Home Assistant discovery entity by publishing empty payload.
func (c *Client) removeHAEntity(entityType, id string) {
	hostID := strings.ReplaceAll(c.hostname, " ", "_")
	discoveryTopic := fmt.Sprintf("%s/%s/%s/%s/config",
		c.config.HADiscoveryPrefix,
		entityType,
		hostID,
		id,
	)

	if err := c.publish(discoveryTopic, "", true); err != nil {
		logger.Debug("MQTT: Failed to remove HA entity %s: %v", id, err)
	}
}

// removeHAEntities removes HA discovery entities across all possible entity types.
func (c *Client) removeHAEntities(id string) {
	for _, t := range []string{"sensor", "binary_sensor", "switch", "button"} {
		c.removeHAEntity(t, id)
	}
}

// publishHADiscovery publishes all Home Assistant MQTT Discovery configurations.
func (c *Client) publishHADiscovery() {
	logger.Info("MQTT: Publishing Home Assistant discovery configurations...")

	c.publishSystemDiscovery()
	c.publishArrayDiscovery()
	c.publishUPSDiscovery()
	c.publishNotificationDiscovery()
	c.publishServiceDiscovery()
	c.publishSystemControlDiscovery()

	logger.Success("MQTT: Home Assistant discovery published")
}

// ──────────────────────────────────────────────────────────────────────────────
// System
// ──────────────────────────────────────────────────────────────────────────────

// publishSystemDiscovery publishes HA discovery for system metrics.
func (c *Client) publishSystemDiscovery() {
	topic := c.buildTopic("system")

	// CPU sensors
	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: "cpu_usage", name: "System: CPU Usage", unit: "%",
		icon: "mdi:cpu-64-bit", template: "{{ value_json.cpu_usage_percent | round(1) }}",
		stateClass: "measurement",
	})
	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: "cpu_temp", name: "System: CPU Temperature", unit: "°C",
		icon: "mdi:thermometer", template: "{{ value_json.cpu_temp_celsius }}",
		deviceClass: "temperature", stateClass: "measurement",
	})
	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: "cpu_mhz", name: "System: CPU Frequency", unit: "MHz",
		icon: "mdi:speedometer", template: "{{ value_json.cpu_mhz | round(0) }}",
		stateClass: "measurement",
	})
	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: "cpu_power", name: "System: CPU Power", unit: "W",
		icon: "mdi:lightning-bolt", template: "{{ value_json.cpu_power_watts | default(0) | round(1) }}",
		deviceClass: "power", stateClass: "measurement",
	})
	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: "dram_power", name: "System: DRAM Power", unit: "W",
		icon: "mdi:lightning-bolt", template: "{{ value_json.dram_power_watts | default(0) | round(1) }}",
		deviceClass: "power", stateClass: "measurement",
	})

	// RAM sensors
	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: "ram_usage", name: "System: RAM Usage", unit: "%",
		icon: "mdi:memory", template: "{{ value_json.ram_usage_percent | round(1) }}",
		stateClass: "measurement",
	})
	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: "ram_used", name: "System: RAM Used", unit: "B",
		icon: "mdi:memory", template: "{{ value_json.ram_used_bytes }}",
		deviceClass: "data_size", stateClass: "measurement",
	})
	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: "ram_free", name: "System: RAM Free", unit: "B",
		icon: "mdi:memory", template: "{{ value_json.ram_free_bytes }}",
		deviceClass: "data_size", stateClass: "measurement",
	})
	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: "ram_total", name: "System: RAM Total", unit: "B",
		icon: "mdi:memory", template: "{{ value_json.ram_total_bytes }}",
		deviceClass: "data_size", stateClass: "measurement", entityCategory: "diagnostic",
	})

	// Motherboard temperature
	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: "motherboard_temp", name: "System: Motherboard Temperature", unit: "°C",
		icon: "mdi:thermometer", template: "{{ value_json.motherboard_temp_celsius | default(0) }}",
		deviceClass: "temperature", stateClass: "measurement",
	})

	// Uptime
	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: "uptime", name: "System: Uptime", unit: "s",
		icon: "mdi:clock-outline", template: "{{ value_json.uptime_seconds }}",
		deviceClass: "duration", stateClass: "measurement",
	})

	// Version info (diagnostic)
	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: "unraid_version", name: "System: Unraid Version",
		icon: "mdi:information-outline", template: "{{ value_json.version }}",
		entityCategory: "diagnostic",
	})
	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: "agent_version", name: "System: Agent Version",
		icon: "mdi:information-outline", template: "{{ value_json.agent_version }}",
		entityCategory: "diagnostic",
	})
	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: "kernel_version", name: "System: Kernel Version",
		icon: "mdi:linux", template: "{{ value_json.kernel_version }}",
		entityCategory: "diagnostic",
	})

	// CPU info (diagnostic)
	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: "cpu_model", name: "System: CPU Model",
		icon: "mdi:cpu-64-bit", template: "{{ value_json.cpu_model }}",
		entityCategory: "diagnostic",
	})
	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: "cpu_cores", name: "System: CPU Cores",
		icon: "mdi:cpu-64-bit", template: "{{ value_json.cpu_cores }}",
		entityCategory: "diagnostic",
	})

	// Binary sensors
	c.publishHAEntity(haEntityOpts{
		entityType: "binary_sensor", stateTopic: topic,
		id: "hvm_support", name: "System: HVM Support",
		icon: "mdi:chip", template: "{{ 'ON' if value_json.hvm_enabled else 'OFF' }}",
		entityCategory: "diagnostic",
	})
	c.publishHAEntity(haEntityOpts{
		entityType: "binary_sensor", stateTopic: topic,
		id: "iommu_support", name: "System: IOMMU Support",
		icon: "mdi:chip", template: "{{ 'ON' if value_json.iommu_enabled else 'OFF' }}",
		entityCategory: "diagnostic",
	})
}

// ──────────────────────────────────────────────────────────────────────────────
// Array
// ──────────────────────────────────────────────────────────────────────────────

// publishArrayDiscovery publishes HA discovery for array metrics.
func (c *Client) publishArrayDiscovery() {
	topic := c.buildTopic("array")

	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: "array_state", name: "Array: State",
		icon: "mdi:server", template: "{{ value_json.state }}",
	})
	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: "array_usage", name: "Array: Usage", unit: "%",
		icon: "mdi:chart-pie", template: "{{ value_json.used_percent | round(1) }}",
		stateClass: "measurement",
	})
	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: "array_free", name: "Array: Free Space", unit: "B",
		icon: "mdi:harddisk", template: "{{ value_json.free_bytes }}",
		deviceClass: "data_size", stateClass: "measurement",
	})
	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: "array_total", name: "Array: Total Space", unit: "B",
		icon: "mdi:harddisk", template: "{{ value_json.total_bytes }}",
		deviceClass: "data_size", stateClass: "measurement", entityCategory: "diagnostic",
	})
	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: "array_num_disks", name: "Array: Disk Count",
		icon: "mdi:harddisk", template: "{{ value_json.num_disks }}",
		entityCategory: "diagnostic",
	})
	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: "parity_status", name: "Array: Parity Status",
		icon: "mdi:shield-check", template: "{{ value_json.parity_check_status | default('idle') }}",
	})
	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: "parity_progress", name: "Array: Parity Progress", unit: "%",
		icon: "mdi:progress-check", template: "{{ value_json.parity_check_progress | default(0) | round(1) }}",
		stateClass: "measurement",
	})

	// Binary sensors
	c.publishHAEntity(haEntityOpts{
		entityType: "binary_sensor", stateTopic: topic,
		id: "parity_valid", name: "Array: Parity Valid",
		icon: "mdi:shield-check", template: "{{ 'ON' if value_json.parity_valid else 'OFF' }}",
		deviceClass: "safety",
	})
	c.publishHAEntity(haEntityOpts{
		entityType: "binary_sensor", stateTopic: topic,
		id: "array_started", name: "Array: Started",
		icon: "mdi:server", template: "{{ 'ON' if value_json.state == 'Started' else 'OFF' }}",
		deviceClass: "running",
	})

	// Array switch (start/stop)
	c.publishHAEntity(haEntityOpts{
		entityType: "switch", stateTopic: topic,
		commandTopic: c.buildCommandTopic("array", "set"),
		id:           "array_switch", name: "Array: Power",
		icon: "mdi:server", template: "{{ value_json.state }}",
		stateOn: "STARTED", stateOff: "STOPPED",
	})

	// Parity buttons
	c.publishHAEntity(haEntityOpts{
		entityType:   "button",
		commandTopic: c.buildCommandTopic("array", "parity", "start"),
		id:           "parity_start", name: "Array: Start Parity Check",
		icon: "mdi:shield-sync",
	})
	c.publishHAEntity(haEntityOpts{
		entityType:   "button",
		commandTopic: c.buildCommandTopic("array", "parity", "stop"),
		id:           "parity_stop", name: "Array: Stop Parity Check",
		icon: "mdi:shield-off",
	})
	c.publishHAEntity(haEntityOpts{
		entityType:   "button",
		commandTopic: c.buildCommandTopic("array", "parity", "pause"),
		id:           "parity_pause", name: "Array: Pause Parity Check",
		icon: "mdi:pause-circle",
	})
	c.publishHAEntity(haEntityOpts{
		entityType:   "button",
		commandTopic: c.buildCommandTopic("array", "parity", "resume"),
		id:           "parity_resume", name: "Array: Resume Parity Check",
		icon: "mdi:play-circle",
	})
}

// ──────────────────────────────────────────────────────────────────────────────
// UPS
// ──────────────────────────────────────────────────────────────────────────────

// publishUPSDiscovery publishes HA discovery for UPS metrics.
func (c *Client) publishUPSDiscovery() {
	topic := c.buildTopic("ups")

	c.publishHAEntity(haEntityOpts{
		entityType: "binary_sensor", stateTopic: topic,
		id: "ups_connected", name: "UPS: Connected",
		icon: "mdi:battery-charging", template: "{{ 'ON' if value_json.connected else 'OFF' }}",
		deviceClass: "connectivity",
	})
	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: "ups_status", name: "UPS: Status",
		icon: "mdi:battery-charging", template: "{{ value_json.status }}",
	})
	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: "ups_load", name: "UPS: Load", unit: "%",
		icon: "mdi:gauge", template: "{{ value_json.load_percent | round(1) }}",
		stateClass: "measurement",
	})
	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: "ups_battery", name: "UPS: Battery Level", unit: "%",
		icon: "mdi:battery", template: "{{ value_json.battery_charge_percent | round(0) }}",
		deviceClass: "battery", stateClass: "measurement",
	})
	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: "ups_runtime", name: "UPS: Runtime Remaining", unit: "s",
		icon: "mdi:clock-outline", template: "{{ value_json.runtime_left_seconds }}",
		deviceClass: "duration", stateClass: "measurement",
	})
	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: "ups_power", name: "UPS: Power Draw", unit: "W",
		icon: "mdi:lightning-bolt", template: "{{ value_json.nominal_power_watts | default(0) | round(0) }}",
		deviceClass: "power", stateClass: "measurement",
	})
	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: "ups_model", name: "UPS: Model",
		icon: "mdi:battery-charging", template: "{{ value_json.model }}",
		entityCategory: "diagnostic",
	})
}

// ──────────────────────────────────────────────────────────────────────────────
// Notifications
// ──────────────────────────────────────────────────────────────────────────────

// publishNotificationDiscovery publishes HA discovery for notification counts.
func (c *Client) publishNotificationDiscovery() {
	topic := c.buildTopic("notifications")

	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: "notif_unread", name: "Notifications: Unread",
		icon: "mdi:bell-badge", template: "{{ value_json.overview.unread.total | default(0) }}",
		stateClass: "measurement",
	})
	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: "notif_alerts", name: "Notifications: Alerts",
		icon: "mdi:alert-circle", template: "{{ value_json.overview.unread.alert | default(0) }}",
		stateClass: "measurement",
	})
	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: "notif_warnings", name: "Notifications: Warnings",
		icon: "mdi:alert", template: "{{ value_json.overview.unread.warning | default(0) }}",
		stateClass: "measurement",
	})
	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: "notif_info", name: "Notifications: Info",
		icon: "mdi:information", template: "{{ value_json.overview.unread.info | default(0) }}",
		stateClass: "measurement",
	})

	// Archive all notifications button
	c.publishHAEntity(haEntityOpts{
		entityType:   "button",
		commandTopic: c.buildCommandTopic("notifications", "archive_all"),
		id:           "notif_archive_all", name: "Notifications: Archive All",
		icon: "mdi:archive-arrow-down",
	})
}

// ──────────────────────────────────────────────────────────────────────────────
// Services
// ──────────────────────────────────────────────────────────────────────────────

// publishServiceDiscovery publishes HA discovery for service switches.
func (c *Client) publishServiceDiscovery() {
	services := controllers.ValidServiceNames()
	servicesTopic := c.buildTopic("services")

	for _, svc := range services {
		svcID := sanitizeID(svc)
		displayName := serviceDisplayName(svc)

		c.publishHAEntity(haEntityOpts{
			entityType:   "switch",
			stateTopic:   servicesTopic,
			commandTopic: c.buildCommandTopic("service", svcID, "set"),
			id:           fmt.Sprintf("service_%s_switch", svcID),
			name:         fmt.Sprintf("Service: %s", displayName),
			icon:         serviceIcon(svc),
			template:     fmt.Sprintf("{{ 'ON' if value_json.%s else 'OFF' }}", svc),
		})
	}
}

// publishServiceStates queries all service running states and publishes
// them to the services topic so HA switches reflect the actual state.
func (c *Client) publishServiceStates() {
	ctrl := controllers.NewServiceController()
	services := controllers.ValidServiceNames()
	states := make(map[string]bool, len(services))

	for _, svc := range services {
		running, err := ctrl.GetServiceStatus(svc)
		if err != nil {
			logger.Debug("MQTT: Failed to check service %s status: %v", svc, err)
			continue
		}
		states[svc] = running
	}

	topic := c.buildTopic("services")
	if err := c.publishJSON(topic, states); err != nil {
		logger.Warning("MQTT: Failed to publish service states: %v", err)
	}
}

// serviceDisplayName returns a human-friendly display name for a service.
func serviceDisplayName(svc string) string {
	names := map[string]string{
		"docker":    "Docker",
		"libvirt":   "Libvirt",
		"smb":       "Samba (SMB)",
		"nfs":       "NFS",
		"ftp":       "FTP",
		"sshd":      "SSH",
		"nginx":     "Nginx",
		"syslog":    "Syslog",
		"ntpd":      "NTP",
		"avahi":     "Avahi",
		"wireguard": "WireGuard",
	}
	if name, ok := names[svc]; ok {
		return name
	}
	return svc
}

// serviceIcon returns an MDI icon for a service.
func serviceIcon(svc string) string {
	icons := map[string]string{
		"docker":    "mdi:docker",
		"libvirt":   "mdi:desktop-classic",
		"smb":       "mdi:folder-network",
		"nfs":       "mdi:folder-network-outline",
		"ftp":       "mdi:file-upload",
		"sshd":      "mdi:console",
		"nginx":     "mdi:web",
		"syslog":    "mdi:math-log",
		"ntpd":      "mdi:clock-outline",
		"avahi":     "mdi:access-point",
		"wireguard": "mdi:vpn",
	}
	if icon, ok := icons[svc]; ok {
		return icon
	}
	return "mdi:cog"
}

// ──────────────────────────────────────────────────────────────────────────────
// System Controls (Reboot/Shutdown)
// ──────────────────────────────────────────────────────────────────────────────

// publishSystemControlDiscovery publishes HA discovery for system control buttons.
func (c *Client) publishSystemControlDiscovery() {
	c.publishHAEntity(haEntityOpts{
		entityType:   "button",
		commandTopic: c.buildCommandTopic("system", "reboot"),
		id:           "system_reboot", name: "System: Reboot",
		icon:        "mdi:restart",
		deviceClass: "restart",
	})
	c.publishHAEntity(haEntityOpts{
		entityType:   "button",
		commandTopic: c.buildCommandTopic("system", "shutdown"),
		id:           "system_shutdown", name: "System: Shutdown",
		icon: "mdi:power",
	})
}

// ──────────────────────────────────────────────────────────────────────────────
// Disks (per-item)
// ──────────────────────────────────────────────────────────────────────────────

// publishDiskDiscovery publishes per-disk HA discovery entities.
func (c *Client) publishDiskDiscovery(disks []dto.DiskInfo) {
	if !c.config.HomeAssistantMode {
		return
	}

	var currentIDs []string

	for _, disk := range disks {
		if disk.ID == "" {
			continue
		}
		diskID := sanitizeID(disk.ID)
		diskTopic := c.buildTopic(fmt.Sprintf("disk/%s", diskID))

		if err := c.publishJSON(diskTopic, disk); err != nil {
			logger.Debug("MQTT: Failed to publish disk %s: %v", diskID, err)
			continue
		}

		prefix := fmt.Sprintf("disk_%s", diskID)
		displayName := disk.Name
		if displayName == "" {
			displayName = disk.ID
		}

		ids := c.publishDiskEntities(diskTopic, prefix, displayName, diskID)
		currentIDs = append(currentIDs, ids...)
	}

	removed := c.tracker.update("disks", currentIDs)
	for _, id := range removed {
		c.removeHAEntities(id)
	}
}

// publishDiskEntities publishes HA discovery entities for a single disk.
func (c *Client) publishDiskEntities(topic, prefix, displayName, diskID string) []string {
	ids := []string{
		prefix + "_temp",
		prefix + "_status",
		prefix + "_smart_status",
		prefix + "_usage",
		prefix + "_used",
		prefix + "_free",
		prefix + "_spin_state",
		prefix + "_power_hours",
		prefix + "_io_util",
		prefix + "_healthy",
		prefix + "_spin_up",
		prefix + "_spin_down",
	}

	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: prefix + "_temp", name: fmt.Sprintf("Disk: %s Temperature", displayName), unit: "°C",
		icon: "mdi:thermometer", template: "{{ value_json.temperature_celsius }}",
		deviceClass: "temperature", stateClass: "measurement",
	})
	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: prefix + "_status", name: fmt.Sprintf("Disk: %s Status", displayName),
		icon: "mdi:harddisk", template: "{{ value_json.status }}",
	})
	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: prefix + "_smart_status", name: fmt.Sprintf("Disk: %s SMART Status", displayName),
		icon: "mdi:harddisk", template: "{{ value_json.smart_status }}",
		entityCategory: "diagnostic",
	})
	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: prefix + "_usage", name: fmt.Sprintf("Disk: %s Usage", displayName), unit: "%",
		icon: "mdi:chart-pie", template: "{{ value_json.usage_percent | default(0) | round(1) }}",
		stateClass: "measurement",
	})
	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: prefix + "_used", name: fmt.Sprintf("Disk: %s Used", displayName), unit: "B",
		icon: "mdi:harddisk", template: "{{ value_json.used_bytes }}",
		deviceClass: "data_size", stateClass: "measurement",
	})
	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: prefix + "_free", name: fmt.Sprintf("Disk: %s Free", displayName), unit: "B",
		icon: "mdi:harddisk", template: "{{ value_json.free_bytes }}",
		deviceClass: "data_size", stateClass: "measurement",
	})
	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: prefix + "_spin_state", name: fmt.Sprintf("Disk: %s Spin State", displayName),
		icon: "mdi:rotate-3d-variant", template: "{{ value_json.spin_state | default('unknown') }}",
	})
	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: prefix + "_power_hours", name: fmt.Sprintf("Disk: %s Power On Hours", displayName), unit: "h",
		icon: "mdi:clock-outline", template: "{{ value_json.power_on_hours | default(0) }}",
		deviceClass: "duration", stateClass: "total_increasing",
	})
	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: prefix + "_io_util", name: fmt.Sprintf("Disk: %s I/O Utilization", displayName), unit: "%",
		icon: "mdi:speedometer", template: "{{ value_json.io_utilization_percent | default(0) | round(1) }}",
		stateClass: "measurement",
	})
	c.publishHAEntity(haEntityOpts{
		entityType: "binary_sensor", stateTopic: topic,
		id: prefix + "_healthy", name: fmt.Sprintf("Disk: %s Healthy", displayName),
		icon: "mdi:check-circle", template: "{{ 'ON' if value_json.smart_status == 'PASSED' else 'OFF' }}",
		deviceClass: "safety",
	})

	// Disk spin buttons
	c.publishHAEntity(haEntityOpts{
		entityType:   "button",
		commandTopic: c.buildCommandTopic("disk", diskID, "spin_up"),
		id:           prefix + "_spin_up", name: fmt.Sprintf("Disk: %s Spin Up", displayName),
		icon: "mdi:rotate-right",
	})
	c.publishHAEntity(haEntityOpts{
		entityType:   "button",
		commandTopic: c.buildCommandTopic("disk", diskID, "spin_down"),
		id:           prefix + "_spin_down", name: fmt.Sprintf("Disk: %s Spin Down", displayName),
		icon: "mdi:stop-circle",
	})

	return ids
}

// ──────────────────────────────────────────────────────────────────────────────
// Docker Containers (per-item)
// ──────────────────────────────────────────────────────────────────────────────

// publishContainerDiscovery publishes per-container HA discovery entities.
func (c *Client) publishContainerDiscovery(containers []dto.ContainerInfo) {
	if !c.config.HomeAssistantMode {
		return
	}

	var currentIDs []string

	containersTopic := c.buildTopic("docker/containers")
	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: containersTopic,
		id: "docker_total", name: "Docker: Total Containers",
		icon: "mdi:docker", template: "{{ value_json | length }}",
		stateClass: "measurement",
	})
	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: containersTopic,
		id: "docker_running", name: "Docker: Running Containers",
		icon: "mdi:docker", template: "{{ value_json | selectattr('state', 'eq', 'running') | list | length }}",
		stateClass: "measurement",
	})
	currentIDs = append(currentIDs, "docker_total", "docker_running")

	for _, container := range containers {
		nameID := sanitizeID(container.Name)
		containerTopic := c.buildTopic(fmt.Sprintf("docker/%s", nameID))

		if err := c.publishJSON(containerTopic, container); err != nil {
			logger.Debug("MQTT: Failed to publish container %s: %v", nameID, err)
			continue
		}

		prefix := fmt.Sprintf("container_%s", nameID)

		ids := c.publishContainerEntities(containerTopic, prefix, container.Name, nameID)
		currentIDs = append(currentIDs, ids...)
	}

	removed := c.tracker.update("containers", currentIDs)
	for _, id := range removed {
		c.removeHAEntities(id)
	}
}

// publishContainerEntities publishes HA discovery entities for a single container.
func (c *Client) publishContainerEntities(topic, prefix, displayName, nameID string) []string {
	ids := []string{
		prefix + "_state",
		prefix + "_cpu",
		prefix + "_memory",
		prefix + "_net_rx",
		prefix + "_net_tx",
		prefix + "_switch",
		prefix + "_restart",
		prefix + "_pause",
		prefix + "_unpause",
	}

	c.publishHAEntity(haEntityOpts{
		entityType: "binary_sensor", stateTopic: topic,
		id: prefix + "_state", name: fmt.Sprintf("Docker: %s Running", displayName),
		icon: "mdi:docker", template: "{{ 'ON' if value_json.state == 'running' else 'OFF' }}",
		deviceClass: "running",
	})
	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: prefix + "_cpu", name: fmt.Sprintf("Docker: %s CPU", displayName), unit: "%",
		icon: "mdi:cpu-64-bit", template: "{{ value_json.cpu_percent | round(1) }}",
		stateClass: "measurement",
	})
	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: prefix + "_memory", name: fmt.Sprintf("Docker: %s Memory", displayName), unit: "B",
		icon: "mdi:memory", template: "{{ value_json.memory_usage_bytes }}",
		deviceClass: "data_size", stateClass: "measurement",
	})
	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: prefix + "_net_rx", name: fmt.Sprintf("Docker: %s Network RX", displayName), unit: "B",
		icon: "mdi:download", template: "{{ value_json.network_rx_bytes }}",
		deviceClass: "data_size", stateClass: "total_increasing",
	})
	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: prefix + "_net_tx", name: fmt.Sprintf("Docker: %s Network TX", displayName), unit: "B",
		icon: "mdi:upload", template: "{{ value_json.network_tx_bytes }}",
		deviceClass: "data_size", stateClass: "total_increasing",
	})

	// Power switch (start/stop)
	c.publishHAEntity(haEntityOpts{
		entityType: "switch", stateTopic: topic,
		commandTopic: c.buildCommandTopic("docker", nameID, "set"),
		id:           prefix + "_switch", name: fmt.Sprintf("Docker: %s Power", displayName),
		icon: "mdi:docker", template: "{{ value_json.state }}",
		stateOn: "running", stateOff: "exited",
	})

	// Action buttons
	c.publishHAEntity(haEntityOpts{
		entityType:   "button",
		commandTopic: c.buildCommandTopic("docker", nameID, "restart"),
		id:           prefix + "_restart", name: fmt.Sprintf("Docker: %s Restart", displayName),
		icon:        "mdi:restart",
		deviceClass: "restart",
	})
	c.publishHAEntity(haEntityOpts{
		entityType:   "button",
		commandTopic: c.buildCommandTopic("docker", nameID, "pause"),
		id:           prefix + "_pause", name: fmt.Sprintf("Docker: %s Pause", displayName),
		icon: "mdi:pause-circle",
	})
	c.publishHAEntity(haEntityOpts{
		entityType:   "button",
		commandTopic: c.buildCommandTopic("docker", nameID, "unpause"),
		id:           prefix + "_unpause", name: fmt.Sprintf("Docker: %s Unpause", displayName),
		icon: "mdi:play-circle",
	})

	return ids
}

// ──────────────────────────────────────────────────────────────────────────────
// VMs (per-item)
// ──────────────────────────────────────────────────────────────────────────────

// publishVMDiscovery publishes per-VM HA discovery entities.
func (c *Client) publishVMDiscovery(vms []dto.VMInfo) {
	if !c.config.HomeAssistantMode {
		return
	}

	var currentIDs []string

	vmsTopic := c.buildTopic("vm/list")
	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: vmsTopic,
		id: "vm_total", name: "VM: Total",
		icon: "mdi:desktop-classic", template: "{{ value_json | length }}",
		stateClass: "measurement",
	})
	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: vmsTopic,
		id: "vm_running", name: "VM: Running",
		icon: "mdi:desktop-classic", template: "{{ value_json | selectattr('state', 'eq', 'running') | list | length }}",
		stateClass: "measurement",
	})
	currentIDs = append(currentIDs, "vm_total", "vm_running")

	for _, vm := range vms {
		nameID := sanitizeID(vm.Name)
		vmTopic := c.buildTopic(fmt.Sprintf("vm/%s", nameID))

		if err := c.publishJSON(vmTopic, vm); err != nil {
			logger.Debug("MQTT: Failed to publish VM %s: %v", nameID, err)
			continue
		}

		prefix := fmt.Sprintf("vm_%s", nameID)

		ids := c.publishVMEntities(vmTopic, prefix, vm.Name, nameID)
		currentIDs = append(currentIDs, ids...)
	}

	removed := c.tracker.update("vms", currentIDs)
	for _, id := range removed {
		c.removeHAEntities(id)
	}
}

// publishVMEntities publishes HA discovery entities for a single VM.
func (c *Client) publishVMEntities(topic, prefix, displayName, nameID string) []string {
	ids := []string{
		prefix + "_state",
		prefix + "_guest_cpu",
		prefix + "_host_cpu",
		prefix + "_memory_used",
		prefix + "_memory_allocated",
		prefix + "_switch",
		prefix + "_restart",
		prefix + "_pause",
		prefix + "_resume",
		prefix + "_hibernate",
		prefix + "_force_stop",
	}

	c.publishHAEntity(haEntityOpts{
		entityType: "binary_sensor", stateTopic: topic,
		id: prefix + "_state", name: fmt.Sprintf("VM: %s Running", displayName),
		icon: "mdi:desktop-classic", template: "{{ 'ON' if value_json.state == 'running' else 'OFF' }}",
		deviceClass: "running",
	})
	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: prefix + "_guest_cpu", name: fmt.Sprintf("VM: %s Guest CPU", displayName), unit: "%",
		icon: "mdi:cpu-64-bit", template: "{{ value_json.guest_cpu_percent | round(1) }}",
		stateClass: "measurement",
	})
	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: prefix + "_host_cpu", name: fmt.Sprintf("VM: %s Host CPU", displayName), unit: "%",
		icon: "mdi:cpu-64-bit", template: "{{ value_json.host_cpu_percent | round(1) }}",
		stateClass: "measurement",
	})
	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: prefix + "_memory_used", name: fmt.Sprintf("VM: %s Memory Used", displayName), unit: "B",
		icon: "mdi:memory", template: "{{ value_json.memory_used_bytes }}",
		deviceClass: "data_size", stateClass: "measurement",
	})
	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: prefix + "_memory_allocated", name: fmt.Sprintf("VM: %s Memory Allocated", displayName), unit: "B",
		icon: "mdi:memory", template: "{{ value_json.memory_allocated_bytes }}",
		deviceClass: "data_size", entityCategory: "diagnostic",
	})

	// Power switch (start/stop)
	c.publishHAEntity(haEntityOpts{
		entityType: "switch", stateTopic: topic,
		commandTopic: c.buildCommandTopic("vm", nameID, "set"),
		id:           prefix + "_switch", name: fmt.Sprintf("VM: %s Power", displayName),
		icon: "mdi:desktop-classic", template: "{{ value_json.state }}",
		stateOn: "running", stateOff: "shut off",
	})

	// Action buttons
	c.publishHAEntity(haEntityOpts{
		entityType:   "button",
		commandTopic: c.buildCommandTopic("vm", nameID, "restart"),
		id:           prefix + "_restart", name: fmt.Sprintf("VM: %s Restart", displayName),
		icon:        "mdi:restart",
		deviceClass: "restart",
	})
	c.publishHAEntity(haEntityOpts{
		entityType:   "button",
		commandTopic: c.buildCommandTopic("vm", nameID, "pause"),
		id:           prefix + "_pause", name: fmt.Sprintf("VM: %s Pause", displayName),
		icon: "mdi:pause-circle",
	})
	c.publishHAEntity(haEntityOpts{
		entityType:   "button",
		commandTopic: c.buildCommandTopic("vm", nameID, "resume"),
		id:           prefix + "_resume", name: fmt.Sprintf("VM: %s Resume", displayName),
		icon: "mdi:play-circle",
	})
	c.publishHAEntity(haEntityOpts{
		entityType:   "button",
		commandTopic: c.buildCommandTopic("vm", nameID, "hibernate"),
		id:           prefix + "_hibernate", name: fmt.Sprintf("VM: %s Hibernate", displayName),
		icon: "mdi:power-sleep",
	})
	c.publishHAEntity(haEntityOpts{
		entityType:   "button",
		commandTopic: c.buildCommandTopic("vm", nameID, "force_stop"),
		id:           prefix + "_force_stop", name: fmt.Sprintf("VM: %s Force Stop", displayName),
		icon: "mdi:power-off",
	})

	return ids
}

// ──────────────────────────────────────────────────────────────────────────────
// GPU (per-item)
// ──────────────────────────────────────────────────────────────────────────────

// publishGPUDiscovery publishes per-GPU HA discovery entities.
func (c *Client) publishGPUDiscovery(gpus []*dto.GPUMetrics) {
	if !c.config.HomeAssistantMode {
		return
	}

	var currentIDs []string

	for _, gpu := range gpus {
		if gpu == nil || !gpu.Available {
			continue
		}
		gpuID := sanitizeID(fmt.Sprintf("%d", gpu.Index))
		gpuTopic := c.buildTopic(fmt.Sprintf("gpu/%s", gpuID))

		if err := c.publishJSON(gpuTopic, gpu); err != nil {
			logger.Debug("MQTT: Failed to publish GPU %s: %v", gpuID, err)
			continue
		}

		prefix := fmt.Sprintf("gpu_%s", gpuID)
		displayName := gpu.Name
		if displayName == "" {
			displayName = fmt.Sprintf("GPU %d", gpu.Index)
		}

		ids := c.publishGPUEntities(gpuTopic, prefix, displayName)
		currentIDs = append(currentIDs, ids...)
	}

	removed := c.tracker.update("gpus", currentIDs)
	for _, id := range removed {
		c.removeHAEntities(id)
	}
}

// publishGPUEntities publishes HA discovery entities for a single GPU.
func (c *Client) publishGPUEntities(topic, prefix, displayName string) []string {
	ids := []string{
		prefix + "_temp",
		prefix + "_util",
		prefix + "_mem_util",
		prefix + "_mem_used",
		prefix + "_power",
		prefix + "_fan",
	}

	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: prefix + "_temp", name: fmt.Sprintf("GPU: %s Temperature", displayName), unit: "°C",
		icon: "mdi:thermometer", template: "{{ value_json.temperature_celsius }}",
		deviceClass: "temperature", stateClass: "measurement",
	})
	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: prefix + "_util", name: fmt.Sprintf("GPU: %s Utilization", displayName), unit: "%",
		icon: "mdi:expansion-card", template: "{{ value_json.utilization_gpu_percent | round(1) }}",
		stateClass: "measurement",
	})
	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: prefix + "_mem_util", name: fmt.Sprintf("GPU: %s Memory Utilization", displayName), unit: "%",
		icon: "mdi:expansion-card", template: "{{ value_json.utilization_memory_percent | round(1) }}",
		stateClass: "measurement",
	})
	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: prefix + "_mem_used", name: fmt.Sprintf("GPU: %s Memory Used", displayName), unit: "B",
		icon: "mdi:memory", template: "{{ value_json.memory_used_bytes }}",
		deviceClass: "data_size", stateClass: "measurement",
	})
	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: prefix + "_power", name: fmt.Sprintf("GPU: %s Power Draw", displayName), unit: "W",
		icon: "mdi:lightning-bolt", template: "{{ value_json.power_draw_watts | round(1) }}",
		deviceClass: "power", stateClass: "measurement",
	})
	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: prefix + "_fan", name: fmt.Sprintf("GPU: %s Fan Speed", displayName), unit: "%",
		icon: "mdi:fan", template: "{{ value_json.fan_speed_percent | default(0) | round(0) }}",
		stateClass: "measurement",
	})

	return ids
}

// ──────────────────────────────────────────────────────────────────────────────
// Network (per-item)
// ──────────────────────────────────────────────────────────────────────────────

// isPhysicalInterface returns true if the interface is a physical or meaningful
// network interface that should be exposed as an HA entity.
func isPhysicalInterface(name string) bool {
	if strings.HasPrefix(name, "veth") {
		return false
	}
	if strings.HasPrefix(name, "tunl") || strings.HasPrefix(name, "tun") {
		return false
	}
	if strings.HasPrefix(name, "virbr") {
		return false
	}
	if name == "docker0" {
		return false
	}
	if strings.HasPrefix(name, "br-") || strings.HasPrefix(name, "br_") {
		return false
	}
	if strings.HasPrefix(name, "shim-") || strings.HasPrefix(name, "shim_") {
		return false
	}
	if strings.HasPrefix(name, "vhost") {
		return false
	}
	return true
}

// publishNetworkDiscovery publishes per-network-interface HA discovery entities.
func (c *Client) publishNetworkDiscovery(interfaces []dto.NetworkInfo) {
	if !c.config.HomeAssistantMode {
		return
	}

	var currentIDs []string

	for _, iface := range interfaces {
		if !isPhysicalInterface(iface.Name) {
			continue
		}

		ifaceID := sanitizeID(iface.Name)
		ifaceTopic := c.buildTopic(fmt.Sprintf("network/%s", ifaceID))

		if err := c.publishJSON(ifaceTopic, iface); err != nil {
			logger.Debug("MQTT: Failed to publish network %s: %v", ifaceID, err)
			continue
		}

		prefix := fmt.Sprintf("net_%s", ifaceID)
		displayName := iface.Name

		ids := c.publishNetworkEntities(ifaceTopic, prefix, displayName)
		currentIDs = append(currentIDs, ids...)
	}

	removed := c.tracker.update("network", currentIDs)
	for _, id := range removed {
		c.removeHAEntities(id)
	}
}

// publishNetworkEntities publishes HA discovery entities for a single network interface.
func (c *Client) publishNetworkEntities(topic, prefix, displayName string) []string {
	ids := []string{
		prefix + "_state",
		prefix + "_speed",
		prefix + "_rx",
		prefix + "_tx",
		prefix + "_errors_rx",
		prefix + "_errors_tx",
	}

	c.publishHAEntity(haEntityOpts{
		entityType: "binary_sensor", stateTopic: topic,
		id: prefix + "_state", name: fmt.Sprintf("Network: %s Link", displayName),
		icon: "mdi:ethernet", template: "{{ 'ON' if value_json.state == 'up' else 'OFF' }}",
		deviceClass: "connectivity",
	})
	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: prefix + "_speed", name: fmt.Sprintf("Network: %s Speed", displayName), unit: "Mbit/s",
		icon: "mdi:speedometer", template: "{{ value_json.speed_mbps }}",
		entityCategory: "diagnostic",
	})
	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: prefix + "_rx", name: fmt.Sprintf("Network: %s Bytes Received", displayName), unit: "B",
		icon: "mdi:download", template: "{{ value_json.bytes_received }}",
		deviceClass: "data_size", stateClass: "total_increasing",
	})
	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: prefix + "_tx", name: fmt.Sprintf("Network: %s Bytes Sent", displayName), unit: "B",
		icon: "mdi:upload", template: "{{ value_json.bytes_sent }}",
		deviceClass: "data_size", stateClass: "total_increasing",
	})
	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: prefix + "_errors_rx", name: fmt.Sprintf("Network: %s RX Errors", displayName),
		icon: "mdi:alert-circle", template: "{{ value_json.errors_received }}",
		stateClass: "total_increasing", entityCategory: "diagnostic",
	})
	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: prefix + "_errors_tx", name: fmt.Sprintf("Network: %s TX Errors", displayName),
		icon: "mdi:alert-circle", template: "{{ value_json.errors_sent }}",
		stateClass: "total_increasing", entityCategory: "diagnostic",
	})

	return ids
}

// ──────────────────────────────────────────────────────────────────────────────
// Shares (per-item)
// ──────────────────────────────────────────────────────────────────────────────

// publishShareDiscovery publishes per-share HA discovery entities.
func (c *Client) publishShareDiscovery(shares []dto.ShareInfo) {
	if !c.config.HomeAssistantMode {
		return
	}

	var currentIDs []string

	for _, share := range shares {
		shareID := sanitizeID(share.Name)
		shareTopic := c.buildTopic(fmt.Sprintf("shares/%s", shareID))

		if err := c.publishJSON(shareTopic, share); err != nil {
			logger.Debug("MQTT: Failed to publish share %s: %v", shareID, err)
			continue
		}

		prefix := fmt.Sprintf("share_%s", shareID)
		displayName := share.Name

		ids := c.publishShareEntities(shareTopic, prefix, displayName)
		currentIDs = append(currentIDs, ids...)
	}

	removed := c.tracker.update("shares", currentIDs)
	for _, id := range removed {
		c.removeHAEntities(id)
	}
}

// publishShareEntities publishes HA discovery entities for a single share.
func (c *Client) publishShareEntities(topic, prefix, displayName string) []string {
	ids := []string{
		prefix + "_usage",
		prefix + "_used",
		prefix + "_free",
	}

	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: prefix + "_usage", name: fmt.Sprintf("Share: %s Usage", displayName), unit: "%",
		icon: "mdi:folder", template: "{{ value_json.usage_percent | round(1) }}",
		stateClass: "measurement",
	})
	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: prefix + "_used", name: fmt.Sprintf("Share: %s Used", displayName), unit: "B",
		icon: "mdi:folder", template: "{{ value_json.used_bytes }}",
		deviceClass: "data_size", stateClass: "measurement",
	})
	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: prefix + "_free", name: fmt.Sprintf("Share: %s Free", displayName), unit: "B",
		icon: "mdi:folder", template: "{{ value_json.free_bytes }}",
		deviceClass: "data_size", stateClass: "measurement",
	})

	return ids
}

// ──────────────────────────────────────────────────────────────────────────────
// ZFS (per-item)
// ──────────────────────────────────────────────────────────────────────────────

// publishZFSDiscovery publishes per-pool HA discovery entities.
func (c *Client) publishZFSDiscovery(pools []dto.ZFSPool) {
	if !c.config.HomeAssistantMode {
		return
	}

	var currentIDs []string

	for _, pool := range pools {
		poolID := sanitizeID(pool.Name)
		poolTopic := c.buildTopic(fmt.Sprintf("zfs/%s", poolID))

		if err := c.publishJSON(poolTopic, pool); err != nil {
			logger.Debug("MQTT: Failed to publish ZFS pool %s: %v", poolID, err)
			continue
		}

		prefix := fmt.Sprintf("zfs_%s", poolID)
		displayName := pool.Name

		ids := c.publishZFSEntities(poolTopic, prefix, displayName)
		currentIDs = append(currentIDs, ids...)
	}

	removed := c.tracker.update("zfs", currentIDs)
	for _, id := range removed {
		c.removeHAEntities(id)
	}
}

// publishZFSEntities publishes HA discovery entities for a single ZFS pool.
func (c *Client) publishZFSEntities(topic, prefix, displayName string) []string {
	ids := []string{
		prefix + "_health",
		prefix + "_capacity",
		prefix + "_free",
		prefix + "_fragmentation",
		prefix + "_errors",
		prefix + "_healthy",
	}

	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: prefix + "_health", name: fmt.Sprintf("ZFS: %s Health", displayName),
		icon: "mdi:database", template: "{{ value_json.health }}",
	})
	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: prefix + "_capacity", name: fmt.Sprintf("ZFS: %s Usage", displayName), unit: "%",
		icon: "mdi:database", template: "{{ value_json.capacity_percent | round(1) }}",
		stateClass: "measurement",
	})
	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: prefix + "_free", name: fmt.Sprintf("ZFS: %s Free", displayName), unit: "B",
		icon: "mdi:database", template: "{{ value_json.free_bytes }}",
		deviceClass: "data_size", stateClass: "measurement",
	})
	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: prefix + "_fragmentation", name: fmt.Sprintf("ZFS: %s Fragmentation", displayName), unit: "%",
		icon: "mdi:chart-scatter-plot", template: "{{ value_json.fragmentation_percent | round(1) }}",
		stateClass: "measurement",
	})
	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: prefix + "_errors", name: fmt.Sprintf("ZFS: %s Errors", displayName),
		icon:       "mdi:alert-circle",
		template:   "{{ (value_json.read_errors | default(0)) + (value_json.write_errors | default(0)) + (value_json.checksum_errors | default(0)) }}",
		stateClass: "total_increasing",
	})
	c.publishHAEntity(haEntityOpts{
		entityType: "binary_sensor", stateTopic: topic,
		id: prefix + "_healthy", name: fmt.Sprintf("ZFS: %s Healthy", displayName),
		icon: "mdi:check-circle", template: "{{ 'ON' if value_json.health == 'ONLINE' else 'OFF' }}",
		deviceClass: "safety",
	})

	return ids
}

// ──────────────────────────────────────────────────────────────────────────────
// Helpers
// ──────────────────────────────────────────────────────────────────────────────

// sanitizeID converts a string into a safe MQTT/HA entity ID.
func sanitizeID(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, " ", "_")
	s = strings.ReplaceAll(s, "/", "_")
	s = strings.ReplaceAll(s, ".", "_")
	s = strings.ReplaceAll(s, "-", "_")
	return s
}
