// Package api provides HTTP REST API handlers and WebSocket functionality for the Unraid Management Agent.
package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/lib"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/services/collectors"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/services/controllers"
	"github.com/gorilla/mux"
)

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleSystem(w http.ResponseWriter, r *http.Request) {
	// Get latest system info from cache
	s.cacheMutex.RLock()
	info := s.systemCache
	s.cacheMutex.RUnlock()

	if info == nil {
		info = &dto.SystemInfo{
			Hostname:  "unknown",
			Version:   s.ctx.Version,
			Timestamp: time.Now(),
		}
	}

	respondJSON(w, http.StatusOK, info)
}

func (s *Server) handleArray(w http.ResponseWriter, r *http.Request) {
	// Get latest array status from cache
	s.cacheMutex.RLock()
	status := s.arrayCache
	s.cacheMutex.RUnlock()

	if status == nil {
		status = &dto.ArrayStatus{
			State:     "unknown",
			Timestamp: time.Now(),
		}
	}

	respondJSON(w, http.StatusOK, status)
}

func (s *Server) handleDisks(w http.ResponseWriter, r *http.Request) {
	// Get latest disk list from cache
	s.cacheMutex.RLock()
	disks := s.disksCache
	s.cacheMutex.RUnlock()

	if disks == nil {
		disks = []dto.DiskInfo{}
	}

	respondJSON(w, http.StatusOK, disks)
}

func (s *Server) handleDisk(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	diskID := vars["id"]
	logger.Debug("API: Getting disk info for %s", diskID)

	s.cacheMutex.RLock()
	disks := s.disksCache
	s.cacheMutex.RUnlock()

	// Find disk by ID
	for _, disk := range disks {
		if disk.ID == diskID || disk.Device == diskID || disk.Name == diskID {
			respondJSON(w, http.StatusOK, disk)
			return
		}
	}

	// Disk not found
	respondJSON(w, http.StatusNotFound, dto.Response{
		Success:   false,
		Message:   fmt.Sprintf("Disk not found: %s", diskID),
		Timestamp: time.Now(),
	})
}

func (s *Server) handleShares(w http.ResponseWriter, r *http.Request) {
	// Get latest share list from cache
	s.cacheMutex.RLock()
	shares := s.sharesCache
	s.cacheMutex.RUnlock()

	if shares == nil {
		shares = []dto.ShareInfo{}
	}

	respondJSON(w, http.StatusOK, shares)
}

func (s *Server) handleDockerList(w http.ResponseWriter, r *http.Request) {
	// Get latest container list from cache
	s.cacheMutex.RLock()
	containers := s.dockerCache
	s.cacheMutex.RUnlock()

	if containers == nil {
		containers = []dto.ContainerInfo{}
	}

	respondJSON(w, http.StatusOK, containers)
}

func (s *Server) handleDockerInfo(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	containerID := vars["id"]
	logger.Debug("API: Getting container info for %s", containerID)

	s.cacheMutex.RLock()
	containers := s.dockerCache
	s.cacheMutex.RUnlock()

	// Find container by ID or name
	for _, container := range containers {
		if container.ID == containerID || container.Name == containerID {
			respondJSON(w, http.StatusOK, container)
			return
		}
	}

	// Container not found
	respondJSON(w, http.StatusNotFound, dto.Response{
		Success:   false,
		Message:   fmt.Sprintf("Container not found: %s", containerID),
		Timestamp: time.Now(),
	})
}

func (s *Server) handleVMList(w http.ResponseWriter, r *http.Request) {
	// Get latest VM list from cache
	s.cacheMutex.RLock()
	vms := s.vmsCache
	s.cacheMutex.RUnlock()

	if vms == nil {
		vms = []dto.VMInfo{}
	}

	respondJSON(w, http.StatusOK, vms)
}

func (s *Server) handleVMInfo(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	vmID := vars["id"]
	logger.Debug("API: Getting VM info for %s", vmID)

	s.cacheMutex.RLock()
	vms := s.vmsCache
	s.cacheMutex.RUnlock()

	// Find VM by ID or name
	for _, vm := range vms {
		if vm.ID == vmID || vm.Name == vmID {
			respondJSON(w, http.StatusOK, vm)
			return
		}
	}

	// VM not found
	respondJSON(w, http.StatusNotFound, dto.Response{
		Success:   false,
		Message:   fmt.Sprintf("VM not found: %s", vmID),
		Timestamp: time.Now(),
	})
}

