// Package mcp provides a Model Context Protocol (MCP) server implementation.
// This file contains a custom HTTP transport that integrates with standard net/http handlers.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/metoro-io/mcp-golang/transport"
)

// StdHTTPTransport implements a stateless HTTP transport for MCP that works with standard net/http handlers.
// Unlike the library's built-in HTTPTransport which manages its own server, this transport can be
// integrated with existing HTTP routers like gorilla/mux.
type StdHTTPTransport struct {
	messageHandler func(ctx context.Context, message *transport.BaseJsonRpcMessage)
	errorHandler   func(error)
	closeHandler   func()
	mu             sync.RWMutex
	responseMap    map[int64]chan *transport.BaseJsonRpcMessage
}

// NewStdHTTPTransport creates a new standard HTTP transport.
func NewStdHTTPTransport() *StdHTTPTransport {
	return &StdHTTPTransport{
		responseMap: make(map[int64]chan *transport.BaseJsonRpcMessage),
	}
}

// Start implements Transport.Start - no-op for this transport as it's handled by the router.
func (t *StdHTTPTransport) Start(ctx context.Context) error {
	return nil
}

// Send implements Transport.Send to send response messages.
func (t *StdHTTPTransport) Send(ctx context.Context, message *transport.BaseJsonRpcMessage) error {
	key := message.JsonRpcResponse.Id
	t.mu.RLock()
	responseChannel := t.responseMap[int64(key)]
	t.mu.RUnlock()

	if responseChannel == nil {
		return fmt.Errorf("no response channel found for key: %d", key)
	}
	responseChannel <- message
	return nil
}

// Close implements Transport.Close.
func (t *StdHTTPTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closeHandler != nil {
		t.closeHandler()
	}
	return nil
}

// SetCloseHandler implements Transport.SetCloseHandler.
func (t *StdHTTPTransport) SetCloseHandler(handler func()) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.closeHandler = handler
}

// SetErrorHandler implements Transport.SetErrorHandler.
func (t *StdHTTPTransport) SetErrorHandler(handler func(error)) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.errorHandler = handler
}

// SetMessageHandler implements Transport.SetMessageHandler.
func (t *StdHTTPTransport) SetMessageHandler(handler func(ctx context.Context, message *transport.BaseJsonRpcMessage)) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.messageHandler = handler
}

