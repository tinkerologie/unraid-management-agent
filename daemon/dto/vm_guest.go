package dto

import "time"

// VMGuestInfo is data read from inside a VM through the QEMU guest agent.
// Only VMs listed in the vm_guest_agent configuration are queried. While a VM
// is not running, the GPUs and disks seen last keep their entries with every
// reading set to null, so consumers can show them as unknown rather than gone.
type VMGuestInfo struct {
	VMName         string              `json:"vm_name" example:"Windows 11"`
	Running        bool                `json:"running" example:"true"`
	AgentConnected bool                `json:"agent_connected" example:"true"`
	Hostname       string              `json:"hostname,omitempty" example:"WIN11"`
	OSName         string              `json:"os_name,omitempty" example:"Microsoft Windows 11"`
	Filesystems    []VMGuestFilesystem `json:"filesystems"`
	GPUs           []VMGuestGPU        `json:"gpus"`
	Disks          []VMGuestDisk       `json:"disks"`
	Sensors        []VMGuestSensor     `json:"sensors"`
	Error          string              `json:"error,omitempty" example:"guest agent not responding"`
	Timestamp      time.Time           `json:"timestamp"`
}

// VMGuestFilesystem is a mounted filesystem inside the guest.
type VMGuestFilesystem struct {
	Mountpoint string  `json:"mountpoint" example:"C:\\"`
	Type       string  `json:"type" example:"NTFS"`
	TotalBytes *uint64 `json:"total_bytes" example:"1073741819904" extensions:"x-nullable"`
	UsedBytes  *uint64 `json:"used_bytes" example:"173512880128" extensions:"x-nullable"`
}

// VMGuestGPU is an NVIDIA GPU passed through to the guest, read with nvidia-smi.
// A reading is null when the VM is off or nvidia-smi reports it as N/A.
type VMGuestGPU struct {
	Index              int      `json:"index" example:"0"`
	Name               string   `json:"name" example:"NVIDIA GeForce RTX 4090"`
	TemperatureC       *float64 `json:"temperature_celsius" example:"34" extensions:"x-nullable"`
	UtilizationPercent *float64 `json:"utilization_percent" example:"12" extensions:"x-nullable"`
	MemoryUsedBytes    *uint64  `json:"memory_used_bytes" example:"13631488" extensions:"x-nullable"`
	MemoryTotalBytes   *uint64  `json:"memory_total_bytes" example:"25757220864" extensions:"x-nullable"`
	PowerWatts         *float64 `json:"power_watts" example:"63.29" extensions:"x-nullable"`
	FanPercent         *float64 `json:"fan_percent" example:"0" extensions:"x-nullable"`
	ClockMHz           *float64 `json:"clock_mhz" example:"2535" extensions:"x-nullable"`
}

// VMGuestDisk is a physical disk seen by a Windows guest, read with
// Get-StorageReliabilityCounter.
type VMGuestDisk struct {
	Model           string   `json:"model" example:"WD_BLACK SN850X 4000GB"`
	Serial          string   `json:"serial" example:"E823_8FA6_BF53_0001_001B_448B_4EB4_93E5."`
	Bus             string   `json:"bus" example:"NVMe"`
	TemperatureC    *float64 `json:"temperature_celsius" example:"35" extensions:"x-nullable"`
	TemperatureMaxC *float64 `json:"temperature_max_celsius" example:"90" extensions:"x-nullable"`
	WearPercent     *float64 `json:"wear_percent" example:"1" extensions:"x-nullable"`
}

// VMGuestSensor is the output of a custom command configured to run inside the
// guest. Value is the trimmed standard output, or null when the VM is off or
// the last run failed. A sensor configured with json: true is listed once per
// key of the JSON object its command prints, named <name>_<key>. Attributes
// holds the keys configured as attributes of that sensor, null when missing or
// the last run failed.
type VMGuestSensor struct {
	Name      string     `json:"name" example:"fps"`
	Value     *string    `json:"value" example:"144" extensions:"x-nullable"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
	Error     string     `json:"error,omitempty" example:"exit code 1"`
	// Attributes are extra values of a json: true sensor, by JSON key.
	Attributes map[string]*string `json:"attributes,omitempty"`
}
