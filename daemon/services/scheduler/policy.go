package scheduler

import (
	"time"

	"github.com/tinkerologie/unraid-management-agent/daemon/constants"
)

// Policy is a collector's built-in Dynamic-mode cadence.
type Policy struct {
	// Active is the interval while something consumes the collector's data.
	Active time.Duration
	// Idle is the interval with no consumer. For an event-driven collector it
	// is the safety-net poll that catches missed events.
	Idle time.Duration
	// Floor is the slowest interval allowed while a safety feature (an
	// enabled alert rule) reads the collector: its Manual default.
	Floor time.Duration
	// Staleness is the oldest data a REST, MCP or /metrics read accepts
	// before it triggers a collection.
	Staleness time.Duration
	// MinGap is the shortest time between two runs, however they are triggered.
	MinGap time.Duration
	// InitialDelay postpones the first run after start.
	InitialDelay time.Duration
	// EventDriven reports that change events trigger the collector.
	EventDriven bool
	// RefreshOnRead lets a stale read trigger a collection.
	RefreshOnRead bool
	// Topics are the event bus topics the collector publishes. The first one
	// is the topic whose cache age decides staleness.
	Topics []string
}

// fixed returns the policy of a slow-changing collector that keeps a long
// fixed schedule: no demand scaling and no refresh on read.
func fixed(interval, initialDelay time.Duration, topics ...string) Policy {
	return Policy{
		Active: interval, Idle: interval, Floor: interval, Staleness: interval,
		MinGap: defaultMinGap, InitialDelay: initialDelay, Topics: topics,
	}
}

// demand returns the policy of a collector whose cadence follows its consumers.
// The floor is its Manual default.
func demand(active, idle, floor time.Duration, topics ...string) Policy {
	return Policy{
		Active: active, Idle: idle, Floor: floor, Staleness: active,
		MinGap: defaultMinGap, RefreshOnRead: true, Topics: topics,
	}
}

// evented returns a demand policy whose collector is also triggered by change events.
func evented(active, safetyNet, floor time.Duration, topics ...string) Policy {
	p := demand(active, safetyNet, floor, topics...)
	p.EventDriven = true
	return p
}

// withEvents marks a policy's collector as triggered by change events.
func withEvents(p Policy) Policy {
	p.EventDriven = true
	return p
}

// delayed sets a policy's InitialDelay: the startup stagger its Start uses.
func delayed(d time.Duration, p Policy) Policy {
	p.InitialDelay = d
	return p
}

// defaultMinGap keeps bursts of triggers from running a collector back to back.
const defaultMinGap = 2 * time.Second

const (
	sec    = time.Second
	minute = time.Minute
)

// policies holds every collector's policy. Active is at most the plugin's
// Manual default, Idle at least it, and Floor equals it ("auto" collectors
// default to IntervalAutoEnable, 300 s).
var policies = map[string]Policy{
	"system":           demand(15*sec, 60*sec, 15*sec, constants.TopicSystemUpdate.Name),
	"array":            evented(60*sec, 10*minute, 60*sec, constants.TopicArrayStatusUpdate.Name),
	"disk":             evented(5*minute, 30*minute, 5*minute, constants.TopicDiskListUpdate.Name),
	"docker":           evented(30*sec, 5*minute, 30*sec, constants.TopicContainerListUpdate.Name),
	"vm":               evented(60*sec, 10*minute, 60*sec, constants.TopicVMListUpdate.Name, constants.TopicVMGuestUpdate.Name),
	"ups":              demand(60*sec, 5*minute, 5*minute, constants.TopicUPSStatusUpdate.Name),
	"nut":              demand(60*sec, 5*minute, 5*minute, constants.TopicNUTStatusUpdate.Name),
	"gpu":              demand(60*sec, 5*minute, 5*minute, constants.TopicGPUMetricsUpdate.Name),
	"shares":           evented(60*sec, 10*minute, 60*sec, constants.TopicShareListUpdate.Name),
	"network":          demand(60*sec, 5*minute, 60*sec, constants.TopicNetworkListUpdate.Name),
	"hardware":         withEvents(fixed(10*minute, 0, constants.TopicHardwareUpdate.Name)),
	"zfs":              demand(5*minute, 15*minute, 5*minute, constants.TopicZFSPoolsUpdate.Name, constants.TopicZFSDatasetsUpdate.Name, constants.TopicZFSSnapshotsUpdate.Name, constants.TopicZFSARCStatsUpdate.Name),
	"notification":     evented(30*sec, 5*minute, 30*sec, constants.TopicNotificationsUpdate.Name),
	"registration":     fixed(10*minute, 0, constants.TopicRegistrationUpdate.Name),
	"unassigned":       evented(60*sec, 10*minute, 60*sec, constants.TopicUnassignedDevicesUpdate.Name),
	"fancontrol":       demand(60*sec, 5*minute, 60*sec, constants.TopicFanControlUpdate.Name),
	"tuning":           fixed(2*minute, 0, constants.TopicTuningUpdate.Name),
	"docker_update":    fixed(6*time.Hour, 45*sec, constants.TopicDockerUpdatesUpdate.Name),
	"docker_networks":  delayed(5*sec, demand(60*sec, 10*minute, 60*sec, constants.TopicDockerNetworksUpdate.Name)),
	"plugin_update":    pluginUpdatePolicy(),
	"os_update":        fixed(24*time.Hour, 60*sec, constants.TopicOSUpdateUpdate.Name),
	"mover":            delayed(10*sec, evented(30*sec, 5*minute, 30*sec, constants.TopicMoverUpdate.Name)),
	"storage_topology": delayed(20*sec, demand(5*minute, 15*minute, 5*minute, constants.TopicStorageTopologyUpdate.Name)),
	"psu":              evented(60*sec, 5*minute, 60*sec, constants.TopicPSUStatusUpdate.Name),
	"nic":              delayed(10*sec, demand(60*sec, 5*minute, 5*minute, constants.TopicNICUpdate.Name)),
}

// pluginUpdatePolicy keeps the hourly check and also runs on plugin installs,
// at most once a minute.
func pluginUpdatePolicy() Policy {
	p := withEvents(fixed(time.Hour, 30*sec, constants.TopicPluginUpdatesUpdate.Name))
	p.MinGap = time.Minute
	return p
}

// PolicyFor returns the named collector's policy.
func PolicyFor(name string) (Policy, bool) {
	p, ok := policies[name]
	return p, ok
}

// CollectorNames returns the collectors that have a policy.
func CollectorNames() []string {
	names := make([]string, 0, len(policies))
	for name := range policies {
		names = append(names, name)
	}
	return names
}

// topicCollectors maps each topic to the collector that publishes it.
var topicCollectors = func() map[string]string {
	m := map[string]string{}
	for name, p := range policies {
		for _, t := range p.Topics {
			m[t] = name
		}
	}
	return m
}()

// CollectorForTopic returns the collector that publishes topic.
func CollectorForTopic(topic string) (string, bool) {
	name, ok := topicCollectors[topic]
	return name, ok
}
