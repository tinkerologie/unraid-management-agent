// Package api provides HTTP REST API handlers and WebSocket functionality for the Unraid Management Agent.
package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/lib"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/services/collectors"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/services/controllers"
)

// handleHealth godoc
//
//	@Summary		Health check
//	@Description	Check if the API server is running and healthy
//	@Tags			System
//	@Produce		json
//	@Success		200	{object}	map[string]string	"Server is healthy"
//	@Router			/health [get]
func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleSystem godoc
//
//	@Summary		Get system information
//	@Description	Retrieve comprehensive system metrics including CPU, RAM, temperatures, and uptime
//	@Tags			System
//	@Produce		json
//	@Success		200	{object}	dto.SystemInfo	"System information"
//	@Router			/system [get]
func (s *Server) handleSystem(w http.ResponseWriter, _ *http.Request) {
	// Get latest system info from cache
	info := s.systemCache.Load()

	if info == nil {
		info = &dto.SystemInfo{
			Hostname:  "unknown",
			Version:   s.ctx.Version,
			Timestamp: time.Now(),
		}
	}

	respondJSON(w, http.StatusOK, info)
}

// handleSystemReboot godoc
//
//	@Summary		Reboot system
//	@Description	Initiate a system reboot
//	@Tags			System
//	@Produce		json
//	@Success		200	{object}	dto.Response	"Reboot initiated"
//	@Failure		500	{object}	dto.Response	"Failed to initiate reboot"
//	@Router			/system/reboot [post]
func (s *Server) handleSystemReboot(w http.ResponseWriter, _ *http.Request) {
	logger.Info("API: System reboot requested")

	systemCtrl := controllers.NewSystemController(s.ctx)
	err := systemCtrl.Reboot()

	if err != nil {
		logger.Error("API: Failed to initiate reboot: %v", err)
		respondJSON(w, http.StatusInternalServerError, dto.Response{
			Success:   false,
			Message:   fmt.Sprintf("Failed to initiate reboot: %v", err),
			Timestamp: time.Now(),
		})
		return
	}

	respondJSON(w, http.StatusOK, dto.Response{
		Success:   true,
		Message:   "Server reboot initiated",
		Timestamp: time.Now(),
	})
}

// handleSystemShutdown godoc
//
//	@Summary		Shutdown system
//	@Description	Initiate a system shutdown
//	@Tags			System
//	@Produce		json
//	@Success		200	{object}	dto.Response	"Shutdown initiated"
//	@Failure		500	{object}	dto.Response	"Failed to initiate shutdown"
//	@Router			/system/shutdown [post]
func (s *Server) handleSystemShutdown(w http.ResponseWriter, _ *http.Request) {
	logger.Info("API: System shutdown requested")

	systemCtrl := controllers.NewSystemController(s.ctx)
	err := systemCtrl.Shutdown()

	if err != nil {
		logger.Error("API: Failed to initiate shutdown: %v", err)
		respondJSON(w, http.StatusInternalServerError, dto.Response{
			Success:   false,
			Message:   fmt.Sprintf("Failed to initiate shutdown: %v", err),
			Timestamp: time.Now(),
		})
		return
	}

	respondJSON(w, http.StatusOK, dto.Response{
		Success:   true,
		Message:   "Server shutdown initiated",
		Timestamp: time.Now(),
	})
}

// handleArray godoc
//
//	@Summary		Get array status
//	@Description	Retrieve Unraid array status including parity information
//	@Tags			Array
//	@Produce		json
//	@Success		200	{object}	dto.ArrayStatus	"Array status"
//	@Router			/array [get]
func (s *Server) handleArray(w http.ResponseWriter, _ *http.Request) {
	// Get latest array status from cache
	status := s.arrayCache.Load()

	if status == nil {
		status = &dto.ArrayStatus{
			State:     "unknown",
			Timestamp: time.Now(),
		}
	}

	respondJSON(w, http.StatusOK, status)
}

// handleDisks godoc
//
//	@Summary		Get all disks
//	@Description	Retrieve information about all disks including SMART data
//	@Tags			Disks
//	@Produce		json
//	@Success		200	{array}	dto.DiskInfo	"List of disks"
//	@Router			/disks [get]
func (s *Server) handleDisks(w http.ResponseWriter, _ *http.Request) {
	// Get latest disk list from cache
	disks := s.GetDisksCache()

	if disks == nil {
		disks = []dto.DiskInfo{}
	}

	respondJSON(w, http.StatusOK, disks)
}

