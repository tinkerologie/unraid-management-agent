package services

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eclipse/paho.mqtt.golang/packets"

	"github.com/tinkerologie/unraid-management-agent/daemon/constants"
	"github.com/tinkerologie/unraid-management-agent/daemon/domain"
	"github.com/tinkerologie/unraid-management-agent/daemon/dto"
	"github.com/tinkerologie/unraid-management-agent/daemon/platform"
	"github.com/tinkerologie/unraid-management-agent/daemon/services/api"
	"github.com/tinkerologie/unraid-management-agent/daemon/services/collectors"
)

func TestPSULifecycleGatingAndInvalidation(t *testing.T) {
	for _, mode := range []string{domain.CollectionModeManual, domain.CollectionModeDynamic} {
		t.Run(mode, func(t *testing.T) {
			hub := domain.NewEventBus(30)
			app := &domain.Context{Hub: hub, CollectionMode: mode, GatedCollectors: map[string]bool{"psu": true}}
			var wg sync.WaitGroup
			cm := NewCollectorManager(app, &wg)
			cm.Register("psu", func(*domain.Context) Collector { return newMockCollector() }, 60, false)
			// A Dynamic mock needs a Collect method, supplied by the adapter below.
			if mode == domain.CollectionModeDynamic {
				cm.collectors["psu"].factory = func(*domain.Context) Collector { return psuLifecycleMock{} }
			}
			updates := hub.Sub(constants.TopicPSUStatusUpdate.Name)
			defer hub.Unsub(updates)
			defer wg.Wait()
			defer cm.StopAll()
			policy := newCollectorAutoPolicy(cm, false)
			apply := func(state dto.CapabilityState) {
				policy.Apply(snapshot(map[string]dto.CapabilityState{collectors.CapabilityPSU: state}))
			}
			apply(dto.CapabilityAbsent)
			if got := (<-updates).(*dto.PSUStatus); got.State != dto.PSUStateNotDetected {
				t.Fatal(got)
			}
			apply(dto.CapabilityPresent)
			if st, _ := cm.GetStatus("psu"); st.Status != "running" {
				t.Fatalf("not enabled: %+v", st)
			}
			ctx, cancel := context.WithCancel(context.Background())
			working := &dto.PSUStatus{State: dto.PSUStateOK, Devices: []dto.PSUDevice{{ID: "a"}}}
			cm.publishPSU(ctx, working)
			if got := (<-updates).(*dto.PSUStatus); got != working {
				t.Fatal("working snapshot missing")
			}
			apply(dto.CapabilityAbsent)
			if got := (<-updates).(*dto.PSUStatus); len(got.Devices) != 0 {
				t.Fatal("removal retained readings")
			}
			// First absence clears availability before the two-absence stop policy.
			cm.publishPSU(ctx, working)
			select {
			case <-updates:
				t.Fatal("old in-flight snapshot restored readings")
			default:
			}
			apply(dto.CapabilityAbsent)
			if st, _ := cm.GetStatus("psu"); st.Status != "disabled" {
				t.Fatalf("not stopped: %+v", st)
			}
			// Drain stop's availability publication.
			for len(updates) > 0 {
				<-updates
			}
			cancel()
			cm.publishPSU(ctx, working)
			select {
			case <-updates:
				t.Fatal("cancelled run published")
			default:
			}
			apply(dto.CapabilityPresent)
			if st, _ := cm.GetStatus("psu"); st.Status != "running" {
				t.Fatal("not restarted")
			}
			if err := cm.DisableCollector("psu"); err != nil {
				t.Fatal(err)
			}
			if got := (<-updates).(*dto.PSUStatus); got.State != dto.PSUStateDisabled {
				t.Fatal(got)
			}
			apply(dto.CapabilityPresent)
			if st, _ := cm.GetStatus("psu"); st.Enabled {
				t.Fatal("manual disable overridden")
			}
		})
	}
}

type psuLifecycleMock struct{}

func (psuLifecycleMock) Start(ctx context.Context, _ time.Duration) { <-ctx.Done() }
func (psuLifecycleMock) Collect()                                   {}

type psuInventoryEvent struct{ started chan<- struct{} }

func (s psuInventoryEvent) Run(ctx context.Context, trigger func()) {
	trigger()
	s.started <- struct{}{}
	<-ctx.Done()
}

