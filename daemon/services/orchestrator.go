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
	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/services/api"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/services/mcp"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/services/mqtt"
)

// Orchestrator coordinates the lifecycle of all collectors, API server, and handles graceful shutdown.
// It manages the initialization order, starts all components, and ensures proper cleanup on termination.
type Orchestrator struct {
	ctx              *domain.Context
	collectorManager *CollectorManager
	mqttClient       *mqtt.Client
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

	// Create context for cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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

	// Initialize MQTT client if enabled
	if o.ctx.MQTTConfig.Enabled {
		o.initializeMQTT(ctx, &wg, apiServer)
	}

	// Initialize Streamable HTTP MCP server (MCP spec 2025-03-26) for modern AI clients
	// This is the primary MCP endpoint supporting Cursor, Claude, GitHub Copilot, Codex, Windsurf, Gemini, etc.
	mcpStreamableServer := mcp.NewServerWithTransport(o.ctx, apiServer, mcp.TransportStreamableHTTP)
	if err := mcpStreamableServer.Initialize(); err != nil {
		logger.Error("Failed to initialize MCP Streamable HTTP server: %v", err)
	} else {
		// Single endpoint supporting POST, GET, DELETE, OPTIONS per the Streamable HTTP spec
		apiServer.GetRouter().HandleFunc("/mcp", mcpStreamableServer.GetStreamableHTTPHandler()).
			Methods("POST", "GET", "DELETE", "OPTIONS")
		logger.Success("MCP Streamable HTTP server initialized at /mcp endpoint")
	}

	// Initialize legacy SSE MCP server for backward compatibility with older clients
	// Clients using the deprecated HTTP+SSE transport (spec 2024-11-05) connect here
	mcpSSEServer := mcp.NewServerWithTransport(o.ctx, apiServer, mcp.TransportSSE)
	if err := mcpSSEServer.Initialize(); err != nil {
		logger.Error("Failed to initialize MCP SSE server: %v", err)
	} else {
		// Register SSE endpoints - GET for event stream, POST for messages
		apiServer.GetRouter().HandleFunc("/mcp/sse", mcpSSEServer.GetSSEHandler()).Methods("GET")
		apiServer.GetRouter().HandleFunc("/mcp/sse", mcpSSEServer.GetSSEPostHandler()).Methods("POST", "OPTIONS")
		logger.Success("MCP SSE server initialized at /mcp/sse endpoint (legacy)")
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

	// Cancel the context to stop all goroutines
	cancel()

	// Graceful shutdown
	// 1. Stop MQTT client if running
	if o.mqttClient != nil {
		o.mqttClient.Disconnect()
		logger.Info("MQTT client disconnected")
	}

	// 2. Stop all collectors via manager
	o.collectorManager.StopAll()

	// 3. Stop API server (which also cancels its internal goroutines)
	apiServer.Stop()

	// 4. Wait for all goroutines to complete
	logger.Info("Waiting for all goroutines to complete...")
	wg.Wait()

	logger.Info("Shutdown complete")

	return nil
}

// initializeMQTT sets up the MQTT client and starts publishing events.
func (o *Orchestrator) initializeMQTT(ctx context.Context, wg *sync.WaitGroup, apiServer *api.Server) {
	// Get hostname for MQTT client
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unraid"
	}

	// Convert domain config to DTO config
	mqttConfig := o.ctx.MQTTConfig.ToDTOConfig()

	// Create MQTT client
	o.mqttClient = mqtt.NewClient(mqttConfig, hostname, o.ctx.Version)

	// Connect to broker
	if err := o.mqttClient.Connect(ctx); err != nil {
		logger.Error("Failed to connect to MQTT broker: %v", err)
		return
	}

	logger.Success("MQTT client connected to %s", o.ctx.MQTTConfig.Broker)

	// Set MQTT client on API server for REST endpoints
	apiServer.SetMQTTClient(o.mqttClient)

	// Start MQTT event subscriber
	wg.Add(1)
	go func() {
		defer wg.Done()
		o.subscribeMQTTEvents(ctx, apiServer)
	}()
}

// subscribeMQTTEvents subscribes to collector events and publishes them via MQTT.
func (o *Orchestrator) subscribeMQTTEvents(ctx context.Context, apiServer *api.Server) {
	logger.Info("MQTT: Starting event subscription...")

	// Subscribe to all relevant events
	ch := o.ctx.Hub.Sub(
		"system_update",
		"array_status_update",
		"disk_list_update",
		"share_list_update",
		"container_list_update",
		"vm_list_update",
		"ups_status_update",
		"gpu_metrics_update",
		"network_list_update",
		"notifications_update",
	)

	defer o.ctx.Hub.Unsub(ch)

	for {
		select {
		case <-ctx.Done():
			logger.Info("MQTT: Event subscription stopping")
			return
		case msg := <-ch:
			o.handleMQTTEvent(msg)
		}
	}
}

// handleMQTTEvent processes an event and publishes it via MQTT.
func (o *Orchestrator) handleMQTTEvent(msg interface{}) {
	if o.mqttClient == nil || !o.mqttClient.IsConnected() {
		return
	}

	switch v := msg.(type) {
	case *dto.SystemInfo:
		if err := o.mqttClient.PublishSystemInfo(v); err != nil {
			logger.Debug("MQTT: Failed to publish system info: %v", err)
		}
	case *dto.ArrayStatus:
		if err := o.mqttClient.PublishArrayStatus(v); err != nil {
			logger.Debug("MQTT: Failed to publish array status: %v", err)
		}
	case []dto.DiskInfo:
		if err := o.mqttClient.PublishDisks(v); err != nil {
			logger.Debug("MQTT: Failed to publish disks: %v", err)
		}
	case []dto.ShareInfo:
		if err := o.mqttClient.PublishShares(v); err != nil {
			logger.Debug("MQTT: Failed to publish shares: %v", err)
		}
	case []*dto.ContainerInfo:
		// Convert pointer slice to value slice
		containers := make([]dto.ContainerInfo, len(v))
		for i, c := range v {
			containers[i] = *c
		}
		if err := o.mqttClient.PublishContainers(containers); err != nil {
			logger.Debug("MQTT: Failed to publish containers: %v", err)
		}
	case []*dto.VMInfo:
		// Convert pointer slice to value slice
		vms := make([]dto.VMInfo, len(v))
		for i, vm := range v {
			vms[i] = *vm
		}
		if err := o.mqttClient.PublishVMs(vms); err != nil {
			logger.Debug("MQTT: Failed to publish VMs: %v", err)
		}
	case *dto.UPSStatus:
		if err := o.mqttClient.PublishUPSStatus(v); err != nil {
			logger.Debug("MQTT: Failed to publish UPS status: %v", err)
		}
	case []*dto.GPUMetrics:
		if err := o.mqttClient.PublishGPUMetrics(v); err != nil {
			logger.Debug("MQTT: Failed to publish GPU metrics: %v", err)
		}
	case []dto.NetworkInfo:
		if err := o.mqttClient.PublishNetworkInfo(v); err != nil {
			logger.Debug("MQTT: Failed to publish network info: %v", err)
		}
	case *dto.NotificationList:
		if err := o.mqttClient.PublishNotifications(v); err != nil {
			logger.Debug("MQTT: Failed to publish notifications: %v", err)
		}
	}
}
