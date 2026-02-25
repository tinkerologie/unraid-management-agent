package domain

import "sync"

// EventBus is a type-safe publish/subscribe event bus.
// It provides an untyped API (Sub/Pub/Unsub) that mirrors the cskr/pubsub
// interface for multi-topic receivers, and a typed generic API
// (Publish[T]/Topic[T]) that catches publisher type mismatches at compile time.
type EventBus struct {
	mu         sync.RWMutex
	subs       map[string][]chan any
	bufferSize int
}

// NewEventBus creates a new EventBus with the given per-subscriber buffer size.
// If bufferSize is less than 1, it defaults to 1.
func NewEventBus(bufferSize int) *EventBus {
	if bufferSize < 1 {
		bufferSize = 1
	}
	return &EventBus{
		subs:       make(map[string][]chan any),
		bufferSize: bufferSize,
	}
}

// Sub subscribes to one or more topics and returns a channel that receives
// messages published to any of those topics. The channel is shared across all
// requested topics, so a type switch is required when reading.
func (bus *EventBus) Sub(topics ...string) chan any {
	ch := make(chan any, bus.bufferSize)
	bus.mu.Lock()
	for _, t := range topics {
		bus.subs[t] = append(bus.subs[t], ch)
	}
	bus.mu.Unlock()
	return ch
}

// Pub publishes msg to all subscribers of the given topics.
// Argument order matches cskr/pubsub: data first, then topic(s).
func (bus *EventBus) Pub(msg any, topics ...string) {
	bus.mu.RLock()
	for _, t := range topics {
		for _, ch := range bus.subs[t] {
			select {
			case ch <- msg:
			default:
				// subscriber is slow — drop to avoid blocking publishers
			}
		}
	}
	bus.mu.RUnlock()
}

// Unsub removes ch from the given topics. If no topics are specified,
// ch is removed from all topics. When ch is no longer subscribed to any
// topic it is closed, unblocking any goroutine reading from it.
func (bus *EventBus) Unsub(ch chan any, topics ...string) {
	bus.mu.Lock()
	defer bus.mu.Unlock()

	if len(topics) == 0 {
		// Remove from all topics and close immediately.
		for t, subs := range bus.subs {
			bus.subs[t] = removeChan(subs, ch)
		}
		close(ch)
		return
	}

	for _, t := range topics {
		if subs, ok := bus.subs[t]; ok {
			bus.subs[t] = removeChan(subs, ch)
		}
	}

	// Close ch only if it is no longer subscribed to any remaining topic.
	for _, subs := range bus.subs {
		for _, s := range subs {
			if s == ch {
				return // still subscribed elsewhere
			}
		}
	}
	close(ch)
}

// removeChan removes ch from a slice of channels without preserving order.
func removeChan(subs []chan any, ch chan any) []chan any {
	for i, s := range subs {
		if s == ch {
			subs[i] = subs[len(subs)-1]
			return subs[:len(subs)-1]
		}
	}
	return subs
}

// ---------------------------------------------------------------------------
// Typed generic API
// ---------------------------------------------------------------------------

// Topic is a typed topic identifier. The type parameter T documents (and
// enforces at compile time) what Go type is published on this topic.
type Topic[T any] struct {
	Name string
}

// NewTopic creates a typed topic with the given name.
func NewTopic[T any](name string) Topic[T] {
	return Topic[T]{Name: name}
}

// Publish sends typed data to all subscribers of topic.
// Because topic carries type parameter T, passing the wrong data type is
// a compile-time error.
func Publish[T any](bus *EventBus, topic Topic[T], data T) {
	bus.Pub(data, topic.Name)
}

// topicNamer is satisfied by any Topic[T] and allows accepting mixed generic
// topic types in a single variadic argument list.
type topicNamer interface{ TopicName() string }

// TopicName returns the string name of the topic (implements topicNamer).
func (t Topic[T]) TopicName() string { return t.Name }

// SubTopics subscribes to one or more typed topics. It extracts the string
// name from each Topic[T] automatically, avoiding manual .Name access.
func (bus *EventBus) SubTopics(topics ...topicNamer) chan any {
	names := make([]string, len(topics))
	for i, t := range topics {
		names[i] = t.TopicName()
	}
	return bus.Sub(names...)
}
