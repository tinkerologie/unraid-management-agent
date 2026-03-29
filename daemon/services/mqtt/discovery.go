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
	c.publishNUTDiscovery()
	c.publishHardwareDiscovery()
	c.publishRegistrationDiscovery()
	c.publishZFSSnapshotDiscovery()
	c.publishZFSARCDiscovery()

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

// publishFanDiscovery publishes per-fan HA discovery entities.
// Fans are embedded in the system JSON payload; each entity uses a Jinja2 selectattr
// template to extract the RPM for its specific fan by name.
func (c *Client) publishFanDiscovery(fans []dto.FanInfo) {
	if !c.config.HomeAssistantMode {
		return
	}

	topic := c.buildTopic("system")
	var currentIDs []string

	for _, fan := range fans {
		fanID := "fan_" + sanitizeID(fan.Name)
		c.publishHAEntity(haEntityOpts{
			entityType: "sensor", stateTopic: topic,
			id: fanID, name: fmt.Sprintf("System: %s", fan.Name), unit: "RPM",
			icon:       "mdi:fan",
			template:   fmt.Sprintf(`{{ (value_json.fans | selectattr('name', 'eq', '%s') | map(attribute='rpm') | first | default(0)) }}`, fan.Name),
			stateClass: "measurement",
		})
		currentIDs = append(currentIDs, fanID)
	}

	removed := c.tracker.update("fans", currentIDs)
	for _, id := range removed {
		c.removeHAEntities(id)
	}
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
		icon: "mdi:lightning-bolt", template: "{{ value_json.power_watts | default(0) | round(0) }}",
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
		id: prefix + "_rx", name: fmt.Sprintf("Network: %s Throughput In", displayName), unit: "B/s",
		icon: "mdi:download", template: "{{ value_json.rx_bytes_per_sec | round(1) }}",
		deviceClass: "data_rate", stateClass: "measurement",
	})
	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: prefix + "_tx", name: fmt.Sprintf("Network: %s Throughput Out", displayName), unit: "B/s",
		icon: "mdi:upload", template: "{{ value_json.tx_bytes_per_sec | round(1) }}",
		deviceClass: "data_rate", stateClass: "measurement",
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
// NUT UPS
// ──────────────────────────────────────────────────────────────────────────────

// publishNUTDiscovery publishes HA discovery for NUT UPS metrics.
// NUTResponse.Status is a pointer — templates use | default() guards for nil safety.
func (c *Client) publishNUTDiscovery() {
	topic := c.buildTopic("nut/status")
	c.publishHAEntity(haEntityOpts{
		entityType: "binary_sensor", stateTopic: topic,
		id: "nut_connected", name: "NUT: UPS Connected",
		icon:        "mdi:battery-charging",
		template:    "{{ 'ON' if value_json.status.connected | default(false) else 'OFF' }}",
		deviceClass: "connectivity",
	})
	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: "nut_status", name: "NUT: UPS Status",
		icon:     "mdi:battery-charging",
		template: "{{ value_json.status.status | default('unknown') }}",
	})
	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: "nut_battery_charge", name: "NUT: Battery Charge", unit: "%",
		icon:        "mdi:battery",
		template:    "{{ value_json.status.battery_charge_percent | default(0) | round(0) }}",
		deviceClass: "battery", stateClass: "measurement",
	})
	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: "nut_battery_runtime", name: "NUT: Battery Runtime", unit: "s",
		icon:        "mdi:clock-outline",
		template:    "{{ value_json.status.battery_runtime_seconds | default(0) }}",
		deviceClass: "duration", stateClass: "measurement",
	})
	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: "nut_load", name: "NUT: Load", unit: "%",
		icon:       "mdi:gauge",
		template:   "{{ value_json.status.load_percent | default(0) | round(1) }}",
		stateClass: "measurement",
	})
	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: "nut_realpower", name: "NUT: Real Power", unit: "W",
		icon:        "mdi:lightning-bolt",
		template:    "{{ value_json.status.realpower_watts | default(0) | round(0) }}",
		deviceClass: "power", stateClass: "measurement",
	})
	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: "nut_input_voltage", name: "NUT: Input Voltage", unit: "V",
		icon:        "mdi:sine-wave",
		template:    "{{ value_json.status.input_voltage | default(0) | round(1) }}",
		deviceClass: "voltage", stateClass: "measurement",
	})
	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: "nut_output_voltage", name: "NUT: Output Voltage", unit: "V",
		icon:        "mdi:sine-wave",
		template:    "{{ value_json.status.output_voltage | default(0) | round(1) }}",
		deviceClass: "voltage", stateClass: "measurement",
	})
	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: "nut_model", name: "NUT: UPS Model",
		icon:           "mdi:battery-charging",
		template:       "{{ value_json.status.model | default('') }}",
		entityCategory: "diagnostic",
	})
}

