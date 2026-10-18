package alerting

import (
	"maps"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/tinkerologie/unraid-management-agent/daemon/dto"
)

func collectorSet(names ...string) map[string]bool {
	m := make(map[string]bool, len(names))
	for _, n := range names {
		m[n] = true
	}
	return m
}

func TestRequiredCollectors(t *testing.T) {
	tests := []struct {
		name  string
		rules []dto.AlertRule
		want  map[string]bool
	}{
		{"no rules", nil, collectorSet()},
		{"simple identifier", []dto.AlertRule{{Expression: "CPU > 90", Enabled: true}}, collectorSet("system")},
		{"multiple identifiers", []dto.AlertRule{{Expression: "MaxDiskTemp > 50 && ArrayState == 'Started' || NUTStatus == 'OB'", Enabled: true}}, collectorSet("disk", "array", "nut")},
		{"member access uses root identifier", []dto.AlertRule{{Expression: "UPSStatus.foo == 'x'", Enabled: true}}, collectorSet("ups")},
		{"function call arguments", []dto.AlertRule{{Expression: "max(MaxGPUTemp, CPUTemp) > 80 && len(BootPoolHealth) > 0", Enabled: true}}, collectorSet("gpu", "system", "zfs")},
		{"multiple collectors per field", []dto.AlertRule{{Expression: "ContainerUpdatesAvailable > 0", Enabled: true}}, collectorSet("docker", "docker_update")},
		{"unknown identifiers ignored", []dto.AlertRule{{Expression: "let x = Bogus; x > 1 && foo(VMCount)", Enabled: true}}, collectorSet("vm")},
		{"field without collector", []dto.AlertRule{{Expression: "DegradedSubsystemCount > 0", Enabled: true}}, collectorSet()},
		{"disabled rule excluded", []dto.AlertRule{
			{Expression: "CPU > 90", Enabled: true},
			{Expression: "UnreadNotifications > 0", Enabled: false},
		}, collectorSet("system")},
		{"parse failure requires all sources", []dto.AlertRule{
			{Expression: "CPU > 90", Enabled: true},
			{Expression: "CPU >", Enabled: true},
		}, allAlertSources()},
		{"disabled parse failure ignored", []dto.AlertRule{{Expression: "CPU >", Enabled: false}}, collectorSet()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := requiredCollectors(tt.rules); !maps.Equal(got, tt.want) {
				t.Errorf("requiredCollectors() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAllAlertSourcesCoversTaggedCollectors(t *testing.T) {
	all := allAlertSources()
	for _, c := range []string{"system", "array", "disk", "docker", "docker_update", "vm", "ups", "nut", "gpu", "zfs", "network", "notification", "plugin_update"} {
		if !all[c] {
			t.Errorf("allAlertSources() missing %q", c)
		}
	}
	if len(all) != 13 {
		t.Errorf("allAlertSources() = %v, want 13 collectors", slices.Sorted(maps.Keys(all)))
	}
}

// TestBuiltinRulesRequiredCollectors reports the collectors the built-in
// rules keep at their default interval in Dynamic mode.
func TestBuiltinRulesRequiredCollectors(t *testing.T) {
	got := requiredCollectors(BuiltinRules())
	t.Logf("built-in rules require: %v", slices.Sorted(maps.Keys(got)))
	if len(got) != 0 {
		t.Errorf("built-in rules require %v, want none", got)
	}
}

func TestEvalIntervalUnchanged(t *testing.T) {
	if EvalInterval != 15*time.Second {
		t.Errorf("EvalInterval = %s, want 15s", EvalInterval)
	}
}

// floorRecorder records every set delivered to the floor listener.
type floorRecorder struct {
	mu   sync.Mutex
	sets []map[string]bool
}

func (r *floorRecorder) record(m map[string]bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sets = append(r.sets, m)
}

func (r *floorRecorder) last(t *testing.T) map[string]bool {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.sets) == 0 {
		t.Fatal("floor listener never called")
	}
	return r.sets[len(r.sets)-1]
}

func TestEngineFloorListenerPropagation(t *testing.T) {
	store := NewStore(t.TempDir())
	e := NewEngine(store, nil)
	rec := &floorRecorder{}

	e.SetFloorListener(rec.record)
	if got := rec.last(t); len(got) != 0 {
		t.Fatalf("initial floor = %v, want empty", got)
	}

	rule := dto.AlertRule{ID: "cpu", Name: "CPU", Expression: "CPU > 90", Enabled: true}
	steps := []struct {
		name   string
		change func() error
		want   map[string]bool
	}{
		{"create", func() error { return store.CreateRule(rule) }, collectorSet("system")},
		{"update expression", func() error {
			rule.Expression = "CPU > 90 && MaxDiskTemp > 50"
			return store.UpdateRule(rule)
		}, collectorSet("system", "disk")},
		{"disable", func() error {
			rule.Enabled = false
			return store.UpdateRule(rule)
		}, collectorSet()},
		{"enable", func() error {
			rule.Enabled = true
			return store.UpdateRule(rule)
		}, collectorSet("system", "disk")},
		{"delete", func() error { return store.DeleteRule(rule.ID) }, collectorSet()},
	}
	for _, step := range steps {
		if err := step.change(); err != nil {
			t.Fatalf("%s: %v", step.name, err)
		}
		e.RecompileRules()
		if got := rec.last(t); !maps.Equal(got, step.want) {
			t.Errorf("%s: floor = %v, want %v", step.name, got, step.want)
		}
		if got := e.RequiredCollectors(); !maps.Equal(got, step.want) {
			t.Errorf("%s: RequiredCollectors() = %v, want %v", step.name, got, step.want)
		}
	}
}

func TestEngineFloorListenerReentrant(t *testing.T) {
	store := NewStore(t.TempDir())
	if err := store.CreateRule(dto.AlertRule{ID: "gpu", Expression: "MaxGPUTemp > 80", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	e := NewEngine(store, nil)
	rec := &floorRecorder{}
	reentered := false
	e.SetFloorListener(func(m map[string]bool) {
		rec.record(m)
		_ = e.RequiredCollectors()
		if !reentered {
			reentered = true
			e.RecompileRules() // must not deadlock; delivered by the outer call
		}
	})

	done := make(chan struct{})
	go func() {
		e.RecompileRules()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("RecompileRules deadlocked with a re-entrant floor listener")
	}
	if got := rec.last(t); !maps.Equal(got, collectorSet("gpu")) {
		t.Errorf("floor = %v, want gpu", got)
	}
}

func TestEngineFloorListenerConcurrent(t *testing.T) {
	store := NewStore(t.TempDir())
	if err := store.CreateRule(dto.AlertRule{ID: "net", Expression: "NetworkErrors > 0", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	e := NewEngine(store, nil)
	rec := &floorRecorder{}
	e.SetFloorListener(rec.record)

	var wg sync.WaitGroup
	for range 8 {
		wg.Go(e.RecompileRules)
	}
	wg.Wait()
	if got := rec.last(t); !maps.Equal(got, collectorSet("network")) {
		t.Errorf("floor = %v, want network", got)
	}
}

func TestRequiredCollectorsReturnsCopy(t *testing.T) {
	e := NewEngine(NewStore(t.TempDir()), nil)
	got := e.RequiredCollectors()
	if got == nil {
		t.Fatal("RequiredCollectors() = nil, want empty map")
	}
	got["system"] = true
	if e.RequiredCollectors()["system"] {
		t.Error("mutating the returned map changed the engine state")
	}
}