// handleDisk godoc
//
//	@Summary		Get specific disk
//	@Description	Retrieve information about a specific disk by ID, device name, or name
//	@Tags			Disks
//	@Produce		json
//	@Param			id	path		string	true	"Disk ID, device name (e.g., sda), or disk name (e.g., disk1)"
//	@Success		200	{object}	dto.DiskInfo	"Disk information"
//	@Failure		404	{object}	dto.Response	"Disk not found"
//	@Router			/disks/{id} [get]
func (s *Server) handleDisk(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	diskID := vars["id"]
	logger.Debug("API: Getting disk info for %s", diskID)

	disks := s.GetDisksCache()

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

// handleShares godoc
//
//	@Summary		Get all shares
//	@Description	Retrieve information about all user shares
//	@Tags			Shares
//	@Produce		json
//	@Success		200	{array}	dto.ShareInfo	"List of shares"
//	@Router			/shares [get]
func (s *Server) handleShares(w http.ResponseWriter, _ *http.Request) {
	// Get latest share list from cache
	shares := s.GetSharesCache()

	if shares == nil {
		shares = []dto.ShareInfo{}
	}

	respondJSON(w, http.StatusOK, shares)
}

// handleDockerList godoc
//
//	@Summary		Get all Docker containers
//	@Description	Retrieve information about all Docker containers including stats
//	@Tags			Docker
//	@Produce		json
//	@Success		200	{array}	dto.ContainerInfo	"List of containers"
//	@Router			/docker [get]
func (s *Server) handleDockerList(w http.ResponseWriter, _ *http.Request) {
	// Get latest container list from cache
	containers := s.GetDockerCache()

	if containers == nil {
		containers = []dto.ContainerInfo{}
	}

	respondJSON(w, http.StatusOK, containers)
}

// handleDockerInfo godoc
//
//	@Summary		Get specific Docker container
//	@Description	Retrieve information about a specific Docker container by ID or name
//	@Tags			Docker
//	@Produce		json
//	@Param			id	path		string	true	"Container ID or name"
//	@Success		200	{object}	dto.ContainerInfo	"Container information"
//	@Failure		404	{object}	dto.Response		"Container not found"
//	@Router			/docker/{id} [get]
func (s *Server) handleDockerInfo(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	containerID := vars["id"]
	logger.Debug("API: Getting container info for %s", containerID)

	containers := s.GetDockerCache()

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

// handleVMList godoc
//
//	@Summary		Get all VMs
//	@Description	Retrieve information about all virtual machines
//	@Tags			VMs
//	@Produce		json
//	@Success		200	{array}	dto.VMInfo	"List of VMs"
//	@Router			/vm [get]
func (s *Server) handleVMList(w http.ResponseWriter, _ *http.Request) {
	// Get latest VM list from cache
	vms := s.GetVMsCache()

	if vms == nil {
		vms = []dto.VMInfo{}
	}

	respondJSON(w, http.StatusOK, vms)
}

// handleVMInfo godoc
//
//	@Summary		Get specific VM
//	@Description	Retrieve information about a specific virtual machine by ID or name
//	@Tags			VMs
//	@Produce		json
//	@Param			id	path		string	true	"VM ID or name"
//	@Success		200	{object}	dto.VMInfo		"VM information"
//	@Failure		404	{object}	dto.Response	"VM not found"
//	@Router			/vm/{id} [get]
func (s *Server) handleVMInfo(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	vmID := vars["id"]
	logger.Debug("API: Getting VM info for %s", vmID)

	vms := s.GetVMsCache()

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

// handleUPS godoc
//
//	@Summary		Get UPS status
//	@Description	Retrieve UPS status information from apcupsd
//	@Tags			UPS
//	@Produce		json
//	@Success		200	{object}	dto.UPSStatus	"UPS status"
//	@Router			/ups [get]
func (s *Server) handleUPS(w http.ResponseWriter, _ *http.Request) {
	// Get latest UPS status from cache
	ups := s.upsCache.Load()

	if ups == nil {
		ups = &dto.UPSStatus{
			Connected: false,
			Timestamp: time.Now(),
		}
	}

	respondJSON(w, http.StatusOK, ups)
}

// handleNUT godoc
//
//	@Summary		Get NUT status
//	@Description	Retrieve Network UPS Tools (NUT) status information
//	@Tags			UPS
//	@Produce		json
//	@Success		200	{object}	dto.NUTResponse	"NUT status"
//	@Router			/nut [get]
func (s *Server) handleNUT(w http.ResponseWriter, _ *http.Request) {
	// Get latest NUT status from cache
	nut := s.nutCache.Load()

	if nut == nil {
		nut = &dto.NUTResponse{
			Installed: false,
			Running:   false,
			Timestamp: time.Now(),
		}
	}

	respondJSON(w, http.StatusOK, nut)
}

// handleGPU godoc
//
//	@Summary		Get GPU metrics
//	@Description	Retrieve GPU metrics for NVIDIA and AMD GPUs
//	@Tags			GPU
//	@Produce		json
//	@Success		200	{array}	dto.GPUMetrics	"List of GPU metrics"
//	@Router			/gpu [get]
func (s *Server) handleGPU(w http.ResponseWriter, _ *http.Request) {
	// Get latest GPU metrics from cache
	gpus := s.GetGPUCache()

	if gpus == nil {
		gpus = []*dto.GPUMetrics{}
	}

	respondJSON(w, http.StatusOK, gpus)
}

// handleNetwork godoc
//
//	@Summary		Get network interfaces
//	@Description	Retrieve information about all network interfaces
//	@Tags			Network
//	@Produce		json
//	@Success		200	{array}	dto.NetworkInfo	"List of network interfaces"
//	@Router			/network [get]
func (s *Server) handleNetwork(w http.ResponseWriter, _ *http.Request) {
	// Get latest network interfaces from cache
	interfaces := s.GetNetworkCache()

	if interfaces == nil {
		interfaces = []dto.NetworkInfo{}
	}

	respondJSON(w, http.StatusOK, interfaces)
}

// handleNetworkAccessURLs godoc
//
//	@Summary		Get network access URLs
//	@Description	Returns all methods to access the Unraid server including LAN IP, mDNS hostname, WireGuard VPN IPs, WAN IP, and IPv6 addresses
//	@Tags			Network
//	@Produce		json
//	@Success		200	{object}	dto.NetworkAccessURLs	"Network access URLs"
//	@Router			/network/access-urls [get]
func (s *Server) handleNetworkAccessURLs(w http.ResponseWriter, _ *http.Request) {
	accessURLs := collectors.CollectNetworkAccessURLs()
	respondJSON(w, http.StatusOK, accessURLs)
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

// handleDockerStart godoc
//
//	@Summary		Start Docker container
//	@Description	Start a specific Docker container by ID or name
//	@Tags			Docker
//	@Produce		json
//	@Param			id	path		string	true	"Container ID or name"
//	@Success		200	{object}	dto.Response	"Container started"
//	@Failure		400	{object}	dto.Response	"Invalid container ID"
//	@Failure		500	{object}	dto.Response	"Failed to start container"
//	@Router			/docker/{id}/start [post]
func (s *Server) handleDockerStart(w http.ResponseWriter, r *http.Request) {
	controller := controllers.NewDockerController()
	s.handleDockerOperation(w, r, "started", controller.Start)
}

// handleDockerStop godoc
//
//	@Summary		Stop Docker container
//	@Description	Stop a specific Docker container by ID or name
//	@Tags			Docker
//	@Produce		json
//	@Param			id	path		string	true	"Container ID or name"
//	@Success		200	{object}	dto.Response	"Container stopped"
//	@Failure		400	{object}	dto.Response	"Invalid container ID"
//	@Failure		500	{object}	dto.Response	"Failed to stop container"
//	@Router			/docker/{id}/stop [post]
func (s *Server) handleDockerStop(w http.ResponseWriter, r *http.Request) {
	controller := controllers.NewDockerController()
	s.handleDockerOperation(w, r, "stopped", controller.Stop)
}

// handleDockerRestart godoc
//
//	@Summary		Restart Docker container
//	@Description	Restart a specific Docker container by ID or name
//	@Tags			Docker
//	@Produce		json
//	@Param			id	path		string	true	"Container ID or name"
//	@Success		200	{object}	dto.Response	"Container restarted"
//	@Failure		400	{object}	dto.Response	"Invalid container ID"
//	@Failure		500	{object}	dto.Response	"Failed to restart container"
//	@Router			/docker/{id}/restart [post]
func (s *Server) handleDockerRestart(w http.ResponseWriter, r *http.Request) {
	controller := controllers.NewDockerController()
	s.handleDockerOperation(w, r, "restarted", controller.Restart)
}

// handleDockerPause godoc
//
//	@Summary		Pause Docker container
//	@Description	Pause a specific Docker container by ID or name
//	@Tags			Docker
//	@Produce		json
//	@Param			id	path		string	true	"Container ID or name"
//	@Success		200	{object}	dto.Response	"Container paused"
//	@Failure		400	{object}	dto.Response	"Invalid container ID"
//	@Failure		500	{object}	dto.Response	"Failed to pause container"
//	@Router			/docker/{id}/pause [post]
func (s *Server) handleDockerPause(w http.ResponseWriter, r *http.Request) {
	controller := controllers.NewDockerController()
	s.handleDockerOperation(w, r, "paused", controller.Pause)
}

// handleDockerUnpause godoc
//
//	@Summary		Unpause Docker container
//	@Description	Unpause a specific Docker container by ID or name
//	@Tags			Docker
//	@Produce		json
//	@Param			id	path		string	true	"Container ID or name"
//	@Success		200	{object}	dto.Response	"Container unpaused"
//	@Failure		400	{object}	dto.Response	"Invalid container ID"
//	@Failure		500	{object}	dto.Response	"Failed to unpause container"
//	@Router			/docker/{id}/unpause [post]
func (s *Server) handleDockerUnpause(w http.ResponseWriter, r *http.Request) {
	controller := controllers.NewDockerController()
	s.handleDockerOperation(w, r, "unpaused", controller.Unpause)
}

// handleVMStart godoc
//
//	@Summary		Start VM
//	@Description	Start a specific virtual machine by name
//	@Tags			VMs
//	@Produce		json
//	@Param			name	path		string	true	"VM name"
//	@Success		200		{object}	dto.Response	"VM started"
//	@Failure		400		{object}	dto.Response	"Invalid VM name"
//	@Failure		500		{object}	dto.Response	"Failed to start VM"
//	@Router			/vm/{name}/start [post]
func (s *Server) handleVMStart(w http.ResponseWriter, r *http.Request) {
	controller := controllers.NewVMController()
	s.handleVMOperation(w, r, "started", controller.Start)
}

// handleVMStop godoc
//
//	@Summary		Stop VM
//	@Description	Gracefully stop a specific virtual machine by name
//	@Tags			VMs
//	@Produce		json
//	@Param			name	path		string	true	"VM name"
//	@Success		200		{object}	dto.Response	"VM stopped"
//	@Failure		400		{object}	dto.Response	"Invalid VM name"
//	@Failure		500		{object}	dto.Response	"Failed to stop VM"
//	@Router			/vm/{name}/stop [post]
func (s *Server) handleVMStop(w http.ResponseWriter, r *http.Request) {
	controller := controllers.NewVMController()
	s.handleVMOperation(w, r, "stopped", controller.Stop)
}

// handleVMRestart godoc
//
//	@Summary		Restart VM
//	@Description	Restart a specific virtual machine by name
//	@Tags			VMs
//	@Produce		json
//	@Param			name	path		string	true	"VM name"
//	@Success		200		{object}	dto.Response	"VM restarted"
//	@Failure		400		{object}	dto.Response	"Invalid VM name"
//	@Failure		500		{object}	dto.Response	"Failed to restart VM"
//	@Router			/vm/{name}/restart [post]
func (s *Server) handleVMRestart(w http.ResponseWriter, r *http.Request) {
	controller := controllers.NewVMController()
	s.handleVMOperation(w, r, "restarted", controller.Restart)
}

// handleVMPause godoc
//
//	@Summary		Pause VM
//	@Description	Pause a specific virtual machine by name
//	@Tags			VMs
//	@Produce		json
//	@Param			name	path		string	true	"VM name"
//	@Success		200		{object}	dto.Response	"VM paused"
//	@Failure		400		{object}	dto.Response	"Invalid VM name"
//	@Failure		500		{object}	dto.Response	"Failed to pause VM"
//	@Router			/vm/{name}/pause [post]
func (s *Server) handleVMPause(w http.ResponseWriter, r *http.Request) {
	controller := controllers.NewVMController()
	s.handleVMOperation(w, r, "paused", controller.Pause)
}

// handleVMResume godoc
//
//	@Summary		Resume VM
//	@Description	Resume a paused virtual machine by name
//	@Tags			VMs
//	@Produce		json
//	@Param			name	path		string	true	"VM name"
//	@Success		200		{object}	dto.Response	"VM resumed"
//	@Failure		400		{object}	dto.Response	"Invalid VM name"
//	@Failure		500		{object}	dto.Response	"Failed to resume VM"
//	@Router			/vm/{name}/resume [post]
func (s *Server) handleVMResume(w http.ResponseWriter, r *http.Request) {
	controller := controllers.NewVMController()
	s.handleVMOperation(w, r, "resumed", controller.Resume)
}

// handleVMHibernate godoc
//
//	@Summary		Hibernate VM
//	@Description	Hibernate a specific virtual machine by name
//	@Tags			VMs
//	@Produce		json
//	@Param			name	path		string	true	"VM name"
//	@Success		200		{object}	dto.Response	"VM hibernated"
//	@Failure		400		{object}	dto.Response	"Invalid VM name"
//	@Failure		500		{object}	dto.Response	"Failed to hibernate VM"
//	@Router			/vm/{name}/hibernate [post]
func (s *Server) handleVMHibernate(w http.ResponseWriter, r *http.Request) {
	controller := controllers.NewVMController()
	s.handleVMOperation(w, r, "hibernated", controller.Hibernate)
}

// handleVMForceStop godoc
//
//	@Summary		Force stop VM
//	@Description	Force stop a specific virtual machine by name (equivalent to pulling the power cord)
//	@Tags			VMs
//	@Produce		json
//	@Param			name	path		string	true	"VM name"
//	@Success		200		{object}	dto.Response	"VM force stopped"
//	@Failure		400		{object}	dto.Response	"Invalid VM name"
//	@Failure		500		{object}	dto.Response	"Failed to force stop VM"
//	@Router			/vm/{name}/force-stop [post]
func (s *Server) handleVMForceStop(w http.ResponseWriter, r *http.Request) {
	controller := controllers.NewVMController()
	s.handleVMOperation(w, r, "force stopped", controller.ForceStop)
}

// handleArrayStart godoc
//
//	@Summary		Start array
//	@Description	Start the Unraid array
//	@Tags			Array
//	@Produce		json
//	@Success		200	{object}	dto.Response	"Array started"
//	@Failure		500	{object}	dto.Response	"Failed to start array"
//	@Router			/array/start [post]
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

// handleArrayStop godoc
//
//	@Summary		Stop array
//	@Description	Stop the Unraid array
//	@Tags			Array
//	@Produce		json
//	@Success		200	{object}	dto.Response	"Array stopped"
//	@Failure		500	{object}	dto.Response	"Failed to stop array"
//	@Router			/array/stop [post]
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

// handleParityCheckStart godoc
//
//	@Summary		Start parity check
//	@Description	Start a parity check operation, optionally with correction
//	@Tags			Array
//	@Produce		json
//	@Param			correcting	query		boolean	false	"Enable correcting mode"
//	@Success		200			{object}	dto.Response	"Parity check started"
//	@Failure		500			{object}	dto.Response	"Failed to start parity check"
//	@Router			/array/parity-check/start [post]
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

// handleParityCheckStop godoc
//
//	@Summary		Stop parity check
//	@Description	Stop an in-progress parity check operation
//	@Tags			Array
//	@Produce		json
//	@Success		200	{object}	dto.Response	"Parity check stopped"
//	@Failure		500	{object}	dto.Response	"Failed to stop parity check"
//	@Router			/array/parity-check/stop [post]
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

// handleParityCheckPause godoc
//
//	@Summary		Pause parity check
//	@Description	Pause an in-progress parity check operation
//	@Tags			Array
//	@Produce		json
//	@Success		200	{object}	dto.Response	"Parity check paused"
//	@Failure		500	{object}	dto.Response	"Failed to pause parity check"
//	@Router			/array/parity-check/pause [post]
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

// handleParityCheckResume godoc
//
//	@Summary		Resume parity check
//	@Description	Resume a paused parity check operation
//	@Tags			Array
//	@Produce		json
//	@Success		200	{object}	dto.Response	"Parity check resumed"
//	@Failure		500	{object}	dto.Response	"Failed to resume parity check"
//	@Router			/array/parity-check/resume [post]
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

// handleParityCheckHistory godoc
//
//	@Summary		Get parity check history
//	@Description	Retrieve the history of parity check operations
//	@Tags			Array
//	@Produce		json
//	@Success		200	{object}	dto.ParityCheckHistory	"Parity check history"
//	@Failure		500	{object}	dto.Response		"Failed to get parity check history"
//	@Router			/array/parity-check/history [get]
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

// handleShareConfig godoc
//
//	@Summary		Get share configuration
//	@Description	Retrieve configuration for a specific user share
//	@Tags			Configuration
//	@Produce		json
//	@Param			name	path		string	true	"Share name"
//	@Success		200		{object}	dto.ShareConfig	"Share configuration"
//	@Failure		400		{object}	dto.Response	"Invalid share name"
//	@Failure		404		{object}	dto.Response	"Share not found"
//	@Router			/shares/{name}/config [get]
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

// handleNetworkConfig godoc
//
//	@Summary		Get network interface configuration
//	@Description	Retrieve configuration for a specific network interface
//	@Tags			Configuration
//	@Produce		json
//	@Param			interface	path		string	true	"Interface name (e.g., eth0, bond0)"
//	@Success		200			{object}	dto.NetworkConfig	"Network configuration"
//	@Failure		404			{object}	dto.Response		"Interface not found"
//	@Router			/network/{interface}/config [get]
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

// handleSystemSettings godoc
//
//	@Summary		Get system settings
//	@Description	Retrieve Unraid system settings
//	@Tags			Configuration
//	@Produce		json
//	@Success		200	{object}	dto.SystemSettings	"System settings"
//	@Failure		500	{object}	dto.Response		"Failed to get settings"
//	@Router			/settings/system [get]
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

// handleDockerSettings godoc
//
//	@Summary		Get Docker settings
//	@Description	Retrieve Docker daemon settings
//	@Tags			Configuration
//	@Produce		json
//	@Success		200	{object}	dto.DockerSettings	"Docker settings"
//	@Failure		500	{object}	dto.Response		"Failed to get settings"
//	@Router			/settings/docker [get]
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

// handleVMSettings godoc
//
//	@Summary		Get VM settings
//	@Description	Retrieve virtual machine manager settings
//	@Tags			Configuration
//	@Produce		json
//	@Success		200	{object}	dto.VMSettings	"VM settings"
//	@Failure		500	{object}	dto.Response	"Failed to get settings"
//	@Router			/settings/vm [get]
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

// handleDiskSettings godoc
//
//	@Summary		Get disk settings
//	@Description	Retrieve disk configuration settings
//	@Tags			Configuration
//	@Produce		json
//	@Success		200	{object}	dto.DiskSettings	"Disk settings"
//	@Failure		500	{object}	dto.Response		"Failed to get settings"
//	@Router			/settings/disks [get]
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

// handleUpdateShareConfig godoc
//
//	@Summary		Update share configuration
//	@Description	Update configuration for a specific user share
//	@Tags			Configuration
//	@Accept			json
//	@Produce		json
//	@Param			name	path		string			true	"Share name"
//	@Param			config	body		dto.ShareConfig	true	"Share configuration"
//	@Success		200		{object}	dto.Response	"Configuration updated"
//	@Failure		400		{object}	dto.Response	"Invalid request"
//	@Failure		500		{object}	dto.Response	"Failed to update"
//	@Router			/shares/{name}/config [post]
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

// handleUpdateSystemSettings godoc
//
//	@Summary		Update system settings
//	@Description	Update Unraid system settings
//	@Tags			Configuration
//	@Accept			json
//	@Produce		json
//	@Param			settings	body		dto.SystemSettings	true	"System settings"
//	@Success		200			{object}	dto.Response		"Settings updated"
//	@Failure		400			{object}	dto.Response		"Invalid request"
//	@Failure		500			{object}	dto.Response		"Failed to update"
//	@Router			/settings/system [post]
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

// handleUserScripts godoc
//
//	@Summary		Get user scripts
//	@Description	Retrieve a list of all available user scripts
//	@Tags			User Scripts
//	@Produce		json
//	@Success		200	{array}		dto.UserScriptInfo	"List of user scripts"
//	@Failure		500	{object}	dto.Response	"Failed to list scripts"
//	@Router			/user-scripts [get]
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

// handleUserScriptExecute godoc
//
//	@Summary		Execute user script
//	@Description	Execute a user script by name
//	@Tags			User Scripts
//	@Accept			json
//	@Produce		json
//	@Param			name	path		string							true	"Script name"
//	@Param			options	body		dto.UserScriptExecuteRequest	false	"Execution options"
//	@Success		200		{object}	dto.UserScriptExecuteResponse	"Execution result"
//	@Failure		500		{object}	dto.UserScriptExecuteResponse	"Failed to execute"
//	@Router			/user-scripts/{name}/execute [post]
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

// handleHardwareFull godoc
//
//	@Summary		Get full hardware information
//	@Description	Retrieve complete hardware information including BIOS, CPU, and memory
//	@Tags			Hardware
//	@Produce		json
//	@Success		200	{object}	dto.HardwareInfo	"Hardware information"
//	@Router			/hardware/full [get]
func (s *Server) handleHardwareFull(w http.ResponseWriter, _ *http.Request) {
	hardware := s.hardwareCache.Load()

	if hardware == nil {
		hardware = &dto.HardwareInfo{
			Timestamp: time.Now(),
		}
	}

	respondJSON(w, http.StatusOK, hardware)
}

// handleHardwareBIOS godoc
//
//	@Summary		Get BIOS information
//	@Description	Retrieve BIOS information from DMI data
//	@Tags			Hardware
//	@Produce		json
//	@Success		200	{object}	dto.BIOSInfo		"BIOS information"
//	@Failure		404	{object}	map[string]string	"BIOS info not available"
//	@Router			/hardware/bios [get]
func (s *Server) handleHardwareBIOS(w http.ResponseWriter, _ *http.Request) {
	hardware := s.hardwareCache.Load()

	if hardware == nil || hardware.BIOS == nil {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "BIOS information not available"})
		return
	}

	respondJSON(w, http.StatusOK, hardware.BIOS)
}

// handleHardwareBaseboard godoc
//
//	@Summary		Get baseboard information
//	@Description	Retrieve motherboard/baseboard information from DMI data
//	@Tags			Hardware
//	@Produce		json
//	@Success		200	{object}	dto.BaseboardInfo	"Baseboard information"
//	@Failure		404	{object}	map[string]string	"Baseboard info not available"
//	@Router			/hardware/baseboard [get]
func (s *Server) handleHardwareBaseboard(w http.ResponseWriter, _ *http.Request) {
	hardware := s.hardwareCache.Load()

	if hardware == nil || hardware.Baseboard == nil {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Baseboard information not available"})
		return
	}

	respondJSON(w, http.StatusOK, hardware.Baseboard)
}

// handleHardwareCPU godoc
//
//	@Summary		Get CPU hardware information
//	@Description	Retrieve CPU hardware information from DMI data
//	@Tags			Hardware
//	@Produce		json
//	@Success		200	{object}	dto.CPUHardwareInfo		"CPU information"
//	@Failure		404	{object}	map[string]string	"CPU info not available"
//	@Router			/hardware/cpu [get]
func (s *Server) handleHardwareCPU(w http.ResponseWriter, _ *http.Request) {
	hardware := s.hardwareCache.Load()

	if hardware == nil || hardware.CPU == nil {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "CPU hardware information not available"})
		return
	}

	respondJSON(w, http.StatusOK, hardware.CPU)
}

