package dto

import "time"

// UPSStatus contains UPS status information.
//
// Measurement fields are pointers. A field is null in JSON when the UPS (or
// apcupsd/NUT) does not report that value, so clients can show "unknown"
// instead of a reading of 0 that was never taken.
type UPSStatus struct {
	Connected bool `json:"connected" example:"true"`
	// NUT device name (upsc -l) when the data comes from NUT; empty for apcupsd.
	// GET /nut lists every NUT device; this is the first one.
	DeviceName string `json:"device_name,omitempty" example:"ups"`
	Status     string `json:"status" example:"OL"`
	// Load percentage; null when the UPS does not report it
	LoadPercent *float64 `json:"load_percent" example:"25.5" extensions:"x-nullable"`
	// Battery charge percentage; null when the UPS does not report it
	BatteryCharge *float64 `json:"battery_charge_percent" example:"100" extensions:"x-nullable"`
	// Battery runtime remaining in seconds; null when the UPS does not report it
	RuntimeLeft *int `json:"runtime_left_seconds" example:"3600" extensions:"x-nullable"`
	// Real power draw in watts; null unless the UPS reports it or both nominal power and load
	PowerWatts *float64 `json:"power_watts" example:"250.5" extensions:"x-nullable"`
	// Nominal power in watts; null when the UPS does not report it
	NominalPower *float64  `json:"nominal_power_watts" example:"1000" extensions:"x-nullable"`
	Model        string    `json:"model" example:"APC Smart-UPS 1500"`
	Timestamp    time.Time `json:"timestamp"`
}