func (s *Server) handleUPS(w http.ResponseWriter, r *http.Request) {
	// Get latest UPS status from cache
	s.cacheMutex.RLock()
	ups := s.upsCache
	s.cacheMutex.RUnlock()

	if ups == nil {
		ups = &dto.UPSStatus{
			Connected: false,
			Timestamp: time.Now(),
		}
	}

	respondJSON(w, http.StatusOK, ups)
}

func (s *Server) handleGPU(w http.ResponseWriter, r *http.Request) {
	// Get latest GPU metrics from cache
	s.cacheMutex.RLock()
	gpus := s.gpuCache
	s.cacheMutex.RUnlock()

	if gpus == nil {
		gpus = []*dto.GPUMetrics{}
	}

	respondJSON(w, http.StatusOK, gpus)
}

func (s *Server) handleNetwork(w http.ResponseWriter, r *http.Request) {
	// Get latest network interfaces from cache
	s.cacheMutex.RLock()
	interfaces := s.networkCache
	s.cacheMutex.RUnlock()

	if interfaces == nil {
		interfaces = []dto.NetworkInfo{}
	}

	respondJSON(w, http.StatusOK, interfaces)
}

// Generic Docker operation handler to reduce code duplication
//
//nolint:dupl // Similar to handleVMOperation but serves different purpose (Docker vs VM)
func (s *Server) handleDockerOperation(w http.ResponseWriter, r *http.Request, operation string, operationFunc func(string) error) {
	vars := mux.Vars(r)
	containerID := vars["id"]

	// Validate container ID format
	if err := lib.ValidateContainerID(containerID); err != nil {
		logger.Warning("Invalid container ID for %s operation: %s - %v", operation, containerID, err)
		respondJSON(w, http.StatusBadRequest, dto.Response{
			Success:   false,
			Message:   err.Error(),
			Timestamp: time.Now(),
		})
		return
	}

	logger.Info("%s container %s", operation, containerID)

	if err := operationFunc(containerID); err != nil {
		logger.Error("Failed to %s container %s: %v", operation, containerID, err)
		respondJSON(w, http.StatusInternalServerError, dto.Response{
			Success:   false,
			Message:   fmt.Sprintf("Failed to %s container: %v", operation, err),
			Timestamp: time.Now(),
		})
		return
	}

	respondJSON(w, http.StatusOK, dto.Response{
		Success:   true,
		Message:   fmt.Sprintf("Container %s", operation),
		Timestamp: time.Now(),
	})
}

// Generic VM operation handler to reduce code duplication
//
//nolint:dupl // Similar to handleDockerOperation but serves different purpose (VM vs Docker)
func (s *Server) handleVMOperation(w http.ResponseWriter, r *http.Request, operation string, operationFunc func(string) error) {
	vars := mux.Vars(r)
	vmName := vars["name"]

	// Validate VM name format
	if err := lib.ValidateVMName(vmName); err != nil {
		logger.Warning("Invalid VM name for %s operation: %s - %v", operation, vmName, err)
		respondJSON(w, http.StatusBadRequest, dto.Response{
			Success:   false,
			Message:   err.Error(),
			Timestamp: time.Now(),
		})
		return
	}

	logger.Info("%s VM %s", operation, vmName)

	if err := operationFunc(vmName); err != nil {
		logger.Error("Failed to %s VM %s: %v", operation, vmName, err)
		respondJSON(w, http.StatusInternalServerError, dto.Response{
			Success:   false,
			Message:   fmt.Sprintf("Failed to %s VM: %v", operation, err),
			Timestamp: time.Now(),
		})
		return
	}

	respondJSON(w, http.StatusOK, dto.Response{
		Success:   true,
		Message:   fmt.Sprintf("VM %s", operation),
		Timestamp: time.Now(),
	})
}

// Docker control handlers
func (s *Server) handleDockerStart(w http.ResponseWriter, r *http.Request) {
	controller := controllers.NewDockerController()
	s.handleDockerOperation(w, r, "started", controller.Start)
}

func (s *Server) handleDockerStop(w http.ResponseWriter, r *http.Request) {
	controller := controllers.NewDockerController()
	s.handleDockerOperation(w, r, "stopped", controller.Stop)
}

