package dto

import "time"

// Exit status bits of smartctl, as documented in smartctl(8).
const (
	SMARTExitCommandLine   = 1 << 0 // command line did not parse
	SMARTExitDeviceOpen    = 1 << 1 // device open failed, or disk in low-power mode with -n
	SMARTExitCommandFailed = 1 << 2 // a SMART command failed, or a checksum error
	SMARTExitDiskFailing   = 1 << 3 // SMART status check returned "DISK FAILING"
	SMARTExitPrefailNow    = 1 << 4 // a pre-fail attribute is at or below threshold now
	SMARTExitPastThreshold = 1 << 5 // an attribute was at or below threshold in the past
	SMARTExitErrorLog      = 1 << 6 // the device error log contains records
	SMARTExitSelfTestLog   = 1 << 7 // the self-test log contains errors
)

// DiskInfo contains disk information
type DiskInfo struct {
	ID          string  `json:"id" example:"disk1"`
	Device      string  `json:"device" example:"sda"`
	Name        string  `json:"name" example:"Disk 1"`
	Status      string  `json:"status" example:"OK"`
	Size        uint64  `json:"size_bytes" example:"12000138625024"`
	Used        uint64  `json:"used_bytes" example:"5400062381260"`
	Free        uint64  `json:"free_bytes" example:"6600076243764"`
	Temperature float64 `json:"temperature_celsius" example:"35"`
	SMARTStatus string  `json:"smart_status" example:"PASSED"`
	SMARTErrors int     `json:"smart_errors" example:"0"`
	// SMARTExitStatus is the bitmask smartctl exited with (see the SMARTExit
	// constants), omitted when zero or when smartctl did not run to its own exit.
	SMARTExitStatus int    `json:"smart_exit_status,omitempty" example:"32"`
	SpindownDelay   int    `json:"spindown_delay" example:"30"`
	FileSystem      string `json:"filesystem" example:"xfs"`

	// Disk identification
	SerialNumber string `json:"serial_number,omitempty" example:"WD-WMC4N0123456"`
	Model        string `json:"model,omitempty" example:"WDC WD120EFBX-68B0EN0"`
	Role         string `json:"role,omitempty" example:"data"`         // "parity", "parity2", "data", "cache", "pool"
	SpinState    string `json:"spin_state,omitempty" example:"active"` // "active", "standby", "unknown"

	// Enhanced SMART attributes
	SMARTAttributes map[string]SMARTAttribute `json:"smart_attributes,omitempty"`
	PowerOnHours    uint64                    `json:"power_on_hours,omitempty" example:"25000"`
	PowerCycleCount uint64                    `json:"power_cycle_count,omitempty" example:"100"`

	// I/O Statistics
	ReadBytes     uint64  `json:"read_bytes,omitempty" example:"1073741824"`
	WriteBytes    uint64  `json:"write_bytes,omitempty" example:"536870912"`
	ReadOps       uint64  `json:"read_ops,omitempty" example:"100000"`
	WriteOps      uint64  `json:"write_ops,omitempty" example:"50000"`
	IOUtilization float64 `json:"io_utilization_percent,omitempty" example:"5.2"`

	// Mount information
	MountPoint   string  `json:"mount_point,omitempty" example:"/mnt/disk1"`
	UsagePercent float64 `json:"usage_percent,omitempty" example:"45.0"`

	// Per-disk temperature thresholds (Issue #46)
	// If set, these override the global defaults. Null/omitted means use global defaults.
	TempWarning  *int `json:"temp_warning_celsius,omitempty" example:"50"`  // Per-disk warning threshold override
	TempCritical *int `json:"temp_critical_celsius,omitempty" example:"60"` // Per-disk critical threshold override

	// SourceStatus is non-nil only when the backing data source is not healthy.
	SourceStatus *SourceStatus `json:"source_status,omitempty"`

	Timestamp time.Time `json:"timestamp"`
}

// SMARTAttribute represents a SMART attribute
type SMARTAttribute struct {
	ID         int    `json:"id" example:"5"`
	Name       string `json:"name" example:"Reallocated_Sector_Ct"`
	Value      int    `json:"value" example:"100"`
	Worst      int    `json:"worst" example:"100"`
	Threshold  int    `json:"threshold" example:"5"`
	RawValue   string `json:"raw_value" example:"0"`
	WhenFailed string `json:"when_failed,omitempty" example:""`
}
