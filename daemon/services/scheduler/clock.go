// Package scheduler decides when each collector runs in Dynamic collection
// mode (#280): from change events, active consumers and alert-derived floors.
package scheduler

import (
	"sync"
	"time"
)

// Clock is the time source of the Dynamic scheduler, replaced by FakeClock in tests.
type Clock interface {
	Now() time.Time
	NewTimer(d time.Duration) Timer
}

// Timer is the part of *time.Timer the scheduler uses.
type Timer interface {
	C() <-chan time.Time
	Stop() bool
	Reset(d time.Duration) bool
}

// RealClock is the wall clock.
type RealClock struct{}

// Now returns the current time.
func (RealClock) Now() time.Time { return time.Now() }

// NewTimer returns a timer that fires after d.
func (RealClock) NewTimer(d time.Duration) Timer { return realTimer{time.NewTimer(d)} }

type realTimer struct{ t *time.Timer }

// C, Stop and Reset delegate to the wrapped *time.Timer.
func (r realTimer) C() <-chan time.Time        { return r.t.C }
func (r realTimer) Stop() bool                 { return r.t.Stop() }
func (r realTimer) Reset(d time.Duration) bool { return r.t.Reset(d) }

// FakeClock is a manually advanced Clock for tests. Timers fire when Advance
// moves the time past their deadline.
type FakeClock struct {
	mu     sync.Mutex
	now    time.Time
	timers []*fakeTimer
	// changed is closed and replaced whenever a timer is created or reset, so
	// tests can wait until the code under test has armed its next timer.
	changed chan struct{}
}

// NewFakeClock returns a FakeClock set to now.
func NewFakeClock(now time.Time) *FakeClock {
	return &FakeClock{now: now, changed: make(chan struct{})}
}

// Now returns the fake time.
func (c *FakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// NewTimer returns a timer that fires once the fake time reaches now+d.
func (c *FakeClock) NewTimer(d time.Duration) Timer {
	c.mu.Lock()
	defer c.mu.Unlock()
	t := &fakeTimer{clock: c, ch: make(chan time.Time, 1)}
	t.armLocked(d)
	c.timers = append(c.timers, t)
	return t
}

// Advance moves the fake time forward by d and fires every timer that is due.
func (c *FakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	now := c.now
	for _, t := range c.timers {
		if t.active && !t.deadline.After(now) {
			t.active = false
			select {
			case t.ch <- now:
			default:
			}
		}
	}
	c.mu.Unlock()
}

// Changed returns a channel that is closed the next time a timer is created
// or reset.
func (c *FakeClock) Changed() <-chan struct{} {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.changed
}

// Pending returns the durations from now until each active timer fires.
func (c *FakeClock) Pending() []time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []time.Duration
	for _, t := range c.timers {
		if t.active {
			out = append(out, t.deadline.Sub(c.now))
		}
	}
	return out
}

type fakeTimer struct {
	clock    *FakeClock
	ch       chan time.Time
	deadline time.Time
	active   bool
}

// armLocked sets the deadline; the clock's mutex must be held.
func (t *fakeTimer) armLocked(d time.Duration) {
	// Like time.Timer since Go 1.23, a reset discards an unreceived fire.
	select {
	case <-t.ch:
	default:
	}
	t.deadline = t.clock.now.Add(d)
	t.active = true
	close(t.clock.changed)
	t.clock.changed = make(chan struct{})
	if d <= 0 {
		t.active = false
		select {
		case t.ch <- t.clock.now:
		default:
		}
	}
}

// C returns the channel the timer fires on.
func (t *fakeTimer) C() <-chan time.Time { return t.ch }

// Stop disarms the timer and reports whether it was armed.
func (t *fakeTimer) Stop() bool {
	t.clock.mu.Lock()
	defer t.clock.mu.Unlock()
	was := t.active
	t.active = false
	return was
}

// Reset re-arms the timer d from the fake now and reports whether it was armed.
func (t *fakeTimer) Reset(d time.Duration) bool {
	t.clock.mu.Lock()
	defer t.clock.mu.Unlock()
	was := t.active
	t.armLocked(d)
	return was
}
