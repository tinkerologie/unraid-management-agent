package dto

import "time"

// ContainerInfo contains Docker container information
type ContainerInfo struct {
	ID             string          `json:"id"`
	Name           string          `json:"name"`
	Image          string          `json:"image"`
	Version        string          `json:"version"`
	State          string          `json:"state"`
	Status         string          `json:"status"`
	NetworkMode    string          `json:"network_mode"`
	IPAddress      string          `json:"ip_address"`
	CPUPercent     float64         `json:"cpu_percent"`
	MemoryUsage    uint64          `json:"memory_usage_bytes"`
	MemoryLimit    uint64          `json:"memory_limit_bytes"`
	MemoryDisplay  string          `json:"memory_display"`
	NetworkRX      uint64          `json:"network_rx_bytes"`
	NetworkTX      uint64          `json:"network_tx_bytes"`
	Ports          []PortMapping   `json:"ports"`
	PortMappings   []string        `json:"port_mappings"`
	VolumeMappings []VolumeMapping `json:"volume_mappings"`
	RestartPolicy  string          `json:"restart_policy"`
	Uptime         string          `json:"uptime"`
	Timestamp      time.Time       `json:"timestamp"`
}

// PortMapping represents a port mapping
type PortMapping struct {
	PrivatePort int    `json:"private_port"`
	PublicPort  int    `json:"public_port"`
	Type        string `json:"type"`
}

// VolumeMapping represents a volume mapping
type VolumeMapping struct {
	ContainerPath string `json:"container_path"`
	HostPath      string `json:"host_path"`
	Mode          string `json:"mode"`
}
