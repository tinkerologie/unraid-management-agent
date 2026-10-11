package dto

import "time"

// NUTStatus contains detailed NUT (Network UPS Tools) status information.
// This provides more comprehensive UPS data than the basic UPSStatus struct.
//
// Numeric readings are pointers: a field is null in JSON when the device does
// not report the matching NUT variable (many UPSes have no ups.load or
// ups.realpower, and an ATS has no battery), instead of a fabricated 0.
type NUTStatus struct {
	// Connection and detection info
	Connected   bool   `json:"connected"`
	DeviceName  string `json:"device_name"`  // e.g., "ups"
	Host        string `json:"host"`         // e.g., "localhost" or remote IP
	Driver      string `json:"driver"`       // e.g., "usbhid-ups"
	DriverState string `json:"driver_state"` // e.g., "quiet", "running"

	// UPS identification
	Manufacturer string `json:"manufacturer"`
	Model        string `json:"model"`
	Serial       string `json:"serial"`
	Type         string `json:"type"` // e.g., "ups"

	// UPS status
	Status         string   `json:"status"`        // e.g., "OL" (Online), "OB" (On Battery)
	StatusText     string   `json:"status_text"`   // Human-readable status
	Alarms         []string `json:"alarms"`        // Active alarms if any
	BeeperStatus   string   `json:"beeper_status"` // e.g., "enabled", "disabled"
	TestResult     string   `json:"test_result"`   // Last test result
	TestResultDate string   `json:"test_result_date"`

	// Battery info
	BatteryCharge         *float64 `json:"battery_charge_percent" extensions:"x-nullable"`
	BatteryChargeLow      *float64 `json:"battery_charge_low_percent" extensions:"x-nullable"`
	BatteryChargeWarning  *float64 `json:"battery_charge_warning_percent" extensions:"x-nullable"`
	BatteryRuntime        *int     `json:"battery_runtime_seconds" extensions:"x-nullable"`
	BatteryRuntimeLow     *int     `json:"battery_runtime_low_seconds" extensions:"x-nullable"`
	BatteryVoltage        *float64 `json:"battery_voltage" extensions:"x-nullable"`
	BatteryVoltageNominal *float64 `json:"battery_voltage_nominal" extensions:"x-nullable"`
	BatteryType           string   `json:"battery_type"` // e.g., "PbAcid"
	BatteryStatus         string   `json:"battery_status"`
	BatteryMfrDate        string   `json:"battery_mfr_date"`

	// Input power
	InputVoltage        *float64 `json:"input_voltage" extensions:"x-nullable"`
	InputVoltageNominal *float64 `json:"input_voltage_nominal" extensions:"x-nullable"`
	InputFrequency      *float64 `json:"input_frequency" extensions:"x-nullable"`
	InputTransferHigh   *float64 `json:"input_transfer_high" extensions:"x-nullable"`
	InputTransferLow    *float64 `json:"input_transfer_low" extensions:"x-nullable"`
	InputCurrent        *float64 `json:"input_current" extensions:"x-nullable"`

	// Output power
	OutputVoltage   *float64 `json:"output_voltage" extensions:"x-nullable"`
	OutputFrequency *float64 `json:"output_frequency" extensions:"x-nullable"`
	OutputCurrent   *float64 `json:"output_current" extensions:"x-nullable"`

	// Load and power
	LoadPercent          *float64 `json:"load_percent" extensions:"x-nullable"`
	RealPower            *float64 `json:"realpower_watts" extensions:"x-nullable"`
	RealPowerNominal     *float64 `json:"realpower_nominal_watts" extensions:"x-nullable"`
	ApparentPower        *float64 `json:"apparent_power_va" extensions:"x-nullable"`
	ApparentPowerNominal *float64 `json:"apparent_power_nominal_va" extensions:"x-nullable"`

	// Timing
	DelayShutdown *int `json:"delay_shutdown_seconds" extensions:"x-nullable"`
	DelayStart    *int `json:"delay_start_seconds" extensions:"x-nullable"`
	TimerShutdown *int `json:"timer_shutdown" extensions:"x-nullable"`
	TimerStart    *int `json:"timer_start" extensions:"x-nullable"`

	// Driver info
	DriverVersion     string `json:"driver_version"`
	DriverVersionData string `json:"driver_version_data"`
	DriverVersionUSB  string `json:"driver_version_usb"`
	ProductID         string `json:"product_id"`
	VendorID          string `json:"vendor_id"`

	// Raw variables for advanced users
	RawVariables map[string]string `json:"raw_variables,omitempty"`

	// Metadata
	Timestamp time.Time `json:"timestamp"`
}

// NUTConfig represents the NUT plugin configuration from nut-dw.cfg
type NUTConfig struct {
	ServiceEnabled bool   `json:"service_enabled"`
	Mode           string `json:"mode"`          // "standalone", "netserver", "netclient"
	UPSName        string `json:"ups_name"`      // e.g., "ups"
	Driver         string `json:"driver"`        // e.g., "usbhid-ups"
	Port           string `json:"port"`          // e.g., "auto"
	IPAddress      string `json:"ip_address"`    // For netclient mode
	PollInterval   int    `json:"poll_interval"` // Seconds between polls
	ShutdownMode   string `json:"shutdown_mode"` // e.g., "sec_timer", "fsd"
	BatteryLevel   int    `json:"battery_level"` // Low battery threshold
	RuntimeValue   int    `json:"runtime_value"` // Low runtime threshold
	Timeout        int    `json:"timeout"`       // Shutdown timeout
}

// NUTDevice represents a single NUT UPS device
type NUTDevice struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Available   bool   `json:"available"`
}

// NUTResponse is the complete response for the /api/v1/nut endpoint
type NUTResponse struct {
	Installed bool        `json:"installed"` // Is NUT plugin installed?
	Running   bool        `json:"running"`   // Is NUT service running?
	Config    *NUTConfig  `json:"config,omitempty"`
	Devices   []NUTDevice `json:"devices,omitempty"`
	// Status of the first device in Devices (the primary UPS), kept for
	// clients that only know one device. It is also the first entry of
	// Statuses when that device answered.
	Status *NUTStatus `json:"status,omitempty"`
	// Statuses has the detailed status of every device in Devices that
	// answered, in the same order (e.g. a second UPS or an ATS).
	Statuses  []*NUTStatus `json:"statuses,omitempty"`
	Timestamp time.Time    `json:"timestamp"`
}

// NUTStatusText converts NUT status codes to human-readable text
func NUTStatusText(status string) string {
	statusMap := map[string]string{
		"OL":      "Online",
		"OB":      "On Battery",
		"LB":      "Low Battery",
		"HB":      "High Battery",
		"RB":      "Replace Battery",
		"CHRG":    "Charging",
		"DISCHRG": "Discharging",
		"BYPASS":  "Bypass",
		"CAL":     "Calibrating",
		"OFF":     "Offline",
		"OVER":    "Overloaded",
		"TRIM":    "Trimming Voltage",
		"BOOST":   "Boosting Voltage",
		"FSD":     "Forced Shutdown",
	}

	// Handle multiple status codes (e.g., "OL CHRG")
	if text, ok := statusMap[status]; ok {
		return text
	}
	return status
}
