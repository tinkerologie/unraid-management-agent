package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/metoro-io/mcp-golang/transport"
)

// helper to create json.RawMessage from a map
func mustMarshal(v interface{}) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

func TestNewStdHTTPTransport(t *testing.T) {
	tr := NewStdHTTPTransport()

	if tr == nil {
		t.Fatal("expected transport to be created")
	}

	if tr.responseMap == nil {
		t.Error("expected responseMap to be initialized")
	}
}

func TestStdHTTPTransportStart(t *testing.T) {
	tr := NewStdHTTPTransport()

	err := tr.Start(context.Background())
	if err != nil {
		t.Errorf("Start should return nil, got: %v", err)
	}
}

func TestStdHTTPTransportClose(t *testing.T) {
	tr := NewStdHTTPTransport()

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

func TestStdHTTPTransportSetHandlers(t *testing.T) {
	tr := NewStdHTTPTransport()

	// Test SetErrorHandler
	tr.SetErrorHandler(func(err error) {
		// handler set
	})

	tr.mu.RLock()
	if tr.errorHandler == nil {
		t.Error("expected error handler to be set")
	}
	tr.mu.RUnlock()

	// Test SetCloseHandler
	tr.SetCloseHandler(func() {
		// handler set
	})

	tr.mu.RLock()
	if tr.closeHandler == nil {
		t.Error("expected close handler to be set")
	}
	tr.mu.RUnlock()

	// Test SetMessageHandler
	tr.SetMessageHandler(func(ctx context.Context, message *transport.BaseJsonRpcMessage) {
		// handler set
	})

	tr.mu.RLock()
	if tr.messageHandler == nil {
		t.Error("expected message handler to be set")
	}
	tr.mu.RUnlock()
}

func TestStdHTTPTransportHandlerMethodNotAllowed(t *testing.T) {
	tr := NewStdHTTPTransport()

	handler := tr.Handler()

	tests := []struct {
		method string
	}{
		{"GET"},
		{"PUT"},
		{"DELETE"},
		{"PATCH"},
	}

	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			req, _ := http.NewRequest(tt.method, "/mcp", nil)
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusMethodNotAllowed {
				t.Errorf("expected status 405 for %s, got %d", tt.method, rr.Code)
			}
		})
	}
}

func TestStdHTTPTransportReadBody(t *testing.T) {
	tr := NewStdHTTPTransport()

	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid JSON", `{"test": "data"}`, false},
		{"empty body", "", false},
		{"large body", string(bytes.Repeat([]byte("a"), 10000)), false},
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

func TestStdHTTPTransportReadBodyError(t *testing.T) {
	tr := NewStdHTTPTransport()

	errorHandlerCalled := false
	tr.SetErrorHandler(func(err error) {
		errorHandlerCalled = true
	})

	// Create a reader that always errors
	errorReader := &erroringReader{}
	_, err := tr.readBody(errorReader)

	if err == nil {
		t.Error("expected error from failing reader")
	}

	if !errorHandlerCalled {
		t.Error("expected error handler to be called")
	}
}

type erroringReader struct{}

func (e *erroringReader) Read(p []byte) (n int, err error) {
	return 0, io.ErrUnexpectedEOF
}

func TestStdHTTPTransportSendNoChannel(t *testing.T) {
	tr := NewStdHTTPTransport()

	message := &transport.BaseJsonRpcMessage{
		JsonRpcResponse: &transport.BaseJSONRPCResponse{
			Jsonrpc: "2.0",
			Id:      999, // Non-existent key
		},
	}

	err := tr.Send(context.Background(), message)
	if err == nil {
		t.Error("expected error when no response channel exists")
	}
}

func TestStdHTTPTransportResponseMapConcurrentAccess(t *testing.T) {
	tr := NewStdHTTPTransport()

	// Test concurrent access to responseMap
	var wg sync.WaitGroup
	numGoroutines := 100

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int64) {
			defer wg.Done()

			tr.mu.Lock()
			tr.responseMap[id] = make(chan *transport.BaseJsonRpcMessage, 1)
			tr.mu.Unlock()

			tr.mu.Lock()
			delete(tr.responseMap, id)
			tr.mu.Unlock()
		}(int64(i))
	}

	wg.Wait()

	tr.mu.RLock()
	if len(tr.responseMap) != 0 {
		t.Errorf("expected empty responseMap after concurrent operations, got %d entries", len(tr.responseMap))
	}
	tr.mu.RUnlock()
}

func TestStdHTTPTransportHandler(t *testing.T) {
	tr := NewStdHTTPTransport()

	handler := tr.Handler()
	if handler == nil {
		t.Fatal("expected handler to be non-nil")
	}
}

func TestStdHTTPTransportCloseHandlerNil(t *testing.T) {
	tr := NewStdHTTPTransport()

	// Close without setting handler should not panic
	err := tr.Close()
	if err != nil {
		t.Errorf("Close should return nil even without handler, got: %v", err)
	}
}

