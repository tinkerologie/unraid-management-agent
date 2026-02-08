package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/metoro-io/mcp-golang/transport"
)

func TestNewStreamableHTTPTransport(t *testing.T) {
	tr := NewStreamableHTTPTransport()

	if tr == nil {
		t.Fatal("expected transport to be created")
	}

	if tr.responseMap == nil {
		t.Error("expected responseMap to be initialized")
	}

	if tr.sseClients == nil {
		t.Error("expected sseClients to be initialized")
	}

	if tr.initialized {
		t.Error("expected initialized to be false")
	}
}

func TestStreamableHTTPTransportStart(t *testing.T) {
	tr := NewStreamableHTTPTransport()

	err := tr.Start(context.Background())
	if err != nil {
		t.Errorf("Start should return nil, got: %v", err)
	}
}

func TestStreamableHTTPTransportClose(t *testing.T) {
	tr := NewStreamableHTTPTransport()

	closeHandlerCalled := false
	tr.SetCloseHandler(func() {
		closeHandlerCalled = true
	})

	err := tr.Close()
	if err != nil {
		t.Errorf("Close should return nil, got: %v", err)
	}

	if !closeHandlerCalled {
		t.Error("expected close handler to be called")
	}
}

func TestStreamableHTTPTransportSetHandlers(t *testing.T) {
	tr := NewStreamableHTTPTransport()

	// Test SetErrorHandler
	tr.SetErrorHandler(func(_ error) {})
	tr.mu.RLock()
	if tr.errorHandler == nil {
		t.Error("expected error handler to be set")
	}
	tr.mu.RUnlock()

	// Test SetCloseHandler
	tr.SetCloseHandler(func() {})
	tr.mu.RLock()
	if tr.closeHandler == nil {
		t.Error("expected close handler to be set")
	}
	tr.mu.RUnlock()

	// Test SetMessageHandler
	tr.SetMessageHandler(func(_ context.Context, _ *transport.BaseJsonRpcMessage) {})
	tr.mu.RLock()
	if tr.messageHandler == nil {
		t.Error("expected message handler to be set")
	}
	tr.mu.RUnlock()
}

func TestStreamableHTTPTransportCORSPreflight(t *testing.T) {
	tr := NewStreamableHTTPTransport()
	handler := tr.Handler()

	req, _ := http.NewRequest(http.MethodOptions, "/mcp", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200 for OPTIONS, got %d", rr.Code)
	}

	// Verify CORS headers
	corsHeaders := map[string]string{
		"Access-Control-Allow-Origin":  "*",
		"Access-Control-Allow-Methods": "GET, POST, DELETE, OPTIONS",
		"Access-Control-Expose-Headers": "Mcp-Session-Id",
	}

	for header, expected := range corsHeaders {
		got := rr.Header().Get(header)
		if got != expected {
			t.Errorf("expected %s header to be %q, got %q", header, expected, got)
		}
	}

	// Check that Allow-Headers includes Mcp-Session-Id
	allowHeaders := rr.Header().Get("Access-Control-Allow-Headers")
	if allowHeaders == "" {
		t.Error("expected Access-Control-Allow-Headers to be set")
	}
}

func TestStreamableHTTPTransportMethodNotAllowed(t *testing.T) {
	tr := NewStreamableHTTPTransport()
	handler := tr.Handler()

	unsupportedMethods := []string{"PUT", "PATCH"}

	for _, method := range unsupportedMethods {
		t.Run(method, func(t *testing.T) {
			req, _ := http.NewRequest(method, "/mcp", nil)
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusMethodNotAllowed {
				t.Errorf("expected status 405 for %s, got %d", method, rr.Code)
			}
		})
	}
}

