package main

import (
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/alecthomas/kong"
	"github.com/cskr/pubsub"
	"gopkg.in/natefinch/lumberjack.v2"

	"github.com/ruaandeysel/unraid-management-agent/daemon/cmd"
	"github.com/ruaandeysel/unraid-management-agent/daemon/domain"
)

var Version string = "dev"

var cli struct {
	LogsDir string `default:"/var/log" help:"directory to store logs"`
	Port    int    `default:"8043" help:"HTTP server port"`
	Debug   bool   `default:"false" help:"enable debug mode with stdout logging"`

	Boot cmd.Boot `cmd:"" default:"1" help:"start the management agent"`
}

func main() {
	ctx := kong.Parse(&cli)

	// Set up logging
	if cli.Debug {
		// Debug mode: direct stdout/stderr with no buffering
		log.SetOutput(os.Stdout)
		log.SetFlags(log.LstdFlags | log.Lshortfile)
		log.Println("Debug mode enabled - logging to stdout")
	} else {
		// Production mode: log rotation with both file and stdout
		fileLogger := &lumberjack.Logger{
			Filename:   filepath.Join(cli.LogsDir, "unraid-management-agent.log"),
			MaxSize:    5,
			MaxBackups: 3,
			MaxAge:     7,
			Compress:   true,
		}
		// Write to both file and stdout
		multiWriter := io.MultiWriter(fileLogger, os.Stdout)
		log.SetOutput(multiWriter)
	}

	log.Printf("Starting Unraid Management Agent v%s", Version)

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
