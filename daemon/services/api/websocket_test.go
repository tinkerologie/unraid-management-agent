package api

import (
	"context"
	"testing"
	"time"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

func TestNewWSHub(t *testing.T) {
	hub := NewWSHub()

	if hub == nil {
		t.Fatal("NewWSHub() returned nil")
	}

	if hub.clients == nil {
		t.Error("clients map is nil")
	}

	if hub.broadcast == nil {
		t.Error("broadcast channel is nil")
	}

	if hub.register == nil {
		t.Error("register channel is nil")
	}

	if hub.unregister == nil {
		t.Error("unregister channel is nil")
	}
}

func TestWSHubRun(t *testing.T) {
	t.Run("stops on context cancellation", func(t *testing.T) {
		hub := NewWSHub()
		ctx, cancel := context.WithCancel(context.Background())

		done := make(chan struct{})
		go func() {
			hub.Run(ctx)
			close(done)
		}()

		time.Sleep(10 * time.Millisecond)
		cancel()

		select {
		case <-done:
		case <-time.After(1 * time.Second):
			t.Error("Hub did not stop after context cancellation")
		}
	})

	t.Run("registers client", func(t *testing.T) {
		hub := NewWSHub()
		ctx := t.Context()

		go hub.Run(ctx)
		time.Sleep(10 * time.Millisecond)

		client := &WSClient{
			hub:  hub,
			send: make(chan dto.WSEvent, 256),
		}

		hub.register <- client
		time.Sleep(10 * time.Millisecond)

		hub.mu.RLock()
		_, exists := hub.clients[client]
		hub.mu.RUnlock()

		if !exists {
			t.Error("Client was not registered")
		}
	})

	t.Run("unregisters client", func(t *testing.T) {
		hub := NewWSHub()
		ctx := t.Context()

		go hub.Run(ctx)
		time.Sleep(10 * time.Millisecond)

		client := &WSClient{
			hub:  hub,
			send: make(chan dto.WSEvent, 256),
		}

		hub.register <- client
		time.Sleep(10 * time.Millisecond)

		hub.unregister <- client
		time.Sleep(10 * time.Millisecond)

		hub.mu.RLock()
		_, exists := hub.clients[client]
		hub.mu.RUnlock()

		if exists {
			t.Error("Client was not unregistered")
		}
	})
}

func TestWSHubBroadcast(t *testing.T) {
	hub := NewWSHub()
	ctx := t.Context()

	go hub.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	clientSend := make(chan dto.WSEvent, 256)
	client := &WSClient{
		hub:  hub,
		send: clientSend,
	}

	hub.register <- client
	time.Sleep(10 * time.Millisecond)

	testMessage := map[string]string{"test": "message"}
	hub.Broadcast("update", testMessage)

	select {
	case msg := <-clientSend:
		if msg.Data == nil {
			t.Error("Received nil data")
		}
	case <-time.After(500 * time.Millisecond):
		t.Error("Did not receive broadcast message")
	}
}

func TestWSHubMultipleClients(t *testing.T) {
	hub := NewWSHub()
	ctx := t.Context()

	go hub.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	numClients := 5
	clients := make([]*WSClient, numClients)
	sends := make([]chan dto.WSEvent, numClients)

	for i := range numClients {
		sends[i] = make(chan dto.WSEvent, 256)
		clients[i] = &WSClient{
			hub:  hub,
			send: sends[i],
		}
		hub.register <- clients[i]
	}

	time.Sleep(50 * time.Millisecond)

	hub.mu.RLock()
	clientCount := len(hub.clients)
	hub.mu.RUnlock()

	if clientCount != numClients {
		t.Errorf("Expected %d clients, got %d", numClients, clientCount)
	}

	testMessage := "test broadcast"
	hub.Broadcast("update", testMessage)

	for i := range numClients {
		select {
		case <-sends[i]:
		case <-time.After(500 * time.Millisecond):
			t.Errorf("Client %d did not receive broadcast", i)
		}
	}
}

func TestWSHubUnregisterNonExistentClient(t *testing.T) {
	hub := NewWSHub()
	ctx := t.Context()

	go hub.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	client := &WSClient{
		hub:  hub,
		send: make(chan dto.WSEvent, 256),
	}

	hub.unregister <- client
	time.Sleep(10 * time.Millisecond)
}

func TestWSHubClosesClientsOnShutdown(t *testing.T) {
	hub := NewWSHub()
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		hub.Run(ctx)
		close(done)
	}()

	time.Sleep(10 * time.Millisecond)

	numClients := 3
	for range numClients {
		client := &WSClient{
			hub:  hub,
			send: make(chan dto.WSEvent, 256),
		}
		hub.register <- client
	}

	time.Sleep(50 * time.Millisecond)

	hub.mu.RLock()
	initialCount := len(hub.clients)
	hub.mu.RUnlock()

	if initialCount != numClients {
		t.Errorf("Expected %d clients before shutdown, got %d", numClients, initialCount)
	}

	cancel()

	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Error("Hub did not stop")
	}

	hub.mu.RLock()
	finalCount := len(hub.clients)
	hub.mu.RUnlock()

	if finalCount != 0 {
		t.Errorf("Expected 0 clients after shutdown, got %d", finalCount)
	}
}

func BenchmarkWSHubBroadcast(b *testing.B) {
	hub := NewWSHub()
	ctx := b.Context()

	go hub.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	for range 10 {
		client := &WSClient{
			hub:  hub,
			send: make(chan dto.WSEvent, 256),
		}
		hub.register <- client
		go func(c *WSClient) {
			for range c.send {
			}
		}(client)
	}

	time.Sleep(50 * time.Millisecond)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		hub.Broadcast("update", map[string]int{"count": i})
	}
}
