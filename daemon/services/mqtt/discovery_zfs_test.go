package mqtt

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	pahomqtt "github.com/eclipse/paho.mqtt.golang"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

// doneToken is a completed, successful publish token.
type doneToken struct{ pahomqtt.Token }

func (doneToken) WaitTimeout(time.Duration) bool { return true }
func (doneToken) Error() error                   { return nil }

// capturingClient records every publish by topic.
type capturingClient struct {
	pahomqtt.Client
	published map[string]string
}

func (c *capturingClient) Publish(topic string, _ byte, _ bool, payload any) pahomqtt.Token {
	c.published[topic], _ = payload.(string)
	return doneToken{}
}

// TestZFSDiscoveryCorruptedFiles checks the pool state payload carries
// corrupted_files and the HA sensor template counts it, treating null
// (zpool status unreadable) or a missing field as an empty list.
func TestZFSDiscoveryCorruptedFiles(t *testing.T) {
	config := DefaultConfig()
	config.Enabled = true
	config.HomeAssistantMode = true
	config.HADiscoveryPrefix = "homeassistant"
	config.TopicPrefix = "unraid"
	client := NewClient(config, "test-server", "1.0.0", nil)
	capture := &capturingClient{published: map[string]string{}}
	client.client = capture

	client.publishZFSDiscovery([]dto.ZFSPool{{Name: "tank", Health: "ONLINE", CorruptedFiles: []string{}}})

	state := capture.published["unraid/zfs/tank"]
	if !strings.Contains(state, `"corrupted_files":[]`) {
		t.Errorf("state payload %s does not contain \"corrupted_files\":[]", state)
	}

	raw, ok := capture.published["homeassistant/sensor/test-server/zfs_tank_corrupted_files/config"]
	if !ok {
		t.Fatalf("corrupted files discovery config not published; topics: %v", capture.published)
	}
	var discovery map[string]any
	if err := json.Unmarshal([]byte(raw), &discovery); err != nil {
		t.Fatalf("discovery config is not JSON: %v", err)
	}
	if got, want := discovery["value_template"], "{{ (value_json.corrupted_files or []) | count }}"; got != want {
		t.Errorf("value_template = %v, want %v", got, want)
	}
}
