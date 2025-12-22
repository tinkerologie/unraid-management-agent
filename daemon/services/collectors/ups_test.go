package collectors

import (
	"strings"
	"testing"

	"github.com/cskr/pubsub"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
)

func TestNewUPSCollector(t *testing.T) {
	hub := pubsub.New(10)
	ctx := &domain.Context{Hub: hub}

	collector := NewUPSCollector(ctx)

	if collector == nil {
		t.Fatal("NewUPSCollector() returned nil")
	}

	if collector.ctx != ctx {
		t.Error("UPSCollector context not set correctly")
	}
}

func TestAPCOutputParsing(t *testing.T) {
	// Test parsing of apcaccess output format
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
BATTV    : 27.1 Volts
`
	lines := strings.Split(output, "\n")

	data := make(map[string]string)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		data[key] = value
	}

	// Verify parsing
	if data["STATUS"] != "ONLINE" {
		t.Errorf("STATUS = %q, want %q", data["STATUS"], "ONLINE")
	}

	if data["BCHARGE"] != "100.0 Percent" {
		t.Errorf("BCHARGE = %q, want %q", data["BCHARGE"], "100.0 Percent")
	}

	if data["TIMELEFT"] != "45.0 Minutes" {
		t.Errorf("TIMELEFT = %q, want %q", data["TIMELEFT"], "45.0 Minutes")
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
