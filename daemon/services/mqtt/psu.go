package mqtt

import (
	"fmt"

	"github.com/tinkerologie/unraid-management-agent/daemon/dto"
)

// PublishPSUStatus publishes state and discovery in order, so a disconnect cannot
// be followed by discovery from an older asynchronous update.
func (c *Client) PublishPSUStatus(status *dto.PSUStatus) error {
	c.psuMu.Lock()
	defer c.psuMu.Unlock()
	if !c.shouldPublish() {
		return nil
	}
	if err := c.publishJSON(c.buildTopic("psu"), status); err != nil {
		return err
	}
	if status != nil {
		c.publishPSUDiscovery(status)
	}
	return nil
}

func (c *Client) publishPSUDiscovery(status *dto.PSUStatus) {
	if !c.config.HomeAssistantMode {
		return
	}
	var ids []string
	for _, dev := range status.Devices {
		for _, sensor := range dev.Sensors {
			// Unmeasured input watts/efficiency never become fabricated entities.
			if sensor.Value == nil {
				continue
			}
			id := "psu_" + sanitizeID(dev.ID) + "_" + sanitizeID(sensor.Name)
			lookup := fmt.Sprintf(`{%% set d = value_json.devices | selectattr('id', 'eq', '%s') | first | default({}) %%}{%% set v = d.sensors | default([]) | selectattr('name', 'eq', '%s') | map(attribute='value') | first | default(none) %%}`, dev.ID, sensor.Name)
			class := ""
			switch sensor.Kind {
			case "voltage", "current", "power", "temperature":
				class = sensor.Kind
			}
			c.publishHAEntity(haEntityOpts{
				entityType: "sensor", stateTopic: c.buildTopic("psu"), id: id,
				name: fmt.Sprintf("PSU %s %s %s", dev.Model, sensor.Label, sensor.Kind),
				unit: sensor.Unit, deviceClass: class, stateClass: "measurement", icon: "mdi:power-plug",
				template:                  lookup + `{{ v if v is not none else none }}`,
				stateAvailabilityTemplate: lookup + `{{ 'online' if v is not none else 'offline' }}`,
			})
			ids = append(ids, id)
		}
	}
	for _, id := range c.tracker.update("psu", ids) {
		c.removeHAEntities(id)
	}
}