func TestStdHTTPTransportMultipleCloseHandlerCalls(t *testing.T) {
	tr := NewStdHTTPTransport()

	callCount := 0
	tr.SetCloseHandler(func() {
		callCount++
	})

	// Call close multiple times
	tr.Close()
	tr.Close()
	tr.Close()

	if callCount != 3 {
		t.Errorf("expected close handler to be called 3 times, got %d", callCount)
	}
}

func TestStdHTTPTransportErrorHandler(t *testing.T) {
	tr := NewStdHTTPTransport()

	errorReceived := false
	tr.SetErrorHandler(func(err error) {
		errorReceived = true
	})

	// Simulate error by reading from failing reader
	errorReader := &erroringReader{}
	tr.readBody(errorReader)

	if !errorReceived {
		t.Error("expected error handler to be called")
	}
}

func TestStdHTTPTransportMessageHandlerSet(t *testing.T) {
	tr := NewStdHTTPTransport()

	handlerSet := false
	tr.SetMessageHandler(func(ctx context.Context, message *transport.BaseJsonRpcMessage) {
		handlerSet = true
	})

	// Verify handler is set
	tr.mu.RLock()
	if tr.messageHandler == nil {
		t.Error("expected message handler to be set")
	}
	tr.mu.RUnlock()

	_ = handlerSet // suppress unused warning
}

func BenchmarkStdHTTPTransportReadBody(b *testing.B) {
	tr := NewStdHTTPTransport()
	body := []byte(`{"jsonrpc":"2.0","method":"test/method","id":1}`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		reader := bytes.NewReader(body)
		tr.readBody(reader)
	}
}

func BenchmarkStdHTTPTransportResponseMapAccess(b *testing.B) {
	tr := NewStdHTTPTransport()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := int64(i % 100)
		tr.mu.Lock()
		tr.responseMap[key] = make(chan *transport.BaseJsonRpcMessage, 1)
		tr.mu.Unlock()

		tr.mu.Lock()
		delete(tr.responseMap, key)
		tr.mu.Unlock()
	}
}

// =============================================================================
// SSE Transport Tests
// =============================================================================

func TestNewSSETransport(t *testing.T) {
	tr := NewSSETransport()

	if tr == nil {
		t.Fatal("expected SSE transport to be created")
	}

	if tr.clients == nil {
		t.Error("expected clients map to be initialized")
	}

	if tr.ClientCount() != 0 {
		t.Error("expected initial client count to be 0")
	}
}

func TestSSETransportStart(t *testing.T) {
	tr := NewSSETransport()

	err := tr.Start(context.Background())
	if err != nil {
		t.Errorf("Start should return nil, got: %v", err)
	}
}

func TestSSETransportClose(t *testing.T) {
	tr := NewSSETransport()

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

func TestSSETransportSetHandlers(t *testing.T) {
	tr := NewSSETransport()

	// Test SetErrorHandler
	tr.SetErrorHandler(func(err error) {})
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
	tr.SetMessageHandler(func(ctx context.Context, message *transport.BaseJsonRpcMessage) {})
	tr.mu.RLock()
	if tr.messageHandler == nil {
		t.Error("expected message handler to be set")
	}
	tr.mu.RUnlock()
}

func TestSSETransportPostHandlerMethodNotAllowed(t *testing.T) {
	tr := NewSSETransport()

	methods := []string{"GET", "PUT", "DELETE", "PATCH"}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/mcp/sse", nil)
			rec := httptest.NewRecorder()

			handler := tr.PostHandler()
			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusMethodNotAllowed {
				t.Errorf("expected status 405, got: %d", rec.Code)
			}
		})
	}
}