func (s *Server) handleDockerRestart(w http.ResponseWriter, r *http.Request) {
	controller := controllers.NewDockerController()
	s.handleDockerOperation(w, r, "restarted", controller.Restart)
}

func (s *Server) handleDockerPause(w http.ResponseWriter, r *http.Request) {
	controller := controllers.NewDockerController()
	s.handleDockerOperation(w, r, "paused", controller.Pause)
}

func (s *Server) handleDockerUnpause(w http.ResponseWriter, r *http.Request) {
	controller := controllers.NewDockerController()
	s.handleDockerOperation(w, r, "unpaused", controller.Unpause)
}

// VM control handlers
func (s *Server) handleVMStart(w http.ResponseWriter, r *http.Request) {
	controller := controllers.NewVMController()
	s.handleVMOperation(w, r, "started", controller.Start)
}

func (s *Server) handleVMStop(w http.ResponseWriter, r *http.Request) {
	controller := controllers.NewVMController()
	s.handleVMOperation(w, r, "stopped", controller.Stop)
}

func (s *Server) handleVMRestart(w http.ResponseWriter, r *http.Request) {
	controller := controllers.NewVMController()
	s.handleVMOperation(w, r, "restarted", controller.Restart)
}

func (s *Server) handleVMPause(w http.ResponseWriter, r *http.Request) {
	controller := controllers.NewVMController()
	s.handleVMOperation(w, r, "paused", controller.Pause)
}

func (s *Server) handleVMResume(w http.ResponseWriter, r *http.Request) {
	controller := controllers.NewVMController()
	s.handleVMOperation(w, r, "resumed", controller.Resume)
}

func (s *Server) handleVMHibernate(w http.ResponseWriter, r *http.Request) {
	controller := controllers.NewVMController()
	s.handleVMOperation(w, r, "hibernated", controller.Hibernate)
}

func (s *Server) handleVMForceStop(w http.ResponseWriter, r *http.Request) {
	controller := controllers.NewVMController()
	s.handleVMOperation(w, r, "force stopped", controller.ForceStop)
}

// Array control handlers
func (s *Server) handleArrayStart(w http.ResponseWriter, r *http.Request) {
	logger.Info("API: Starting array")

	arrayCtrl := controllers.NewArrayController(s.ctx)
	err := arrayCtrl.StartArray()

	if err != nil {
		logger.Error("API: Failed to start array: %v", err)
		respondJSON(w, http.StatusInternalServerError, dto.Response{
			Success:   false,
			Message:   fmt.Sprintf("Failed to start array: %v", err),
			Timestamp: time.Now(),
		})
		return
	}

	respondJSON(w, http.StatusOK, dto.Response{
		Success:   true,
		Message:   "Array started successfully",
		Timestamp: time.Now(),
	})
}

func (s *Server) handleArrayStop(w http.ResponseWriter, r *http.Request) {
	logger.Info("API: Stopping array")

	arrayCtrl := controllers.NewArrayController(s.ctx)
	err := arrayCtrl.StopArray()

	if err != nil {
		logger.Error("API: Failed to stop array: %v", err)
		respondJSON(w, http.StatusInternalServerError, dto.Response{
			Success:   false,
			Message:   fmt.Sprintf("Failed to stop array: %v", err),
			Timestamp: time.Now(),
		})
		return
	}

	respondJSON(w, http.StatusOK, dto.Response{
		Success:   true,
		Message:   "Array stopped successfully",
		Timestamp: time.Now(),
	})
}

func (s *Server) handleParityCheckStart(w http.ResponseWriter, r *http.Request) {
	// Read optional 'correcting' parameter from query
	correcting := r.URL.Query().Get("correcting") == "true"
	logger.Info("API: Starting parity check (correcting: %v)", correcting)

	arrayCtrl := controllers.NewArrayController(s.ctx)
	err := arrayCtrl.StartParityCheck(correcting)

	if err != nil {
		logger.Error("API: Failed to start parity check: %v", err)
		respondJSON(w, http.StatusInternalServerError, dto.Response{
			Success:   false,
			Message:   fmt.Sprintf("Failed to start parity check: %v", err),
			Timestamp: time.Now(),
		})
		return
	}

	respondJSON(w, http.StatusOK, dto.Response{
		Success:   true,
		Message:   "Parity check started successfully",
		Timestamp: time.Now(),
	})
}

