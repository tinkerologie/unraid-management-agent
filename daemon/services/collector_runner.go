package services

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/tinkerologie/unraid-management-agent/daemon/logger"
	"github.com/tinkerologie/unraid-management-agent/daemon/services/collectors"
	"github.com/tinkerologie/unraid-management-agent/daemon/services/scheduler"
)

// Collector owns the resources for one generation. Open and Refresh share its
// cancellation context. Calls are serialized and Close runs once after Open
// is attempted, after Refresh and the event sources have returned.
type Collector interface {
	Open(ctx context.Context) error
	Refresh(ctx context.Context) error
	Close()
}

// eventSource is collectors.EventSource: it calls trigger on source changes
// until ctx is cancelled.
type eventSource interface {
	Run(ctx context.Context, trigger func())
}

// errRunnerStopped is the outcome of a refresh request the runner never served.
var errRunnerStopped = errors.New("collector stopped")

// collectFunc adapts a Collect method that needs no setup or teardown.
type collectFunc func(ctx context.Context)

// Open does nothing: the adapted collector needs no setup.
func (f collectFunc) Open(context.Context) error { return nil }

// Refresh runs the adapted Collect once.
func (f collectFunc) Refresh(ctx context.Context) error {
	f(ctx)
	return nil
}

// Close does nothing: the adapted collector needs no teardown.
func (f collectFunc) Close() {}

// collectOnce adapts a stateless collection method that has no context parameter.
func collectOnce(collect func()) collectFunc {
	return func(context.Context) { collect() }
}

// collectorRunner serializes one collector generation's scheduled runs
// and event triggers. In Dynamic mode concurrent requests share one run.
type collectorRunner struct {
	name     string
	policy   scheduler.Policy
	coll     Collector
	clock    scheduler.Clock
	lowPower bool
	active   func() bool
	floor    func() bool
	sources  []eventSource

	trigger chan struct{} // change events
	refresh chan struct{} // read requests
	wake    chan struct{} // demand or floor changed

	mu          sync.Mutex
	waiters     []chan error
	stopped     bool
	pending     bool // an event or request is waiting for its run
	started     time.Time
	lastAttempt time.Time
	lastSuccess time.Time
	lastErr     error
	runs        uint64
	errorCount  int
	decision    scheduler.Decision
}

// runnerState is a snapshot of a runner for status reporting.
type runnerState struct {
	decision    scheduler.Decision
	lastAttempt time.Time
	lastSuccess time.Time
	lastErr     error
	runs        uint64
	errorCount  int
}

// newCollectorRunner returns a runner for one collector, inactive and with no floor until set.
func newCollectorRunner(name string, policy scheduler.Policy, coll Collector, clock scheduler.Clock) *collectorRunner {
	return &collectorRunner{
		name:    name,
		policy:  policy,
		coll:    coll,
		clock:   clock,
		active:  func() bool { return false },
		floor:   func() bool { return false },
		trigger: make(chan struct{}, 1),
		refresh: make(chan struct{}, 1),
		wake:    make(chan struct{}, 1),
	}
}

// notify sends on a buffered channel without blocking: one pending signal is enough.
func notify(ch chan struct{}) {
	select {
	case ch <- struct{}{}:
	default:
	}
}

// Trigger asks for a run because the source changed. Safe after stop.
func (r *collectorRunner) Trigger() { notify(r.trigger) }

// Wake asks the runner to re-decide its cadence. Safe after stop.
func (r *collectorRunner) Wake() { notify(r.wake) }

// RequestRefresh asks for a run and returns a channel that receives its
// outcome. Requests made while a run is in progress share that run.
func (r *collectorRunner) RequestRefresh() <-chan error {
	ch := make(chan error, 1)
	r.mu.Lock()
	if r.stopped {
		r.mu.Unlock()
		ch <- errRunnerStopped
		return ch
	}
	r.waiters = append(r.waiters, ch)
	r.mu.Unlock()
	notify(r.refresh)
	return ch
}

// snapshot returns the runner's state.
func (r *collectorRunner) snapshot() runnerState {
	r.mu.Lock()
	defer r.mu.Unlock()
	return runnerState{
		decision:    r.decision,
		lastAttempt: r.lastAttempt,
		lastSuccess: r.lastSuccess,
		lastErr:     r.lastErr,
		runs:        r.runs,
		errorCount:  r.errorCount,
	}
}

// run serves the demand scheduler over the shared collector lifetime.
func (r *collectorRunner) run(ctx context.Context) {
	r.runLifetime(ctx, r.runDynamicSchedule)
}

