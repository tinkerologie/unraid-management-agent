package dto

import "time"

// VMInfo contains virtual machine information
type VMInfo struct {
	ID              string    `json:"id" example:"1"`
	Name            string    `json:"name" example:"Windows 11"`
	State           string    `json:"state" example:"running"`
	CPUCount        int       `json:"cpu_count" example:"4"`
	GuestCPUPercent float64   `json:"guest_cpu_percent" example:"25.5"`
	HostCPUPercent  float64   `json:"host_cpu_percent" example:"12.3"`
	MemoryAllocated uint64    `json:"memory_allocated_bytes" example:"8589934592"`
	MemoryUsed      uint64    `json:"memory_used_bytes" example:"4294967296"`
	MemoryDisplay   string    `json:"memory_display" example:"4 GiB / 8 GiB"`
	DiskPath        string    `json:"disk_path" example:"/mnt/user/domains/Windows 11/vdisk1.img"`
	DiskSize        uint64    `json:"disk_size_bytes" example:"107374182400"`
	DiskReadBytes   uint64    `json:"disk_read_bytes" example:"1073741824"`
	DiskWriteBytes  uint64    `json:"disk_write_bytes" example:"536870912"`
	NetworkRXBytes  uint64    `json:"network_rx_bytes" example:"104857600"`
	NetworkTXBytes  uint64    `json:"network_tx_bytes" example:"52428800"`
	Autostart       bool      `json:"autostart" example:"false"`
	PersistentState bool      `json:"persistent" example:"true"`
	Timestamp       time.Time `json:"timestamp"`
}
