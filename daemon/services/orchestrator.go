// Package services provides the orchestration layer for managing collectors, API server, and application lifecycle.
package services

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

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

	// Initialize collectors (only if enabled - interval > 0)
	// Interval of 0 means the collector is disabled
	enabledCount := 0
	disabledCollectors := []string{}

	// System collector
	if o.ctx.Intervals.System > 0 {
		systemCollector := collectors.NewSystemCollector(o.ctx)
		wg.Add(1)
		go func() {
			defer wg.Done()
			systemCollector.Start(ctx, time.Duration(o.ctx.Intervals.System)*time.Second)
		}()
		enabledCount++
	} else {
		disabledCollectors = append(disabledCollectors, "system")
	}

	// Array collector
	if o.ctx.Intervals.Array > 0 {
		arrayCollector := collectors.NewArrayCollector(o.ctx)
		wg.Add(1)
		go func() {
			defer wg.Done()
			arrayCollector.Start(ctx, time.Duration(o.ctx.Intervals.Array)*time.Second)
		}()
		enabledCount++
	} else {
		disabledCollectors = append(disabledCollectors, "array")
	}

	// Disk collector
	if o.ctx.Intervals.Disk > 0 {
		diskCollector := collectors.NewDiskCollector(o.ctx)
		wg.Add(1)
		go func() {
			defer wg.Done()
			diskCollector.Start(ctx, time.Duration(o.ctx.Intervals.Disk)*time.Second)
		}()
		enabledCount++
	} else {
		disabledCollectors = append(disabledCollectors, "disk")
	}

	// Docker collector
	if o.ctx.Intervals.Docker > 0 {
		dockerCollector := collectors.NewDockerCollector(o.ctx)
		wg.Add(1)
		go func() {
			defer wg.Done()
			dockerCollector.Start(ctx, time.Duration(o.ctx.Intervals.Docker)*time.Second)
		}()
		enabledCount++
	} else {
		disabledCollectors = append(disabledCollectors, "docker")
	}

	// VM collector
	if o.ctx.Intervals.VM > 0 {
		vmCollector := collectors.NewVMCollector(o.ctx)
		wg.Add(1)
		go func() {
			defer wg.Done()
			vmCollector.Start(ctx, time.Duration(o.ctx.Intervals.VM)*time.Second)
		}()
		enabledCount++
	} else {
		disabledCollectors = append(disabledCollectors, "vm")
	}

	// UPS collector
	if o.ctx.Intervals.UPS > 0 {
		upsCollector := collectors.NewUPSCollector(o.ctx)
		wg.Add(1)
		go func() {
			defer wg.Done()
			upsCollector.Start(ctx, time.Duration(o.ctx.Intervals.UPS)*time.Second)
		}()
		enabledCount++
	} else {
		disabledCollectors = append(disabledCollectors, "ups")
	}

	// NUT collector (separate from UPS - for NUT plugin users)
	if o.ctx.Intervals.NUT > 0 {
		nutCollector := collectors.NewNUTCollector(o.ctx)
		wg.Add(1)
		go func() {
			defer wg.Done()
			nutCollector.Start(ctx, time.Duration(o.ctx.Intervals.NUT)*time.Second)
		}()
		enabledCount++
	} else {
		disabledCollectors = append(disabledCollectors, "nut")
	}

	// GPU collector
	if o.ctx.Intervals.GPU > 0 {
		gpuCollector := collectors.NewGPUCollector(o.ctx)
		wg.Add(1)
		go func() {
			defer wg.Done()
			gpuCollector.Start(ctx, time.Duration(o.ctx.Intervals.GPU)*time.Second)
		}()
		enabledCount++
	} else {
		disabledCollectors = append(disabledCollectors, "gpu")
	}

	// Share collector
	if o.ctx.Intervals.Shares > 0 {
		shareCollector := collectors.NewShareCollector(o.ctx)
		wg.Add(1)
		go func() {
			defer wg.Done()
			shareCollector.Start(ctx, time.Duration(o.ctx.Intervals.Shares)*time.Second)
		}()
		enabledCount++
	} else {
		disabledCollectors = append(disabledCollectors, "shares")
	}

	// Network collector
	if o.ctx.Intervals.Network > 0 {
		networkCollector := collectors.NewNetworkCollector(o.ctx)
		wg.Add(1)
		go func() {
			defer wg.Done()
			networkCollector.Start(ctx, time.Duration(o.ctx.Intervals.Network)*time.Second)
		}()
		enabledCount++
	} else {
		disabledCollectors = append(disabledCollectors, "network")
	}

	// Hardware collector
	if o.ctx.Intervals.Hardware > 0 {
		hardwareCollector := collectors.NewHardwareCollector(o.ctx)
		wg.Add(1)
		go func() {
			defer wg.Done()
			hardwareCollector.Start(ctx, time.Duration(o.ctx.Intervals.Hardware)*time.Second)
		}()
		enabledCount++
	} else {
		disabledCollectors = append(disabledCollectors, "hardware")
	}

	// Registration collector
	if o.ctx.Intervals.Registration > 0 {
		registrationCollector := collectors.NewRegistrationCollector(o.ctx)
		wg.Add(1)
		go func() {
			defer wg.Done()
			registrationCollector.Start(ctx, time.Duration(o.ctx.Intervals.Registration)*time.Second)
		}()
		enabledCount++
	} else {
		disabledCollectors = append(disabledCollectors, "registration")
	}

	// Notification collector
	if o.ctx.Intervals.Notification > 0 {
		notificationCollector := collectors.NewNotificationCollector(o.ctx)
		wg.Add(1)
		go func() {
			defer wg.Done()
			notificationCollector.Start(ctx, time.Duration(o.ctx.Intervals.Notification)*time.Second)
		}()
		enabledCount++
	} else {
		disabledCollectors = append(disabledCollectors, "notification")
	}

	// Unassigned collector
	if o.ctx.Intervals.Unassigned > 0 {
		unassignedCollector := collectors.NewUnassignedCollector(o.ctx)
		wg.Add(1)
		go func() {
			defer wg.Done()
			unassignedCollector.Start(ctx, time.Duration(o.ctx.Intervals.Unassigned)*time.Second)
		}()
		enabledCount++
	} else {
		disabledCollectors = append(disabledCollectors, "unassigned")
	}

	// ZFS collector
	if o.ctx.Intervals.ZFS > 0 {
		zfsCollector := collectors.NewZFSCollector(o.ctx)
		wg.Add(1)
		go func() {
			defer wg.Done()
			zfsCollector.Start(ctx, time.Duration(o.ctx.Intervals.ZFS)*time.Second)
		}()
		enabledCount++
	} else {
		disabledCollectors = append(disabledCollectors, "zfs")
	}

	logger.Success("%d collectors started", enabledCount)
	if len(disabledCollectors) > 0 {
		logger.Info("Disabled collectors: %v", disabledCollectors)
	}

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
