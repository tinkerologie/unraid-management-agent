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

	// Create application context
	appCtx := &domain.Context{
		Config: domain.Config{
			Version: Version,
			Port:    cli.Port,
		},
		Hub: pubsub.New(1024), // Buffer size for event bus
	}

	// Run the boot command
	err := ctx.Run(appCtx)
	ctx.FatalIfErrorf(err)
}
