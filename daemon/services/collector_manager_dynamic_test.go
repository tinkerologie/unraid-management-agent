package services

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tinkerologie/unraid-management-agent/daemon/constants"
	"github.com/tinkerologie/unraid-management-agent/daemon/domain"
	"github.com/tinkerologie/unraid-management-agent/daemon/dto"
	"github.com/tinkerologie/unraid-management-agent/daemon/services/scheduler"
)

// newDynamicManager returns a Dynamic-mode manager on a fake clock whose
// collectors are fakes, so no real collector runs.
func newDynamicManager(t *testing.T) (*CollectorManager, map[string]*fakeDynamic) {
	t.Helper()
	ctx := &domain.Context{
		Hub:                domain.NewEventBus(100),
		CollectionMode:     domain.CollectionModeDynamic,
		DisabledCollectors: map[string]bool{"gpu": true},
		GatedCollectors:    map[string]bool{"ups": true},
		AutoCollectors:     map[string]bool{"ups": true},
	}
	var wg sync.WaitGroup
	cm := NewCollectorManager(ctx, &wg)
	cm.clock = scheduler.NewFakeClock(runnerEpoch)
	fakes := map[string]*fakeDynamic{}
	for _, name := range []string{"docker", "gpu", "ups", "system"} {
		f := newFakeDynamic()
		fakes[name] = f
		cm.Register(name, func(*domain.Context) Collector { return dynCollect{f} }, 0, name == "system")
	}
	t.Cleanup(func() {
		cm.StopAll()
		wg.Wait()
	})
	return cm, fakes
}