// ──────────────────────────────────────────────────────────────────────────────
// Hardware Info
// ──────────────────────────────────────────────────────────────────────────────

// publishHardwareDiscovery publishes HA discovery for hardware information.
func (c *Client) publishHardwareDiscovery() {
	topic := c.buildTopic("hardware")
	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: "bios_version", name: "Hardware: BIOS Version",
		icon:           "mdi:chip",
		template:       "{{ value_json.bios.version | default('') }}",
		entityCategory: "diagnostic",
	})
	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: "bios_date", name: "Hardware: BIOS Release Date",
		icon:           "mdi:calendar",
		template:       "{{ value_json.bios.release_date | default('') }}",
		entityCategory: "diagnostic",
	})
	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: "board_manufacturer", name: "Hardware: Board Manufacturer",
		icon:           "mdi:factory",
		template:       "{{ value_json.baseboard.manufacturer | default('') }}",
		entityCategory: "diagnostic",
	})
	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: "board_model", name: "Hardware: Board Model",
		icon:           "mdi:circuit-board",
		template:       "{{ value_json.baseboard.product_name | default('') }}",
		entityCategory: "diagnostic",
	})
	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: "cpu_max_speed", name: "Hardware: CPU Max Speed", unit: "MHz",
		icon:           "mdi:cpu-64-bit",
		template:       "{{ value_json.cpu.max_speed_mhz | default(0) }}",
		entityCategory: "diagnostic",
	})
	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: "memory_slots_total", name: "Hardware: Memory Slots",
		icon:           "mdi:memory",
		template:       "{{ value_json.memory_array.number_of_devices | default(0) }}",
		entityCategory: "diagnostic",
	})
}

// ──────────────────────────────────────────────────────────────────────────────
// Registration / License
// ──────────────────────────────────────────────────────────────────────────────

// publishRegistrationDiscovery publishes HA discovery for Unraid license/registration info.
func (c *Client) publishRegistrationDiscovery() {
	topic := c.buildTopic("registration")
	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: "registration_state", name: "Registration: State",
		icon:     "mdi:license",
		template: "{{ value_json.state }}",
	})
	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: "registration_type", name: "Registration: Type",
		icon:           "mdi:tag",
		template:       "{{ value_json.type }}",
		entityCategory: "diagnostic",
	})
	c.publishHAEntity(haEntityOpts{
		entityType: "binary_sensor", stateTopic: topic,
		id: "registration_valid", name: "Registration: Valid",
		icon:        "mdi:check-decagram",
		template:    "{{ 'ON' if value_json.state == 'valid' else 'OFF' }}",
		deviceClass: "safety",
	})
	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: "registration_expiry", name: "Registration: Expiry",
		icon:        "mdi:calendar-clock",
		template:    "{{ value_json.expiration }}",
		deviceClass: "timestamp",
	})
}

// ──────────────────────────────────────────────────────────────────────────────
// Unassigned Devices
// ──────────────────────────────────────────────────────────────────────────────

