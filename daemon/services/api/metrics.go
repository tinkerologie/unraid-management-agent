package api

import (
	"fmt"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Prometheus metric definitions
var (
	// System metrics
	systemInfo = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "unraid_system_info",
			Help: "Unraid system information (always 1)",
		},
		[]string{"hostname", "version", "agent_version"},
	)
	systemUptime = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "unraid_system_uptime_seconds",
		Help: "System uptime in seconds",
	})
	cpuUsage = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "unraid_cpu_usage_percent",
		Help: "CPU usage percentage",
	})
	cpuTemperature = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "unraid_cpu_temperature_celsius",
		Help: "CPU temperature in Celsius",
	})
	cpuPowerWatts = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "unraid_cpu_power_watts",
		Help: "CPU package power consumption in watts (from Intel RAPL)",
	})
	dramPowerWatts = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "unraid_dram_power_watts",
		Help: "DRAM power consumption in watts (from Intel RAPL)",
	})
	memoryTotal = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "unraid_memory_total_bytes",
		Help: "Total memory in bytes",
	})
	memoryUsed = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "unraid_memory_used_bytes",
		Help: "Used memory in bytes",
	})
	memoryUsagePercent = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "unraid_memory_usage_percent",
		Help: "Memory usage percentage",
	})

	// Array metrics
	arrayState = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "unraid_array_state",
		Help: "Array state (1=started, 0=stopped)",
	})
	arrayTotalBytes = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "unraid_array_total_bytes",
		Help: "Total array capacity in bytes",
	})
	arrayUsedBytes = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "unraid_array_used_bytes",
		Help: "Used array space in bytes",
	})
	arrayFreeBytes = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "unraid_array_free_bytes",
		Help: "Free array space in bytes",
	})
	arrayUsagePercent = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "unraid_array_usage_percent",
		Help: "Array usage percentage",
	})
	parityValid = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "unraid_parity_valid",
		Help: "Parity validity (1=valid, 0=invalid)",
	})
	parityCheckRunning = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "unraid_parity_check_running",
		Help: "Parity check in progress (1=yes, 0=no)",
	})
	parityCheckProgress = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "unraid_parity_check_progress",
		Help: "Parity check progress percentage",
	})

	// Disk metrics
	diskTemperature = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "unraid_disk_temperature_celsius",
			Help: "Disk temperature in Celsius",
		},
		[]string{"disk", "device", "type"},
	)
	diskSizeBytes = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "unraid_disk_size_bytes",
			Help: "Disk size in bytes",
		},
		[]string{"disk", "device"},
	)
	diskUsedBytes = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "unraid_disk_used_bytes",
			Help: "Disk used space in bytes",
		},
		[]string{"disk", "device"},
	)
	diskFreeBytes = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "unraid_disk_free_bytes",
			Help: "Disk free space in bytes",
		},
		[]string{"disk", "device"},
	)
	diskStatus = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "unraid_disk_status",
			Help: "Disk status (1=healthy, 0=problem)",
		},
		[]string{"disk", "device", "status"},
	)
	diskStandby = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "unraid_disk_standby",
			Help: "Disk standby state (1=standby, 0=active)",
		},
		[]string{"disk", "device"},
	)
	diskSmartStatus = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "unraid_disk_smart_status",
			Help: "SMART status (1=passed, 0=failed)",
		},
		[]string{"disk", "device"},
	)

	// Docker metrics
	containerState = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "unraid_docker_container_state",
			Help: "Container state (1=running, 0=stopped)",
		},
		[]string{"name", "id", "image"},
	)
	containersTotal = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "unraid_docker_containers_total",
		Help: "Total number of containers",
	})
	containersRunning = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "unraid_docker_containers_running",
		Help: "Number of running containers",
	})

	// VM metrics
	vmState = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "unraid_vm_state",
			Help: "VM state (1=running, 0=stopped)",
		},
		[]string{"name", "uuid"},
	)
	vmsTotal = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "unraid_vms_total",
		Help: "Total number of VMs",
	})
	vmsRunning = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "unraid_vms_running",
		Help: "Number of running VMs",
	})

	// UPS metrics
	upsStatus = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "unraid_ups_status",
			Help: "UPS status (1=online, 0=on battery)",
		},
		[]string{"name", "model"},
	)
	upsBatteryCharge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "unraid_ups_battery_charge_percent",
			Help: "UPS battery charge percentage",
		},
		[]string{"name"},
	)
	upsLoad = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "unraid_ups_load_percent",
			Help: "UPS load percentage",
		},
		[]string{"name"},
	)
	upsRuntime = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "unraid_ups_runtime_seconds",
			Help: "UPS runtime remaining in seconds",
		},
		[]string{"name"},
	)

	// Share metrics
	shareUsedBytes = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "unraid_share_used_bytes",
			Help: "Share used space in bytes",
		},
		[]string{"name"},
	)
	sharesTotal = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "unraid_shares_total",
		Help: "Total number of shares",
	})

	// Network service metrics
	serviceEnabled = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "unraid_service_enabled",
			Help: "Service enabled state (1=enabled, 0=disabled)",
		},
		[]string{"service"},
	)
	serviceRunning = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "unraid_service_running",
			Help: "Service running state (1=running, 0=stopped)",
		},
		[]string{"service"},
	)

	// GPU metrics
	gpuTemperature = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "unraid_gpu_temperature_celsius",
			Help: "GPU temperature in Celsius",
		},
		[]string{"gpu", "name"},
	)
	gpuUtilization = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "unraid_gpu_utilization_percent",
			Help: "GPU utilization percentage",
		},
		[]string{"gpu", "name"},
	)
	gpuMemoryUsed = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "unraid_gpu_memory_used_bytes",
			Help: "GPU memory used in bytes",
		},
		[]string{"gpu", "name"},
	)
	gpuMemoryTotal = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "unraid_gpu_memory_total_bytes",
			Help: "GPU total memory in bytes",
		},
		[]string{"gpu", "name"},
	)
	gpuPowerWatts = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "unraid_gpu_power_watts",
			Help: "GPU power consumption in watts",
		},
		[]string{"gpu", "name"},
	)
)