func TestStreamableHTTPTransportPostInitialize(t *testing.T) {
	tr := NewStreamableHTTPTransport()
	handler := tr.Handler()

	// Set up a message handler that replies to initialize
	tr.SetMessageHandler(func(ctx context.Context, message *transport.BaseJsonRpcMessage) {
		if message.JsonRpcRequest != nil && message.JsonRpcRequest.Method == "initialize" {
			result, _ := json.Marshal(map[string]interface{}{
				"protocolVersion": "2025-03-26",
				"capabilities":   map[string]interface{}{},
				"serverInfo": map[string]interface{}{
					"name":    "test-server",
					"version": "1.0.0",
				},
			})
			response := &transport.BaseJsonRpcMessage{
				Type: transport.BaseMessageTypeJSONRPCResponseType,
				JsonRpcResponse: &transport.BaseJSONRPCResponse{
					Jsonrpc: "2.0",
					Id:      message.JsonRpcRequest.Id,
					Result:  result,
				},
			}
			_ = tr.Send(ctx, response)
		}
	})

	body, _ := json.Marshal(map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]interface{}{
			"protocolVersion": "2025-03-26",
			"capabilities":   map[string]interface{}{},
			"clientInfo": map[string]interface{}{
				"name":    "test-client",
				"version": "1.0.0",
			},
		},
	})

	req, _ := http.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d, body: %s", rr.Code, rr.Body.String())
	}

	// Verify Mcp-Session-Id is returned
	sessionID := rr.Header().Get("Mcp-Session-Id")
	if sessionID == "" {
		t.Error("expected Mcp-Session-Id header to be set on initialize response")
	}

	// Verify Content-Type
	ct := rr.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", ct)
	}

	// Verify the response is valid JSON-RPC
	var response map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Errorf("failed to parse response body: %v", err)
	}

	if response["jsonrpc"] != "2.0" {
		t.Errorf("expected jsonrpc 2.0, got %v", response["jsonrpc"])
	}
}

func TestStreamableHTTPTransportPostNotification(t *testing.T) {
	tr := NewStreamableHTTPTransport()
	handler := tr.Handler()

	notificationReceived := false
	tr.SetMessageHandler(func(_ context.Context, message *transport.BaseJsonRpcMessage) {
		if message.JsonRpcNotification != nil {
			notificationReceived = true
		}
	})

	body, _ := json.Marshal(map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "notifications/initialized",
	})

	req, _ := http.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	// Notifications should return 202 Accepted
	if rr.Code != http.StatusAccepted {
		t.Errorf("expected status 202 Accepted for notification, got %d", rr.Code)
	}

	if !notificationReceived {
		t.Error("expected notification to be received by handler")
	}
}

func TestStreamableHTTPTransportInvalidSessionID(t *testing.T) {
	tr := NewStreamableHTTPTransport()
	handler := tr.Handler()

	// Simulate an initialized session
	tr.mu.Lock()
	tr.initialized = true
	tr.sessionID = "valid-session-id"
	tr.mu.Unlock()

	body, _ := json.Marshal(map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/list",
	})

	req, _ := http.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Mcp-Session-Id", "wrong-session-id")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	// Should return 404 for invalid session ID per spec
	if rr.Code != http.StatusNotFound {
		t.Errorf("expected status 404 for invalid session ID, got %d", rr.Code)
	}
}

func TestStreamableHTTPTransportDelete(t *testing.T) {
	tr := NewStreamableHTTPTransport()
	handler := tr.Handler()

	// Set up a session
	tr.mu.Lock()
	tr.initialized = true
	tr.sessionID = "session-to-delete"
	tr.mu.Unlock()

	// DELETE with correct session ID
	req, _ := http.NewRequest(http.MethodDelete, "/mcp", nil)
	req.Header.Set("Mcp-Session-Id", "session-to-delete")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200 for DELETE, got %d", rr.Code)
	}

	// Verify session was cleared
	tr.mu.RLock()
	if tr.initialized {
		t.Error("expected session to be terminated")
	}
	if tr.sessionID != "" {
		t.Error("expected session ID to be cleared")
	}
	tr.mu.RUnlock()
}