func (s *Server) handleParityCheckStop(w http.ResponseWriter, r *http.Request) {
	logger.Info("API: Stopping parity check")

	arrayCtrl := controllers.NewArrayController(s.ctx)
	err := arrayCtrl.StopParityCheck()

	if err != nil {
		logger.Error("API: Failed to stop parity check: %v", err)
		respondJSON(w, http.StatusInternalServerError, dto.Response{
			Success:   false,
			Message:   fmt.Sprintf("Failed to stop parity check: %v", err),
			Timestamp: time.Now(),
		})
		return
	}

	respondJSON(w, http.StatusOK, dto.Response{
		Success:   true,
		Message:   "Parity check stopped successfully",
		Timestamp: time.Now(),
	})
}

func (s *Server) handleParityCheckPause(w http.ResponseWriter, r *http.Request) {
	logger.Info("API: Pausing parity check")

	arrayCtrl := controllers.NewArrayController(s.ctx)
	err := arrayCtrl.PauseParityCheck()

	if err != nil {
		logger.Error("API: Failed to pause parity check: %v", err)
		respondJSON(w, http.StatusInternalServerError, dto.Response{
			Success:   false,
			Message:   fmt.Sprintf("Failed to pause parity check: %v", err),
			Timestamp: time.Now(),
		})
		return
	}

	respondJSON(w, http.StatusOK, dto.Response{
		Success:   true,
		Message:   "Parity check paused successfully",
		Timestamp: time.Now(),
	})
}

func (s *Server) handleParityCheckResume(w http.ResponseWriter, r *http.Request) {
	logger.Info("API: Resuming parity check")

	arrayCtrl := controllers.NewArrayController(s.ctx)
	err := arrayCtrl.ResumeParityCheck()

	if err != nil {
		logger.Error("API: Failed to resume parity check: %v", err)
		respondJSON(w, http.StatusInternalServerError, dto.Response{
			Success:   false,
			Message:   fmt.Sprintf("Failed to resume parity check: %v", err),
			Timestamp: time.Now(),
		})
		return
	}

	respondJSON(w, http.StatusOK, dto.Response{
		Success:   true,
		Message:   "Parity check resumed successfully",
		Timestamp: time.Now(),
	})
}

func (s *Server) handleParityCheckHistory(w http.ResponseWriter, r *http.Request) {
	logger.Debug("API: Getting parity check history")

	parityCollector := collectors.NewParityCollector()
	history, err := parityCollector.GetParityHistory()

	if err != nil {
		logger.Error("API: Failed to get parity check history: %v", err)
		respondJSON(w, http.StatusInternalServerError, dto.Response{
			Success:   false,
			Message:   fmt.Sprintf("Failed to get parity check history: %v", err),
			Timestamp: time.Now(),
		})
		return
	}

	respondJSON(w, http.StatusOK, history)
}

// Configuration handlers
func (s *Server) handleShareConfig(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	shareName := vars["name"]
	logger.Debug("API: Getting share config for %s", shareName)

	configCollector := collectors.NewConfigCollector()
	config, err := configCollector.GetShareConfig(shareName)

	if err != nil {
		logger.Error("API: Failed to get share config: %v", err)
		respondJSON(w, http.StatusNotFound, dto.Response{
			Success:   false,
			Message:   fmt.Sprintf("Failed to get share config: %v", err),
			Timestamp: time.Now(),
		})
		return
	}

	respondJSON(w, http.StatusOK, config)
}

func (s *Server) handleNetworkConfig(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	interfaceName := vars["interface"]
	logger.Debug("API: Getting network config for %s", interfaceName)

	configCollector := collectors.NewConfigCollector()
	config, err := configCollector.GetNetworkConfig(interfaceName)

	if err != nil {
		logger.Error("API: Failed to get network config: %v", err)
		respondJSON(w, http.StatusNotFound, dto.Response{
			Success:   false,
			Message:   fmt.Sprintf("Failed to get network config: %v", err),
			Timestamp: time.Now(),
		})
		return
	}

	respondJSON(w, http.StatusOK, config)
}