// publishUnassignedDiscovery publishes per-device HA discovery for unassigned devices.
func (c *Client) publishUnassignedDiscovery(list *dto.UnassignedDeviceList) {
	if !c.config.HomeAssistantMode {
		return
	}
	if list == nil {
		return
	}
	var currentIDs []string
	for _, dev := range list.Devices {
		if dev.Device == "" {
			continue
		}
		devID := sanitizeID(dev.Device)
		devTopic := c.buildTopic(fmt.Sprintf("unassigned/%s", devID))
		if err := c.publishJSON(devTopic, dev); err != nil {
			logger.Debug("MQTT: Failed to publish unassigned device %s: %v", devID, err)
			continue
		}
		displayName := dev.Model
		if displayName == "" {
			displayName = dev.Device
		}
		ids := c.publishUnassignedEntities(devTopic, fmt.Sprintf("unassigned_%s", devID), displayName, dev)
		currentIDs = append(currentIDs, ids...)
	}
	removed := c.tracker.update("unassigned", currentIDs)
	for _, id := range removed {
		c.removeHAEntities(id)
	}
}

// publishUnassignedEntities publishes HA entity discovery for a single unassigned device.
func (c *Client) publishUnassignedEntities(topic, prefix, displayName string, dev dto.UnassignedDevice) []string {
	ids := []string{prefix + "_connected", prefix + "_temp", prefix + "_spin_state"}
	c.publishHAEntity(haEntityOpts{
		entityType: "binary_sensor", stateTopic: topic,
		id: prefix + "_connected", name: fmt.Sprintf("Unassigned: %s Connected", displayName),
		icon:        "mdi:harddisk",
		template:    "{{ 'ON' if value_json.status != 'error' else 'OFF' }}",
		deviceClass: "connectivity",
	})
	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: prefix + "_temp", name: fmt.Sprintf("Unassigned: %s Temperature", displayName), unit: "°C",
		icon:        "mdi:thermometer",
		template:    "{{ value_json.temperature_celsius | default(0) }}",
		deviceClass: "temperature", stateClass: "measurement",
	})
	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: prefix + "_spin_state", name: fmt.Sprintf("Unassigned: %s Spin State", displayName),
		icon:     "mdi:rotate-3d-variant",
		template: "{{ value_json.spin_state | default('unknown') }}",
	})
	for i, part := range dev.Partitions {
		partPrefix := fmt.Sprintf("%s_part%d", prefix, i+1)
		partLabel := part.Label
		if partLabel == "" {
			partLabel = fmt.Sprintf("Part %d", part.PartitionNumber)
		}
		ids = append(ids, partPrefix+"_usage", partPrefix+"_used", partPrefix+"_free")
		c.publishHAEntity(haEntityOpts{
			entityType: "sensor", stateTopic: topic,
			id:   partPrefix + "_usage",
			name: fmt.Sprintf("Unassigned: %s %s Usage", displayName, partLabel), unit: "%",
			icon:       "mdi:harddisk",
			template:   fmt.Sprintf("{{ value_json.partitions[%d].usage_percent | default(0) | round(1) }}", i),
			stateClass: "measurement",
		})
		c.publishHAEntity(haEntityOpts{
			entityType: "sensor", stateTopic: topic,
			id:   partPrefix + "_used",
			name: fmt.Sprintf("Unassigned: %s %s Used", displayName, partLabel), unit: "B",
			icon:        "mdi:harddisk",
			template:    fmt.Sprintf("{{ value_json.partitions[%d].used_bytes | default(0) }}", i),
			deviceClass: "data_size", stateClass: "measurement",
		})
		c.publishHAEntity(haEntityOpts{
			entityType: "sensor", stateTopic: topic,
			id:   partPrefix + "_free",
			name: fmt.Sprintf("Unassigned: %s %s Free", displayName, partLabel), unit: "B",
			icon:        "mdi:harddisk",
			template:    fmt.Sprintf("{{ value_json.partitions[%d].free_bytes | default(0) }}", i),
			deviceClass: "data_size", stateClass: "measurement",
		})
	}
	return ids
}

// ──────────────────────────────────────────────────────────────────────────────
// ZFS Datasets
// ──────────────────────────────────────────────────────────────────────────────

