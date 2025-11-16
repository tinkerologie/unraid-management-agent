package dto

import "time"

// GPUMetrics contains GPU metrics
type GPUMetrics struct {
	Available         bool      `json:"available"`
	Index             int       `json:"index"`            // GPU index for multi-GPU systems
	PCIID             string    `json:"pci_id,omitempty"` // PCI bus ID (e.g., "0000:01:00.0")
	Vendor            string    `json:"vendor"`           // "nvidia", "intel", "amd"
	UUID              string    `json:"uuid,omitempty"`   // Device UUID (NVIDIA only)
	Name              string    `json:"name"`
	DriverVersion     string    `json:"driver_version"`
	Temperature       float64   `json:"temperature_celsius"`
	CPUTemperature    float64   `json:"cpu_temperature_celsius,omitempty"` // CPU temp for Intel iGPUs (shares die with CPU)
	UtilizationGPU    float64   `json:"utilization_gpu_percent"`
	UtilizationMemory float64   `json:"utilization_memory_percent"`
	MemoryTotal       uint64    `json:"memory_total_bytes"`
	MemoryUsed        uint64    `json:"memory_used_bytes"`
	PowerDraw         float64   `json:"power_draw_watts"`
	FanSpeed          float64   `json:"fan_speed_percent,omitempty"` // Fan speed % (NVIDIA)
	FanRPM            int       `json:"fan_rpm,omitempty"`           // Fan RPM (AMD discrete)
	FanMaxRPM         int       `json:"fan_max_rpm,omitempty"`       // Max fan RPM (AMD discrete)
	Timestamp         time.Time `json:"timestamp"`
}
