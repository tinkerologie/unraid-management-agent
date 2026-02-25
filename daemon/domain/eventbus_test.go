package domain

import (
	"sync"
	"testing"
	"time"
)

func TestEventBus_PubSub(t *testing.T) {
	bus := NewEventBus(10)
	ch := bus.Sub("test")

	bus.Pub("hello", "test")

	select {
	case msg := <-ch:
		if msg != "hello" {
			t.Errorf("expected %q, got %q", "hello", msg)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for message")
	}
}

func TestEventBus_MultiTopic(t *testing.T) {
	bus := NewEventBus(10)
	ch := bus.Sub("a", "b")

	bus.Pub("from_a", "a")
	bus.Pub("from_b", "b")

	msgs := make([]any, 0, 2)
	for range 2 {
		select {
		case msg := <-ch:
			msgs = append(msgs, msg)
		case <-time.After(time.Second):
			t.Fatal("timed out")
		}
	}

	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
}

func TestEventBus_MultipleSubscribers(t *testing.T) {
	bus := NewEventBus(10)
	ch1 := bus.Sub("topic")
	ch2 := bus.Sub("topic")

	bus.Pub(42, "topic")

	for _, ch := range []chan any{ch1, ch2} {
		select {
		case msg := <-ch:
			if msg != 42 {
				t.Errorf("expected 42, got %v", msg)
			}
		case <-time.After(time.Second):
			t.Fatal("timed out")
		}
	}
}

func TestEventBus_Unsub(t *testing.T) {
	bus := NewEventBus(10)
	ch := bus.Sub("topic")

	bus.Unsub(ch, "topic")

	// Channel should be closed because ch is no longer in any topic.
	_, ok := <-ch
	if ok {
		t.Fatal("expected channel to be closed after full unsubscribe")
	}

	// Publishing after unsub should not panic.
	bus.Pub("msg", "topic")
}

func TestEventBus_UnsubAll(t *testing.T) {
	bus := NewEventBus(10)
	ch := bus.Sub("a", "b")

	bus.Unsub(ch) // no topics = unsub from all

	// Channel should be closed.
	_, ok := <-ch
	if ok {
		t.Fatal("expected channel to be closed after unsubscribe-all")
	}

	// Publishing after unsub should not panic.
	bus.Pub("msg", "a")
	bus.Pub("msg", "b")
}

func TestEventBus_UnsubPartial(t *testing.T) {
	bus := NewEventBus(10)
	ch := bus.Sub("a", "b")

	// Unsub from "a" only — ch is still subscribed to "b", so it stays open.
	bus.Unsub(ch, "a")

	bus.Pub("still-here", "b")

	select {
	case msg := <-ch:
		if msg != "still-here" {
			t.Errorf("expected still-here, got %v", msg)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for message on partially unsubscribed channel")
	}

	// Now unsub from "b" — channel should close.
	bus.Unsub(ch, "b")
	_, ok := <-ch
	if ok {
		t.Fatal("expected channel to be closed after full unsubscribe")
	}
}

func TestEventBus_SlowSubscriberDrops(t *testing.T) {
	bus := NewEventBus(1) // tiny buffer
	ch := bus.Sub("topic")

	// Fill the buffer
	bus.Pub("msg1", "topic")
	// This should be dropped (non-blocking)
	bus.Pub("msg2", "topic")

	msg := <-ch
	if msg != "msg1" {
		t.Errorf("expected msg1, got %v", msg)
	}

	select {
	case <-ch:
		t.Fatal("should not have received dropped message")
	case <-time.After(50 * time.Millisecond):
		// expected — msg2 was dropped
	}
}

func TestEventBus_ConcurrentPubSub(t *testing.T) {
	bus := NewEventBus(100)
	ch := bus.Sub("topic")

	var wg sync.WaitGroup
	count := 50

	wg.Add(count)
	for range count {
		go func() {
			defer wg.Done()
			bus.Pub("msg", "topic")
		}()
	}

	received := 0
	done := make(chan struct{})
	go func() {
		for range count {
			<-ch
			received++
		}
		close(done)
	}()

	wg.Wait()

	select {
	case <-done:
		if received != count {
			t.Errorf("expected %d messages, got %d", count, received)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out, received %d/%d", received, count)
	}
}

// ---------------------------------------------------------------------------
// Typed generic API tests
// ---------------------------------------------------------------------------

func TestTypedPublish(t *testing.T) {
	type payload struct{ Value int }
	topic := NewTopic[payload]("typed")
	bus := NewEventBus(10)

	ch := bus.Sub(topic.Name)

	Publish(bus, topic, payload{Value: 42})

	select {
	case msg := <-ch:
		v, ok := msg.(payload)
		if !ok {
			t.Fatalf("expected payload, got %T", msg)
		}
		if v.Value != 42 {
			t.Errorf("expected 42, got %d", v.Value)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out")
	}
}

func TestTypedPublish_Pointer(t *testing.T) {
	type info struct{ Name string }
	topic := NewTopic[*info]("ptr_topic")
	bus := NewEventBus(10)

	ch := bus.Sub(topic.Name)

	Publish(bus, topic, &info{Name: "test"})

	select {
	case msg := <-ch:
		v, ok := msg.(*info)
		if !ok {
			t.Fatalf("expected *info, got %T", msg)
		}
		if v.Name != "test" {
			t.Errorf("expected %q, got %q", "test", v.Name)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out")
	}
}