// runLifetime joins all sources before releasing collector-owned resources.
func (r *collectorRunner) runLifetime(parent context.Context, schedule func(context.Context, func())) {
	ctx, cancel := context.WithCancel(parent)
	var sources sync.WaitGroup
	defer r.recoverPanic("lifetime")
	if ctx.Err() != nil {
		cancel()
		r.stop()
		return
	}
	defer r.coll.Close()
	defer func() {
		cancel()
		sources.Wait()
	}()
	defer r.stop()
	if err := r.coll.Open(ctx); err != nil {
		logger.Warning("Collector %s: open failed: %v", r.name, err)
		return
	}
	if ctx.Err() != nil {
		return
	}
	startSources := func() {
		for _, source := range r.sources {
			sources.Go(func() {
				defer r.recoverPanic("event source")
				source.Run(ctx, r.Trigger)
			})
		}
	}
	schedule(ctx, startSources)
}

func (r *collectorRunner) recoverPanic(stage string) {
	if rec := recover(); rec != nil {
		logger.LogPanicWithStack(fmt.Sprintf("Collector %s (%s)", r.name, stage), rec)
	}
}

func (r *collectorRunner) runDynamicSchedule(ctx context.Context, startSources func()) {
	startSources()
	r.started = r.clock.Now()
	r.updateDecision()

	timer := r.clock.NewTimer(r.policy.InitialDelay)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C():
			if ctx.Err() != nil {
				return
			}
			r.collect(ctx)
		case <-r.trigger:
			r.setPending(false)
		case <-r.refresh:
			r.setPending(true)
		case <-r.wake:
		}
		timer.Reset(r.updateDecision())
	}
}

// setPending records a waiting change event or refresh request. Change
// events before the first run are dropped: the first run reads the source
// anyway, and InitialDelay keeps its startup stagger.
func (r *collectorRunner) setPending(request bool) {
	r.mu.Lock()
	if request || !r.lastAttempt.IsZero() {
		r.pending = true
	}
	r.mu.Unlock()
}

// updateDecision recomputes cadence without holding the lock around callbacks.
func (r *collectorRunner) updateDecision() time.Duration {
	active, floor := r.active(), r.floor()
	r.mu.Lock()
	defer r.mu.Unlock()
	r.decision = scheduler.Decide(r.policy, scheduler.Inputs{
		Active:   active,
		Floor:    floor,
		LowPower: r.lowPower,
		LastRun:  r.lastAttempt,
	})
	return r.delayLocked()
}

// delayLocked returns the time until the next run: the decided next run, or
// sooner for a pending event or request, but never within MinGap of the last run.
func (r *collectorRunner) delayLocked() time.Duration {
	next := r.decision.Next
	if r.lastAttempt.IsZero() {
		next = r.started.Add(r.policy.InitialDelay)
	}
	if r.pending {
		if gap := r.lastAttempt.Add(r.policy.MinGap); gap.Before(next) {
			next = gap
		}
	}
	return max(next.Sub(r.clock.Now()), 0)
}

// collect runs Refresh once and hands its outcome to every waiting request,
// including the ones that arrived while it ran.
func (r *collectorRunner) collect(ctx context.Context) {
	r.mu.Lock()
	r.pending = false
	r.lastAttempt = r.clock.Now()
	interval := r.decision.Interval
	r.mu.Unlock()

	err := r.safeRefresh(ctx, interval)

	r.mu.Lock()
	r.runs++
	r.lastErr = err
	if err != nil {
		r.errorCount++
	} else {
		r.lastSuccess = r.clock.Now()
	}
	// Requests that arrived during the run are served by it. Drain their
	// signal under the lock, so a request appended after the snapshot keeps
	// its own signal and gets the next run.
	select {
	case <-r.refresh:
	default:
	}
	waiters := r.waiters
	r.waiters = nil
	r.mu.Unlock()
	for _, w := range waiters {
		w <- err
	}
}

// safeRefresh calls Refresh under the stall watchdog, turning a panic into an error.
func (r *collectorRunner) safeRefresh(ctx context.Context, interval time.Duration) (err error) {
	defer func() {
		if rec := recover(); rec != nil {
			logger.LogPanicWithStack(fmt.Sprintf("Collector %s", r.name), rec)
			err = fmt.Errorf("collector %s panicked: %v", r.name, rec)
		}
	}()
	collectors.CollectWithWatchdog(ctx, r.name, interval, func() { err = r.coll.Refresh(ctx) })
	return err
}

// stop fails every waiting request and refuses new ones.
func (r *collectorRunner) stop() {
	r.mu.Lock()
	r.stopped = true
	waiters := r.waiters
	r.waiters = nil
	r.mu.Unlock()
	for _, w := range waiters {
		w <- errRunnerStopped
	}
}
