package api

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/mux"
	httpSwagger "github.com/swaggo/http-swagger/v2"

	_ "github.com/ruaan-deysel/unraid-management-agent/daemon/docs" // Swagger docs
	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/services/collectors"
)

// CollectorManagerInterface defines the methods required from CollectorManager
type CollectorManagerInterface interface {
	EnableCollector(name string) error
	DisableCollector(name string) error
	UpdateInterval(name string, intervalSeconds int) error
	GetStatus(name string) (*dto.CollectorStatus, error)
	GetAllStatus() dto.CollectorsStatusResponse
}

// MQTTClientInterface defines the methods required from MQTT client for API integration
type MQTTClientInterface interface {
	IsConnected() bool
	GetStatus() *dto.MQTTStatus
	TestConnection() error
	PublishCustom(topic string, payload interface{}, retain bool) error
}

// Server represents the HTTP API server that handles REST endpoints and WebSocket connections.
// It maintains an in-memory cache of data from collectors and broadcasts updates to WebSocket clients.
type Server struct {
	ctx              *domain.Context
	httpServer       *http.Server
	router           *mux.Router
	wsHub            *WSHub
	cancelCtx        context.Context
	cancelFunc       context.CancelFunc
	collectorManager CollectorManagerInterface
	mqttClient       MQTTClientInterface

	// Cache for latest data from collectors
	cacheMutex         sync.RWMutex
	systemCache        *dto.SystemInfo
	arrayCache         *dto.ArrayStatus
	disksCache         []dto.DiskInfo
	sharesCache        []dto.ShareInfo
	dockerCache        []dto.ContainerInfo
	vmsCache           []dto.VMInfo
	upsCache           *dto.UPSStatus
	gpuCache           []*dto.GPUMetrics
	networkCache       []dto.NetworkInfo
	hardwareCache      *dto.HardwareInfo
	registrationCache  *dto.Registration
	notificationsCache *dto.NotificationList
	unassignedCache    *dto.UnassignedDeviceList
	zfsPoolsCache      []dto.ZFSPool
	zfsDatasetsCache   []dto.ZFSDataset
	zfsSnapshotsCache  []dto.ZFSSnapshot
	zfsARCStatsCache   *dto.ZFSARCStats
	nutCache           *dto.NUTResponse
}

// NewServer creates a new API server instance with the given context.
// It initializes the HTTP router, WebSocket hub, and sets up all API routes.
func NewServer(ctx *domain.Context) *Server {
	return NewServerWithCollectorManager(ctx, nil)
}

// NewServerWithCollectorManager creates a new API server with a collector manager for runtime control.
func NewServerWithCollectorManager(ctx *domain.Context, cm CollectorManagerInterface) *Server {
	cancelCtx, cancelFunc := context.WithCancel(context.Background())
	s := &Server{
		ctx:              ctx,
		router:           mux.NewRouter(),
		wsHub:            NewWSHub(),
		cancelCtx:        cancelCtx,
		cancelFunc:       cancelFunc,
		collectorManager: cm,
	}

	s.setupRoutes()
	return s
}

