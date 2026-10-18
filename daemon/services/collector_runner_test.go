package services

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tinkerologie/unraid-management-agent/daemon/services/scheduler"
)

const testWait = 5 * time.Second

var runnerEpoch = time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)

// fakeDynamic is a DynamicCollector whose Refresh calls are observable and,
// with block set, held until a value is sent on release.
type fakeDynamic struct {
	openErr    error
	refreshErr error
	panicMsg   string
	block      bool

	calls   chan struct{}
	release chan error

	inflight, maxInflight atomic.Int32
	opened, closed        atomic.Bool

	orderMu *sync.Mutex
	order   *[]string
}

func newFakeDynamic() *fakeDynamic {
	return &fakeDynamic{calls: make(chan struct{}, 64), release: make(chan error)}
}

func (f *fakeDynamic) Open(context.Context) error {
	f.opened.Store(true)
	return f.openErr
}

func (f *fakeDynamic) Refresh(context.Context) error {
	n := f.inflight.Add(1)
	defer f.inflight.Add(-1)
	for {
		m := f.maxInflight.Load()
		if n <= m || f.maxInflight.CompareAndSwap(m, n) {
			break
		}
	}
	f.calls <- struct{}{}
	if f.panicMsg != "" {
		panic(f.panicMsg)
	}
	if f.block {
		return <-f.release
	}
	return f.refreshErr
}

func (f *fakeDynamic) Close() {
	f.closed.Store(true)
	if f.order != nil {
		f.orderMu.Lock()
		*f.order = append(*f.order, "close")
		f.orderMu.Unlock()
	}
}

func (f *fakeDynamic) waitCall(t *testing.T) {
	t.Helper()
	select {
	case <-f.calls:
	case <-time.After(testWait):
		t.Fatal("timed out waiting for Refresh")
	}
}

func (f *fakeDynamic) noCall(t *testing.T) {
	t.Helper()
	select {
	case <-f.calls:
		t.Fatal("unexpected Refresh")
	default:
	}
}

var testPolicy = scheduler.Policy{
	Active: 15 * time.Second, Idle: 60 * time.Second, Floor: 30 * time.Second,
	Staleness: 15 * time.Second, MinGap: 2 * time.Second, InitialDelay: time.Hour,
	RefreshOnRead: true, Topics: []string{"system_update"},
}

type runnerHarness struct {
	r      *collectorRunner
	coll   *fakeDynamic
	clock  *scheduler.FakeClock
	cancel context.CancelFunc
	done   chan struct{}
}

// startRunner starts a runner and waits until it armed its first timer.
func startRunner(t *testing.T, p scheduler.Policy, coll *fakeDynamic, setup func(*collectorRunner)) *runnerHarness {
	t.Helper()
	clock := scheduler.NewFakeClock(runnerEpoch)
	r := newCollectorRunner("test", p, coll, clock)
	if setup != nil {
		setup(r)
	}
	ctx, cancel := context.WithCancel(context.Background())
	h := &runnerHarness{r: r, coll: coll, clock: clock, cancel: cancel, done: make(chan struct{})}
	armed := clock.Changed()
	go func() {
		defer close(h.done)
		r.run(ctx)
	}()
	waitChan(t, armed, "first timer")
	t.Cleanup(h.stop)
	return h
}

func (h *runnerHarness) stop() {
	h.cancel()
	<-h.done
}

func waitChan[T any](t *testing.T, ch <-chan T, what string) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(testWait):
		t.Fatalf("timed out waiting for %s", what)
	}
	var zero T
	return zero
}

// act runs fn and waits until the runner re-armed its timer.
func (h *runnerHarness) act(t *testing.T, fn func()) {
	t.Helper()
	armed := h.clock.Changed()
	fn()
	waitChan(t, armed, "timer re-arm")
}

// eventually polls cond until it holds; it synchronises with the runner
// goroutine, not with cadence (that runs on the fake clock).
func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(testWait)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(time.Millisecond)
	}
}

// idleWith waits until the runner finished runs runs and armed a timer for want.
func (h *runnerHarness) idleWith(t *testing.T, runs uint64, want time.Duration) {
	t.Helper()
	eventually(t, "runner idle", func() bool {
		p := h.clock.Pending()
		return h.r.snapshot().runs == runs && len(p) == 1 && p[0] == want
	})
}

func (h *runnerHarness) pending(t *testing.T, want time.Duration) {
	t.Helper()
	got := h.clock.Pending()
	if len(got) != 1 || got[0] != want {
		t.Fatalf("pending timers = %v, want [%v]", got, want)
	}
}

