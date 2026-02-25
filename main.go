// Package main is the entry point for the Unraid Management Agent.
// It provides a REST API and WebSocket interface for monitoring and controlling Unraid systems.
package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/alecthomas/kong"

	"gopkg.in/natefinch/lumberjack.v2"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/cmd"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
)

// Version is the application version, set at build time via ldflags.
var Version = "dev"

// validCollectorNames contains all valid collector names for validation
var validCollectorNames = map[string]bool{
	"system":       true,
	"array":        true,
	"disk":         true,
	"docker":       true,
	"vm":           true,
	"ups":          true,
	"nut":          true,
	"gpu":          true,
	"shares":       true,
	"network":      true,
	"hardware":     true,
	"zfs":          true,
	"notification": true,
	"registration": true,
	"unassigned":   true,
}

var cli struct {
	LogsDir  string `default:"/var/log" help:"directory to store logs"`
	Port     int    `default:"8043" help:"HTTP server port"`
	Debug    bool   `default:"false" help:"enable debug mode with stdout logging"`
	LogLevel string `default:"info" help:"log level: debug, info, warning, error"`

	// CORS
	CORSOrigin string `default:"*" env:"CORS_ORIGIN" help:"Access-Control-Allow-Origin value (default: *)"`

	// Low power mode - multiplies all intervals for resource-constrained systems
	LowPowerMode bool `default:"false" env:"UNRAID_LOW_POWER" help:"enable low power mode (4x longer intervals for old/slow hardware)"`

	// Collector disable flag (alternative to setting interval=0)
	DisableCollectors string `default:"" env:"UNRAID_DISABLE_COLLECTORS" help:"comma-separated list of collectors to disable (e.g., gpu,ups,zfs)"`

	// MQTT Configuration
	MQTTEnabled            bool   `default:"false" env:"MQTT_ENABLED" help:"enable MQTT publishing"`
	MQTTBroker             string `default:"" env:"MQTT_BROKER" help:"MQTT broker hostname or IP"`
	MQTTPort               int    `default:"1883" env:"MQTT_PORT" help:"MQTT broker port"`
	MQTTUsername           string `default:"" env:"MQTT_USERNAME" help:"MQTT username"`
	MQTTPassword           string `default:"" env:"MQTT_PASSWORD" help:"MQTT password"`
	MQTTClientID           string `default:"unraid-management-agent" env:"MQTT_CLIENT_ID" help:"MQTT client ID"`
	MQTTTopicPrefix        string `default:"unraid" env:"MQTT_TOPIC_PREFIX" help:"MQTT topic prefix"`
	MQTTUseTLS             bool   `default:"false" env:"MQTT_USE_TLS" help:"use TLS for MQTT connection"`
	MQTTInsecureSkipVerify bool   `default:"false" env:"MQTT_INSECURE_SKIP_VERIFY" help:"skip TLS certificate verification"`
	MQTTQoS                int    `default:"0" env:"MQTT_QOS" help:"MQTT QoS level (0, 1, or 2)"`
	MQTTRetain             bool   `default:"true" env:"MQTT_RETAIN" help:"retain MQTT messages"`
	MQTTHomeAssistant      bool   `default:"false" env:"MQTT_HOME_ASSISTANT" help:"enable Home Assistant MQTT discovery"`
	MQTTHAPrefix           string `default:"homeassistant" env:"MQTT_HA_PREFIX" help:"Home Assistant discovery prefix"`

	// Collection intervals (overridable via environment variables)
	// Use 0 to disable a collector completely
	// Maximum interval: 86400 seconds (24 hours)
	IntervalSystem       int `default:"15" env:"INTERVAL_SYSTEM" help:"system metrics interval (seconds, 0=disabled, max 86400)"`
	IntervalArray        int `default:"60" env:"INTERVAL_ARRAY" help:"array metrics interval (seconds, 0=disabled, max 86400)"`
	IntervalDisk         int `default:"300" env:"INTERVAL_DISK" help:"disk metrics interval (seconds, 0=disabled, max 86400)"`
	IntervalDocker       int `default:"30" env:"INTERVAL_DOCKER" help:"docker metrics interval (seconds, 0=disabled, max 86400)"`
	IntervalVM           int `default:"60" env:"INTERVAL_VM" help:"VM metrics interval (seconds, 0=disabled, max 86400)"`
	IntervalUPS          int `default:"60" env:"INTERVAL_UPS" help:"UPS metrics interval (seconds, 0=disabled, max 86400)"`
	IntervalNUT          int `default:"0" env:"INTERVAL_NUT" help:"NUT plugin metrics interval (seconds, 0=disabled, max 86400)"`
	IntervalGPU          int `default:"60" env:"INTERVAL_GPU" help:"GPU metrics interval (seconds, 0=disabled, max 86400)"`
	IntervalShares       int `default:"60" env:"INTERVAL_SHARES" help:"shares metrics interval (seconds, 0=disabled, max 86400)"`
	IntervalNetwork      int `default:"60" env:"INTERVAL_NETWORK" help:"network metrics interval (seconds, 0=disabled, max 86400)"`
	IntervalHardware     int `default:"600" env:"INTERVAL_HARDWARE" help:"hardware metrics interval (seconds, 0=disabled, max 86400)"`
	IntervalZFS          int `default:"300" env:"INTERVAL_ZFS" help:"ZFS metrics interval (seconds, 0=disabled, max 86400)"`
	IntervalNotification int `default:"30" env:"INTERVAL_NOTIFICATION" help:"notification interval (seconds, 0=disabled, max 86400)"`
	IntervalRegistration int `default:"600" env:"INTERVAL_REGISTRATION" help:"registration interval (seconds, 0=disabled, max 86400)"`
	IntervalUnassigned   int `default:"60" env:"INTERVAL_UNASSIGNED" help:"unassigned devices interval (seconds, 0=disabled, max 86400)"`

	Boot     cmd.Boot     `cmd:"" default:"1" help:"start the management agent"`
	MCPStdio cmd.MCPStdio `cmd:"mcp-stdio" help:"run MCP server over stdin/stdout for local AI clients"`
}

