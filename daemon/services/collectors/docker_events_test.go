package collectors

import (
	"context"
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moby/moby/api/types/events"
	"github.com/moby/moby/client"
)

// fakeDockerClient streams events written to msgs and errs, ending its stream
// goroutine when the Events context is cancelled, like the real client.
type fakeDockerClient struct {
	pingErr error
	msgs    chan events.Message
	errs    chan error

	opts    client.EventsListOptions
	closed  atomic.Bool
	stopped chan struct{} // closed when the stream goroutine exits
}

func newFakeDockerClient(pingErr error) *fakeDockerClient {
	return &fakeDockerClient{
		pingErr: pingErr,
		msgs:    make(chan events.Message),
		errs:    make(chan error),
		stopped: make(chan struct{}),
	}
}

func (f *fakeDockerClient) Ping(context.Context, client.PingOptions) (client.PingResult, error) {
	return client.PingResult{}, f.pingErr
}

func (f *fakeDockerClient) Events(ctx context.Context, opts client.EventsListOptions) client.EventsResult {
	f.opts = opts
	msgs := make(chan events.Message)
	errs := make(chan error, 1)
	go func() {
		defer close(f.stopped)
		defer close(errs)
		for {
			select {
			case m := <-f.msgs:
				select {
				case msgs <- m:
				case <-ctx.Done():
					return
				}
			case err := <-f.errs:
				errs <- err
				return
			case <-ctx.Done():
				errs <- ctx.Err()
				return
			}
		}
	}()
	return client.EventsResult{Messages: msgs, Err: errs}
}

func (f *fakeDockerClient) Close() error {
	f.closed.Store(true)
	return nil
}

// fakeFactory hands out the given values in order, then fails.
func fakeFactory[T any](values ...T) func() (T, error) {
	var mu sync.Mutex
	return func() (T, error) {
		mu.Lock()
		defer mu.Unlock()
		var zero T
		if len(values) == 0 {
			return zero, errors.New("no more fakes")
		}
		v := values[0]
		values = values[1:]
		return v, nil
	}
}

func waitClosed(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(eventWaitTimeout):
		t.Fatalf("%s did not exit", what)
	}
}

func TestDockerEventSource(t *testing.T) {
	unreachable := newFakeDockerClient(errors.New("connection refused"))
	first := newFakeDockerClient(nil)
	second := newFakeDockerClient(nil)
	src := fastSource(newDockerEventSource(fakeFactory[dockerEventsClient](unreachable, first, second)), 20*time.Millisecond)

	rec := newTriggerRecorder()
	stop := runSource(t, src, rec.trigger)

	rec.wait(t) // catch-up after the first connect
	for _, a := range []events.Action{"health_status: healthy", events.ActionStop, events.ActionDie} {
		first.msgs <- events.Message{Type: events.ContainerEventType, Action: a}
	}
	rec.wait(t) // one debounced trigger for the burst

	first.errs <- io.EOF
	waitClosed(t, first.stopped, "first stream")
	rec.wait(t) // catch-up after reconnect

	stop()
	waitClosed(t, second.stopped, "second stream")

	if got := rec.n.Load(); got != 3 {
		t.Errorf("triggers = %d, want 3", got)
	}
	for name, c := range map[string]*fakeDockerClient{"unreachable": unreachable, "first": first, "second": second} {
		if !c.closed.Load() {
			t.Errorf("%s client not closed", name)
		}
	}
	f := first.opts.Filters
	if !f["type"]["container"] || !f["event"]["health_status"] || !f["event"]["destroy"] {
		t.Errorf("filters = %v, want type=container and the lifecycle actions", f)
	}
}

// closedStreamClient returns a stream whose error channel closes without an error.
type closedStreamClient struct{ *fakeDockerClient }

func (closedStreamClient) Events(context.Context, client.EventsListOptions) client.EventsResult {
	errs := make(chan error)
	close(errs)
	return client.EventsResult{Messages: make(chan events.Message), Err: errs}
}

func TestDockerEventSource_SessionErrors(t *testing.T) {
	tests := []struct {
		name          string
		clients       []dockerEventsClient
		wantConnected bool
		wantErr       string
	}{
		{"client creation fails", nil, false, "creating Docker client: no more fakes"},
		{"ping fails", []dockerEventsClient{newFakeDockerClient(errors.New("refused"))}, false, "pinging Docker: refused"},
		{"stream closes", []dockerEventsClient{closedStreamClient{newFakeDockerClient(nil)}}, true, "event stream closed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := newDockerEventSource(fakeFactory(tt.clients...))
			connected := false
			err := src.session(t.Context(), func() { connected = true }, func() {})
			if connected != tt.wantConnected {
				t.Errorf("connected = %v, want %v", connected, tt.wantConnected)
			}
			if err == nil || err.Error() != tt.wantErr {
				t.Errorf("err = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestIsDockerContainerChange(t *testing.T) {
	tests := []struct {
		name string
		msg  events.Message
		want bool
	}{
		{"start", events.Message{Type: events.ContainerEventType, Action: events.ActionStart}, true},
		{"stop", events.Message{Type: events.ContainerEventType, Action: events.ActionStop}, true},
		{"die", events.Message{Type: events.ContainerEventType, Action: events.ActionDie}, true},
		{"create", events.Message{Type: events.ContainerEventType, Action: events.ActionCreate}, true},
		{"destroy", events.Message{Type: events.ContainerEventType, Action: events.ActionDestroy}, true},
		{"health healthy", events.Message{Type: events.ContainerEventType, Action: events.ActionHealthStatusHealthy}, true},
		{"health unhealthy", events.Message{Type: events.ContainerEventType, Action: events.ActionHealthStatusUnhealthy}, true},
		{"exec start", events.Message{Type: events.ContainerEventType, Action: "exec_start: sh -c true"}, false},
		{"attach", events.Message{Type: events.ContainerEventType, Action: events.ActionAttach}, false},
		{"image pull", events.Message{Type: events.ImageEventType, Action: events.ActionPull}, false},
		{"network start", events.Message{Type: events.NetworkEventType, Action: events.ActionStart}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isDockerContainerChange(tt.msg); got != tt.want {
				t.Errorf("isDockerContainerChange(%v) = %v, want %v", tt.msg, got, tt.want)
			}
		})
	}
}

func TestNewDockerEventSource_ReturnsOnCancel(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	NewDockerEventSource().Run(ctx, func() {})
}
