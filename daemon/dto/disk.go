package dto

import "time"

// DiskInfo contains disk information
type DiskInfo struct {
	ID            string  `json:"id" example:"disk1"`
	Device        string  `json:"device" example:"sda"`
	Name          string  `json:"name" example:"Disk 1"`
	Status        string  `json:"status" example:"OK"`
	Size          uint64  `json:"size_bytes" example:"12000138625024"`
	Used          uint64  `json:"used_bytes" example:"5400062381260"`
	Free          uint64  `json:"free_bytes" example:"6600076243764"`
	Temperature   float64 `json:"temperature_celsius" example:"35"`
	SMARTStatus   string  `json:"smart_status" example:"PASSED"`
	SMARTErrors   int     `json:"smart_errors" example:"0"`
	SpindownDelay int     `json:"spindown_delay" example:"30"`
	FileSystem    string  `json:"filesystem" example:"xfs"`

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