// Handler returns an http.HandlerFunc that can be used with any standard net/http router.
func (t *StdHTTPTransport) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Only POST method is supported", http.StatusMethodNotAllowed)
			return
		}

		ctx := r.Context()
		body, err := t.readBody(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		response, err := t.handleMessage(ctx, body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		jsonData, err := json.Marshal(response)
		if err != nil {
			t.mu.RLock()
			errHandler := t.errorHandler
			t.mu.RUnlock()
			if errHandler != nil {
				errHandler(fmt.Errorf("failed to marshal response: %w", err))
			}
			http.Error(w, "Failed to marshal response", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(jsonData)
	}
}

// handleMessage processes an incoming message and returns a response.
func (t *StdHTTPTransport) handleMessage(ctx context.Context, body []byte) (*transport.BaseJsonRpcMessage, error) {
	// Allocate a unique key for this request
	t.mu.Lock()
	var key int64 = 0
	for key < 1000000 {
		if _, ok := t.responseMap[key]; !ok {
			break
		}
		key = key + 1
	}
	t.responseMap[key] = make(chan *transport.BaseJsonRpcMessage, 1)
	t.mu.Unlock()

	var prevId *transport.RequestId = nil
	deserialized := false

	// Try to unmarshal as a request first
	var request transport.BaseJSONRPCRequest
	if err := json.Unmarshal(body, &request); err == nil && request.Method != "" {
		deserialized = true
		id := request.Id
		prevId = &id
		request.Id = transport.RequestId(key)

		t.mu.RLock()
		handler := t.messageHandler
		t.mu.RUnlock()

		if handler != nil {
			handler(ctx, transport.NewBaseMessageRequest(&request))
		}
	}

	// Try as a notification
	if !deserialized {
		var notification transport.BaseJSONRPCNotification
		if err := json.Unmarshal(body, &notification); err == nil && notification.Method != "" {
			deserialized = true
			t.mu.RLock()
			handler := t.messageHandler
			t.mu.RUnlock()

			if handler != nil {
				handler(ctx, transport.NewBaseMessageNotification(&notification))
			}
		}
	}

	// Try as a response
	if !deserialized {
		var response transport.BaseJSONRPCResponse
		if err := json.Unmarshal(body, &response); err == nil {
			deserialized = true
			t.mu.RLock()
			handler := t.messageHandler
			t.mu.RUnlock()

			if handler != nil {
				handler(ctx, transport.NewBaseMessageResponse(&response))
			}
		}
	}

	// Try as an error
	if !deserialized {
		var errorResponse transport.BaseJSONRPCError
		if err := json.Unmarshal(body, &errorResponse); err == nil {
			deserialized = true
			t.mu.RLock()
			handler := t.messageHandler
			t.mu.RUnlock()

			if handler != nil {
				handler(ctx, transport.NewBaseMessageError(&errorResponse))
			}
		}
	}

	if !deserialized {
		return nil, fmt.Errorf("failed to deserialize message")
	}

	// Block until the response is received
	t.mu.RLock()
	responseChan := t.responseMap[key]
	t.mu.RUnlock()

	responseToUse := <-responseChan

	t.mu.Lock()
	delete(t.responseMap, key)
	t.mu.Unlock()

	if prevId != nil {
		responseToUse.JsonRpcResponse.Id = *prevId
	}

	return responseToUse, nil
}

// readBody reads and returns the body from an io.Reader.
func (t *StdHTTPTransport) readBody(reader io.Reader) ([]byte, error) {
	body, err := io.ReadAll(reader)
	if err != nil {
		t.mu.RLock()
		errHandler := t.errorHandler
		t.mu.RUnlock()
		if errHandler != nil {
			errHandler(fmt.Errorf("failed to read request body: %w", err))
		}
		return nil, fmt.Errorf("failed to read request body: %w", err)
	}
	return body, nil
}

// =============================================================================
// SSE Transport - Server-Sent Events for real-time notifications
// =============================================================================

// SSEClient represents a connected SSE client.
type SSEClient struct {
	id       string
	messages chan []byte
	done     chan struct{}
}

// SSETransport implements a Server-Sent Events transport for MCP.
// This is ideal for real-time server notifications and one-way communication.
type SSETransport struct {
	clients        map[string]*SSEClient
	messageHandler func(ctx context.Context, message *transport.BaseJsonRpcMessage)
	errorHandler   func(error)
	closeHandler   func()
	mu             sync.RWMutex
	responseMap    map[int64]chan *transport.BaseJsonRpcMessage
}

// NewSSETransport creates a new SSE transport.
func NewSSETransport() *SSETransport {
	return &SSETransport{
		clients:     make(map[string]*SSEClient),
		responseMap: make(map[int64]chan *transport.BaseJsonRpcMessage),
	}
}

// Start implements Transport.Start.
func (t *SSETransport) Start(ctx context.Context) error {
	return nil
}

// Send implements Transport.Send to broadcast messages to all SSE clients.
func (t *SSETransport) Send(ctx context.Context, message *transport.BaseJsonRpcMessage) error {
	// If it's a response to a request, send to the response channel
	if message.JsonRpcResponse != nil {
		key := message.JsonRpcResponse.Id
		t.mu.RLock()
		responseChannel := t.responseMap[int64(key)]
		t.mu.RUnlock()

		if responseChannel != nil {
			responseChannel <- message
			return nil
		}
	}

	// Otherwise, broadcast to all SSE clients
	jsonData, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	t.mu.RLock()
	defer t.mu.RUnlock()

	for _, client := range t.clients {
		select {
		case client.messages <- jsonData:
		default:
			// Client buffer full, skip
		}
	}

	return nil
}

// Close implements Transport.Close.
func (t *SSETransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Close all client connections
	for _, client := range t.clients {
		close(client.done)
	}
	t.clients = make(map[string]*SSEClient)

	if t.closeHandler != nil {
		t.closeHandler()
	}
	return nil
}

// SetCloseHandler implements Transport.SetCloseHandler.
func (t *SSETransport) SetCloseHandler(handler func()) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.closeHandler = handler
}

// SetErrorHandler implements Transport.SetErrorHandler.
func (t *SSETransport) SetErrorHandler(handler func(error)) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.errorHandler = handler
}

// SetMessageHandler implements Transport.SetMessageHandler.
func (t *SSETransport) SetMessageHandler(handler func(ctx context.Context, message *transport.BaseJsonRpcMessage)) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.messageHandler = handler
}