// publishZFSDatasetDiscovery publishes per-dataset HA discovery for ZFS datasets.
func (c *Client) publishZFSDatasetDiscovery(datasets []dto.ZFSDataset) {
	if !c.config.HomeAssistantMode {
		return
	}
	var currentIDs []string
	for _, ds := range datasets {
		if ds.Name == "" {
			continue
		}
		dsID := sanitizeID(ds.Name)
		dsTopic := c.buildTopic(fmt.Sprintf("zfs/datasets/%s", dsID))
		if err := c.publishJSON(dsTopic, ds); err != nil {
			logger.Debug("MQTT: Failed to publish ZFS dataset %s: %v", dsID, err)
			continue
		}
		ids := c.publishZFSDatasetEntities(dsTopic, fmt.Sprintf("zfs_ds_%s", dsID), ds.Name)
		currentIDs = append(currentIDs, ids...)
	}
	removed := c.tracker.update("zfs_datasets", currentIDs)
	for _, id := range removed {
		c.removeHAEntities(id)
	}
}

// publishZFSDatasetEntities publishes HA entity discovery for a single ZFS dataset.
func (c *Client) publishZFSDatasetEntities(topic, prefix, displayName string) []string {
	ids := []string{prefix + "_used", prefix + "_available", prefix + "_compress_ratio", prefix + "_readonly"}
	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: prefix + "_used", name: fmt.Sprintf("ZFS Dataset: %s Used", displayName), unit: "B",
		icon:        "mdi:database",
		template:    "{{ value_json.used_bytes }}",
		deviceClass: "data_size", stateClass: "measurement",
	})
	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: prefix + "_available", name: fmt.Sprintf("ZFS Dataset: %s Available", displayName), unit: "B",
		icon:        "mdi:database",
		template:    "{{ value_json.available_bytes }}",
		deviceClass: "data_size", stateClass: "measurement",
	})
	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: prefix + "_compress_ratio", name: fmt.Sprintf("ZFS Dataset: %s Compression Ratio", displayName),
		icon:       "mdi:zip-box",
		template:   "{{ value_json.compress_ratio | round(2) }}",
		stateClass: "measurement",
	})
	c.publishHAEntity(haEntityOpts{
		entityType: "binary_sensor", stateTopic: topic,
		id: prefix + "_readonly", name: fmt.Sprintf("ZFS Dataset: %s Read-Only", displayName),
		icon:     "mdi:lock",
		template: "{{ 'ON' if value_json.readonly else 'OFF' }}",
	})
	return ids
}

// ──────────────────────────────────────────────────────────────────────────────
// ZFS Snapshots
// ──────────────────────────────────────────────────────────────────────────────

// publishZFSSnapshotDiscovery publishes aggregate HA discovery for ZFS snapshots.
func (c *Client) publishZFSSnapshotDiscovery() {
	topic := c.buildTopic("zfs/snapshots")
	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: "zfs_snapshot_count", name: "ZFS: Snapshot Count",
		icon:       "mdi:camera",
		template:   "{{ value_json | length }}",
		stateClass: "measurement",
	})
	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: "zfs_snapshot_total_size", name: "ZFS: Snapshot Total Size", unit: "B",
		icon:        "mdi:camera",
		template:    "{{ value_json | sum(attribute='used_bytes') | default(0) }}",
		deviceClass: "data_size", stateClass: "measurement",
	})
}

// ──────────────────────────────────────────────────────────────────────────────
// ZFS ARC Stats
// ──────────────────────────────────────────────────────────────────────────────

