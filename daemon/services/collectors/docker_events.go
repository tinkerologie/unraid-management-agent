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

type dockerEventSpec map[events.Type][]events.Action

var dockerContainerEvents = dockerEventSpec{
	events.ContainerEventType: {
		events.ActionStart, events.ActionStop, events.ActionDie,
		events.ActionHealthStatus, events.ActionCreate, events.ActionDestroy,
	},
	events.NetworkEventType: {events.ActionConnect, events.ActionDisconnect},
}

var dockerInventoryEvents = dockerEventSpec{
	events.NetworkEventType: {events.ActionCreate, events.ActionDestroy, events.ActionConnect, events.ActionDisconnect},
}

// dockerEventsClient is the part of the Docker client the event source uses.
type dockerEventsClient interface {
	Ping(ctx context.Context, options client.PingOptions) (client.PingResult, error)
	Events(ctx context.Context, options client.EventsListOptions) client.EventsResult
	Close() error
}

// NewDockerEventSource reports container lifecycle and attachment changes.
func NewDockerEventSource() EventSource {
	return newDockerEventSource(dockerContainerEvents, openDockerEventsClient)
}

// NewDockerNetworkEventSource reports Docker network lifecycle and membership changes.
func NewDockerNetworkEventSource() EventSource {
	return newDockerEventSource(dockerInventoryEvents, openDockerEventsClient)
}

func openDockerEventsClient() (dockerEventsClient, error) {
	return client.New(client.FromEnv)
}

// newDockerEventSource builds the Docker source on an injectable client factory.
func newDockerEventSource(spec dockerEventSpec, newClient func() (dockerEventsClient, error)) *sessionSource {
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
		filters := make(client.Filters)
		for eventType, actions := range spec {
			filters.Add("type", string(eventType))
			for _, action := range actions {
				filters.Add("event", string(action))
			}
		}
		stream := cli.Events(streamCtx, client.EventsListOptions{Filters: filters})
		connected()

		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case msg, ok := <-stream.Messages:
				if !ok {
					return errors.New("event stream closed")
				}
				if spec.matches(msg) {
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

// matches rejects cross-type combinations that Docker's filter permits.
// Health events carry the status after a colon.
func (spec dockerEventSpec) matches(msg events.Message) bool {
	action, _, _ := strings.Cut(string(msg.Action), ":")
	return slices.Contains(spec[msg.Type], events.Action(action))
}
