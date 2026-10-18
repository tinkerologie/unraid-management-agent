package collectors

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/digitalocean/go-libvirt"
)

// fakeLibvirtConn mimics go-libvirt's lifecycle stream: the stream goroutine
// blocks on send and closes its channel when the context is cancelled or the
// stream ends.
type fakeLibvirtConn struct {
	subErr       error
	events       chan libvirt.DomainEventLifecycleMsg
	endStream    chan struct{}
	disconnected chan struct{}

	disconnectCalled atomic.Bool
	stopped          chan struct{} // closed when the stream goroutine exits
}

func newFakeLibvirtConn(subErr error) *fakeLibvirtConn {
	return &fakeLibvirtConn{
		subErr:       subErr,
		events:       make(chan libvirt.DomainEventLifecycleMsg),
		endStream:    make(chan struct{}),
		disconnected: make(chan struct{}),
		stopped:      make(chan struct{}),
	}
}

func (f *fakeLibvirtConn) LifecycleEvents(ctx context.Context) (<-chan libvirt.DomainEventLifecycleMsg, error) {
	if f.subErr != nil {
		return nil, f.subErr
	}
	out := make(chan libvirt.DomainEventLifecycleMsg)
	go func() {
		defer close(f.stopped)
		defer close(out)
		for {
			select {
			case ev := <-f.events:
				out <- ev // like go-libvirt, the send ignores ctx
			case <-f.endStream:
				return
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}

func (f *fakeLibvirtConn) Disconnected() <-chan struct{} { return f.disconnected }

func (f *fakeLibvirtConn) Disconnect() error {
	f.disconnectCalled.Store(true)
	return nil
}

func TestLibvirtEventSource(t *testing.T) {
	subFails := newFakeLibvirtConn(errors.New("no permission"))
	first := newFakeLibvirtConn(nil)
	second := newFakeLibvirtConn(nil)
	third := newFakeLibvirtConn(nil)
	src := fastSource(newLibvirtEventSource(fakeFactory[libvirtEventsConn](subFails, first, second, third)), 20*time.Millisecond)

	rec := newTriggerRecorder()
	stop := runSource(t, src, rec.trigger)

	rec.wait(t) // catch-up after connect
	first.events <- libvirt.DomainEventLifecycleMsg{Event: 2}
	first.events <- libvirt.DomainEventLifecycleMsg{Event: 5}
	rec.wait(t) // one debounced trigger for both

	close(first.disconnected)
	waitClosed(t, first.stopped, "first stream")
	rec.wait(t) // catch-up after reconnect

	close(second.endStream)
	waitClosed(t, second.stopped, "second stream")
	rec.wait(t) // catch-up after reconnect

	// An event in flight when Run is cancelled must not leak the stream goroutine.
	go func() {
		select {
		case third.events <- libvirt.DomainEventLifecycleMsg{}:
		case <-third.stopped:
		}
	}()
	stop()
	waitClosed(t, third.stopped, "third stream")

	if got := rec.n.Load(); got < 4 {
		t.Errorf("triggers = %d, want at least 4", got)
	}
	for name, c := range map[string]*fakeLibvirtConn{"subFails": subFails, "first": first, "second": second, "third": third} {
		if !c.disconnectCalled.Load() {
			t.Errorf("%s connection not disconnected", name)
		}
	}
}

func TestLibvirtEventSource_ConnectFails(t *testing.T) {
	src := newLibvirtEventSource(fakeFactory[libvirtEventsConn]())
	err := src.session(t.Context(), func() { t.Error("unexpected connect") }, func() {})
	if err == nil || err.Error() != "connecting to libvirt: no more fakes" {
		t.Errorf("err = %v, want connect error", err)
	}
}

func TestNewLibvirtEventSource_ReturnsOnCancel(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	NewLibvirtEventSource().Run(ctx, func() {})
}
