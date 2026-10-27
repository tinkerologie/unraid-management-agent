package mqtt

import (
	"context"
	"fmt"

	"github.com/tinkerologie/unraid-management-agent/daemon/dto"
	"github.com/tinkerologie/unraid-management-agent/daemon/logger"
)

type psuDiscoveryUpdate struct {
	ctx    context.Context
	status *dto.PSUStatus
}

// PublishPSUStatus orders state publishes without waiting for HA discovery.
// Status snapshots are immutable after publication, like the API cache snapshots.
func (c *Client) PublishPSUStatus(status *dto.PSUStatus) error {
	c.psuMu.Lock()
	defer c.psuMu.Unlock()
	if !c.shouldPublish() {
		return nil
	}
	if err := c.publishJSON(c.buildTopic("psu"), status); err != nil {
		return err
	}
	if status != nil && c.config.HomeAssistantMode {
		c.mu.RLock()
		ctx := c.connectContext
		c.mu.RUnlock()
		c.psuDiscoveryMu.Lock()
		// One pending slot coalesces bursts while the single worker drains slowly.
		c.psuDiscoveryPending = &psuDiscoveryUpdate{ctx: ctx, status: status}
		c.startPSUDiscoveryLocked()
		c.psuDiscoveryMu.Unlock()
	}
	return nil
}

func (c *Client) startPSUDiscoveryLocked() {
	if !c.psuDiscoveryRunning {
		c.psuDiscoveryRunning = true
		go c.runPSUDiscovery()
	}
}

func (c *Client) runPSUDiscovery() {
	defer func() {
		if r := recover(); r != nil {
			logger.LogPanicWithStack("MQTT PSU discovery worker", r)
		}
		c.psuDiscoveryMu.Lock()
		defer c.psuDiscoveryMu.Unlock()
		c.psuDiscoveryRunning = false
		// An update can arrive between observing an empty slot and this defer.
		if c.psuDiscoveryPending != nil {
			c.startPSUDiscoveryLocked()
		}
	}()
	for {
		c.psuDiscoveryMu.Lock()
		update := c.psuDiscoveryPending
		c.psuDiscoveryPending = nil
		c.psuDiscoveryMu.Unlock()
		if update == nil {
			return
		}
		c.publishPSUDiscovery(update.ctx, update.status)
	}
}

func (c *Client) publishPSUDiscovery(ctx context.Context, status *dto.PSUStatus) {
	if !c.config.HomeAssistantMode || ctx.Err() != nil || !c.shouldPublish() {
		return
	}
	if c.psuDiscoveryIDs == nil {
		c.psuDiscoveryIDs = make(map[string]bool)
	}
	ids := make(map[string]bool)
	for _, dev := range status.Devices {
		for _, sensor := range dev.Sensors {
			if ctx.Err() != nil || !c.shouldPublish() {
				return
			}
			// Unmeasured input watts/efficiency never become fabricated entities.
			if sensor.Value == nil {
				continue
			}
			id := "psu_" + sanitizeID(dev.ID) + "_" + sanitizeID(sensor.Name)
			// Retain attempted IDs even if cancellation interrupts this snapshot,
			// so the next completed snapshot can remove partially published configs.
			c.psuDiscoveryIDs[id] = true
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
			ids[id] = true
		}
	}
	for id := range c.psuDiscoveryIDs {
		if ctx.Err() != nil || !c.shouldPublish() {
			return
		}
		if !ids[id] {
			// PSU discovery only creates sensors, so one retained deletion suffices.
			c.removeHAEntity("sensor", id)
			delete(c.psuDiscoveryIDs, id)
		}
	}
}
