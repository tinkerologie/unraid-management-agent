// Package domain provides core domain models and configuration structures for the Unraid Management Agent.
package domain

import (
	"fmt"
	"strings"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

// MQTTConfig holds MQTT broker connection and publishing settings.
type MQTTConfig struct {
	// Connection settings
	Enabled  bool   `json:"enabled"`
	Broker   string `json:"broker"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"-"` // Never serialize password
	ClientID string `json:"client_id"`

	// TLS settings
	UseTLS             bool `json:"use_tls"`
	InsecureSkipVerify bool `json:"insecure_skip_verify"`

	// Publishing settings
	TopicPrefix    string `json:"topic_prefix"`
	QoS            int    `json:"qos"`
	RetainMessages bool   `json:"retain_messages"`

	// Home Assistant integration
	HomeAssistantMode   bool   `json:"home_assistant_mode"`
	HomeAssistantPrefix string `json:"home_assistant_prefix"`
	DiscoveryEnabled    bool   `json:"discovery_enabled"`
}

// Config holds the application configuration settings.
type Config struct {
	Version string `json:"version"`
	Port    int    `json:"port"`
	// BindAddress is the IP address the HTTP server binds to.
	// Empty means all interfaces (the default).
	BindAddress string `json:"bind_address,omitempty"`
	CORSOrigin  string `json:"cors_origin,omitempty"`
	// ReadOnly blocks all state-changing MCP tools so AI agents can only
	// consume data. The REST API is unaffected.
	ReadOnly bool `json:"read_only,omitempty"`
	// TLSCertFile and TLSKeyFile point at a PEM certificate/key pair. When both
	// are set the HTTP server (including the /mcp endpoint) is served over HTTPS;
	// when either is empty the server stays on plain HTTP.
	TLSCertFile string `json:"tls_cert_file,omitempty"`
	TLSKeyFile  string `json:"tls_key_file,omitempty"`
}

// TLSEnabled reports whether HTTPS should be served. TLS is considered enabled
// only when both a certificate and key file are configured. Whitespace-only
// values are treated as unset so they don't mislead the HTTPS branch before
// validation runs.
func (c Config) TLSEnabled() bool {
	return strings.TrimSpace(c.TLSCertFile) != "" && strings.TrimSpace(c.TLSKeyFile) != ""
}

// DiscoveryConfig holds zeroconf (mDNS/DNS-SD) auto-discovery settings.
// When enabled, the agent advertises itself on the local network so that
// integrations (e.g. the Home Assistant integration) can auto-discover it.
type DiscoveryConfig struct {
	// Enabled controls whether the agent advertises itself via mDNS.
	Enabled bool `json:"enabled"`
	// ServiceName optionally overrides the advertised instance name.
	// When empty, the system hostname is used.
	ServiceName string `json:"service_name,omitempty"`
}

// DefaultDiscoveryConfig returns the default zeroconf discovery configuration.
func DefaultDiscoveryConfig() DiscoveryConfig {
	return DiscoveryConfig{
		Enabled:     true,
		ServiceName: "",
	}
}

// DefaultMQTTConfig returns the default MQTT configuration.
func DefaultMQTTConfig() MQTTConfig {
	return MQTTConfig{
		Enabled:             false,
		Broker:              "",
		Port:                1883,
		Username:            "",
		Password:            "",
		ClientID:            "unraid-management-agent",
		UseTLS:              false,
		InsecureSkipVerify:  false,
		TopicPrefix:         "unraid",
		QoS:                 0,
		RetainMessages:      true,
		HomeAssistantMode:   false,
		HomeAssistantPrefix: "homeassistant",
		DiscoveryEnabled:    true,
	}
}

// ToDTOConfig converts domain.MQTTConfig to dto.MQTTConfig for use with the MQTT client.
func (c *MQTTConfig) ToDTOConfig() *dto.MQTTConfig {
	// Build broker URL with protocol and port
	broker := c.Broker
	if broker != "" && c.Port > 0 {
		protocol := "tcp"
		if c.UseTLS {
			protocol = "ssl"
		}
		broker = fmt.Sprintf("%s://%s:%d", protocol, c.Broker, c.Port)
	}

	return &dto.MQTTConfig{
		Enabled:           c.Enabled,
		Broker:            broker,
		ClientID:          c.ClientID,
		Username:          c.Username,
		Password:          c.Password,
		TopicPrefix:       c.TopicPrefix,
		QoS:               c.QoS,
		RetainMessages:    c.RetainMessages,
		ConnectTimeout:    30,
		KeepAlive:         60,
		CleanSession:      true,
		AutoReconnect:     true,
		HomeAssistantMode: c.HomeAssistantMode,
		HADiscoveryPrefix: c.HomeAssistantPrefix,
	}
}
