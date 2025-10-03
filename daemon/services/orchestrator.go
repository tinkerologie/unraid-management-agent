package services

import (
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ruaandeysel/unraid-management-agent/daemon/common"
	"github.com/ruaandeysel/unraid-management-agent/daemon/domain"
	"github.com/ruaandeysel/unraid-management-agent/daemon/logger"
	"github.com/ruaandeysel/unraid-management-agent/daemon/services/api"
	"github.com/ruaandeysel/unraid-management-agent/daemon/services/collectors"
)

type Orchestrator struct {
	ctx *domain.Context
}

func CreateOrchestrator(ctx *domain.Context) *Orchestrator {
	return &Orchestrator{ctx: ctx}
}

func (o *Orchestrator) Run() error {
	logger.Info("Starting Unraid Management Agent v%s", o.ctx.Version)

	// Initialize API server FIRST so subscriptions are ready
	apiServer := api.NewServer(o.ctx)
	
	// Start API server subscriptions and WebSocket hub
	apiServer.StartSubscriptions()
	logger.Success("API server subscriptions ready")
	
	// Small delay to ensure subscriptions are fully set up
	time.Sleep(100 * time.Millisecond)

	// Initialize collectors
	systemCollector := collectors.NewSystemCollector(o.ctx)
	arrayCollector := collectors.NewArrayCollector(o.ctx)
	diskCollector := collectors.NewDiskCollector(o.ctx)
	dockerCollector := collectors.NewDockerCollector(o.ctx)
	vmCollector := collectors.NewVMCollector(o.ctx)
	upsCollector := collectors.NewUPSCollector(o.ctx)
	gpuCollector := collectors.NewGPUCollector(o.ctx)
	shareCollector := collectors.NewShareCollector(o.ctx)
	networkCollector := collectors.NewNetworkCollector(o.ctx)

	// Start collectors
	go systemCollector.Start(time.Duration(common.IntervalSystem) * time.Second)
	go arrayCollector.Start(time.Duration(common.IntervalArray) * time.Second)
	go diskCollector.Start(time.Duration(common.IntervalDisk) * time.Second)
	go dockerCollector.Start(time.Duration(common.IntervalDocker) * time.Second)
	go vmCollector.Start(time.Duration(common.IntervalVM) * time.Second)
	go upsCollector.Start(time.Duration(common.IntervalUPS) * time.Second)
	go gpuCollector.Start(time.Duration(common.IntervalGPU) * time.Second)
	go shareCollector.Start(time.Duration(common.IntervalShares) * time.Second)
	go networkCollector.Start(time.Duration(common.IntervalNetwork) * time.Second)

	logger.Success("All collectors started")

	// Start HTTP server
	go func() {
		if err := apiServer.StartHTTP(); err != nil {
			logger.Error("API server error: %v", err)
		}
	}()

	logger.Success("API server started on port %d", o.ctx.Port)

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)
	sig := <-sigChan

	logger.Warning("Received %s signal, shutting down...", sig)

	// Graceful shutdown
	apiServer.Stop()
	logger.Info("Shutdown complete")

	return nil
}