// metricsRegistry is a custom registry for Unraid metrics
var metricsRegistry = prometheus.NewRegistry()

func init() {
	// Register all metrics with our custom registry
	metricsRegistry.MustRegister(
		// System
		systemInfo,
		systemUptime,
		cpuUsage,
		cpuTemperature,
		memoryTotal,
		memoryUsed,
		memoryUsagePercent,
		// Array
		arrayState,
		arrayTotalBytes,
		arrayUsedBytes,
		arrayFreeBytes,
		arrayUsagePercent,
		parityValid,
		parityCheckRunning,
		parityCheckProgress,
		// Disks
		diskTemperature,
		diskSizeBytes,
		diskUsedBytes,
		diskFreeBytes,
		diskStatus,
		diskStandby,
		diskSmartStatus,
		// Docker
		containerState,
		containersTotal,
		containersRunning,
		// VMs
		vmState,
		vmsTotal,
		vmsRunning,
		// UPS
		upsStatus,
		upsBatteryCharge,
		upsLoad,
		upsRuntime,
		// Shares
		shareUsedBytes,
		sharesTotal,
		// Services
		serviceEnabled,
		serviceRunning,
		// GPU
		gpuTemperature,
		gpuUtilization,
		gpuMemoryUsed,
		gpuMemoryTotal,
		gpuPowerWatts,
		// CPU Power
		cpuPowerWatts,
		dramPowerWatts,
	)
}