func (s *Server) setupRoutes() {
	// Apply middleware
	s.router.Use(corsMiddleware)
	s.router.Use(loggingMiddleware)
	s.router.Use(recoveryMiddleware)

	// Prometheus metrics endpoint (at root level, no /api/v1 prefix)
	s.router.HandleFunc("/metrics", s.handleMetrics).Methods("GET")

	// Swagger UI endpoint (accessible at /swagger/index.html)
	s.router.PathPrefix("/swagger/").Handler(httpSwagger.Handler(
		httpSwagger.URL("/swagger/doc.json"),
		httpSwagger.DeepLinking(true),
		httpSwagger.DocExpansion("none"),
		httpSwagger.DomID("swagger-ui"),
	))

	api := s.router.PathPrefix("/api/v1").Subrouter()

	// Health check
	api.HandleFunc("/health", s.handleHealth).Methods("GET")

	// Monitoring endpoints
	api.HandleFunc("/system", s.handleSystem).Methods("GET")
	api.HandleFunc("/array", s.handleArray).Methods("GET")
	api.HandleFunc("/disks", s.handleDisks).Methods("GET")
	api.HandleFunc("/disks/{id}", s.handleDisk).Methods("GET")
	api.HandleFunc("/shares", s.handleShares).Methods("GET")
	api.HandleFunc("/docker", s.handleDockerList).Methods("GET")
	api.HandleFunc("/docker/{id}", s.handleDockerInfo).Methods("GET")
	api.HandleFunc("/vm", s.handleVMList).Methods("GET")
	api.HandleFunc("/vm/{id}", s.handleVMInfo).Methods("GET")
	api.HandleFunc("/ups", s.handleUPS).Methods("GET")
	api.HandleFunc("/nut", s.handleNUT).Methods("GET")
	api.HandleFunc("/gpu", s.handleGPU).Methods("GET")

	// System control endpoints
	api.HandleFunc("/system/reboot", s.handleSystemReboot).Methods("POST")
	api.HandleFunc("/system/shutdown", s.handleSystemShutdown).Methods("POST")
	api.HandleFunc("/system/flash", s.handleFlashHealth).Methods("GET") // Issue #51
	api.HandleFunc("/network", s.handleNetwork).Methods("GET")
	api.HandleFunc("/network/access-urls", s.handleNetworkAccessURLs).Methods("GET")

	// ZFS endpoints
	api.HandleFunc("/zfs/pools", s.handleZFSPools).Methods("GET")
	api.HandleFunc("/zfs/pools/{name}", s.handleZFSPool).Methods("GET")
	api.HandleFunc("/zfs/datasets", s.handleZFSDatasets).Methods("GET")
	api.HandleFunc("/zfs/snapshots", s.handleZFSSnapshots).Methods("GET")
	api.HandleFunc("/zfs/arc", s.handleZFSARC).Methods("GET")

	// Hardware endpoints
	api.HandleFunc("/hardware/full", s.handleHardwareFull).Methods("GET")
	api.HandleFunc("/hardware/bios", s.handleHardwareBIOS).Methods("GET")
	api.HandleFunc("/hardware/baseboard", s.handleHardwareBaseboard).Methods("GET")
	api.HandleFunc("/hardware/cpu", s.handleHardwareCPU).Methods("GET")
	api.HandleFunc("/hardware/cache", s.handleHardwareCache).Methods("GET")
	api.HandleFunc("/hardware/memory-array", s.handleHardwareMemoryArray).Methods("GET")
	api.HandleFunc("/hardware/memory-devices", s.handleHardwareMemoryDevices).Methods("GET")

	// Control endpoints
	api.HandleFunc("/docker/{id}/start", s.handleDockerStart).Methods("POST")
	api.HandleFunc("/docker/{id}/stop", s.handleDockerStop).Methods("POST")
	api.HandleFunc("/docker/{id}/restart", s.handleDockerRestart).Methods("POST")
	api.HandleFunc("/docker/{id}/pause", s.handleDockerPause).Methods("POST")
	api.HandleFunc("/docker/{id}/unpause", s.handleDockerUnpause).Methods("POST")

	api.HandleFunc("/vm/{name}/start", s.handleVMStart).Methods("POST")
	api.HandleFunc("/vm/{name}/stop", s.handleVMStop).Methods("POST")
	api.HandleFunc("/vm/{name}/restart", s.handleVMRestart).Methods("POST")
	api.HandleFunc("/vm/{name}/pause", s.handleVMPause).Methods("POST")
	api.HandleFunc("/vm/{name}/resume", s.handleVMResume).Methods("POST")
	api.HandleFunc("/vm/{name}/hibernate", s.handleVMHibernate).Methods("POST")
	api.HandleFunc("/vm/{name}/force-stop", s.handleVMForceStop).Methods("POST")

	// Array control endpoints
	api.HandleFunc("/array/start", s.handleArrayStart).Methods("POST")
	api.HandleFunc("/array/stop", s.handleArrayStop).Methods("POST")
	api.HandleFunc("/array/parity-check/start", s.handleParityCheckStart).Methods("POST")
	api.HandleFunc("/array/parity-check/stop", s.handleParityCheckStop).Methods("POST")
	api.HandleFunc("/array/parity-check/pause", s.handleParityCheckPause).Methods("POST")
	api.HandleFunc("/array/parity-check/resume", s.handleParityCheckResume).Methods("POST")
	api.HandleFunc("/array/parity-check/history", s.handleParityCheckHistory).Methods("GET")
	api.HandleFunc("/array/parity-check/schedule", s.handleParitySchedule).Methods("GET") // Issue #47

	// Configuration endpoints (read-only)
	api.HandleFunc("/shares/{name}/config", s.handleShareConfig).Methods("GET")
	api.HandleFunc("/network/{interface}/config", s.handleNetworkConfig).Methods("GET")
	api.HandleFunc("/settings/system", s.handleSystemSettings).Methods("GET")
	api.HandleFunc("/settings/docker", s.handleDockerSettings).Methods("GET")
	api.HandleFunc("/settings/vm", s.handleVMSettings).Methods("GET")
	api.HandleFunc("/settings/disks", s.handleDiskSettings).Methods("GET")
	api.HandleFunc("/settings/disk-thresholds", s.handleDiskSettingsExtended).Methods("GET") // Issue #45
	api.HandleFunc("/settings/mover", s.handleMoverSettings).Methods("GET")                  // Issue #48
	api.HandleFunc("/settings/services", s.handleServiceStatus).Methods("GET")               // Issue #49
	api.HandleFunc("/settings/network-services", s.handleNetworkServices).Methods("GET")     // Network services status

	// Plugin endpoints (Issue #52)
	api.HandleFunc("/plugins", s.handlePluginList).Methods("GET")

	// Update status endpoint (Issue #50)
	api.HandleFunc("/updates", s.handleUpdateStatus).Methods("GET")

	// Configuration endpoints (write)
	api.HandleFunc("/shares/{name}/config", s.handleUpdateShareConfig).Methods("POST")
	api.HandleFunc("/settings/system", s.handleUpdateSystemSettings).Methods("POST")

	// User Scripts endpoints
	api.HandleFunc("/user-scripts", s.handleUserScripts).Methods("GET")
	api.HandleFunc("/user-scripts/{name}/execute", s.handleUserScriptExecute).Methods("POST")

	// Registration/License endpoint
	api.HandleFunc("/registration", s.handleRegistration).Methods("GET")

	// Log file endpoints
	api.HandleFunc("/logs", s.handleLogs).Methods("GET")
	api.HandleFunc("/logs/{filename}", s.handleLogFile).Methods("GET")

	// Notification endpoints (monitoring)
	api.HandleFunc("/notifications", s.handleNotifications).Methods("GET")
	api.HandleFunc("/notifications/unread", s.handleNotificationsUnread).Methods("GET")
	api.HandleFunc("/notifications/archive", s.handleNotificationsArchive).Methods("GET")
	api.HandleFunc("/notifications/overview", s.handleNotificationsOverview).Methods("GET")
	api.HandleFunc("/notifications/{id}", s.handleNotificationByID).Methods("GET")

	// Notification endpoints (control)
	api.HandleFunc("/notifications", s.handleCreateNotification).Methods("POST")
	api.HandleFunc("/notifications/{id}/archive", s.handleArchiveNotification).Methods("POST")
	api.HandleFunc("/notifications/{id}/unarchive", s.handleUnarchiveNotification).Methods("POST")
	api.HandleFunc("/notifications/{id}", s.handleDeleteNotification).Methods("DELETE")
	api.HandleFunc("/notifications/archive/all", s.handleArchiveAllNotifications).Methods("POST")

	// Unassigned Devices endpoints (monitoring)
	api.HandleFunc("/unassigned", s.handleUnassignedDevices).Methods("GET")
	api.HandleFunc("/unassigned/devices", s.handleUnassignedDevicesList).Methods("GET")
	api.HandleFunc("/unassigned/remote-shares", s.handleUnassignedRemoteShares).Methods("GET")

	// Collectors management endpoints
	api.HandleFunc("/collectors/status", s.handleCollectorsStatus).Methods("GET")
	api.HandleFunc("/collectors/{name}/enable", s.handleCollectorEnable).Methods("POST")
	api.HandleFunc("/collectors/{name}/disable", s.handleCollectorDisable).Methods("POST")
	api.HandleFunc("/collectors/{name}/interval", s.handleCollectorInterval).Methods("PATCH")
	api.HandleFunc("/collectors/{name}", s.handleCollectorStatus).Methods("GET")

	// MQTT endpoints
	api.HandleFunc("/mqtt/status", s.handleMQTTStatus).Methods("GET")
	api.HandleFunc("/mqtt/test", s.handleMQTTTest).Methods("POST")
	api.HandleFunc("/mqtt/publish", s.handleMQTTPublish).Methods("POST")

	// WebSocket endpoint
	api.HandleFunc("/ws", s.handleWebSocket)
}

