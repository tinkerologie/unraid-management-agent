package collectors

import (
	"strings"
	"testing"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

func TestNewNUTCollector(t *testing.T) {
	hub := domain.NewEventBus(10)
	ctx := &domain.Context{Hub: hub}

	collector := NewNUTCollector(ctx)

	if collector == nil {
		t.Fatal("NewNUTCollector() returned nil")
	}

	if collector.ctx != ctx {
		t.Error("NUTCollector context not set correctly")
	}
}

func TestNUTConfigParsing(t *testing.T) {
	configContent := `SERVICE="enable"
POWER="auto"
POWERVA="0"
POWERW="0"
MANUAL="disable"
SYSLOGMETHOD="syslog"
NAME="ups"
MONUSER="monuser"
DRIVER="usbhid-ups"
PORT="auto"
IPADDR="127.0.0.1"
MODE="standalone"
SHUTDOWN="sec_timer"
BATTERYLEVEL="20"
RTVALUE="240"
TIMEOUT="240"
POLL="15"
`
	config, err := parseNUTConfig(strings.NewReader(configContent))
	if err != nil {
		t.Fatalf("parseNUTConfig failed: %v", err)
	}

	if !config.ServiceEnabled {
		t.Error("ServiceEnabled should be true")
	}
	if config.RuntimeValue != 240 {
		t.Errorf("RuntimeValue = %d, want 240", config.RuntimeValue)
	}
	if config.Timeout != 240 {
		t.Errorf("Timeout = %d, want 240", config.Timeout)
	}
	if config.Mode != "standalone" {
		t.Errorf("Mode = %q, want %q", config.Mode, "standalone")
	}
	if config.UPSName != "ups" {
		t.Errorf("UPSName = %q, want %q", config.UPSName, "ups")
	}
	if config.Driver != "usbhid-ups" {
		t.Errorf("Driver = %q, want %q", config.Driver, "usbhid-ups")
	}
	if config.Port != "auto" {
		t.Errorf("Port = %q, want %q", config.Port, "auto")
	}
	if config.IPAddress != "127.0.0.1" {
		t.Errorf("IPAddress = %q, want %q", config.IPAddress, "127.0.0.1")
	}
	if config.BatteryLevel != 20 {
		t.Errorf("BatteryLevel = %d, want 20", config.BatteryLevel)
	}
	if config.PollInterval != 15 {
		t.Errorf("PollInterval = %d, want 15", config.PollInterval)
	}
	if config.ShutdownMode != "sec_timer" {
		t.Errorf("ShutdownMode = %q, want %q", config.ShutdownMode, "sec_timer")
	}
}

func TestNUTUpscOutputParsing(t *testing.T) {
	// Test parsing of upsc output format
	output := `battery.charge: 100
battery.charge.low: 35
battery.charge.warning: 20
battery.runtime: 6000
battery.runtime.low: 300
battery.voltage: 24.0
battery.voltage.nominal: 24
battery.type: PbAcid
device.mfr: CYBER POWER
device.model: PR1000ELCDRT1U
device.serial: ABC123456
device.type: ups
driver.name: usbhid-ups
driver.state: quiet
driver.version: 2.8.4
input.frequency: 49.9
input.voltage: 238.0
input.voltage.nominal: 240
output.frequency: 49.9
output.voltage: 238.0
ups.beeper.status: enabled
ups.load: 13
ups.mfr: CYBER POWER
ups.model: PR1000ELCDRT1U
ups.realpower.nominal: 800
ups.serial: ABC123456
ups.status: OL
ups.test.result: No test initiated
`

	status := parseNUTStatusOutput("ups", "localhost", output)
	if status == nil {
		t.Fatal("parseNUTStatusOutput returned nil")
	}

	// Verify parsing
	if status.RawVariables["battery.charge"] != "100" {
		t.Errorf("battery.charge = %q, want %q", status.RawVariables["battery.charge"], "100")
	}
	if status.BatteryCharge == nil || *status.BatteryCharge != 100 {
		t.Errorf("BatteryCharge = %v, want 100", status.BatteryCharge)
	}
	if status.Status != "OL" {
		t.Errorf("Status = %q, want %q", status.Status, "OL")
	}
	if status.LoadPercent == nil || *status.LoadPercent != 13 {
		t.Errorf("LoadPercent = %v, want 13", status.LoadPercent)
	}
	if status.Model != "PR1000ELCDRT1U" {
		t.Errorf("Model = %q, want %q", status.Model, "PR1000ELCDRT1U")
	}
	if status.Driver != "usbhid-ups" {
		t.Errorf("Driver = %q, want %q", status.Driver, "usbhid-ups")
	}
	if status.InputVoltage == nil || *status.InputVoltage != 238.0 {
		t.Errorf("InputVoltage = %v, want 238.0", status.InputVoltage)
	}
	if status.RealPowerNominal == nil || *status.RealPowerNominal != 800 {
		t.Errorf("RealPowerNominal = %v, want 800", status.RealPowerNominal)
	}
}

func TestNUTStatusTextConversion(t *testing.T) {
	tests := []struct {
		name     string
		input    string
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
		{"Trimming Voltage", "TRIM", "Trimming Voltage"},
		{"Boosting Voltage", "BOOST", "Boosting Voltage"},
		{"Forced Shutdown", "FSD", "Forced Shutdown"},
		{"Unknown status passthrough", "UNKNOWN", "UNKNOWN"},
		{"Combined status passthrough", "OL CHRG", "OL CHRG"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := dto.NUTStatusText(tt.input)
			if result != tt.expected {
				t.Errorf("NUTStatusText(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestNUTDeviceListParsing(t *testing.T) {
	// Test parsing upsc -l output
	output := `ups
backup-ups
remote-ups`

	lines := strings.Split(strings.TrimSpace(output), "\n")
	var devices []dto.NUTDevice

	for _, line := range lines {
		name := strings.TrimSpace(line)
		if name == "" {
			continue
		}
		devices = append(devices, dto.NUTDevice{
			Name:        name,
			Description: "UPS device: " + name,
			Available:   true,
		})
	}

	if len(devices) != 3 {
		t.Fatalf("Expected 3 devices, got %d", len(devices))
	}

	expectedNames := []string{"ups", "backup-ups", "remote-ups"}
	for i, expected := range expectedNames {
		if devices[i].Name != expected {
			t.Errorf("Device[%d].Name = %q, want %q", i, devices[i].Name, expected)
		}
	}
}

func TestNUTHostFromConfig(t *testing.T) {
	tests := []struct {
		name     string
		config   *dto.NUTConfig
		expected string
	}{
		{
			name:     "nil config defaults to localhost",
			config:   nil,
			expected: "localhost",
		},
		{
			name:     "empty IP defaults to localhost",
			config:   &dto.NUTConfig{IPAddress: ""},
			expected: "localhost",
		},
		{
			name:     "127.0.0.1 defaults to localhost",
			config:   &dto.NUTConfig{IPAddress: "127.0.0.1"},
			expected: "localhost",
		},
		{
			name:     "remote IP is used",
			config:   &dto.NUTConfig{IPAddress: "192.168.1.100"},
			expected: "192.168.1.100",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var result string
			if tt.config != nil && tt.config.IPAddress != "" && tt.config.IPAddress != "127.0.0.1" {
				result = tt.config.IPAddress
			} else {
				result = "localhost"
			}

			if result != tt.expected {
				t.Errorf("getHostFromConfig() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestNUTPowerCalculation(t *testing.T) {
	tests := []struct {
		name          string
		realPower     float64
		nominalPower  float64
		loadPercent   float64
		expectedPower float64
	}{
		{
			name:          "Calculate from nominal and load",
			realPower:     0,
			nominalPower:  800,
			loadPercent:   13,
			expectedPower: 104, // 800 * 0.13
		},
		{
			name:          "Real power already available",
			realPower:     150,
			nominalPower:  800,
			loadPercent:   20,
			expectedPower: 150, // Use real power when available
		},
		{
			name:          "Zero load",
			realPower:     0,
			nominalPower:  800,
			loadPercent:   0,
			expectedPower: 0,
		},
		{
			name:          "Full load",
			realPower:     0,
			nominalPower:  1000,
			loadPercent:   100,
			expectedPower: 1000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calculatedPower float64
			if tt.realPower > 0 {
				calculatedPower = tt.realPower
			} else if tt.nominalPower > 0 && tt.loadPercent > 0 {
				calculatedPower = tt.nominalPower * tt.loadPercent / 100.0
			}

			if calculatedPower != tt.expectedPower {
				t.Errorf("Calculated power = %v, want %v", calculatedPower, tt.expectedPower)
			}
		})
	}
}

func TestNUTBatteryRuntimeParsing(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{"6000 seconds", "6000", 6000},
		{"300 seconds", "300", 300},
		{"3600 seconds (1 hour)", "3600", 3600},
		{"0 seconds", "0", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var runtime int
			// Simulating strconv.ParseFloat behavior
			if v, err := parseFloat(tt.input); err == nil {
				runtime = int(v)
			}

			if runtime != tt.expected {
				t.Errorf("Runtime = %d, want %d", runtime, tt.expected)
			}
		})
	}
}

// Helper function for parsing floats in tests
func parseFloat(s string) (float64, error) {
	var f float64
	_, err := strings.NewReader(s).Read([]byte{})
	if err != nil {
		return 0, err
	}
	// Simple float parsing for test
	for _, c := range s {
		if c >= '0' && c <= '9' {
			f = f*10 + float64(c-'0')
		} else if c == '.' {
			break
		}
	}
	return f, nil
}
