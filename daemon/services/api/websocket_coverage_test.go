package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

// newTestServerWithHub creates a Server with a running WSHub and returns a cleanup function.
func newTestServerWithHub(t *testing.T) (*Server, func()) {
	t.Helper()
	hub := domain.NewEventBus(10)
	ctx := &domain.Context{
		Hub:    hub,
		Config: domain.Config{Version: "test"},
	}
	server := NewServer(ctx)
	populateTestCaches(server)

	// Start the WSHub event loop (required for register/unregister/broadcast)
	go server.wsHub.Run(server.cancelCtx)

	return server, server.cancelFunc
}

// dialWS dials a WebSocket connection to the given httptest server.
func dialWS(t *testing.T, ts *httptest.Server) *websocket.Conn {
	t.Helper()
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/v1/ws"
	ws, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("WebSocket dial failed: %v", err)
	}
	if resp != nil {
		resp.Body.Close()
	}
	return ws
}

func TestWebSocketConnection(t *testing.T) {
	server, cancel := newTestServerWithHub(t)
	defer cancel()

	ts := httptest.NewServer(server.router)
	defer ts.Close()

	ws := dialWS(t, ts)
	defer ws.Close()

	// Verify the connection is usable by setting a short read deadline.
	// No initial message is expected, so a timeout is normal.
	ws.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	_, _, err := ws.ReadMessage()
	if err == nil {
		t.Log("Unexpectedly received an initial message")
	}
}

func TestWebSocketReceivesBroadcast(t *testing.T) {
	server, cancel := newTestServerWithHub(t)
	defer cancel()

	ts := httptest.NewServer(server.router)
	defer ts.Close()

	ws := dialWS(t, ts)
	defer ws.Close()

	// Allow time for the client to register with the hub
	time.Sleep(50 * time.Millisecond)

	// Broadcast a message through the hub
	testData := map[string]string{"status": "ok"}
	server.wsHub.Broadcast("update", testData)

	// The client should receive the broadcast wrapped in a WSEvent
	ws.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg, err := ws.ReadMessage()
	if err != nil {
		t.Fatalf("Failed to read broadcast message: %v", err)
	}

	var event dto.WSEvent
	if err := json.Unmarshal(msg, &event); err != nil {
		t.Fatalf("Failed to unmarshal WSEvent: %v", err)
	}

	if event.Event != "update" {
		t.Errorf("Expected event type 'update', got %q", event.Event)
	}

	if event.Data == nil {
		t.Error("WSEvent.Data is nil")
	}

	if event.Timestamp.IsZero() {
		t.Error("WSEvent.Timestamp is zero")
	}
}

func TestWebSocketMultipleBroadcasts(t *testing.T) {
	server, cancel := newTestServerWithHub(t)
	defer cancel()

	ts := httptest.NewServer(server.router)
	defer ts.Close()

	ws := dialWS(t, ts)
	defer ws.Close()

	time.Sleep(50 * time.Millisecond)

	// Send multiple broadcasts
	count := 5
	for i := range count {
		server.wsHub.Broadcast("update", map[string]int{"seq": i})
	}

	// Read all messages
	received := 0
	ws.SetReadDeadline(time.Now().Add(2 * time.Second))
	for range count {
		_, _, err := ws.ReadMessage()
		if err != nil {
			t.Logf("Read stopped after %d messages: %v", received, err)
			break
		}
		received++
	}

	if received != count {
		t.Errorf("Expected %d messages, received %d", count, received)
	}
}

func TestWebSocketMultipleClients(t *testing.T) {
	server, cancel := newTestServerWithHub(t)
	defer cancel()

	ts := httptest.NewServer(server.router)
	defer ts.Close()

	const numClients = 3
	clients := make([]*websocket.Conn, numClients)
	for i := range numClients {
		clients[i] = dialWS(t, ts)
		defer clients[i].Close()
	}

	// Wait for all clients to register
	time.Sleep(100 * time.Millisecond)

	// Broadcast a message
	server.wsHub.Broadcast("update", map[string]string{"to": "all"})

	// All clients should receive it
	for i, ws := range clients {
		ws.SetReadDeadline(time.Now().Add(2 * time.Second))
		_, msg, err := ws.ReadMessage()
		if err != nil {
			t.Errorf("Client %d failed to read: %v", i, err)
			continue
		}

		var event dto.WSEvent
		if err := json.Unmarshal(msg, &event); err != nil {
			t.Errorf("Client %d: unmarshal failed: %v", i, err)
		}
	}
}