func TestStreamableHTTPTransportDeleteInvalidSession(t *testing.T) {
	tr := NewStreamableHTTPTransport()
	handler := tr.Handler()

	tr.mu.Lock()
	tr.initialized = true
	tr.sessionID = "valid-session"
	tr.mu.Unlock()

	req, _ := http.NewRequest(http.MethodDelete, "/mcp", nil)
	req.Header.Set("Mcp-Session-Id", "wrong-session")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	// Per spec 2025-06-18: terminated/unknown session returns 404
	if rr.Code != http.StatusNotFound {
		t.Errorf("expected status 404 for invalid session on DELETE, got %d", rr.Code)
	}
}

func TestStreamableHTTPTransportDeleteMissingSession(t *testing.T) {
	tr := NewStreamableHTTPTransport()
	handler := tr.Handler()

	tr.mu.Lock()
	tr.initialized = true
	tr.sessionID = "valid-session"
	tr.mu.Unlock()

	req, _ := http.NewRequest(http.MethodDelete, "/mcp", nil)
	// No Mcp-Session-Id header
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	// Per spec: missing session ID on DELETE returns 400
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 for missing session ID on DELETE, got %d", rr.Code)
	}
}

func TestStreamableHTTPTransportProtocolVersionHeader(t *testing.T) {
	tr := NewStreamableHTTPTransport()
	handler := tr.Handler()

	tr.mu.Lock()
	tr.initialized = true
	tr.sessionID = "test-session"
	tr.mu.Unlock()

	tests := []struct {
		name           string
		version        string
		expectedStatus int
	}{
		{"valid version 2025-06-18", "2025-06-18", http.StatusAccepted},
		{"valid version 2025-03-26", "2025-03-26", http.StatusAccepted},
		{"missing version (defaults to 2025-03-26)", "", http.StatusAccepted},
		{"unsupported version", "2020-01-01", http.StatusBadRequest},
		{"invalid version string", "not-a-version", http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(map[string]interface{}{
				"jsonrpc": "2.0",
				"method":  "notifications/initialized",
			})

			req, _ := http.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Mcp-Session-Id", "test-session")
			if tt.version != "" {
				req.Header.Set("MCP-Protocol-Version", tt.version)
			}
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Errorf("version %q: expected status %d, got %d", tt.version, tt.expectedStatus, rr.Code)
			}
		})
	}
}

func TestStreamableHTTPTransportGetWithoutAccept(t *testing.T) {
	tr := NewStreamableHTTPTransport()
	handler := tr.Handler()

	req, _ := http.NewRequest(http.MethodGet, "/mcp", nil)
	// No Accept header
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotAcceptable {
		t.Errorf("expected status 406 for GET without Accept: text/event-stream, got %d", rr.Code)
	}
}

func TestStreamableHTTPTransportGetInvalidSession(t *testing.T) {
	tr := NewStreamableHTTPTransport()
	handler := tr.Handler()

	tr.mu.Lock()
	tr.initialized = true
	tr.sessionID = "valid-session"
	tr.mu.Unlock()

	req, _ := http.NewRequest(http.MethodGet, "/mcp", nil)
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Mcp-Session-Id", "wrong-session")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected status 404 for invalid session on GET, got %d", rr.Code)
	}
}

