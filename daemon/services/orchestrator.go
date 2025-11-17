// Package services provides the orchestration layer for managing collectors, API server, and application lifecycle.
package services

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/constants"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/services/api"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/services/collectors"
)

// Orchestrator coordinates the lifecycle of all collectors, API server, and handles graceful shutdown.
// It manages the initialization order, starts all components, and ensures proper cleanup on termination.
type Orchestrator struct {
	ctx *domain.Context
}

// CreateOrchestrator creates a new orchestrator with the given context.
func CreateOrchestrator(ctx *domain.Context) *Orchestrator {
	return &Orchestrator{ctx: ctx}
}

// Run starts all collectors and the API server, then waits for a termination signal.
// It ensures proper initialization order and handles graceful shutdown of all components.
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
	hardwareCollector := collectors.NewHardwareCollector(o.ctx)
	registrationCollector := collectors.NewRegistrationCollector(o.ctx)
	notificationCollector := collectors.NewNotificationCollector(o.ctx)
	unassignedCollector := collectors.NewUnassignedCollector(o.ctx)

	// Start collectors with context and WaitGroup
	wg.Add(13)
	go func() {
		defer wg.Done()
		systemCollector.Start(ctx, time.Duration(constants.IntervalSystem)*time.Second)
	}()
	go func() {
		defer wg.Done()
		arrayCollector.Start(ctx, time.Duration(constants.IntervalArray)*time.Second)
	}()
	go func() {
		defer wg.Done()
		diskCollector.Start(ctx, time.Duration(constants.IntervalDisk)*time.Second)
	}()
	go func() {
		defer wg.Done()
		dockerCollector.Start(ctx, time.Duration(constants.IntervalDocker)*time.Second)
	}()
	go func() {
		defer wg.Done()
		vmCollector.Start(ctx, time.Duration(constants.IntervalVM)*time.Second)
	}()
	go func() {
		defer wg.Done()
		upsCollector.Start(ctx, time.Duration(constants.IntervalUPS)*time.Second)
	}()
	go func() {
		defer wg.Done()
		gpuCollector.Start(ctx, time.Duration(constants.IntervalGPU)*time.Second)
	}()
	go func() {
		defer wg.Done()
		shareCollector.Start(ctx, time.Duration(constants.IntervalShares)*time.Second)
	}()
	go func() {
		defer wg.Done()
		networkCollector.Start(ctx, time.Duration(constants.IntervalNetwork)*time.Second)
	}()
	go func() {
		defer wg.Done()
		hardwareCollector.Start(ctx, time.Duration(constants.IntervalHardware)*time.Second)
	}()
	go func() {
		defer wg.Done()
		registrationCollector.Start(ctx, time.Duration(constants.IntervalHardware)*time.Second)
	}()
	go func() {
		defer wg.Done()
		notificationCollector.Start(ctx, 15*time.Second) // 15 seconds for notifications
	}()
	go func() {
		defer wg.Done()
		unassignedCollector.Start(ctx, 30*time.Second) // 30 seconds for unassigned devices
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
