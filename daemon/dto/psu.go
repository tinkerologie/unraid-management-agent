package dto

import "time"

// PSU monitoring states distinguish host visibility from usable measurements.
const (
	PSUStatePending           = "pending"
	PSUStateNotDetected       = "not_detected"
	PSUStateDriverUnavailable = "driver_unavailable"
	PSUStateOK                = "ok"
	PSUStatePartial           = "partial"
	PSUStateError             = "error"
	PSUStateDisabled          = "disabled"
)

// PSUStatus is a fresh snapshot of host-visible PSU telemetry interfaces.
// Not detected does not imply that the server has no physical power supply.
type PSUStatus struct {
	State     string      `json:"state" example:"ok"`
	Devices   []PSUDevice `json:"devices"`
	Errors    []string    `json:"errors,omitempty"`
	Timestamp time.Time   `json:"timestamp"`
}

// PSUDevice describes one recognized power supply, including unavailable devices.
// IDs hash a serial plus model, falling back to physical USB port identity.
type PSUDevice struct {
	ID            string      `json:"id"`
	Vendor        string      `json:"vendor"`
	Model         string      `json:"model"`
	Family        string      `json:"family"`
	VendorID      string      `json:"vendor_id"`
	ProductID     string      `json:"product_id"`
	Backend       string      `json:"backend"`
	Driver        string      `json:"driver,omitempty"`
	State         string      `json:"state"`
	Message       string      `json:"message,omitempty"`
	CapacityWatts *float64    `json:"capacity_watts" extensions:"x-nullable"`
	Sensors       []PSUSensor `json:"sensors"`
}

// PSUSensor is a normalized channel. Missing/failed values are null, never zero.
// Name is stable within a device; Label is for display only.
type PSUSensor struct {
	Name    string   `json:"name"`
	Label   string   `json:"label"`
	Kind    string   `json:"kind" example:"power"`
	Unit    string   `json:"unit" example:"W"`
	Value   *float64 `json:"value" extensions:"x-nullable"`
	Derived bool     `json:"derived,omitempty"`
}

// NewPSUStatus creates an empty, explicit availability snapshot.
func NewPSUStatus(state string, now time.Time) *PSUStatus {
	return &PSUStatus{State: state, Devices: []PSUDevice{}, Timestamp: now}
}
