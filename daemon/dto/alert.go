package dto

import "time"

// AlertRule defines a user-configurable alert rule that evaluates expressions against cached system data.
type AlertRule struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	Expression      string   `json:"expression"`                 // expr-lang expression, e.g. "CPU > 90 && ArrayState == 'Started'"
	DurationSeconds int      `json:"duration_seconds,omitempty"` // Must be true for this many seconds before firing (0 = immediate)
	Severity        string   `json:"severity"`                   // "info", "warning", "critical"
	Channels        []string `json:"channels"`                   // shoutrrr URLs or "unraid" for local notification
	Enabled         bool     `json:"enabled"`
	CooldownMinutes int      `json:"cooldown_minutes,omitempty"` // Minutes between re-fires (default 5)
}

// AlertEvent represents a state transition of an alert (firing or resolved).
type AlertEvent struct {
	RuleID     string    `json:"rule_id"`
	RuleName   string    `json:"rule_name"`
	Severity   string    `json:"severity"`
	State      string    `json:"state"` // "firing" or "resolved"
	Message    string    `json:"message"`
	FiredAt    time.Time `json:"fired_at"`
	ResolvedAt time.Time `json:"resolved_at"`
}

// AlertStatus represents the current state of a single alert rule.
type AlertStatus struct {
	RuleID    string    `json:"rule_id"`
	RuleName  string    `json:"rule_name"`
	State     string    `json:"state"` // "ok", "pending", "firing"
	Severity  string    `json:"severity"`
	Since     time.Time `json:"since"`
	EvalCount int64     `json:"eval_count"`
	Message   string    `json:"message,omitempty"`
}