// handleHardwareCache godoc
//
//	@Summary		Get CPU cache information
//	@Description	Retrieve CPU cache hierarchy information from DMI data
//	@Tags			Hardware
//	@Produce		json
//	@Success		200	{array}		dto.CPUCacheInfo		"CPU cache information"
//	@Failure		404	{object}	map[string]string	"Cache info not available"
//	@Router			/hardware/cache [get]
func (s *Server) handleHardwareCache(w http.ResponseWriter, _ *http.Request) {
	hardware := s.hardwareCache.Load()

	if hardware == nil || len(hardware.Cache) == 0 {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "CPU cache information not available"})
		return
	}

	respondJSON(w, http.StatusOK, hardware.Cache)
}

// handleHardwareMemoryArray godoc
//
//	@Summary		Get memory array information
//	@Description	Retrieve physical memory array information from DMI data
//	@Tags			Hardware
//	@Produce		json
//	@Success		200	{object}	dto.MemoryArrayInfo	"Memory array information"
//	@Failure		404	{object}	map[string]string	"Memory array info not available"
//	@Router			/hardware/memory-array [get]
func (s *Server) handleHardwareMemoryArray(w http.ResponseWriter, _ *http.Request) {
	hardware := s.hardwareCache.Load()

	if hardware == nil || hardware.MemoryArray == nil {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Memory array information not available"})
		return
	}

	respondJSON(w, http.StatusOK, hardware.MemoryArray)
}

// handleHardwareMemoryDevices godoc
//
//	@Summary		Get memory device information
//	@Description	Retrieve individual memory device (DIMM) information from DMI data
//	@Tags			Hardware
//	@Produce		json
//	@Success		200	{array}		dto.MemoryDeviceInfo	"Memory device information"
//	@Failure		404	{object}	map[string]string		"Memory device info not available"
//	@Router			/hardware/memory-devices [get]
func (s *Server) handleHardwareMemoryDevices(w http.ResponseWriter, _ *http.Request) {
	hardware := s.hardwareCache.Load()

	if hardware == nil || len(hardware.MemoryDevices) == 0 {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "Memory device information not available"})
		return
	}

	respondJSON(w, http.StatusOK, hardware.MemoryDevices)
}

// handleRegistration godoc
//
//	@Summary		Get registration status
//	@Description	Retrieve Unraid license/registration information
//	@Tags			System
//	@Produce		json
//	@Success		200	{object}	dto.Registration	"Registration information"
//	@Router			/registration [get]
func (s *Server) handleRegistration(w http.ResponseWriter, _ *http.Request) {
	logger.Debug("API: Getting registration information")

	registration := s.registrationCache.Load()

	if registration == nil {
		registration = &dto.Registration{
			Type:      "unknown",
			State:     "invalid",
			Timestamp: time.Now(),
		}
	}

	respondJSON(w, http.StatusOK, registration)
}

// handleLogs godoc
//
//	@Summary		Get logs
//	@Description	List available log files or get log content with optional pagination
//	@Tags			Logs
//	@Produce		json
//	@Param			path	query		string	false	"Log file path (if empty, lists all logs)"
//	@Param			lines	query		integer	false	"Number of lines to return"
//	@Param			start	query		integer	false	"Starting line number"
//	@Success		200		{object}	dto.LogFileContent	"Log content or list"
//	@Failure		500		{object}	map[string]string	"Error reading logs"
//	@Router			/logs [get]
func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	logger.Debug("API: Getting logs")

	// Get query parameters
	path := r.URL.Query().Get("path")
	linesParam := r.URL.Query().Get("lines")
	startParam := r.URL.Query().Get("start")

	// If no path specified, list all available logs
	if path == "" {
		logs := s.listLogFiles()
		respondJSON(w, http.StatusOK, map[string]any{"logs": logs})
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

// handleLogFile godoc
//
//	@Summary		Get specific log file
//	@Description	Retrieve a specific log file by filename with optional pagination
//	@Tags			Logs
//	@Produce		json
//	@Param			filename	path		string	true	"Log filename"
//	@Param			lines		query		integer	false	"Number of lines to return"
//	@Param			start		query		integer	false	"Starting line number"
//	@Success		200			{object}	dto.LogFileContent	"Log content"
//	@Failure		400			{object}	dto.Response	"Invalid filename"
//	@Failure		404			{object}	dto.Response	"Log file not found"
//	@Failure		500			{object}	dto.Response	"Error reading log"
//	@Router			/logs/{filename} [get]
func (s *Server) handleLogFile(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	filename := vars["filename"]
	logger.Debug("API: Getting log file: %s", filename)

	// Validate filename to prevent directory traversal (CWE-22)
	if !lib.ValidateLogFilename(filename) {
		respondJSON(w, http.StatusBadRequest, dto.Response{
			Success:   false,
			Message:   "Invalid filename",
			Timestamp: time.Now(),
		})
		return
	}

	// Find the log file in our known log paths
	logs := s.listLogFiles()
	var foundPath string
	for _, log := range logs {
		// Match by filename (base name) or full name (for plugin logs)
		if log.Name == filename || filepath.Base(log.Path) == filename {
			foundPath = log.Path
			break
		}
	}

	if foundPath == "" {
		respondJSON(w, http.StatusNotFound, dto.Response{
			Success:   false,
			Message:   fmt.Sprintf("Log file not found: %s", filename),
			Timestamp: time.Now(),
		})
		return
	}

	// Get optional query parameters for pagination
	linesParam := r.URL.Query().Get("lines")
	startParam := r.URL.Query().Get("start")

	// Get log content
	content, err := s.getLogContent(foundPath, linesParam, startParam)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, dto.Response{
			Success:   false,
			Message:   err.Error(),
			Timestamp: time.Now(),
		})
		return
	}

	respondJSON(w, http.StatusOK, content)
}

// Helper function to respond with JSON
func respondJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		logger.Error("Failed to encode JSON response: %v", err)
	}
}

// Helper function to respond with error
func respondWithError(w http.ResponseWriter, status int, message string) {
	respondJSON(w, status, dto.Response{
		Success:   false,
		Message:   message,
		Timestamp: time.Now(),
	})
}

