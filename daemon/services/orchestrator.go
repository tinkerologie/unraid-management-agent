// Package services provides the orchestration layer for managing collectors, API server, and application lifecycle.
package services

import (
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/services/api"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/services/mcp"
)

// Orchestrator coordinates the lifecycle of all collectors, API server, and handles graceful shutdown.
// It manages the initialization order, starts all components, and ensures proper cleanup on termination.
type Orchestrator struct {
	ctx              *domain.Context
	collectorManager *CollectorManager
}

// CreateOrchestrator creates a new orchestrator with the given context.
func CreateOrchestrator(ctx *domain.Context) *Orchestrator {
	return &Orchestrator{ctx: ctx}
}

// Run starts all collectors and the API server, then waits for a termination signal.
// It ensures proper initialization order and handles graceful shutdown of all components.
func (o *Orchestrator) Run() error {
	logger.Info("Starting Unraid Management Agent v%s", o.ctx.Version)

	// WaitGroup to track all goroutines
	var wg sync.WaitGroup

	// Initialize collector manager
	o.collectorManager = NewCollectorManager(o.ctx, &wg)

	// Register all collectors with their configured intervals
	o.collectorManager.RegisterAllCollectors()

	// Initialize API server FIRST so subscriptions are ready
	// Pass the collector manager for runtime control
	apiServer := api.NewServerWithCollectorManager(o.ctx, o.collectorManager)

	// Start API server subscriptions and WebSocket hub
	apiServer.StartSubscriptions()
	logger.Success("API server subscriptions ready")

	// Small delay to ensure subscriptions are fully set up
	time.Sleep(100 * time.Millisecond)

	// Initialize MCP server for AI agent integration (HTTP transport)
	mcpServer := mcp.NewServer(o.ctx, apiServer)
	if err := mcpServer.Initialize(); err != nil {
		logger.Error("Failed to initialize MCP server: %v", err)
	} else {
		// Register MCP endpoint on the API router
		apiServer.GetRouter().HandleFunc("/mcp", mcpServer.GetHandler()).Methods("POST", "OPTIONS")
		logger.Success("MCP server initialized at /mcp endpoint")
	}

	// Initialize SSE MCP server for VS Code and other SSE clients
	mcpSSEServer := mcp.NewServerWithTransport(o.ctx, apiServer, mcp.TransportSSE)
	if err := mcpSSEServer.Initialize(); err != nil {
		logger.Error("Failed to initialize MCP SSE server: %v", err)
	} else {
		// Register SSE endpoints - GET for event stream, POST for messages
		apiServer.GetRouter().HandleFunc("/mcp/sse", mcpSSEServer.GetSSEHandler()).Methods("GET")
		apiServer.GetRouter().HandleFunc("/mcp/sse", mcpSSEServer.GetSSEPostHandler()).Methods("POST", "OPTIONS")
		logger.Success("MCP SSE server initialized at /mcp/sse endpoint")
	}

	// Start all enabled collectors
	enabledCount := o.collectorManager.StartAll()

	// Log status
	status := o.collectorManager.GetAllStatus()
	logger.Success("%d collectors started", enabledCount)
	if status.DisabledCount > 0 {
		disabledNames := []string{}
		for _, c := range status.Collectors {
			if !c.Enabled {
				disabledNames = append(disabledNames, c.Name)
			}
		}
		logger.Info("Disabled collectors: %v", disabledNames)
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
	// 1. Stop all collectors via manager
	o.collectorManager.StopAll()

	// 2. Stop API server (which also cancels its internal goroutines)
	apiServer.Stop()

	// 3. Wait for all goroutines to complete
	logger.Info("Waiting for all goroutines to complete...")
	wg.Wait()

	logger.Info("Shutdown complete")

	return nil
}