func TestRunnerInitialDelayAndCadence(t *testing.T) {
	var active atomic.Bool
	h := startRunner(t, testPolicy, newFakeDynamic(), func(r *collectorRunner) {
		r.active = active.Load
	})
	if !h.coll.opened.Load() {
		t.Fatal("Open should run before the first timer")
	}
	h.pending(t, time.Hour)
	h.clock.Advance(time.Hour - time.Second)
	h.coll.noCall(t)

	h.act(t, func() { h.clock.Advance(time.Second) })
	h.coll.waitCall(t)
	h.pending(t, 60*time.Second) // idle
	if st := h.r.snapshot(); st.decision.Reason != scheduler.ReasonIdle || st.runs != 1 || st.lastSuccess.IsZero() {
		t.Fatalf("snapshot = %+v", st)
	}

	// Demand turns on: Wake re-decides to the active interval.
	active.Store(true)
	h.act(t, h.r.Wake)
	h.pending(t, 15*time.Second)
	if st := h.r.snapshot(); st.decision.Reason != scheduler.ReasonActive {
		t.Fatalf("reason = %s, want active", st.decision.Reason)
	}
	h.act(t, func() { h.clock.Advance(15 * time.Second) })
	h.coll.waitCall(t)
	h.pending(t, 15*time.Second)
}

func TestRunnerFloorAndLowPower(t *testing.T) {
	var floor atomic.Bool
	h := startRunner(t, testPolicy, newFakeDynamic(), func(r *collectorRunner) {
		r.lowPower = true
		r.floor = floor.Load
	})
	h.act(t, func() { h.clock.Advance(time.Hour) })
	h.coll.waitCall(t)
	h.pending(t, 240*time.Second) // idle x4
	floor.Store(true)
	h.act(t, h.r.Wake)
	h.pending(t, 30*time.Second)
}

func TestRunnerTriggerRespectsMinGap(t *testing.T) {
	p := testPolicy
	p.InitialDelay = 0
	h := startRunner(t, p, newFakeDynamic(), nil)
	h.coll.waitCall(t) // zero initial delay runs at once
	h.idleWith(t, 1, 60*time.Second)

	// Right after a run a trigger waits out MinGap.
	h.act(t, h.r.Trigger)
	h.pending(t, 2*time.Second)
	h.coll.noCall(t)
	h.act(t, func() { h.clock.Advance(2 * time.Second) })
	h.coll.waitCall(t)
	h.pending(t, 60*time.Second)

	// Long after a run a trigger runs at once.
	h.clock.Advance(30 * time.Second)
	h.act(t, h.r.Trigger)
	h.coll.waitCall(t)
	if got := h.coll.maxInflight.Load(); got != 1 {
		t.Fatalf("runs overlapped: max in flight %d", got)
	}
}

func TestRunnerSharedRefresh(t *testing.T) {
	coll := newFakeDynamic()
	coll.block = true
	h := startRunner(t, testPolicy, coll, nil)

	a := h.r.RequestRefresh()
	b := h.r.RequestRefresh()
	coll.waitCall(t)
	c := h.r.RequestRefresh() // arrives during the run: served by it
	want := errors.New("boom")
	h.act(t, func() { coll.release <- want })

	for i, ch := range []<-chan error{a, b, c} {
		if got := waitChan(t, ch, "refresh outcome"); !errors.Is(got, want) {
			t.Fatalf("waiter %d got %v, want %v", i, got, want)
		}
	}
	coll.noCall(t)
	h.pending(t, 60*time.Second) // the request during the run did not queue another run
	st := h.r.snapshot()
	if st.runs != 1 || st.errorCount != 1 || !errors.Is(st.lastErr, want) || !st.lastSuccess.IsZero() {
		t.Fatalf("snapshot = %+v", st)
	}
	if got := coll.maxInflight.Load(); got != 1 {
		t.Fatalf("runs overlapped: max in flight %d", got)
	}
}

func TestRunnerRefreshErrorAndSuccess(t *testing.T) {
	coll := newFakeDynamic()
	coll.refreshErr = errors.New("source down")
	h := startRunner(t, testPolicy, coll, nil)
	if got := waitChan(t, h.r.RequestRefresh(), "outcome"); got == nil || got.Error() != "source down" {
		t.Fatalf("got %v", got)
	}
	coll.waitCall(t)
}