func TestWebSocketClientDisconnect(t *testing.T) {
	server, cancel := newTestServerWithHub(t)
	defer cancel()

	ts := httptest.NewServer(server.router)
	defer ts.Close()

	ws := dialWS(t, ts)

	// Wait for registration
	time.Sleep(50 * time.Millisecond)

	// Verify client is registered
	server.wsHub.mu.RLock()
	clientsBefore := len(server.wsHub.clients)
	server.wsHub.mu.RUnlock()

	if clientsBefore != 1 {
		t.Errorf("Expected 1 registered client, got %d", clientsBefore)
	}

	// Close the WebSocket — readPump should detect this and unregister
	ws.Close()

	// Wait for unregister to propagate
	time.Sleep(200 * time.Millisecond)

	server.wsHub.mu.RLock()
	clientsAfter := len(server.wsHub.clients)
	server.wsHub.mu.RUnlock()

	if clientsAfter != 0 {
		t.Errorf("Expected 0 clients after disconnect, got %d", clientsAfter)
	}
}

func TestWebSocketCloseAndReconnect(t *testing.T) {
	server, cancel := newTestServerWithHub(t)
	defer cancel()

	ts := httptest.NewServer(server.router)
	defer ts.Close()

	// Connect, close, reconnect
	ws1 := dialWS(t, ts)
	time.Sleep(50 * time.Millisecond)
	ws1.Close()
	time.Sleep(100 * time.Millisecond)

	ws2 := dialWS(t, ts)
	defer ws2.Close()
	time.Sleep(50 * time.Millisecond)

	// Broadcast should reach the new connection
	server.wsHub.Broadcast("update", map[string]string{"reconnect": "yes"})

	ws2.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _, err := ws2.ReadMessage()
	if err != nil {
		t.Fatalf("Reconnected client failed to receive broadcast: %v", err)
	}
}

func TestWebSocketWritePumpCloseMessage(t *testing.T) {
	server, cancel := newTestServerWithHub(t)

	ts := httptest.NewServer(server.router)
	defer ts.Close()

	ws := dialWS(t, ts)
	defer ws.Close()

	time.Sleep(50 * time.Millisecond)

	// Cancel the server context — the hub will close all client send channels,
	// which triggers writePump to send a CloseMessage.
	cancel()

	// The client should receive a close frame or a read error
	ws.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _, err := ws.ReadMessage()
	if err == nil {
		t.Log("Received a message after server shutdown (unexpected but not fatal)")
	}
	// A close error or read error is expected — the key thing is no hang.
}

func TestWebSocketReadPumpMessage(t *testing.T) {
	server, cancel := newTestServerWithHub(t)
	defer cancel()

	ts := httptest.NewServer(server.router)
	defer ts.Close()

	ws := dialWS(t, ts)
	defer ws.Close()

	time.Sleep(50 * time.Millisecond)

	// Send a text message from the client — readPump reads and discards it.
	err := ws.WriteMessage(websocket.TextMessage, []byte(`{"type":"test"}`))
	if err != nil {
		t.Fatalf("Failed to write message: %v", err)
	}

	// Send a second message to confirm the read loop is still active
	err = ws.WriteMessage(websocket.TextMessage, []byte(`{"type":"test2"}`))
	if err != nil {
		t.Fatalf("Failed to write second message: %v", err)
	}

	// Give readPump time to process
	time.Sleep(50 * time.Millisecond)
}

func TestWebSocketPongHandler(t *testing.T) {
	server, cancel := newTestServerWithHub(t)
	defer cancel()

	ts := httptest.NewServer(server.router)
	defer ts.Close()

	ws := dialWS(t, ts)
	defer ws.Close()

	time.Sleep(50 * time.Millisecond)

	// Send a pong frame from the client — the server's readPump has a PongHandler
	// that extends the read deadline. This exercises that code path.
	err := ws.WriteMessage(websocket.PongMessage, nil)
	if err != nil {
		t.Fatalf("Failed to send pong: %v", err)
	}

	// Connection should still be alive
	time.Sleep(50 * time.Millisecond)

	// Broadcast after pong to verify the connection is still active
	server.wsHub.Broadcast("update", map[string]string{"after": "pong"})

	ws.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _, err = ws.ReadMessage()
	if err != nil {
		t.Fatalf("Connection died after pong: %v", err)
	}
}

