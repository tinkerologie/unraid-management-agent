package api

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
)

// Server represents the HTTP API server that handles REST endpoints and WebSocket connections.
// It maintains an in-memory cache of data from collectors and broadcasts updates to WebSocket clients.
type Server struct {
	ctx        *domain.Context
	httpServer *http.Server
	router     *mux.Router
	wsHub      *WSHub
	cancelCtx  context.Context
	cancelFunc context.CancelFunc

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
}

// NewServer creates a new API server instance with the given context.
// It initializes the HTTP router, WebSocket hub, and sets up all API routes.
func NewServer(ctx *domain.Context) *Server {
	cancelCtx, cancelFunc := context.WithCancel(context.Background())
	s := &Server{
		ctx:        ctx,
		router:     mux.NewRouter(),
		wsHub:      NewWSHub(),
		cancelCtx:  cancelCtx,
		cancelFunc: cancelFunc,
	}

	s.setupRoutes()
	return s
}

func (s *Server) setupRoutes() {
	// Apply middleware
	s.router.Use(corsMiddleware)
	s.router.Use(loggingMiddleware)
	s.router.Use(recoveryMiddleware)

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
	api.HandleFunc("/gpu", s.handleGPU).Methods("GET")
	api.HandleFunc("/network", s.handleNetwork).Methods("GET")

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

	// Configuration endpoints (read-only)
	api.HandleFunc("/shares/{name}/config", s.handleShareConfig).Methods("GET")
	api.HandleFunc("/network/{interface}/config", s.handleNetworkConfig).Methods("GET")
	api.HandleFunc("/settings/system", s.handleSystemSettings).Methods("GET")
	api.HandleFunc("/settings/docker", s.handleDockerSettings).Methods("GET")
	api.HandleFunc("/settings/vm", s.handleVMSettings).Methods("GET")
	api.HandleFunc("/settings/disks", s.handleDiskSettings).Methods("GET")

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
		"gpu_metrics_update",
		"network_list_update",
		"hardware_update",
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
