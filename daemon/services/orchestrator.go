// Package services provides the orchestration layer for managing collectors, API server, and application lifecycle.
package services

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"reflect"
	"sync"
	"syscall"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/constants"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/services/alerting"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/services/api"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/services/mcp"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/services/mqtt"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/services/watchdog"
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

	// Create context that cancels on shutdown signals
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	// Initialize collector manager
	o.collectorManager = NewCollectorManager(o.ctx, &wg)

	// Register all collectors with their configured intervals
	o.collectorManager.RegisterAllCollectors()

	// Initialize API server FIRST so subscriptions are ready
	// Pass the collector manager for runtime control
	apiServer := api.NewServerWithCollectorManager(o.ctx, o.collectorManager)

	// Start API server subscriptions and WebSocket hub
	apiServer.StartSubscriptions()

	// Wait for subscriptions to be fully wired (deterministic, replaces time.Sleep)
	<-apiServer.Ready()
	logger.Success("API server subscriptions ready")

	// Initialize MQTT client if enabled
	if o.ctx.MQTTConfig.Enabled {
		o.initializeMQTT(ctx, &wg, apiServer)
	}

	// Initialize MCP server with Streamable HTTP transport (MCP spec 2025-06-18)
	// Uses the official MCP Go SDK for protocol compliance with Claude, ChatGPT, Cursor, Copilot, etc.
	mcpServer := mcp.NewServer(o.ctx, apiServer)
	if err := mcpServer.Initialize(); err != nil {
		logger.Error("Failed to initialize MCP server: %v", err)
	} else {
		// Mount as PathPrefix handler — the StreamableHTTPHandler manages all HTTP methods internally
		apiServer.GetRouter().PathPrefix("/mcp").Handler(mcpServer.GetHTTPHandler())
		logger.Success("MCP server initialized at /mcp endpoint (official SDK, protocol 2025-06-18)")
	}

	// Initialize alerting engine
	alertStore := alerting.NewStore("")
	alertEngine := alerting.NewEngine(alertStore, apiServer)
	apiServer.SetAlertEngine(alertEngine, alertStore)
	mcpServer.SetAlertEngine(alertEngine, alertStore)
	wg.Go(func() {
		alertEngine.Start(ctx)
	})
	logger.Success("Alerting engine started")

	// Initialize watchdog (health checks)
	watchdogStore := watchdog.NewStore("")
	watchdogRunner := watchdog.NewRunner(watchdogStore)
	watchdog.SetDockerProvider(apiServer)
	apiServer.SetWatchdog(watchdogRunner, watchdogStore)
	mcpServer.SetWatchdog(watchdogRunner, watchdogStore)
	wg.Go(func() {
		watchdogRunner.Start(ctx)
	})
	logger.Success("Watchdog started")

	// Start all enabled collectors
	enabledCount := o.collectorManager.StartAll()

	// Log status
	status := o.collectorManager.GetAllStatus()
	logger.Success("%d collectors started", enabledCount)
	if status.DisabledCount > 0 {
		var disabledNames []string
		for _, c := range status.Collectors {
			if !c.Enabled {
				disabledNames = append(disabledNames, c.Name)
			}
		}
		logger.Info("Disabled collectors: %v", disabledNames)
	}

	// Start HTTP server
	wg.Go(func() {
		if err := apiServer.StartHTTP(); err != nil {
			logger.Error("API server error: %v", err)
		}
	})

	logger.Success("API server started on port %d", o.ctx.Port)

	// Wait for shutdown signal
	<-ctx.Done()
	stop() // unregister signal handler immediately

	logger.Warning("Received shutdown signal, shutting down...")

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