// publishZFSARCDiscovery publishes HA discovery for ZFS ARC cache statistics.
func (c *Client) publishZFSARCDiscovery() {
	topic := c.buildTopic("zfs/arc")
	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: "arc_size", name: "ZFS ARC: Size", unit: "B",
		icon:        "mdi:memory",
		template:    "{{ value_json.size_bytes }}",
		deviceClass: "data_size", stateClass: "measurement",
	})
	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: "arc_target_size", name: "ZFS ARC: Target Size", unit: "B",
		icon:        "mdi:memory",
		template:    "{{ value_json.target_size_bytes }}",
		deviceClass: "data_size", stateClass: "measurement",
		entityCategory: "diagnostic",
	})
	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: "arc_hit_ratio", name: "ZFS ARC: Hit Ratio", unit: "%",
		icon:       "mdi:chart-line",
		template:   "{{ value_json.hit_ratio_percent | round(1) }}",
		stateClass: "measurement",
	})
	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: "arc_l2_size", name: "ZFS L2ARC: Size", unit: "B",
		icon:        "mdi:memory",
		template:    "{{ value_json.l2_size_bytes | default(0) }}",
		deviceClass: "data_size", stateClass: "measurement",
		entityCategory: "diagnostic",
	})
	c.publishHAEntity(haEntityOpts{
		entityType: "sensor", stateTopic: topic,
		id: "arc_l2_hit_ratio", name: "ZFS L2ARC: Hit Ratio", unit: "%",
		icon:           "mdi:chart-line",
		template:       "{{ ((value_json.l2_hits | default(0)) / ((value_json.l2_hits | default(0)) + (value_json.l2_misses | default(0))) * 100) | round(1) if ((value_json.l2_hits | default(0)) + (value_json.l2_misses | default(0))) > 0 else 0 }}",
		stateClass:     "measurement",
		entityCategory: "diagnostic",
	})
}

// ──────────────────────────────────────────────────────────────────────────────
// Fan Control Discovery
// ──────────────────────────────────────────────────────────────────────────────

// publishFanControlDiscovery publishes per-fan HA discovery entities from fan control data.
func (c *Client) publishFanControlDiscovery(status *dto.FanControlStatus) {
	if !c.config.HomeAssistantMode {
		return
	}

	topic := c.buildTopic("fancontrol")
	var currentIDs []string

	for _, fan := range status.Fans {
		fanID := "fanctrl_" + sanitizeID(fan.ID)

		// Fan RPM sensor
		rpmID := fanID + "_rpm"
		c.publishHAEntity(haEntityOpts{
			entityType: "sensor", stateTopic: topic,
			id: rpmID, name: fmt.Sprintf("Fan Control: %s RPM", fan.Name), unit: "RPM",
			icon:       "mdi:fan",
			template:   fmt.Sprintf(`{{ (value_json.fans | selectattr('id', 'eq', '%s') | map(attribute='rpm') | first | default(0)) }}`, fan.ID),
			stateClass: "measurement",
		})
		currentIDs = append(currentIDs, rpmID)

		// Fan PWM percent sensor
		pwmID := fanID + "_pwm"
		c.publishHAEntity(haEntityOpts{
			entityType: "sensor", stateTopic: topic,
			id: pwmID, name: fmt.Sprintf("Fan Control: %s PWM", fan.Name), unit: "%",
			icon:       "mdi:fan",
			template:   fmt.Sprintf(`{{ (value_json.fans | selectattr('id', 'eq', '%s') | map(attribute='pwm_percent') | first | default(0)) }}`, fan.ID),
			stateClass: "measurement",
		})
		currentIDs = append(currentIDs, pwmID)

		// Fan mode sensor
		modeID := fanID + "_mode"
		c.publishHAEntity(haEntityOpts{
			entityType: "sensor", stateTopic: topic,
			id: modeID, name: fmt.Sprintf("Fan Control: %s Mode", fan.Name),
			icon:     "mdi:fan-clock",
			template: fmt.Sprintf(`{{ (value_json.fans | selectattr('id', 'eq', '%s') | map(attribute='mode') | first | default('unknown')) }}`, fan.ID),
		})
		currentIDs = append(currentIDs, modeID)
	}

	// Fan control enabled binary sensor
	enabledID := "fanctrl_enabled"
	c.publishHAEntity(haEntityOpts{
		entityType: "binary_sensor", stateTopic: topic,
		id: enabledID, name: "Fan Control: Enabled",
		icon:       "mdi:fan-alert",
		template:   `{{ value_json.config.control_enabled }}`,
		payloadOn:  "true",
		payloadOff: "false",
	})
	currentIDs = append(currentIDs, enabledID)

	removed := c.tracker.update("fancontrol", currentIDs)
	for _, id := range removed {
		c.removeHAEntities(id)
	}
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