func TestPSUScopedInventoryRefresh(t *testing.T) {
	registry := platform.NewRegistry()
	registry.SetCapabilities(dto.Capabilities{Items: []dto.Capability{{Name: "other", State: dto.CapabilityPresent}}})
	calls := 0
	r := NewCapabilityRefresher(registry, func() []platform.Probe {
		return []platform.Probe{
			{Name: collectors.CapabilityPSU, Kind: platform.ProbeCheck, Check: func(context.Context) (dto.CapabilityState, string) { calls++; return dto.CapabilityPresent, "USB PSU" }},
			{Name: "other", Kind: platform.ProbeCheck, Check: func(context.Context) (dto.CapabilityState, string) {
				t.Error("unrelated probe ran")
				return dto.CapabilityAbsent, ""
			}},
		}
	})
	hooks := make(chan dto.Capabilities, 10)
	r.SetHook(func(c dto.Capabilities) { hooks <- c })
	r.RefreshCapability(context.Background(), collectors.CapabilityPSU)
	previous := registry.Capabilities()
	if len((<-hooks).Items) != 1 {
		t.Fatal("hook advanced unrelated absence counters")
	}
	r.RefreshCapability(context.Background(), collectors.CapabilityPSU)
	<-hooks
	if previous.Items[0].State != dto.CapabilityPresent {
		t.Fatal("old snapshot mutated")
	}
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{}, 1)
	done := make(chan struct{})
	go func() { runPSUInventory(ctx, r, time.Millisecond, psuInventoryEvent{started}); close(done) }()
	<-started
	select {
	case <-hooks:
	case <-time.After(time.Second):
		t.Fatal("no event-triggered refresh")
	}
	select {
	case <-hooks:
	case <-time.After(time.Second):
		t.Fatal("no safety-net refresh")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("inventory watcher did not stop")
	}
	if calls < 4 {
		t.Fatal("missing scoped refreshes")
	}
	r.RefreshCapability(ctx, collectors.CapabilityPSU)
	r.RefreshCapability(context.Background(), "missing")
}

func TestPSUAvailabilityTransitionsStayQuiet(t *testing.T) {
	hub := domain.NewEventBus(20)
	app := &domain.Context{Hub: hub, GatedCollectors: map[string]bool{"psu": true}, Intervals: domain.Intervals{PSU: 60}}
	var wg sync.WaitGroup
	cm := NewCollectorManager(app, &wg)
	cm.RegisterAllCollectors()
	collector, ok := cm.collectors["psu"].factory(app).(*collectors.PSUCollector)
	if !ok || collector.PublishGuard == nil {
		t.Fatal("PSU registration lacks guarded publication")
	}
	updates := hub.Sub(constants.TopicPSUStatusUpdate.Name)
	defer hub.Unsub(updates)
	cm.publishPSU(context.Background(), dto.NewPSUStatus(dto.PSUStateNotDetected, time.Now()))
	select {
	case <-updates:
		t.Fatal("stopped collector published")
	default:
	}
	cm.setPSUPresence(dto.CapabilityAbsent)
	if got := (<-updates).(*dto.PSUStatus); got.State != dto.PSUStateNotDetected {
		t.Fatal(got)
	}
	cm.setPSUPresence(dto.CapabilityAbsent)
	cm.setPSUPresence(dto.CapabilityUnknown)
	select {
	case <-updates:
		t.Fatal("unchanged absence generated traffic")
	default:
	}
	cm.collectors["psu"].configured = 0
	cm.setPSUPresence(dto.CapabilityPresent)
	if got := (<-updates).(*dto.PSUStatus); got.State != dto.PSUStateDisabled {
		t.Fatal(got)
	}
	cm.setPSUPresence(dto.CapabilityPresent)
	select {
	case <-updates:
		t.Fatal("unchanged disabled state generated traffic")
	default:
	}
	if err := cm.DisableCollector("psu"); err != nil {
		t.Fatal(err)
	}
	cm.setPSUPresence(dto.CapabilityAbsent)
	select {
	case <-updates:
		t.Fatal("explicit disable overridden by detection")
	default:
	}
}

func TestPSUDisabledSnapshotAfterSubscribersReady(t *testing.T) {
	hub := domain.NewEventBus(10)
	app := &domain.Context{Hub: hub, DisabledCollectors: map[string]bool{"psu": true}}
	var wg sync.WaitGroup
	cm := NewCollectorManager(app, &wg)
	cm.RegisterAllCollectors()
	// Registration precedes subscriptions in both orchestrator entry points.
	updates := hub.Sub(constants.TopicPSUStatusUpdate.Name)
	defer hub.Unsub(updates)
	cm.initializePSUAvailability()
	select {
	case v := <-updates:
		if v.(*dto.PSUStatus).State != dto.PSUStateDisabled {
			t.Fatal(v)
		}
	case <-time.After(time.Second):
		t.Fatal("startup disabled state lost")
	}
	cm.initializePSUAvailability()
	select {
	case <-updates:
		t.Fatal("duplicate initialization event")
	default:
	}
}

