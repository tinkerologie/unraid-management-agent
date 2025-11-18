// Package api provides HTTP REST API handlers and WebSocket functionality for the Unraid Management Agent.
package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/lib"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/services/collectors"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/services/controllers"
)

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleSystem(w http.ResponseWriter, _ *http.Request) {
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

func (s *Server) handleArray(w http.ResponseWriter, _ *http.Request) {
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

func (s *Server) handleDisks(w http.ResponseWriter, _ *http.Request) {
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

func (s *Server) handleShares(w http.ResponseWriter, _ *http.Request) {
	// Get latest share list from cache
	s.cacheMutex.RLock()
	shares := s.sharesCache
	s.cacheMutex.RUnlock()

	if shares == nil {
		shares = []dto.ShareInfo{}
	}

	respondJSON(w, http.StatusOK, shares)
}

func (s *Server) handleDockerList(w http.ResponseWriter, _ *http.Request) {
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

func (s *Server) handleVMList(w http.ResponseWriter, _ *http.Request) {
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

func (s *Server) handleUPS(w http.ResponseWriter, _ *http.Request) {
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

func (s *Server) handleGPU(w http.ResponseWriter, _ *http.Request) {
	// Get latest GPU metrics from cache
	s.cacheMutex.RLock()
	gpus := s.gpuCache
	s.cacheMutex.RUnlock()

	if gpus == nil {
		gpus = []*dto.GPUMetrics{}
	}

	respondJSON(w, http.StatusOK, gpus)
}

func (s *Server) handleNetwork(w http.ResponseWriter, _ *http.Request) {
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
func (s *Server) handleArrayStart(w http.ResponseWriter, _ *http.Request) {
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

func (s *Server) handleArrayStop(w http.ResponseWriter, _ *http.Request) {
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

func (s *Server) handleParityCheckStop(w http.ResponseWriter, _ *http.Request) {
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

func (s *Server) handleParityCheckPause(w http.ResponseWriter, _ *http.Request) {
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

func (s *Server) handleParityCheckResume(w http.ResponseWriter, _ *http.Request) {
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

func (s *Server) handleParityCheckHistory(w http.ResponseWriter, _ *http.Request) {
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

	// Validate share name to prevent path traversal attacks
	if err := lib.ValidateShareName(shareName); err != nil {
		logger.Error("API: Invalid share name: %v", err)
		respondJSON(w, http.StatusBadRequest, dto.Response{
			Success:   false,
			Message:   fmt.Sprintf("Invalid share name: %v", err),
			Timestamp: time.Now(),
		})
		return
	}

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

func (s *Server) handleSystemSettings(w http.ResponseWriter, _ *http.Request) {
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

func (s *Server) handleDockerSettings(w http.ResponseWriter, _ *http.Request) {
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

func (s *Server) handleVMSettings(w http.ResponseWriter, _ *http.Request) {
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

func (s *Server) handleDiskSettings(w http.ResponseWriter, _ *http.Request) {
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

	// Validate share name to prevent path traversal attacks
	if err := lib.ValidateShareName(shareName); err != nil {
		logger.Error("API: Invalid share name: %v", err)
		respondJSON(w, http.StatusBadRequest, dto.Response{
			Success:   false,
			Message:   fmt.Sprintf("Invalid share name: %v", err),
			Timestamp: time.Now(),
		})
		return
	}

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

// handleUserScripts returns a list of all available user scripts
func (s *Server) handleUserScripts(w http.ResponseWriter, _ *http.Request) {
	scripts, err := controllers.ListUserScripts()
	if err != nil {
		logger.Error("API: Failed to list user scripts: %v", err)
		respondJSON(w, http.StatusInternalServerError, dto.Response{
			Success:   false,
			Message:   fmt.Sprintf("Failed to list user scripts: %v", err),
			Timestamp: time.Now(),
		})
		return
	}

	respondJSON(w, http.StatusOK, scripts)
}

// handleUserScriptExecute executes a user script
func (s *Server) handleUserScriptExecute(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	scriptName := vars["name"]

	// Parse request body for execution options
	var req dto.UserScriptExecuteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		// Use defaults if no body provided
		req.Background = true
		req.Wait = false
	}

	// Execute the script
	response, err := controllers.ExecuteUserScript(scriptName, req.Background, req.Wait)
	if err != nil {
		logger.Error("API: Failed to execute user script %s: %v", scriptName, err)
		respondJSON(w, http.StatusInternalServerError, response)
		return
	}

	respondJSON(w, http.StatusOK, response)
}

// Hardware endpoints

func (s *Server) handleHardwareFull(w http.ResponseWriter, _ *http.Request) {
	s.cacheMutex.RLock()
	hardware := s.hardwareCache
	s.cacheMutex.RUnlock()

	if hardware == nil {
		hardware = &dto.HardwareInfo{
			Timestamp: time.Now(),
		}
	}

	respondJSON(w, http.StatusOK, hardware)
}

func (s *Server) handleHardwareBIOS(w http.ResponseWriter, _ *http.Request) {
	s.cacheMutex.RLock()
	hardware := s.hardwareCache
	s.cacheMutex.RUnlock()

	if hardware == nil || hardware.BIOS == nil {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "BIOS information not available"})
		return
	}

	respondJSON(w, http.StatusOK, hardware.BIOS)
}

func (s *Server) handleHardwareBaseboard(w http.ResponseWriter, _ *http.Request) {
	s.cacheMutex.RLock()
	hardware := s.hardwareCache
	s.cacheMutex.RUnlock()

	if hardware == nil || hardware.Baseboard == nil {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Baseboard information not available"})
		return
	}

	respondJSON(w, http.StatusOK, hardware.Baseboard)
}

func (s *Server) handleHardwareCPU(w http.ResponseWriter, _ *http.Request) {
	s.cacheMutex.RLock()
	hardware := s.hardwareCache
	s.cacheMutex.RUnlock()

	if hardware == nil || hardware.CPU == nil {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "CPU hardware information not available"})
		return
	}

	respondJSON(w, http.StatusOK, hardware.CPU)
}

func (s *Server) handleHardwareCache(w http.ResponseWriter, _ *http.Request) {
	s.cacheMutex.RLock()
	hardware := s.hardwareCache
	s.cacheMutex.RUnlock()

	if hardware == nil || len(hardware.Cache) == 0 {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "CPU cache information not available"})
		return
	}

	respondJSON(w, http.StatusOK, hardware.Cache)
}

func (s *Server) handleHardwareMemoryArray(w http.ResponseWriter, _ *http.Request) {
	s.cacheMutex.RLock()
	hardware := s.hardwareCache
	s.cacheMutex.RUnlock()

	if hardware == nil || hardware.MemoryArray == nil {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Memory array information not available"})
		return
	}

	respondJSON(w, http.StatusOK, hardware.MemoryArray)
}

func (s *Server) handleHardwareMemoryDevices(w http.ResponseWriter, _ *http.Request) {
	s.cacheMutex.RLock()
	hardware := s.hardwareCache
	s.cacheMutex.RUnlock()

	if hardware == nil || len(hardware.MemoryDevices) == 0 {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Memory device information not available"})
		return
	}

	respondJSON(w, http.StatusOK, hardware.MemoryDevices)
}

func (s *Server) handleRegistration(w http.ResponseWriter, _ *http.Request) {
	logger.Debug("API: Getting registration information")

	s.cacheMutex.RLock()
	registration := s.registrationCache
	s.cacheMutex.RUnlock()

	if registration == nil {
		registration = &dto.Registration{
			Type:      "unknown",
			State:     "invalid",
			Timestamp: time.Now(),
		}
	}

	respondJSON(w, http.StatusOK, registration)
}

func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	logger.Debug("API: Getting logs")

	// Get query parameters
	path := r.URL.Query().Get("path")
	linesParam := r.URL.Query().Get("lines")
	startParam := r.URL.Query().Get("start")

	// If no path specified, list all available logs
	if path == "" {
		logs := s.listLogFiles()
		respondJSON(w, http.StatusOK, map[string]interface{}{"logs": logs})
		return
	}

	// Get log content with optional pagination
	content, err := s.getLogContent(path, linesParam, startParam)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	respondJSON(w, http.StatusOK, content)
}

// Helper function to respond with JSON
func respondJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		logger.Error("Failed to encode JSON response: %v", err)
	}
}

// Helper function to respond with error
func respondWithError(w http.ResponseWriter, status int, message string) {
	respondJSON(w, status, map[string]string{"error": message})
}

// handleNotifications returns all notifications with overview
func (s *Server) handleNotifications(w http.ResponseWriter, r *http.Request) {
	s.cacheMutex.RLock()
	notificationList := s.notificationsCache
	s.cacheMutex.RUnlock()

	if notificationList == nil {
		notificationList = &dto.NotificationList{
			Overview: dto.NotificationOverview{
				Unread:  dto.NotificationCounts{},
				Archive: dto.NotificationCounts{},
			},
			Notifications: []dto.Notification{},
			Timestamp:     time.Now(),
		}
	}

	// Filter by importance if specified
	importance := r.URL.Query().Get("importance")
	if importance != "" {
		filtered := []dto.Notification{}
		for _, n := range notificationList.Notifications {
			if n.Importance == importance {
				filtered = append(filtered, n)
			}
		}
		notificationList.Notifications = filtered
	}

	respondJSON(w, http.StatusOK, notificationList)
}

// handleNotificationsUnread returns only unread notifications
func (s *Server) handleNotificationsUnread(w http.ResponseWriter, _ *http.Request) {
	s.cacheMutex.RLock()
	notificationList := s.notificationsCache
	s.cacheMutex.RUnlock()

	if notificationList == nil {
		respondJSON(w, http.StatusOK, map[string]interface{}{
			"notifications": []dto.Notification{},
			"count":         0,
		})
		return
	}

	unread := []dto.Notification{}
	for _, n := range notificationList.Notifications {
		if n.Type == "unread" {
			unread = append(unread, n)
		}
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"notifications": unread,
		"count":         len(unread),
	})
}

// handleNotificationsArchive returns only archived notifications
func (s *Server) handleNotificationsArchive(w http.ResponseWriter, _ *http.Request) {
	s.cacheMutex.RLock()
	notificationList := s.notificationsCache
	s.cacheMutex.RUnlock()

	if notificationList == nil {
		respondJSON(w, http.StatusOK, map[string]interface{}{
			"notifications": []dto.Notification{},
			"count":         0,
		})
		return
	}

	archived := []dto.Notification{}
	for _, n := range notificationList.Notifications {
		if n.Type == "archive" {
			archived = append(archived, n)
		}
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"notifications": archived,
		"count":         len(archived),
	})
}

// handleNotificationsOverview returns only the overview counts
func (s *Server) handleNotificationsOverview(w http.ResponseWriter, _ *http.Request) {
	s.cacheMutex.RLock()
	notificationList := s.notificationsCache
	s.cacheMutex.RUnlock()

	if notificationList == nil {
		respondJSON(w, http.StatusOK, dto.NotificationOverview{
			Unread:  dto.NotificationCounts{},
			Archive: dto.NotificationCounts{},
		})
		return
	}

	respondJSON(w, http.StatusOK, notificationList.Overview)
}

// handleNotificationByID returns a specific notification by ID
func (s *Server) handleNotificationByID(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	s.cacheMutex.RLock()
	notificationList := s.notificationsCache
	s.cacheMutex.RUnlock()

	if notificationList == nil {
		respondWithError(w, http.StatusNotFound, "Notification not found")
		return
	}

	for _, n := range notificationList.Notifications {
		if n.ID == id {
			respondJSON(w, http.StatusOK, n)
			return
		}
	}

	respondWithError(w, http.StatusNotFound, "Notification not found")
}

// handleCreateNotification creates a new notification
func (s *Server) handleCreateNotification(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Title       string `json:"title"`
		Subject     string `json:"subject"`
		Description string `json:"description"`
		Importance  string `json:"importance"`
		Link        string `json:"link"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.Title == "" {
		respondWithError(w, http.StatusBadRequest, "Title is required")
		return
	}

	if req.Importance == "" {
		req.Importance = "info"
	}

	if err := controllers.CreateNotification(req.Title, req.Subject, req.Description, req.Importance, req.Link); err != nil {
		respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusCreated, map[string]string{"message": "Notification created successfully"})
}

// handleArchiveNotification archives a specific notification
func (s *Server) handleArchiveNotification(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	if err := controllers.ArchiveNotification(id); err != nil {
		respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"message": "Notification archived successfully"})
}

// handleUnarchiveNotification unarchives a specific notification
func (s *Server) handleUnarchiveNotification(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	if err := controllers.UnarchiveNotification(id); err != nil {
		respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"message": "Notification unarchived successfully"})
}

// handleDeleteNotification deletes a specific notification
func (s *Server) handleDeleteNotification(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	// Check if notification is in archive
	isArchived := r.URL.Query().Get("archived") == "true"

	if err := controllers.DeleteNotification(id, isArchived); err != nil {
		respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"message": "Notification deleted successfully"})
}

// handleArchiveAllNotifications archives all unread notifications
func (s *Server) handleArchiveAllNotifications(w http.ResponseWriter, _ *http.Request) {
	if err := controllers.ArchiveAllNotifications(); err != nil {
		respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"message": "All notifications archived successfully"})
}

// handleUnassignedDevices returns all unassigned devices and remote shares
func (s *Server) handleUnassignedDevices(w http.ResponseWriter, _ *http.Request) {
	s.cacheMutex.RLock()
	defer s.cacheMutex.RUnlock()

	if s.unassignedCache == nil {
		respondJSON(w, http.StatusOK, map[string]interface{}{
			"devices":       []interface{}{},
			"remote_shares": []interface{}{},
			"timestamp":     time.Now(),
		})
		return
	}

	respondJSON(w, http.StatusOK, s.unassignedCache)
}

// handleUnassignedDevicesList returns only unassigned devices (no remote shares)
func (s *Server) handleUnassignedDevicesList(w http.ResponseWriter, _ *http.Request) {
	s.cacheMutex.RLock()
	defer s.cacheMutex.RUnlock()

	if s.unassignedCache == nil {
		respondJSON(w, http.StatusOK, map[string]interface{}{
			"devices":   []interface{}{},
			"timestamp": time.Now(),
		})
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"devices":   s.unassignedCache.Devices,
		"timestamp": s.unassignedCache.Timestamp,
	})
}

// handleUnassignedRemoteShares returns only remote shares (no devices)
func (s *Server) handleUnassignedRemoteShares(w http.ResponseWriter, _ *http.Request) {
	s.cacheMutex.RLock()
	defer s.cacheMutex.RUnlock()

	if s.unassignedCache == nil {
		respondJSON(w, http.StatusOK, map[string]interface{}{
			"remote_shares": []interface{}{},
			"timestamp":     time.Now(),
		})
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"remote_shares": s.unassignedCache.RemoteShares,
		"timestamp":     s.unassignedCache.Timestamp,
	})
}

// ============================================================================
// ZFS Handlers
// ============================================================================

// handleZFSPools returns all ZFS pools
func (s *Server) handleZFSPools(w http.ResponseWriter, _ *http.Request) {
	s.cacheMutex.RLock()
	pools := s.zfsPoolsCache
	s.cacheMutex.RUnlock()

	if pools == nil {
		pools = []dto.ZFSPool{}
	}

	respondJSON(w, http.StatusOK, pools)
}

// handleZFSPool returns a specific ZFS pool by name
func (s *Server) handleZFSPool(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	poolName := vars["name"]

	s.cacheMutex.RLock()
	pools := s.zfsPoolsCache
	s.cacheMutex.RUnlock()

	// Find pool by name
	for _, pool := range pools {
		if pool.Name == poolName {
			respondJSON(w, http.StatusOK, pool)
			return
		}
	}

	// Pool not found
	respondJSON(w, http.StatusNotFound, dto.Response{
		Success:   false,
		Message:   fmt.Sprintf("ZFS pool not found: %s", poolName),
		Timestamp: time.Now(),
	})
}

// handleZFSDatasets returns all ZFS datasets
func (s *Server) handleZFSDatasets(w http.ResponseWriter, _ *http.Request) {
	s.cacheMutex.RLock()
	datasets := s.zfsDatasetsCache
	s.cacheMutex.RUnlock()

	if datasets == nil {
		datasets = []dto.ZFSDataset{}
	}

	respondJSON(w, http.StatusOK, datasets)
}

// handleZFSSnapshots returns all ZFS snapshots
func (s *Server) handleZFSSnapshots(w http.ResponseWriter, _ *http.Request) {
	s.cacheMutex.RLock()
	snapshots := s.zfsSnapshotsCache
	s.cacheMutex.RUnlock()

	if snapshots == nil {
		snapshots = []dto.ZFSSnapshot{}
	}

	respondJSON(w, http.StatusOK, snapshots)
}

// handleZFSARC returns ZFS ARC statistics
func (s *Server) handleZFSARC(w http.ResponseWriter, _ *http.Request) {
	s.cacheMutex.RLock()
	arcStats := s.zfsARCStatsCache
	s.cacheMutex.RUnlock()

	if arcStats == nil {
		arcStats = &dto.ZFSARCStats{
			Timestamp: time.Now(),
		}
	}

	respondJSON(w, http.StatusOK, arcStats)
}