// StartSubscriptions initializes event subscriptions and WebSocket hub
// This should be called before collectors start to avoid race conditions
func (s *Server) StartSubscriptions() {
	logger.Info("Starting API server subscriptions...")

	// Start WebSocket hub
	go s.wsHub.Run(s.cancelCtx)

	// Subscribe to events and update cache
	go s.subscribeToEvents(s.cancelCtx)

	// Broadcast events to WebSocket clients
	go s.broadcastEvents(s.cancelCtx)

	logger.Info("API server subscriptions started")
}

// StartHTTP starts the HTTP server
func (s *Server) StartHTTP() error {
	s.httpServer = &http.Server{
		Addr:         fmt.Sprintf(":%d", s.ctx.Port),
		Handler:      s.router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	logger.Info("HTTP server listening on %s", s.httpServer.Addr)
	return s.httpServer.ListenAndServe()
}

// Start starts both subscriptions and HTTP server (legacy method)
func (s *Server) Start() error {
	s.StartSubscriptions()
	return s.StartHTTP()
}

// Stop gracefully shuts down the API server.
// It cancels all background goroutines and shuts down the HTTP server with a 5-second timeout.
func (s *Server) Stop() {
	// Cancel all background goroutines
	s.cancelFunc()

	// Shutdown HTTP server with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := s.httpServer.Shutdown(ctx); err != nil {
		logger.Error("Server shutdown error: %v", err)
	}
}

