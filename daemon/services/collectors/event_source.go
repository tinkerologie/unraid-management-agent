package collectors

import (
	"context"
	"sync"
	"time"

	"github.com/tinkerologie/unraid-management-agent/daemon/logger"
)

// EventSource reports changes in a collector's data source so the collector
// can run right away instead of waiting for its next poll.
type EventSource interface {
	// Run blocks until ctx is cancelled. It calls trigger once per debounced
	// change, and once after every successful (re)connect so changes missed
	// while disconnected are caught up. trigger never blocks. Run owns and
	// closes everything it opens. When the source cannot be opened it logs,
	// retries with backoff and keeps going; it returns only when ctx is done.
	Run(ctx context.Context, trigger func())
}

// Reconnect backoff shared by all event sources.
const (
	eventSourceMinBackoff = time.Second
	eventSourceMaxBackoff = 60 * time.Second
)

// sessionFunc opens a source and reports changes until the connection ends.
// It calls connected once the source is open and changed for every relevant
// event, and returns why the session ended.
type sessionFunc func(ctx context.Context, connected, changed func()) error

// sessionSource is an EventSource that runs sessions in a loop, reconnecting
// with exponential backoff whenever a session ends.
type sessionSource struct {
	name       string
	debounce   time.Duration
	minBackoff time.Duration
	maxBackoff time.Duration
	session    sessionFunc
}

// newSessionSource returns a source that runs session with reconnect backoff and debounced triggers.
func newSessionSource(name string, debounce time.Duration, session sessionFunc) *sessionSource {
	return &sessionSource{
		name:       name,
		debounce:   debounce,
		minBackoff: eventSourceMinBackoff,
		maxBackoff: eventSourceMaxBackoff,
		session:    session,
	}
}

// Run implements EventSource.
func (s *sessionSource) Run(ctx context.Context, trigger func()) {
	d := &debouncer{delay: s.debounce, fire: trigger}
	defer d.stop()

	backoff := s.minBackoff
	for {
		connected := false
		err := s.session(ctx, func() {
			connected = true
			logger.Debug("%s: connected", s.name)
			trigger()
		}, d.poke)
		if ctx.Err() != nil {
			return
		}
		if connected {
			backoff = s.minBackoff
			logger.Warning("%s: connection lost: %v; reconnecting in %s", s.name, err, backoff)
		} else {
			logger.Debug("%s: unavailable: %v; retrying in %s", s.name, err, backoff)
		}

		t := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			t.Stop()
			return
		case <-t.C:
		}
		backoff = min(backoff*2, s.maxBackoff)
	}
}

// debouncer coalesces bursts of pokes into one fire per delay window: the
// first poke starts the window and later pokes inside it are absorbed.
type debouncer struct {
	mu      sync.Mutex
	delay   time.Duration
	fire    func()
	timer   *time.Timer
	pending bool
	stopped bool
}

// poke records a change; the trigger fires once at the end of the window.
func (d *debouncer) poke() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.pending || d.stopped {
		return
	}
	d.pending = true
	d.timer = time.AfterFunc(d.delay, func() {
		// fire runs under the lock so it never runs after stop returns.
		d.mu.Lock()
		defer d.mu.Unlock()
		d.pending = false
		if !d.stopped {
			d.fire()
		}
	})
}

// stop cancels any pending fire. No fire happens after stop returns.
func (d *debouncer) stop() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.stopped = true
	if d.timer != nil {
		d.timer.Stop()
	}
}
