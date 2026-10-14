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
	// SourceDisabled means the underlying service is intentionally turned off in
	// Unraid settings (e.g. Docker or the VM manager). This is a normal,
	// non-error condition: the subsystem correctly serves empty data and is not
	// counted as degraded. It is distinct from SourceUnavailable, which means the
	// service should be running but cannot be reached.
	SourceDisabled SourceState = "disabled"
)

// Severity orders states for "worst-of" rollups: disabled/healthy < degraded <
// unavailable. SourceDisabled ranks alongside healthy (severity 0) because an
// intentionally-disabled service is not a fault.
func (s SourceState) Severity() int {
	switch s {
	case SourceDegraded:
		return 1
	case SourceUnavailable:
		return 2
	default:
		// SourceHealthy and SourceDisabled
		return 0
	}
}

// SourceStatus is the health of one subsystem's data source.
type SourceStatus struct {
	Subsystem   string      `json:"subsystem"`
	State       SourceState `json:"state"`
	Reason      string      `json:"reason,omitempty"`
	LastChecked time.Time   `json:"last_checked" format:"date-time"`
	// LastHealthy is the RFC3339 timestamp of when the subsystem last reported
	// healthy. It is preserved across degradations so a persistent degraded
	// state can be dated. Omitted from JSON when the subsystem has never been
	// healthy.
	LastHealthy time.Time `json:"last_healthy,omitzero" format:"date-time"`
	LastError   string    `json:"last_error,omitempty"`
}

// CapabilityState is the outcome of one capability check.
type CapabilityState string

const (
	// CapabilityPresent means the capability (path, binary or hardware) was found.
	CapabilityPresent CapabilityState = "present"
	// CapabilityAbsent means the check ran and found nothing.
	CapabilityAbsent CapabilityState = "absent"
	// CapabilityUnknown means the check could not decide (read error, timeout,
	// panic). Callers must not act on an unknown result.
	CapabilityUnknown CapabilityState = "unknown"
)

// Capability is one probed capability: an OS binary or path, or a hardware
// class such as "hardware.nic_optics".
type Capability struct {
	Name      string `json:"name" example:"hardware.storage_topology"`
	Available bool   `json:"available" example:"true"`
	// State is present, absent or unknown. Available is true only when present.
	State  CapabilityState `json:"state,omitempty" enums:"present,absent,unknown" example:"present"`
	Target string          `json:"target,omitempty"`
	// Detail explains the result, e.g. "SAS controller at 0000:01:00.0".
	Detail string `json:"detail,omitempty" example:"SAS controller at 0000:01:00.0"`
}

// Capabilities is the periodically refreshed capability snapshot.
type Capabilities struct {
	UnraidVersion string       `json:"unraid_version" example:"7.3.3"`
	Items         []Capability `json:"items"`
	// CheckedAt is when the snapshot was taken.
	CheckedAt time.Time `json:"checked_at" format:"date-time"`
}

// Item returns the capability with the given name.
func (c Capabilities) Item(name string) (Capability, bool) {
	for _, it := range c.Items {
		if it.Name == name {
			return it, true
		}
	}
	return Capability{}, false
}
