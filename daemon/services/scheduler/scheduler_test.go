package scheduler

import (
	"slices"
	"sort"
	"testing"
	"time"
)

var epoch = time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)

// manualDefaults are the plugin's default.cfg intervals ("auto" = 300 s).
var manualDefaults = map[string]time.Duration{
	"psu":    60 * sec,
	"system": 15 * sec, "array": 60 * sec, "disk": 300 * sec, "network": 60 * sec,
	"docker": 30 * sec, "vm": 60 * sec, "gpu": 300 * sec, "ups": 300 * sec, "nut": 300 * sec,
	"hardware": 600 * sec, "shares": 60 * sec, "zfs": 300 * sec, "unassigned": 60 * sec,
	"notification": 30 * sec, "registration": 600 * sec, "storage_topology": 300 * sec,
	"nic": 300 * sec, "fancontrol": 60 * sec, "tuning": 120 * sec, "docker_update": 21600 * sec,
	"docker_networks": 60 * sec, "plugin_update": 3600 * sec, "os_update": 86400 * sec, "mover": 30 * sec,
}

func TestRealClock(t *testing.T) {
	c := RealClock{}
	if d := time.Since(c.Now()); d < 0 || d > time.Minute {
		t.Fatalf("Now off by %v", d)
	}
	tm := c.NewTimer(time.Hour)
	if !tm.Stop() {
		t.Fatal("Stop of an active timer should report true")
	}
	if tm.Reset(time.Millisecond) {
		t.Fatal("Reset of a stopped timer should report false")
	}
	select {
	case <-tm.C():
	case <-time.After(5 * time.Second):
		t.Fatal("real timer did not fire")
	}
}

func fired(tm Timer) bool {
	select {
	case <-tm.C():
		return true
	default:
		return false
	}
}

func TestFakeClock(t *testing.T) {
	c := NewFakeClock(epoch)
	if !c.Now().Equal(epoch) {
		t.Fatal("Now should start at epoch")
	}

	changed := c.Changed()
	tm := c.NewTimer(10 * sec)
	select {
	case <-changed:
	default:
		t.Fatal("NewTimer should close Changed")
	}
	if got := c.Pending(); len(got) != 1 || got[0] != 10*sec {
		t.Fatalf("Pending = %v, want [10s]", got)
	}

	c.Advance(9 * sec)
	if fired(tm) {
		t.Fatal("timer fired early")
	}
	c.Advance(sec)
	if !fired(tm) {
		t.Fatal("timer did not fire at its deadline")
	}
	if len(c.Pending()) != 0 {
		t.Fatal("a fired timer should not be pending")
	}
	if tm.Stop() {
		t.Fatal("Stop after fire should report false")
	}

	// Reset discards a fire that was never received.
	if tm.Reset(sec) {
		t.Fatal("Reset of an inactive timer should report false")
	}
	c.Advance(sec) // fires; nobody receives it
	if tm.Reset(5 * sec) {
		t.Fatal("Reset after an unreceived fire should report false")
	}
	if fired(tm) {
		t.Fatal("Reset should discard an unreceived fire")
	}
	if !tm.Stop() {
		t.Fatal("Stop of an active timer should report true")
	}
	c.Advance(time.Hour)
	if fired(tm) {
		t.Fatal("a stopped timer must not fire")
	}

	// Zero and negative delays fire at once.
	if z := c.NewTimer(0); !fired(z) {
		t.Fatal("zero-delay timer should fire immediately")
	}
	n := c.NewTimer(sec)
	n.Reset(-sec)
	if !fired(n) {
		t.Fatal("negative reset should fire immediately")
	}
}

