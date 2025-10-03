package dto

import "time"

// DiskInfo contains disk information
type DiskInfo struct {
	ID            string  `json:"id"`
	Device        string  `json:"device"`
	Name          string  `json:"name"`
	Status        string  `json:"status"`
	Size          uint64  `json:"size_bytes"`
	Used          uint64  `json:"used_bytes"`
	Free          uint64  `json:"free_bytes"`
	Temperature   float64 `json:"temperature_celsius"`
	SMARTStatus   string  `json:"smart_status"`
	SMARTErrors   int     `json:"smart_errors"`
	SpindownDelay int     `json:"spindown_delay"`
	FileSystem    string  `json:"filesystem"`

	// Disk identification
	SerialNumber string `json:"serial_number,omitempty"`
	Model        string `json:"model,omitempty"`
	Role         string `json:"role,omitempty"`       // "parity", "parity2", "data", "cache", "pool"
	SpinState    string `json:"spin_state,omitempty"` // "active", "standby", "unknown"

	// Enhanced SMART attributes
	SMARTAttributes map[string]SMARTAttribute `json:"smart_attributes,omitempty"`
	PowerOnHours    uint64                    `json:"power_on_hours,omitempty"`
	PowerCycleCount uint64                    `json:"power_cycle_count,omitempty"`

	// I/O Statistics
	ReadBytes     uint64  `json:"read_bytes,omitempty"`
	WriteBytes    uint64  `json:"write_bytes,omitempty"`
	ReadOps       uint64  `json:"read_ops,omitempty"`
	WriteOps      uint64  `json:"write_ops,omitempty"`
	IOUtilization float64 `json:"io_utilization_percent,omitempty"`

	// Mount information
	MountPoint   string  `json:"mount_point,omitempty"`
	UsagePercent float64 `json:"usage_percent,omitempty"`

	Timestamp time.Time `json:"timestamp"`
}

// SMARTAttribute represents a SMART attribute
type SMARTAttribute struct {
	ID         int    `json:"id"`
	Name       string `json:"name"`
	Value      int    `json:"value"`
	Worst      int    `json:"worst"`
	Threshold  int    `json:"threshold"`
	RawValue   string `json:"raw_value"`
	WhenFailed string `json:"when_failed,omitempty"`
}
