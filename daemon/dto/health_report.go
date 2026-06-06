package dto

import "time"

// ActionRef describes a single recommended remediation action.
type ActionRef struct {
	Action string `json:"action" example:"restart_container"`
	Target string `json:"target" example:"abc123def456"`
	Reason string `json:"reason,omitempty"`
}

// HealthFinding is a single prioritized finding in a health report.
type HealthFinding struct {
	Severity           string      `json:"severity" example:"warning"` // info|warning|critical
	Title              string      `json:"title"`
	Detail             string      `json:"detail"`
	RecommendedActions []ActionRef `json:"recommended_actions,omitempty"`
}

// HealthReport is the aggregate result of BuildHealthReport.
type HealthReport struct {
	Findings    []HealthFinding `json:"findings"`
	Critical    int             `json:"critical_count"`
	Warning     int             `json:"warning_count"`
	Info        int             `json:"info_count"`
	GeneratedAt time.Time       `json:"generated_at"`

	// DegradedSubsystems summarizes data sources whose health is not "healthy"
	// (OS-resilience). Omitted when all sources are healthy.
	DegradedSubsystems *DegradedSubsystems `json:"degraded_subsystems,omitempty"`
}

// DegradedSubsystems is the health-report rollup of non-healthy data sources.
type DegradedSubsystems struct {
	Count int            `json:"count"`
	Items []SourceStatus `json:"items"`
}

// ActionResult records the outcome of a single executor action.
type ActionResult struct {
	Action     string `json:"action"`
	Target     string `json:"target"`
	Succeeded  bool   `json:"succeeded"`
	DurationMs int64  `json:"duration_ms"`
	Error      string `json:"error,omitempty"`
}
