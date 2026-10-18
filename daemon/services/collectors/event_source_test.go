package collectors

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

// eventWaitTimeout bounds every wait in the event source tests.
const eventWaitTimeout = 5 * time.Second

// triggerRecorder counts trigger calls and signals each one on ch.
type triggerRecorder struct {
	n  atomic.Int32
	ch chan struct{}
}

func newTriggerRecorder() *triggerRecorder {
	return &triggerRecorder{ch: make(chan struct{}, 64)}
}

func (r *triggerRecorder) trigger() {
	r.n.Add(1)
	select {
	case r.ch <- struct{}{}:
	default:
	}
}

func (r *triggerRecorder) wait(t *testing.T) {
	t.Helper()
	select {
	case <-r.ch:
	case <-time.After(eventWaitTimeout):
		t.Fatal("timed out waiting for trigger")
	}
}

// none fails if a trigger arrives within d.
func (r *triggerRecorder) none(t *testing.T, d time.Duration) {
	t.Helper()
	select {
	case <-r.ch:
		t.Fatal("unexpected trigger")
	case <-time.After(d):
	}
}

// runSource runs src until the returned stop is called; stop cancels the
// context and fails the test unless Run returns.
func runSource(t *testing.T, src EventSource, trigger func()) (stop func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		src.Run(ctx, trigger)
	}()
	stop = func() {
		cancel()
		select {
		case <-done:
		case <-time.After(eventWaitTimeout):
			t.Fatal("Run did not return after cancel")
		}
	}
	t.Cleanup(stop)
	return stop
}

// fastSource shortens a source's debounce and backoff for tests.
func fastSource(src EventSource, debounce time.Duration) *sessionSource {
	s := src.(*sessionSource)
	s.debounce = debounce
	s.minBackoff = time.Millisecond
	s.maxBackoff = 5 * time.Millisecond
	return s
}

func TestSessionSource_ReconnectsAndCatchesUp(t *testing.T) {
	var attempts atomic.Int32
	src := fastSource(newSessionSource("test", 0, func(ctx context.Context, connected, changed func()) error {
		switch attempts.Add(1) {
		case 1:
			return errors.New("unavailable")
		case 2:
			connected()
			changed()
			changed()
			changed()
			return errors.New("lost")
		default:
			connected()
			<-ctx.Done()
			return ctx.Err()
		}
	}), 20*time.Millisecond)

	rec := newTriggerRecorder()
	stop := runSource(t, src, rec.trigger)
	for range 3 { // catch-up, debounced change, catch-up
		rec.wait(t)
	}
	stop()

	if got := rec.n.Load(); got != 3 {
		t.Errorf("triggers = %d, want 3", got)
	}
	if got := attempts.Load(); got != 3 {
		t.Errorf("attempts = %d, want 3", got)
	}
}

func TestSessionSource_CancelDuringBackoff(t *testing.T) {
	called := make(chan struct{}, 1)
	src := newSessionSource("test", 0, func(context.Context, func(), func()) error {
		called <- struct{}{}
		return errors.New("unavailable")
	})
	src.minBackoff = time.Hour

	stop := runSource(t, src, func() { t.Error("unexpected trigger") })
	select {
	case <-called:
	case <-time.After(eventWaitTimeout):
		t.Fatal("session not called")
	}
	stop()
}

func TestDebouncer(t *testing.T) {
	t.Run("coalesces a burst", func(t *testing.T) {
		rec := newTriggerRecorder()
		d := &debouncer{delay: 50 * time.Millisecond, fire: rec.trigger}
		for range 5 {
			d.poke()
		}
		rec.wait(t)
		d.stop()
		if got := rec.n.Load(); got != 1 {
			t.Errorf("fires = %d, want 1", got)
		}
	})

	t.Run("fires again after the window", func(t *testing.T) {
		rec := newTriggerRecorder()
		d := &debouncer{delay: time.Millisecond, fire: rec.trigger}
		d.poke()
		rec.wait(t)
		d.poke()
		rec.wait(t)
		d.stop()
	})

	t.Run("stop cancels a pending fire", func(t *testing.T) {
		rec := newTriggerRecorder()
		d := &debouncer{delay: 10 * time.Millisecond, fire: rec.trigger}
		d.poke()
		d.stop()
		d.poke()
		rec.none(t, 100*time.Millisecond)
	})
}
