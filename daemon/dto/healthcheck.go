package dto

import "time"

// HealthCheckType represents the type of health check probe.
type HealthCheckType string

const (
	// HealthCheckHTTP performs an HTTP GET and checks the status code.
	HealthCheckHTTP HealthCheckType = "http"

	// HealthCheckTCP attempts a TCP connection to host:port.
	HealthCheckTCP HealthCheckType = "tcp"

	// HealthCheckContainer checks if a Docker container is running.
	HealthCheckContainer HealthCheckType = "container"

	// HealthCheckPing sends an ICMP ping (via the ping binary) and checks for a response.
	HealthCheckPing HealthCheckType = "ping"
)

// HealthCheck defines a user-configured health check probe.
type HealthCheck struct {
	// ID is the unique identifier for this health check.
	ID string `json:"id"`

	// Name is a human-readable name for this health check.
	Name string `json:"name"`

	// Type is the probe type: "http", "tcp", "container", or "ping".
	Type HealthCheckType `json:"type"`

	// Target is the probe target: URL for HTTP, host:port for TCP, container ID/name for container, hostname/IP for ping.
	Target string `json:"target"`

	// IntervalSeconds is how often the check runs (minimum 10, default 30).
	IntervalSeconds int `json:"interval_seconds"`

	// TimeoutSeconds is the probe timeout (minimum 1, default 5).
	TimeoutSeconds int `json:"timeout_seconds"`

	// SuccessCode is the expected HTTP status code (HTTP probes only, default 200).
	SuccessCode int `json:"success_code,omitempty"`

	// OnFail is the remediation action: "notify", "restart_container:<name>", or "webhook:<url>".
	OnFail string `json:"on_fail"`

	// Enabled determines whether this health check is active.
	Enabled bool `json:"enabled"`
}

// HealthCheckStatus represents the current status of a health check.
type HealthCheckStatus struct {
	// CheckID is the health check identifier.
	CheckID string `json:"check_id"`

	// CheckName is the human-readable check name.
	CheckName string `json:"check_name"`

	// CheckType is the probe type.
	CheckType HealthCheckType `json:"check_type"`

	// Target is the probe target.
	Target string `json:"target"`

	// Healthy is true when the last probe succeeded.
	Healthy bool `json:"healthy"`

	// LastCheck is the timestamp of the last probe execution.
	LastCheck time.Time `json:"last_check"`

	// LastError is the error message from the last failed probe, empty if healthy.
	LastError string `json:"last_error,omitempty"`

	// ConsecutiveFails is the number of consecutive probe failures.
	ConsecutiveFails int `json:"consecutive_fails"`

	// LastRemediation is the timestamp of the last remediation action taken.
	LastRemediation *time.Time `json:"last_remediation,omitempty"`

	// RemediationAction is the configured on_fail action.
	RemediationAction string `json:"remediation_action"`
}

// HealthCheckEvent represents a state change event in a health check.
type HealthCheckEvent struct {
	// CheckID is the health check identifier.
	CheckID string `json:"check_id"`

	// CheckName is the human-readable check name.
	CheckName string `json:"check_name"`

	// State is "healthy" or "unhealthy".
	State string `json:"state"`

	// Message describes the event.
	Message string `json:"message"`

	// Timestamp is when the event occurred.
	Timestamp time.Time `json:"timestamp"`

	// RemediationTaken is the action that was executed, if any.
	RemediationTaken string `json:"remediation_taken,omitempty"`
}

// HealthChecksConfig is the on-disk JSON structure for health check persistence.
type HealthChecksConfig struct {
	Checks []HealthCheck `json:"checks"`
}

// HealthChecksStatusResponse is the API response for health check statuses.
type HealthChecksStatusResponse struct {
	Checks []HealthCheckStatus `json:"checks"`
}

// HealthCheckHistoryResponse is the API response for health check event history.
type HealthCheckHistoryResponse struct {
	Events []HealthCheckEvent `json:"events"`
}
