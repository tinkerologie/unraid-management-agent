package dto

import "time"

// ContainerInfo contains Docker container information
type ContainerInfo struct {
	ID             string          `json:"id" example:"abc123def456"`
	Name           string          `json:"name" example:"plex"`
	Image          string          `json:"image" example:"plexinc/pms-docker:latest"`
	Version        string          `json:"version" example:"1.40.1"`
	State          string          `json:"state" example:"running"`
	Status         string          `json:"status" example:"Up 2 days"`
	NetworkMode    string          `json:"network_mode" example:"bridge"`
	IPAddress      string          `json:"ip_address" example:"172.17.0.2"`
	CPUPercent     float64         `json:"cpu_percent" example:"5.2"`
	MemoryUsage    uint64          `json:"memory_usage_bytes" example:"1073741824"`
	MemoryLimit    uint64          `json:"memory_limit_bytes" example:"8589934592"`
	MemoryDisplay  string          `json:"memory_display" example:"1 GiB / 8 GiB"`
	NetworkRX      uint64          `json:"network_rx_bytes" example:"104857600"`
	NetworkTX      uint64          `json:"network_tx_bytes" example:"52428800"`
	Ports          []PortMapping   `json:"ports"`
	PortMappings   []string        `json:"port_mappings"`
	VolumeMappings []VolumeMapping `json:"volume_mappings"`
	RestartPolicy  string          `json:"restart_policy" example:"unless-stopped"`
	Uptime         string          `json:"uptime" example:"2 days"`
	Timestamp      time.Time       `json:"timestamp"`
}

// PortMapping represents a port mapping
type PortMapping struct {
	PrivatePort int    `json:"private_port" example:"32400"`
	PublicPort  int    `json:"public_port" example:"32400"`
	Type        string `json:"type" example:"tcp"`
}

// VolumeMapping represents a volume mapping
type VolumeMapping struct {
	ContainerPath string `json:"container_path" example:"/config"`
	HostPath      string `json:"host_path" example:"/mnt/user/appdata/plex"`
	Mode          string `json:"mode" example:"rw"`
}
