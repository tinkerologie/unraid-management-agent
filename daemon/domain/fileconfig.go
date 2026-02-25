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
	Port     *int    `yaml:"port,omitempty"`
	LogLevel *string `yaml:"log_level,omitempty"`
	LogsDir  *string `yaml:"logs_dir,omitempty"`
	Debug    *bool   `yaml:"debug,omitempty"`

	// Power mode
	LowPowerMode      *bool   `yaml:"low_power_mode,omitempty"`
	DisableCollectors *string `yaml:"disable_collectors,omitempty"`

	// CORS
	CORSOrigin *string `yaml:"cors_origin,omitempty"`

	// MQTT configuration
	MQTT *FileConfigMQTT `yaml:"mqtt,omitempty"`

	// Collection intervals (seconds, 0 = disabled)
	Intervals *FileConfigIntervals `yaml:"intervals,omitempty"`
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
	System       *int `yaml:"system,omitempty"`
	Array        *int `yaml:"array,omitempty"`
	Disk         *int `yaml:"disk,omitempty"`
	Docker       *int `yaml:"docker,omitempty"`
	VM           *int `yaml:"vm,omitempty"`
	UPS          *int `yaml:"ups,omitempty"`
	NUT          *int `yaml:"nut,omitempty"`
	GPU          *int `yaml:"gpu,omitempty"`
	Shares       *int `yaml:"shares,omitempty"`
	Network      *int `yaml:"network,omitempty"`
	Hardware     *int `yaml:"hardware,omitempty"`
	ZFS          *int `yaml:"zfs,omitempty"`
	Notification *int `yaml:"notification,omitempty"`
	Registration *int `yaml:"registration,omitempty"`
	Unassigned   *int `yaml:"unassigned,omitempty"`
}

// LoadConfigFile reads and parses a YAML config file.
// Returns nil without error if the file does not exist.
func LoadConfigFile(path string) (*FileConfig, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path is a trusted config file path, not user input
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
