package dto

import "time"

// GPUMetrics contains GPU metrics
type GPUMetrics struct {
	Available         bool      `json:"available" example:"true"`
	Index             int       `json:"index" example:"0"`                                                 // GPU index for multi-GPU systems
	PCIID             string    `json:"pci_id,omitempty" example:"0000:01:00.0"`                           // PCI bus ID (e.g., "0000:01:00.0")
	Vendor            string    `json:"vendor" example:"nvidia"`                                           // "nvidia", "intel", "amd"
	UUID              string    `json:"uuid,omitempty" example:"GPU-12345678-1234-1234-1234-123456789012"` // Device UUID (NVIDIA only)
	Name              string    `json:"name" example:"NVIDIA GeForce RTX 3080"`
	DriverVersion     string    `json:"driver_version" example:"535.183.01"`
	Temperature       float64   `json:"temperature_celsius" example:"55"`
	CPUTemperature    float64   `json:"cpu_temperature_celsius,omitempty" example:"45"` // CPU temp for Intel iGPUs (shares die with CPU)
	UtilizationGPU    float64   `json:"utilization_gpu_percent" example:"75.5"`
	UtilizationMemory float64   `json:"utilization_memory_percent" example:"60.2"`
	MemoryTotal       uint64    `json:"memory_total_bytes" example:"10737418240"`
	MemoryUsed        uint64    `json:"memory_used_bytes" example:"6442450944"`
	PowerDraw         float64   `json:"power_draw_watts" example:"250.5"`
	FanSpeed          float64   `json:"fan_speed_percent,omitempty" example:"65"` // Fan speed % (NVIDIA)
	FanRPM            int       `json:"fan_rpm,omitempty" example:"2000"`         // Fan RPM (AMD discrete)
	FanMaxRPM         int       `json:"fan_max_rpm,omitempty" example:"3000"`     // Max fan RPM (AMD discrete)
	Timestamp         time.Time `json:"timestamp"`
}
