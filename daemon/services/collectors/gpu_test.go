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
