package dto

import "time"

// SystemInfo contains system-level metrics
type SystemInfo struct {
	Hostname        string    `json:"hostname"`
	Version         string    `json:"version"`
	Uptime          int64     `json:"uptime_seconds"`
	
	// CPU Information
	CPUUsage        float64            `json:"cpu_usage_percent"`
	CPUModel        string             `json:"cpu_model"`
	CPUCores        int                `json:"cpu_cores"`
	CPUThreads      int                `json:"cpu_threads"`
	CPUMHz          float64            `json:"cpu_mhz"`
	CPUPerCore      map[string]float64 `json:"cpu_per_core_usage,omitempty"`
	CPUTemp         float64            `json:"cpu_temp_celsius"`
	
	// Memory Information
	RAMUsage        float64   `json:"ram_usage_percent"`
	RAMTotal        uint64    `json:"ram_total_bytes"`
	RAMUsed         uint64    `json:"ram_used_bytes"`
	RAMFree         uint64    `json:"ram_free_bytes"`
	RAMBuffers      uint64    `json:"ram_buffers_bytes"`
	RAMCached       uint64    `json:"ram_cached_bytes"`
	
	// System Information
	ServerModel     string    `json:"server_model"`
	BIOSVersion     string    `json:"bios_version"`
	BIOSDate        string    `json:"bios_date"`
	MotherboardTemp float64   `json:"motherboard_temp_celsius"`
	
	// Additional Metrics
	Fans            []FanInfo `json:"fans"`
	Timestamp       time.Time `json:"timestamp"`
}

// FanInfo contains fan speed information
type FanInfo struct {
	Name string `json:"name"`
	RPM  int    `json:"rpm"`
}