// RunMCPStdio starts the agent with MCP over STDIO transport for local AI client integration.
// It starts collectors and the API server's cache (for data) but does NOT start the HTTP server.
// The MCP server communicates exclusively via stdin/stdout using newline-delimited JSON.
// This is designed to be spawned by MCP clients like Claude Desktop running locally on the Unraid server.
func (o *Orchestrator) RunMCPStdio() error {
	logger.Info("Starting Unraid Management Agent v%s (MCP STDIO mode)", o.ctx.Version)

	var wg sync.WaitGroup
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize collector manager and register all collectors
	o.collectorManager = NewCollectorManager(o.ctx, &wg)
	o.collectorManager.RegisterAllCollectors()

	// Initialize API server for cache/subscriptions only (no HTTP)
	apiServer := api.NewServerWithCollectorManager(o.ctx, o.collectorManager)
	apiServer.StartSubscriptions()

	// Wait for subscriptions to be fully wired (deterministic, replaces time.Sleep)
	<-apiServer.Ready()
	logger.Success("API server subscriptions ready (cache mode)")

	// Start all enabled collectors so cache gets populated
	enabledCount := o.collectorManager.StartAll()
	logger.Success("%d collectors started for MCP STDIO", enabledCount)

	// Initialize MCP server
	mcpServer := mcp.NewServer(o.ctx, apiServer)
	if err := mcpServer.Initialize(); err != nil {
		cancel()
		o.collectorManager.StopAll()
		apiServer.Stop()
		wg.Wait()
		return fmt.Errorf("failed to initialize MCP server: %w", err)
	}

	// Initialize alerting engine for STDIO mode
	alertStore := alerting.NewStore("")
	alertEngine := alerting.NewEngine(alertStore, apiServer)
	apiServer.SetAlertEngine(alertEngine, alertStore)
	mcpServer.SetAlertEngine(alertEngine, alertStore)
	wg.Go(func() {
		alertEngine.Start(ctx)
	})
	logger.Success("Alerting engine started (STDIO mode)")

	// Initialize watchdog for STDIO mode
	watchdogStore := watchdog.NewStore("")
	watchdogRunner := watchdog.NewRunner(watchdogStore)
	watchdog.SetDockerProvider(apiServer)
	apiServer.SetWatchdog(watchdogRunner, watchdogStore)
	mcpServer.SetWatchdog(watchdogRunner, watchdogStore)
	wg.Go(func() {
		watchdogRunner.Start(ctx)
	})
	logger.Success("Watchdog started (STDIO mode)")

	// Cancel context on shutdown signals (SIGTERM, SIGINT)
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	// Run MCP over STDIO (blocks until context cancelled or pipe closed)
	logger.Info("MCP STDIO transport ready — waiting for client")
	err := mcpServer.RunSTDIO(ctx)

	// Graceful cleanup
	logger.Info("MCP STDIO transport stopped, cleaning up...")
	o.collectorManager.StopAll()
	apiServer.Stop()
	wg.Wait()
	logger.Info("MCP STDIO shutdown complete")

	return err
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
	o.mqttClient = mqtt.NewClient(mqttConfig, hostname, o.ctx.Version, o.ctx)

	// Connect to broker
	if err := o.mqttClient.Connect(ctx); err != nil {
		logger.Error("Failed to connect to MQTT broker: %v", err)
		return
	}

	logger.Success("MQTT client connected to %s", o.ctx.MQTTConfig.Broker)

	// Set MQTT client on API server for REST endpoints
	apiServer.SetMQTTClient(o.mqttClient)

	// Start MQTT event subscriber
	wg.Go(func() {
		o.subscribeMQTTEvents(ctx, apiServer)
	})
}

// mqttBinding connects a topic to its MQTT publish function.
type mqttBinding struct {
	topicName string
	msgType   reflect.Type
	handle    func(any)
}

// mqttBind creates a type-safe mqttBinding using generics.
func mqttBind[T any](topic domain.Topic[T], fn func(T) error) mqttBinding {
	return mqttBinding{
		topicName: topic.Name,
		msgType:   reflect.TypeFor[T](),
		handle: func(v any) {
			if err := fn(v.(T)); err != nil {
				logger.Debug("MQTT: Failed to publish %T: %v", v, err)
			}
		},
	}
}

// subscribeMQTTEvents subscribes to collector events and publishes them via MQTT.
func (o *Orchestrator) subscribeMQTTEvents(ctx context.Context, _ *api.Server) {
	if o.mqttClient == nil {
		logger.Debug("MQTT: Skipping event subscription — client is nil")
		return
	}

	logger.Info("MQTT: Starting event subscription...")

	bindings := []mqttBinding{
		mqttBind(constants.TopicSystemUpdate, o.mqttClient.PublishSystemInfo),
		mqttBind(constants.TopicArrayStatusUpdate, o.mqttClient.PublishArrayStatus),
		mqttBind(constants.TopicDiskListUpdate, o.mqttClient.PublishDisks),
		mqttBind(constants.TopicShareListUpdate, o.mqttClient.PublishShares),
		mqttBind(constants.TopicContainerListUpdate, func(v []*dto.ContainerInfo) error {
			containers := make([]dto.ContainerInfo, len(v))
			for i, c := range v {
				containers[i] = *c
			}
			return o.mqttClient.PublishContainers(containers)
		}),
		mqttBind(constants.TopicVMListUpdate, func(v []*dto.VMInfo) error {
			vms := make([]dto.VMInfo, len(v))
			for i, vm := range v {
				vms[i] = *vm
			}
			return o.mqttClient.PublishVMs(vms)
		}),
		mqttBind(constants.TopicUPSStatusUpdate, o.mqttClient.PublishUPSStatus),
		mqttBind(constants.TopicGPUMetricsUpdate, o.mqttClient.PublishGPUMetrics),
		mqttBind(constants.TopicNetworkListUpdate, o.mqttClient.PublishNetworkInfo),
		mqttBind(constants.TopicNotificationsUpdate, o.mqttClient.PublishNotifications),
		mqttBind(constants.TopicZFSPoolsUpdate, o.mqttClient.PublishZFSPools),
	}

	topics := make([]string, len(bindings))
	dispatch := make(map[reflect.Type]func(any), len(bindings))
	for i, b := range bindings {
		topics[i] = b.topicName
		dispatch[b.msgType] = b.handle
	}

	ch := o.ctx.Hub.Sub(topics...)
	defer o.ctx.Hub.Unsub(ch)

	for {
		select {
		case <-ctx.Done():
			logger.Info("MQTT: Event subscription stopping")
			return
		case msg := <-ch:
			if o.mqttClient == nil || !o.mqttClient.IsConnected() {
				continue
			}
			if handler, ok := dispatch[reflect.TypeOf(msg)]; ok {
				handler(msg)
			}
		}
	}
}
