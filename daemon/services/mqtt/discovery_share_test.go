package mqtt

import (
	"encoding/json"
	"testing"

	"github.com/tinkerologie/unraid-management-agent/daemon/dto"
)

func TestShareDiscoveryCapacity(t *testing.T) {
	config := DefaultConfig()
	config.Enabled = true
	config.HomeAssistantMode = true
	config.HADiscoveryPrefix = "homeassistant"
	config.TopicPrefix = "unraid"
	client := NewClient(config, "test-server", "1.0.0", nil)
	capture := &capturingClient{published: map[string]string{}}
	client.client = capture
	client.publishShareDiscovery([]dto.ShareInfo{{Name: "media", StorageUsed: 40, StorageFree: 60, StorageTotal: 100, StorageUsagePercent: 40}})
	for _, tt := range []struct{ id, name, template, unit string }{
		{"usage", "Storage Usage", "{{ value_json.storage_usage_percent | round(1) }}", "%"},
		{"used", "Used", "{{ value_json.used_bytes if value_json.used_bytes is number else None }}", "B"},
		{"free", "Storage Free", "{{ value_json.storage_free_bytes }}", "B"},
		{"storage_used", "Storage Used", "{{ value_json.storage_used_bytes }}", "B"},
	} {
		t.Run(tt.id, func(t *testing.T) {
			raw := capture.published["homeassistant/sensor/test-server/share_media_"+tt.id+"/config"]
			var cfg map[string]any
			if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
				t.Fatalf("config %q: %v", raw, err)
			}
			for key, want := range map[string]string{"name": "Share: media " + tt.name, "unique_id": "unraid_test-server_share_media_" + tt.id, "value_template": tt.template, "unit_of_measurement": tt.unit, "state_topic": "unraid/shares/media", "state_class": "measurement"} {
				if got := cfg[key]; got != want {
					t.Errorf("%s=%v want %s", key, got, want)
				}
			}
			if tt.unit == "B" && cfg["device_class"] != "data_size" {
				t.Errorf("device_class=%v", cfg["device_class"])
			}
		})
	}
	var state map[string]json.RawMessage
	if err := json.Unmarshal([]byte(capture.published["unraid/shares/media"]), &state); err != nil {
		t.Fatal(err)
	}
	if _, ok := state["used_bytes"]; ok {
		t.Error("unknown usage present in state")
	}
	client.publishShareDiscovery([]dto.ShareInfo{{Name: "media", Used: new(uint64(0))}})
	state = nil
	if err := json.Unmarshal([]byte(capture.published["unraid/shares/media"]), &state); err != nil {
		t.Fatal(err)
	}
	if string(state["used_bytes"]) != "0" {
		t.Error("known zero missing in state")
	}
	ids := client.publishShareEntities("unraid/shares/media", "share_media", "media")
	if len(ids) != 4 {
		t.Errorf("tracked IDs=%v", ids)
	}
}
