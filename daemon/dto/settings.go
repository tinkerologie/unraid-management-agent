package dto

import "time"

// DiskSettingsExtended represents extended disk configuration including temperature thresholds
// @Description Extended disk settings with temperature thresholds from Unraid configuration
type DiskSettingsExtended struct {
	// Basic disk settings (from disk.cfg)
	SpindownDelay   int    `json:"spindown_delay_minutes" example:"30"`             // Default spin down delay in minutes
	StartArray      bool   `json:"start_array" example:"true"`                      // Auto start array on boot
	SpinupGroups    bool   `json:"spinup_groups" example:"false"`                   // Enable spinup groups
	ShutdownTimeout int    `json:"shutdown_timeout_seconds,omitempty" example:"90"` // Shutdown timeout in seconds
	DefaultFsType   string `json:"default_filesystem,omitempty" example:"xfs"`      // Default filesystem type

	// Temperature thresholds from dynamix.cfg (Issue #45)
	HDDTempWarning      int `json:"hdd_temp_warning_celsius" example:"45"`     // HDD warning temperature threshold
	HDDTempCritical     int `json:"hdd_temp_critical_celsius" example:"55"`    // HDD critical temperature threshold
	SSDTempWarning      int `json:"ssd_temp_warning_celsius" example:"60"`     // SSD warning temperature threshold
	SSDTempCritical     int `json:"ssd_temp_critical_celsius" example:"70"`    // SSD critical temperature threshold
	WarningUtilization  int `json:"warning_utilization_percent" example:"70"`  // Disk utilization warning threshold
	CriticalUtilization int `json:"critical_utilization_percent" example:"90"` // Disk utilization critical threshold

	// NVME power monitoring setting
	NVMEPowerMonitoring bool `json:"nvme_power_monitoring" example:"false"` // Enable NVME power monitoring

	Timestamp time.Time `json:"timestamp"`
}

// MoverSettings represents mover configuration and status (Issue #48)
// @Description Mover configuration, schedule, and current status
type MoverSettings struct {
	// Mover status
	Active bool `json:"active" example:"false"` // Is mover currently running

	// Mover schedule (cron format)
	Schedule string `json:"schedule" example:"0 12 * * *"` // Mover schedule in cron format

	// Additional mover settings
	Logging    bool `json:"logging" example:"false"`          // Mover logging enabled
	CacheFloor int  `json:"cache_floor_kb" example:"2000000"` // Cache floor in KB

	Timestamp time.Time `json:"timestamp"`
}

// ParitySchedule represents parity check scheduling information (Issue #47)
// @Description Parity check schedule configuration
type ParitySchedule struct {
	// Schedule settings
	Mode       string `json:"mode" example:"manual"`      // Schedule mode: "manual", "daily", "weekly", "monthly", "yearly"
	Day        int    `json:"day" example:"0"`            // Day of week (0-6) or day of month (1-31)
	Hour       int    `json:"hour" example:"0"`           // Hour to run (0-23)
	DayOfMonth int    `json:"day_of_month" example:"1"`   // Day of month for monthly schedule
	Frequency  int    `json:"frequency" example:"1"`      // Frequency multiplier
	Duration   int    `json:"duration_hours" example:"6"` // Max duration in hours (0 = unlimited)
	Cumulative bool   `json:"cumulative" example:"true"`  // Resume paused checks
	Correcting bool   `json:"correcting" example:"true"`  // Correcting vs non-correcting check

	// Pause/resume schedule
	PauseHour  int `json:"pause_hour,omitempty" example:"6"`  // Hour to pause (if scheduled)
	ResumeHour int `json:"resume_hour,omitempty" example:"0"` // Hour to resume (if scheduled)

	Timestamp time.Time `json:"timestamp"`
}

// ParityHistoryExtended extends ParityCheckHistory with schedule info
// @Description Parity check history with schedule information
type ParityHistoryExtended struct {
	Schedule  ParitySchedule      `json:"schedule"`
	Records   []ParityCheckRecord `json:"records"`
	Timestamp time.Time           `json:"timestamp"`
}

// ServiceStatus represents Docker/VM service enabled status (Issue #49)
// @Description Docker and VM Manager service enabled status
type ServiceStatus struct {
	DockerEnabled    bool `json:"docker_enabled" example:"true"`      // Docker service enabled
	DockerAutostart  bool `json:"docker_autostart" example:"true"`    // Docker autostart on boot
	VMManagerEnabled bool `json:"vm_manager_enabled" example:"false"` // VM Manager service enabled
	VMAutostart      bool `json:"vm_autostart" example:"false"`       // VM autostart on boot

	Timestamp time.Time `json:"timestamp"`
}

// PluginInfo represents an installed plugin (Issue #52)
// @Description Information about an installed Unraid plugin
type PluginInfo struct {
	Name            string `json:"name" example:"community.applications"`         // Plugin name
	Version         string `json:"version" example:"2025.10.27"`                  // Current installed version
	Author          string `json:"author" example:"Lime Technology"`              // Plugin author
	Enabled         bool   `json:"enabled" example:"true"`                        // Is plugin enabled
	URL             string `json:"url,omitempty" example:"https://..."`           // Plugin URL
	SupportURL      string `json:"support_url,omitempty"`                         // Support forum URL
	Icon            string `json:"icon,omitempty" example:"users"`                // Plugin icon
	UpdateAvailable bool   `json:"update_available" example:"false"`              // Is update available
	LatestVersion   string `json:"latest_version,omitempty" example:"2025.10.28"` // Latest available version
}

