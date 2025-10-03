package dto

import "time"

// ShareConfig represents share configuration
type ShareConfig struct {
	Name         string    `json:"name"`
	Comment      string    `json:"comment,omitempty"`
	Allocator    string    `json:"allocator,omitempty"`     // "highwater", "mostfree", "fillup"
	Floor        string    `json:"floor,omitempty"`         // Minimum free space
	SplitLevel   string    `json:"split_level,omitempty"`   // Directory depth for splitting
	IncludeDisks []string  `json:"include_disks,omitempty"` // Disks to include
	ExcludeDisks []string  `json:"exclude_disks,omitempty"` // Disks to exclude
	UseCache     string    `json:"use_cache,omitempty"`     // "yes", "no", "only", "prefer"
	Export       string    `json:"export,omitempty"`        // SMB/NFS/AFP export settings
	Security     string    `json:"security,omitempty"`      // "public", "private", "secure"
	Timestamp    time.Time `json:"timestamp"`
}

// NetworkConfig represents network interface configuration
type NetworkConfig struct {
	Interface     string    `json:"interface"`
	Type          string    `json:"type"` // "physical", "bond", "bridge", "vlan"
	IPAddress     string    `json:"ip_address,omitempty"`
	Netmask       string    `json:"netmask,omitempty"`
	Gateway       string    `json:"gateway,omitempty"`
	BondingMode   string    `json:"bonding_mode,omitempty"`   // If bond
	BondSlaves    []string  `json:"bond_slaves,omitempty"`    // If bond
	BridgeMembers []string  `json:"bridge_members,omitempty"` // If bridge
	VLANID        int       `json:"vlan_id,omitempty"`        // If VLAN
	Timestamp     time.Time `json:"timestamp"`
}

// SystemSettings represents system configuration
type SystemSettings struct {
	ServerName   string    `json:"server_name"`
	Description  string    `json:"description,omitempty"`
	Model        string    `json:"model,omitempty"`
	Timezone     string    `json:"timezone,omitempty"`
	DateFormat   string    `json:"date_format,omitempty"`
	TimeFormat   string    `json:"time_format,omitempty"`
	SecurityMode string    `json:"security_mode,omitempty"` // "public", "private"
	Timestamp    time.Time `json:"timestamp"`
}

// DockerSettings represents Docker configuration
type DockerSettings struct {
	Enabled        bool      `json:"enabled"`
	ImagePath      string    `json:"image_path,omitempty"`
	DefaultNetwork string    `json:"default_network,omitempty"`
	CustomNetworks []string  `json:"custom_networks,omitempty"`
	Timestamp      time.Time `json:"timestamp"`
}

// VMSettings represents VM Manager configuration
type VMSettings struct {
	Enabled         bool              `json:"enabled"`
	PCIDevices      []string          `json:"pci_devices,omitempty"`
	USBDevices      []string          `json:"usb_devices,omitempty"`
	DefaultSettings map[string]string `json:"default_settings,omitempty"`
	Timestamp       time.Time         `json:"timestamp"`
}

// DiskSettings represents disk configuration
type DiskSettings struct {
	SpindownDelay   int       `json:"spindown_delay_minutes"`             // Default spin down delay in minutes
	StartArray      bool      `json:"start_array"`                        // Auto start array on boot
	SpinupGroups    bool      `json:"spinup_groups"`                      // Enable spinup groups
	ShutdownTimeout int       `json:"shutdown_timeout_seconds,omitempty"` // Shutdown timeout in seconds
	DefaultFsType   string    `json:"default_filesystem,omitempty"`       // Default filesystem type (xfs, btrfs, etc.)
	Timestamp       time.Time `json:"timestamp"`
}