func (s *Server) subscribeToEvents(ctx context.Context) {
	// Subscribe to specific events to update cache
	logger.Info("Cache: Subscribing to event topics...")
	ch := s.ctx.Hub.Sub(
		"system_update",
		"array_status_update",
		"disk_list_update",
		"share_list_update",
		"container_list_update",
		"vm_list_update",
		"ups_status_update",
		"nut_status_update",
		"gpu_metrics_update",
		"network_list_update",
		"hardware_update",
		"registration_update",
		"notifications_update",
		"unassigned_devices_update",
		"zfs_pools_update",
		"zfs_datasets_update",
		"zfs_snapshots_update",
		"zfs_arc_stats_update",
	)
	logger.Info("Cache: Subscription ready, waiting for events...")

	for {
		select {
		case <-ctx.Done():
			logger.Info("Cache subscription stopping due to context cancellation")
			s.ctx.Hub.Unsub(ch)
			return
		case msg := <-ch:
			// Update cache based on message type
			switch v := msg.(type) {
			case *dto.SystemInfo:
				s.cacheMutex.Lock()
				s.systemCache = v
				s.cacheMutex.Unlock()
				logger.Debug("Cache: Updated system info - CPU: %.1f%%, RAM: %.1f%%", v.CPUUsage, v.RAMUsage)
			case *dto.ArrayStatus:
				s.cacheMutex.Lock()
				s.arrayCache = v
				s.cacheMutex.Unlock()
				logger.Debug("Cache: Updated array status - state=%s, disks=%d", v.State, v.NumDisks)
			case []dto.DiskInfo:
				s.cacheMutex.Lock()
				s.disksCache = v
				s.cacheMutex.Unlock()
				logger.Debug("Cache: Updated disk list - count=%d", len(v))
			case []dto.ShareInfo:
				s.cacheMutex.Lock()
				s.sharesCache = v
				s.cacheMutex.Unlock()
				logger.Debug("Cache: Updated share list - count=%d", len(v))
			case []*dto.ContainerInfo:
				// Convert pointer slice to value slice for cache
				containers := make([]dto.ContainerInfo, len(v))
				for i, c := range v {
					containers[i] = *c
				}
				s.cacheMutex.Lock()
				s.dockerCache = containers
				s.cacheMutex.Unlock()
				logger.Debug("Cache: Updated container list - count=%d", len(v))
			case []*dto.VMInfo:
				// Convert pointer slice to value slice for cache
				vms := make([]dto.VMInfo, len(v))
				for i, vm := range v {
					vms[i] = *vm
				}
				s.cacheMutex.Lock()
				s.vmsCache = vms
				s.cacheMutex.Unlock()
				logger.Debug("Cache: Updated VM list - count=%d", len(v))
			case *dto.UPSStatus:
				s.cacheMutex.Lock()
				s.upsCache = v
				s.cacheMutex.Unlock()
				logger.Debug("Cache: Updated UPS status - %s", v.Status)
			case *dto.NUTResponse:
				s.cacheMutex.Lock()
				s.nutCache = v
				s.cacheMutex.Unlock()
				logger.Debug("Cache: Updated NUT status - installed=%t, running=%t", v.Installed, v.Running)
			case []*dto.GPUMetrics:
				s.cacheMutex.Lock()
				s.gpuCache = v
				s.cacheMutex.Unlock()
				logger.Debug("Cache: Updated GPU metrics - count=%d", len(v))
			case []dto.NetworkInfo:
				s.cacheMutex.Lock()
				s.networkCache = v
				s.cacheMutex.Unlock()
				logger.Debug("Cache: Updated network list - count=%d", len(v))
			case *dto.HardwareInfo:
				s.cacheMutex.Lock()
				s.hardwareCache = v
				s.cacheMutex.Unlock()
				logger.Debug("Cache: Updated hardware info - BIOS: %s, Baseboard: %s",
					func() string {
						if v.BIOS != nil {
							return v.BIOS.Vendor
						}
						return "N/A"
					}(),
					func() string {
						if v.Baseboard != nil {
							return v.Baseboard.Manufacturer
						}
						return "N/A"
					}())
			case *dto.Registration:
				s.cacheMutex.Lock()
				s.registrationCache = v
				s.cacheMutex.Unlock()
				logger.Debug("Cache: Updated registration info - type=%s, state=%s", v.Type, v.State)
			case *dto.NotificationList:
				s.cacheMutex.Lock()
				s.notificationsCache = v
				s.cacheMutex.Unlock()
				logger.Debug("Cache: Updated notifications - unread=%d, archived=%d",
					v.Overview.Unread.Total, v.Overview.Archive.Total)
			case *dto.UnassignedDeviceList:
				s.cacheMutex.Lock()
				s.unassignedCache = v
				s.cacheMutex.Unlock()
				logger.Debug("Cache: Updated unassigned devices - devices=%d, remote_shares=%d",
					len(v.Devices), len(v.RemoteShares))
			case []dto.ZFSPool:
				s.cacheMutex.Lock()
				s.zfsPoolsCache = v
				s.cacheMutex.Unlock()
				logger.Debug("Cache: Updated ZFS pools - count=%d", len(v))
			case []dto.ZFSDataset:
				s.cacheMutex.Lock()
				s.zfsDatasetsCache = v
				s.cacheMutex.Unlock()
				logger.Debug("Cache: Updated ZFS datasets - count=%d", len(v))
			case []dto.ZFSSnapshot:
				s.cacheMutex.Lock()
				s.zfsSnapshotsCache = v
				s.cacheMutex.Unlock()
				logger.Debug("Cache: Updated ZFS snapshots - count=%d", len(v))
			case dto.ZFSARCStats:
				s.cacheMutex.Lock()
				s.zfsARCStatsCache = &v
				s.cacheMutex.Unlock()
				logger.Debug("Cache: Updated ZFS ARC stats - hit_ratio=%.2f%%", v.HitRatioPct)
			default:
				logger.Warning("Cache: Received unknown event type: %T", msg)
			}
		}
	}
}