// PluginList represents the list of installed plugins
// @Description List of installed Unraid plugins
type PluginList struct {
	Plugins          []PluginInfo `json:"plugins"`
	TotalCount       int          `json:"total_count" example:"18"`
	UpdatesAvailable int          `json:"updates_available" example:"2"`

	Timestamp time.Time `json:"timestamp"`
}

// UpdateStatus represents update availability (Issue #50)
// @Description Unraid OS and plugin update availability
type UpdateStatus struct {
	// Unraid OS update status
	CurrentVersion    string `json:"current_version" example:"7.2.3"`
	LatestVersion     string `json:"latest_version,omitempty" example:"7.2.4"`
	OSUpdateAvailable bool   `json:"os_update_available" example:"false"`

	// Plugin update status
	PluginsWithUpdates []PluginInfo `json:"plugins_with_updates,omitempty"`
	TotalPlugins       int          `json:"total_plugins" example:"18"`
	PluginUpdatesCount int          `json:"plugin_updates_count" example:"2"`

	Timestamp time.Time `json:"timestamp"`
}

// FlashDriveHealth represents USB flash drive health information (Issue #51)
// @Description USB flash boot drive health information
type FlashDriveHealth struct {
	Device string `json:"device" example:"/dev/sda"`         // Device path
	Model  string `json:"model" example:"SanDisk Ultra Fit"` // Device model
	Vendor string `json:"vendor" example:"SanDisk"`          // Device vendor
	GUID   string `json:"guid" example:"0781-5583-..."`      // Flash GUID

	// Size and usage
	SizeBytes    uint64  `json:"size_bytes" example:"30752636928"` // Total size in bytes
	UsedBytes    uint64  `json:"used_bytes" example:"2000000000"`  // Used space in bytes
	FreeBytes    uint64  `json:"free_bytes" example:"28752636928"` // Free space in bytes
	UsagePercent float64 `json:"usage_percent" example:"6.5"`      // Usage percentage

	// SMART status (may be limited for USB)
	SMARTAvailable bool   `json:"smart_available" example:"false"`         // Is SMART data available
	SMARTStatus    string `json:"smart_status,omitempty" example:"PASSED"` // SMART status if available

	Timestamp time.Time `json:"timestamp"`
}

// PluginUpdateResult contains the result of updating a single plugin
type PluginUpdateResult struct {
	PluginName      string    `json:"plugin_name" example:"community.applications"`
	PreviousVersion string    `json:"previous_version" example:"2025.10.27"`
	NewVersion      string    `json:"new_version" example:"2025.10.28"`
	Success         bool      `json:"success" example:"true"`
	Message         string    `json:"message" example:"Updated successfully"`
	Timestamp       time.Time `json:"timestamp"`
}

// PluginBulkUpdateResult contains the results of updating multiple plugins
type PluginBulkUpdateResult struct {
	Results   []PluginUpdateResult `json:"results"`
	Succeeded int                  `json:"succeeded" example:"3"`
	Failed    int                  `json:"failed" example:"0"`
	Timestamp time.Time            `json:"timestamp"`
}

// NetworkServiceInfo represents a single network service status
// @Description Status information for a single network service
type NetworkServiceInfo struct {
	Name        string `json:"name" example:"SMB"`                                   // Service name
	Enabled     bool   `json:"enabled" example:"true"`                               // Is service enabled in configuration
	Running     bool   `json:"running" example:"true"`                               // Is service currently running
	Port        int    `json:"port,omitempty" example:"445"`                         // Primary port (if applicable)
	Description string `json:"description,omitempty" example:"Windows file sharing"` // Service description
}

// NetworkServicesStatus represents the status of all network services
// @Description Status of all Unraid network services (SMB, NFS, FTP, SSH, etc.)
type NetworkServicesStatus struct {
	// File Sharing Services
	SMB NetworkServiceInfo `json:"smb"` // Samba/Windows file sharing
	NFS NetworkServiceInfo `json:"nfs"` // NFS file sharing
	AFP NetworkServiceInfo `json:"afp"` // Apple Filing Protocol (via Avahi)
	FTP NetworkServiceInfo `json:"ftp"` // FTP server

	// Remote Access Services
	SSH    NetworkServiceInfo `json:"ssh"`    // SSH server
	Telnet NetworkServiceInfo `json:"telnet"` // Telnet server

	// Discovery Services
	Avahi   NetworkServiceInfo `json:"avahi"`   // mDNS/DNS-SD service discovery
	NetBIOS NetworkServiceInfo `json:"netbios"` // NetBIOS name service
	WSD     NetworkServiceInfo `json:"wsd"`     // Web Services Discovery

	// VPN Services
	WireGuard NetworkServiceInfo `json:"wireguard"` // WireGuard VPN

	// System Services
	UPNP         NetworkServiceInfo `json:"upnp"`          // UPnP/IGD
	NTP          NetworkServiceInfo `json:"ntp"`           // NTP time sync
	SyslogServer NetworkServiceInfo `json:"syslog_server"` // Syslog remote server

	// Summary
	TotalServices   int `json:"total_services" example:"13"`  // Total services monitored
	EnabledServices int `json:"enabled_services" example:"8"` // Services enabled
	RunningServices int `json:"running_services" example:"6"` // Services currently running

	Timestamp time.Time `json:"timestamp"`
}
