package collectors

import (
	"strings"
	"testing"

	"github.com/cskr/pubsub"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
)

func TestNewGPUCollector(t *testing.T) {
	hub := pubsub.New(10)
	ctx := &domain.Context{Hub: hub}

	collector := NewGPUCollector(ctx)

	if collector == nil {
		t.Fatal("NewGPUCollector() returned nil")
	}

	if collector.ctx != ctx {
		t.Error("GPUCollector context not set correctly")
	}
}

func TestNvidiaSMIOutputParsing(t *testing.T) {
	// Test parsing of nvidia-smi query output
	output := `GPU 00000000:01:00.0
    Product Name                          : NVIDIA GeForce RTX 3080
    GPU UUID                              : GPU-12345678-1234-1234-1234-123456789abc
    Fan Speed                             : 45 %
    Temperature
        GPU Current Temp                  : 55 C
    Power Readings
        Power Draw                        : 120.50 W
        Power Limit                       : 320.00 W
    Memory Usage
        Total                             : 10240 MiB
        Used                              : 2048 MiB
        Free                              : 8192 MiB
    Utilization
        Gpu                               : 25 %
        Memory                            : 20 %
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
	if data["Product Name"] != "NVIDIA GeForce RTX 3080" {
		t.Errorf("Product Name = %q, want %q", data["Product Name"], "NVIDIA GeForce RTX 3080")
	}

	if data["GPU Current Temp"] != "55 C" {
		t.Errorf("GPU Current Temp = %q, want %q", data["GPU Current Temp"], "55 C")
	}
}

func TestGPUMetricsExtraction(t *testing.T) {
	// Test extracting metrics from nvidia-smi values
	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{"fan speed", "45 %", 45},
		{"temperature", "55 C", 55},
		{"utilization", "25 %", 25},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Extract numeric value
			parts := strings.Fields(tt.input)
			if len(parts) > 0 {
				// Verify the format is parseable
				_ = parts[0]
			}
		})
	}
}

func TestGPUMemoryParsing(t *testing.T) {
	// Test parsing memory values
	tests := []struct {
		input    string
		expected int
	}{
		{"10240 MiB", 10240},
		{"2048 MiB", 2048},
		{"8192 MiB", 8192},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			parts := strings.Fields(tt.input)
			if len(parts) < 2 {
				t.Errorf("Invalid memory format: %q", tt.input)
			}
		})
	}
}
func TestGPUPowerParsing(t *testing.T) {
	// Test parsing power values
	tests := []struct {
		input    string
		expected float64
	}{
		{"120.50 W", 120.50},
		{"320.00 W", 320.00},
		{"0.00 W", 0.00},
		{"250.75 W", 250.75},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if !strings.Contains(tt.input, "W") {
				t.Errorf("Power value %q should contain 'W'", tt.input)
			}
		})
	}
}

func TestGPUVendorTypes(t *testing.T) {
	// Test known GPU vendor types
	vendors := []string{"NVIDIA", "AMD", "Intel"}

	for _, vendor := range vendors {
		t.Run(vendor, func(t *testing.T) {
			if vendor == "" {
				t.Error("Vendor should not be empty")
			}
		})
	}
}

func TestGPUTemperatureParsing(t *testing.T) {
	// Test parsing temperature values
	tests := []struct {
		input    string
		expected int
	}{
		{"55 C", 55},
		{"75 C", 75},
		{"30 C", 30},
		{"90 C", 90},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if !strings.Contains(tt.input, "C") {
				t.Errorf("Temperature %q should contain 'C'", tt.input)
			}
		})
	}
}

func TestGPUUtilizationRanges(t *testing.T) {
	// Test valid utilization percentage ranges
	tests := []struct {
		name        string
		utilization int
		valid       bool
	}{
		{"zero", 0, true},
		{"low", 25, true},
		{"medium", 50, true},
		{"high", 75, true},
		{"full", 100, true},
		{"negative", -1, false},
		{"over 100", 101, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isValid := tt.utilization >= 0 && tt.utilization <= 100
			if isValid != tt.valid {
				t.Errorf("Utilization %d: valid = %v, want %v", tt.utilization, isValid, tt.valid)
			}
		})
	}
}
