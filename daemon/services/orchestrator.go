// Package services provides the orchestration layer for managing collectors, API server, and application lifecycle.
package services

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/common"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/services/api"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/services/collectors"
)

type Orchestrator struct {
	ctx *domain.Context
}

func CreateOrchestrator(ctx *domain.Context) *Orchestrator {
	return &Orchestrator{ctx: ctx}
}

func (o *Orchestrator) Run() error {
	logger.Info("Starting Unraid Management Agent v%s", o.ctx.Version)

	// Create cancellable context for all goroutines
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// WaitGroup to track all goroutines
	var wg sync.WaitGroup

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

	// Start collectors with context and WaitGroup
	wg.Add(9)
	go func() {
		defer wg.Done()
		systemCollector.Start(ctx, time.Duration(common.IntervalSystem)*time.Second)
	}()
	go func() {
		defer wg.Done()
		arrayCollector.Start(ctx, time.Duration(common.IntervalArray)*time.Second)
	}()
	go func() {
		defer wg.Done()
		diskCollector.Start(ctx, time.Duration(common.IntervalDisk)*time.Second)
	}()
	go func() {
		defer wg.Done()
		dockerCollector.Start(ctx, time.Duration(common.IntervalDocker)*time.Second)
	}()
	go func() {
		defer wg.Done()
		vmCollector.Start(ctx, time.Duration(common.IntervalVM)*time.Second)
	}()
	go func() {
		defer wg.Done()
		upsCollector.Start(ctx, time.Duration(common.IntervalUPS)*time.Second)
	}()
	go func() {
		defer wg.Done()
		gpuCollector.Start(ctx, time.Duration(common.IntervalGPU)*time.Second)
	}()
	go func() {
		defer wg.Done()
		shareCollector.Start(ctx, time.Duration(common.IntervalShares)*time.Second)
	}()
	go func() {
		defer wg.Done()
		networkCollector.Start(ctx, time.Duration(common.IntervalNetwork)*time.Second)
	}()

	logger.Success("All collectors started")

	// Start HTTP server
	wg.Add(1)
	go func() {
		defer wg.Done()
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
	// 1. Cancel context to stop all goroutines
	cancel()

	// 2. Stop API server (which also cancels its internal goroutines)
	apiServer.Stop()

	// 3. Wait for all goroutines to complete
	logger.Info("Waiting for all goroutines to complete...")
	wg.Wait()

	logger.Info("Shutdown complete")

	return nil
}