func (s *Server) broadcastEvents(ctx context.Context) {
	// Subscribe to all event topics for WebSocket broadcasting
	ch := s.ctx.Hub.Sub(
		"system_update",
		"array_status_update",
		"disk_list_update",
		"share_list_update",
		"container_list_update",
		"vm_list_update",
		"ups_status_update",
		"nut_status_update",
		"gpu_metrics_update",
		"network_list_update",
		"hardware_update",
		"collector_state_change",
	)

	for {
		select {
		case <-ctx.Done():
			logger.Info("WebSocket broadcast stopping due to context cancellation")
			s.ctx.Hub.Unsub(ch)
			return
		case msg := <-ch:
			s.wsHub.Broadcast(msg)
		}
	}
}

// CacheProvider interface implementation for MCP integration.
// These methods provide read-only access to cached collector data.

// GetSystemCache returns cached system information.
func (s *Server) GetSystemCache() *dto.SystemInfo {
	s.cacheMutex.RLock()
	defer s.cacheMutex.RUnlock()
	return s.systemCache
}

// GetArrayCache returns cached array status.
func (s *Server) GetArrayCache() *dto.ArrayStatus {
	s.cacheMutex.RLock()
	defer s.cacheMutex.RUnlock()
	return s.arrayCache
}

// GetDisksCache returns cached disk information.
func (s *Server) GetDisksCache() []dto.DiskInfo {
	s.cacheMutex.RLock()
	defer s.cacheMutex.RUnlock()
	return s.disksCache
}

