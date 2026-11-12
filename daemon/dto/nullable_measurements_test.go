package dto

import (
	"encoding/json"
	"testing"
)

func TestNullableMeasurementJSON(t *testing.T) {
	tests := []struct {
		name          string
		missing, zero any
		fields        []string
	}{
		{"system", SystemInfo{}, SystemInfo{MotherboardTemp: new(0.0)}, []string{"motherboard_temp_celsius"}},
		{"disk", DiskInfo{}, DiskInfo{Temperature: new(0.0)}, []string{"temperature_celsius"}},
		{"gpu", GPUMetrics{}, GPUMetrics{Temperature: new(0.0), MemoryTemperature: new(0.0), PowerDraw: new(0.0), MemoryUsed: new(uint64(0)), MemoryTotal: new(uint64(0)), UtilizationMemory: new(0.0)}, []string{"temperature_celsius", "memory_temperature_celsius", "power_draw_watts", "memory_used_bytes", "memory_total_bytes", "utilization_memory_percent"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, sample := range []struct {
				value any
				want  string
			}{{tt.missing, "null"}, {tt.zero, "0"}} {
				data, err := json.Marshal(sample.value)
				if err != nil {
					t.Fatal(err)
				}
				var fields map[string]json.RawMessage
				if err := json.Unmarshal(data, &fields); err != nil {
					t.Fatal(err)
				}
				for _, field := range tt.fields {
					if got := string(fields[field]); got != sample.want {
						t.Errorf("%s=%q, want %s (key must remain present)", field, got, sample.want)
					}
				}
			}
		})
	}
}