// updateMetrics updates all Prometheus metrics from the server's cache
func (s *Server) updateMetrics() {
	// Load all cache values atomically (lock-free reads)
	sysCache := s.systemCache.Load()
	arrCache := s.arrayCache.Load()
	disksSlice := s.GetDisksCache()
	dockerSlice := s.GetDockerCache()
	vmsSlice := s.GetVMsCache()
	upsVal := s.upsCache.Load()
	sharesSlice := s.GetSharesCache()
	gpuSlice := s.GetGPUCache()

	// Update system metrics
	if sysCache != nil {
		systemInfo.Reset()
		systemInfo.WithLabelValues(
			sysCache.Hostname,
			sysCache.Version,
			sysCache.AgentVersion,
		).Set(1)

		systemUptime.Set(float64(sysCache.Uptime))
		cpuUsage.Set(sysCache.CPUUsage)
		cpuTemperature.Set(sysCache.CPUTemp)
		memoryTotal.Set(float64(sysCache.RAMTotal))
		memoryUsed.Set(float64(sysCache.RAMUsed))
		memoryUsagePercent.Set(sysCache.RAMUsage)

		// CPU power from Intel RAPL
		if sysCache.CPUPowerWatts != nil {
			cpuPowerWatts.Set(*sysCache.CPUPowerWatts)
		} else {
			cpuPowerWatts.Set(0)
		}
		if sysCache.DRAMPowerWatts != nil {
			dramPowerWatts.Set(*sysCache.DRAMPowerWatts)
		} else {
			dramPowerWatts.Set(0)
		}
	}

	// Update array metrics
	if arrCache != nil {
		if arrCache.State == "STARTED" || arrCache.State == "Started" {
			arrayState.Set(1)
		} else {
			arrayState.Set(0)
		}
		arrayTotalBytes.Set(float64(arrCache.TotalBytes))
		// Calculate used bytes from total - free
		usedBytes := arrCache.TotalBytes - arrCache.FreeBytes
		arrayUsedBytes.Set(float64(usedBytes))
		arrayFreeBytes.Set(float64(arrCache.FreeBytes))
		arrayUsagePercent.Set(arrCache.UsedPercent)

		if arrCache.ParityValid {
			parityValid.Set(1)
		} else {
			parityValid.Set(0)
		}

		if arrCache.ParityCheckStatus != "" && arrCache.ParityCheckStatus != "IDLE" && arrCache.ParityCheckStatus != "idle" {
			parityCheckRunning.Set(1)
		} else {
			parityCheckRunning.Set(0)
		}
		parityCheckProgress.Set(arrCache.ParityCheckProgress)
	}

	// Update disk metrics
	if disksSlice != nil {
		// Reset disk metrics to clear stale entries
		diskTemperature.Reset()
		diskSizeBytes.Reset()
		diskUsedBytes.Reset()
		diskFreeBytes.Reset()
		diskStatus.Reset()
		diskStandby.Reset()
		diskSmartStatus.Reset()

		for _, disk := range disksSlice {
			// Determine disk type based on role (cache/pool disks are often SSDs)
			diskType := "HDD"
			if disk.Role == "cache" || disk.Role == "pool" {
				diskType = "SSD"
			}

			if disk.Temperature > 0 {
				diskTemperature.WithLabelValues(disk.Name, disk.Device, diskType).Set(disk.Temperature)
			}

			diskSizeBytes.WithLabelValues(disk.Name, disk.Device).Set(float64(disk.Size))
			diskUsedBytes.WithLabelValues(disk.Name, disk.Device).Set(float64(disk.Used))
			diskFreeBytes.WithLabelValues(disk.Name, disk.Device).Set(float64(disk.Free))

			// Status: 1 = healthy, 0 = problem
			statusValue := 1.0
			if disk.Status != "DISK_OK" && disk.Status != "OK" && disk.Status != "" {
				statusValue = 0.0
			}
			diskStatus.WithLabelValues(disk.Name, disk.Device, disk.Status).Set(statusValue)

			// Standby: 1 = standby, 0 = active
			standbyValue := 0.0
			if disk.SpinState == "standby" {
				standbyValue = 1.0
			}
			diskStandby.WithLabelValues(disk.Name, disk.Device).Set(standbyValue)

			// SMART status: 1 = passed, 0 = failed
			smartValue := 1.0
			if disk.SMARTStatus == "FAILED" {
				smartValue = 0.0
			}
			diskSmartStatus.WithLabelValues(disk.Name, disk.Device).Set(smartValue)
		}
	}

	// Update Docker metrics
	if dockerSlice != nil {
		containerState.Reset()
		running := 0
		for _, container := range dockerSlice {
			stateValue := 0.0
			if container.State == "running" {
				stateValue = 1.0
				running++
			}
			containerState.WithLabelValues(container.Name, container.ID, container.Image).Set(stateValue)
		}
		containersTotal.Set(float64(len(dockerSlice)))
		containersRunning.Set(float64(running))
	}

	// Update VM metrics
	if vmsSlice != nil {
		vmState.Reset()
		running := 0
		for _, vm := range vmsSlice {
			stateValue := 0.0
			if vm.State == "running" {
				stateValue = 1.0
				running++
			}
			vmState.WithLabelValues(vm.Name, vm.ID).Set(stateValue)
		}
		vmsTotal.Set(float64(len(vmsSlice)))
		vmsRunning.Set(float64(running))
	}

	// Update UPS metrics
	if upsVal != nil {
		upsStatus.Reset()
		upsBatteryCharge.Reset()
		upsLoad.Reset()
		upsRuntime.Reset()

		statusValue := 1.0 // Online by default
		if upsVal.Status != "ONLINE" && upsVal.Status != "OL" {
			statusValue = 0.0
		}
		upsName := "ups"
		upsStatus.WithLabelValues(upsName, upsVal.Model).Set(statusValue)
		upsBatteryCharge.WithLabelValues(upsName).Set(upsVal.BatteryCharge)
		upsLoad.WithLabelValues(upsName).Set(upsVal.LoadPercent)
		upsRuntime.WithLabelValues(upsName).Set(float64(upsVal.RuntimeLeft))
	}

	// Update share metrics
	if sharesSlice != nil {
		shareUsedBytes.Reset()
		for _, share := range sharesSlice {
			shareUsedBytes.WithLabelValues(share.Name).Set(float64(share.Used))
		}
		sharesTotal.Set(float64(len(sharesSlice)))
	}

	// Update GPU metrics
	if gpuSlice != nil {
		gpuTemperature.Reset()
		gpuUtilization.Reset()
		gpuMemoryUsed.Reset()
		gpuMemoryTotal.Reset()
		gpuPowerWatts.Reset()

		for i, gpu := range gpuSlice {
			if gpu == nil {
				continue
			}
			idx := fmt.Sprintf("%d", i)
			gpuTemperature.WithLabelValues(idx, gpu.Name).Set(gpu.Temperature)
			gpuUtilization.WithLabelValues(idx, gpu.Name).Set(gpu.UtilizationGPU)
			gpuMemoryUsed.WithLabelValues(idx, gpu.Name).Set(float64(gpu.MemoryUsed))
			gpuMemoryTotal.WithLabelValues(idx, gpu.Name).Set(float64(gpu.MemoryTotal))
			gpuPowerWatts.WithLabelValues(idx, gpu.Name).Set(gpu.PowerDraw)
		}
	}
}

