package collectors

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/moby/moby/api/types/events"
	"github.com/moby/moby/client"
)

// dockerEventsDebounce coalesces bursts such as stop+die+destroy.
const dockerEventsDebounce = 250 * time.Millisecond

// dockerEventActions are the container actions that change collected data.
var dockerEventActions = []events.Action{
	events.ActionStart, events.ActionStop, events.ActionDie,
	events.ActionHealthStatus, events.ActionCreate, events.ActionDestroy,
}

// dockerEventsClient is the part of the Docker client the event source uses.
type dockerEventsClient interface {
	Ping(ctx context.Context, options client.PingOptions) (client.PingResult, error)
	Events(ctx context.Context, options client.EventsListOptions) client.EventsResult
	Close() error
}

// NewDockerEventSource returns an EventSource that reports container
// lifecycle events from the Docker daemon. It opens its own Docker client.
func NewDockerEventSource() EventSource {
	return newDockerEventSource(func() (dockerEventsClient, error) {
		// API-version negotiation is the default in this client version.
		return client.New(client.FromEnv)
	})
}

// newDockerEventSource builds the Docker source on an injectable client factory.
func newDockerEventSource(newClient func() (dockerEventsClient, error)) *sessionSource {
	return newSessionSource("Docker event source", dockerEventsDebounce, func(ctx context.Context, connected, changed func()) error {
		cli, err := newClient()
		if err != nil {
			return fmt.Errorf("creating Docker client: %w", err)
		}
		defer func() { _ = cli.Close() }()
		if _, err := cli.Ping(ctx, client.PingOptions{}); err != nil {
			return fmt.Errorf("pinging Docker: %w", err)
		}

		// Cancelling the stream context ends the client's stream goroutine.
		streamCtx, cancel := context.WithCancel(ctx)
		defer cancel()
		filters := make(client.Filters).Add("type", string(events.ContainerEventType))
		for _, a := range dockerEventActions {
			filters.Add("event", string(a))
		}
		stream := cli.Events(streamCtx, client.EventsListOptions{Filters: filters})
		connected()

		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case msg := <-stream.Messages:
				if isDockerContainerChange(msg) {
					changed()
				}
			case err, ok := <-stream.Err:
				if !ok {
					return errors.New("event stream closed")
				}
				return fmt.Errorf("event stream: %w", err)
			}
		}
	})
}

// isDockerContainerChange reports whether msg is a container event with one of
// dockerEventActions. Health events carry the status after a colon, e.g.
// "health_status: healthy".
func isDockerContainerChange(msg events.Message) bool {
	if msg.Type != events.ContainerEventType {
		return false
	}
	action, _, _ := strings.Cut(string(msg.Action), ":")
	return slices.Contains(dockerEventActions, events.Action(action))
}
