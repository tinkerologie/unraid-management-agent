// Package main is the entry point for the Unraid Management Agent.
// It provides a REST API and WebSocket interface for monitoring and controlling Unraid systems.
package main

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/alecthomas/kong"
	"github.com/cskr/pubsub"
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

	// Low power mode - multiplies all intervals for resource-constrained systems
	LowPowerMode bool `default:"false" env:"UNRAID_LOW_POWER" help:"enable low power mode (4x longer intervals for old/slow hardware)"`

	// Collector disable flag (alternative to setting interval=0)
	DisableCollectors string `default:"" env:"UNRAID_DISABLE_COLLECTORS" help:"comma-separated list of collectors to disable (e.g., gpu,ups,zfs)"`

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

	Boot cmd.Boot `cmd:"" default:"1" help:"start the management agent"`
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
	if cli.Debug {
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
		for _, name := range strings.Split(cli.DisableCollectors, ",") {
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
			Version: Version,
			Port:    cli.Port,
		},
		Hub: pubsub.New(1024), // Buffer size for event bus
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
	err := ctx.Run(appCtx)
	ctx.FatalIfErrorf(err)
}