func TestPSURealFactoryRunsInDynamicMode(t *testing.T) {
	root := t.TempDir()
	write := func(name, value string) {
		t.Helper()
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("bus/usb/devices/1-1/idVendor", "1b1c")
	write("bus/usb/devices/1-1/idProduct", "1c07")
	write("class/hwmon/hwmon12/name", "corsairpsu")
	write("class/hwmon/hwmon12/power1_input", "0")
	hid := filepath.Join(root, "bus/usb/devices/1-1/1-1:1.0/0003:1B1C:1C07.0001")
	driver := filepath.Join(root, "bus/hid/drivers/corsair-psu")
	for _, path := range []string{hid, driver} {
		if err := os.MkdirAll(path, 0755); err != nil {
			t.Fatal(err)
		}
	}
	for link, target := range map[string]string{filepath.Join(hid, "driver"): driver, filepath.Join(root, "class/hwmon/hwmon12/device"): hid} {
		if err := os.Symlink(target, link); err != nil {
			t.Fatal(err)
		}
	}
	hub := domain.NewEventBus(10)
	app := &domain.Context{Hub: hub, CollectionMode: domain.CollectionModeDynamic, GatedCollectors: map[string]bool{"psu": true}}
	var wg sync.WaitGroup
	cm := NewCollectorManager(app, &wg)
	cm.RegisterAllCollectors()
	original := cm.collectors["psu"].factory
	cm.collectors["psu"].factory = func(ctx *domain.Context) Collector {
		c := original(ctx).(*collectors.PSUCollector)
		c.SysfsRoot = root
		return c
	}
	updates := hub.Sub(constants.TopicPSUStatusUpdate.Name)
	defer hub.Unsub(updates)
	defer wg.Wait()
	defer cm.StopAll()
	cm.setPSUPresence(dto.CapabilityPresent)
	cm.ApplyHardwareState("psu", true, 60)
	select {
	case v := <-updates:
		s := v.(*dto.PSUStatus)
		if s.State != dto.PSUStateOK || len(s.Devices) != 1 {
			t.Fatal("real Dynamic collector did not publish telemetry", s)
		}
	case <-time.After(time.Second):
		t.Fatal("real PSU factory not adapted by Dynamic runner")
	}
}

func TestPSUAndFullRefreshHooksAreSerialized(t *testing.T) {
	r := NewCapabilityRefresher(platform.NewRegistry(), func() []platform.Probe {
		return []platform.Probe{{Name: collectors.CapabilityPSU, Kind: platform.ProbeCheck, Check: func(context.Context) (dto.CapabilityState, string) { return dto.CapabilityPresent, "PSU" }}}
	})
	r.MinInterval = -1
	var active, calls atomic.Int64
	r.SetHook(func(dto.Capabilities) {
		if active.Add(1) != 1 {
			t.Error("capability hooks overlapped")
		}
		time.Sleep(time.Millisecond)
		active.Add(-1)
		calls.Add(1)
	})
	var wg sync.WaitGroup
	wg.Go(func() {
		for range 5 {
			r.Refresh(context.Background())
		}
	})
	wg.Go(func() {
		for range 5 {
			r.RefreshCapability(context.Background(), collectors.CapabilityPSU)
		}
	})
	wg.Wait()
	if calls.Load() != 10 {
		t.Fatal("refreshes missing", calls.Load())
	}
}

func TestPSUIntervalRestartPreservesAvailability(t *testing.T) {
	hub := domain.NewEventBus(10)
	app := &domain.Context{Hub: hub}
	var wg sync.WaitGroup
	cm := NewCollectorManager(app, &wg)
	cm.Register("psu", func(*domain.Context) Collector { return newMockCollector() }, 60, false)
	updates := hub.Sub(constants.TopicPSUStatusUpdate.Name)
	defer hub.Unsub(updates)
	defer wg.Wait()
	defer cm.StopAll()
	if err := cm.EnableCollector("psu"); err != nil {
		t.Fatal(err)
	}
	oldCtx := cm.collectors["psu"].ctx
	working := &dto.PSUStatus{State: dto.PSUStateOK, Devices: []dto.PSUDevice{{ID: "psu-a"}}}
	cm.publishPSU(oldCtx, working)
	<-updates
	if err := cm.UpdateInterval("psu", 120); err != nil {
		t.Fatal(err)
	}
	cm.publishPSU(oldCtx, working)
	select {
	case s := <-updates:
		t.Fatal("interval restart withdrew availability or accepted an obsolete run", s)
	default:
	}
}

// Exercise the real subscriber startup barrier against a local MQTT peer, so
// the first disabled snapshot cannot disappear before the MQTT subscription.
func TestPSUInitialDisabledStateReachesMQTT(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	published := make(chan *packets.PublishPacket, 10)
	peerDone := make(chan struct{})
	go func() {
		defer close(peerDone)
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		if err := conn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
			return
		}
		if _, err := packets.ReadPacket(conn); err != nil {
			return
		}
		if err := packets.NewControlPacket(packets.Connack).Write(conn); err != nil {
			return
		}
		for {
			packet, err := packets.ReadPacket(conn)
			if err != nil {
				return
			}
			if packet, ok := packet.(*packets.PublishPacket); ok {
				published <- packet
			}
			if _, ok := packet.(*packets.DisconnectPacket); ok {
				return
			}
		}
	}()
	config := domain.DefaultMQTTConfig()
	config.Enabled = true
	config.Broker = "127.0.0.1"
	config.Port = listener.Addr().(*net.TCPAddr).Port
	app := &domain.Context{Hub: domain.NewEventBus(10), MQTTConfig: config, DisabledCollectors: map[string]bool{"psu": true}}
	o := CreateOrchestrator(app)
	var wg sync.WaitGroup
	o.collectorManager = NewCollectorManager(app, &wg)
	o.collectorManager.RegisterAllCollectors()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer wg.Wait()
	defer cancel()
	o.initializeMQTT(ctx, &wg, api.NewServer(app))
	if o.mqttClient == nil {
		t.Fatal("MQTT client not initialized")
	}
	defer o.mqttClient.Disconnect()
	// OnConnect may finish after Connect's token; online confirms its completion.
	select {
	case packet := <-published:
		if !strings.HasSuffix(packet.TopicName, "/availability") {
			t.Fatal("missing online availability")
		}
	case <-ctx.Done():
		t.Fatal("MQTT peer did not become online")
	}
	o.collectorManager.initializePSUAvailability()
	select {
	case packet := <-published:
		if !strings.HasSuffix(packet.TopicName, "/psu") {
			t.Fatal("PSU snapshot not forwarded")
		}
		var snapshot dto.PSUStatus
		if err := json.Unmarshal(packet.Payload, &snapshot); err != nil {
			t.Fatal(err)
		}
		if snapshot.State != dto.PSUStateDisabled || len(snapshot.Devices) != 0 {
			t.Fatalf("unexpected initial snapshot: %+v", snapshot)
		}
	case <-ctx.Done():
		t.Fatal("initial PSU snapshot lost before subscriber readiness")
	}
	cancel()
	o.mqttClient.Disconnect()
	select {
	case <-peerDone:
	case <-time.After(time.Second):
		t.Fatal("MQTT peer did not stop")
	}
}

