package dto

import "time"

// FanControlMode represents the PWM control mode of a fan.
type FanControlMode string

const (
	// FanModeAutomatic means the hardware manages fan speed via firmware/BIOS curves.
	FanModeAutomatic FanControlMode = "automatic"
	// FanModeManual means software is controlling the PWM duty cycle directly.
	FanModeManual FanControlMode = "manual"
	// FanModeOff means PWM output is disabled (fan runs at full speed).
	FanModeOff FanControlMode = "off"
)

// FanControlMethod represents the underlying control mechanism.
type FanControlMethod string

const (
	// FanMethodHwmon uses Linux hwmon sysfs for PWM control.
	FanMethodHwmon FanControlMethod = "hwmon"
	// FanMethodIPMI uses IPMI raw commands for BMC-level fan control.
	FanMethodIPMI FanControlMethod = "ipmi"
)

// FanTempSourceType identifies where a fan curve reads its temperature.
type FanTempSourceType string

const (
	// FanTempSourceHwmon reads a single hwmon sysfs temperature input.
	FanTempSourceHwmon FanTempSourceType = "hwmon"
	// FanTempSourceDrives reads the max temperature across selected active drives.
	FanTempSourceDrives FanTempSourceType = "drives"
)

// FanTempSource describes a fan curve's temperature input. For Type=="drives"
// the engine uses the max temperature of the active (non-standby) DriveIDs and
// falls back to FallbackSensorPath when they are all spun down.
type FanTempSource struct {
	Type               FanTempSourceType `json:"type" example:"drives"`
	SensorPath         string            `json:"sensor_path,omitempty" example:"/sys/class/hwmon/hwmon0/temp1_input"`
	DriveIDs           []string          `json:"drive_ids,omitempty" example:"disk1,disk2"`
	FallbackSensorPath string            `json:"fallback_sensor_path,omitempty" example:"/sys/class/hwmon/hwmon0/temp1_input"`
}

// AvailableTempSensor is a discoverable hwmon temperature sensor.
type AvailableTempSensor struct {
	Path      string  `json:"path" example:"/sys/class/hwmon/hwmon0/temp1_input"`
	Label     string  `json:"label,omitempty" example:"Tctl"`
	TempC     float64 `json:"temp_celsius" example:"45"`
	Plausible bool    `json:"plausible" example:"true"`
}

// AvailableDriveSensor is a discoverable drive temperature source.
type AvailableDriveSensor struct {
	ID       string  `json:"id" example:"disk1"`
	Device   string  `json:"device,omitempty" example:"sdb"`
	TempC    float64 `json:"temp_celsius" example:"38"`
	SpunDown bool    `json:"spun_down" example:"false"`
}

// FanSensorCatalog lists everything a fan curve can be pointed at.
type FanSensorCatalog struct {
	HwmonSensors []AvailableTempSensor  `json:"hwmon_sensors"`
	Drives       []AvailableDriveSensor `json:"drives"`
	Timestamp    time.Time              `json:"timestamp"`
}

// FanDevice represents a single fan with monitoring and control state.
type FanDevice struct {
	ID             string         `json:"id" example:"hwmon0_fan1"`
	Name           string         `json:"name" example:"CPU Fan"`
	RPM            int            `json:"rpm" example:"1200"`
	PWMValue       int            `json:"pwm_value" example:"180"`
	PWMPercent     int            `json:"pwm_percent" example:"71"`
	Mode           FanControlMode `json:"mode" example:"automatic"`
	Controllable   bool           `json:"controllable" example:"true"`
	HwmonPath      string         `json:"hwmon_path,omitempty" example:"/sys/class/hwmon/hwmon0"`
	HwmonIndex     int            `json:"hwmon_index,omitempty" example:"1"`
	ActiveProfile  string         `json:"active_profile,omitempty" example:"balanced"`
	TempSensorPath string         `json:"temp_sensor_path,omitempty" example:"/sys/class/hwmon/hwmon0/temp1_input"`
	TempSource     *FanTempSource `json:"temp_source,omitempty"`
}

// FanCurvePoint defines a temperature-to-speed mapping point.
type FanCurvePoint struct {
	TempCelsius  float64 `json:"temp_celsius" example:"40"`
	SpeedPercent int     `json:"speed_percent" example:"30"`
}

// FanProfile defines a named set of fan curve points.
type FanProfile struct {
	Name        string          `json:"name" example:"balanced"`
	Description string          `json:"description" example:"Balanced cooling and noise"`
	CurvePoints []FanCurvePoint `json:"curve_points"`
	BuiltIn     bool            `json:"built_in" example:"true"`
}

// FanSafetyConfig holds safety thresholds for fan control.
type FanSafetyConfig struct {
	MinSpeedPercent     int     `json:"min_speed_percent" example:"20"`
	CriticalTempC       float64 `json:"critical_temp_celsius" example:"90"`
	FailureRPMThreshold int     `json:"failure_rpm_threshold" example:"100"`
}

// FanControlConfig holds the overall fan control configuration.
type FanControlConfig struct {
	Enabled        bool             `json:"enabled" example:"true"`
	ControlEnabled bool             `json:"control_enabled" example:"false"`
	ControlMethod  FanControlMethod `json:"control_method" example:"hwmon"`
	PollInterval   int              `json:"poll_interval_seconds" example:"5"`
	Safety         FanSafetyConfig  `json:"safety"`
}

// FanControlSummary provides an overview of the fan control state.
type FanControlSummary struct {
	TotalFans        int      `json:"total_fans" example:"3"`
	ControllableFans int      `json:"controllable_fans" example:"2"`
	FailedFans       []string `json:"failed_fans,omitempty"`
}

// FanControlStatus is the top-level DTO published by the fan control collector.
type FanControlStatus struct {
	Fans      []FanDevice       `json:"fans"`
	Profiles  []FanProfile      `json:"profiles"`
	Config    FanControlConfig  `json:"config"`
	Summary   FanControlSummary `json:"summary"`
	Timestamp time.Time         `json:"timestamp"`
}

// FanSpeedRequest is the JSON body for setting a fan's PWM speed.
type FanSpeedRequest struct {
	FanID      string `json:"fan_id" example:"hwmon0_fan1"`
	PWMPercent int    `json:"pwm_percent" example:"50"`
}

// FanModeRequest is the JSON body for setting a fan's control mode.
type FanModeRequest struct {
	FanID string `json:"fan_id" example:"hwmon0_fan1"`
	Mode  string `json:"mode" example:"manual"`
}

// FanProfileRequest is the JSON body for assigning a profile to a fan.
type FanProfileRequest struct {
	FanID          string         `json:"fan_id" example:"hwmon0_fan1"`
	ProfileName    string         `json:"profile_name" example:"balanced"`
	TempSensorPath string         `json:"temp_sensor_path,omitempty" example:"/sys/class/hwmon/hwmon0/temp1_input"`
	Source         *FanTempSource `json:"source,omitempty"`
}

// FanProfileCreateRequest is the JSON body for creating a custom profile.
type FanProfileCreateRequest struct {
	Name        string          `json:"name" example:"custom_quiet"`
	Description string          `json:"description" example:"Custom quiet profile"`
	CurvePoints []FanCurvePoint `json:"curve_points"`
}