func TestPolicies(t *testing.T) {
	seen := map[string]string{}
	for name, want := range manualDefaults {
		p, ok := PolicyFor(name)
		if !ok {
			t.Fatalf("collector %s has no policy", name)
		}
		if p.Active > want {
			t.Errorf("%s: Active %v exceeds Manual default %v", name, p.Active, want)
		}
		if p.Idle < want {
			t.Errorf("%s: Idle %v below Manual default %v", name, p.Idle, want)
		}
		if p.Floor != want {
			t.Errorf("%s: Floor %v != Manual default %v", name, p.Floor, want)
		}
		if p.Staleness != p.Active {
			t.Errorf("%s: Staleness %v != Active %v", name, p.Staleness, p.Active)
		}
		if p.MinGap <= 0 {
			t.Errorf("%s: MinGap must be positive", name)
		}
		if p.Active == p.Idle && p.RefreshOnRead {
			t.Errorf("%s: fixed schedules must not refresh on read", name)
		}
		if len(p.Topics) == 0 {
			t.Errorf("%s: no topics", name)
		}
		for _, topic := range p.Topics {
			if other, dup := seen[topic]; dup {
				t.Errorf("topic %s used by %s and %s", topic, other, name)
			}
			seen[topic] = name
			if got, ok := CollectorForTopic(topic); !ok || got != name {
				t.Errorf("CollectorForTopic(%s) = %q, %v; want %s", topic, got, ok, name)
			}
		}
	}
	names := CollectorNames()
	if len(names) != len(manualDefaults) {
		t.Errorf("CollectorNames has %d entries, want %d", len(names), len(manualDefaults))
	}
	if _, ok := PolicyFor("nope"); ok {
		t.Error("unknown collector should have no policy")
	}
	if _, ok := CollectorForTopic("nope"); ok {
		t.Error("unknown topic should have no collector")
	}
	if p, _ := PolicyFor("plugin_update"); !p.EventDriven || p.MinGap != time.Minute || p.InitialDelay != 30*sec {
		t.Errorf("plugin_update policy = %+v", p)
	}
	if p, _ := PolicyFor("nic"); p.InitialDelay != 10*sec {
		t.Errorf("nic InitialDelay = %v, want 10s", p.InitialDelay)
	}
}

func TestDecide(t *testing.T) {
	demandP := Policy{Active: 15 * sec, Idle: 60 * sec, Floor: 30 * sec}
	eventP := Policy{Active: 30 * sec, Idle: 300 * sec, Floor: 30 * sec, EventDriven: true}
	slowFloor := Policy{Active: 15 * sec, Idle: 60 * sec, Floor: 120 * sec}
	last := epoch

	tests := []struct {
		name   string
		p      Policy
		in     Inputs
		want   time.Duration
		reason Reason
	}{
		{"manual uses configured", demandP, Inputs{Manual: true, Configured: 45 * sec, Active: true}, 45 * sec, ReasonManual},
		{"manual ignores floor", demandP, Inputs{Manual: true, Configured: 90 * sec, Floor: true}, 90 * sec, ReasonManual},
		{"manual ignores low power", demandP, Inputs{Manual: true, Configured: 90 * sec, LowPower: true}, 90 * sec, ReasonManual},
		{"active", demandP, Inputs{Active: true}, 15 * sec, ReasonActive},
		{"active not scaled by low power", demandP, Inputs{Active: true, LowPower: true}, 15 * sec, ReasonActive},
		{"idle", demandP, Inputs{}, 60 * sec, ReasonIdle},
		{"idle low power x4", demandP, Inputs{LowPower: true}, 240 * sec, ReasonIdle},
		{"event safety net", eventP, Inputs{}, 300 * sec, ReasonEvent},
		{"event safety net low power x4", eventP, Inputs{LowPower: true}, 1200 * sec, ReasonEvent},
		{"floor beats idle", demandP, Inputs{Floor: true}, 30 * sec, ReasonFloor},
		{"floor beats event", eventP, Inputs{Floor: true}, 30 * sec, ReasonFloor},
		{"floor unscaled in low power", eventP, Inputs{Floor: true, LowPower: true}, 30 * sec, ReasonFloor},
		{"active faster than floor", demandP, Inputs{Active: true, Floor: true}, 15 * sec, ReasonActive},
		{"slower floor ignored", slowFloor, Inputs{Floor: true}, 60 * sec, ReasonIdle},
		{"zero floor ignored", Policy{Active: sec, Idle: 2 * sec}, Inputs{Floor: true}, 2 * sec, ReasonIdle},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := Decide(tt.p, tt.in)
			if d.Interval != tt.want || d.Reason != tt.reason {
				t.Fatalf("Decide = %v/%s, want %v/%s", d.Interval, d.Reason, tt.want, tt.reason)
			}
			if !d.Next.IsZero() {
				t.Fatal("never-run collector should run now (zero Next)")
			}
			tt.in.LastRun = last
			if d := Decide(tt.p, tt.in); !d.Next.Equal(last.Add(tt.want)) {
				t.Fatalf("Next = %v, want %v", d.Next, last.Add(tt.want))
			}
		})
	}
}

func TestBaselineReason(t *testing.T) {
	if got := BaselineReason(Policy{EventDriven: true}, true); got != ReasonManual {
		t.Errorf("manual = %s", got)
	}
	if got := BaselineReason(Policy{EventDriven: true}, false); got != ReasonEvent {
		t.Errorf("event = %s", got)
	}
	if got := BaselineReason(Policy{}, false); got != ReasonIdle {
		t.Errorf("idle = %s", got)
	}
}

// recorder collects onChange calls.
type recorder struct {
	d     *Demand
	calls [][]string
}