// cleanupOldLogs removes old rotated log files from previous versions
// This is needed because lumberjack's MaxBackups only prevents new backups,
// it doesn't clean up existing ones from before the setting was changed
func cleanupOldLogs(logsDir, baseName string) {
	pattern := filepath.Join(logsDir, baseName+"-*.log")
	files, err := filepath.Glob(pattern)
	if err != nil {
		return
	}
	for _, f := range files {
		_ = os.Remove(f)
	}
}

func main() {
	ctx := kong.Parse(&cli)

	// Detect STDIO mode — stdout is reserved for MCP JSON-RPC
	isStdio := ctx.Command() == "mcp-stdio"

	// Load config file (defaults; CLI/env override)
	fileCfg, err := domain.LoadConfigFile(domain.DefaultConfigPath)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "WARNING: Failed to load config file: %v\n", err)
	}
	applyFileConfig(fileCfg)

	// Set log level based on CLI flag
	switch strings.ToLower(cli.LogLevel) {
	case "debug":
		logger.SetLevel(logger.LevelDebug)
	case "info":
		logger.SetLevel(logger.LevelInfo)
	case "warning", "warn":
		logger.SetLevel(logger.LevelWarning)
	case "error":
		logger.SetLevel(logger.LevelError)
	default:
		logger.SetLevel(logger.LevelInfo)
	}

	// Set up logging
	if isStdio {
		// STDIO mode: stdout is reserved for MCP JSON-RPC protocol.
		// Log to file + stderr so MCP communication is not corrupted.
		cleanupOldLogs(cli.LogsDir, "unraid-management-agent")

		fileLogger := &lumberjack.Logger{
			Filename:   filepath.Join(cli.LogsDir, "unraid-management-agent.log"),
			MaxSize:    5,
			MaxBackups: 1,
			MaxAge:     1,
			Compress:   false,
		}
		multiWriter := io.MultiWriter(fileLogger, os.Stderr)
		log.SetOutput(multiWriter)
	} else if cli.Debug {
		// Debug mode: direct stdout/stderr with no buffering
		log.SetOutput(os.Stdout)
		log.SetFlags(log.LstdFlags | log.Lshortfile)
		logger.SetLevel(logger.LevelDebug)
		log.Println("Debug mode enabled - logging to stdout")
	} else {
		// Clean up old rotated log files from previous versions
		cleanupOldLogs(cli.LogsDir, "unraid-management-agent")

		// Production mode: log rotation with 5MB max size
		// MaxAge=1 ensures old backup files are deleted after 1 day
		fileLogger := &lumberjack.Logger{
			Filename:   filepath.Join(cli.LogsDir, "unraid-management-agent.log"),
			MaxSize:    5,     // 5 MB max file size
			MaxBackups: 1,     // Keep only 1 backup file
			MaxAge:     1,     // Delete backups older than 1 day
			Compress:   false, // No compression
		}
		// Write to both file and stdout
		multiWriter := io.MultiWriter(fileLogger, os.Stdout)
		log.SetOutput(multiWriter)
	}

	log.Printf("Starting Unraid Management Agent v%s (log level: %s)", Version, cli.LogLevel)

	// Parse disabled collectors from CLI/env and create a map
	disabledCollectors := make(map[string]bool)
	if cli.DisableCollectors != "" {
		for name := range strings.SplitSeq(cli.DisableCollectors, ",") {
			name = strings.TrimSpace(strings.ToLower(name))
			if name == "" {
				continue
			}
			if name == "system" {
				log.Printf("WARNING: Cannot disable system collector (always required), ignoring")
				continue
			}
			if !validCollectorNames[name] {
				log.Printf("WARNING: Unknown collector name '%s' in disable list, ignoring", name)
				continue
			}
			disabledCollectors[name] = true
			log.Printf("Collector '%s' disabled via UNRAID_DISABLE_COLLECTORS", name)
		}
	}

	// Helper function to get interval (returns 0 if collector is disabled)
	// In low power mode, intervals are multiplied by 4 for reduced CPU usage
	getInterval := func(name string, cliInterval int) int {
		if disabledCollectors[name] {
			return 0
		}
		if cli.LowPowerMode && cliInterval > 0 {
			return cliInterval * 4
		}
		return cliInterval
	}

	if cli.LowPowerMode {
		log.Printf("Low power mode enabled - all intervals multiplied by 4x")
	}

	// Create application context with intervals from CLI/env
	appCtx := &domain.Context{
		Config: domain.Config{
			Version:    Version,
			Port:       cli.Port,
			CORSOrigin: cli.CORSOrigin,
		},
		Hub: domain.NewEventBus(1024), // Buffer size for event bus
		MQTTConfig: domain.MQTTConfig{
			Enabled:             cli.MQTTEnabled,
			Broker:              cli.MQTTBroker,
			Port:                cli.MQTTPort,
			Username:            cli.MQTTUsername,
			Password:            cli.MQTTPassword,
			ClientID:            cli.MQTTClientID,
			UseTLS:              cli.MQTTUseTLS,
			InsecureSkipVerify:  cli.MQTTInsecureSkipVerify,
			TopicPrefix:         cli.MQTTTopicPrefix,
			QoS:                 cli.MQTTQoS,
			RetainMessages:      cli.MQTTRetain,
			HomeAssistantMode:   cli.MQTTHomeAssistant,
			HomeAssistantPrefix: cli.MQTTHAPrefix,
			DiscoveryEnabled:    cli.MQTTHomeAssistant, // Enable discovery when HA mode is enabled
		},
		Intervals: domain.Intervals{
			System:       getInterval("system", cli.IntervalSystem),
			Array:        getInterval("array", cli.IntervalArray),
			Disk:         getInterval("disk", cli.IntervalDisk),
			Docker:       getInterval("docker", cli.IntervalDocker),
			VM:           getInterval("vm", cli.IntervalVM),
			UPS:          getInterval("ups", cli.IntervalUPS),
			NUT:          getInterval("nut", cli.IntervalNUT),
			GPU:          getInterval("gpu", cli.IntervalGPU),
			Shares:       getInterval("shares", cli.IntervalShares),
			Network:      getInterval("network", cli.IntervalNetwork),
			Hardware:     getInterval("hardware", cli.IntervalHardware),
			ZFS:          getInterval("zfs", cli.IntervalZFS),
			Notification: getInterval("notification", cli.IntervalNotification),
			Registration: getInterval("registration", cli.IntervalRegistration),
			Unassigned:   getInterval("unassigned", cli.IntervalUnassigned),
		},
	}

	// Run the boot command
	err = ctx.Run(appCtx)
	ctx.FatalIfErrorf(err)
}