// handleMetrics handles Prometheus metrics endpoint
// @Summary Prometheus metrics endpoint
// @Description Returns metrics in Prometheus exposition format for Grafana integration
// @Tags Monitoring
// @Produce text/plain
// @Success 200 {string} string "Prometheus metrics"
// @Router /metrics [get]
func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	// Update metrics from cache before serving
	s.updateMetrics()

	// Update network service metrics (these aren't in cache, need to fetch)
	s.updateNetworkServiceMetrics()

	// Serve metrics using Prometheus HTTP handler
	promhttp.HandlerFor(metricsRegistry, promhttp.HandlerOpts{
		EnableOpenMetrics: true,
	}).ServeHTTP(w, r)
}

// updateNetworkServiceMetrics reads network service status from the cache
func (s *Server) updateNetworkServiceMetrics() {
	status := s.GetNetworkServicesCache()
	if status == nil {
		return
	}

	serviceEnabled.Reset()
	serviceRunning.Reset()

	// Helper to set service metrics
	setServiceMetrics := func(name string, enabled, running bool) {
		enabledVal := 0.0
		if enabled {
			enabledVal = 1.0
		}
		runningVal := 0.0
		if running {
			runningVal = 1.0
		}
		serviceEnabled.WithLabelValues(name).Set(enabledVal)
		serviceRunning.WithLabelValues(name).Set(runningVal)
	}

	setServiceMetrics("smb", status.SMB.Enabled, status.SMB.Running)
	setServiceMetrics("nfs", status.NFS.Enabled, status.NFS.Running)
	setServiceMetrics("afp", status.AFP.Enabled, status.AFP.Running)
	setServiceMetrics("ftp", status.FTP.Enabled, status.FTP.Running)
	setServiceMetrics("ssh", status.SSH.Enabled, status.SSH.Running)
	setServiceMetrics("telnet", status.Telnet.Enabled, status.Telnet.Running)
	setServiceMetrics("avahi", status.Avahi.Enabled, status.Avahi.Running)
	setServiceMetrics("netbios", status.NetBIOS.Enabled, status.NetBIOS.Running)
	setServiceMetrics("wsd", status.WSD.Enabled, status.WSD.Running)
	setServiceMetrics("wireguard", status.WireGuard.Enabled, status.WireGuard.Running)
	setServiceMetrics("upnp", status.UPNP.Enabled, status.UPNP.Running)
	setServiceMetrics("ntp", status.NTP.Enabled, status.NTP.Running)
	setServiceMetrics("syslog", status.SyslogServer.Enabled, status.SyslogServer.Running)
}
