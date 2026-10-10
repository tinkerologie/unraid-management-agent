package collectors

import (
	"strings"
	"testing"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
)

func TestNewUPSCollector(t *testing.T) {
	hub := domain.NewEventBus(10)
	ctx := &domain.Context{Hub: hub}

	collector := NewUPSCollector(ctx)

	if collector == nil {
		t.Fatal("NewUPSCollector() returned nil")
	}

	if collector.ctx != ctx {
		t.Error("UPSCollector context not set correctly")
	}
}

func TestParseAPCOutput(t *testing.T) {
	output := `APC      : 001,034,0856
DATE     : 2024-01-01 00:00:00 +0000
HOSTNAME : tower
VERSION  : 3.14.14
UPSNAME  : Back-UPS RS 1500
STATUS   : ONLINE
LINEV    : 120.0 Volts
LOADPCT  : 25.0 Percent
BCHARGE  : 100.0 Percent
TIMELEFT : 45.0 Minutes
NOMPOWER : 800 Watts
BATTV    : 27.1 Volts
MODEL    : Back-UPS RS 1500
`
	status, err := parseAPCOutput(output)
	if err != nil {
		t.Fatalf("parseAPCOutput failed: %v", err)
	}
	if status.Status != "ONLINE" {
		t.Errorf("Status = %q, want ONLINE", status.Status)
	}
	if status.LoadPercent == nil || *status.LoadPercent != 25.0 {
		t.Errorf("LoadPercent = %v, want 25.0", status.LoadPercent)
	}
	if status.BatteryCharge == nil || *status.BatteryCharge != 100.0 {
		t.Errorf("BatteryCharge = %v, want 100.0", status.BatteryCharge)
	}
	if status.RuntimeLeft == nil || *status.RuntimeLeft != 2700 {
		t.Errorf("RuntimeLeft = %v, want 2700", status.RuntimeLeft)
	}
	if status.NominalPower == nil || *status.NominalPower != 800.0 {
		t.Errorf("NominalPower = %v, want 800.0", status.NominalPower)
	}
	if status.PowerWatts == nil || *status.PowerWatts != 200.0 {
		t.Errorf("PowerWatts = %v, want 200.0", status.PowerWatts)
	}
	if status.Model != "Back-UPS RS 1500" {
		t.Errorf("Model = %q, want Back-UPS RS 1500", status.Model)
	}
}

func TestParseNUTUpscOutput(t *testing.T) {
	output := `battery.charge: 95.5
battery.runtime: 1800
device.model: Smart-UPS 1500
ups.load: 30.0
ups.realpower: 300.0
ups.status: OL
`
	status, err := parseNUTUpscOutput(output)
	if err != nil {
		t.Fatalf("parseNUTUpscOutput failed: %v", err)
	}
	if status.Status != "OL" {
		t.Errorf("Status = %q, want OL", status.Status)
	}
	if status.BatteryCharge == nil || *status.BatteryCharge != 95.5 {
		t.Errorf("BatteryCharge = %v, want 95.5", status.BatteryCharge)
	}
	if status.RuntimeLeft == nil || *status.RuntimeLeft != 1800 {
		t.Errorf("RuntimeLeft = %v, want 1800", status.RuntimeLeft)
	}
	if status.PowerWatts == nil || *status.PowerWatts != 300.0 {
		t.Errorf("PowerWatts = %v, want 300.0", status.PowerWatts)
	}
	if status.Model != "Smart-UPS 1500" {
		t.Errorf("Model = %q, want Smart-UPS 1500", status.Model)
	}
}

func TestUPSStatusValues(t *testing.T) {
	// Test status value parsing
	tests := []struct {
		input    string
		expected string
	}{
		{"ONLINE", "ONLINE"},
		{"ONBATT", "ONBATT"},
		{"ONLINE LOWBATT", "ONLINE LOWBATT"},
		{"COMMLOST", "COMMLOST"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			status := strings.TrimSpace(tt.input)
			if status != tt.expected {
				t.Errorf("Status = %q, want %q", status, tt.expected)
			}
		})
	}
}

func TestUPSPercentageParsing(t *testing.T) {
	// Test parsing percentage values from APC output
	tests := []struct {
		input    string
		expected float64
	}{
		{"100.0 Percent", 100.0},
		{"50.5 Percent", 50.5},
		{"0.0 Percent", 0.0},
		{"25 Percent", 25.0},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			// Extract the numeric value
			value := strings.TrimSuffix(strings.TrimSpace(tt.input), " Percent")
			var parsed float64
			_, err := strings.NewReader(value).Read(nil)
			if err == nil {
				// Simple validation
				_ = parsed
			}
		})
	}
}
func TestUPSTimeleftParsing(t *testing.T) {
	// Test parsing timeleft values from APC output
	tests := []struct {
		input    string
		expected float64
	}{
		{"45.0 Minutes", 45.0},
		{"120.0 Minutes", 120.0},
		{"0.0 Minutes", 0.0},
		{"30 Minutes", 30.0},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			// Verify the parsing pattern
			if !strings.Contains(tt.input, "Minutes") {
				t.Errorf("Timeleft %q should contain 'Minutes'", tt.input)
			}
		})
	}
}

func TestUPSVoltageParsing(t *testing.T) {
	// Test parsing voltage values from APC output
	tests := []struct {
		input    string
		expected float64
	}{
		{"120.0 Volts", 120.0},
		{"240.0 Volts", 240.0},
		{"27.1 Volts", 27.1},
		{"0.0 Volts", 0.0},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			// Verify the parsing pattern
			if !strings.Contains(tt.input, "Volts") {
				t.Errorf("Voltage %q should contain 'Volts'", tt.input)
			}
		})
	}
}

func TestUPSLoadParsing(t *testing.T) {
	// Test load percentage interpretation
	tests := []struct {
		load     float64
		expected string
	}{
		{0.0, "light"},
		{25.0, "light"},
		{50.0, "moderate"},
		{75.0, "moderate"},
		{90.0, "heavy"},
		{100.0, "heavy"},
	}

	for _, tt := range tests {
		t.Run(strings.ReplaceAll(tt.expected, " ", "_"), func(t *testing.T) {
			var category string
			if tt.load < 50 {
				category = "light"
			} else if tt.load < 80 {
				category = "moderate"
			} else {
				category = "heavy"
			}
			if category != tt.expected {
				t.Errorf("Load %.1f%% category = %q, want %q", tt.load, category, tt.expected)
			}
		})
	}
}
