package collectors

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/digitalocean/go-libvirt"
)

// libvirtEventsDebounce coalesces bursts such as a domain stop+undefine.
const libvirtEventsDebounce = 250 * time.Millisecond

// libvirtEventsConn is the part of a libvirt connection the event source uses.
type libvirtEventsConn interface {
	LifecycleEvents(ctx context.Context) (<-chan libvirt.DomainEventLifecycleMsg, error)
	Disconnected() <-chan struct{}
	Disconnect() error
}

// NewLibvirtEventSource returns an EventSource that reports domain lifecycle
// events (define, start, stop, ...). It holds its own libvirt connection.
func NewLibvirtEventSource() EventSource {
	return newLibvirtEventSource(func() (libvirtEventsConn, error) {
		uri, _ := url.Parse(string(libvirt.QEMUSystem))
		return libvirt.ConnectToURI(uri)
	})
}

// newLibvirtEventSource builds the libvirt source on an injectable connection factory.
func newLibvirtEventSource(connect func() (libvirtEventsConn, error)) *sessionSource {
	return newSessionSource("libvirt event source", libvirtEventsDebounce, func(ctx context.Context, connected, changed func()) error {
		l, err := connect()
		if err != nil {
			return fmt.Errorf("connecting to libvirt: %w", err)
		}
		defer func() { _ = l.Disconnect() }()

		streamCtx, cancel := context.WithCancel(ctx)
		defer cancel()
		events, err := l.LifecycleEvents(streamCtx)
		if err != nil {
			return fmt.Errorf("subscribing to lifecycle events: %w", err)
		}
		// The stream goroutine blocks on send, so drain the channel until it
		// closes after cancel; this runs before Disconnect.
		defer func() {
			cancel()
			for range events {
			}
		}()
		connected()

		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-l.Disconnected():
				return errors.New("connection closed")
			case _, ok := <-events:
				if !ok {
					return errors.New("lifecycle event stream closed")
				}
				changed()
			}
		}
	})
}