func TestStreamableHTTPTransportClassifyMessage(t *testing.T) {
	tr := NewStreamableHTTPTransport()

	tests := []struct {
		name     string
		body     interface{}
		expected messageType
	}{
		{
			name: "request",
			body: map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      1,
				"method":  "tools/list",
			},
			expected: messageTypeRequest,
		},
		{
			name: "notification",
			body: map[string]interface{}{
				"jsonrpc": "2.0",
				"method":  "notifications/initialized",
			},
			expected: messageTypeNotification,
		},
		{
			name: "response",
			body: map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      1,
				"result":  map[string]interface{}{},
			},
			expected: messageTypeResponse,
		},
		{
			name: "error",
			body: map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      1,
				"error": map[string]interface{}{
					"code":    -32600,
					"message": "Invalid Request",
				},
			},
			expected: messageTypeError,
		},
		{
			name:     "unknown",
			body:     map[string]interface{}{"jsonrpc": "2.0"},
			expected: messageTypeUnknown,
		},
		{
			name:     "invalid JSON",
			body:     "not json",
			expected: messageTypeUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var bodyBytes []byte
			switch v := tt.body.(type) {
			case string:
				bodyBytes = []byte(v)
			default:
				bodyBytes, _ = json.Marshal(v)
			}

			result := tr.classifyMessage(bodyBytes)
			if result != tt.expected {
				t.Errorf("classifyMessage() = %d, expected %d", result, tt.expected)
			}
		})
	}
}

func TestStreamableHTTPTransportSendResponse(t *testing.T) {
	tr := NewStreamableHTTPTransport()

	// Create a response channel
	tr.mu.Lock()
	tr.responseMap[42] = make(chan *transport.BaseJsonRpcMessage, 1)
	tr.mu.Unlock()

	message := &transport.BaseJsonRpcMessage{
		Type: transport.BaseMessageTypeJSONRPCResponseType,
		JsonRpcResponse: &transport.BaseJSONRPCResponse{
			Jsonrpc: "2.0",
			Id:      42,
			Result:  mustMarshal(map[string]string{"status": "ok"}),
		},
	}

	err := tr.Send(context.Background(), message)
	if err != nil {
		t.Errorf("Send should succeed, got: %v", err)
	}
}

func TestStreamableHTTPTransportSendNoChannel(t *testing.T) {
	tr := NewStreamableHTTPTransport()

	// No SSE clients connected, should not error (broadcasts to empty set)
	message := &transport.BaseJsonRpcMessage{
		Type: transport.BaseMessageTypeJSONRPCNotificationType,
		JsonRpcNotification: &transport.BaseJSONRPCNotification{
			Jsonrpc: "2.0",
			Method:  "some/notification",
		},
	}

	err := tr.Send(context.Background(), message)
	if err != nil {
		t.Errorf("Send notification to no clients should not error, got: %v", err)
	}
}

func TestStreamableHTTPTransportConcurrentRequests(t *testing.T) {
	tr := NewStreamableHTTPTransport()
	handler := tr.Handler()

	// Set up a message handler that echoes back
	tr.SetMessageHandler(func(ctx context.Context, message *transport.BaseJsonRpcMessage) {
		if message.JsonRpcRequest != nil {
			result, _ := json.Marshal(map[string]interface{}{
				"method": message.JsonRpcRequest.Method,
			})
			response := &transport.BaseJsonRpcMessage{
				Type: transport.BaseMessageTypeJSONRPCResponseType,
				JsonRpcResponse: &transport.BaseJSONRPCResponse{
					Jsonrpc: "2.0",
					Id:      message.JsonRpcRequest.Id,
					Result:  result,
				},
			}
			_ = tr.Send(ctx, response)
		}
	})

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			body, _ := json.Marshal(map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      id,
				"method":  "tools/list",
			})

			req, _ := http.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Errorf("concurrent request %d: expected status 200, got %d", id, rr.Code)
			}
		}(i)
	}

	wg.Wait()
}

func TestStreamableHTTPTransportInvalidBody(t *testing.T) {
	tr := NewStreamableHTTPTransport()
	handler := tr.Handler()

	tr.SetMessageHandler(func(_ context.Context, _ *transport.BaseJsonRpcMessage) {})

	req, _ := http.NewRequest(http.MethodPost, "/mcp", bytes.NewReader([]byte("not valid json")))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	// Invalid JSON should return 400
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 for invalid JSON, got %d", rr.Code)
	}
}