// AlertEnv is the typed evaluation environment for expr-lang expressions.
// Fields are populated from cached collector data before each evaluation cycle.
// The source tag names the collectors whose data feeds each field (comma
// separated when several do), or "-" when no collector does. Enabled rules keep
// those collectors running at their default interval in Dynamic collection mode.
type AlertEnv struct {
	// System
	CPU             float64 `expr:"CPU" source:"system"`
	RAMUsedPct      float64 `expr:"RAMUsedPct" source:"system"`
	RAMTotalBytes   uint64  `expr:"RAMTotalBytes" source:"system"`
	RAMUsedBytes    uint64  `expr:"RAMUsedBytes" source:"system"`
	RAMFreeBytes    uint64  `expr:"RAMFreeBytes" source:"system"`
	SwapUsedPct     float64 `expr:"SwapUsedPct" source:"system"`
	SwapTotalBytes  uint64  `expr:"SwapTotalBytes" source:"system"`
	SwapUsedBytes   uint64  `expr:"SwapUsedBytes" source:"system"`
	SwapFreeBytes   uint64  `expr:"SwapFreeBytes" source:"system"`
	CPUTemp         float64 `expr:"CPUTemp" source:"system"`
	MotherboardTemp float64 `expr:"MotherboardTemp" source:"system"`
	Uptime          int64   `expr:"Uptime" source:"system"`

	// Array
	ArrayState          string  `expr:"ArrayState" source:"array"`
	ArrayUsedPct        float64 `expr:"ArrayUsedPct" source:"array"`
	ArrayFreeBytes      uint64  `expr:"ArrayFreeBytes" source:"array"`
	ArrayTotalBytes     uint64  `expr:"ArrayTotalBytes" source:"array"`
	ParityValid         bool    `expr:"ParityValid" source:"array"`
	ParityCheckStatus   string  `expr:"ParityCheckStatus" source:"array"`
	ParityCheckProgress float64 `expr:"ParityCheckProgress" source:"array"`
	NumDisks            int     `expr:"NumDisks" source:"array"`
	NumParityDisks      int     `expr:"NumParityDisks" source:"array"`

	// Aggregated
	ContainerCount            int     `expr:"ContainerCount" source:"docker"`
	RunningContainers         int     `expr:"RunningContainers" source:"docker"`
	StoppedContainers         int     `expr:"StoppedContainers" source:"docker"`
	ContainerUpdatesAvailable int     `expr:"ContainerUpdatesAvailable" source:"docker,docker_update"`
	PluginUpdatesAvailable    int     `expr:"PluginUpdatesAvailable" source:"plugin_update"`
	VMCount                   int     `expr:"VMCount" source:"vm"`
	RunningVMs                int     `expr:"RunningVMs" source:"vm"`
	MaxDiskTemp               float64 `expr:"MaxDiskTemp" source:"disk"`
	MaxDiskUsedPct            float64 `expr:"MaxDiskUsedPct" source:"disk"`
	TotalDiskErrors           int     `expr:"TotalDiskErrors" source:"disk"`
	UPSStatus                 string  `expr:"UPSStatus" source:"ups"`
	// UPS and NUT readings are nil when there is no UPS or it does not report
	// the value. Guard them in expressions, e.g.
	// "UPSLoadPercent != nil && UPSLoadPercent > 80". An unguarded comparison
	// with nil fails to evaluate: the rule never fires on a fabricated 0, keeps
	// its current state (a firing rule stays firing) and logs one warning until
	// the reading is known again.
	UPSBatteryCharge *float64 `expr:"UPSBatteryCharge" source:"ups"`
	UPSLoadPercent   *float64 `expr:"UPSLoadPercent" source:"ups"`
	UPSRuntimeLeft   *float64 `expr:"UPSRuntimeLeft" source:"ups"`

	// GPU
	GPUCount      int     `expr:"GPUCount" source:"gpu"`
	MaxGPUTemp    float64 `expr:"MaxGPUTemp" source:"gpu"`
	MaxGPUUtil    float64 `expr:"MaxGPUUtil" source:"gpu"`
	TotalGPUPower float64 `expr:"TotalGPUPower" source:"gpu"`

	// ZFS
	ZFSPoolCount      int     `expr:"ZFSPoolCount" source:"zfs"`
	MaxZFSPoolUsedPct float64 `expr:"MaxZFSPoolUsedPct" source:"zfs"`
	ZFSDegradedPools  int     `expr:"ZFSDegradedPools" source:"zfs"`
	ZFSFaultedPools   int     `expr:"ZFSFaultedPools" source:"zfs"`
	ZFSCorruptedFiles int     `expr:"ZFSCorruptedFiles" source:"zfs"` // Total corrupted files across all pools
	BootPoolHealthy   bool    `expr:"BootPoolHealthy" source:"zfs"`   // True when the ZFS boot pool is ONLINE (or no boot pool)
	BootPoolHealth    string  `expr:"BootPoolHealth" source:"zfs"`    // Boot pool health ("ONLINE", "DEGRADED", ...); empty when no ZFS boot pool exists

	// Network
	NetworkErrors  uint64 `expr:"NetworkErrors" source:"network"`
	NetworkIFCount int    `expr:"NetworkIFCount" source:"network"`

	// NUT (Network UPS Tools). Readings are nil when not reported; see the UPS
	// fields above.
	NUTBatteryCharge  *float64 `expr:"NUTBatteryCharge" source:"nut"`
	NUTBatteryRuntime *int     `expr:"NUTBatteryRuntime" source:"nut"`
	NUTLoadPercent    *float64 `expr:"NUTLoadPercent" source:"nut"`
	NUTStatus         string   `expr:"NUTStatus" source:"nut"`

	// Notifications
	UnreadNotifications  int `expr:"UnreadNotifications" source:"notification"`
	WarningNotifications int `expr:"WarningNotifications" source:"notification"`
	AlertNotifications   int `expr:"AlertNotifications" source:"notification"`

	// Trends (derived from MetricsHistory)
	ArrayFillETAHours           float64 `expr:"ArrayFillETAHours" source:"array"`
	MaxDiskFillETAHours         float64 `expr:"MaxDiskFillETAHours" source:"disk"`
	CPUTempSlopePerMin          float64 `expr:"CPUTempSlopePerMin" source:"system"`
	MaxDiskTempSlopePerMin      float64 `expr:"MaxDiskTempSlopePerMin" source:"disk"`
	MaxContainerRestartsPerHour float64 `expr:"MaxContainerRestartsPerHour" source:"docker"`
	MaxReallocatedSectors       int     `expr:"MaxReallocatedSectors" source:"disk"`
	MaxPendingSectors           int     `expr:"MaxPendingSectors" source:"disk"`
	DiskErrorsIncreasing        bool    `expr:"DiskErrorsIncreasing" source:"disk"`

	// OS-resilience: number of data sources currently not healthy.
	DegradedSubsystemCount int `expr:"DegradedSubsystemCount" source:"-"`
}

// AlertRulesConfig is the top-level structure persisted to the JSON config file.
type AlertRulesConfig struct {
	Rules []AlertRule `json:"rules"`
}

// AlertsStatusResponse contains the current status of all alert rules.
type AlertsStatusResponse struct {
	Statuses []AlertStatus `json:"statuses"`
}

// AlertHistoryResponse contains recent alert events.
type AlertHistoryResponse struct {
	Events []AlertEvent `json:"events"`
	Total  int          `json:"total"`
}