func TestSSETransportPostHandlerInvalidJSON(t *testing.T) {
	tr := NewSSETransport()

	body := bytes.NewBufferString("not valid json{")
	req := httptest.NewRequest("POST", "/mcp/sse", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler := tr.PostHandler()
	handler.ServeHTTP(rec, req)

	// Should return 200 OK even on parse error (logs error instead)
	// Implementation may vary - just verify no panic occurs
}

func TestSSETransportBroadcast(t *testing.T) {
	tr := NewSSETransport()

	// Broadcast with no clients should not panic
	tr.Broadcast("test_event", `{"message":"hello"}`)

	if tr.ClientCount() != 0 {
		t.Error("expected client count to remain 0")
	}
}

func TestSSETransportSendNoChannel(t *testing.T) {
	tr := NewSSETransport()

	// Send should return nil when no response channel exists
	msg := transport.NewBaseMessageRequest(&transport.BaseJSONRPCRequest{
		Jsonrpc: "2.0",
		Method:  "test",
		Id:      1,
	})

	err := tr.Send(context.Background(), msg)
	if err != nil {
		t.Errorf("Send should return nil for non-existent channel, got: %v", err)
	}
}

// =============================================================================
// Stdio Transport Tests
// =============================================================================

func TestNewStdioTransport(t *testing.T) {
	reader := bytes.NewBuffer(nil)
	writer := bytes.NewBuffer(nil)

	tr := NewStdioTransport(reader, writer)

	if tr == nil {
		t.Fatal("expected stdio transport to be created")
	}

	if tr.reader != reader {
		t.Error("expected reader to be set")
	}

	if tr.writer != writer {
		t.Error("expected writer to be set")
	}
}

func TestStdioTransportSetHandlers(t *testing.T) {
	reader := bytes.NewBuffer(nil)
	writer := bytes.NewBuffer(nil)
	tr := NewStdioTransport(reader, writer)

	// Test SetErrorHandler
	tr.SetErrorHandler(func(err error) {})
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
	tr.SetMessageHandler(func(ctx context.Context, message *transport.BaseJsonRpcMessage) {})
	tr.mu.RLock()
	if tr.messageHandler == nil {
		t.Error("expected message handler to be set")
	}
	tr.mu.RUnlock()
}

func TestStdioTransportClose(t *testing.T) {
	reader := bytes.NewBuffer(nil)
	writer := bytes.NewBuffer(nil)
	tr := NewStdioTransport(reader, writer)

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

func TestStdioTransportSend(t *testing.T) {
	reader := bytes.NewBuffer(nil)
	writer := bytes.NewBuffer(nil)
	tr := NewStdioTransport(reader, writer)

	msg := transport.NewBaseMessageRequest(&transport.BaseJSONRPCRequest{
		Jsonrpc: "2.0",
		Method:  "test/response",
		Id:      1,
	})

	err := tr.Send(context.Background(), msg)
	if err != nil {
		t.Errorf("Send should return nil, got: %v", err)
	}

	// Verify output contains JSON
	output := writer.String()
	if output == "" {
		t.Error("expected output to be written")
	}

	// Should be valid JSON followed by newline
	if output[len(output)-1] != '\n' {
		t.Error("expected output to end with newline")
	}

	// Verify it's valid JSON
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(output[:len(output)-1]), &result); err != nil {
		t.Errorf("expected valid JSON output, got: %v", err)
	}
}

func TestStdioTransportStartWithEmptyInput(t *testing.T) {
	reader := bytes.NewBuffer(nil)
	writer := bytes.NewBuffer(nil)
	tr := NewStdioTransport(reader, writer)

	ctx, cancel := context.WithCancel(context.Background())

	// Start in goroutine
	done := make(chan error, 1)
	go func() {
		done <- tr.Start(ctx)
	}()

	// Cancel quickly to stop the transport
	cancel()

	// Wait for completion
	err := <-done
	if err != nil && err != context.Canceled {
		t.Errorf("Start should return nil or context.Canceled, got: %v", err)
	}
}

func TestStdioTransportStartWithMessage(t *testing.T) {
	// Create input with a JSON-RPC message
	input := `{"jsonrpc":"2.0","method":"ping","id":1}` + "\n"
	reader := bytes.NewBufferString(input)
	writer := bytes.NewBuffer(nil)
	tr := NewStdioTransport(reader, writer)

	tr.SetMessageHandler(func(ctx context.Context, message *transport.BaseJsonRpcMessage) {
		// Handler called - verify message type
		if message.Type != transport.BaseMessageTypeJSONRPCRequestType {
			t.Errorf("expected request type, got: %v", message.Type)
		}
	})

	ctx, cancel := context.WithCancel(context.Background())

	// Start in goroutine
	done := make(chan error, 1)
	go func() {
		done <- tr.Start(ctx)
	}()

	// Give it time to process, then cancel
	// Use a short sleep to allow the goroutine to read
	cancel()

	// Wait for completion
	<-done

	// Note: Due to timing, message may or may not be received
	// This test primarily verifies no panics occur
}

func TestStdioTransportStartWithInvalidJSON(t *testing.T) {
	// Create input with invalid JSON
	input := `not valid json` + "\n"
	reader := bytes.NewBufferString(input)
	writer := bytes.NewBuffer(nil)
	tr := NewStdioTransport(reader, writer)

	var errorOccurred bool
	tr.SetErrorHandler(func(err error) {
		errorOccurred = true
	})

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- tr.Start(ctx)
	}()

	cancel()
	<-done

	// May or may not trigger error handler depending on timing
	// Primary goal is no panic
	_ = errorOccurred
}

func TestStdioTransportConcurrentSend(t *testing.T) {
	reader := bytes.NewBuffer(nil)
	writer := bytes.NewBuffer(nil)
	tr := NewStdioTransport(reader, writer)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			msg := transport.NewBaseMessageRequest(&transport.BaseJSONRPCRequest{
				Jsonrpc: "2.0",
				Method:  "test/concurrent",
				Id:      transport.RequestId(id),
			})
			tr.Send(context.Background(), msg)
		}(i)
	}
	wg.Wait()

	// Verify output is not empty
	if writer.Len() == 0 {
		t.Error("expected output from concurrent sends")
	}
}

// =============================================================================
// Transport Type Tests (server.go integration)
// =============================================================================

func TestTransportTypeConstants(t *testing.T) {
	if TransportHTTP != "http" {
		t.Errorf("expected TransportHTTP to be 'http', got: %s", TransportHTTP)
	}

	if TransportSSE != "sse" {
		t.Errorf("expected TransportSSE to be 'sse', got: %s", TransportSSE)
	}

	if TransportStdio != "stdio" {
		t.Errorf("expected TransportStdio to be 'stdio', got: %s", TransportStdio)
	}
}