// handleNotifications godoc
//
//	@Summary		Get all notifications
//	@Description	Retrieve all notifications with overview counts, optionally filtered by importance
//	@Tags			Notifications
//	@Produce		json
//	@Param			importance	query		string	false	"Filter by importance level (alert, warning, normal)"
//	@Success		200			{object}	dto.NotificationList	"Notifications with overview"
//	@Router			/notifications [get]
func (s *Server) handleNotifications(w http.ResponseWriter, r *http.Request) {
	notificationList := s.notificationsCache.Load()

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

// handleNotificationsUnread godoc
//
//	@Summary		Get unread notifications
//	@Description	Retrieve only unread notifications
//	@Tags			Notifications
//	@Produce		json
//	@Success		200	{object}	map[string]interface{}	"Unread notifications with count"
//	@Router			/notifications/unread [get]
func (s *Server) handleNotificationsUnread(w http.ResponseWriter, _ *http.Request) {
	notificationList := s.notificationsCache.Load()

	if notificationList == nil {
		respondJSON(w, http.StatusOK, map[string]any{
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

	respondJSON(w, http.StatusOK, map[string]any{
		"notifications": unread,
		"count":         len(unread),
	})
}

// handleNotificationsArchive godoc
//
//	@Summary		Get archived notifications
//	@Description	Retrieve only archived notifications
//	@Tags			Notifications
//	@Produce		json
//	@Success		200	{object}	map[string]interface{}	"Archived notifications with count"
//	@Router			/notifications/archive [get]
func (s *Server) handleNotificationsArchive(w http.ResponseWriter, _ *http.Request) {
	notificationList := s.notificationsCache.Load()

	if notificationList == nil {
		respondJSON(w, http.StatusOK, map[string]any{
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

	respondJSON(w, http.StatusOK, map[string]any{
		"notifications": archived,
		"count":         len(archived),
	})
}

// handleNotificationsOverview godoc
//
//	@Summary		Get notification counts
//	@Description	Retrieve only the notification overview counts
//	@Tags			Notifications
//	@Produce		json
//	@Success		200	{object}	dto.NotificationOverview	"Notification counts by category"
//	@Router			/notifications/overview [get]
func (s *Server) handleNotificationsOverview(w http.ResponseWriter, _ *http.Request) {
	notificationList := s.notificationsCache.Load()

	if notificationList == nil {
		respondJSON(w, http.StatusOK, dto.NotificationOverview{
			Unread:  dto.NotificationCounts{},
			Archive: dto.NotificationCounts{},
		})
		return
	}

	respondJSON(w, http.StatusOK, notificationList.Overview)
}

// handleNotificationByID godoc
//
//	@Summary		Get notification by ID
//	@Description	Retrieve a specific notification by its ID
//	@Tags			Notifications
//	@Produce		json
//	@Param			id	path		string	true	"Notification ID"
//	@Success		200	{object}	dto.Notification	"Notification details"
//	@Failure		404	{object}	map[string]string	"Notification not found"
//	@Router			/notifications/{id} [get]
func (s *Server) handleNotificationByID(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	notificationList := s.notificationsCache.Load()

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

// handleCreateNotification godoc
//
//	@Summary		Create notification
//	@Description	Create a new system notification
//	@Tags			Notifications
//	@Accept			json
//	@Produce		json
//	@Param			notification	body		object{title=string,subject=string,description=string,importance=string,link=string}	true	"Notification data"
//	@Success		201				{object}	map[string]string	"Notification created"
//	@Failure		400				{object}	map[string]string	"Invalid request"
//	@Failure		500				{object}	map[string]string	"Failed to create notification"
//	@Router			/notifications [post]
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

// handleArchiveNotification godoc
//
//	@Summary		Archive notification
//	@Description	Archive a specific notification by ID
//	@Tags			Notifications
//	@Produce		json
//	@Param			id	path		string	true	"Notification ID"
//	@Success		200	{object}	map[string]string	"Notification archived"
//	@Failure		500	{object}	map[string]string	"Failed to archive"
//	@Router			/notifications/{id}/archive [post]
func (s *Server) handleArchiveNotification(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	if err := controllers.ArchiveNotification(id); err != nil {
		respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"message": "Notification archived successfully"})
}

// handleUnarchiveNotification godoc
//
//	@Summary		Unarchive notification
//	@Description	Unarchive a specific notification by ID
//	@Tags			Notifications
//	@Produce		json
//	@Param			id	path		string	true	"Notification ID"
//	@Success		200	{object}	map[string]string	"Notification unarchived"
//	@Failure		500	{object}	map[string]string	"Failed to unarchive"
//	@Router			/notifications/{id}/unarchive [post]
func (s *Server) handleUnarchiveNotification(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	if err := controllers.UnarchiveNotification(id); err != nil {
		respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"message": "Notification unarchived successfully"})
}

// handleDeleteNotification godoc
//
//	@Summary		Delete notification
//	@Description	Delete a specific notification by ID
//	@Tags			Notifications
//	@Produce		json
//	@Param			id			path		string	true	"Notification ID"
//	@Param			archived	query		boolean	false	"Whether notification is archived"
//	@Success		200			{object}	map[string]string	"Notification deleted"
//	@Failure		500			{object}	map[string]string	"Failed to delete"
//	@Router			/notifications/{id} [delete]
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

// handleArchiveAllNotifications godoc
//
//	@Summary		Archive all notifications
//	@Description	Archive all unread notifications
//	@Tags			Notifications
//	@Produce		json
//	@Success		200	{object}	map[string]string	"All notifications archived"
//	@Failure		500	{object}	map[string]string	"Failed to archive"
//	@Router			/notifications/archive/all [post]
func (s *Server) handleArchiveAllNotifications(w http.ResponseWriter, _ *http.Request) {
	if err := controllers.ArchiveAllNotifications(); err != nil {
		respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"message": "All notifications archived successfully"})
}

// handleUnassignedDevices godoc
//
//	@Summary		Get all unassigned devices
//	@Description	Retrieve all unassigned devices and remote shares
//	@Tags			Unassigned Devices
//	@Produce		json
//	@Success		200	{object}	dto.UnassignedDeviceList	"Unassigned devices and remote shares"
//	@Router			/unassigned [get]
func (s *Server) handleUnassignedDevices(w http.ResponseWriter, _ *http.Request) {
	cache := s.unassignedCache.Load()

	if cache == nil {
		respondJSON(w, http.StatusOK, map[string]any{
			"devices":       []any{},
			"remote_shares": []any{},
			"timestamp":     time.Now(),
		})
		return
	}

	respondJSON(w, http.StatusOK, cache)
}

// handleUnassignedDevicesList godoc
//
//	@Summary		Get unassigned devices only
//	@Description	Retrieve only unassigned devices (excludes remote shares)
//	@Tags			Unassigned Devices
//	@Produce		json
//	@Success		200	{object}	map[string]interface{}	"Unassigned devices"
//	@Router			/unassigned/devices [get]
func (s *Server) handleUnassignedDevicesList(w http.ResponseWriter, _ *http.Request) {
	cache := s.unassignedCache.Load()

	if cache == nil {
		respondJSON(w, http.StatusOK, map[string]any{
			"devices":   []any{},
			"timestamp": time.Now(),
		})
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"devices":   cache.Devices,
		"timestamp": cache.Timestamp,
	})
}

// handleUnassignedRemoteShares godoc
//
//	@Summary		Get remote shares only
//	@Description	Retrieve only remote shares (excludes local unassigned devices)
//	@Tags			Unassigned Devices
//	@Produce		json
//	@Success		200	{object}	map[string]interface{}	"Remote shares"
//	@Router			/unassigned/remote-shares [get]
func (s *Server) handleUnassignedRemoteShares(w http.ResponseWriter, _ *http.Request) {
	cache := s.unassignedCache.Load()

	if cache == nil {
		respondJSON(w, http.StatusOK, map[string]any{
			"remote_shares": []any{},
			"timestamp":     time.Now(),
		})
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"remote_shares": cache.RemoteShares,
		"timestamp":     cache.Timestamp,
	})
}

// ============================================================================
// ZFS Handlers
// ============================================================================

// handleZFSPools godoc
//
//	@Summary		Get all ZFS pools
//	@Description	Retrieve information about all ZFS pools
//	@Tags			ZFS
//	@Produce		json
//	@Success		200	{array}	dto.ZFSPool	"List of ZFS pools"
//	@Router			/zfs/pools [get]
func (s *Server) handleZFSPools(w http.ResponseWriter, _ *http.Request) {
	pools := s.GetZFSPoolsCache()

	if pools == nil {
		pools = []dto.ZFSPool{}
	}

	respondJSON(w, http.StatusOK, pools)
}

// handleZFSPool godoc
//
//	@Summary		Get specific ZFS pool
//	@Description	Retrieve information about a specific ZFS pool by name
//	@Tags			ZFS
//	@Produce		json
//	@Param			name	path		string	true	"Pool name"
//	@Success		200		{object}	dto.ZFSPool		"ZFS pool information"
//	@Failure		404		{object}	dto.Response	"Pool not found"
//	@Router			/zfs/pools/{name} [get]
func (s *Server) handleZFSPool(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	poolName := vars["name"]

	pools := s.GetZFSPoolsCache()

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

// handleZFSDatasets godoc
//
//	@Summary		Get all ZFS datasets
//	@Description	Retrieve information about all ZFS datasets
//	@Tags			ZFS
//	@Produce		json
//	@Success		200	{array}	dto.ZFSDataset	"List of ZFS datasets"
//	@Router			/zfs/datasets [get]
func (s *Server) handleZFSDatasets(w http.ResponseWriter, _ *http.Request) {
	datasets := s.GetZFSDatasetsCache()

	if datasets == nil {
		datasets = []dto.ZFSDataset{}
	}

	respondJSON(w, http.StatusOK, datasets)
}

// handleZFSSnapshots godoc
//
//	@Summary		Get all ZFS snapshots
//	@Description	Retrieve information about all ZFS snapshots
//	@Tags			ZFS
//	@Produce		json
//	@Success		200	{array}	dto.ZFSSnapshot	"List of ZFS snapshots"
//	@Router			/zfs/snapshots [get]
func (s *Server) handleZFSSnapshots(w http.ResponseWriter, _ *http.Request) {
	snapshots := s.GetZFSSnapshotsCache()

	if snapshots == nil {
		snapshots = []dto.ZFSSnapshot{}
	}

	respondJSON(w, http.StatusOK, snapshots)
}

// handleZFSARC godoc
//
//	@Summary		Get ZFS ARC statistics
//	@Description	Retrieve ZFS Adaptive Replacement Cache (ARC) statistics
//	@Tags			ZFS
//	@Produce		json
//	@Success		200	{object}	dto.ZFSARCStats	"ZFS ARC statistics"
//	@Router			/zfs/arc [get]
func (s *Server) handleZFSARC(w http.ResponseWriter, _ *http.Request) {
	arcStats := s.zfsARCStatsCache.Load()

	if arcStats == nil {
		arcStats = &dto.ZFSARCStats{
			Timestamp: time.Now(),
		}
	}

	respondJSON(w, http.StatusOK, arcStats)
}

// handleCollectorsStatus godoc
//
//	@Summary		Get all collectors status
//	@Description	Retrieve status of all data collectors including enabled state and intervals
//	@Tags			Collectors
//	@Produce		json
//	@Success		200	{object}	dto.CollectorsStatusResponse	"Collectors status"
//	@Router			/collectors/status [get]
func (s *Server) handleCollectorsStatus(w http.ResponseWriter, _ *http.Request) {
	// If we have a collector manager, use it for real-time status
	if s.collectorManager != nil {
		status := s.collectorManager.GetAllStatus()
		respondJSON(w, http.StatusOK, status)
		return
	}

	// Fallback to static configuration (legacy mode)
	type collectorDef struct {
		name     string
		interval int
	}

	collectors := []collectorDef{
		{"system", s.ctx.Intervals.System},
		{"array", s.ctx.Intervals.Array},
		{"disk", s.ctx.Intervals.Disk},
		{"docker", s.ctx.Intervals.Docker},
		{"vm", s.ctx.Intervals.VM},
		{"ups", s.ctx.Intervals.UPS},
		{"nut", s.ctx.Intervals.NUT},
		{"gpu", s.ctx.Intervals.GPU},
		{"shares", s.ctx.Intervals.Shares},
		{"network", s.ctx.Intervals.Network},
		{"hardware", s.ctx.Intervals.Hardware},
		{"zfs", s.ctx.Intervals.ZFS},
		{"notification", s.ctx.Intervals.Notification},
		{"registration", s.ctx.Intervals.Registration},
		{"unassigned", s.ctx.Intervals.Unassigned},
	}

	var statuses []dto.CollectorStatus
	enabledCount := 0
	disabledCount := 0

	for _, c := range collectors {
		enabled := c.interval > 0
		status := "running"
		if !enabled {
			status = "disabled"
			disabledCount++
		} else {
			enabledCount++
		}

		statuses = append(statuses, dto.CollectorStatus{
			Name:     c.name,
			Enabled:  enabled,
			Interval: c.interval,
			Status:   status,
		})
	}

	respondJSON(w, http.StatusOK, dto.CollectorsStatusResponse{
		Collectors:    statuses,
		Total:         len(collectors),
		EnabledCount:  enabledCount,
		DisabledCount: disabledCount,
		Timestamp:     time.Now(),
	})
}

// handleCollectorStatus godoc
//
//	@Summary		Get specific collector status
//	@Description	Retrieve status of a specific collector by name
//	@Tags			Collectors
//	@Produce		json
//	@Param			name	path		string	true	"Collector name (e.g., system, docker, vm)"
//	@Success		200		{object}	dto.CollectorResponse	"Collector status"
//	@Failure		404		{object}	dto.Response			"Collector not found"
//	@Failure		503		{object}	dto.Response			"Collector management not available"
//	@Router			/collectors/{name} [get]
func (s *Server) handleCollectorStatus(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	name := vars["name"]

	if s.collectorManager == nil {
		respondJSON(w, http.StatusServiceUnavailable, dto.Response{
			Success:   false,
			Message:   "collector management not available",
			Timestamp: time.Now(),
		})
		return
	}

	status, err := s.collectorManager.GetStatus(name)
	if err != nil {
		respondJSON(w, http.StatusNotFound, dto.Response{
			Success:   false,
			Message:   err.Error(),
			Timestamp: time.Now(),
		})
		return
	}

	respondJSON(w, http.StatusOK, dto.CollectorResponse{
		Success:   true,
		Message:   fmt.Sprintf("collector %s status retrieved", name),
		Collector: *status,
		Timestamp: time.Now(),
	})
}

// handleCollectorEnable godoc
//
//	@Summary		Enable collector
//	@Description	Enable a specific collector at runtime
//	@Tags			Collectors
//	@Produce		json
//	@Param			name	path		string	true	"Collector name"
//	@Success		200		{object}	dto.CollectorResponse	"Collector enabled"
//	@Failure		400		{object}	dto.Response			"Invalid request"
//	@Failure		404		{object}	dto.Response			"Collector not found"
//	@Failure		503		{object}	dto.Response			"Collector management not available"
//	@Router			/collectors/{name}/enable [post]
func (s *Server) handleCollectorEnable(w http.ResponseWriter, r *http.Request) {
	s.handleCollectorStateChange(w, r, true)
}

// handleCollectorDisable godoc
//
//	@Summary		Disable collector
//	@Description	Disable a specific collector at runtime
//	@Tags			Collectors
//	@Produce		json
//	@Param			name	path		string	true	"Collector name"
//	@Success		200		{object}	dto.CollectorResponse	"Collector disabled"
//	@Failure		400		{object}	dto.Response			"Invalid request"
//	@Failure		404		{object}	dto.Response			"Collector not found"
//	@Failure		503		{object}	dto.Response			"Collector management not available"
//	@Router			/collectors/{name}/disable [post]
func (s *Server) handleCollectorDisable(w http.ResponseWriter, r *http.Request) {
	s.handleCollectorStateChange(w, r, false)
}

// handleCollectorStateChange is a shared helper for enable/disable operations
func (s *Server) handleCollectorStateChange(w http.ResponseWriter, r *http.Request, enable bool) {
	vars := mux.Vars(r)
	name := vars["name"]

	if s.collectorManager == nil {
		respondJSON(w, http.StatusServiceUnavailable, dto.Response{
			Success:   false,
			Message:   "collector management not available",
			Timestamp: time.Now(),
		})
		return
	}

	action := "Enabling"
	actionPast := "enabled"
	var actionErr error
	if enable {
		logger.Info("Enabling collector: %s", name)
		actionErr = s.collectorManager.EnableCollector(name)
	} else {
		action = "Disabling"
		actionPast = "disabled"
		logger.Info("Disabling collector: %s", name)
		actionErr = s.collectorManager.DisableCollector(name)
	}

	if actionErr != nil {
		statusCode := http.StatusBadRequest
		if actionErr.Error() == fmt.Sprintf("unknown collector: %s", name) {
			statusCode = http.StatusNotFound
		}
		respondJSON(w, statusCode, dto.Response{
			Success:   false,
			Message:   actionErr.Error(),
			Timestamp: time.Now(),
		})
		return
	}

	status, err := s.collectorManager.GetStatus(name)
	if err != nil {
		logger.Warning("%s collector %s succeeded but status retrieval failed: %v", action, name, err)
	}
	respondJSON(w, http.StatusOK, dto.CollectorResponse{
		Success:   true,
		Message:   fmt.Sprintf("%s collector %s", name, actionPast),
		Collector: *status,
		Timestamp: time.Now(),
	})
}

// handleCollectorInterval godoc
//
//	@Summary		Update collector interval
//	@Description	Update the collection interval for a specific collector
//	@Tags			Collectors
//	@Accept			json
//	@Produce		json
//	@Param			name		path		string							true	"Collector name"
//	@Param			interval	body		dto.CollectorIntervalRequest	true	"New interval in seconds"
//	@Success		200			{object}	dto.CollectorResponse			"Interval updated"
//	@Failure		400			{object}	dto.Response					"Invalid request"
//	@Failure		404			{object}	dto.Response					"Collector not found"
//	@Failure		503			{object}	dto.Response					"Collector management not available"
//	@Router			/collectors/{name}/interval [patch]
func (s *Server) handleCollectorInterval(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	name := vars["name"]

	if s.collectorManager == nil {
		respondJSON(w, http.StatusServiceUnavailable, dto.Response{
			Success:   false,
			Message:   "collector management not available",
			Timestamp: time.Now(),
		})
		return
	}

	var req dto.CollectorIntervalRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, dto.Response{
			Success:   false,
			Message:   "invalid request body",
			Timestamp: time.Now(),
		})
		return
	}

	logger.Info("Updating collector %s interval to %d seconds", name, req.Interval)

	if err := s.collectorManager.UpdateInterval(name, req.Interval); err != nil {
		statusCode := http.StatusBadRequest
		if err.Error() == fmt.Sprintf("unknown collector: %s", name) {
			statusCode = http.StatusNotFound
		}
		respondJSON(w, statusCode, dto.Response{
			Success:   false,
			Message:   err.Error(),
			Timestamp: time.Now(),
		})
		return
	}

	status, _ := s.collectorManager.GetStatus(name)
	respondJSON(w, http.StatusOK, dto.CollectorResponse{
		Success:   true,
		Message:   fmt.Sprintf("%s collector interval updated to %d seconds", name, req.Interval),
		Collector: *status,
		Timestamp: time.Now(),
	})
}

// =============================================================================
// Settings Endpoints - Issues #45, #46, #47, #48, #49, #50, #51, #52, #53
// =============================================================================

// handleDiskSettingsExtended godoc
//
//	@Summary		Get extended disk settings with temperature thresholds
//	@Description	Retrieve disk configuration settings including global temperature thresholds for HDD and SSD
//	@Tags			Configuration
//	@Produce		json
//	@Success		200	{object}	dto.DiskSettingsExtended	"Extended disk settings with temp thresholds"
//	@Failure		500	{object}	dto.Response				"Failed to get settings"
//	@Router			/settings/disk-thresholds [get]
func (s *Server) handleDiskSettingsExtended(w http.ResponseWriter, _ *http.Request) {
	logger.Debug("API: Getting extended disk settings with temperature thresholds")

	settingsCollector := collectors.NewSettingsCollector()
	settings, err := settingsCollector.GetDiskSettingsExtended()

	if err != nil {
		logger.Error("API: Failed to get extended disk settings: %v", err)
		respondJSON(w, http.StatusInternalServerError, dto.Response{
			Success:   false,
			Message:   fmt.Sprintf("Failed to get extended disk settings: %v", err),
			Timestamp: time.Now(),
		})
		return
	}

	respondJSON(w, http.StatusOK, settings)
}

// handleMoverSettings godoc
//
//	@Summary		Get mover schedule and status
//	@Description	Retrieve mover configuration, schedule, and current running status
//	@Tags			Configuration
//	@Produce		json
//	@Success		200	{object}	dto.MoverSettings	"Mover settings and status"
//	@Failure		500	{object}	dto.Response		"Failed to get settings"
//	@Router			/settings/mover [get]
func (s *Server) handleMoverSettings(w http.ResponseWriter, _ *http.Request) {
	logger.Debug("API: Getting mover settings")

	settingsCollector := collectors.NewSettingsCollector()
	settings, err := settingsCollector.GetMoverSettings()

	if err != nil {
		logger.Error("API: Failed to get mover settings: %v", err)
		respondJSON(w, http.StatusInternalServerError, dto.Response{
			Success:   false,
			Message:   fmt.Sprintf("Failed to get mover settings: %v", err),
			Timestamp: time.Now(),
		})
		return
	}

	respondJSON(w, http.StatusOK, settings)
}

// handleParitySchedule godoc
//
//	@Summary		Get parity check schedule
//	@Description	Retrieve parity check schedule configuration
//	@Tags			Array
//	@Produce		json
//	@Success		200	{object}	dto.ParitySchedule	"Parity check schedule"
//	@Failure		500	{object}	dto.Response		"Failed to get schedule"
//	@Router			/array/parity-check/schedule [get]
func (s *Server) handleParitySchedule(w http.ResponseWriter, _ *http.Request) {
	logger.Debug("API: Getting parity check schedule")

	settingsCollector := collectors.NewSettingsCollector()
	schedule, err := settingsCollector.GetParitySchedule()

	if err != nil {
		logger.Error("API: Failed to get parity schedule: %v", err)
		respondJSON(w, http.StatusInternalServerError, dto.Response{
			Success:   false,
			Message:   fmt.Sprintf("Failed to get parity schedule: %v", err),
			Timestamp: time.Now(),
		})
		return
	}

	respondJSON(w, http.StatusOK, schedule)
}

// handleServiceStatus godoc
//
//	@Summary		Get Docker and VM service enabled status
//	@Description	Retrieve whether Docker and VM Manager services are enabled in Unraid settings
//	@Tags			Configuration
//	@Produce		json
//	@Success		200	{object}	dto.ServiceStatus	"Service enabled status"
//	@Failure		500	{object}	dto.Response		"Failed to get status"
//	@Router			/settings/services [get]
func (s *Server) handleServiceStatus(w http.ResponseWriter, _ *http.Request) {
	logger.Debug("API: Getting service status")

	settingsCollector := collectors.NewSettingsCollector()
	status, err := settingsCollector.GetServiceStatus()

	if err != nil {
		logger.Error("API: Failed to get service status: %v", err)
		respondJSON(w, http.StatusInternalServerError, dto.Response{
			Success:   false,
			Message:   fmt.Sprintf("Failed to get service status: %v", err),
			Timestamp: time.Now(),
		})
		return
	}

	respondJSON(w, http.StatusOK, status)
}

// handlePluginList godoc
//
//	@Summary		Get installed plugins list
//	@Description	Retrieve list of installed plugins with their versions and update status
//	@Tags			Plugins
//	@Produce		json
//	@Success		200	{object}	dto.PluginList	"List of installed plugins"
//	@Failure		500	{object}	dto.Response	"Failed to get plugins"
//	@Router			/plugins [get]
func (s *Server) handlePluginList(w http.ResponseWriter, _ *http.Request) {
	logger.Debug("API: Getting plugin list")

	settingsCollector := collectors.NewSettingsCollector()
	plugins, err := settingsCollector.GetPluginList()

	if err != nil {
		logger.Error("API: Failed to get plugin list: %v", err)
		respondJSON(w, http.StatusInternalServerError, dto.Response{
			Success:   false,
			Message:   fmt.Sprintf("Failed to get plugin list: %v", err),
			Timestamp: time.Now(),
		})
		return
	}

	respondJSON(w, http.StatusOK, plugins)
}

// handleUpdateStatus godoc
//
//	@Summary		Get update availability status
//	@Description	Retrieve Unraid OS and plugin update availability information
//	@Tags			Updates
//	@Produce		json
//	@Success		200	{object}	dto.UpdateStatus	"Update availability status"
//	@Failure		500	{object}	dto.Response		"Failed to get update status"
//	@Router			/updates [get]
func (s *Server) handleUpdateStatus(w http.ResponseWriter, _ *http.Request) {
	logger.Debug("API: Getting update status")

	settingsCollector := collectors.NewSettingsCollector()
	status, err := settingsCollector.GetUpdateStatus()

	if err != nil {
		logger.Error("API: Failed to get update status: %v", err)
		respondJSON(w, http.StatusInternalServerError, dto.Response{
			Success:   false,
			Message:   fmt.Sprintf("Failed to get update status: %v", err),
			Timestamp: time.Now(),
		})
		return
	}

	respondJSON(w, http.StatusOK, status)
}

// handleFlashHealth godoc
//
//	@Summary		Get USB flash drive health
//	@Description	Retrieve health information for the USB flash boot drive
//	@Tags			System
//	@Produce		json
//	@Success		200	{object}	dto.FlashDriveHealth	"Flash drive health information"
//	@Failure		500	{object}	dto.Response			"Failed to get flash health"
//	@Router			/system/flash [get]
func (s *Server) handleFlashHealth(w http.ResponseWriter, _ *http.Request) {
	logger.Debug("API: Getting flash drive health")

	settingsCollector := collectors.NewSettingsCollector()
	health, err := settingsCollector.GetFlashDriveHealth()

	if err != nil {
		logger.Error("API: Failed to get flash drive health: %v", err)
		respondJSON(w, http.StatusInternalServerError, dto.Response{
			Success:   false,
			Message:   fmt.Sprintf("Failed to get flash drive health: %v", err),
			Timestamp: time.Now(),
		})
		return
	}

	respondJSON(w, http.StatusOK, health)
}

// handleNetworkServices godoc
//
//	@Summary		Get network services status
//	@Description	Retrieve status of all network services including SMB, NFS, FTP, SSH, Telnet, Avahi, WireGuard, etc.
//	@Tags			Configuration
//	@Produce		json
//	@Success		200	{object}	dto.NetworkServicesStatus	"Network services status"
//	@Failure		500	{object}	dto.Response				"Failed to get network services status"
//	@Router			/settings/network-services [get]
func (s *Server) handleNetworkServices(w http.ResponseWriter, _ *http.Request) {
	logger.Debug("API: Getting network services status")

	settingsCollector := collectors.NewSettingsCollector()
	status, err := settingsCollector.GetNetworkServicesStatus()

	if err != nil {
		logger.Error("API: Failed to get network services status: %v", err)
		respondJSON(w, http.StatusInternalServerError, dto.Response{
			Success:   false,
			Message:   fmt.Sprintf("Failed to get network services status: %v", err),
			Timestamp: time.Now(),
		})
		return
	}

	respondJSON(w, http.StatusOK, status)
}

// =============================================================================
// MQTT Endpoints
// =============================================================================

// handleMQTTStatus godoc
//
//	@Summary		Get MQTT status
//	@Description	Retrieve MQTT connection status and configuration
//	@Tags			MQTT
//	@Produce		json
//	@Success		200	{object}	dto.MQTTStatus	"MQTT status"
//	@Router			/mqtt/status [get]
func (s *Server) handleMQTTStatus(w http.ResponseWriter, _ *http.Request) {
	logger.Debug("API: Getting MQTT status")

	// Check if MQTT is configured
	if s.mqttClient == nil {
		respondJSON(w, http.StatusOK, dto.MQTTStatus{
			Connected: false,
			Enabled:   s.ctx.MQTTConfig.Enabled,
			Broker:    s.ctx.MQTTConfig.Broker,
			LastError: "MQTT client not initialized",
			Timestamp: time.Now(),
		})
		return
	}

	status := s.mqttClient.GetStatus()
	respondJSON(w, http.StatusOK, status)
}

// handleMQTTTest godoc
//
//	@Summary		Test MQTT connection
//	@Description	Test the MQTT broker connection
//	@Tags			MQTT
//	@Produce		json
//	@Success		200	{object}	dto.MQTTTestResponse	"Test successful"
//	@Failure		500	{object}	dto.MQTTTestResponse	"Test failed"
//	@Router			/mqtt/test [post]
func (s *Server) handleMQTTTest(w http.ResponseWriter, _ *http.Request) {
	logger.Info("API: Testing MQTT connection")

	if s.mqttClient == nil {
		respondJSON(w, http.StatusServiceUnavailable, dto.MQTTTestResponse{
			Success:   false,
			Message:   "MQTT client not initialized. Enable MQTT in configuration.",
			Timestamp: time.Now(),
		})
		return
	}

	err := s.mqttClient.TestConnection()
	if err != nil {
		logger.Error("API: MQTT test failed: %v", err)
		respondJSON(w, http.StatusInternalServerError, dto.MQTTTestResponse{
			Success:   false,
			Message:   fmt.Sprintf("MQTT connection test failed: %v", err),
			Timestamp: time.Now(),
		})
		return
	}

	respondJSON(w, http.StatusOK, dto.MQTTTestResponse{
		Success:   true,
		Message:   "MQTT connection test successful",
		Timestamp: time.Now(),
	})
}

// handleMQTTPublish godoc
//
//	@Summary		Publish custom MQTT message
//	@Description	Publish a custom message to a specific MQTT topic
//	@Tags			MQTT
//	@Accept			json
//	@Produce		json
//	@Param			message	body		dto.MQTTPublishRequest	true	"Message to publish"
//	@Success		200		{object}	dto.Response			"Message published"
//	@Failure		400		{object}	dto.Response			"Invalid request"
//	@Failure		500		{object}	dto.Response			"Failed to publish"
//	@Router			/mqtt/publish [post]
func (s *Server) handleMQTTPublish(w http.ResponseWriter, r *http.Request) {
	logger.Info("API: Publishing custom MQTT message")

	if s.mqttClient == nil {
		respondJSON(w, http.StatusServiceUnavailable, dto.Response{
			Success:   false,
			Message:   "MQTT client not initialized. Enable MQTT in configuration.",
			Timestamp: time.Now(),
		})
		return
	}

	var req dto.MQTTPublishRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, dto.Response{
			Success:   false,
			Message:   fmt.Sprintf("Invalid request body: %v", err),
			Timestamp: time.Now(),
		})
		return
	}

	if req.Topic == "" {
		respondJSON(w, http.StatusBadRequest, dto.Response{
			Success:   false,
			Message:   "Topic is required",
			Timestamp: time.Now(),
		})
		return
	}

	err := s.mqttClient.PublishCustom(req.Topic, req.Payload, req.Retained)
	if err != nil {
		logger.Error("API: Failed to publish MQTT message: %v", err)
		respondJSON(w, http.StatusInternalServerError, dto.Response{
			Success:   false,
			Message:   fmt.Sprintf("Failed to publish message: %v", err),
			Timestamp: time.Now(),
		})
		return
	}

	respondJSON(w, http.StatusOK, dto.Response{
		Success:   true,
		Message:   fmt.Sprintf("Message published to topic: %s", req.Topic),
		Timestamp: time.Now(),
	})
}