func newRecorder(c Clock) *recorder {
	r := &recorder{d: NewDemand(c)}
	r.d.SetOnChange(func(names []string) {
		// Called without the lock held: Active must not deadlock.
		for _, n := range names {
			_ = r.d.Active(n)
		}
		sorted := slices.Clone(names)
		sort.Strings(sorted)
		r.calls = append(r.calls, sorted)
	})
	return r
}

func (r *recorder) last() []string {
	if len(r.calls) == 0 {
		return nil
	}
	return r.calls[len(r.calls)-1]
}

func TestDemandWebSocket(t *testing.T) {
	r := newRecorder(NewFakeClock(epoch))
	d := r.d
	docker := map[string]bool{"container_list_update": true, "not_a_topic": true}

	d.WSAdd(docker)
	if !d.Active("docker") || d.Active("system") || d.Active("vm") {
		t.Fatal("a docker-only subscriber should activate docker only")
	}
	if !slices.Equal(r.last(), []string{"docker"}) {
		t.Fatalf("onChange = %v, want [docker]", r.last())
	}

	// A second docker client: no flip, no notification.
	n := len(r.calls)
	d.WSAdd(map[string]bool{"container_list_update": true})
	d.WSRemove(docker)
	if len(r.calls) != n || !d.Active("docker") {
		t.Fatal("docker should stay active while one subscriber remains")
	}
	d.WSRemove(map[string]bool{"container_list_update": true})
	if d.Active("docker") || !slices.Equal(r.last(), []string{"docker"}) {
		t.Fatal("docker should go idle with no subscribers")
	}

	// A secondary topic (vm guest) activates its collector too.
	d.WSAdd(map[string]bool{"vm_guest_update": true})
	if !d.Active("vm") {
		t.Fatal("vm_guest_update subscriber should activate vm")
	}
	d.WSRemove(map[string]bool{"vm_guest_update": true})

	// A nil filter receives every topic.
	d.WSAdd(nil)
	for _, name := range CollectorNames() {
		if !d.Active(name) {
			t.Fatalf("nil filter should activate %s", name)
		}
	}
	if len(r.last()) != len(CollectorNames()) {
		t.Fatalf("nil filter flipped %d collectors, want all %d", len(r.last()), len(CollectorNames()))
	}
	d.WSRemove(nil)
	if d.Active("system") {
		t.Fatal("removing the nil-filter client should deactivate everything")
	}
}

func TestDemandMQTT(t *testing.T) {
	r := newRecorder(NewFakeClock(epoch))
	d := r.d
	d.SetMQTT(true, map[string]bool{"system": true, "ups": true})
	if !d.Active("system") || !d.Active("ups") || d.Active("docker") {
		t.Fatal("MQTT should activate only bound collectors")
	}
	if !slices.Equal(r.last(), []string{"system", "ups"}) {
		t.Fatalf("onChange = %v", r.last())
	}
	d.SetMQTT(true, map[string]bool{"system": true, "docker": true})
	if d.Active("ups") || !d.Active("docker") || !d.Active("system") {
		t.Fatal("replacing the bound set should move activation")
	}
	if !slices.Equal(r.last(), []string{"docker", "ups"}) {
		t.Fatalf("onChange = %v, want [docker ups]", r.last())
	}
	d.SetMQTT(false, map[string]bool{"system": true, "docker": true})
	if d.Active("system") || d.Active("docker") {
		t.Fatal("disconnected MQTT activates nothing")
	}
}

func TestDemandReadWindow(t *testing.T) {
	clock := NewFakeClock(epoch)
	r := newRecorder(clock)
	d := r.d
	d.RecordRead("disk", "array")
	if !d.Active("disk") || !d.Active("array") || d.Active("system") {
		t.Fatal("reads should activate the read collectors only")
	}
	if !slices.Equal(r.last(), []string{"array", "disk"}) {
		t.Fatalf("onChange = %v", r.last())
	}
	n := len(r.calls)
	clock.Advance(ReadWindow - sec)
	d.RecordRead("disk") // still active: no flip
	if len(r.calls) != n {
		t.Fatal("re-reading an active collector should not notify")
	}
	clock.Advance(2 * sec)
	if !d.Active("disk") || d.Active("array") {
		t.Fatal("array's read should have expired, disk's not")
	}
	clock.Advance(ReadWindow)
	if d.Active("disk") {
		t.Fatal("disk's read should have expired")
	}
}

func TestDemandNoListener(t *testing.T) {
	d := NewDemand(NewFakeClock(epoch))
	d.RecordRead("system") // must not panic without a listener
	if !d.Active("system") {
		t.Fatal("system should be active")
	}
}
