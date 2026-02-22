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
type AlertEnv struct {
	// System
	CPU             float64 `expr:"CPU"`
	RAMUsedPct      float64 `expr:"RAMUsedPct"`
	RAMTotalBytes   uint64  `expr:"RAMTotalBytes"`
	RAMUsedBytes    uint64  `expr:"RAMUsedBytes"`
	RAMFreeBytes    uint64  `expr:"RAMFreeBytes"`
	CPUTemp         float64 `expr:"CPUTemp"`
	MotherboardTemp float64 `expr:"MotherboardTemp"`
	Uptime          int64   `expr:"Uptime"`

	// Array
	ArrayState          string  `expr:"ArrayState"`
	ArrayUsedPct        float64 `expr:"ArrayUsedPct"`
	ArrayFreeBytes      uint64  `expr:"ArrayFreeBytes"`
	ArrayTotalBytes     uint64  `expr:"ArrayTotalBytes"`
	ParityValid         bool    `expr:"ParityValid"`
	ParityCheckStatus   string  `expr:"ParityCheckStatus"`
	ParityCheckProgress float64 `expr:"ParityCheckProgress"`
	NumDisks            int     `expr:"NumDisks"`
	NumParityDisks      int     `expr:"NumParityDisks"`

	// Aggregated
	ContainerCount    int     `expr:"ContainerCount"`
	RunningContainers int     `expr:"RunningContainers"`
	StoppedContainers int     `expr:"StoppedContainers"`
	VMCount           int     `expr:"VMCount"`
	RunningVMs        int     `expr:"RunningVMs"`
	MaxDiskTemp       float64 `expr:"MaxDiskTemp"`
	MaxDiskUsedPct    float64 `expr:"MaxDiskUsedPct"`
	TotalDiskErrors   int     `expr:"TotalDiskErrors"`
	UPSStatus         string  `expr:"UPSStatus"`
	UPSBatteryCharge  float64 `expr:"UPSBatteryCharge"`
	UPSLoadPercent    float64 `expr:"UPSLoadPercent"`
	UPSRuntimeLeft    float64 `expr:"UPSRuntimeLeft"`
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
