package mqtt

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	pahomqtt "github.com/eclipse/paho.mqtt.golang"

	"github.com/tinkerologie/unraid-management-agent/daemon/dto"
)

func TestPSUPublicationDiscoveryAndRemoval(t *testing.T) {
	config := DefaultConfig()
	config.Enabled = true
	config.HomeAssistantMode = true
	c := NewClient(config, "test-server", "test", nil)
	if err := c.PublishPSUStatus(nil); err != nil {
		t.Fatal(err)
	}
	capture := &capturingClient{published: map[string]string{}}
	c.client = capture
	c.connected.Store(true)
	zero := 0.0
	status := &dto.PSUStatus{State: dto.PSUStateOK, Devices: []dto.PSUDevice{{ID: "psu-a", Model: "HX1000i", State: dto.PSUStateOK, Sensors: []dto.PSUSensor{
		{Name: "output_power", Label: "DC output", Kind: "power", Unit: "W", Value: &zero},
		{Name: "input_power", Label: "AC input", Kind: "power", Unit: "W"},
	}}}}
	if err := c.PublishPSUStatus(status); err != nil {
		t.Fatal(err)
	}
	state := capture.published[c.buildTopic("psu")]
	if !strings.Contains(state, `"value":0`) || !strings.Contains(state, `"value":null`) {
		t.Fatal("zero/null state not preserved", state)
	}
	topic := "homeassistant/sensor/test-server/psu_psu_a_output_power/config"
	var discovery map[string]any
	if err := json.Unmarshal([]byte(capture.published[topic]), &discovery); err != nil {
		t.Fatal(err, capture.published)
	}
	if discovery["device_class"] != "power" || discovery["availability_mode"] != "all" {
		t.Fatal(discovery)
	}
	availability := discovery["availability"].([]any)
	if len(availability) != 2 || !strings.Contains(availability[1].(map[string]any)["value_template"].(string), "is not none") {
		t.Fatal("missing per-sensor availability", discovery)
	}
	for topic := range capture.published {
		if strings.Contains(topic, "input_power/config") {
			t.Fatal("unknown measurement discovered")
		}
	}
	if err := c.PublishPSUStatus(dto.NewPSUStatus(dto.PSUStateNotDetected, time.Now())); err != nil {
		t.Fatal(err)
	}
	if capture.published[topic] != "" {
		t.Fatal("disconnected entity retained")
	}
	config.HomeAssistantMode = false
	c.publishPSUDiscovery(status)
	if err := c.PublishPSUStatus(nil); err != nil {
		t.Fatal(err)
	}
}

type psuFailureToken struct{ doneToken }

func (psuFailureToken) Error() error { return errors.New("broker rejected publish") }

type psuFailureClient struct{ capturingClient }

func (*psuFailureClient) Publish(string, byte, bool, any) pahomqtt.Token { return psuFailureToken{} }

func TestPSUPublishErrorIsPropagated(t *testing.T) {
	config := DefaultConfig()
	config.Enabled = true
	c := NewClient(config, "test", "test", nil)
	c.client = &psuFailureClient{}
	c.connected.Store(true)
	if err := c.PublishPSUStatus(dto.NewPSUStatus(dto.PSUStateNotDetected, time.Now())); err == nil {
		t.Fatal("broker error swallowed")
	}
}
