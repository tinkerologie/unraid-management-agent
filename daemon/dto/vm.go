package dto

import "time"

// VMInfo contains virtual machine information
type VMInfo struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	State           string    `json:"state"`
	CPUCount        int       `json:"cpu_count"`
	GuestCPUPercent float64   `json:"guest_cpu_percent"`
	HostCPUPercent  float64   `json:"host_cpu_percent"`
	MemoryAllocated uint64    `json:"memory_allocated_bytes"`
	MemoryUsed      uint64    `json:"memory_used_bytes"`
	MemoryDisplay   string    `json:"memory_display"`
	DiskPath        string    `json:"disk_path"`
	DiskSize        uint64    `json:"disk_size_bytes"`
	DiskReadBytes   uint64    `json:"disk_read_bytes"`
	DiskWriteBytes  uint64    `json:"disk_write_bytes"`
	NetworkRXBytes  uint64    `json:"network_rx_bytes"`
	NetworkTXBytes  uint64    `json:"network_tx_bytes"`
	Autostart       bool      `json:"autostart"`
	PersistentState bool      `json:"persistent"`
	Timestamp       time.Time `json:"timestamp"`
}