// GetSharesCache returns cached share information.
func (s *Server) GetSharesCache() []dto.ShareInfo {
	s.cacheMutex.RLock()
	defer s.cacheMutex.RUnlock()
	return s.sharesCache
}

// GetDockerCache returns cached Docker container information.
func (s *Server) GetDockerCache() []dto.ContainerInfo {
	s.cacheMutex.RLock()
	defer s.cacheMutex.RUnlock()
	return s.dockerCache
}

// GetVMsCache returns cached VM information.
func (s *Server) GetVMsCache() []dto.VMInfo {
	s.cacheMutex.RLock()
	defer s.cacheMutex.RUnlock()
	return s.vmsCache
}

// GetUPSCache returns cached UPS status.
func (s *Server) GetUPSCache() *dto.UPSStatus {
	s.cacheMutex.RLock()
	defer s.cacheMutex.RUnlock()
	return s.upsCache
}

// GetGPUCache returns cached GPU metrics.
func (s *Server) GetGPUCache() []*dto.GPUMetrics {
	s.cacheMutex.RLock()
	defer s.cacheMutex.RUnlock()
	return s.gpuCache
}

// GetNetworkCache returns cached network information.
func (s *Server) GetNetworkCache() []dto.NetworkInfo {
	s.cacheMutex.RLock()
	defer s.cacheMutex.RUnlock()
	return s.networkCache
}

// GetHardwareCache returns cached hardware information.
func (s *Server) GetHardwareCache() *dto.HardwareInfo {
	s.cacheMutex.RLock()
	defer s.cacheMutex.RUnlock()
	return s.hardwareCache
}

// GetRegistrationCache returns cached registration information.
func (s *Server) GetRegistrationCache() *dto.Registration {
	s.cacheMutex.RLock()
	defer s.cacheMutex.RUnlock()
	return s.registrationCache
}

