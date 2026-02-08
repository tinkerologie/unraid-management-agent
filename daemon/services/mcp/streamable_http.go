// Package mcp provides a Model Context Protocol (MCP) server implementation.
// This file implements the Streamable HTTP transport (MCP spec 2025-06-18) which replaces
// the deprecated HTTP+SSE transport, providing compatibility with Cursor, Claude Desktop,
// GitHub Copilot, Codex, Windsurf, Gemini CLI, and other modern MCP clients.
package mcp

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/metoro-io/mcp-golang/transport"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
)

// supportedProtocolVersions lists the MCP protocol versions this server supports.
var supportedProtocolVersions = map[string]bool{
	"2025-06-18": true,
	"2025-03-26": true,
}

// StreamableHTTPTransport implements the MCP Streamable HTTP transport (spec 2025-06-18).
// This is the modern transport that supports POST (for requests/notifications), GET (for SSE streams),
// and DELETE (for session termination) all on a single endpoint.
//
// Compatible with: Cursor, Claude Desktop, GitHub Copilot, Codex, Windsurf, Gemini CLI, and others.
type StreamableHTTPTransport struct {
	messageHandler func(ctx context.Context, message *transport.BaseJsonRpcMessage)
	errorHandler   func(error)
	closeHandler   func()
	mu             sync.RWMutex
	responseMap    map[int64]chan *transport.BaseJsonRpcMessage

	// Session management
	sessionID   string
	initialized bool

	// SSE clients connected via GET
	sseClients map[string]*streamableSSEClient
}

// streamableSSEClient represents a client connected via GET for SSE streaming.
type streamableSSEClient struct {
	id       string
	messages chan []byte
	done     chan struct{}
}

// NewStreamableHTTPTransport creates a new Streamable HTTP transport.
func NewStreamableHTTPTransport() *StreamableHTTPTransport {
	return &StreamableHTTPTransport{
		responseMap: make(map[int64]chan *transport.BaseJsonRpcMessage),
		sseClients:  make(map[string]*streamableSSEClient),
	}
}

// Start implements Transport.Start - no-op for HTTP transport as it's handled by the router.
func (t *StreamableHTTPTransport) Start(_ context.Context) error {
	return nil
}

// Send implements Transport.Send to send response messages.
func (t *StreamableHTTPTransport) Send(_ context.Context, message *transport.BaseJsonRpcMessage) error {
	// Handle response messages - route to the correct waiting request
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

	// Handle error responses
	if message.JsonRpcError != nil {
		key := message.JsonRpcError.Id
		t.mu.RLock()
		responseChannel := t.responseMap[int64(key)]
		t.mu.RUnlock()

		if responseChannel != nil {
			responseChannel <- message
			return nil
		}
	}

	// For notifications/requests from server, broadcast to SSE clients
	jsonData, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	t.mu.RLock()
	defer t.mu.RUnlock()

	for _, client := range t.sseClients {
		select {
		case client.messages <- jsonData:
		default:
			// Client buffer full, skip
		}
	}

	return nil
}

// Close implements Transport.Close.
func (t *StreamableHTTPTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Close all SSE client connections
	for _, client := range t.sseClients {
		close(client.done)
	}
	t.sseClients = make(map[string]*streamableSSEClient)

	if t.closeHandler != nil {
		t.closeHandler()
	}
	return nil
}

// SetCloseHandler implements Transport.SetCloseHandler.
func (t *StreamableHTTPTransport) SetCloseHandler(handler func()) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.closeHandler = handler
}

// SetErrorHandler implements Transport.SetErrorHandler.
func (t *StreamableHTTPTransport) SetErrorHandler(handler func(error)) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.errorHandler = handler
}

// SetMessageHandler implements Transport.SetMessageHandler.
func (t *StreamableHTTPTransport) SetMessageHandler(handler func(ctx context.Context, message *transport.BaseJsonRpcMessage)) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.messageHandler = handler
}