func TestWebSocketConcurrentBroadcasts(t *testing.T) {
	server, cancel := newTestServerWithHub(t)
	defer cancel()

	ts := httptest.NewServer(server.router)
	defer ts.Close()

	ws := dialWS(t, ts)
	defer ws.Close()

	time.Sleep(50 * time.Millisecond)

	// Concurrently broadcast from multiple goroutines
	var wg sync.WaitGroup
	const goroutines = 5
	const msgsPerGoroutine = 3

	for g := range goroutines {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := range msgsPerGoroutine {
				server.wsHub.Broadcast("update", map[string]int{"goroutine": id, "msg": j})
			}
		}(g)
	}
	wg.Wait()

	// Read all messages
	totalExpected := goroutines * msgsPerGoroutine
	received := 0
	ws.SetReadDeadline(time.Now().Add(3 * time.Second))
	for range totalExpected {
		_, _, err := ws.ReadMessage()
		if err != nil {
			break
		}
		received++
	}

	if received != totalExpected {
		t.Errorf("Expected %d messages from concurrent broadcast, got %d", totalExpected, received)
	}
}

func TestWebSocketUpgradeFailsOnNonGET(t *testing.T) {
	server, cancel := newTestServerWithHub(t)
	defer cancel()

	ts := httptest.NewServer(server.router)
	defer ts.Close()

	// POST to the WebSocket endpoint should fail to upgrade
	resp, err := http.Post(ts.URL+"/api/v1/ws", "application/json", nil)
	if err != nil {
		t.Fatalf("POST request failed: %v", err)
	}
	resp.Body.Close()

	// Should not be 101 Switching Protocols
	if resp.StatusCode == http.StatusSwitchingProtocols {
		t.Error("POST should not upgrade to WebSocket")
	}
}

func TestWebSocketBroadcastAfterClientClose(t *testing.T) {
	server, cancel := newTestServerWithHub(t)
	defer cancel()

	ts := httptest.NewServer(server.router)
	defer ts.Close()

	ws := dialWS(t, ts)

	time.Sleep(50 * time.Millisecond)

	// Close the client
	ws.Close()
	time.Sleep(100 * time.Millisecond)

	// Broadcasting after client disconnect should not panic
	server.wsHub.Broadcast("update", map[string]string{"after": "close"})
	time.Sleep(50 * time.Millisecond)
}

func TestWebSocketGracefulCloseFromClient(t *testing.T) {
	server, cancel := newTestServerWithHub(t)
	defer cancel()

	ts := httptest.NewServer(server.router)
	defer ts.Close()

	ws := dialWS(t, ts)
	time.Sleep(50 * time.Millisecond)

	// Send a proper close frame from the client
	closeMsg := websocket.FormatCloseMessage(websocket.CloseNormalClosure, "bye")
	err := ws.WriteMessage(websocket.CloseMessage, closeMsg)
	if err != nil {
		t.Logf("Error writing close message: %v", err)
	}

	// Wait for server-side cleanup
	time.Sleep(200 * time.Millisecond)

	server.wsHub.mu.RLock()
	count := len(server.wsHub.clients)
	server.wsHub.mu.RUnlock()

	if count != 0 {
		t.Errorf("Expected 0 clients after graceful close, got %d", count)
	}
}

// newTestServerWithCORSOrigin creates a Server with the given CORSOrigin configuration.
func newTestServerWithCORSOrigin(t *testing.T, corsOrigin string) (*Server, func()) {
	t.Helper()
	hub := domain.NewEventBus(10)
	ctx := &domain.Context{
		Hub: hub,
		Config: domain.Config{
			Version:    "test",
			CORSOrigin: corsOrigin,
		},
	}
	server := NewServer(ctx)
	populateTestCaches(server)
	go server.wsHub.Run(server.cancelCtx)
	return server, server.cancelFunc
}

// dialWSWithOrigin dials a WebSocket connection sending the given Origin header.
// It returns the connection and response; the caller decides whether to expect success
// or failure.
func dialWSWithOrigin(ts *httptest.Server, origin string) (*websocket.Conn, *http.Response, error) {
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/v1/ws"
	headers := http.Header{}
	if origin != "" {
		headers.Set("Origin", origin)
	}
	return websocket.DefaultDialer.Dial(wsURL, headers)
}

