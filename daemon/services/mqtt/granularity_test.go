package mqtt

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	pahomqtt "github.com/eclipse/paho.mqtt.golang"

	"github.com/tinkerologie/unraid-management-agent/daemon/dto"
)

func TestParseDisabledTopics(t *testing.T) {
	got, err := ParseDisabledTopics(" zfs/datasets,psu,zfs/datasets, ")
	if err != nil || !slices.Equal(got, []string{TopicZFSDatasets, TopicPSU}) {
		t.Fatalf("ParseDisabledTopics() = %v, %v", got, err)
	}
	for _, input := range []string{"availability", "docker/+", "zfs/#", "unknown"} {
		if _, err := ParseDisabledTopics(input); err == nil {
			t.Errorf("accepted invalid topic %q", input)
		}
	}
}

func TestTelemetryGroup(t *testing.T) {
	for suffix, want := range map[string]string{
		"docker/containers":       TopicContainers,
		"docker/media":            TopicContainers,
		"vm/list":                 TopicVMs,
		"disk/disk1":              TopicDisks,
		"unassigned/remote/share": TopicUnassigned,
		"zfs/datasets":            TopicZFSDatasets,
		"zfs/datasets/tank_media": TopicZFSDatasets,
		"zfs/snapshots":           TopicZFSSnapshots,
		"zfs/pools/snapshots":     TopicZFSPools,
		"zfs/tank":                TopicZFSPools,
		"zfs/":                    "",
		"zfs/other/leaf":          "",
		"availability":            "",
	} {
		if got := telemetryGroup(suffix); got != want {
			t.Errorf("telemetryGroup(%q) = %q, want %q", suffix, got, want)
		}
	}
}

func TestDisableCommandsGuardsDispatch(t *testing.T) {
	command := &commandProbe{retainedFixtureMessage: retainedFixtureMessage{topic: "unraid/cmd/system/fixture-only"}}
	c := NewClient(&dto.MQTTConfig{DisableCommands: true}, "tower", "test", nil)
	c.handleCommand(command)
	if command.inspected {
		t.Fatal("disabled command reached dispatch")
	}
	NewClient(DefaultConfig(), "tower", "test", nil).handleCommand(command)
	if !command.inspected {
		t.Fatal("enabled command did not reach dispatch")
	}
}

type commandProbe struct {
	retainedFixtureMessage
	inspected bool
}

func (p *commandProbe) Topic() string {
	p.inspected = true
	return p.retainedFixtureMessage.Topic()
}

