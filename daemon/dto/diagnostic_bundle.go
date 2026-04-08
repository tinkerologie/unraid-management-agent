package dto

// DiagnosticBundle represents a complete diagnostic data collection for troubleshooting.
type DiagnosticBundle struct {
	Metadata      BundleMetadata      `json:"metadata"`
	SystemState   BundleSystemState   `json:"system_state"`
	ArrayStatus   BundleArrayStatus   `json:"array_status"`
	Containers    []BundleContainer   `json:"containers"`
	VMs           []BundleVM          `json:"vms"`
	Network       []BundleNetwork     `json:"network"`
	Logs          BundleLogs          `json:"logs"`
	Configuration BundleConfiguration `json:"configuration"`
}

// BundleMetadata holds metadata about the diagnostic bundle.
type BundleMetadata struct {
	Timestamp     string `json:"timestamp"`
	AgentVersion  string `json:"agent_version"`
	UnraidVersion string `json:"unraid_version,omitempty"`
	Hostname      string `json:"hostname"`
	KernelVersion string `json:"kernel_version,omitempty"`
}

// BundleSystemState holds system metrics at the time of bundle creation.
type BundleSystemState struct {
	CPUUsage     float64             `json:"cpu_usage"`
	CPUModel     string              `json:"cpu_model,omitempty"`
	RAMUsage     float64             `json:"ram_usage"`
	RAMTotalMB   float64             `json:"ram_total_mb"`
	RAMUsedMB    float64             `json:"ram_used_mb"`
	Uptime       string              `json:"uptime,omitempty"`
	Temperatures []BundleTemperature `json:"temperatures,omitempty"`
}

// BundleTemperature holds a temperature sensor reading.
type BundleTemperature struct {
	Label    string  `json:"label"`
	Value    float64 `json:"value"`
	Category string  `json:"category,omitempty"`
}

// BundleArrayStatus holds Unraid array state.
type BundleArrayStatus struct {
	State       string       `json:"state"`
	TotalDisks  int          `json:"total_disks"`
	ActiveDisks int          `json:"active_disks,omitempty"`
	Disks       []BundleDisk `json:"disks,omitempty"`
}

// BundleDisk holds per-disk diagnostic information.
type BundleDisk struct {
	Name        string  `json:"name"`
	Device      string  `json:"device,omitempty"`
	Status      string  `json:"status"`
	Temperature float64 `json:"temperature,omitempty"`
	Size        int64   `json:"size,omitempty"`
	Used        int64   `json:"used,omitempty"`
	SMARTStatus string  `json:"smart_status,omitempty"`
}

// BundleContainer holds a Docker container summary.
type BundleContainer struct {
	Name   string `json:"name"`
	Image  string `json:"image,omitempty"`
	State  string `json:"state"`
	Status string `json:"status,omitempty"`
}

// BundleVM holds a VM summary.
type BundleVM struct {
	Name  string `json:"name"`
	State string `json:"state"`
}

// BundleNetwork holds a network interface summary.
type BundleNetwork struct {
	Name   string `json:"name"`
	Status string `json:"status,omitempty"`
	IPAddr string `json:"ip_addr,omitempty"`
	Speed  string `json:"speed,omitempty"`
}

// BundleLogs holds collected log entries.
type BundleLogs struct {
	DiagnosticEntries []DiagnosticLogEntry `json:"diagnostic_entries,omitempty"`
	AgentLog          []string             `json:"agent_log,omitempty"`
	SysLog            []string             `json:"sys_log,omitempty"`
}

// BundleConfiguration holds redacted configuration values.
type BundleConfiguration struct {
	CollectorIntervals map[string]int `json:"collector_intervals,omitempty"`
	MQTTConfig         map[string]any `json:"mqtt_config,omitempty"`
	Port               int            `json:"port"`
	Version            string         `json:"version"`
}
