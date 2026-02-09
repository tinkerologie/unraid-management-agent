package dto

import "time"

// SystemInfo contains system-level metrics
type SystemInfo struct {
	Hostname     string `json:"hostname" example:"tower"`
	Version      string `json:"version" example:"6.12.6"`
	AgentVersion string `json:"agent_version" example:"2025.12.1"`
	Uptime       int64  `json:"uptime_seconds" example:"86400"`

	// CPU Information
	CPUUsage       float64            `json:"cpu_usage_percent" example:"45.2"`
	CPUModel       string             `json:"cpu_model" example:"Intel(R) Core(TM) i7-9700K CPU @ 3.60GHz"`
	CPUCores       int                `json:"cpu_cores" example:"8"`
	CPUThreads     int                `json:"cpu_threads" example:"8"`
	CPUMHz         float64            `json:"cpu_mhz" example:"4900.0"`
	CPUPerCore     map[string]float64 `json:"cpu_per_core_usage,omitempty"`
	CPUTemp        float64            `json:"cpu_temp_celsius" example:"45.0"`
	CPUPowerWatts  *float64           `json:"cpu_power_watts,omitempty" example:"65.5"`
	DRAMPowerWatts *float64           `json:"dram_power_watts,omitempty" example:"5.2"`

	// Memory Information
	RAMUsage   float64 `json:"ram_usage_percent" example:"65.5"`
	RAMTotal   uint64  `json:"ram_total_bytes" example:"34359738368"`
	RAMUsed    uint64  `json:"ram_used_bytes" example:"22548578304"`
	RAMFree    uint64  `json:"ram_free_bytes" example:"11811160064"`
	RAMBuffers uint64  `json:"ram_buffers_bytes" example:"1073741824"`
	RAMCached  uint64  `json:"ram_cached_bytes" example:"8589934592"`

	// System Information
	ServerModel     string  `json:"server_model" example:"Supermicro X11SCL-F"`
	BIOSVersion     string  `json:"bios_version" example:"1.4"`
	BIOSDate        string  `json:"bios_date" example:"12/25/2023"`
	MotherboardTemp float64 `json:"motherboard_temp_celsius" example:"35.0"`

	// Virtualization Features
	HVMEnabled   bool `json:"hvm_enabled" example:"true"`
	IOMMUEnabled bool `json:"iommu_enabled" example:"true"`

	// Additional System Information
	OpenSSLVersion   string `json:"openssl_version,omitempty" example:"3.0.2"`
	ParityCheckSpeed string `json:"parity_check_speed,omitempty" example:"100 MB/s"`
	KernelVersion    string `json:"kernel_version,omitempty" example:"6.1.64-Unraid"`

	// Additional Metrics
	Fans      []FanInfo `json:"fans"`
	Timestamp time.Time `json:"timestamp"`
}

// FanInfo contains fan speed information
type FanInfo struct {
	Name string `json:"name" example:"CPU Fan"`
	RPM  int    `json:"rpm" example:"1200"`
}
