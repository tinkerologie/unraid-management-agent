package services

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tinkerologie/unraid-management-agent/daemon/constants"
	"github.com/tinkerologie/unraid-management-agent/daemon/domain"
	"github.com/tinkerologie/unraid-management-agent/daemon/dto"
	"github.com/tinkerologie/unraid-management-agent/daemon/platform"
	"github.com/tinkerologie/unraid-management-agent/daemon/services/collectors"
)

// newAutoTestManager registers mock collectors: nic and storage_topology in
// automatic mode, gpu gated with an explicit 60 s interval, zfs gated and
// explicitly disabled, and ups not gated (as if in the disabled list).
func newAutoTestManager(t *testing.T) (*CollectorManager, map[string]*mockCollector) {
	t.Helper()
	ctx := &domain.Context{
		Hub:             domain.NewEventBus(100),
		GatedCollectors: map[string]bool{"nic": true, "storage_topology": true, "gpu": true, "zfs": true},
		AutoCollectors:  map[string]bool{"nic": true, "storage_topology": true, "gpu": true},
	}
	var wg sync.WaitGroup
	cm := NewCollectorManager(ctx, &wg)
	mocks := map[string]*mockCollector{}
	for name, interval := range map[string]int{"nic": 0, "storage_topology": 0, "gpu": 60, "zfs": 0, "ups": 60} {
		m := newMockCollector()
		mocks[name] = m
		cm.Register(name, func(*domain.Context) Collector { return m }, interval, false)
	}
	t.Cleanup(func() {
		cm.StopAll()
		wg.Wait()
	})
	return cm, mocks
}

// snapshot builds a capability snapshot with the given hardware states.
func snapshot(states map[string]dto.CapabilityState) dto.Capabilities {
	caps := dto.Capabilities{}
	for name, state := range states {
		caps.Items = append(caps.Items, dto.Capability{Name: name, State: state, Available: state == dto.CapabilityPresent, Detail: "test"})
	}
	return caps
}

