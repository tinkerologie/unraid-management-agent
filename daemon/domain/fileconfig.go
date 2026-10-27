package domain

import (
	"fmt"
	"os"

	"go.yaml.in/yaml/v3"
)

// DefaultConfigPath is the standard location for the config file on Unraid.
const DefaultConfigPath = "/boot/config/plugins/unraid-management-agent/config.yml"

// FileConfig represents the YAML configuration file structure.
// Values set in the config file serve as defaults that can be overridden
// by CLI flags and environment variables.
type FileConfig struct {
	// Server settings
	Port        *int    `yaml:"port,omitempty"`
	BindAddress *string `yaml:"bind_address,omitempty"`
	LogLevel    *string `yaml:"log_level,omitempty"`
	LogsDir     *string `yaml:"logs_dir,omitempty"`
	Debug       *bool   `yaml:"debug,omitempty"`

	// ReadOnly blocks all state-changing MCP tools (AI agents can only read).
	ReadOnly *bool `yaml:"read_only,omitempty"`

	// ToolPolicy configures per-tool access policies (hidden, read_only, allow, ask, default).
	ToolPolicy map[string]string `yaml:"tool_policy,omitempty"`

	// Power mode
	LowPowerMode      *bool   `yaml:"low_power_mode,omitempty"`
	DisableCollectors *string `yaml:"disable_collectors,omitempty"`
	// AutoCollectors lists hardware-gated collectors in automatic mode (#261).
	AutoCollectors *string `yaml:"auto_collectors,omitempty"`
	// CollectionMode is "dynamic" or "manual" (#280).
	CollectionMode *string `yaml:"collection_mode,omitempty"`

	// CORS
	CORSOrigin *string `yaml:"cors_origin,omitempty"`

	// APIToken enables bearer-token authentication when non-empty.
	APIToken *string `yaml:"api_token,omitempty"`

	// AllowUnauthenticatedWrites restores network write access without an API
	// token (not recommended).
	AllowUnauthenticatedWrites *bool `yaml:"allow_unauthenticated_writes,omitempty"`

	// WANIPLookup enables the external WAN IP lookup (default true).
	WANIPLookup *bool `yaml:"wan_ip_lookup,omitempty"`

	// MCPConnectSecret enables the secret-bearing MCP connect URL (/mcp/<secret>) when non-empty.
	MCPConnectSecret *string `yaml:"mcp_connect_secret,omitempty"`

	// TLS: serve HTTPS when both a certificate and key file are provided.
	TLSCertFile *string `yaml:"tls_cert_file,omitempty"`
	TLSKeyFile  *string `yaml:"tls_key_file,omitempty"`

	// VMGuestAgent reads data from inside the listed VMs through the QEMU
	// guest agent.
	VMGuestAgent *FileConfigVMGuest `yaml:"vm_guest_agent,omitempty"`

	// MQTT configuration
	MQTT *FileConfigMQTT `yaml:"mqtt,omitempty"`

	// Discovery (zeroconf/mDNS) configuration
	Discovery *FileConfigDiscovery `yaml:"discovery,omitempty"`

	// Collection intervals (seconds, 0 = disabled)
	Intervals *FileConfigIntervals `yaml:"intervals,omitempty"`
}

// FileConfigDiscovery holds zeroconf (mDNS) discovery settings from the config file.
type FileConfigDiscovery struct {
	Enabled     *bool   `yaml:"enabled,omitempty"`
	ServiceName *string `yaml:"service_name,omitempty"`
}

// FileConfigVMGuest holds the vm_guest_agent section of config.yml.
type FileConfigVMGuest struct {
	VMs      []string                  `yaml:"vms,omitempty"`
	Interval *int                      `yaml:"interval,omitempty"`
	Sensors  []FileConfigVMGuestSensor `yaml:"sensors,omitempty"`
}

// FileConfigVMGuestSensor is a custom command run inside a VM. Command is the
// program followed by its arguments; it runs without a shell.
type FileConfigVMGuestSensor struct {
	VM       string   `yaml:"vm"`
	Name     string   `yaml:"name"`
	Command  []string `yaml:"command"`
	Interval int      `yaml:"interval,omitempty"`
	// JSON expands a flat JSON object on standard output into one sensor per key.
	JSON bool `yaml:"json,omitempty"`
	// Attributes maps a JSON key to the key whose sensor gets it as an
	// attribute instead of a sensor of its own. Only valid with json: true.
	Attributes map[string]string `yaml:"attributes,omitempty"`
}

// FileConfigMQTT holds MQTT-specific settings from the config file.
type FileConfigMQTT struct {
	Enabled            *bool   `yaml:"enabled,omitempty"`
	Broker             *string `yaml:"broker,omitempty"`
	Port               *int    `yaml:"port,omitempty"`
	Username           *string `yaml:"username,omitempty"`
	Password           *string `yaml:"password,omitempty"`
	ClientID           *string `yaml:"client_id,omitempty"`
	TopicPrefix        *string `yaml:"topic_prefix,omitempty"`
	UseTLS             *bool   `yaml:"use_tls,omitempty"`
	InsecureSkipVerify *bool   `yaml:"insecure_skip_verify,omitempty"`
	QoS                *int    `yaml:"qos,omitempty"`
	Retain             *bool   `yaml:"retain,omitempty"`
	HomeAssistant      *bool   `yaml:"home_assistant,omitempty"`
	HAPrefix           *string `yaml:"ha_prefix,omitempty"`
}

// FileConfigIntervals holds collection interval overrides from the config file.
type FileConfigIntervals struct {
	System         *int `yaml:"system,omitempty"`
	Array          *int `yaml:"array,omitempty"`
	Disk           *int `yaml:"disk,omitempty"`
	Docker         *int `yaml:"docker,omitempty"`
	VM             *int `yaml:"vm,omitempty"`
	UPS            *int `yaml:"ups,omitempty"`
	NUT            *int `yaml:"nut,omitempty"`
	GPU            *int `yaml:"gpu,omitempty"`
	Shares         *int `yaml:"shares,omitempty"`
	Network        *int `yaml:"network,omitempty"`
	Hardware       *int `yaml:"hardware,omitempty"`
	ZFS            *int `yaml:"zfs,omitempty"`
	Notification   *int `yaml:"notification,omitempty"`
	Registration   *int `yaml:"registration,omitempty"`
	Unassigned     *int `yaml:"unassigned,omitempty"`
	FanControl     *int `yaml:"fancontrol,omitempty"`
	Tuning         *int `yaml:"tuning,omitempty"`
	DockerUpdate   *int `yaml:"docker_update,omitempty"`
	DockerNetworks *int `yaml:"docker_networks,omitempty"`
	PluginUpdate   *int `yaml:"plugin_update,omitempty"`
	OSUpdate       *int `yaml:"os_update,omitempty"`
	Mover          *int `yaml:"mover,omitempty"`

	// StorageTopology is the storage_topology collector interval in seconds.
	StorageTopology *int `yaml:"storage_topology,omitempty"`
	// NIC is the nic collector interval in seconds.
	NIC *int `yaml:"nic,omitempty"`
	PSU *int `yaml:"psu,omitempty"`
}

// LoadConfigFile reads and parses a YAML config file.
// Returns nil without error if the file does not exist.
func LoadConfigFile(path string) (*FileConfig, error) {
	// #nosec G304 -- path is an internal config file location, not direct user input.
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	var cfg FileConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}
	return &cfg, nil
}
