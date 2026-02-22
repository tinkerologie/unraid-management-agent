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

// ContainerUpdateInfo contains update status for a Docker container
type ContainerUpdateInfo struct {
	ContainerID     string    `json:"container_id" example:"abc123def456"`
	ContainerName   string    `json:"container_name" example:"plex"`
	Image           string    `json:"image" example:"plexinc/pms-docker:latest"`
	CurrentDigest   string    `json:"current_digest,omitempty"`
	LatestDigest    string    `json:"latest_digest,omitempty"`
	UpdateAvailable bool      `json:"update_available" example:"true"`
	Timestamp       time.Time `json:"timestamp"`
}

// ContainerUpdateResult contains the result of a container update operation
type ContainerUpdateResult struct {
	ContainerID    string    `json:"container_id" example:"abc123def456"`
	ContainerName  string    `json:"container_name" example:"plex"`
	Image          string    `json:"image" example:"plexinc/pms-docker:latest"`
	PreviousDigest string    `json:"previous_digest,omitempty"`
	NewDigest      string    `json:"new_digest,omitempty"`
	Updated        bool      `json:"updated" example:"true"`
	Recreated      bool      `json:"recreated" example:"true"`
	Message        string    `json:"message" example:"Container updated successfully"`
	Timestamp      time.Time `json:"timestamp"`
}

// ContainerSizeInfo contains size information for a Docker container
type ContainerSizeInfo struct {
	ContainerID   string    `json:"container_id" example:"abc123def456"`
	ContainerName string    `json:"container_name" example:"plex"`
	SizeRw        int64     `json:"size_rw_bytes" example:"104857600"`
	SizeRootFs    int64     `json:"size_root_fs_bytes" example:"1073741824"`
	ImageSize     int64     `json:"image_size_bytes" example:"536870912"`
	SizeDisplay   string    `json:"size_display" example:"1.0 GiB"`
	Timestamp     time.Time `json:"timestamp"`
}

// ContainerUpdatesResult contains update status for multiple containers
type ContainerUpdatesResult struct {
	Containers       []ContainerUpdateInfo `json:"containers"`
	TotalCount       int                   `json:"total_count" example:"10"`
	UpdatesAvailable int                   `json:"updates_available" example:"2"`
	Timestamp        time.Time             `json:"timestamp"`
}

// ContainerBulkUpdateResult contains results of updating multiple containers
type ContainerBulkUpdateResult struct {
	Results   []ContainerUpdateResult `json:"results"`
	Succeeded int                     `json:"succeeded" example:"8"`
	Failed    int                     `json:"failed" example:"1"`
	Skipped   int                     `json:"skipped" example:"1"`
	Timestamp time.Time               `json:"timestamp"`
}

// ContainerLogs contains log output from a Docker container
type ContainerLogs struct {
	ContainerID   string    `json:"container_id" example:"abc123def456"`
	ContainerName string    `json:"container_name" example:"plex"`
	Logs          string    `json:"logs"`
	LineCount     int       `json:"line_count" example:"100"`
	Since         string    `json:"since,omitempty" example:"2026-02-17T00:00:00Z"`
	Timestamp     time.Time `json:"timestamp"`
}