func TestPSUInventoryRecoversFromEventAndHookPanics(t *testing.T) {
	r := NewCapabilityRefresher(platform.NewRegistry(), func() []platform.Probe {
		return []platform.Probe{{Name: collectors.CapabilityPSU, Kind: platform.ProbeCheck, Check: func(context.Context) (dto.CapabilityState, string) { return dto.CapabilityAbsent, "no PSU" }}}
	})
	var calls atomic.Int64
	recovered := make(chan struct{})
	r.SetHook(func(dto.Capabilities) {
		if calls.Add(1) == 1 {
			panic("inventory hook failure")
		}
		if calls.Load() == 2 {
			close(recovered)
		}
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { runPSUInventory(ctx, r, time.Millisecond, panicSource{}); close(done) }()
	select {
	case <-recovered:
	case <-time.After(time.Second):
		t.Fatal("inventory did not recover after event/hook failure")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("inventory did not stop")
	}
}

func TestPSUScopedRefreshDiscardsCancelledProbe(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := NewCapabilityRefresher(platform.NewRegistry(), func() []platform.Probe {
		return []platform.Probe{{Name: collectors.CapabilityPSU, Kind: platform.ProbeCheck, Check: func(context.Context) (dto.CapabilityState, string) {
			cancel()
			return dto.CapabilityPresent, "stale evidence"
		}}}
	})
	r.SetHook(func(dto.Capabilities) { t.Error("cancelled probe was published") })
	r.RefreshCapability(ctx, collectors.CapabilityPSU)
	if len(r.registry.Capabilities().Items) != 0 {
		t.Fatal("cancelled probe updated registry")
	}
}
