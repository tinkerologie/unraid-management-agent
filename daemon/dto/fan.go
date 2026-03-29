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
	FanID          string `json:"fan_id" example:"hwmon0_fan1"`
	ProfileName    string `json:"profile_name" example:"balanced"`
	TempSensorPath string `json:"temp_sensor_path,omitempty" example:"/sys/class/hwmon/hwmon0/temp1_input"`
}

// FanProfileCreateRequest is the JSON body for creating a custom profile.
type FanProfileCreateRequest struct {
	Name        string          `json:"name" example:"custom_quiet"`
	Description string          `json:"description" example:"Custom quiet profile"`
	CurvePoints []FanCurvePoint `json:"curve_points"`
}
