package dto

import (
	"testing"
)

func TestNUTStatusText(t *testing.T) {
	tests := []struct {
		name     string
		status   string
		expected string
	}{
		{"Online", "OL", "Online"},
		{"On Battery", "OB", "On Battery"},
		{"Low Battery", "LB", "Low Battery"},
		{"High Battery", "HB", "High Battery"},
		{"Replace Battery", "RB", "Replace Battery"},
		{"Charging", "CHRG", "Charging"},
		{"Discharging", "DISCHRG", "Discharging"},
		{"Bypass", "BYPASS", "Bypass"},
		{"Calibrating", "CAL", "Calibrating"},
		{"Offline", "OFF", "Offline"},
		{"Overloaded", "OVER", "Overloaded"},
		{"Trimming", "TRIM", "Trimming Voltage"},
		{"Boosting", "BOOST", "Boosting Voltage"},
		{"Forced Shutdown", "FSD", "Forced Shutdown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NUTStatusText(tt.status)
			if result != tt.expected {
				t.Errorf("NUTStatusText(%q) = %q, want %q", tt.status, result, tt.expected)
			}
		})
	}
}

func TestNUTStatusTextUnknown(t *testing.T) {
	// Test unknown status codes - they return the raw value
	tests := []struct {
		name   string
		status string
	}{
		{"Unknown status", "UNKNOWN"},
		{"Empty status", ""},
		{"Random text", "RANDOM"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NUTStatusText(tt.status)
			// Unknown statuses return themselves
			if result != tt.status {
				t.Errorf("NUTStatusText(%q) = %q, want %q", tt.status, result, tt.status)
			}
		})
	}
}

func TestNUTStatusStructure(t *testing.T) {
	// Test that NUTStatus struct can be created with expected fields
	status := NUTStatus{
		Connected:     true,
		DeviceName:    "ups",
		Host:          "localhost",
		Driver:        "usbhid-ups",
		Status:        "OL",
		StatusText:    "Online",
		BatteryCharge: 100.0,
		LoadPercent:   30.5,
	}

	if !status.Connected {
		t.Error("Expected Connected to be true")
	}
	if status.DeviceName != "ups" {
		t.Errorf("Expected DeviceName 'ups', got %q", status.DeviceName)
	}
	if status.BatteryCharge != 100.0 {
		t.Errorf("Expected BatteryCharge 100.0, got %f", status.BatteryCharge)
	}
}

func TestNUTConfigStructure(t *testing.T) {
	// Test NUTConfig struct fields
	config := NUTConfig{
		ServiceEnabled: true,
		Mode:           "standalone",
		UPSName:        "ups",
		Driver:         "usbhid-ups",
		Port:           "auto",
		PollInterval:   10,
		BatteryLevel:   20,
		RuntimeValue:   300,
	}

	if !config.ServiceEnabled {
		t.Error("Expected ServiceEnabled to be true")
	}
	if config.Mode != "standalone" {
		t.Errorf("Expected Mode 'standalone', got %q", config.Mode)
	}
	if config.PollInterval != 10 {
		t.Errorf("Expected PollInterval 10, got %d", config.PollInterval)
	}
}

func TestNUTDeviceStructure(t *testing.T) {
	// Test NUTDevice struct fields
	device := NUTDevice{
		Name:        "ups",
		Description: "Main UPS",
		Available:   true,
	}

	if device.Name != "ups" {
		t.Errorf("Expected Name 'ups', got %q", device.Name)
	}
	if !device.Available {
		t.Error("Expected Available to be true")
	}
}

func TestNUTResponseStructure(t *testing.T) {
	// Test NUTResponse struct fields
	response := NUTResponse{
		Installed: true,
		Running:   true,
		Config: &NUTConfig{
			ServiceEnabled: true,
			Mode:           "standalone",
		},
		Status: &NUTStatus{
			Connected: true,
			Status:    "OL",
		},
	}

	if !response.Installed {
		t.Error("Expected Installed to be true")
	}
	if !response.Running {
		t.Error("Expected Running to be true")
	}
	if response.Config == nil {
		t.Error("Expected Config to not be nil")
	}
	if response.Status == nil {
		t.Error("Expected Status to not be nil")
	}
}

func TestNUTModeValues(t *testing.T) {
	// Valid NUT modes
	validModes := []string{
		"standalone",
		"netserver",
		"netclient",
	}

	expectedModes := map[string]bool{
		"standalone": true,
		"netserver":  true,
		"netclient":  true,
	}

	for _, mode := range validModes {
		if !expectedModes[mode] {
			t.Errorf("Unexpected NUT mode: %s", mode)
		}
	}
}

func TestNUTDriverTypes(t *testing.T) {
	// Common NUT drivers
	commonDrivers := []string{
		"usbhid-ups",
		"blazer_usb",
		"snmp-ups",
		"apcsmart",
		"nutdrv_qx",
		"dummy-ups",
	}

	// Verify all drivers are non-empty
	for _, driver := range commonDrivers {
		if len(driver) == 0 {
			t.Error("Empty driver name")
		}
	}
}

func TestNUTBatteryStatusValues(t *testing.T) {
	// Possible battery status values
	statuses := []string{
		"OL",      // Online
		"OB",      // On Battery
		"LB",      // Low Battery
		"HB",      // High Battery
		"RB",      // Replace Battery
		"CHRG",    // Charging
		"DISCHRG", // Discharging
	}

	// Verify each produces a readable status text
	for _, status := range statuses {
		result := NUTStatusText(status)
		if len(result) == 0 {
			t.Errorf("Empty status text for status: %s", status)
		}
	}
}