// ===== Container Update Handlers =====

// handleDockerCheckUpdates godoc
//
//	@Summary		Check all containers for updates
//	@Description	Pull latest images and check if any containers have updates available
//	@Tags			Docker
//	@Produce		json
//	@Success		200	{object}	dto.ContainerUpdatesResult	"Container update status"
//	@Failure		500	{object}	dto.Response				"Failed to check updates"
//	@Router			/docker/updates [get]
func (s *Server) handleDockerCheckUpdates(w http.ResponseWriter, _ *http.Request) {
	logger.Info("API: Checking all containers for updates")

	controller := controllers.NewDockerController()
	defer controller.Close() //nolint:errcheck

	result, err := controller.CheckAllContainerUpdates()
	if err != nil {
		logger.Error("API: Failed to check container updates: %v", err)
		respondJSON(w, http.StatusInternalServerError, dto.Response{
			Success:   false,
			Message:   fmt.Sprintf("Failed to check for updates: %v", err),
			Timestamp: time.Now(),
		})
		return
	}

	respondJSON(w, http.StatusOK, result)
}

// handleDockerCheckUpdate godoc
//
//	@Summary		Check a specific container for updates
//	@Description	Pull latest image and check if a specific container has an update available
//	@Tags			Docker
//	@Produce		json
//	@Param			id	path		string						true	"Container ID or name"
//	@Success		200	{object}	dto.ContainerUpdateInfo		"Container update status"
//	@Failure		400	{object}	dto.Response				"Invalid container reference"
//	@Failure		500	{object}	dto.Response				"Failed to check update"
//	@Router			/docker/{id}/check-update [get]
func (s *Server) handleDockerCheckUpdate(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	containerRef := vars["id"]

	if err := lib.ValidateContainerRef(containerRef); err != nil {
		respondJSON(w, http.StatusBadRequest, dto.Response{
			Success:   false,
			Message:   err.Error(),
			Timestamp: time.Now(),
		})
		return
	}

	controller := controllers.NewDockerController()
	defer controller.Close() //nolint:errcheck

	result, err := controller.CheckContainerUpdate(containerRef)
	if err != nil {
		logger.Error("API: Failed to check container update for %s: %v", containerRef, err)
		respondJSON(w, http.StatusInternalServerError, dto.Response{
			Success:   false,
			Message:   fmt.Sprintf("Failed to check for update: %v", err),
			Timestamp: time.Now(),
		})
		return
	}

	respondJSON(w, http.StatusOK, result)
}