func TestMQTTGranularityWithBroker(t *testing.T) {
	brokerURL := os.Getenv("UMA_TEST_MQTT_BROKER")
	if brokerURL == "" {
		t.Skip("set UMA_TEST_MQTT_BROKER to a disposable broker to exercise retained cleanup")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	prefix := fmt.Sprintf("uma356-%d", time.Now().UnixNano())
	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.Broker = brokerURL
	cfg.ClientID = prefix
	cfg.TopicPrefix = prefix
	cfg.HADiscoveryPrefix = prefix + "-discovery"
	cfg.QoS = 0
	host := "tower"
	peer := connectBrokerFixture(t, brokerURL, prefix+"-observer")
	legacy := NewClient(cfg, host, "old", nil)
	legacy.client = peer // Publish the same discovery shape as a prior release.

	containerState := legacy.buildTopic("docker/media")
	containerSensor := cfg.HADiscoveryPrefix + "/binary_sensor/tower/container_media_state/config"
	containerButton := cfg.HADiscoveryPrefix + "/button/tower/container_media_restart/config"
	arrayControl := cfg.HADiscoveryPrefix + "/switch/tower/array_switch/config"
	systemSensor := cfg.HADiscoveryPrefix + "/sensor/tower/fixture_system/config"
	foreign := cfg.HADiscoveryPrefix + "/sensor/other/fixture_system/config"
	foreignPrefix := cfg.HADiscoveryPrefix + "/sensor/tower/other_prefix/config"
	zfsPool := legacy.buildTopic("zfs/tank")
	zfsDataset := legacy.buildTopic("zfs/datasets/tank_media")
	unrelated := legacy.buildTopic("custom/fixture")
	nestedAvailability := legacy.buildTopic("docker/availability")
	nestedSystem := legacy.buildTopic("docker/system")
	legacyARC := legacy.buildTopic("zfs/arc")
	legacyARCControl := cfg.HADiscoveryPrefix + "/sensor/tower/zfs_arc_health/config"
	arcStats := cfg.HADiscoveryPrefix + "/sensor/tower/arc_size/config"

	// Seed retained records, including per-resource records from a stopped agent.
	legacy.publishContainerDiscovery([]dto.ContainerInfo{{Name: "media"}})
	legacy.publishHAEntity(haEntityOpts{
		entityType: "switch", id: "array_switch", stateTopic: legacy.buildTopic("array"),
		commandTopic: legacy.buildCommandTopic("array", "set"),
	})
	legacy.publishHAEntity(haEntityOpts{
		entityType: "sensor", id: "fixture_system", stateTopic: legacy.buildTopic("system"),
	})
	otherCfg := *cfg
	otherCfg.TopicPrefix += "-other"
	foreignAgent := NewClient(&otherCfg, "other", "old", nil)
	foreignAgent.client = peer
	foreignAgent.publishHAEntity(haEntityOpts{
		entityType: "sensor", id: "fixture_system", stateTopic: foreignAgent.buildTopic("system"),
	})
	foreignPrefixAgent := NewClient(&otherCfg, host, "old", nil)
	foreignPrefixAgent.client = peer
	foreignPrefixAgent.publishHAEntity(haEntityOpts{
		entityType: "sensor", id: "other_prefix", stateTopic: legacy.buildTopic("docker/media"),
	})
	legacy.publishZFSEntities(legacyARC, "zfs_arc", "arc")
	legacy.publishZFSARCDiscovery()
	for topic, payload := range map[string]any{
		zfsPool: dto.ZFSPool{Name: "tank"}, zfsDataset: dto.ZFSDataset{Name: "tank/media"},
		legacyARC: dto.ZFSPool{Name: "arc"}, unrelated: map[string]string{"name": "fixture"},
		nestedSystem: dto.SystemInfo{Hostname: "neighbor"},
	} {
		if err := legacy.publishJSON(topic, payload); err != nil {
			t.Fatal(err)
		}
	}
	waitForBrokerToken(t, peer.Publish(nestedAvailability, 1, true, "online"))

	removed := []string{containerState, containerSensor, containerButton, arrayControl, zfsPool, legacyARC, legacyARCControl}
	kept := []string{systemSensor, foreign, foreignPrefix, zfsDataset, unrelated, nestedSystem, nestedAvailability, arcStats}
	observed := append(slices.Clone(removed), kept...)
	observed = append(observed, legacy.buildTopic("availability"), legacy.buildTopic("system"))
	observed = append(observed, legacy.buildTopic("docker/containers"))
	waitForRetained(t, peer, prefix, observed, func(records map[string]string) bool {
		return allRetained(records, append(slices.Clone(removed), kept...)...)
	})

	disabledCfg := *cfg
	disabledCfg.ClientID += "-disabled"
	disabledCfg.DisableCommands = true
	disabledCfg.DisabledTopics = []string{TopicContainers, TopicZFSPools}
	client := NewClient(&disabledCfg, host, "new", nil)
	if err := client.Connect(ctx); err != nil {
		t.Fatal(err)
	}
	defer client.Disconnect()

	recordFilter := func(records map[string]string) bool {
		return records[legacy.buildTopic("availability")] == "online" &&
			allRetained(records, kept...) && !anyRetained(records, removed...)
	}
	waitForRetained(t, peer, prefix, observed, recordFilter)
	if err := client.PublishContainers([]dto.ContainerInfo{{Name: "media"}}); err != nil {
		t.Fatal(err)
	}
	if err := client.PublishSystemInfo(&dto.SystemInfo{}); err != nil {
		t.Fatal(err)
	}
	waitForRetained(t, peer, prefix, observed, func(records map[string]string) bool {
		return records[legacy.buildTopic("system")] != "" &&
			!anyRetained(records, append(removed, legacy.buildTopic("docker/containers"))...)
	})
	assertBrokerCommand(t, peer, legacy.buildCommandTopic("system", "fixture-only"), false)

	// A fresh retained record must also be removed when this same client reconnects.
	if err := legacy.publishJSON(zfsPool, dto.ZFSPool{Name: "tank"}); err != nil {
		t.Fatal(err)
	}
	waitForRetained(t, peer, prefix, observed, func(records map[string]string) bool {
		return records[zfsPool] != ""
	})
	takeover := connectBrokerFixture(t, brokerURL, disabledCfg.ClientID)
	takeover.Disconnect(250)
	waitForRetained(t, peer, prefix, observed, recordFilter)
	assertBrokerCommand(t, peer, legacy.buildCommandTopic("system", "fixture-only"), false)
	client.Disconnect()

	enabled := NewClient(cfg, host, "new", nil)
	if err := enabled.Connect(ctx); err != nil {
		t.Fatal(err)
	}
	defer enabled.Disconnect()
	deadline := time.Now().Add(5 * time.Second)
	for !enabled.IsConnected() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if err := enabled.PublishContainers([]dto.ContainerInfo{{Name: "media"}}); err != nil {
		t.Fatal(err)
	}
	waitForRetained(t, peer, prefix, observed, func(records map[string]string) bool {
		return allRetained(records, containerState, containerSensor, containerButton, legacy.buildTopic("docker/containers"))
	})
	assertBrokerCommand(t, peer, legacy.buildCommandTopic("system", "fixture-only"), true)
	enabled.Disconnect()

	// A parent agent's container can also occupy a nested agent's exact
	// system suffix. Verify cleanup removes the nested agent's own PSU data
	// while preserving the retained container at the overlapping address.
	legacy.publishContainerDiscovery([]dto.ContainerInfo{{Name: "system", Image: "fixture"}})
	nestedCfg := *cfg
	nestedCfg.ClientID += "-nested"
	nestedCfg.TopicPrefix += "/docker"
	nestedCfg.DisabledTopics = []string{TopicSystem, TopicPSU}
	nestedCfg.HomeAssistantMode = false
	nestedClient := NewClient(&nestedCfg, "neighbor", "new", nil)
	nestedPSU := nestedClient.buildTopic(TopicPSU)
	waitForBrokerToken(t, peer.Publish(nestedPSU, 1, true, `{"fixture":true}`))
	observedNested := []string{nestedSystem, nestedPSU}
	containerRetained := func(records map[string]string) bool {
		return strings.Contains(records[nestedSystem], `"image":"fixture"`)
	}
	waitForRetained(t, peer, prefix, observedNested, func(records map[string]string) bool {
		return containerRetained(records) && records[nestedPSU] != ""
	})
	if err := nestedClient.Connect(ctx); err != nil {
		t.Fatal(err)
	}
	defer nestedClient.Disconnect()
	waitForRetained(t, peer, prefix, observedNested, func(records map[string]string) bool {
		return containerRetained(records) && records[nestedPSU] == ""
	})
}

func connectBrokerFixture(t *testing.T, brokerURL, id string) pahomqtt.Client {
	t.Helper()
	opts := pahomqtt.NewClientOptions().AddBroker(brokerURL).SetClientID(id).
		SetCleanSession(true).SetAutoReconnect(false).SetConnectTimeout(5 * time.Second)
	client := pahomqtt.NewClient(opts)
	waitForBrokerToken(t, client.Connect())
	t.Cleanup(func() { client.Disconnect(250) })
	return client
}

func waitForBrokerToken(t *testing.T, token pahomqtt.Token) {
	t.Helper()
	if !token.WaitTimeout(5*time.Second) || token.Error() != nil {
		t.Fatalf("fixture MQTT operation failed: %v", token.Error())
	}
}

func waitForRetained(t *testing.T, peer pahomqtt.Client, prefix string, topics []string, check func(map[string]string) bool) {
	t.Helper()
	deadline := time.Now().Add(6 * time.Second)
	for {
		records := brokerRetained(t, peer, prefix, topics)
		if check(records) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("retained topics did not reach expected state: %v", retainedNames(records))
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// Observe a subscription's initial retained replay, followed by a marker
// published on the same connection after SUBACK.
func brokerRetained(t *testing.T, peer pahomqtt.Client, prefix string, topics []string) map[string]string {
	t.Helper()
	marker := prefix + "/fixture-marker"
	filters := map[string]byte{marker: 1}
	for _, topic := range topics {
		filters[topic] = 1
	}
	seen := make(chan pahomqtt.Message, len(topics)*2+1)
	waitForBrokerToken(t, peer.SubscribeMultiple(filters, func(_ pahomqtt.Client, msg pahomqtt.Message) {
		seen <- msg
	}))
	waitForBrokerToken(t, peer.Publish(marker, 1, false, "end"))
	records := make(map[string]string)
	timeout := time.NewTimer(5 * time.Second)
	defer timeout.Stop()
	for {
		select {
		case msg := <-seen:
			if msg.Topic() == marker {
				waitForBrokerToken(t, peer.Unsubscribe(append(slices.Clone(topics), marker)...))
				return records
			}
			if msg.Retained() && len(msg.Payload()) > 0 {
				records[msg.Topic()] = string(msg.Payload())
			}
		case <-timeout.C:
			t.Fatal("MQTT snapshot marker not received")
		}
	}
}

func retainedNames(records map[string]string) string {
	names := make([]string, 0, len(records))
	for topic := range records {
		names = append(names, topic)
	}
	slices.Sort(names)
	return strings.Join(names, ", ")
}

func allRetained(records map[string]string, topics ...string) bool {
	for _, topic := range topics {
		if records[topic] == "" {
			return false
		}
	}
	return true
}

func anyRetained(records map[string]string, topics ...string) bool {
	for _, topic := range topics {
		if records[topic] != "" {
			return true
		}
	}
	return false
}

func assertBrokerCommand(t *testing.T, peer pahomqtt.Client, command string, enabled bool) {
	t.Helper()
	reply := make(chan []byte, 1)
	waitForBrokerToken(t, peer.Subscribe(command+"/result", 1, func(_ pahomqtt.Client, msg pahomqtt.Message) {
		select {
		case reply <- msg.Payload():
		default:
		}
	}))
	defer func() { waitForBrokerToken(t, peer.Unsubscribe(command+"/result")) }()
	// This action is intentionally invalid. It exercises routing without
	// invoking a state-changing controller even in the enabled control case.
	window := 500 * time.Millisecond
	if enabled {
		window = 3 * time.Second
	}
	deadline := time.After(window)
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		waitForBrokerToken(t, peer.Publish(command, 1, false, "fixture"))
		select {
		case payload := <-reply:
			var result struct {
				Success bool   `json:"success"`
				Error   string `json:"error"`
			}
			if !enabled || json.Unmarshal(payload, &result) != nil || result.Success || result.Error == "" {
				t.Fatalf("unexpected MQTT command result: %s", payload)
			}
			return
		case <-deadline:
			if enabled {
				t.Fatal("enabled MQTT command probe did not reach dispatch")
			}
			return
		case <-ticker.C:
		}
	}
}

type retainedFixtureMessage struct {
	pahomqtt.Message
	topic    string
	payload  string
	retained bool
}

func (m retainedFixtureMessage) Topic() string   { return m.topic }
func (m retainedFixtureMessage) Payload() []byte { return []byte(m.payload) }
func (m retainedFixtureMessage) Retained() bool  { return m.retained }

type retainedFixtureClient struct {
	pahomqtt.Client
	filters map[string]byte
	message retainedFixtureMessage
	deleted chan string
}

func (b *retainedFixtureClient) SubscribeMultiple(filters map[string]byte, handle pahomqtt.MessageHandler) pahomqtt.Token {
	b.filters = filters
	handle(b, b.message)
	return doneToken{}
}

func (b *retainedFixtureClient) Publish(topic string, qos byte, retained bool, payload any) pahomqtt.Token {
	if qos == 1 && retained && payload == "" {
		b.deleted <- topic
	}
	return doneToken{}
}

func TestReconcileRetainedWithScopedFilters(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DisableCommands = true
	cfg.DisabledTopics = []string{TopicContainers, TopicZFSDatasets}
	client := NewClient(cfg, "test tower", "test", nil)
	topic := client.buildTopic("docker/media")
	broker := &retainedFixtureClient{
		message: retainedFixtureMessage{topic: topic, payload: `{"name":"media","image":""}`, retained: true},
		deleted: make(chan string, 1),
	}
	client.client = broker
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client.reconcileRetained(ctx)
	filters := []string{"unraid/docker/containers", "unraid/docker/+", "unraid/zfs/datasets",
		"unraid/zfs/datasets/+", "homeassistant/+/test_tower/+/config"}
	if len(broker.filters) != len(filters) {
		t.Fatalf("cleanup subscriptions = %v", broker.filters)
	}
	for _, filter := range filters {
		if broker.filters[filter] != 1 {
			t.Errorf("expected QoS 1 subscription to %q, got %v", filter, broker.filters)
		}
	}
	select {
	case got := <-broker.deleted:
		if got != topic {
			t.Errorf("retained deletion topic = %q, want %q", got, topic)
		}
	case <-time.After(time.Second):
		t.Fatal("cleanup did not publish a retained deletion")
	}
}

func TestRetainedCleanupOwnership(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DisabledTopics = []string{TopicPSU, TopicZFSPools}
	client := NewClient(cfg, "tower", "test", nil)

	record := func(state, availability string) string {
		return `{"state_topic":"` + state + `","unique_id":"unraid_tower_fixture","device":{"identifiers":["unraid_tower"]},"availability":` +
			`[{"topic":"` + availability + `"}]}`
	}
	discovery := "homeassistant/sensor/tower/fixture/config"
	for _, tc := range []struct {
		name     string
		message  retainedFixtureMessage
		deletion bool
	}{
		{"live", retainedFixtureMessage{topic: "unraid/psu", payload: "fixture"}, false},
		{"tombstone", retainedFixtureMessage{topic: "unraid/psu", retained: true}, false},
		{"pool", retainedFixtureMessage{topic: "unraid/zfs/tank", payload: `{"name":"tank","capacity_percent":0}`, retained: true}, true},
		{"legacy named pool", retainedFixtureMessage{topic: "unraid/zfs/arc", payload: `{"name":"arc","capacity_percent":0}`, retained: true}, true},
		{"enabled ARC", retainedFixtureMessage{topic: "unraid/zfs/arc", payload: `{"target_size_bytes":0}`, retained: true}, false},
		{"custom payload", retainedFixtureMessage{topic: "unraid/zfs/tank", payload: `{"name":"tank"}`, retained: true}, false},
		{"dataset", retainedFixtureMessage{topic: "unraid/zfs/datasets", payload: "fixture", retained: true}, false},
		{"foreign telemetry", retainedFixtureMessage{topic: "other/psu", payload: "fixture", retained: true}, false},
		{"availability", retainedFixtureMessage{topic: "unraid/availability", payload: "online", retained: true}, false},
		{"owned discovery", retainedFixtureMessage{topic: discovery, payload: record("unraid/psu", "unraid/availability"), retained: true}, true},
		{"enabled discovery", retainedFixtureMessage{topic: discovery, payload: record("unraid/system", "unraid/availability"), retained: true}, false},
		{"foreign discovery", retainedFixtureMessage{topic: discovery, payload: record("unraid/psu", "other/availability"), retained: true}, false},
		{"foreign host", retainedFixtureMessage{topic: "homeassistant/sensor/other/fixture/config", payload: record("unraid/psu", "unraid/availability"), retained: true}, false},
		{"unknown discovery", retainedFixtureMessage{topic: discovery, payload: `{"state_topic":"unraid/psu"}`, retained: true}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := client.shouldClearRetained(tc.message); got != tc.deletion {
				t.Errorf("retained deletion = %v, want %v", got, tc.deletion)
			}
		})
	}
}

func TestZFSPoolWithAggregateNameKeepsItsDiscovery(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.DisabledTopics = []string{TopicZFSARC}
	client := NewClient(cfg, "tower", "test", nil)
	capture := &capturingClient{published: make(map[string]string)}
	client.client = capture
	client.connected.Store(true)
	client.publishZFSDiscovery([]dto.ZFSPool{{Name: "arc", Health: "ONLINE"}})
	client.publishZFSARCDiscovery()
	if err := client.PublishZFSARCStats(dto.ZFSARCStats{}); err != nil {
		t.Fatal(err)
	}
	poolTopic := "unraid/zfs/pools/arc"
	if !strings.Contains(capture.published[poolTopic], `"name":"arc"`) {
		t.Fatal("enabled pool did not publish state on its distinct topic")
	}
	if !strings.Contains(capture.published["homeassistant/sensor/tower/zfs_arc_health/config"], poolTopic) {
		t.Fatal("pool discovery did not retain its identity and reference the distinct topic")
	}
	if _, found := capture.published["unraid/zfs/arc"]; found {
		t.Fatal("disabled ARC topic received state")
	}
	if _, found := capture.published["homeassistant/sensor/tower/arc_size/config"]; found {
		t.Fatal("disabled ARC topic published discovery")
	}
}

func TestExactCleanupPreservesParentResource(t *testing.T) {
	cfg := DefaultConfig()
	cfg.TopicPrefix = "unraid/docker"
	cfg.DisabledTopics = []string{TopicSystem}
	client := NewClient(cfg, "neighbor", "test", nil)
	msg := retainedFixtureMessage{topic: "unraid/docker/system", retained: true}
	msg.payload = `{"name":"system","image":"fixture"}`
	if client.shouldClearRetained(msg) {
		t.Fatal("cleanup selected the parent agent's container payload")
	}
	msg.payload = `{"hostname":"neighbor"}`
	if !client.shouldClearRetained(msg) {
		t.Fatal("cleanup did not select the local agent's system payload")
	}
}

func TestRetainedCleanupMatchesLegacyGPUIdentity(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DisabledTopics = []string{TopicGPU}
	client := NewClient(cfg, "tower", "test", nil)
	msg := retainedFixtureMessage{topic: "unraid/gpu/0", retained: true}
	for payload, want := range map[string]bool{
		`{"index":0,"driver_version":"fixture"}`:    true,
		`{"index":1,"driver_version":"fixture"}`:    false,
		`{"index":null,"driver_version":"fixture"}`: false,
		`{"driver_version":"fixture"}`:              false,
	} {
		msg.payload = payload
		if client.shouldClearRetained(msg) != want {
			t.Errorf("GPU retained identity match for %s, want %v", payload, want)
		}
	}
}

func TestCancelledRetainedCleanupStopsWaitingForAck(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if waitForCleanupToken(ctx, pendingCleanupToken{done: make(chan struct{})}) {
		t.Fatal("cancelled cleanup reported a completed acknowledgement")
	}
}

type pendingCleanupToken struct {
	pahomqtt.Token
	done chan struct{}
}

func (t pendingCleanupToken) Done() <-chan struct{} { return t.done }

func TestZFSSelectionsKeepEnabledCollectionsPublishing(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.HomeAssistantMode = false
	cfg.DisabledTopics = []string{TopicZFSARC}
	client := NewClient(cfg, "tower", "test", nil)
	capture := &capturingClient{published: make(map[string]string)}
	client.client = capture
	client.connected.Store(true)
	for _, err := range []error{
		client.PublishZFSPools([]dto.ZFSPool{{Name: "tank"}}),
		client.PublishZFSDatasets([]dto.ZFSDataset{{Name: "tank/media"}}),
		client.PublishZFSSnapshots([]dto.ZFSSnapshot{{Name: "tank/media@fixture"}}),
		client.PublishZFSARCStats(dto.ZFSARCStats{}),
	} {
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, topic := range []string{"unraid/zfs/pools", "unraid/zfs/datasets", "unraid/zfs/snapshots"} {
		if capture.published[topic] == "" {
			t.Errorf("enabled ZFS collection did not publish on %s", topic)
		}
	}
	if capture.published["unraid/zfs/arc"] != "" {
		t.Fatal("disabled ZFS ARC collection published state")
	}
}

func TestLegacyZFSCleanupKeepsNewlyPublishedAggregate(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.DisabledTopics = []string{TopicZFSPools}
	client := NewClient(cfg, "tower", "test", nil)
	capture := &capturingClient{published: make(map[string]string)}
	client.client = capture
	client.connected.Store(true)

	topic := "unraid/zfs/arc"
	legacy := retainedFixtureMessage{topic: topic, payload: `{"name":"arc","capacity_percent":0}`, retained: true}
	if !client.shouldClearRetained(legacy) {
		t.Fatal("disabled legacy pool incorrectly classified as enabled ARC")
	}
	if err := client.PublishZFSARCStats(dto.ZFSARCStats{TargetSizeBytes: 42}); err != nil {
		t.Fatal(err)
	}
	if !client.clearRetained(t.Context(), topic) {
		t.Fatal("cleanup should be satisfied by the replacement publication")
	}
	if !strings.Contains(capture.published[topic], `"target_size_bytes":42`) {
		t.Fatal("cleanup deleted the enabled ARC sample")
	}
}

func TestMQTTTelemetryAndCommandDiscoveryPolicy(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.DisableCommands = true
	cfg.DisabledTopics = []string{TopicContainers, TopicNotificationEvent, TopicFanControl, TopicServices}
	client := NewClient(cfg, "tower", "test", nil)
	capture := &psuCaptureClient{published: make(map[string]string)}
	client.client = capture
	client.connected.Store(true)

	if err := client.PublishContainers([]dto.ContainerInfo{{Name: "media"}}); err != nil {
		t.Fatal(err)
	}
	if err := client.PublishFanControlStatus(&dto.FanControlStatus{}); err != nil {
		t.Fatal(err)
	}
	client.publishServiceStates()
	if err := client.PublishNotifications(&dto.NotificationList{}); err != nil {
		t.Fatal(err)
	}
	client.publishHAEntity(haEntityOpts{entityType: "button", id: "reboot", commandTopic: "unraid/cmd/system/reboot"})
	client.publishHAEntity(haEntityOpts{entityType: "event", id: "event", stateTopic: "unraid/notifications/event"})
	client.publishHAEntity(haEntityOpts{entityType: "sensor", id: "system", stateTopic: "unraid/system"})
	got := capture.snapshot()
	if len(got) != 2 || got["unraid/notifications"] == "" || got["homeassistant/sensor/tower/system/config"] == "" {
		t.Fatalf("unexpected publications: %v", got)
	}

	cfg.DisableCommands = false
	controlsEnabled := NewClient(cfg, "tower", "test", nil)
	if controlsEnabled.allowDiscovery("", "", "unraid/cmd/docker/media/restart") {
		t.Error("disabled Docker telemetry exposed a Docker button")
	}
	if !controlsEnabled.allowDiscovery("", "", "unraid/cmd/system/reboot") {
		t.Error("disabled Docker telemetry hid a system button")
	}
}

func TestRetainedCleanupDoesNotSubscribeWithUnsafeFilters(t *testing.T) {
	for _, tc := range []struct {
		name, host string
		config     dto.MQTTConfig
	}{
		{"pre-upgrade defaults", "tower", dto.MQTTConfig{}},
		{"wildcard in telemetry prefix", "tower", dto.MQTTConfig{TopicPrefix: "bad/+", DisabledTopics: []string{TopicPSU}}},
		{"invalid host", "bad/host", dto.MQTTConfig{DisableCommands: true}},
		{"invalid discovery prefix", "tower", dto.MQTTConfig{HADiscoveryPrefix: "bad/#", DisableCommands: true}},
		{"unknown topic cannot create a filter", "", dto.MQTTConfig{DisabledTopics: []string{"foo/#"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := NewClient(&tc.config, tc.host, "test", nil)
			// No MQTT client is necessary if no safe cleanup filters exist.
			client.reconcileRetained(t.Context())
		})
	}
}

func TestNotificationEventsCanPublishWithoutStatus(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.DisabledTopics = []string{TopicNotifications}
	client := NewClient(cfg, "tower", "test", nil)
	capture := &psuCaptureClient{published: make(map[string]string)}
	client.client = capture
	client.connected.Store(true)
	for _, snapshot := range []*dto.NotificationList{
		{},
		{Notifications: []dto.Notification{{ID: "fixture", Importance: "warning"}}},
	} {
		if err := client.PublishNotifications(snapshot); err != nil {
			t.Fatal(err)
		}
	}
	if got := capture.snapshot(); len(got) != 1 || got["unraid/notifications/event"] == "" {
		t.Fatalf("unexpected notification publications: %v", got)
	}
}