// SSEHandler returns an http.HandlerFunc for SSE connections.
// Clients connect here to receive server-sent events.
func (t *SSETransport) SSEHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Set SSE headers
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		// Create flush support
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "SSE not supported", http.StatusInternalServerError)
			return
		}

		// Create client
		clientID := fmt.Sprintf("%d", time.Now().UnixNano())
		client := &SSEClient{
			id:       clientID,
			messages: make(chan []byte, 100),
			done:     make(chan struct{}),
		}

		// Register client
		t.mu.Lock()
		t.clients[clientID] = client
		t.mu.Unlock()

		// Cleanup on disconnect
		defer func() {
			t.mu.Lock()
			delete(t.clients, clientID)
			t.mu.Unlock()
		}()

		// Build the message endpoint URL from the request
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		// Check for forwarded headers
		if fwdProto := r.Header.Get("X-Forwarded-Proto"); fwdProto != "" {
			scheme = fwdProto
		}
		messageEndpoint := fmt.Sprintf("%s://%s/mcp/sse?clientId=%s", scheme, r.Host, clientID)

		// Send initial endpoint event (MCP SSE protocol requires this)
		_, _ = fmt.Fprintf(w, "event: endpoint\ndata: %s\n\n", messageEndpoint) //nolint:errcheck
		flusher.Flush()

		// Stream events
		for {
			select {
			case <-r.Context().Done():
				return
			case <-client.done:
				return
			case msg := <-client.messages:
				_, _ = fmt.Fprintf(w, "event: message\ndata: %s\n\n", msg) //nolint:errcheck
				flusher.Flush()
			}
		}
	}
}

// PostHandler returns an http.HandlerFunc for POST requests to the SSE endpoint.
// This allows clients to send messages while connected via SSE.
func (t *SSETransport) PostHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Handle CORS preflight
		if r.Method == http.MethodOptions {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.WriteHeader(http.StatusOK)
			return
		}

		if r.Method != http.MethodPost {
			http.Error(w, "Only POST method is supported", http.StatusMethodNotAllowed)
			return
		}

		// Get clientId from query params to route response to correct SSE client
		clientID := r.URL.Query().Get("clientId")

		ctx := r.Context()
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read request body", http.StatusBadRequest)
			return
		}

		response, err := t.handleMessage(ctx, body, clientID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		jsonData, err := json.Marshal(response)
		if err != nil {
			http.Error(w, "Failed to marshal response", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		_, _ = w.Write(jsonData)
	}
}

// handleMessage processes an incoming message (shared with HTTP transport logic).
func (t *SSETransport) handleMessage(ctx context.Context, body []byte, _ string) (*transport.BaseJsonRpcMessage, error) {
	t.mu.Lock()
	var key int64 = 0
	for key < 1000000 {
		if _, ok := t.responseMap[key]; !ok {
			break
		}
		key = key + 1
	}
	t.responseMap[key] = make(chan *transport.BaseJsonRpcMessage, 1)
	t.mu.Unlock()

	var prevId *transport.RequestId = nil
	deserialized := false

	var request transport.BaseJSONRPCRequest
	if err := json.Unmarshal(body, &request); err == nil && request.Method != "" {
		deserialized = true
		id := request.Id
		prevId = &id
		request.Id = transport.RequestId(key)

		t.mu.RLock()
		handler := t.messageHandler
		t.mu.RUnlock()

		if handler != nil {
			handler(ctx, transport.NewBaseMessageRequest(&request))
		}
	}

	if !deserialized {
		return nil, fmt.Errorf("failed to deserialize message")
	}

	t.mu.RLock()
	responseChan := t.responseMap[key]
	t.mu.RUnlock()

	responseToUse := <-responseChan

	t.mu.Lock()
	delete(t.responseMap, key)
	t.mu.Unlock()

	if prevId != nil {
		responseToUse.JsonRpcResponse.Id = *prevId
	}

	return responseToUse, nil
}

// Broadcast sends a notification to all connected SSE clients.
func (t *SSETransport) Broadcast(event string, data interface{}) error {
	jsonData, err := json.Marshal(map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  event,
		"params":  data,
	})
	if err != nil {
		return fmt.Errorf("failed to marshal broadcast: %w", err)
	}

	t.mu.RLock()
	defer t.mu.RUnlock()

	for _, client := range t.clients {
		select {
		case client.messages <- jsonData:
		default:
			// Client buffer full, skip
		}
	}

	return nil
}