// handleDockerSize godoc
//
//	@Summary		Get container size
//	@Description	Get size information for a specific Docker container
//	@Tags			Docker
//	@Produce		json
//	@Param			id	path		string					true	"Container ID or name"
//	@Success		200	{object}	dto.ContainerSizeInfo	"Container size information"
//	@Failure		400	{object}	dto.Response			"Invalid container reference"
//	@Failure		500	{object}	dto.Response			"Failed to get container size"
//	@Router			/docker/{id}/size [get]
func (s *Server) handleDockerSize(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	containerRef := vars["id"]

	if err := lib.ValidateContainerRef(containerRef); err != nil {
		respondJSON(w, http.StatusBadRequest, dto.Response{
			Success:   false,
			Message:   err.Error(),
			Timestamp: time.Now(),
		})
		return
	}

	controller := controllers.NewDockerController()
	defer controller.Close() //nolint:errcheck

	result, err := controller.GetContainerSize(containerRef)
	if err != nil {
		logger.Error("API: Failed to get container size for %s: %v", containerRef, err)
		respondJSON(w, http.StatusInternalServerError, dto.Response{
			Success:   false,
			Message:   fmt.Sprintf("Failed to get container size: %v", err),
			Timestamp: time.Now(),
		})
		return
	}

	respondJSON(w, http.StatusOK, result)
}

// handleDockerUpdate godoc
//
//	@Summary		Update a specific container
//	@Description	Pull latest image and recreate the container with the updated image
//	@Tags			Docker
//	@Produce		json
//	@Param			id	path		string						true	"Container ID or name"
//	@Success		200	{object}	dto.ContainerUpdateResult	"Container update result"
//	@Failure		400	{object}	dto.Response				"Invalid container reference"
//	@Failure		500	{object}	dto.Response				"Failed to update container"
//	@Router			/docker/{id}/update [post]
func (s *Server) handleDockerUpdate(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	containerRef := vars["id"]

	if err := lib.ValidateContainerRef(containerRef); err != nil {
		respondJSON(w, http.StatusBadRequest, dto.Response{
			Success:   false,
			Message:   err.Error(),
			Timestamp: time.Now(),
		})
		return
	}

	// Check for force parameter
	force := r.URL.Query().Get("force") == "true"

	controller := controllers.NewDockerController()
	defer controller.Close() //nolint:errcheck

	result, err := controller.UpdateContainer(containerRef, force)
	if err != nil {
		logger.Error("API: Failed to update container %s: %v", containerRef, err)
		respondJSON(w, http.StatusInternalServerError, dto.Response{
			Success:   false,
			Message:   fmt.Sprintf("Failed to update container: %v", err),
			Timestamp: time.Now(),
		})
		return
	}

	respondJSON(w, http.StatusOK, result)
}

// handleDockerUpdateAll godoc
//
//	@Summary		Update all containers
//	@Description	Check all containers for updates and update those that have updates available
//	@Tags			Docker
//	@Produce		json
//	@Success		200	{object}	dto.ContainerBulkUpdateResult	"Bulk update results"
//	@Failure		500	{object}	dto.Response					"Failed to update containers"
//	@Router			/docker/update-all [post]
func (s *Server) handleDockerUpdateAll(w http.ResponseWriter, _ *http.Request) {
	logger.Info("API: Updating all containers")

	controller := controllers.NewDockerController()
	defer controller.Close() //nolint:errcheck

	result, err := controller.UpdateAllContainers()
	if err != nil {
		logger.Error("API: Failed to update all containers: %v", err)
		respondJSON(w, http.StatusInternalServerError, dto.Response{
			Success:   false,
			Message:   fmt.Sprintf("Failed to update containers: %v", err),
			Timestamp: time.Now(),
		})
		return
	}

	respondJSON(w, http.StatusOK, result)
}

// ===== Plugin Update Handlers =====

// handlePluginCheckUpdates godoc
//
//	@Summary		Check for plugin updates
//	@Description	Check all installed plugins for available updates
//	@Tags			Plugins
//	@Produce		json
//	@Success		200	{array}		dto.PluginInfo	"Plugins with available updates"
//	@Failure		500	{object}	dto.Response	"Failed to check updates"
//	@Router			/plugins/check-updates [get]
func (s *Server) handlePluginCheckUpdates(w http.ResponseWriter, _ *http.Request) {
	logger.Info("API: Checking for plugin updates")

	controller := controllers.NewPluginController()
	updates, err := controller.CheckPluginUpdates()
	if err != nil {
		logger.Error("API: Failed to check plugin updates: %v", err)
		respondJSON(w, http.StatusInternalServerError, dto.Response{
			Success:   false,
			Message:   fmt.Sprintf("Failed to check for plugin updates: %v", err),
			Timestamp: time.Now(),
		})
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"plugins_with_updates": updates,
		"count":                len(updates),
		"timestamp":            time.Now(),
	})
}

// handlePluginUpdate godoc
//
//	@Summary		Update a specific plugin
//	@Description	Update a specific plugin by name
//	@Tags			Plugins
//	@Produce		json
//	@Param			name	path		string			true	"Plugin name"
//	@Success		200		{object}	dto.Response	"Plugin updated"
//	@Failure		400		{object}	dto.Response	"Invalid plugin name"
//	@Failure		500		{object}	dto.Response	"Failed to update plugin"
//	@Router			/plugins/{name}/update [post]
func (s *Server) handlePluginUpdate(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	pluginName := vars["name"]

	if err := lib.ValidatePluginName(pluginName); err != nil {
		respondJSON(w, http.StatusBadRequest, dto.Response{
			Success:   false,
			Message:   err.Error(),
			Timestamp: time.Now(),
		})
		return
	}

	controller := controllers.NewPluginController()
	err := controller.UpdatePlugin(pluginName)
	if err != nil {
		logger.Error("API: Failed to update plugin %s: %v", pluginName, err)
		respondJSON(w, http.StatusInternalServerError, dto.Response{
			Success:   false,
			Message:   fmt.Sprintf("Failed to update plugin: %v", err),
			Timestamp: time.Now(),
		})
		return
	}

	respondJSON(w, http.StatusOK, dto.Response{
		Success:   true,
		Message:   fmt.Sprintf("Plugin %s updated successfully", pluginName),
		Timestamp: time.Now(),
	})
}

// handlePluginUpdateAll godoc
//
//	@Summary		Update all plugins
//	@Description	Update all plugins that have updates available
//	@Tags			Plugins
//	@Produce		json
//	@Success		200	{object}	dto.PluginBulkUpdateResult	"Plugin update results"
//	@Failure		500	{object}	dto.Response				"Failed to update plugins"
//	@Router			/plugins/update-all [post]
func (s *Server) handlePluginUpdateAll(w http.ResponseWriter, _ *http.Request) {
	logger.Info("API: Updating all plugins")

	controller := controllers.NewPluginController()
	results, err := controller.UpdateAllPlugins()
	if err != nil {
		logger.Error("API: Failed to update all plugins: %v", err)
		respondJSON(w, http.StatusInternalServerError, dto.Response{
			Success:   false,
			Message:   fmt.Sprintf("Failed to update plugins: %v", err),
			Timestamp: time.Now(),
		})
		return
	}

	succeeded := 0
	failed := 0
	for _, r := range results {
		if r.Success {
			succeeded++
		} else {
			failed++
		}
	}

	respondJSON(w, http.StatusOK, dto.PluginBulkUpdateResult{
		Results:   results,
		Succeeded: succeeded,
		Failed:    failed,
		Timestamp: time.Now(),
	})
}

// ===== VM Clone & Snapshot Handlers =====

// handleVMClone godoc
//
//	@Summary		Clone a virtual machine
//	@Description	Clone a VM including disk images (source VM must be shut off)
//	@Tags			VMs
//	@Produce		json
//	@Param			name		path		string			true	"Source VM name"
//	@Param			clone_name	query		string			true	"Name for the cloned VM"
//	@Success		200			{object}	dto.Response	"VM cloned successfully"
//	@Failure		400			{object}	dto.Response	"Invalid parameters"
//	@Failure		500			{object}	dto.Response	"Failed to clone VM"
//	@Router			/vm/{name}/clone [post]
func (s *Server) handleVMClone(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	vmName := vars["name"]

	if err := lib.ValidateVMName(vmName); err != nil {
		respondJSON(w, http.StatusBadRequest, dto.Response{
			Success:   false,
			Message:   err.Error(),
			Timestamp: time.Now(),
		})
		return
	}

	cloneName := r.URL.Query().Get("clone_name")
	if cloneName == "" {
		respondJSON(w, http.StatusBadRequest, dto.Response{
			Success:   false,
			Message:   "clone_name query parameter is required",
			Timestamp: time.Now(),
		})
		return
	}

	if err := lib.ValidateVMName(cloneName); err != nil {
		respondJSON(w, http.StatusBadRequest, dto.Response{
			Success:   false,
			Message:   fmt.Sprintf("Invalid clone name: %v", err),
			Timestamp: time.Now(),
		})
		return
	}

	controller := controllers.NewVMController()
	err := controller.CloneVM(vmName, cloneName)
	if err != nil {
		logger.Error("API: Failed to clone VM %s: %v", vmName, err)
		respondJSON(w, http.StatusInternalServerError, dto.Response{
			Success:   false,
			Message:   fmt.Sprintf("Failed to clone VM: %v", err),
			Timestamp: time.Now(),
		})
		return
	}

	respondJSON(w, http.StatusOK, dto.Response{
		Success:   true,
		Message:   fmt.Sprintf("VM '%s' cloned as '%s'", vmName, cloneName),
		Timestamp: time.Now(),
	})
}

// handleVMCreateSnapshot godoc
//
//	@Summary		Create a VM snapshot
//	@Description	Create a snapshot of a virtual machine
//	@Tags			VMs
//	@Produce		json
//	@Param			name			path		string			true	"VM name"
//	@Param			snapshot_name	query		string			true	"Snapshot name"
//	@Param			description		query		string			false	"Snapshot description"
//	@Success		200				{object}	dto.Response	"Snapshot created"
//	@Failure		400				{object}	dto.Response	"Invalid parameters"
//	@Failure		500				{object}	dto.Response	"Failed to create snapshot"
//	@Router			/vm/{name}/snapshot [post]
func (s *Server) handleVMCreateSnapshot(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	vmName := vars["name"]

	if err := lib.ValidateVMName(vmName); err != nil {
		respondJSON(w, http.StatusBadRequest, dto.Response{
			Success:   false,
			Message:   err.Error(),
			Timestamp: time.Now(),
		})
		return
	}

	snapshotName := r.URL.Query().Get("snapshot_name")
	if snapshotName == "" {
		// Auto-generate snapshot name
		snapshotName = fmt.Sprintf("snapshot-%d", time.Now().Unix())
	} else {
		if err := lib.ValidateSnapshotName(snapshotName); err != nil {
			respondJSON(w, http.StatusBadRequest, dto.Response{
				Success:   false,
				Message:   fmt.Sprintf("Invalid snapshot name: %v", err),
				Timestamp: time.Now(),
			})
			return
		}
	}

	description := r.URL.Query().Get("description")

	controller := controllers.NewVMController()
	err := controller.CreateSnapshot(vmName, snapshotName, description)
	if err != nil {
		logger.Error("API: Failed to create snapshot for VM %s: %v", vmName, err)
		respondJSON(w, http.StatusInternalServerError, dto.Response{
			Success:   false,
			Message:   fmt.Sprintf("Failed to create snapshot: %v", err),
			Timestamp: time.Now(),
		})
		return
	}

	respondJSON(w, http.StatusOK, dto.Response{
		Success:   true,
		Message:   fmt.Sprintf("Snapshot '%s' created for VM '%s'", snapshotName, vmName),
		Timestamp: time.Now(),
	})
}

