package dto

import "time"

// SourceState describes the health of a data source backing a subsystem.
type SourceState string

const (
	// SourceHealthy means the source was read and shape-validated successfully.
	SourceHealthy SourceState = "healthy"
	// SourceDegraded means the source was read but failed a sanity/shape check;
	// best-effort partial data is still served.
	SourceDegraded SourceState = "degraded"
	// SourceUnavailable means the source or its binary is absent.
	SourceUnavailable SourceState = "unavailable"
)

// Severity orders states for "worst-of" rollups: healthy < degraded < unavailable.
func (s SourceState) Severity() int {
	switch s {
	case SourceDegraded:
		return 1
	case SourceUnavailable:
		return 2
	default:
		return 0
	}
}

// SourceStatus is the health of one subsystem's data source.
type SourceStatus struct {
	Subsystem   string      `json:"subsystem"`
	State       SourceState `json:"state"`
	Reason      string      `json:"reason,omitempty"`
	LastChecked time.Time   `json:"last_checked"`
	LastError   string      `json:"last_error,omitempty"`
}

// Capability is one probed OS capability (a binary or a path).
type Capability struct {
	Name      string `json:"name"`
	Available bool   `json:"available"`
	Target    string `json:"target,omitempty"`
	Detail    string `json:"detail,omitempty"`
}

// Capabilities is the startup probe snapshot.
type Capabilities struct {
	UnraidVersion string       `json:"unraid_version"`
	Items         []Capability `json:"items"`
}