// TestWebSocketOriginWildcardSameHost verifies that with an empty (wildcard) CORSOrigin,
// a browser Origin that matches the server host is accepted.
func TestWebSocketOriginWildcardSameHost(t *testing.T) {
	server, cancel := newTestServerWithCORSOrigin(t, "")
	defer cancel()

	ts := httptest.NewServer(server.router)
	defer ts.Close()

	// Construct an Origin that matches the test server's host (ts.URL is already "http://host:port").
	sameOrigin := ts.URL
	conn, resp, err := dialWSWithOrigin(ts, sameOrigin)
	if err != nil {
		t.Fatalf("Expected same-host Origin to be accepted, got error: %v", err)
	}
	if resp != nil {
		resp.Body.Close()
	}
	conn.Close()
}

// TestWebSocketOriginWildcardMismatch verifies that with an empty (wildcard) CORSOrigin,
// a browser Origin from a different host is rejected.
func TestWebSocketOriginWildcardMismatch(t *testing.T) {
	server, cancel := newTestServerWithCORSOrigin(t, "")
	defer cancel()

	ts := httptest.NewServer(server.router)
	defer ts.Close()

	_, resp, err := dialWSWithOrigin(ts, "http://evil.example.com")
	if err == nil {
		t.Error("Expected mismatched Origin to be rejected, but dial succeeded")
		return
	}
	// Expect 403 Forbidden from the upgrade rejection.
	if resp != nil {
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("Expected 403 for mismatched Origin, got %d", resp.StatusCode)
		}
	}
}

// TestWebSocketOriginNoHeader verifies that clients without an Origin header (non-browser
// clients such as CLI tools) are always accepted regardless of CORSOrigin config.
func TestWebSocketOriginNoHeader(t *testing.T) {
	server, cancel := newTestServerWithCORSOrigin(t, "http://specific.example.com")
	defer cancel()

	ts := httptest.NewServer(server.router)
	defer ts.Close()

	// Dial without any Origin header — should succeed.
	conn, resp, err := dialWSWithOrigin(ts, "")
	if err != nil {
		t.Fatalf("Expected no-Origin request to succeed, got error: %v", err)
	}
	if resp != nil {
		resp.Body.Close()
	}
	conn.Close()
}

// TestWebSocketOriginSpecificMatch verifies that when CORSOrigin is set to a specific value,
// a matching Origin is accepted.
func TestWebSocketOriginSpecificMatch(t *testing.T) {
	allowed := "http://allowed.example.com"
	server, cancel := newTestServerWithCORSOrigin(t, allowed)
	defer cancel()

	ts := httptest.NewServer(server.router)
	defer ts.Close()

	conn, resp, err := dialWSWithOrigin(ts, allowed)
	if err != nil {
		t.Fatalf("Expected allowed Origin to be accepted, got error: %v", err)
	}
	if resp != nil {
		resp.Body.Close()
	}
	conn.Close()
}

// TestWebSocketOriginSpecificMismatch verifies that when CORSOrigin is set to a specific value,
// a different Origin is rejected.
func TestWebSocketOriginSpecificMismatch(t *testing.T) {
	server, cancel := newTestServerWithCORSOrigin(t, "http://allowed.example.com")
	defer cancel()

	ts := httptest.NewServer(server.router)
	defer ts.Close()

	_, resp, err := dialWSWithOrigin(ts, "http://evil.example.com")
	if err == nil {
		t.Error("Expected mismatched Origin to be rejected, but dial succeeded")
		return
	}
	if resp != nil {
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("Expected 403 for mismatched Origin, got %d", resp.StatusCode)
		}
	}
}

// TestWebSocketReadLimitEnforced verifies that sending a message exceeding the 64 KB
// read limit causes the server to close the connection (DoS protection).
func TestWebSocketReadLimitEnforced(t *testing.T) {
	server, cancel := newTestServerWithHub(t)
	defer cancel()

	ts := httptest.NewServer(server.router)
	defer ts.Close()

	conn, resp, err := dialWSWithOrigin(ts, "")
	if err != nil {
		t.Fatalf("WebSocket dial failed: %v", err)
	}
	if resp != nil {
		resp.Body.Close()
	}
	defer conn.Close()

	// Send a message that exceeds the 64 KB limit.
	oversized := make([]byte, maxWSMessageSize+1)
	for i := range oversized {
		oversized[i] = 'A'
	}
	if err := conn.WriteMessage(websocket.TextMessage, oversized); err != nil {
		// If the write fails the server may have already closed the connection —
		// that also satisfies the requirement.
		t.Logf("Write failed (server likely already closed connection): %v", err)
		return
	}

	// The server's readPump should close the connection after reading the
	// oversized message. The client should receive a close frame or an error.
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _, readErr := conn.ReadMessage()
	if readErr == nil {
		t.Error("Expected connection to be closed after oversized message, but read succeeded")
	}
}