// status returns a collector's status or fails the test.
func status(t *testing.T, cm *CollectorManager, name string) *dto.CollectorStatus {
	t.Helper()
	s, err := cm.GetStatus(name)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// TestCollectorManagerHardwareGating covers ApplyHardwareState for auto, explicit, disabled and ungated collectors.
func TestCollectorManagerHardwareGating(t *testing.T) {
	cm, mocks := newAutoTestManager(t)

	if s := status(t, cm, "nic"); !s.HardwareGated || !s.Auto || s.Enabled || s.Status != "disabled" {
		t.Fatalf("nic should register as gated, auto and disabled: %+v", s)
	}
	if s := status(t, cm, "gpu"); !s.HardwareGated || s.Auto {
		t.Fatalf("gpu has an explicit interval: gated but not auto: %+v", s)
	}
	if s := status(t, cm, "ups"); s.HardwareGated || s.Auto {
		t.Fatalf("ups is not gated: %+v", s)
	}
	if n := cm.StartAll(); n != 2 {
		t.Fatalf("StartAll started %d collectors, want 2 (gpu, ups)", n)
	}
	mocks["gpu"].waitStarted(t)

	// Automatic mode: present runs at the auto interval, absent stops it.
	if !cm.ApplyHardwareState("nic", true, 300) {
		t.Fatal("nic present reported no change")
	}
	mocks["nic"].waitStarted(t)
	if s := status(t, cm, "nic"); s.Status != "running" || s.Interval != 300 || !s.Enabled {
		t.Fatalf("nic after detection: %+v", s)
	}
	if cm.ApplyHardwareState("nic", true, 300) {
		t.Error("same state reported a change")
	}
	if !cm.ApplyHardwareState("nic", false, 300) {
		t.Fatal("nic absent reported no change")
	}
	mocks["nic"].waitDone(t)
	if s := status(t, cm, "nic"); s.Status != "disabled" || s.Interval != 0 || s.Enabled || !s.Auto {
		t.Fatalf("nic after removal: %+v", s)
	}
	if cm.ApplyHardwareState("nic", false, 300) {
		t.Error("stopping a stopped collector reported a change")
	}

	// An explicit interval: absent stops it, present restores the saved interval.
	if !cm.ApplyHardwareState("gpu", false, 300) {
		t.Fatal("gpu absent reported no change")
	}
	mocks["gpu"].waitDone(t)
	if s := status(t, cm, "gpu"); s.Status != "disabled" || s.Interval != 0 {
		t.Fatalf("explicit gpu kept running while absent: %+v", s)
	}
	if !cm.ApplyHardwareState("gpu", true, 300) {
		t.Fatal("gpu present reported no change")
	}
	mocks["gpu"].waitStarted(t)
	if s := status(t, cm, "gpu"); s.Interval != 60 || s.Status != "running" {
		t.Fatalf("gpu did not return to its explicit 60 s: %+v", s)
	}

	// Explicitly disabled stays off when present; ungated and unknown are left alone.
	if cm.ApplyHardwareState("zfs", true, 300) {
		t.Error("explicitly disabled zfs was started")
	}
	if cm.ApplyHardwareState("ups", false, 300) || cm.ApplyHardwareState("missing", true, 300) {
		t.Error("ApplyHardwareState changed a collector that is not gated")
	}
	if s := status(t, cm, "ups"); s.Status != "running" {
		t.Fatalf("ungated ups changed: %+v", s)
	}
}

// TestCollectorManagerGatedRestartsAtNewInterval checks a running gated collector restarts at a new interval.
func TestCollectorManagerGatedRestartsAtNewInterval(t *testing.T) {
	cm, mocks := newAutoTestManager(t)
	cm.ApplyHardwareState("nic", true, 300)
	mocks["nic"].waitStarted(t)
	if !cm.ApplyHardwareState("nic", true, 1200) {
		t.Fatal("interval change reported no change")
	}
	mocks["nic"].waitStarted(t)
	if s := status(t, cm, "nic"); s.Interval != 1200 || s.Status != "running" {
		t.Fatalf("nic after interval change: %+v", s)
	}
}

// TestCollectorManagerManualControlEndsGating checks REST/MCP style control takes a collector out of hardware control.
func TestCollectorManagerManualControlEndsGating(t *testing.T) {
	tests := []struct {
		name string
		act  func(cm *CollectorManager) error
	}{
		{"enable", func(cm *CollectorManager) error { return cm.EnableCollector("nic") }},
		{"disable", func(cm *CollectorManager) error { return cm.DisableCollector("nic") }},
		{"interval", func(cm *CollectorManager) error { return cm.UpdateInterval("nic", 120) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cm, _ := newAutoTestManager(t)
			if err := tt.act(cm); err != nil {
				t.Fatal(err)
			}
			if s := status(t, cm, "nic"); s.HardwareGated || s.Auto {
				t.Fatalf("manual control left the collector gated: %+v", s)
			}
			if s := status(t, cm, "storage_topology"); !s.HardwareGated || !s.Auto {
				t.Fatal("manual control of nic changed another collector")
			}
			if cm.ApplyHardwareState("nic", false, 300) {
				t.Fatal("policy changed a manually controlled collector")
			}
		})
	}
}

// TestCollectorManagerInvalidIntervalKeepsGating checks a rejected interval change keeps hardware control.
func TestCollectorManagerInvalidIntervalKeepsGating(t *testing.T) {
	cm, _ := newAutoTestManager(t)
	if err := cm.UpdateInterval("nic", 1); err == nil {
		t.Fatal("expected invalid interval error")
	}
	if s := status(t, cm, "nic"); !s.HardwareGated || !s.Auto {
		t.Fatal("a rejected interval change ended hardware gating")
	}
}

// TestCollectorAutoPolicy walks hardware absent, added, unknown, flaky and removed through the policy.
func TestCollectorAutoPolicy(t *testing.T) {
	cm, mocks := newAutoTestManager(t)
	cm.StartAll()
	mocks["gpu"].waitStarted(t)
	policy := newCollectorAutoPolicy(cm, false)
	nic, storage, gpu := collectors.CapabilityNICOptics, collectors.CapabilityStorageTopology, collectors.CapabilityGPU

	// Reference server: optics, SAS and GPU absent. Never-seen hardware stops at
	// once, including the explicit gpu interval.
	policy.Apply(snapshot(map[string]dto.CapabilityState{nic: dto.CapabilityAbsent, storage: dto.CapabilityAbsent, gpu: dto.CapabilityAbsent}))
	mocks["gpu"].waitDone(t)
	for _, name := range []string{"nic", "storage_topology", "gpu"} {
		if s := status(t, cm, name); s.Status == "running" || s.Interval != 0 {
			t.Fatalf("%s runs although its hardware is absent: %+v", name, s)
		}
	}

	// SAS HBA added: storage topology runs at the auto interval; the GPU
	// returns to its explicit interval.
	policy.Apply(snapshot(map[string]dto.CapabilityState{storage: dto.CapabilityPresent, gpu: dto.CapabilityPresent}))
	mocks["storage_topology"].waitStarted(t)
	mocks["gpu"].waitStarted(t)
	if s := status(t, cm, "storage_topology"); s.Interval != constants.IntervalAutoEnable || s.Status != "running" {
		t.Fatalf("storage_topology after detection: %+v", s)
	}
	if s := status(t, cm, "gpu"); s.Interval != 60 {
		t.Fatalf("gpu after detection: %+v", s)
	}

	// Present again: nothing to change.
	policy.Apply(snapshot(map[string]dto.CapabilityState{storage: dto.CapabilityPresent, gpu: dto.CapabilityPresent}))
	if s := status(t, cm, "storage_topology"); s.Interval != constants.IntervalAutoEnable || s.Status != "running" {
		t.Fatalf("storage_topology after a repeated detection: %+v", s)
	}
	select {
	case <-mocks["storage_topology"].startChan:
		t.Fatal("a repeated detection restarted storage_topology")
	case <-time.After(50 * time.Millisecond):
	}

	// Unknown and missing results change nothing.
	policy.Apply(snapshot(map[string]dto.CapabilityState{storage: dto.CapabilityUnknown}))
	policy.Apply(dto.Capabilities{})
	if s := status(t, cm, "storage_topology"); s.Status != "running" {
		t.Fatalf("unknown result stopped the collector: %+v", s)
	}

	// Seen hardware: one absent result keeps it; an unknown in between resets the count.
	policy.Apply(snapshot(map[string]dto.CapabilityState{storage: dto.CapabilityAbsent}))
	policy.Apply(snapshot(map[string]dto.CapabilityState{storage: dto.CapabilityUnknown}))
	policy.Apply(snapshot(map[string]dto.CapabilityState{storage: dto.CapabilityAbsent}))
	if s := status(t, cm, "storage_topology"); s.Status != "running" {
		t.Fatalf("non-consecutive absent results stopped the collector: %+v", s)
	}

	// Two consecutive absent results: the hardware was removed.
	policy.Apply(snapshot(map[string]dto.CapabilityState{storage: dto.CapabilityAbsent}))
	mocks["storage_topology"].waitDone(t)
	if s := status(t, cm, "storage_topology"); s.Status != "disabled" || s.Interval != 0 {
		t.Fatalf("storage_topology after removal: %+v", s)
	}

	// Explicitly disabled zfs stays off when present; ungated ups is never touched.
	policy.Apply(snapshot(map[string]dto.CapabilityState{collectors.CapabilityZFS: dto.CapabilityPresent, collectors.CapabilityUPS: dto.CapabilityAbsent}))
	if s := status(t, cm, "zfs"); s.Status == "running" {
		t.Fatalf("policy enabled explicitly disabled zfs: %+v", s)
	}
	if s := status(t, cm, "ups"); s.Status != "running" {
		t.Fatalf("policy stopped ungated ups: %+v", s)
	}
}

// TestCollectorAutoPolicyLowPower checks the auto interval is multiplied by 4 in low power mode.
func TestCollectorAutoPolicyLowPower(t *testing.T) {
	cm, mocks := newAutoTestManager(t)
	newCollectorAutoPolicy(cm, true).Apply(snapshot(map[string]dto.CapabilityState{collectors.CapabilityNICOptics: dto.CapabilityPresent}))
	mocks["nic"].waitStarted(t)
	if s := status(t, cm, "nic"); s.Interval != constants.IntervalAutoEnable*4 {
		t.Fatalf("low power interval = %d, want %d", s.Interval, constants.IntervalAutoEnable*4)
	}
}

// TestHiddenSectionsNeverRun asserts the core rule of #261 on the reference
// server: every hardware class detected as absent leaves its collector off,
// whether it is in automatic mode or has an explicit interval, when the
// policy runs before StartAll as the orchestrator does.
func TestHiddenSectionsNeverRun(t *testing.T) {
	gated, auto := map[string]bool{}, map[string]bool{}
	intervals := map[string]int{}
	for i, c := range collectors.HardwareClasses {
		if c.Collector == "" {
			continue
		}
		gated[c.Collector] = true
		if i%2 == 0 {
			auto[c.Collector] = true
		} else {
			intervals[c.Collector] = 60 // an explicit interval saved before
		}
	}
	ctx := &domain.Context{Hub: domain.NewEventBus(100), GatedCollectors: gated, AutoCollectors: auto}
	var wg sync.WaitGroup
	cm := NewCollectorManager(ctx, &wg)
	for name := range gated {
		cm.Register(name, func(*domain.Context) Collector { return newMockCollector() }, intervals[name], false)
	}
	t.Cleanup(func() { cm.StopAll(); wg.Wait() })

	absent := map[string]dto.CapabilityState{}
	for _, c := range collectors.HardwareClasses {
		absent[c.Capability] = dto.CapabilityAbsent
	}
	newCollectorAutoPolicy(cm, false).Apply(snapshot(absent))
	if n := cm.StartAll(); n != 0 {
		t.Errorf("StartAll started %d collectors with all hardware absent", n)
	}
	for name := range gated {
		if s := status(t, cm, name); s.Status == "running" || s.Enabled || s.Interval != 0 {
			t.Errorf("%s runs although its hardware is absent: %+v", name, s)
		}
	}
}

// TestCapabilityRefresher covers refresh, rate limiting, the hook, snapshot immutability and detached contexts.
func TestCapabilityRefresher(t *testing.T) {
	reg := platform.NewRegistry()
	var checks atomic.Int32
	state := atomic.Value{}
	state.Store(dto.CapabilityAbsent)
	probes := func() []platform.Probe {
		return []platform.Probe{{Name: "hardware.test", Kind: platform.ProbeCheck, Check: func(ctx context.Context) (dto.CapabilityState, string) {
			checks.Add(1)
			if _, ok := ctx.Deadline(); !ok {
				t.Error("check ran without a timeout")
			}
			return state.Load().(dto.CapabilityState), "detail"
		}}}
	}
	r := NewCapabilityRefresher(reg, probes)
	clock := time.Date(2026, 10, 14, 9, 0, 0, 0, time.UTC)
	r.Now = func() time.Time { return clock }
	var hooked []dto.Capabilities
	r.SetHook(func(c dto.Capabilities) { hooked = append(hooked, c) })

	first := r.Refresh(context.Background())
	if checks.Load() != 1 || len(hooked) != 1 {
		t.Fatalf("first refresh: checks=%d hooks=%d", checks.Load(), len(hooked))
	}
	if it, _ := reg.Capabilities().Item("hardware.test"); it.State != dto.CapabilityAbsent {
		t.Fatalf("registry not updated: %+v", reg.Capabilities())
	}

	// Within MinInterval: cached, no check, no hook.
	state.Store(dto.CapabilityPresent)
	clock = clock.Add(10 * time.Second)
	if c := r.Refresh(context.Background()); checks.Load() != 1 || len(hooked) != 1 || c.Items[0].State != dto.CapabilityAbsent {
		t.Fatalf("rate limit: checks=%d hooks=%d state=%q", checks.Load(), len(hooked), c.Items[0].State)
	}

	// After MinInterval: a new snapshot; the old one is unchanged.
	clock = clock.Add(constants.CapabilityMinRefreshInterval)
	second := r.Refresh(context.Background())
	if second.Items[0].State != dto.CapabilityPresent || len(hooked) != 2 {
		t.Fatalf("second refresh: %+v hooks=%d", second.Items, len(hooked))
	}
	if first.Items[0].State != dto.CapabilityAbsent {
		t.Fatal("a refresh modified an earlier snapshot")
	}

	// A cancelled request context does not leave a half-checked snapshot.
	clock = clock.Add(time.Hour)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if c := r.Refresh(ctx); c.Items[0].State != dto.CapabilityPresent {
		t.Fatalf("cancelled caller: state = %q, want present", c.Items[0].State)
	}
}

// TestCapabilityRefresherConcurrentCallers checks concurrent refreshes run the checks once.
func TestCapabilityRefresherConcurrentCallers(t *testing.T) {
	reg := platform.NewRegistry()
	var checks atomic.Int32
	release := make(chan struct{})
	r := NewCapabilityRefresher(reg, func() []platform.Probe {
		return []platform.Probe{{Name: "hardware.slow", Kind: platform.ProbeCheck, Check: func(context.Context) (dto.CapabilityState, string) {
			checks.Add(1)
			<-release
			return dto.CapabilityPresent, ""
		}}}
	})
	var wg sync.WaitGroup
	for range 5 {
		wg.Go(func() { r.Refresh(context.Background()) })
	}
	time.Sleep(50 * time.Millisecond)
	close(release)
	wg.Wait()
	if n := checks.Load(); n != 1 {
		t.Fatalf("concurrent refreshes ran %d checks, want 1", n)
	}
}

// TestCapabilityRefresherStart checks periodic refreshes, hook panic recovery and stop on cancel.
func TestCapabilityRefresherStart(t *testing.T) {
	reg := platform.NewRegistry()
	ticks := make(chan struct{}, 10)
	r := NewCapabilityRefresher(reg, func() []platform.Probe {
		return []platform.Probe{{Name: "hardware.tick", Kind: platform.ProbeCheck, Check: func(context.Context) (dto.CapabilityState, string) {
			ticks <- struct{}{}
			return dto.CapabilityAbsent, ""
		}}}
	})
	r.Interval = 10 * time.Millisecond
	r.MinInterval = 0
	r.SetHook(func(dto.Capabilities) { panic("hook panic is recovered") })
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { r.Start(ctx); close(done) }()
	for range 2 {
		select {
		case <-ticks:
		case <-time.After(2 * time.Second):
			t.Fatal("refresher did not tick")
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("refresher did not stop on cancel")
	}
}
