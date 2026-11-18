package mqtt

import (
	"context"
	"encoding/json"
	"slices"
	"strconv"
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
				if !c.clearRetained(ctx, topic) && ctx.Err() == nil {
					logger.Warning("MQTT: Could not clear retained record for %q", topic)
				}
			}
		}
	}()
	if !waitForCleanupToken(ctx, token) {
		if ctx.Err() == nil {
			logger.Warning("MQTT: Could not subscribe to reconcile retained records")
		}
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

func (c *Client) clearRetained(ctx context.Context, topic string) bool {
	suffix := strings.TrimPrefix(topic, c.buildTopic(""))
	if _, shared := sharedZFSPoolID(suffix); shared && c.TelemetryEnabled(suffix) {
		c.zfsPublishMu.Lock()
		defer c.zfsPublishMu.Unlock()
		if c.zfsPublished[topic] {
			return true // The enabled aggregate already replaced the legacy state.
		}
	}
	return ctx.Err() == nil && waitForCleanupToken(ctx, c.client.Publish(topic, 1, true, ""))
}

func waitForCleanupToken(ctx context.Context, token pahomqtt.Token) bool {
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return false
	case <-token.Done():
		return token.Error() == nil
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
	if !underTopicPrefix {
		return false
	}
	if id, shared := sharedZFSPoolID(suffix); shared &&
		matchesRetainedResource("zfs", id, msg.Payload()) {
		// The pool now uses its own path. Remove the old copy if either
		// owner of the previously shared path is disabled.
		return !c.TelemetryEnabled(TopicZFSPools) || !c.TelemetryEnabled(suffix)
	}
	if _, exact := telemetryTopics[suffix]; exact {
		return !c.TelemetryEnabled(suffix)
	}
	last := strings.LastIndexByte(suffix, '/')
	if last < 0 {
		return false
	}
	resource, id := suffix[:last], suffix[last+1:]
	group := resourceTopicGroups[resource]
	return group != "" && !c.TelemetryEnabled(group) && matchesRetainedResource(resource, id, msg.Payload())
}

// A wildcard can overlap another agent's nested prefix or a custom topic.
// Legacy resource JSON has no owner field, so verify both the resource ID and
// a field specific to the resource kind before deleting a wildcard match.
var retainedResourceFields = map[string]struct{ id, marker string }{
	"disk":              {"id", "smart_status"},
	"docker":            {"name", "image"},
	"vm":                {"name", "memory_allocated_bytes"},
	"gpu":               {"index", "driver_version"},
	"network":           {"name", "packets_received"},
	"shares":            {"name", "smb_export"},
	"zfs":               {"name", "capacity_percent"},
	TopicZFSPools:       {"name", "capacity_percent"},
	"unassigned":        {"device", "partitions"},
	"unassigned/remote": {"mount_point", "source"},
	TopicZFSDatasets:    {"name", "referenced_bytes"},
}

func matchesRetainedResource(resource, id string, payload []byte) bool {
	fields, ok := retainedResourceFields[resource]
	if !ok || id == "" {
		return false
	}
	var record map[string]json.RawMessage
	if json.Unmarshal(payload, &record) != nil || record[fields.marker] == nil {
		return false
	}
	var name string
	if resource == TopicGPU {
		var index *int
		return json.Unmarshal(record[fields.id], &index) == nil && index != nil && strconv.Itoa(*index) == id
	}
	return json.Unmarshal(record[fields.id], &name) == nil && name != "" && sanitizeID(name) == id
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
	return !c.allowDiscovery(id, record.StateTopic, record.CommandTopic)
}
