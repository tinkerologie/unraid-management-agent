package mqtt

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/tinkerologie/unraid-management-agent/daemon/dto"
)

func TestNullableMeasurementDiscovery(t *testing.T) {
	config := DefaultConfig()
	config.Enabled = true
	client := NewClient(config, "test-server", "test", nil)
	capture := &capturingClient{published: map[string]string{}}
	client.client = capture
	client.connected.Store(true)
	client.publishSystemDiscovery()
	client.publishDiskEntities("unraid/disks/disk1", "disk_disk1", "Disk 1", "disk1")
	client.publishGPUEntities("unraid/gpu/0", "gpu_0", "GPU 0")
	tests := []struct {
		id, field string
		precision int
	}{
		{"motherboard_temp", "motherboard_temp_celsius", -1},
		{"disk_disk1_temp", "temperature_celsius", -1},
		{"gpu_0_temp", "temperature_celsius", -1},
		{"gpu_0_memory_temp", "memory_temperature_celsius", -1},
		{"gpu_0_mem_util", "utilization_memory_percent", 1},
		{"gpu_0_mem_used", "memory_used_bytes", -1},
		{"gpu_0_power", "power_draw_watts", 1},
	}
	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			var cfg map[string]any
			topic := "homeassistant/sensor/test-server/" + tt.id + "/config"
			if err := json.Unmarshal([]byte(capture.published[topic]), &cfg); err != nil {
				t.Fatal(err, topic)
			}
			field := "value_json." + tt.field
			rendered := field
			if tt.precision >= 0 {
				rendered += fmt.Sprintf(" | round(%d)", tt.precision)
			}
			want := "{{ " + rendered + " if " + field + " is number else None }}"
			if cfg["value_template"] != want {
				t.Errorf("template=%v, want %s", cfg["value_template"], want)
			}
		})
	}
	// The GPU inventory tracker removes the memory temperature sensor with its GPU.
	memoryTempTopic := "homeassistant/sensor/test-server/gpu_0_memory_temp/config"
	client.publishGPUDiscovery([]*dto.GPUMetrics{{Available: true, Index: 0}})
	if !strings.Contains(capture.published[memoryTempTopic], `"device_class":"temperature"`) {
		t.Fatalf("memory temperature sensor not discovered: %q", capture.published[memoryTempTopic])
	}
	client.publishGPUDiscovery(nil)
	if payload, ok := capture.published[memoryTempTopic]; !ok || payload != "" {
		t.Errorf("memory temperature sensor not removed with its GPU: %q", payload)
	}
	// Direct payload marshalling preserves null and measured zero for every
	// changed field; no broker is used by this capture client.
	for _, zero := range []bool{false, true} {
		system := &dto.SystemInfo{}
		disk := dto.DiskInfo{}
		gpu := &dto.GPUMetrics{}
		want := "null"
		if zero {
			want = "0"
			system.MotherboardTemp = new(0.0)
			disk.Temperature = new(0.0)
			gpu.Temperature = new(0.0)
			gpu.MemoryTemperature = new(0.0)
			gpu.PowerDraw = new(0.0)
			gpu.MemoryUsed = new(uint64(0))
			gpu.MemoryTotal = new(uint64(0))
			gpu.UtilizationMemory = new(0.0)
		}
		for _, sample := range []struct {
			topic  string
			value  any
			fields []string
		}{
			{"system", system, []string{"motherboard_temp_celsius"}},
			{"disk", disk, []string{"temperature_celsius"}},
			{"gpu", gpu, []string{"temperature_celsius", "memory_temperature_celsius", "power_draw_watts", "memory_used_bytes", "memory_total_bytes", "utilization_memory_percent"}},
		} {
			if err := client.publishJSON(sample.topic, sample.value); err != nil {
				t.Fatal(err)
			}
			for _, field := range sample.fields {
				if !strings.Contains(capture.published[sample.topic], `"`+field+`":`+want) {
					t.Errorf("%s payload lost %s: %s", sample.topic, want, capture.published[sample.topic])
				}
			}
		}
	}
}
