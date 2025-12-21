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

var cli struct {
	LogsDir  string `default:"/var/log" help:"directory to store logs"`
	Port     int    `default:"8043" help:"HTTP server port"`
	Debug    bool   `default:"false" help:"enable debug mode with stdout logging"`
	LogLevel string `default:"warning" help:"log level: debug, info, warning, error"`

	// Collection intervals (overridable via environment variables)
	IntervalSystem       int `default:"15" env:"INTERVAL_SYSTEM" help:"system metrics collection interval (seconds)"`
	IntervalArray        int `default:"30" env:"INTERVAL_ARRAY" help:"array metrics collection interval (seconds)"`
	IntervalDisk         int `default:"30" env:"INTERVAL_DISK" help:"disk metrics collection interval (seconds)"`
	IntervalDocker       int `default:"30" env:"INTERVAL_DOCKER" help:"docker metrics collection interval (seconds)"`
	IntervalVM           int `default:"30" env:"INTERVAL_VM" help:"VM metrics collection interval (seconds)"`
	IntervalUPS          int `default:"60" env:"INTERVAL_UPS" help:"UPS metrics collection interval (seconds)"`
	IntervalGPU          int `default:"60" env:"INTERVAL_GPU" help:"GPU metrics collection interval (seconds)"`
	IntervalShares       int `default:"60" env:"INTERVAL_SHARES" help:"shares metrics collection interval (seconds)"`
	IntervalNetwork      int `default:"30" env:"INTERVAL_NETWORK" help:"network metrics collection interval (seconds)"`
	IntervalHardware     int `default:"300" env:"INTERVAL_HARDWARE" help:"hardware metrics collection interval (seconds)"`
	IntervalZFS          int `default:"30" env:"INTERVAL_ZFS" help:"ZFS metrics collection interval (seconds)"`
	IntervalNotification int `default:"30" env:"INTERVAL_NOTIFICATION" help:"notification collection interval (seconds)"`
	IntervalRegistration int `default:"300" env:"INTERVAL_REGISTRATION" help:"registration collection interval (seconds)"`
	IntervalUnassigned   int `default:"60" env:"INTERVAL_UNASSIGNED" help:"unassigned devices collection interval (seconds)"`

	Boot cmd.Boot `cmd:"" default:"1" help:"start the management agent"`
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
		logger.SetLevel(logger.LevelWarning)
	}

	// Set up logging
	if cli.Debug {
		// Debug mode: direct stdout/stderr with no buffering
		log.SetOutput(os.Stdout)
		log.SetFlags(log.LstdFlags | log.Lshortfile)
		logger.SetLevel(logger.LevelDebug)
		log.Println("Debug mode enabled - logging to stdout")
	} else {
		// Production mode: log rotation with 5MB max size, NO backups
		fileLogger := &lumberjack.Logger{
			Filename:   filepath.Join(cli.LogsDir, "unraid-management-agent.log"),
			MaxSize:    5,     // 5 MB max file size
			MaxBackups: 0,     // No backup files - only keep current log
			MaxAge:     0,     // No age-based retention
			Compress:   false, // No compression
		}
		// Write to both file and stdout
		multiWriter := io.MultiWriter(fileLogger, os.Stdout)
		log.SetOutput(multiWriter)
	}

	log.Printf("Starting Unraid Management Agent v%s (log level: %s)", Version, cli.LogLevel)

	// Create application context with intervals from CLI/env
	appCtx := &domain.Context{
		Config: domain.Config{
			Version: Version,
			Port:    cli.Port,
		},
		Hub: pubsub.New(1024), // Buffer size for event bus
		Intervals: domain.Intervals{
			System:       cli.IntervalSystem,
			Array:        cli.IntervalArray,
			Disk:         cli.IntervalDisk,
			Docker:       cli.IntervalDocker,
			VM:           cli.IntervalVM,
			UPS:          cli.IntervalUPS,
			GPU:          cli.IntervalGPU,
			Shares:       cli.IntervalShares,
			Network:      cli.IntervalNetwork,
			Hardware:     cli.IntervalHardware,
			ZFS:          cli.IntervalZFS,
			Notification: cli.IntervalNotification,
			Registration: cli.IntervalRegistration,
			Unassigned:   cli.IntervalUnassigned,
		},
	}

	// Run the boot command
	err := ctx.Run(appCtx)
	ctx.FatalIfErrorf(err)
}