// handleVMListSnapshots godoc
//
//	@Summary		List VM snapshots
//	@Description	List all snapshots for a virtual machine
//	@Tags			VMs
//	@Produce		json
//	@Param			name	path		string				true	"VM name"
//	@Success		200		{object}	dto.VMSnapshotList	"Snapshot list"
//	@Failure		400		{object}	dto.Response		"Invalid VM name"
//	@Failure		500		{object}	dto.Response		"Failed to list snapshots"
//	@Router			/vm/{name}/snapshots [get]
func (s *Server) handleVMListSnapshots(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	vmName := vars["name"]

	if err := lib.ValidateVMName(vmName); err != nil {
		respondJSON(w, http.StatusBadRequest, dto.Response{
			Success:   false,
			Message:   err.Error(),
			Timestamp: time.Now(),
		})
		return
	}

	controller := controllers.NewVMController()
	result, err := controller.ListSnapshots(vmName)
	if err != nil {
		logger.Error("API: Failed to list snapshots for VM %s: %v", vmName, err)
		respondJSON(w, http.StatusInternalServerError, dto.Response{
			Success:   false,
			Message:   fmt.Sprintf("Failed to list snapshots: %v", err),
			Timestamp: time.Now(),
		})
		return
	}

	respondJSON(w, http.StatusOK, result)
}

// handleVMDeleteSnapshot godoc
//
//	@Summary		Delete a VM snapshot
//	@Description	Delete a specific snapshot of a virtual machine
//	@Tags			VMs
//	@Produce		json
//	@Param			name			path		string			true	"VM name"
//	@Param			snapshot_name	path		string			true	"Snapshot name"
//	@Success		200				{object}	dto.Response	"Snapshot deleted"
//	@Failure		400				{object}	dto.Response	"Invalid parameters"
//	@Failure		500				{object}	dto.Response	"Failed to delete snapshot"
//	@Router			/vm/{name}/snapshots/{snapshot_name} [delete]
func (s *Server) handleVMDeleteSnapshot(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	vmName := vars["name"]
	snapshotName := vars["snapshot_name"]

	if err := lib.ValidateVMName(vmName); err != nil {
		respondJSON(w, http.StatusBadRequest, dto.Response{
			Success:   false,
			Message:   err.Error(),
			Timestamp: time.Now(),
		})
		return
	}

	if err := lib.ValidateSnapshotName(snapshotName); err != nil {
		respondJSON(w, http.StatusBadRequest, dto.Response{
			Success:   false,
			Message:   fmt.Sprintf("Invalid snapshot name: %v", err),
			Timestamp: time.Now(),
		})
		return
	}

	controller := controllers.NewVMController()
	err := controller.DeleteSnapshot(vmName, snapshotName)
	if err != nil {
		logger.Error("API: Failed to delete snapshot %s for VM %s: %v", snapshotName, vmName, err)
		respondJSON(w, http.StatusInternalServerError, dto.Response{
			Success:   false,
			Message:   fmt.Sprintf("Failed to delete snapshot: %v", err),
			Timestamp: time.Now(),
		})
		return
	}

	respondJSON(w, http.StatusOK, dto.Response{
		Success:   true,
		Message:   fmt.Sprintf("Snapshot '%s' deleted from VM '%s'", snapshotName, vmName),
		Timestamp: time.Now(),
	})
}

// handleVMRestoreSnapshot godoc
//
//	@Summary		Restore a VM snapshot
//	@Description	Restore a virtual machine to a previously created snapshot. WARNING: This reverts the VM to the snapshot state.
//	@Tags			VMs
//	@Produce		json
//	@Param			name			path		string			true	"VM name"
//	@Param			snapshot_name	path		string			true	"Snapshot name"
//	@Success		200				{object}	dto.Response	"Snapshot restored"
//	@Failure		400				{object}	dto.Response	"Invalid parameters"
//	@Failure		500				{object}	dto.Response	"Failed to restore snapshot"
//	@Router			/vm/{name}/snapshots/{snapshot_name}/restore [post]
func (s *Server) handleVMRestoreSnapshot(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	vmName := vars["name"]
	snapshotName := vars["snapshot_name"]

	if err := lib.ValidateVMName(vmName); err != nil {
		respondJSON(w, http.StatusBadRequest, dto.Response{
			Success:   false,
			Message:   err.Error(),
			Timestamp: time.Now(),
		})
		return
	}

	if err := lib.ValidateSnapshotName(snapshotName); err != nil {
		respondJSON(w, http.StatusBadRequest, dto.Response{
			Success:   false,
			Message:   fmt.Sprintf("Invalid snapshot name: %v", err),
			Timestamp: time.Now(),
		})
		return
	}

	controller := controllers.NewVMController()
	err := controller.RestoreSnapshot(vmName, snapshotName)
	if err != nil {
		logger.Error("API: Failed to restore snapshot %s for VM %s: %v", snapshotName, vmName, err)
		respondJSON(w, http.StatusInternalServerError, dto.Response{
			Success:   false,
			Message:   fmt.Sprintf("Failed to restore snapshot: %v", err),
			Timestamp: time.Now(),
		})
		return
	}

	respondJSON(w, http.StatusOK, dto.Response{
		Success:   true,
		Message:   fmt.Sprintf("Snapshot '%s' restored for VM '%s'", snapshotName, vmName),
		Timestamp: time.Now(),
	})
}

// handleDockerLogs godoc
//
//	@Summary		Get Docker container logs
//	@Description	Retrieve stdout/stderr logs from a specific Docker container (equivalent to docker logs)
//	@Tags			Docker
//	@Produce		json
//	@Param			id			path		string				true	"Container ID or name"
//	@Param			tail		query		int					false	"Number of recent lines (default: 100, max: 5000)"
//	@Param			since		query		string				false	"Only logs since this timestamp (RFC3339)"
//	@Param			timestamps	query		bool				false	"Include timestamps in output"
//	@Success		200			{object}	dto.ContainerLogs	"Container logs"
//	@Failure		400			{object}	dto.Response		"Invalid container reference"
//	@Failure		500			{object}	dto.Response		"Failed to get logs"
//	@Router			/docker/{id}/logs [get]
func (s *Server) handleDockerLogs(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	containerRef := vars["id"]

	if err := lib.ValidateContainerRef(containerRef); err != nil {
		respondJSON(w, http.StatusBadRequest, dto.Response{
			Success:   false,
			Message:   err.Error(),
			Timestamp: time.Now(),
		})
		return
	}

	tail := 100
	if tailStr := r.URL.Query().Get("tail"); tailStr != "" {
		if v, err := strconv.Atoi(tailStr); err == nil && v > 0 {
			tail = v
		}
	}

	since := r.URL.Query().Get("since")
	timestamps := r.URL.Query().Get("timestamps") == "true"

	controller := controllers.NewDockerController()
	defer controller.Close() //nolint:errcheck

	result, err := controller.ContainerLogs(containerRef, tail, since, timestamps)
	if err != nil {
		logger.Error("API: Failed to get logs for container %s: %v", containerRef, err)
		respondJSON(w, http.StatusInternalServerError, dto.Response{
			Success:   false,
			Message:   fmt.Sprintf("Failed to get container logs: %v", err),
			Timestamp: time.Now(),
		})
		return
	}

	respondJSON(w, http.StatusOK, result)
}

// ===== Service Management Handlers =====

// handleServiceAction godoc
//
//	@Summary		Control an Unraid service
//	@Description	Start, stop, or restart an Unraid system service
//	@Tags			Services
//	@Produce		json
//	@Param			name	path		string			true	"Service name (docker, libvirt, smb, nfs, ftp, sshd, nginx, syslog, ntpd, avahi, wireguard)"
//	@Param			action	path		string			true	"Action (start, stop, restart)"
//	@Success		200		{object}	dto.Response	"Service action completed"
//	@Failure		400		{object}	dto.Response	"Invalid service or action"
//	@Failure		500		{object}	dto.Response	"Failed to execute action"
//	@Router			/services/{name}/{action} [post]
func (s *Server) handleServiceAction(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	serviceName := vars["name"]
	action := vars["action"]

	if err := lib.ValidateServiceName(serviceName); err != nil {
		respondJSON(w, http.StatusBadRequest, dto.Response{
			Success:   false,
			Message:   err.Error(),
			Timestamp: time.Now(),
		})
		return
	}

	if action != "start" && action != "stop" && action != "restart" {
		respondJSON(w, http.StatusBadRequest, dto.Response{
			Success:   false,
			Message:   "Invalid action: must be start, stop, or restart",
			Timestamp: time.Now(),
		})
		return
	}

	controller := controllers.NewServiceController()
	var err error
	switch action {
	case "start":
		err = controller.StartService(serviceName)
	case "stop":
		err = controller.StopService(serviceName)
	case "restart":
		err = controller.RestartService(serviceName)
	}

	if err != nil {
		logger.Error("API: Failed to %s service %s: %v", action, serviceName, err)
		respondJSON(w, http.StatusInternalServerError, dto.Response{
			Success:   false,
			Message:   fmt.Sprintf("Failed to %s service %s: %v", action, serviceName, err),
			Timestamp: time.Now(),
		})
		return
	}

	respondJSON(w, http.StatusOK, dto.Response{
		Success:   true,
		Message:   fmt.Sprintf("Service %s %sed successfully", serviceName, action),
		Timestamp: time.Now(),
	})
}

// handleServiceList godoc
//
//	@Summary		List available services
//	@Description	List all managed Unraid system services and their status
//	@Tags			Services
//	@Produce		json
//	@Success		200	{object}	map[string]interface{}	"Service list with status"
//	@Router			/services [get]
func (s *Server) handleServiceList(w http.ResponseWriter, _ *http.Request) {
	serviceNames := controllers.ValidServiceNames()
	controller := controllers.NewServiceController()

	type serviceInfo struct {
		Name    string `json:"name"`
		Running bool   `json:"running"`
	}

	services := make([]serviceInfo, 0)
	for _, name := range serviceNames {
		running, _ := controller.GetServiceStatus(name)
		services = append(services, serviceInfo{Name: name, Running: running})
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"services":  services,
		"count":     len(services),
		"timestamp": time.Now(),
	})
}

// ===== Process Handlers =====

// handleProcessList godoc
//
//	@Summary		List running processes
//	@Description	Get all running processes on the Unraid server
//	@Tags			System
//	@Produce		json
//	@Param			sort_by	query		string				false	"Sort by: cpu, memory, or pid (default: cpu)"
//	@Param			limit	query		int					false	"Max processes to return (default: 50)"
//	@Success		200		{object}	dto.ProcessList		"Process list"
//	@Failure		500		{object}	dto.Response		"Failed to list processes"
//	@Router			/processes [get]
func (s *Server) handleProcessList(w http.ResponseWriter, r *http.Request) {
	sortBy := r.URL.Query().Get("sort_by")
	if sortBy == "" {
		sortBy = "cpu"
	}

	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := fmt.Sscanf(l, "%d", &limit); parsed != 1 || err != nil {
			limit = 50
		}
	}
	if limit > 500 {
		limit = 500
	}

	controller := controllers.NewProcessController()
	result, err := controller.ListProcesses(sortBy, limit)
	if err != nil {
		logger.Error("API: Failed to list processes: %v", err)
		respondJSON(w, http.StatusInternalServerError, dto.Response{
			Success:   false,
			Message:   fmt.Sprintf("Failed to list processes: %v", err),
			Timestamp: time.Now(),
		})
		return
	}

	respondJSON(w, http.StatusOK, result)
}

// =============================================================================
// Alert Rule Management Handlers
// =============================================================================

// handleListAlertRules godoc
//
//	@Summary		List all alert rules
//	@Description	Retrieve all configured alert rules
//	@Tags			Alerts
//	@Produce		json
//	@Success		200	{array}	dto.AlertRule	"List of alert rules"
//	@Router			/alerts/rules [get]
func (s *Server) handleListAlertRules(w http.ResponseWriter, _ *http.Request) {
	if s.alertStore == nil {
		respondWithError(w, http.StatusServiceUnavailable, "Alerting engine not initialized")
		return
	}
	respondJSON(w, http.StatusOK, s.alertStore.GetRules())
}

// handleGetAlertRule godoc
//
//	@Summary		Get a specific alert rule
//	@Description	Retrieve a single alert rule by ID
//	@Tags			Alerts
//	@Produce		json
//	@Param			id	path		string	true	"Alert rule ID"
//	@Success		200	{object}	dto.AlertRule	"Alert rule"
//	@Failure		404	{object}	map[string]string	"Rule not found"
//	@Router			/alerts/rules/{id} [get]
func (s *Server) handleGetAlertRule(w http.ResponseWriter, r *http.Request) {
	if s.alertStore == nil {
		respondWithError(w, http.StatusServiceUnavailable, "Alerting engine not initialized")
		return
	}

	id := mux.Vars(r)["id"]
	rule, err := s.alertStore.GetRule(id)
	if err != nil {
		respondJSON(w, http.StatusNotFound, dto.Response{
			Success:   false,
			Message:   err.Error(),
			Timestamp: time.Now(),
		})
		return
	}
	respondJSON(w, http.StatusOK, rule)
}

