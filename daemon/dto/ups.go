package dto

import "time"

// UPSStatus contains UPS status information
type UPSStatus struct {
	Connected     bool      `json:"connected" example:"true"`
	Status        string    `json:"status" example:"OL"`
	LoadPercent   float64   `json:"load_percent" example:"25.5"`
	BatteryCharge float64   `json:"battery_charge_percent" example:"100"`
	RuntimeLeft   int       `json:"runtime_left_seconds" example:"3600"`
	PowerWatts    float64   `json:"power_watts" example:"250.5"`
	NominalPower  float64   `json:"nominal_power_watts" example:"1000"`
	Model         string    `json:"model" example:"APC Smart-UPS 1500"`
	Timestamp     time.Time `json:"timestamp"`
}