func TestRunnerPanicRecovered(t *testing.T) {
	coll := newFakeDynamic()
	coll.panicMsg = "kaboom"
	h := startRunner(t, testPolicy, coll, nil)
	got := waitChan(t, h.r.RequestRefresh(), "outcome")
	if got == nil || !strings.Contains(got.Error(), "panicked") || !strings.Contains(got.Error(), "kaboom") {
		t.Fatalf("got %v, want a panic error", got)
	}
	if st := h.r.snapshot(); st.errorCount != 1 {
		t.Fatalf("errorCount = %d, want 1", st.errorCount)
	}
}

// fakeSource triggers once when started and records its exit.
type fakeSource struct {
	orderMu *sync.Mutex
	order   *[]string
	fire    chan struct{} // trigger when it receives; nil triggers at once
}

func (s fakeSource) Run(ctx context.Context, trigger func()) {
	if s.fire != nil {
		select {
		case <-s.fire:
		case <-ctx.Done():
		}
	}
	trigger()
	<-ctx.Done()
	s.orderMu.Lock()
	*s.order = append(*s.order, "source")
	s.orderMu.Unlock()
}

// panicSource panics: the runner recovers it.
type panicSource struct{}

func (panicSource) Run(context.Context, func()) { panic("source panic") }

func TestRunnerEventSourcesAndStopOrder(t *testing.T) {
	var mu sync.Mutex
	var order []string
	coll := newFakeDynamic()
	coll.orderMu, coll.order = &mu, &order
	coll.openErr = errors.New("open failed")

	fire := make(chan struct{})
	h := startRunner(t, testPolicy, coll, func(r *collectorRunner) {
		r.sources = []eventSource{fakeSource{orderMu: &mu, order: &order, fire: fire}, panicSource{}}
	})
	// A change event before the first run keeps the initial delay.
	rearmed := h.clock.Changed()
	fire <- struct{}{}
	waitChan(t, rearmed, "timer re-armed after the event")
	if got := h.clock.Pending(); len(got) != 1 || got[0] != time.Hour {
		t.Fatalf("pending timers after an early event = %v, want [1h]", got)
	}
	// The scheduled first run happens despite the Open error.
	h.clock.Advance(time.Hour)
	coll.waitCall(t)
	h.idleWith(t, 1, 60*time.Second)

	// A request inside MinGap stays pending; stop fails it.
	pending := h.r.RequestRefresh()
	h.stop()
	if got := waitChan(t, pending, "stopped outcome"); !errors.Is(got, errRunnerStopped) {
		t.Fatalf("pending waiter got %v, want errRunnerStopped", got)
	}
	if got := waitChan(t, h.r.RequestRefresh(), "after-stop outcome"); !errors.Is(got, errRunnerStopped) {
		t.Fatalf("request after stop got %v", got)
	}
	mu.Lock()
	defer mu.Unlock()
	if !slices.Equal(order, []string{"source", "close"}) {
		t.Fatalf("stop order = %v, want sources joined before Close", order)
	}
	// Late signals after stop are harmless.
	h.r.Trigger()
	h.r.Wake()
}

// Collector types for dynamicCollectorFor.
type plainCollect struct{ n *atomic.Int32 }

func (plainCollect) Start(context.Context, time.Duration) {}
func (p plainCollect) Collect()                           { p.n.Add(1) }

type ctxCollect struct{ n *atomic.Int32 }

func (ctxCollect) Start(context.Context, time.Duration) {}
func (p ctxCollect) Collect(context.Context)            { p.n.Add(1) }

type dynCollect struct{ *fakeDynamic }

func (dynCollect) Start(context.Context, time.Duration) {}

type startOnly struct{}

func (startOnly) Start(context.Context, time.Duration) {}

func TestDynamicCollectorFor(t *testing.T) {
	var n atomic.Int32
	for _, c := range []Collector{plainCollect{&n}, ctxCollect{&n}} {
		d, err := dynamicCollectorFor(c)
		if err != nil {
			t.Fatalf("%T: %v", c, err)
		}
		if err := d.Open(context.Background()); err != nil {
			t.Fatal(err)
		}
		if err := d.Refresh(context.Background()); err != nil {
			t.Fatal(err)
		}
		d.Close()
	}
	if n.Load() != 2 {
		t.Fatalf("Collect called %d times, want 2", n.Load())
	}
	f := dynCollect{newFakeDynamic()}
	if d, err := dynamicCollectorFor(f); err != nil || d != DynamicCollector(f) {
		t.Fatalf("a DynamicCollector should be used as is: %v", err)
	}
	if _, err := dynamicCollectorFor(startOnly{}); err == nil {
		t.Fatal("a Start-only collector has no Dynamic contract")
	}
}