// GetNotificationsCache returns cached notifications.
func (s *Server) GetNotificationsCache() *dto.NotificationList {
	s.cacheMutex.RLock()
	defer s.cacheMutex.RUnlock()
	return s.notificationsCache
}

// GetZFSPoolsCache returns cached ZFS pool information.
func (s *Server) GetZFSPoolsCache() []dto.ZFSPool {
	s.cacheMutex.RLock()
	defer s.cacheMutex.RUnlock()
	return s.zfsPoolsCache
}

// GetZFSDatasetsCache returns cached ZFS dataset information.
func (s *Server) GetZFSDatasetsCache() []dto.ZFSDataset {
	s.cacheMutex.RLock()
	defer s.cacheMutex.RUnlock()
	return s.zfsDatasetsCache
}

// GetZFSSnapshotsCache returns cached ZFS snapshot information.
func (s *Server) GetZFSSnapshotsCache() []dto.ZFSSnapshot {
	s.cacheMutex.RLock()
	defer s.cacheMutex.RUnlock()
	return s.zfsSnapshotsCache
}

// GetZFSARCStatsCache returns cached ZFS ARC statistics.
func (s *Server) GetZFSARCStatsCache() *dto.ZFSARCStats {
	s.cacheMutex.RLock()
	defer s.cacheMutex.RUnlock()
	return s.zfsARCStatsCache
}

// GetUnassignedCache returns cached unassigned devices information.
func (s *Server) GetUnassignedCache() *dto.UnassignedDeviceList {
	s.cacheMutex.RLock()
	defer s.cacheMutex.RUnlock()
	return s.unassignedCache
}

// GetNUTCache returns cached NUT (Network UPS Tools) information.
func (s *Server) GetNUTCache() *dto.NUTResponse {
	s.cacheMutex.RLock()
	defer s.cacheMutex.RUnlock()
	return s.nutCache
}

// GetParityHistoryCache returns cached parity check history.
// Note: This is dynamically loaded, not cached by a collector.
func (s *Server) GetParityHistoryCache() *dto.ParityCheckHistory {
	// Parity history is read from file on-demand, return nil for now
	// The MCP server will call the controller directly for this data
	return nil
}

// ListLogFiles returns a list of available log files.
func (s *Server) ListLogFiles() []dto.LogFile {
	return s.listLogFiles()
}

// GetLogContent retrieves log file content with optional pagination.
func (s *Server) GetLogContent(path, lines, start string) (*dto.LogFileContent, error) {
	return s.getLogContent(path, lines, start)
}

// GetCollectorsStatus returns the status of all collectors.
func (s *Server) GetCollectorsStatus() dto.CollectorsStatusResponse {
	if s.collectorManager == nil {
		return dto.CollectorsStatusResponse{}
	}
	return s.collectorManager.GetAllStatus()
}

// GetCollectorStatus returns the status of a specific collector.
func (s *Server) GetCollectorStatus(name string) (*dto.CollectorStatus, error) {
	if s.collectorManager == nil {
		return nil, fmt.Errorf("collector manager not available")
	}
	return s.collectorManager.GetStatus(name)
}

// EnableCollector enables a collector at runtime.
func (s *Server) EnableCollector(name string) error {
	if s.collectorManager == nil {
		return fmt.Errorf("collector manager not available")
	}
	return s.collectorManager.EnableCollector(name)
}

// DisableCollector disables a collector at runtime.
func (s *Server) DisableCollector(name string) error {
	if s.collectorManager == nil {
		return fmt.Errorf("collector manager not available")
	}
	return s.collectorManager.DisableCollector(name)
}

// UpdateCollectorInterval updates the collection interval for a collector.
func (s *Server) UpdateCollectorInterval(name string, interval int) error {
	if s.collectorManager == nil {
		return fmt.Errorf("collector manager not available")
	}
	return s.collectorManager.UpdateInterval(name, interval)
}