// ClientCount returns the number of connected SSE clients.
func (t *SSETransport) ClientCount() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.clients)
}

// =============================================================================
// Stdio Transport - For local AI client integrations (Claude Desktop, etc.)
// =============================================================================

// StdioTransport implements a stdio-based transport for MCP.
// This is used for local AI client integrations like Claude Desktop.
type StdioTransport struct {
	reader         io.Reader
	writer         io.Writer
	messageHandler func(ctx context.Context, message *transport.BaseJsonRpcMessage)
	errorHandler   func(error)
	closeHandler   func()
	mu             sync.RWMutex
	closed         bool
}

// NewStdioTransport creates a new stdio transport with custom reader/writer.
func NewStdioTransport(reader io.Reader, writer io.Writer) *StdioTransport {
	return &StdioTransport{
		reader: reader,
		writer: writer,
	}
}

// Start implements Transport.Start - begins reading from stdin.
func (t *StdioTransport) Start(ctx context.Context) error {
	go t.readLoop(ctx)
	return nil
}

// readLoop continuously reads from stdin and processes messages.
func (t *StdioTransport) readLoop(ctx context.Context) {
	decoder := json.NewDecoder(t.reader)

	for {
		select {
		case <-ctx.Done():
			return
		default:
			var rawMsg json.RawMessage
			if err := decoder.Decode(&rawMsg); err != nil {
				if err == io.EOF {
					return
				}
				t.mu.RLock()
				errHandler := t.errorHandler
				t.mu.RUnlock()
				if errHandler != nil {
					errHandler(fmt.Errorf("failed to decode message: %w", err))
				}
				continue
			}

			t.processMessage(ctx, rawMsg)
		}
	}
}

// processMessage handles an incoming JSON-RPC message.
func (t *StdioTransport) processMessage(ctx context.Context, rawMsg json.RawMessage) {
	t.mu.RLock()
	handler := t.messageHandler
	t.mu.RUnlock()

	if handler == nil {
		return
	}

	// Try as request
	var request transport.BaseJSONRPCRequest
	if err := json.Unmarshal(rawMsg, &request); err == nil && request.Method != "" {
		handler(ctx, transport.NewBaseMessageRequest(&request))
		return
	}

	// Try as notification
	var notification transport.BaseJSONRPCNotification
	if err := json.Unmarshal(rawMsg, &notification); err == nil && notification.Method != "" {
		handler(ctx, transport.NewBaseMessageNotification(&notification))
		return
	}

	// Try as response
	var response transport.BaseJSONRPCResponse
	if err := json.Unmarshal(rawMsg, &response); err == nil {
		handler(ctx, transport.NewBaseMessageResponse(&response))
		return
	}

	// Try as error
	var errorResp transport.BaseJSONRPCError
	if err := json.Unmarshal(rawMsg, &errorResp); err == nil {
		handler(ctx, transport.NewBaseMessageError(&errorResp))
		return
	}
}

// Send implements Transport.Send - writes messages to stdout.
func (t *StdioTransport) Send(ctx context.Context, message *transport.BaseJsonRpcMessage) error {
	t.mu.RLock()
	closed := t.closed
	t.mu.RUnlock()

	if closed {
		return fmt.Errorf("transport is closed")
	}

	jsonData, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	// Write JSON followed by newline (JSON Lines format)
	if _, err := t.writer.Write(jsonData); err != nil {
		return fmt.Errorf("failed to write message: %w", err)
	}
	if _, err := t.writer.Write([]byte("\n")); err != nil {
		return fmt.Errorf("failed to write newline: %w", err)
	}

	return nil
}

// Close implements Transport.Close.
func (t *StdioTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.closed = true

	if t.closeHandler != nil {
		t.closeHandler()
	}

	return nil
}

// SetCloseHandler implements Transport.SetCloseHandler.
func (t *StdioTransport) SetCloseHandler(handler func()) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.closeHandler = handler
}

// SetErrorHandler implements Transport.SetErrorHandler.
func (t *StdioTransport) SetErrorHandler(handler func(error)) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.errorHandler = handler
}

// SetMessageHandler implements Transport.SetMessageHandler.
func (t *StdioTransport) SetMessageHandler(handler func(ctx context.Context, message *transport.BaseJsonRpcMessage)) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.messageHandler = handler
}