// applyFileConfig merges config file values into the CLI struct.
// Only fields not explicitly set via CLI/env are overridden.
// Kong sets fields to their declared defaults before parsing, so file config
// values are applied after kong.Parse to fill in non-defaulted values.
// In practice this means file config acts as a "second default layer":
// CLI flag > env var > config file > struct default.
func applyFileConfig(cfg *domain.FileConfig) {
	if cfg == nil {
		return
	}

	setInt := func(dst *int, src *int) {
		if src != nil {
			*dst = *src
		}
	}
	setStr := func(dst *string, src *string) {
		if src != nil {
			*dst = *src
		}
	}
	setBool := func(dst *bool, src *bool) {
		if src != nil {
			*dst = *src
		}
	}

	// Server settings
	setInt(&cli.Port, cfg.Port)
	setStr(&cli.LogLevel, cfg.LogLevel)
	setStr(&cli.LogsDir, cfg.LogsDir)
	setBool(&cli.Debug, cfg.Debug)
	setBool(&cli.LowPowerMode, cfg.LowPowerMode)
	setStr(&cli.DisableCollectors, cfg.DisableCollectors)
	setStr(&cli.CORSOrigin, cfg.CORSOrigin)

	// MQTT
	if m := cfg.MQTT; m != nil {
		setBool(&cli.MQTTEnabled, m.Enabled)
		setStr(&cli.MQTTBroker, m.Broker)
		setInt(&cli.MQTTPort, m.Port)
		setStr(&cli.MQTTUsername, m.Username)
		setStr(&cli.MQTTPassword, m.Password)
		setStr(&cli.MQTTClientID, m.ClientID)
		setStr(&cli.MQTTTopicPrefix, m.TopicPrefix)
		setBool(&cli.MQTTUseTLS, m.UseTLS)
		setBool(&cli.MQTTInsecureSkipVerify, m.InsecureSkipVerify)
		setInt(&cli.MQTTQoS, m.QoS)
		setBool(&cli.MQTTRetain, m.Retain)
		setBool(&cli.MQTTHomeAssistant, m.HomeAssistant)
		setStr(&cli.MQTTHAPrefix, m.HAPrefix)
	}

	// Intervals
	if iv := cfg.Intervals; iv != nil {
		setInt(&cli.IntervalSystem, iv.System)
		setInt(&cli.IntervalArray, iv.Array)
		setInt(&cli.IntervalDisk, iv.Disk)
		setInt(&cli.IntervalDocker, iv.Docker)
		setInt(&cli.IntervalVM, iv.VM)
		setInt(&cli.IntervalUPS, iv.UPS)
		setInt(&cli.IntervalNUT, iv.NUT)
		setInt(&cli.IntervalGPU, iv.GPU)
		setInt(&cli.IntervalShares, iv.Shares)
		setInt(&cli.IntervalNetwork, iv.Network)
		setInt(&cli.IntervalHardware, iv.Hardware)
		setInt(&cli.IntervalZFS, iv.ZFS)
		setInt(&cli.IntervalNotification, iv.Notification)
		setInt(&cli.IntervalRegistration, iv.Registration)
		setInt(&cli.IntervalUnassigned, iv.Unassigned)
	}
}
