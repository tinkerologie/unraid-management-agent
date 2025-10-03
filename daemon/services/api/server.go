package api

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/ruaandeysel/unraid-management-agent/daemon/domain"
	"github.com/ruaandeysel/unraid-management-agent/daemon/dto"
	"github.com/ruaandeysel/unraid-management-agent/daemon/logger"
)

type Server struct {
	ctx        *domain.Context
	httpServer *http.Server
	router     *mux.Router
	wsHub      *WSHub

	// Cache for latest data from collectors
	cacheMutex   sync.RWMutex
	systemCache  *dto.SystemInfo
	arrayCache   *dto.ArrayStatus
	disksCache   []dto.DiskInfo
	sharesCache  []dto.ShareInfo
	dockerCache  []dto.ContainerInfo
	vmsCache     []dto.VMInfo
	upsCache     *dto.UPSStatus
	gpuCache     []*dto.GPUMetrics
	networkCache []dto.NetworkInfo
}

func NewServer(ctx *domain.Context) *Server {
	s := &Server{
		ctx:    ctx,
		router: mux.NewRouter(),
		wsHub:  NewWSHub(),
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

	// Control endpoints
	api.HandleFunc("/docker/{id}/start", s.handleDockerStart).Methods("POST")
	api.HandleFunc("/docker/{id}/stop", s.handleDockerStop).Methods("POST")
	api.HandleFunc("/docker/{id}/restart", s.handleDockerRestart).Methods("POST")
	api.HandleFunc("/docker/{id}/pause", s.handleDockerPause).Methods("POST")
	api.HandleFunc("/docker/{id}/unpause", s.handleDockerUnpause).Methods("POST")

	api.HandleFunc("/vm/{id}/start", s.handleVMStart).Methods("POST")
	api.HandleFunc("/vm/{id}/stop", s.handleVMStop).Methods("POST")
	api.HandleFunc("/vm/{id}/restart", s.handleVMRestart).Methods("POST")
	api.HandleFunc("/vm/{id}/pause", s.handleVMPause).Methods("POST")
	api.HandleFunc("/vm/{id}/resume", s.handleVMResume).Methods("POST")
	api.HandleFunc("/vm/{id}/hibernate", s.handleVMHibernate).Methods("POST")
	api.HandleFunc("/vm/{id}/force-stop", s.handleVMForceStop).Methods("POST")

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

	// WebSocket endpoint
	api.HandleFunc("/ws", s.handleWebSocket)
}

// StartSubscriptions initializes event subscriptions and WebSocket hub
// This should be called before collectors start to avoid race conditions
func (s *Server) StartSubscriptions() {
	logger.Info("Starting API server subscriptions...")

	// Start WebSocket hub
	go s.wsHub.Run()

	// Subscribe to events and update cache
	go s.subscribeToEvents()

	// Broadcast events to WebSocket clients
	go s.broadcastEvents()

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

func (s *Server) Stop() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := s.httpServer.Shutdown(ctx); err != nil {
		logger.Error("Server shutdown error: %v", err)
	}
}

func (s *Server) subscribeToEvents() {
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
	)
	logger.Info("Cache: Subscription ready, waiting for events...")

	for msg := range ch {
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
		default:
			logger.Warning("Cache: Received unknown event type: %T", msg)
		}
	}
}

func (s *Server) broadcastEvents() {
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
	)

	for msg := range ch {
		s.wsHub.Broadcast(msg)
	}
}
