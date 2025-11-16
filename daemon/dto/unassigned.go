package dto

import "time"

// UnassignedDevice represents an unassigned disk device
type UnassignedDevice struct {
	// Device identification
	Device         string `json:"device"` // e.g., "sdc", "nvme0n1"
	SerialNumber   string `json:"serial_number"`
	Model          string `json:"model"`
	Identification string `json:"identification"` // Friendly name/label

	// Partition information
	Partitions []UnassignedPartition `json:"partitions"`

	// Status
	Status      string  `json:"status"`     // "mounted", "unmounted", "mounting", "error"
	SpinState   string  `json:"spin_state"` // "active", "standby", "unknown"
	Temperature float64 `json:"temperature_celsius,omitempty"`

	// Configuration
	AutoMount     bool   `json:"auto_mount"`
	PassThrough   bool   `json:"pass_through"`
	DisableMount  bool   `json:"disable_mount"`
	ScriptEnabled bool   `json:"script_enabled"`
	ScriptPath    string `json:"script_path,omitempty"`

	// I/O Statistics
	Reads  uint64 `json:"reads,omitempty"`
	Writes uint64 `json:"writes,omitempty"`

	Timestamp time.Time `json:"timestamp"`
}

// UnassignedPartition represents a partition on an unassigned device
type UnassignedPartition struct {
	PartitionNumber int     `json:"partition_number"`
	Label           string  `json:"label,omitempty"`
	FileSystem      string  `json:"filesystem"` // "ntfs", "ext4", "xfs", "btrfs", "exfat", "hfsplus", "apfs"
	MountPoint      string  `json:"mount_point,omitempty"`
	Size            uint64  `json:"size_bytes"`
	Used            uint64  `json:"used_bytes,omitempty"`
	Free            uint64  `json:"free_bytes,omitempty"`
	UsagePercent    float64 `json:"usage_percent,omitempty"`
	ReadOnly        bool    `json:"read_only"`
	SMBShare        bool    `json:"smb_share"` // Is shared via SMB?
	NFSShare        bool    `json:"nfs_share"` // Is shared via NFS?
	Status          string  `json:"status"`    // "mounted", "unmounted"
}

// UnassignedRemoteShare represents a mounted remote SMB/NFS share
type UnassignedRemoteShare struct {
	// Share identification
	Type       string `json:"type"`   // "smb", "nfs", "iso"
	Source     string `json:"source"` // "//server/share", "server:/export", "/path/file.iso"
	MountPoint string `json:"mount_point"`

	// Status
	Status string `json:"status"` // "mounted", "unmounted", "mounting", "error"

	// Capacity (if mounted)
	Size         uint64  `json:"size_bytes,omitempty"`
	Used         uint64  `json:"used_bytes,omitempty"`
	Free         uint64  `json:"free_bytes,omitempty"`
	UsagePercent float64 `json:"usage_percent,omitempty"`

	// Configuration
	AutoMount bool `json:"auto_mount"`
	ReadOnly  bool `json:"read_only"`

	// SMB-specific
	SMBServer string `json:"smb_server,omitempty"`
	SMBShare  string `json:"smb_share,omitempty"`
	SMBDomain string `json:"smb_domain,omitempty"`
	SMBUser   string `json:"smb_user,omitempty"`

	// NFS-specific
	NFSServer  string `json:"nfs_server,omitempty"`
	NFSExport  string `json:"nfs_export,omitempty"`
	NFSOptions string `json:"nfs_options,omitempty"`

	Timestamp time.Time `json:"timestamp"`
}

// UnassignedDeviceList groups unassigned devices
type UnassignedDeviceList struct {
	Devices      []UnassignedDevice      `json:"devices"`
	RemoteShares []UnassignedRemoteShare `json:"remote_shares"`
	Timestamp    time.Time               `json:"timestamp"`
}