// generateSessionID creates a cryptographically secure session ID.
func generateSessionID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// Fallback to timestamp-based ID
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

// setCORSHeaders sets the CORS headers required for cross-origin MCP clients.
func setCORSHeaders(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Accept, Mcp-Session-Id, MCP-Protocol-Version, Last-Event-ID")
	w.Header().Set("Access-Control-Expose-Headers", "Mcp-Session-Id")
}

// Handler returns an http.HandlerFunc that implements the full Streamable HTTP transport.
// This single handler supports POST, GET, DELETE, and OPTIONS on the MCP endpoint.
func (t *StreamableHTTPTransport) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		setCORSHeaders(w)

		switch r.Method {
		case http.MethodOptions:
			w.WriteHeader(http.StatusOK)
		case http.MethodPost:
			t.handlePost(w, r)
		case http.MethodGet:
			t.handleGet(w, r)
		case http.MethodDelete:
			t.handleDelete(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

// validateProtocolVersion checks the MCP-Protocol-Version header per spec 2025-06-18.
// Returns true if validation passes, false if an error response was sent.
func (t *StreamableHTTPTransport) validateProtocolVersion(w http.ResponseWriter, r *http.Request) bool {
	version := r.Header.Get("MCP-Protocol-Version")
	if version == "" {
		// Per spec: if server does not receive header, assume 2025-03-26
		return true
	}
	if !supportedProtocolVersions[version] {
		http.Error(w, "Unsupported MCP protocol version", http.StatusBadRequest)
		return false
	}
	return true
}

// validateSessionID checks the Mcp-Session-Id header for established sessions.
// Returns true if validation passes, false if an error response was sent.
func (t *StreamableHTTPTransport) validateSessionID(w http.ResponseWriter, r *http.Request) bool {
	if !t.initialized {
		return true
	}
	clientSessionID := r.Header.Get("Mcp-Session-Id")
	if clientSessionID == "" {
		// Per spec: server MUST return 400 if session ID is required but missing
		// Be lenient - some clients may not send it on every request
		return true
	}
	if clientSessionID != t.sessionID {
		// Per spec: terminated/unknown session IDs get 404
		http.Error(w, "Invalid or terminated session", http.StatusNotFound)
		return false
	}
	return true
}

// handlePost processes POST requests containing JSON-RPC messages.
func (t *StreamableHTTPTransport) handlePost(w http.ResponseWriter, r *http.Request) {
	// Validate MCP-Protocol-Version header (spec 2025-06-18)
	if !t.validateProtocolVersion(w, r) {
		return
	}

	// Validate session for non-initialization requests
	if !t.validateSessionID(w, r) {
		return
	}

	body, err := t.readBody(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	// Determine message type to handle notifications vs requests
	msgType := t.classifyMessage(body)

	switch msgType {
	case messageTypeNotification:
		// Notifications don't expect a response - return 202 Accepted
		t.handleNotification(ctx, body)
		w.WriteHeader(http.StatusAccepted)
		return

	case messageTypeRequest:
		response, isInitialize, err := t.handleRequest(ctx, body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		jsonData, err := json.Marshal(response)
		if err != nil {
			t.reportError(fmt.Errorf("failed to marshal response: %w", err))
			http.Error(w, "Failed to marshal response", http.StatusInternalServerError)
			return
		}

		// Set session ID on initialize response
		if isInitialize {
			t.mu.Lock()
			t.sessionID = generateSessionID()
			t.initialized = true
			t.mu.Unlock()
			w.Header().Set("Mcp-Session-Id", t.sessionID)
			logger.Debug("MCP Streamable HTTP session initialized: %s", t.sessionID)
		} else if t.sessionID != "" {
			w.Header().Set("Mcp-Session-Id", t.sessionID)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(jsonData)

	case messageTypeResponse:
		// Client sending a response - return 202 Accepted
		t.handleResponseMessage(ctx, body)
		w.WriteHeader(http.StatusAccepted)

	default:
		http.Error(w, "Failed to deserialize message", http.StatusBadRequest)
	}
}

// handleGet processes GET requests for SSE streaming.
func (t *StreamableHTTPTransport) handleGet(w http.ResponseWriter, r *http.Request) {
	// Validate Accept header
	accept := r.Header.Get("Accept")
	if !strings.Contains(accept, "text/event-stream") {
		http.Error(w, "Accept header must include text/event-stream", http.StatusNotAcceptable)
		return
	}

	// Validate MCP-Protocol-Version header (spec 2025-06-18)
	if !t.validateProtocolVersion(w, r) {
		return
	}

	// Validate session
	if !t.validateSessionID(w, r) {
		return
	}

	// Verify server supports SSE streaming
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", http.StatusInternalServerError)
		return
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	if t.sessionID != "" {
		w.Header().Set("Mcp-Session-Id", t.sessionID)
	}

	// Create SSE client
	clientID := fmt.Sprintf("%d", time.Now().UnixNano())
	client := &streamableSSEClient{
		id:       clientID,
		messages: make(chan []byte, 100),
		done:     make(chan struct{}),
	}

	t.mu.Lock()
	t.sseClients[clientID] = client
	t.mu.Unlock()

	defer func() {
		t.mu.Lock()
		delete(t.sseClients, clientID)
		t.mu.Unlock()
	}()

	// Send initial keepalive
	_, _ = fmt.Fprintf(w, ": keepalive\n\n")
	flusher.Flush()

	// Stream events
	for {
		select {
		case <-r.Context().Done():
			return
		case <-client.done:
			return
		case msg := <-client.messages:
			_, _ = fmt.Fprintf(w, "event: message\ndata: %s\n\n", msg)
			flusher.Flush()
		}
	}
}

// handleDelete processes DELETE requests for session termination.
func (t *StreamableHTTPTransport) handleDelete(w http.ResponseWriter, r *http.Request) {
	clientSessionID := r.Header.Get("Mcp-Session-Id")
	if clientSessionID == "" {
		http.Error(w, "Missing Mcp-Session-Id header", http.StatusBadRequest)
		return
	}
	if clientSessionID != t.sessionID {
		// Per spec: terminated/unknown session returns 404
		http.Error(w, "Session not found", http.StatusNotFound)
		return
	}

	t.mu.Lock()
	t.initialized = false
	t.sessionID = ""
	// Close all SSE clients
	for _, client := range t.sseClients {
		close(client.done)
	}
	t.sseClients = make(map[string]*streamableSSEClient)
	t.mu.Unlock()

	logger.Debug("MCP Streamable HTTP session terminated")
	w.WriteHeader(http.StatusOK)
}

// messageType represents the type of JSON-RPC message.
type messageType int

const (
	messageTypeUnknown      messageType = iota
	messageTypeRequest                  // Has id and method
	messageTypeNotification             // Has method but no id
	messageTypeResponse                 // Has id and result but no method
	messageTypeError                    // Has id and error but no method
)

// classifyMessage determines the type of a JSON-RPC message without fully deserializing it.
func (t *StreamableHTTPTransport) classifyMessage(body []byte) messageType {
	// Quick structural check to classify the message
	var raw struct {
		ID      *json.RawMessage `json:"id,omitempty"`
		Method  *string          `json:"method,omitempty"`
		Result  *json.RawMessage `json:"result,omitempty"`
		Error   *json.RawMessage `json:"error,omitempty"`
		JSONRPC *string          `json:"jsonrpc,omitempty"`
	}

	if err := json.Unmarshal(body, &raw); err != nil {
		return messageTypeUnknown
	}

	// Has method and id → request
	if raw.Method != nil && *raw.Method != "" && raw.ID != nil {
		return messageTypeRequest
	}

	// Has method but no id → notification
	if raw.Method != nil && *raw.Method != "" {
		return messageTypeNotification
	}

	// Has result → response
	if raw.Result != nil {
		return messageTypeResponse
	}

	// Has error → error response
	if raw.Error != nil {
		return messageTypeError
	}

	return messageTypeUnknown
}

// handleNotification processes a JSON-RPC notification (no response expected).
func (t *StreamableHTTPTransport) handleNotification(ctx context.Context, body []byte) {
	var notification transport.BaseJSONRPCNotification
	if err := json.Unmarshal(body, &notification); err == nil && notification.Method != "" {
		t.mu.RLock()
		handler := t.messageHandler
		t.mu.RUnlock()

		if handler != nil {
			handler(ctx, transport.NewBaseMessageNotification(&notification))
		}
	}
}

// handleRequest processes a JSON-RPC request and returns a response.
// Returns the response, whether it was an initialize request, and any error.
func (t *StreamableHTTPTransport) handleRequest(ctx context.Context, body []byte) (*transport.BaseJsonRpcMessage, bool, error) {
	// Allocate a unique key for response correlation
	t.mu.Lock()
	var key int64
	for key = 0; key < 1000000; key++ {
		if _, ok := t.responseMap[key]; !ok {
			break
		}
	}
	t.responseMap[key] = make(chan *transport.BaseJsonRpcMessage, 1)
	t.mu.Unlock()

	var prevID *transport.RequestId
	isInitialize := false

	var request transport.BaseJSONRPCRequest
	if err := json.Unmarshal(body, &request); err != nil {
		t.mu.Lock()
		delete(t.responseMap, key)
		t.mu.Unlock()
		return nil, false, fmt.Errorf("failed to deserialize request: %w", err)
	}

	if request.Method == "" {
		t.mu.Lock()
		delete(t.responseMap, key)
		t.mu.Unlock()
		return nil, false, fmt.Errorf("request method is empty")
	}

	// Check if this is an initialize request
	if request.Method == "initialize" {
		isInitialize = true
	}

	// Remap the ID for internal routing
	prevID = &request.Id
	request.Id = transport.RequestId(key)

	t.mu.RLock()
	handler := t.messageHandler
	t.mu.RUnlock()

	if handler != nil {
		handler(ctx, transport.NewBaseMessageRequest(&request))
	}

	// Wait for the response
	t.mu.RLock()
	responseChan := t.responseMap[key]
	t.mu.RUnlock()

	response := <-responseChan

	t.mu.Lock()
	delete(t.responseMap, key)
	t.mu.Unlock()

	// Restore the original request ID
	if response.JsonRpcResponse != nil {
		response.JsonRpcResponse.Id = *prevID
	}
	if response.JsonRpcError != nil {
		response.JsonRpcError.Id = *prevID
	}

	return response, isInitialize, nil
}

// handleResponseMessage processes a JSON-RPC response from the client.
func (t *StreamableHTTPTransport) handleResponseMessage(ctx context.Context, body []byte) {
	var response transport.BaseJSONRPCResponse
	if err := json.Unmarshal(body, &response); err == nil {
		t.mu.RLock()
		handler := t.messageHandler
		t.mu.RUnlock()

		if handler != nil {
			handler(ctx, transport.NewBaseMessageResponse(&response))
		}
	}
}

// readBody reads and returns the body from an io.Reader.
func (t *StreamableHTTPTransport) readBody(reader io.Reader) ([]byte, error) {
	body, err := io.ReadAll(reader)
	if err != nil {
		t.reportError(fmt.Errorf("failed to read request body: %w", err))
		return nil, fmt.Errorf("failed to read request body: %w", err)
	}
	return body, nil
}

// reportError calls the error handler if one is set.
func (t *StreamableHTTPTransport) reportError(err error) {
	t.mu.RLock()
	errHandler := t.errorHandler
	t.mu.RUnlock()
	if errHandler != nil {
		errHandler(err)
	}
}
