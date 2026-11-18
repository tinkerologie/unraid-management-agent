package mqtt

import (
	"fmt"
	"slices"
	"strings"
)

const (
	TopicSystem            = "system"
	TopicArray             = "array"
	TopicDisks             = "disks"
	TopicContainers        = "docker/containers"
	TopicVMs               = "vm/list"
	TopicUPS               = "ups"
	TopicGPU               = "gpu"
	TopicNetwork           = "network"
	TopicShares            = "shares"
	TopicNotifications     = "notifications"
	TopicNotificationEvent = "notifications/event"
	TopicZFSPools          = "zfs/pools"
	TopicNUT               = "nut/status"
	TopicHardware          = "hardware"
	TopicRegistration      = "registration"
	TopicUnassigned        = "unassigned/devices"
	TopicZFSDatasets       = "zfs/datasets"
	TopicZFSSnapshots      = "zfs/snapshots"
	TopicZFSARC            = "zfs/arc"
	TopicFanControl        = "fancontrol"
	TopicPSU               = "psu"
	TopicServices          = "services"
)

// Each built-in topic owns its exact suffix and any per-resource suffixes.
// The per-resource filters append one level to the listed paths.
var telemetryTopics = map[string][]string{
	TopicSystem: nil, TopicArray: nil, TopicDisks: {"disk"},
	TopicContainers: {"docker"}, TopicVMs: {"vm"},
	TopicUPS: nil, TopicGPU: {"gpu"}, TopicNetwork: {"network"},
	TopicShares: {"shares"}, TopicNotifications: nil, TopicNotificationEvent: nil,
	TopicZFSPools: {"zfs", TopicZFSPools}, TopicNUT: nil, TopicHardware: nil, TopicRegistration: nil,
	TopicUnassigned: {"unassigned", "unassigned/remote"}, TopicZFSDatasets: {"zfs/datasets"},
	TopicZFSSnapshots: nil, TopicZFSARC: nil, TopicFanControl: nil, TopicPSU: nil, TopicServices: nil,
}

var resourceTopicGroups = func() map[string]string {
	groups := make(map[string]string)
	for topic, resources := range telemetryTopics {
		for _, resource := range resources {
			groups[resource] = topic
		}
	}
	return groups
}()

// ParseDisabledTopics validates the operator's comma-separated telemetry suffixes.
func ParseDisabledTopics(value string) ([]string, error) {
	var disabled []string
	for item := range strings.SplitSeq(value, ",") {
		topic := strings.TrimSpace(item)
		if topic == "" {
			continue
		}
		if _, ok := telemetryTopics[topic]; !ok {
			return nil, fmt.Errorf("unknown MQTT telemetry topic %q", topic)
		}
		if !slices.Contains(disabled, topic) {
			disabled = append(disabled, topic)
		}
	}
	return disabled, nil
}

// TelemetryEnabled reports whether any of the supplied built-in topics is enabled.
func (c *Client) TelemetryEnabled(topics ...string) bool {
	for _, topic := range topics {
		if !slices.Contains(c.config.DisabledTopics, topic) {
			return true
		}
	}
	return false
}

func (c *Client) shouldPublishTopic(topics ...string) bool {
	return c.shouldPublish() && c.TelemetryEnabled(topics...)
}

func telemetryGroup(suffix string) string {
	// An aggregate takes precedence over a per-resource pattern. For
	// example, zfs/datasets is not a per-pool topic under zfs/+.
	if _, ok := telemetryTopics[suffix]; ok {
		return suffix
	}
	lastSlash := strings.LastIndexByte(suffix, '/')
	if lastSlash < 0 || lastSlash == len(suffix)-1 {
		return ""
	}
	return resourceTopicGroups[suffix[:lastSlash]]
}

var commandTopicGroups = map[string]string{
	"system": TopicSystem, "array": TopicArray, "disk": TopicDisks,
	"docker": TopicContainers, "vm": TopicVMs, "service": TopicServices,
	"unassigned": TopicUnassigned, "notifications": TopicNotifications,
}

func (c *Client) allowDiscovery(id, stateTopic, commandTopic string) bool {
	if commandTopic != "" && c.config.DisableCommands {
		return false
	}
	if len(c.config.DisabledTopics) == 0 {
		return true
	}
	if suffix, ok := strings.CutPrefix(stateTopic, c.buildTopic("")); ok {
		group := telemetryGroup(suffix)
		if poolID, shared := sharedZFSPoolID(suffix); shared && strings.HasPrefix(id, "zfs_"+poolID+"_") {
			group = TopicZFSPools // Legacy per-pool discovery on an aggregate path.
		}
		if !c.TelemetryEnabled(group) {
			return false
		}
	}
	// Buttons do not publish state_topic. Their command path identifies
	// the telemetry group that owns their discovery record.
	if path, ok := strings.CutPrefix(commandTopic, c.buildTopic("cmd/")); ok {
		group, _, _ := strings.Cut(path, "/")
		return c.TelemetryEnabled(commandTopicGroups[group])
	}
	return true
}

func sharedZFSPoolID(suffix string) (string, bool) {
	switch suffix {
	case TopicZFSPools, TopicZFSDatasets, TopicZFSSnapshots, TopicZFSARC:
		return strings.TrimPrefix(suffix, "zfs/"), true
	default:
		return "", false
	}
}