func TestStreamableHTTPTransportSessionLifecycle(t *testing.T) {
	tr := NewStreamableHTTPTransport()
	handler := tr.Handler()

	// Set up handler
	tr.SetMessageHandler(func(ctx context.Context, message *transport.BaseJsonRpcMessage) {
		if message.JsonRpcRequest != nil {
			result, _ := json.Marshal(map[string]interface{}{
				"protocolVersion": "2025-03-26",
				"capabilities":   map[string]interface{}{},
				"serverInfo": map[string]interface{}{
					"name":    "test-server",
					"version": "1.0.0",
				},
			})
			response := &transport.BaseJsonRpcMessage{
				Type: transport.BaseMessageTypeJSONRPCResponseType,
				JsonRpcResponse: &transport.BaseJSONRPCResponse{
					Jsonrpc: "2.0",
					Id:      message.JsonRpcRequest.Id,
					Result:  result,
				},
			}
			_ = tr.Send(ctx, response)
		}
	})

	// Step 1: Initialize
	body, _ := json.Marshal(map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]interface{}{
			"protocolVersion": "2025-03-26",
		},
	})

	req, _ := http.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("initialize: expected 200, got %d", rr.Code)
	}

	sessionID := rr.Header().Get("Mcp-Session-Id")
	if sessionID == "" {
		t.Fatal("expected Mcp-Session-Id header")
	}

	// Step 2: Send notification with correct session ID
	notifBody, _ := json.Marshal(map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "notifications/initialized",
	})

	req2, _ := http.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(notifBody))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Mcp-Session-Id", sessionID)
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req2)

	if rr2.Code != http.StatusAccepted {
		t.Errorf("notification with valid session: expected 202, got %d", rr2.Code)
	}

	// Step 3: Terminate session
	req3, _ := http.NewRequest(http.MethodDelete, "/mcp", nil)
	req3.Header.Set("Mcp-Session-Id", sessionID)
	rr3 := httptest.NewRecorder()
	handler.ServeHTTP(rr3, req3)

	if rr3.Code != http.StatusOK {
		t.Errorf("delete session: expected 200, got %d", rr3.Code)
	}

	// Step 4: Verify session is gone
	tr.mu.RLock()
	if tr.initialized {
		t.Error("session should be terminated after DELETE")
	}
	tr.mu.RUnlock()
}

func TestGenerateSessionID(t *testing.T) {
	ids := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id := generateSessionID()
		if id == "" {
			t.Error("expected non-empty session ID")
		}
		if ids[id] {
			t.Errorf("duplicate session ID generated: %s", id)
		}
		ids[id] = true
	}
}

func TestSetCORSHeaders(t *testing.T) {
	rr := httptest.NewRecorder()
	setCORSHeaders(rr)

	expectedHeaders := map[string]string{
		"Access-Control-Allow-Origin":  "*",
		"Access-Control-Allow-Methods": "GET, POST, DELETE, OPTIONS",
		"Access-Control-Expose-Headers": "Mcp-Session-Id",
	}

	for header, expected := range expectedHeaders {
		got := rr.Header().Get(header)
		if got != expected {
			t.Errorf("setCORSHeaders: %s = %q, want %q", header, got, expected)
		}
	}

	allowHeaders := rr.Header().Get("Access-Control-Allow-Headers")
	if allowHeaders == "" {
		t.Error("expected Access-Control-Allow-Headers to be set")
	}

	// Verify MCP-Protocol-Version is in allowed headers (spec 2025-06-18)
	if !strings.Contains(allowHeaders, "MCP-Protocol-Version") {
		t.Error("expected Access-Control-Allow-Headers to include MCP-Protocol-Version")
	}
}

func TestStreamableHTTPTransportReadBody(t *testing.T) {
	tr := NewStreamableHTTPTransport()

	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid JSON", `{"test": "data"}`, false},
		{"empty body", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := bytes.NewReader([]byte(tt.input))
			body, err := tr.readBody(reader)

			if (err != nil) != tt.wantErr {
				t.Errorf("readBody() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr && body == nil {
				t.Error("expected non-nil body")
			}
		})
	}
}