func mustStatus(t *testing.T, cm *CollectorManager, name string) *dto.CollectorStatus {
	t.Helper()
	st, err := cm.GetStatus(name)
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func TestDynamicRegisterAndStart(t *testing.T) {
	cm, fakes := newDynamicManager(t)
	if !cm.Dynamic() || cm.Demand() == nil {
		t.Fatal("manager should be in Dynamic mode with a demand tracker")
	}

	docker := mustStatus(t, cm, "docker")
	if !docker.Enabled || docker.Status != "registered" || docker.Mode != "dynamic" || docker.Reason != "event" || !docker.EventDriven {
		t.Fatalf("docker before start = %+v", docker)
	}
	if gpu := mustStatus(t, cm, "gpu"); gpu.Enabled || gpu.Status != "disabled" || gpu.HardwareGated {
		t.Fatalf("disabled-list collector = %+v", gpu)
	}
	if ups := mustStatus(t, cm, "ups"); !ups.HardwareGated || ups.Auto {
		t.Fatalf("gated collector = %+v, want gated without auto", ups)
	}

	if n := cm.StartAll(); n != 3 {
		t.Fatalf("StartAll started %d, want 3 (docker, ups, system)", n)
	}
	fakes["docker"].waitCall(t) // docker has no initial delay
	eventually(t, "docker status", func() bool {
		st := mustStatus(t, cm, "docker")
		return st.Status == "running" && st.LastRun != nil && st.EffectiveInterval == 300 && st.Reason == "event"
	})
	all := cm.GetAllStatus()
	if all.Mode != "dynamic" {
		t.Fatalf("GetAllStatus mode = %q", all.Mode)
	}

	// Interval changes are refused; unknown collectors still error first.
	if err := cm.UpdateInterval("docker", 60); !errors.Is(err, ErrDynamicMode) {
		t.Fatalf("UpdateInterval = %v, want ErrDynamicMode", err)
	}
	if err := cm.UpdateInterval("nope", 60); err == nil || errors.Is(err, ErrDynamicMode) {
		t.Fatalf("unknown collector = %v", err)
	}

	// Disable stops the runner; Enable restarts it.
	if err := cm.DisableCollector("docker"); err != nil {
		t.Fatal(err)
	}
	if st := mustStatus(t, cm, "docker"); st.Status != "stopped" || st.EffectiveInterval != 0 || st.Reason != "event" {
		t.Fatalf("stopped docker = %+v", st)
	}
	if _, ok := cm.RequestRefresh("docker"); ok {
		t.Fatal("a stopped collector cannot refresh")
	}
	eventually(t, "docker Close", fakes["docker"].closed.Load)
	if err := cm.EnableCollector("docker"); err != nil {
		t.Fatal(err)
	}
	fakes["docker"].waitCall(t)
	if st := mustStatus(t, cm, "docker"); st.Status != "running" {
		t.Fatalf("re-enabled docker = %+v", st)
	}
	done, ok := cm.RequestRefresh("docker")
	if !ok {
		t.Fatal("a running collector should accept refreshes")
	}
	cm.clock.(*scheduler.FakeClock).Advance(2 * time.Second) // past MinGap
	if err := waitChan(t, done, "refresh"); err != nil {
		t.Fatalf("refresh = %v", err)
	}
	if _, ok := cm.RequestRefresh("nope"); ok {
		t.Fatal("unknown collector cannot refresh")
	}
}

func TestDynamicDemandAndFloors(t *testing.T) {
	cm, fakes := newDynamicManager(t)
	cm.StartAll()
	fakes["docker"].waitCall(t)
	reason := func(want string, interval int) func() bool {
		return func() bool {
			st := mustStatus(t, cm, "docker")
			return st.Reason == want && st.EffectiveInterval == interval
		}
	}
	eventually(t, "docker idle", reason("event", 300))

	cm.Demand().RecordRead("docker")
	eventually(t, "docker active", reason("active", 30))

	cm2, f2 := newDynamicManager(t)
	cm2.StartAll()
	f2["docker"].waitCall(t)
	if cm2.floorRequired("docker") {
		t.Fatal("no floors set yet")
	}
	cm2.SetFloors(map[string]bool{"docker": true})
	if !cm2.floorRequired("docker") {
		t.Fatal("docker should be floored")
	}
	eventually(t, "docker floor", func() bool { return mustStatus(t, cm2, "docker").Reason == "floor" })
	cm2.SetFloors(map[string]bool{"system": true})
	if cm2.floorRequired("docker") {
		t.Fatal("docker floor should be gone")
	}
	eventually(t, "docker back to event", func() bool { return mustStatus(t, cm2, "docker").Reason == "event" })
	cm2.wakeRunners([]string{"nope", "gpu"}) // unknown and stopped names are ignored
}

func TestDynamicApplyHardwareState(t *testing.T) {
	cm, fakes := newDynamicManager(t)
	events := cm.domainCtx.Hub.Sub(constants.TopicCollectorStateChange.Name)
	defer cm.domainCtx.Hub.Unsub(events)

	if !cm.ApplyHardwareState("ups", true, 300) {
		t.Fatal("present hardware should start ups")
	}
	ev := waitChan(t, events, "state change").(dto.CollectorStateEvent)
	if ev.Collector != "ups" || !ev.Enabled {
		t.Fatalf("event = %+v", ev)
	}
	if st := mustStatus(t, cm, "ups"); st.Status != "running" {
		t.Fatalf("ups = %+v", st)
	}
	if cm.ApplyHardwareState("ups", true, 300) {
		t.Fatal("present again is a no-op")
	}
	if !cm.ApplyHardwareState("ups", false, 300) {
		t.Fatal("absent hardware should stop ups")
	}
	if ev := waitChan(t, events, "state change").(dto.CollectorStateEvent); ev.Enabled {
		t.Fatalf("event = %+v", ev)
	}
	if st := mustStatus(t, cm, "ups"); st.Status != "disabled" || st.Enabled {
		t.Fatalf("ups = %+v", st)
	}
	if cm.ApplyHardwareState("ups", false, 300) {
		t.Fatal("absent again is a no-op")
	}
	if cm.ApplyHardwareState("docker", true, 300) {
		t.Fatal("ungated collectors are left alone")
	}
	_ = fakes
}

func TestDynamicStartWithoutContract(t *testing.T) {
	ctx := &domain.Context{Hub: domain.NewEventBus(10), CollectionMode: domain.CollectionModeDynamic}
	var wg sync.WaitGroup
	cm := NewCollectorManager(ctx, &wg)
	cm.Register("nopolicy", func(*domain.Context) Collector { return startOnly{} }, 10, false)
	cm.Register("system", func(*domain.Context) Collector { return startOnly{} }, 10, true)
	if n := cm.StartAll(); n != 2 {
		t.Fatalf("StartAll counted %d", n)
	}
	for _, name := range []string{"nopolicy", "system"} {
		if st := mustStatus(t, cm, name); st.Status == "running" {
			t.Fatalf("%s should not run: %+v", name, st)
		}
	}
	cm.StopAll()
	wg.Wait()
}

func TestDynamicPublishCountAndManualStatus(t *testing.T) {
	ctx := createTestContext()
	var wg sync.WaitGroup
	cm := NewCollectorManager(ctx, &wg)
	mock := newMockCollector()
	cm.Register("docker", func(*domain.Context) Collector { return mock }, 10, false)
	if cm.Dynamic() {
		t.Fatal("default mode is manual")
	}

	st := mustStatus(t, cm, "docker")
	if st.Mode != "manual" || st.Reason != "manual" || st.EffectiveInterval != 0 || st.EventDriven {
		t.Fatalf("registered manual status = %+v", st)
	}
	cm.StartAll()
	mock.waitStarted(t)
	if st := mustStatus(t, cm, "docker"); st.EffectiveInterval != 10 || st.Reason != "manual" {
		t.Fatalf("running manual status = %+v", st)
	}
	if _, ok := cm.RequestRefresh("docker"); ok {
		t.Fatal("Manual mode has no refresh requests")
	}
	if all := cm.GetAllStatus(); all.Mode != "manual" {
		t.Fatalf("mode = %q", all.Mode)
	}

	ctx.Hub.Pub("x", constants.TopicContainerListUpdate.Name)
	ctx.Hub.Pub("y", constants.TopicContainerListUpdate.Name)
	if got := mustStatus(t, cm, "docker").PublishCount; got != 2 {
		t.Fatalf("PublishCount = %d, want 2", got)
	}
	cm.StopAll()
	wg.Wait()
}

func TestErrDynamicModeMessage(t *testing.T) {
	if !strings.Contains(ErrDynamicMode.Error(), "manual mode") || !errors.Is(ErrDynamicMode, domain.ErrDynamicCollectionMode) {
		t.Fatal("ErrDynamicMode should be the domain sentinel")
	}
}