// handleCreateAlertRule godoc
//
//	@Summary		Create a new alert rule
//	@Description	Create a new alert rule with an expr-lang expression
//	@Tags			Alerts
//	@Accept			json
//	@Produce		json
//	@Param			rule	body		dto.AlertRule	true	"Alert rule to create"
//	@Success		201		{object}	dto.Response	"Rule created successfully"
//	@Failure		400		{object}	map[string]string	"Invalid request"
//	@Failure		500		{object}	map[string]string	"Internal error"
//	@Router			/alerts/rules [post]
func (s *Server) handleCreateAlertRule(w http.ResponseWriter, r *http.Request) {
	if s.alertStore == nil || s.alertEngine == nil {
		respondWithError(w, http.StatusServiceUnavailable, "Alerting engine not initialized")
		return
	}

	var rule dto.AlertRule
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Validate required fields
	if rule.ID == "" {
		respondWithError(w, http.StatusBadRequest, "Rule ID is required")
		return
	}
	if rule.Name == "" {
		respondWithError(w, http.StatusBadRequest, "Rule name is required")
		return
	}
	if rule.Expression == "" {
		respondWithError(w, http.StatusBadRequest, "Rule expression is required")
		return
	}
	if rule.Severity == "" {
		rule.Severity = "warning"
	}
	if rule.Severity != "info" && rule.Severity != "warning" && rule.Severity != "critical" {
		respondWithError(w, http.StatusBadRequest, "Severity must be 'info', 'warning', or 'critical'")
		return
	}

	if err := s.alertStore.CreateRule(rule); err != nil {
		respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Recompile rules so the engine picks up the new rule
	s.alertEngine.RecompileRules()

	respondJSON(w, http.StatusCreated, dto.Response{
		Success:   true,
		Message:   fmt.Sprintf("Alert rule '%s' created successfully", rule.Name),
		Timestamp: time.Now(),
	})
}

// handleUpdateAlertRule godoc
//
//	@Summary		Update an existing alert rule
//	@Description	Update an alert rule by ID
//	@Tags			Alerts
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string		true	"Alert rule ID"
//	@Param			rule	body		dto.AlertRule	true	"Updated alert rule"
//	@Success		200		{object}	dto.Response	"Rule updated successfully"
//	@Failure		400		{object}	map[string]string	"Invalid request"
//	@Failure		404		{object}	map[string]string	"Rule not found"
//	@Router			/alerts/rules/{id} [put]
func (s *Server) handleUpdateAlertRule(w http.ResponseWriter, r *http.Request) {
	if s.alertStore == nil || s.alertEngine == nil {
		respondWithError(w, http.StatusServiceUnavailable, "Alerting engine not initialized")
		return
	}

	id := mux.Vars(r)["id"]

	var rule dto.AlertRule
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Ensure the ID from the URL is used
	rule.ID = id

	if rule.Name == "" {
		respondWithError(w, http.StatusBadRequest, "Rule name is required")
		return
	}
	if rule.Expression == "" {
		respondWithError(w, http.StatusBadRequest, "Rule expression is required")
		return
	}
	if rule.Severity != "" && rule.Severity != "info" && rule.Severity != "warning" && rule.Severity != "critical" {
		respondWithError(w, http.StatusBadRequest, "Severity must be 'info', 'warning', or 'critical'")
		return
	}

	if err := s.alertStore.UpdateRule(rule); err != nil {
		respondWithError(w, http.StatusNotFound, err.Error())
		return
	}

	s.alertEngine.RecompileRules()

	respondJSON(w, http.StatusOK, dto.Response{
		Success:   true,
		Message:   fmt.Sprintf("Alert rule '%s' updated successfully", rule.Name),
		Timestamp: time.Now(),
	})
}

// handleDeleteAlertRule godoc
//
//	@Summary		Delete an alert rule
//	@Description	Delete an alert rule by ID
//	@Tags			Alerts
//	@Produce		json
//	@Param			id	path		string	true	"Alert rule ID"
//	@Success		200	{object}	dto.Response	"Rule deleted successfully"
//	@Failure		404	{object}	map[string]string	"Rule not found"
//	@Router			/alerts/rules/{id} [delete]
func (s *Server) handleDeleteAlertRule(w http.ResponseWriter, r *http.Request) {
	if s.alertStore == nil || s.alertEngine == nil {
		respondWithError(w, http.StatusServiceUnavailable, "Alerting engine not initialized")
		return
	}

	id := mux.Vars(r)["id"]

	if err := s.alertStore.DeleteRule(id); err != nil {
		respondJSON(w, http.StatusNotFound, dto.Response{
			Success:   false,
			Message:   err.Error(),
			Timestamp: time.Now(),
		})
		return
	}

	s.alertEngine.RecompileRules()

	respondJSON(w, http.StatusOK, dto.Response{
		Success:   true,
		Message:   fmt.Sprintf("Alert rule '%s' deleted successfully", id),
		Timestamp: time.Now(),
	})
}

// handleAlertStatus godoc
//
//	@Summary		Get alert status
//	@Description	Get the current evaluation status of all enabled alert rules
//	@Tags			Alerts
//	@Produce		json
//	@Success		200	{object}	dto.AlertsStatusResponse	"Alert statuses"
//	@Router			/alerts/status [get]
func (s *Server) handleAlertStatus(w http.ResponseWriter, _ *http.Request) {
	if s.alertEngine == nil {
		respondWithError(w, http.StatusServiceUnavailable, "Alerting engine not initialized")
		return
	}

	respondJSON(w, http.StatusOK, dto.AlertsStatusResponse{
		Statuses: s.alertEngine.GetStatuses(),
	})
}

// handleAlertHistory godoc
//
//	@Summary		Get alert history
//	@Description	Get recent alert events (last 100)
//	@Tags			Alerts
//	@Produce		json
//	@Success		200	{object}	dto.AlertHistoryResponse	"Alert event history"
//	@Router			/alerts/history [get]
func (s *Server) handleAlertHistory(w http.ResponseWriter, _ *http.Request) {
	if s.alertEngine == nil {
		respondWithError(w, http.StatusServiceUnavailable, "Alerting engine not initialized")
		return
	}

	events := s.alertEngine.GetHistory()
	respondJSON(w, http.StatusOK, dto.AlertHistoryResponse{
		Events: events,
		Total:  len(events),
	})
}

// handleFiringAlerts godoc
//
//	@Summary		Get firing alerts
//	@Description	Get only alert rules currently in the firing state
//	@Tags			Alerts
//	@Produce		json
//	@Success		200	{array}	dto.AlertStatus	"Firing alerts"
//	@Router			/alerts/firing [get]
func (s *Server) handleFiringAlerts(w http.ResponseWriter, _ *http.Request) {
	if s.alertEngine == nil {
		respondWithError(w, http.StatusServiceUnavailable, "Alerting engine not initialized")
		return
	}

	respondJSON(w, http.StatusOK, s.alertEngine.GetFiringAlerts())
}

// ============================================================================
// Health Check / Watchdog Handlers
// ============================================================================

// handleListHealthChecks godoc
//
//	@Summary		List health checks
//	@Description	Get all configured health checks
//	@Tags			HealthChecks
//	@Produce		json
//	@Success		200	{array}	dto.HealthCheck	"List of health checks"
//	@Router			/healthchecks [get]
func (s *Server) handleListHealthChecks(w http.ResponseWriter, _ *http.Request) {
	if s.watchdogStore == nil {
		respondWithError(w, http.StatusServiceUnavailable, "Watchdog not initialized")
		return
	}

	respondJSON(w, http.StatusOK, s.watchdogStore.GetChecks())
}

// handleGetHealthCheck godoc
//
//	@Summary		Get health check
//	@Description	Get a specific health check by ID
//	@Tags			HealthChecks
//	@Produce		json
//	@Param			id	path		string			true	"Health check ID"
//	@Success		200	{object}	dto.HealthCheck	"Health check details"
//	@Failure		404	{object}	dto.Response	"Health check not found"
//	@Router			/healthchecks/{id} [get]
func (s *Server) handleGetHealthCheck(w http.ResponseWriter, r *http.Request) {
	if s.watchdogStore == nil {
		respondWithError(w, http.StatusServiceUnavailable, "Watchdog not initialized")
		return
	}

	id := mux.Vars(r)["id"]
	check, err := s.watchdogStore.GetCheck(id)
	if err != nil {
		respondWithError(w, http.StatusNotFound, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, check)
}

// handleCreateHealthCheck godoc
//
//	@Summary		Create health check
//	@Description	Create a new health check probe
//	@Tags			HealthChecks
//	@Accept			json
//	@Produce		json
//	@Param			check	body		dto.HealthCheck	true	"Health check configuration"
//	@Success		201		{object}	dto.Response	"Created"
//	@Failure		400		{object}	dto.Response	"Invalid request"
//	@Router			/healthchecks [post]
func (s *Server) handleCreateHealthCheck(w http.ResponseWriter, r *http.Request) {
	if s.watchdogStore == nil {
		respondWithError(w, http.StatusServiceUnavailable, "Watchdog not initialized")
		return
	}

	var check dto.HealthCheck
	if err := json.NewDecoder(r.Body).Decode(&check); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid JSON: "+err.Error())
		return
	}

	if check.ID == "" {
		respondWithError(w, http.StatusBadRequest, "id is required")
		return
	}
	if check.Name == "" {
		respondWithError(w, http.StatusBadRequest, "name is required")
		return
	}
	if check.Type == "" {
		respondWithError(w, http.StatusBadRequest, "type is required (http, tcp, or container)")
		return
	}
	if check.Target == "" {
		respondWithError(w, http.StatusBadRequest, "target is required")
		return
	}

	if err := s.watchdogStore.CreateCheck(check); err != nil {
		respondWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	respondJSON(w, http.StatusCreated, dto.Response{Success: true, Message: "Health check created", Timestamp: time.Now()})
}

// handleUpdateHealthCheck godoc
//
//	@Summary		Update health check
//	@Description	Update an existing health check configuration
//	@Tags			HealthChecks
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string			true	"Health check ID"
//	@Param			check	body		dto.HealthCheck	true	"Updated health check configuration"
//	@Success		200		{object}	dto.Response	"Updated"
//	@Failure		400		{object}	dto.Response	"Invalid request"
//	@Failure		404		{object}	dto.Response	"Not found"
//	@Router			/healthchecks/{id} [put]
func (s *Server) handleUpdateHealthCheck(w http.ResponseWriter, r *http.Request) {
	if s.watchdogStore == nil {
		respondWithError(w, http.StatusServiceUnavailable, "Watchdog not initialized")
		return
	}

	id := mux.Vars(r)["id"]
	var check dto.HealthCheck
	if err := json.NewDecoder(r.Body).Decode(&check); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid JSON: "+err.Error())
		return
	}

	check.ID = id // URL ID takes precedence

	if err := s.watchdogStore.UpdateCheck(check); err != nil {
		respondWithError(w, http.StatusNotFound, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, dto.Response{Success: true, Message: "Health check updated", Timestamp: time.Now()})
}

// handleDeleteHealthCheck godoc
//
//	@Summary		Delete health check
//	@Description	Delete a health check by ID
//	@Tags			HealthChecks
//	@Produce		json
//	@Param			id	path		string		true	"Health check ID"
//	@Success		200	{object}	dto.Response	"Deleted"
//	@Failure		404	{object}	dto.Response	"Not found"
//	@Router			/healthchecks/{id} [delete]
func (s *Server) handleDeleteHealthCheck(w http.ResponseWriter, r *http.Request) {
	if s.watchdogStore == nil {
		respondWithError(w, http.StatusServiceUnavailable, "Watchdog not initialized")
		return
	}

	id := mux.Vars(r)["id"]

	if err := s.watchdogStore.DeleteCheck(id); err != nil {
		respondWithError(w, http.StatusNotFound, err.Error())
		return
	}

	// Clean up runner state for deleted check
	if s.watchdogRunner != nil {
		s.watchdogRunner.CleanupCheck(id)
	}

	respondJSON(w, http.StatusOK, dto.Response{Success: true, Message: "Health check deleted", Timestamp: time.Now()})
}

// handleHealthCheckStatus godoc
//
//	@Summary		Get health check statuses
//	@Description	Get the current status of all health checks
//	@Tags			HealthChecks
//	@Produce		json
//	@Success		200	{object}	dto.HealthChecksStatusResponse	"Health check statuses"
//	@Router			/healthchecks/status [get]
func (s *Server) handleHealthCheckStatus(w http.ResponseWriter, _ *http.Request) {
	if s.watchdogRunner == nil {
		respondWithError(w, http.StatusServiceUnavailable, "Watchdog not initialized")
		return
	}

	respondJSON(w, http.StatusOK, dto.HealthChecksStatusResponse{
		Checks: s.watchdogRunner.GetStatuses(),
	})
}

// handleHealthCheckHistory godoc
//
//	@Summary		Get health check history
//	@Description	Get recent health check state change events
//	@Tags			HealthChecks
//	@Produce		json
//	@Success		200	{object}	dto.HealthCheckHistoryResponse	"Health check events"
//	@Router			/healthchecks/history [get]
func (s *Server) handleHealthCheckHistory(w http.ResponseWriter, _ *http.Request) {
	if s.watchdogRunner == nil {
		respondWithError(w, http.StatusServiceUnavailable, "Watchdog not initialized")
		return
	}

	events := s.watchdogRunner.GetHistory()
	respondJSON(w, http.StatusOK, dto.HealthCheckHistoryResponse{
		Events: events,
	})
}

// handleRunHealthCheck godoc
//
//	@Summary		Run health check
//	@Description	Manually trigger a specific health check and return the result
//	@Tags			HealthChecks
//	@Produce		json
//	@Param			id	path		string					true	"Health check ID"
//	@Success		200	{object}	dto.HealthCheckStatus	"Check result"
//	@Failure		404	{object}	dto.Response			"Not found"
//	@Router			/healthchecks/{id}/run [post]
func (s *Server) handleRunHealthCheck(w http.ResponseWriter, r *http.Request) {
	if s.watchdogRunner == nil {
		respondWithError(w, http.StatusServiceUnavailable, "Watchdog not initialized")
		return
	}

	id := mux.Vars(r)["id"]

	status, err := s.watchdogRunner.RunSingleCheck(r.Context(), id)
	if err != nil {
		respondWithError(w, http.StatusNotFound, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, status)
}
