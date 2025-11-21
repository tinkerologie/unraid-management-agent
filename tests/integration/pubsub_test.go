package integration

import (
	"testing"
	"time"

	"github.com/cskr/pubsub"
)

func TestPubSubBasicFlow(t *testing.T) {
	hub := pubsub.New(10)

	// Subscribe to a topic
	ch := hub.Sub("test_topic")

	// Publish a message
	hub.Pub("test message", "test_topic")

	// Receive the message
	select {
	case msg := <-ch:
		if msg != "test message" {
			t.Errorf("Received = %v, want %q", msg, "test message")
		}
	case <-time.After(1 * time.Second):
		t.Error("Timeout waiting for message")
	}

	hub.Unsub(ch)
}

func TestPubSubMultipleSubscribers(t *testing.T) {
	hub := pubsub.New(10)

	// Create multiple subscribers
	ch1 := hub.Sub("events")
	ch2 := hub.Sub("events")
	ch3 := hub.Sub("events")

	// Publish a message
	hub.Pub("broadcast message", "events")

	// All subscribers should receive the message
	channels := []chan interface{}{ch1, ch2, ch3}
	for i, ch := range channels {
		select {
		case msg := <-ch:
			if msg != "broadcast message" {
				t.Errorf("Subscriber %d received %v, want %q", i, msg, "broadcast message")
			}
		case <-time.After(1 * time.Second):
			t.Errorf("Subscriber %d timeout", i)
		}
	}

	hub.Unsub(ch1)
	hub.Unsub(ch2)
	hub.Unsub(ch3)
}

func TestPubSubMultipleTopics(t *testing.T) {
	hub := pubsub.New(10)

	// Subscribe to different topics
	systemCh := hub.Sub("system_update")
	dockerCh := hub.Sub("container_list_update")
	arrayCh := hub.Sub("array_status_update")

	// Publish to different topics
	hub.Pub("system data", "system_update")
	hub.Pub("docker data", "container_list_update")
	hub.Pub("array data", "array_status_update")

	// Verify each subscriber receives correct message
	tests := []struct {
		ch       chan interface{}
		expected string
		topic    string
	}{
		{systemCh, "system data", "system_update"},
		{dockerCh, "docker data", "container_list_update"},
		{arrayCh, "array data", "array_status_update"},
	}

	for _, tt := range tests {
		select {
		case msg := <-tt.ch:
			if msg != tt.expected {
				t.Errorf("Topic %s: received %v, want %q", tt.topic, msg, tt.expected)
			}
		case <-time.After(1 * time.Second):
			t.Errorf("Topic %s: timeout", tt.topic)
		}
	}

	hub.Unsub(systemCh)
	hub.Unsub(dockerCh)
	hub.Unsub(arrayCh)
}

func TestPubSubUnsubscribe(t *testing.T) {
	hub := pubsub.New(10)

	ch := hub.Sub("test")

	// Unsubscribe
	hub.Unsub(ch)

	// Publish after unsubscribe
	hub.Pub("message", "test")

	// Channel should be closed or empty
	select {
	case _, ok := <-ch:
		if ok {
			t.Error("Channel should be closed after unsubscribe")
		}
	case <-time.After(100 * time.Millisecond):
		// Timeout is expected - no message received
	}
}

func TestPubSubHighVolume(t *testing.T) {
	hub := pubsub.New(1000)

	ch := hub.Sub("high_volume")

	// Publish many messages
	messageCount := 100
	go func() {
		for i := 0; i < messageCount; i++ {
			hub.Pub(i, "high_volume")
		}
	}()

	// Receive all messages
	received := 0
	timeout := time.After(5 * time.Second)

	for received < messageCount {
		select {
		case <-ch:
			received++
		case <-timeout:
			t.Errorf("Timeout: received %d/%d messages", received, messageCount)
			return
		}
	}

	hub.Unsub(ch)
}

func TestPubSubTypedMessages(t *testing.T) {
	hub := pubsub.New(10)

	ch := hub.Sub("typed")

	// Publish different types
	type TestData struct {
		Name  string
		Value int
	}

	testData := TestData{Name: "test", Value: 42}
	hub.Pub(testData, "typed")

	// Receive and type assert
	select {
	case msg := <-ch:
		data, ok := msg.(TestData)
		if !ok {
			t.Error("Failed to type assert message")
			return
		}
		if data.Name != "test" || data.Value != 42 {
			t.Errorf("Data = %+v, want {Name: test, Value: 42}", data)
		}
	case <-time.After(1 * time.Second):
		t.Error("Timeout waiting for message")
	}

	hub.Unsub(ch)
}

func BenchmarkPubSub(b *testing.B) {
	hub := pubsub.New(1000)
	ch := hub.Sub("benchmark")

	// Drain channel in background
	go func() {
		for range ch {
		}
	}()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		hub.Pub(i, "benchmark")
	}

	hub.Unsub(ch)
}
