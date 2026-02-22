// Package dto provides data transfer objects for the Unraid Management Agent.
package dto

import "time"

// MQTTConfig represents the MQTT client configuration.
type MQTTConfig struct {
	Enabled           bool   `json:"enabled" example:"true"`
	Broker            string `json:"broker" example:"tcp://localhost:1883"`
	ClientID          string `json:"client_id" example:"unraid-agent"`
	Username          string `json:"username,omitempty" example:""`
	Password          string `json:"-"` // Never expose password in JSON
	TopicPrefix       string `json:"topic_prefix" example:"unraid"`
	QoS               int    `json:"qos" example:"1"`
	RetainMessages    bool   `json:"retain_messages" example:"true"`
	ConnectTimeout    int    `json:"connect_timeout" example:"30"`
	KeepAlive         int    `json:"keepalive" example:"60"`
	CleanSession      bool   `json:"clean_session" example:"true"`
	AutoReconnect     bool   `json:"auto_reconnect" example:"true"`
	HomeAssistantMode bool   `json:"homeassistant_mode" example:"true"`
	HADiscoveryPrefix string `json:"ha_discovery_prefix" example:"homeassistant"`
}

// MQTTStatus represents the current status of the MQTT client.
type MQTTStatus struct {
	Connected      bool       `json:"connected" example:"true"`
	Enabled        bool       `json:"enabled" example:"true"`
	Broker         string     `json:"broker" example:"tcp://localhost:1883"`
	ClientID       string     `json:"client_id" example:"unraid-agent"`
	TopicPrefix    string     `json:"topic_prefix" example:"unraid"`
	LastConnected  *time.Time `json:"last_connected,omitempty"`
	LastDisconnect *time.Time `json:"last_disconnect,omitempty"`
	LastError      string     `json:"last_error,omitempty" example:""`
	MessagesSent   int64      `json:"messages_sent" example:"1234"`
	MessagesErrors int64      `json:"messages_errors" example:"0"`
	Uptime         int64      `json:"uptime_seconds" example:"3600"`
	Timestamp      time.Time  `json:"timestamp"`
}

// MQTTMessage represents a message to be published via MQTT.
type MQTTMessage struct {
	Topic    string `json:"topic" example:"unraid/system/status"`
	Payload  any    `json:"payload"`
	QoS      int    `json:"qos" example:"1"`
	Retained bool   `json:"retained" example:"false"`
}

// MQTTPublishRequest represents a request to publish a message via MQTT.
type MQTTPublishRequest struct {
	Topic    string `json:"topic" example:"custom/topic"`
	Payload  any    `json:"payload"`
	QoS      int    `json:"qos" example:"1"`
	Retained bool   `json:"retained" example:"false"`
}

// MQTTPublishResponse represents the response after publishing a message.
type MQTTPublishResponse struct {
	Success   bool      `json:"success" example:"true"`
	Message   string    `json:"message" example:"Message published successfully"`
	Topic     string    `json:"topic" example:"unraid/custom/topic"`
	Timestamp time.Time `json:"timestamp"`
}

// MQTTTestRequest represents a request to test MQTT connectivity.
type MQTTTestRequest struct {
	Broker   string `json:"broker" example:"tcp://localhost:1883"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	ClientID string `json:"client_id,omitempty" example:"unraid-test"`
}

// MQTTTestResponse represents the response from an MQTT connectivity test.
type MQTTTestResponse struct {
	Success     bool      `json:"success" example:"true"`
	Message     string    `json:"message" example:"Connection successful"`
	Latency     int64     `json:"latency_ms" example:"15"`
	BrokerInfo  string    `json:"broker_info,omitempty"`
	TLSEnabled  bool      `json:"tls_enabled" example:"false"`
	ProtocolVer string    `json:"protocol_version" example:"3.1.1"`
	Timestamp   time.Time `json:"timestamp"`
}

// HADiscoveryConfig represents Home Assistant MQTT Discovery configuration.
type HADiscoveryConfig struct {
	Name              string            `json:"name"`
	UniqueID          string            `json:"unique_id"`
	StateTopic        string            `json:"state_topic,omitempty"`
	CommandTopic      string            `json:"command_topic,omitempty"`
	AvailabilityTopic string            `json:"availability_topic,omitempty"`
	PayloadAvailable  string            `json:"payload_available,omitempty"`
	PayloadNotAvail   string            `json:"payload_not_available,omitempty"`
	DeviceClass       string            `json:"device_class,omitempty"`
	UnitOfMeasurement string            `json:"unit_of_measurement,omitempty"`
	ValueTemplate     string            `json:"value_template,omitempty"`
	Icon              string            `json:"icon,omitempty"`
	Device            *HADeviceInfo     `json:"device,omitempty"`
	Attributes        map[string]string `json:"json_attributes_topic,omitempty"`
}

// HADeviceInfo represents Home Assistant device information for discovery.
type HADeviceInfo struct {
	Identifiers  []string `json:"identifiers"`
	Name         string   `json:"name"`
	Manufacturer string   `json:"manufacturer"`
	Model        string   `json:"model"`
	SWVersion    string   `json:"sw_version,omitempty"`
	HWVersion    string   `json:"hw_version,omitempty"`
	ConfigURL    string   `json:"configuration_url,omitempty"`
}

// MQTTTopics represents the standard MQTT topics used by the agent.
type MQTTTopics struct {
	Status       string `json:"status" example:"unraid/status"`
	System       string `json:"system" example:"unraid/system"`
	Array        string `json:"array" example:"unraid/array"`
	Disks        string `json:"disks" example:"unraid/disks"`
	Containers   string `json:"containers" example:"unraid/docker/containers"`
	VMs          string `json:"vms" example:"unraid/vm/list"`
	UPS          string `json:"ups" example:"unraid/ups"`
	GPU          string `json:"gpu" example:"unraid/gpu"`
	Network      string `json:"network" example:"unraid/network"`
	Shares       string `json:"shares" example:"unraid/shares"`
	Notification string `json:"notifications" example:"unraid/notifications"`
	Availability string `json:"availability" example:"unraid/availability"`
}

// MQTTEnableRequest represents a request to enable/disable MQTT.
type MQTTEnableRequest struct {
	Enabled bool `json:"enabled"`
}

// MQTTConfigUpdateRequest represents a request to update MQTT configuration.
type MQTTConfigUpdateRequest struct {
	Broker            string `json:"broker,omitempty"`
	Username          string `json:"username,omitempty"`
	Password          string `json:"password,omitempty"`
	TopicPrefix       string `json:"topic_prefix,omitempty"`
	QoS               *int   `json:"qos,omitempty"`
	RetainMessages    *bool  `json:"retain_messages,omitempty"`
	HomeAssistantMode *bool  `json:"homeassistant_mode,omitempty"`
	HADiscoveryPrefix string `json:"ha_discovery_prefix,omitempty"`
}