func (s *Server) handleSystemSettings(w http.ResponseWriter, r *http.Request) {
	logger.Debug("API: Getting system settings")

	configCollector := collectors.NewConfigCollector()
	settings, err := configCollector.GetSystemSettings()

	if err != nil {
		logger.Error("API: Failed to get system settings: %v", err)
		respondJSON(w, http.StatusInternalServerError, dto.Response{
			Success:   false,
			Message:   fmt.Sprintf("Failed to get system settings: %v", err),
			Timestamp: time.Now(),
		})
		return
	}

	respondJSON(w, http.StatusOK, settings)
}

func (s *Server) handleDockerSettings(w http.ResponseWriter, r *http.Request) {
	logger.Debug("API: Getting Docker settings")

	configCollector := collectors.NewConfigCollector()
	settings, err := configCollector.GetDockerSettings()

	if err != nil {
		logger.Error("API: Failed to get Docker settings: %v", err)
		respondJSON(w, http.StatusInternalServerError, dto.Response{
			Success:   false,
			Message:   fmt.Sprintf("Failed to get Docker settings: %v", err),
			Timestamp: time.Now(),
		})
		return
	}

	respondJSON(w, http.StatusOK, settings)
}

func (s *Server) handleVMSettings(w http.ResponseWriter, r *http.Request) {
	logger.Debug("API: Getting VM settings")

	configCollector := collectors.NewConfigCollector()
	settings, err := configCollector.GetVMSettings()

	if err != nil {
		logger.Error("API: Failed to get VM settings: %v", err)
		respondJSON(w, http.StatusInternalServerError, dto.Response{
			Success:   false,
			Message:   fmt.Sprintf("Failed to get VM settings: %v", err),
			Timestamp: time.Now(),
		})
		return
	}

	respondJSON(w, http.StatusOK, settings)
}

func (s *Server) handleDiskSettings(w http.ResponseWriter, r *http.Request) {
	logger.Debug("API: Getting disk settings")

	configCollector := collectors.NewConfigCollector()
	settings, err := configCollector.GetDiskSettings()

	if err != nil {
		logger.Error("API: Failed to get disk settings: %v", err)
		respondJSON(w, http.StatusInternalServerError, dto.Response{
			Success:   false,
			Message:   fmt.Sprintf("Failed to get disk settings: %v", err),
			Timestamp: time.Now(),
		})
		return
	}

	respondJSON(w, http.StatusOK, settings)
}

func (s *Server) handleUpdateShareConfig(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	shareName := vars["name"]
	logger.Info("API: Updating share config for %s", shareName)

	var config dto.ShareConfig
	if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
		respondJSON(w, http.StatusBadRequest, dto.Response{
			Success:   false,
			Message:   fmt.Sprintf("Invalid request body: %v", err),
			Timestamp: time.Now(),
		})
		return
	}

	// Ensure name matches URL parameter
	config.Name = shareName

	configCollector := collectors.NewConfigCollector()
	if err := configCollector.UpdateShareConfig(&config); err != nil {
		logger.Error("API: Failed to update share config: %v", err)
		respondJSON(w, http.StatusInternalServerError, dto.Response{
			Success:   false,
			Message:   fmt.Sprintf("Failed to update share config: %v", err),
			Timestamp: time.Now(),
		})
		return
	}

	respondJSON(w, http.StatusOK, dto.Response{
		Success:   true,
		Message:   "Share config updated successfully",
		Timestamp: time.Now(),
	})
}

func (s *Server) handleUpdateSystemSettings(w http.ResponseWriter, r *http.Request) {
	logger.Info("API: Updating system settings")

	var settings dto.SystemSettings
	if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
		respondJSON(w, http.StatusBadRequest, dto.Response{
			Success:   false,
			Message:   fmt.Sprintf("Invalid request body: %v", err),
			Timestamp: time.Now(),
		})
		return
	}

	configCollector := collectors.NewConfigCollector()
	if err := configCollector.UpdateSystemSettings(&settings); err != nil {
		logger.Error("API: Failed to update system settings: %v", err)
		respondJSON(w, http.StatusInternalServerError, dto.Response{
			Success:   false,
			Message:   fmt.Sprintf("Failed to update system settings: %v", err),
			Timestamp: time.Now(),
		})
		return
	}

	respondJSON(w, http.StatusOK, dto.Response{
		Success:   true,
		Message:   "System settings updated successfully",
		Timestamp: time.Now(),
	})
}

// Helper function to respond with JSON
func respondJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		logger.Error("Failed to encode JSON response: %v", err)
	}
}