// GetRouter returns the HTTP router for external integration.
func (s *Server) GetRouter() *mux.Router {
	return s.router
}

// GetContext returns the domain context for external access.
func (s *Server) GetContext() *domain.Context {
	return s.ctx
}

// GetSystemSettings returns system settings from config collector.
func (s *Server) GetSystemSettings() *dto.SystemSettings {
	configCollector := collectors.NewConfigCollector()
	settings, err := configCollector.GetSystemSettings()
	if err != nil {
		logger.Error("Failed to get system settings: %v", err)
		return nil
	}
	return settings
}

// GetDockerSettings returns Docker daemon settings from config collector.
func (s *Server) GetDockerSettings() *dto.DockerSettings {
	configCollector := collectors.NewConfigCollector()
	settings, err := configCollector.GetDockerSettings()
	if err != nil {
		logger.Error("Failed to get Docker settings: %v", err)
		return nil
	}
	return settings
}

// GetVMSettings returns VM manager settings from config collector.
func (s *Server) GetVMSettings() *dto.VMSettings {
	configCollector := collectors.NewConfigCollector()
	settings, err := configCollector.GetVMSettings()
	if err != nil {
		logger.Error("Failed to get VM settings: %v", err)
		return nil
	}
	return settings
}

// GetDiskSettings returns disk settings from config collector.
func (s *Server) GetDiskSettings() *dto.DiskSettings {
	configCollector := collectors.NewConfigCollector()
	settings, err := configCollector.GetDiskSettings()
	if err != nil {
		logger.Error("Failed to get disk settings: %v", err)
		return nil
	}
	return settings
}

// GetShareConfig returns configuration for a specific share.
func (s *Server) GetShareConfig(name string) *dto.ShareConfig {
	configCollector := collectors.NewConfigCollector()
	config, err := configCollector.GetShareConfig(name)
	if err != nil {
		logger.Error("Failed to get share config for %s: %v", name, err)
		return nil
	}
	return config
}

// GetNetworkAccessURLs returns all network access URLs for the server.
func (s *Server) GetNetworkAccessURLs() *dto.NetworkAccessURLs {
	accessURLs := collectors.CollectNetworkAccessURLs()
	return accessURLs
}

// GetHealthStatus returns a map with system health metrics.
func (s *Server) GetHealthStatus() map[string]interface{} {
	health := make(map[string]interface{})

	// System health
	if sysInfo := s.GetSystemCache(); sysInfo != nil {
		health["cpu_usage"] = sysInfo.CPUUsage
		health["ram_usage"] = sysInfo.RAMUsage
		health["cpu_temp"] = sysInfo.CPUTemp
		health["uptime"] = sysInfo.Uptime
	}

	// Array health
	if arrayStatus := s.GetArrayCache(); arrayStatus != nil {
		health["array_state"] = arrayStatus.State
		health["parity_valid"] = arrayStatus.ParityValid
		health["array_used_percent"] = arrayStatus.UsedPercent
	}

	// Disk health summary
	disks := s.GetDisksCache()
	healthyDisks := 0
	warningDisks := 0
	for _, disk := range disks {
		if disk.Status == "PASSED" && disk.Temperature <= 50 {
			healthyDisks++
		} else {
			warningDisks++
		}
	}
	health["healthy_disks"] = healthyDisks
	health["warning_disks"] = warningDisks

	// Container health
	containers := s.GetDockerCache()
	runningContainers := 0
	for _, c := range containers {
		if c.State == "running" {
			runningContainers++
		}
	}
	health["running_containers"] = runningContainers
	health["total_containers"] = len(containers)

	// VM health
	vms := s.GetVMsCache()
	runningVMs := 0
	for _, vm := range vms {
		if vm.State == "running" {
			runningVMs++
		}
	}
	health["running_vms"] = runningVMs
	health["total_vms"] = len(vms)

	return health
}

// SetMQTTClient sets the MQTT client for API integration.
func (s *Server) SetMQTTClient(client MQTTClientInterface) {
	s.mqttClient = client
}
