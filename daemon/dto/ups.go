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
	NominalPower *float64 `json:"nominal_power_watts" example:"1000" extensions:"x-nullable"`
	// Input (line) voltage in volts; null when the UPS does not report it
	InputVoltage *float64 `json:"input_voltage" example:"230" extensions:"x-nullable"`
	// Output voltage in volts; null when the UPS does not report it
	OutputVoltage *float64 `json:"output_voltage" example:"230" extensions:"x-nullable"`
	// Battery voltage in volts; null when the UPS does not report it
	BatteryVoltage *float64 `json:"battery_voltage" example:"27.1" extensions:"x-nullable"`
	// Input line frequency in hertz; null when the UPS does not report it
	LineFrequency *float64 `json:"line_frequency" example:"50" extensions:"x-nullable"`
	// Internal UPS temperature in degrees Celsius; null when the UPS does not
	// report it (or reports another unit)
	Temperature *float64 `json:"temperature_celsius" example:"29.2" extensions:"x-nullable"`
	// Seconds on battery in the current outage (0 on line power); null when the
	// UPS does not report it
	TimeOnBattery *int `json:"time_on_battery_seconds" example:"0" extensions:"x-nullable"`
	// Total seconds on battery since apcupsd started; apcupsd only, null for NUT
	CumulativeTimeOnBattery *int `json:"cumulative_time_on_battery_seconds" example:"0" extensions:"x-nullable"`
	// Number of transfers to battery; null when the UPS does not report it
	TransferCount *int `json:"transfer_count" example:"0" extensions:"x-nullable"`
	// Last battery self-test result as the backend reports it, not normalised
	// (apcupsd "NO", "OK", "NG"; NUT "Done and passed"); omitted when unknown
	SelfTest string `json:"self_test,omitempty" example:"OK"`
	// UPS serial number; omitted when the UPS does not report it
	SerialNumber string    `json:"serial_number,omitempty" example:"AS1234567890"`
	Model        string    `json:"model" example:"APC Smart-UPS 1500"`
	Timestamp    time.Time `json:"timestamp"`
}
