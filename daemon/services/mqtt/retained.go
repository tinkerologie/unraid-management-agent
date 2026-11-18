package mqtt

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"sync"
	"time"

	pahomqtt "github.com/eclipse/paho.mqtt.golang"

	"github.com/tinkerologie/unraid-management-agent/daemon/dto"
	"github.com/tinkerologie/unraid-management-agent/daemon/logger"
)

// reconcileRetained subscribes to the affected namespaces so a restart can
// clean up records produced before this process (including older releases).
// MQTT delivers retained records on subscribe and does not signal the end of
// that replay. Ignore live traffic and let the clean session expire the filters.
func (c *Client) reconcileRetained(ctx context.Context) {
	if !c.config.DisableCommands && len(c.config.DisabledTopics) == 0 {
		return
	}
	if strings.ContainsAny(c.config.TopicPrefix, "+#\x00") {
		logger.Warning("MQTT: Cannot reconcile retained records with an invalid topic prefix")
		return
	}

	filters := make(map[string]byte)
	for _, group := range c.config.DisabledTopics {
		resources, known := telemetryTopics[group]
		if !known {
			continue
		}
		filters[c.buildTopic(group)] = 1
		for _, resource := range resources {
			filters[c.buildTopic(resource+"/+")] = 1
		}
	}
	host := c.discoveryHost()
	if host != "" && !strings.ContainsAny(host, "/+#\x00") &&
		!strings.ContainsAny(c.config.HADiscoveryPrefix, "+#\x00") {
		filters[c.config.HADiscoveryPrefix+"/+/"+host+"/+/config"] = 1
	}
	if len(filters) == 0 {
		return
	}

	var mu sync.Mutex
	pending := make(map[string]struct{})
	wake := make(chan struct{}, 1)
	token := c.client.SubscribeMultiple(filters, func(_ pahomqtt.Client, msg pahomqtt.Message) {
		if ctx.Err() != nil || !c.shouldClearRetained(msg) {
			return
		}
		mu.Lock()
		pending[msg.Topic()] = struct{}{}
		mu.Unlock()
		select {
		case wake <- struct{}{}:
		default:
		}
	})

	// Keep broker publication off Paho's ordered callback so it can deliver ACKs.
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-wake:
			}
			mu.Lock()
			batch := pending
			pending = make(map[string]struct{})
			mu.Unlock()
			for topic := range batch {
				if ctx.Err() != nil {
					return
				}
				token := c.client.Publish(topic, 1, true, "")
				if !token.WaitTimeout(5*time.Second) || token.Error() != nil {
					logger.Warning("MQTT: Could not clear retained record for %q", topic)
				}
			}
		}
	}()
	if !token.WaitTimeout(5*time.Second) || token.Error() != nil {
		logger.Warning("MQTT: Could not subscribe to reconcile retained records")
		return
	}
	if sub, ok := token.(*pahomqtt.SubscribeToken); ok {
		for filter, qos := range sub.Result() {
			if qos == 0x80 {
				logger.Warning("MQTT: Broker denied the retained cleanup subscription to %q", filter)
			}
		}
	}
}

func (c *Client) shouldClearRetained(msg pahomqtt.Message) bool {
	if !msg.Retained() || len(msg.Payload()) == 0 {
		return false
	}

	relative, underDiscoveryPrefix := strings.CutPrefix(msg.Topic(), c.config.HADiscoveryPrefix+"/")
	if underDiscoveryPrefix {
		parts := strings.Split(relative, "/")
		if len(parts) == 4 && parts[1] == c.discoveryHost() && parts[3] == "config" {
			return c.shouldClearDiscovery(parts[2], msg.Payload())
		}
	}

	suffix, underTopicPrefix := strings.CutPrefix(msg.Topic(), c.buildTopic(""))
	group := telemetryGroup(suffix)
	return underTopicPrefix && group != "" && !c.TelemetryEnabled(group)
}

func (c *Client) shouldClearDiscovery(id string, payload []byte) bool {
	var record struct {
		dto.HADiscoveryConfig
		Availability []struct {
			Topic string `json:"topic"`
		} `json:"availability"`
	}
	if json.Unmarshal(payload, &record) != nil || record.Device == nil {
		return false
	}
	deviceID := "unraid_" + c.discoveryHost()
	if !slices.Contains(record.Device.Identifiers, deviceID) || record.UniqueID != deviceID+"_"+id {
		return false
	}

	// Require the same telemetry prefix as well as the same HA host ID.
	availability := c.buildTopic("availability")
	ownsRecord := record.AvailabilityTopic == availability || slices.ContainsFunc(record.Availability, func(a struct {
		Topic string `json:"topic"`
	}) bool {
		return a.Topic == availability
	})
	if !ownsRecord {
		return false
	}
	return !c.allowDiscovery(record.StateTopic, record.CommandTopic)
}
