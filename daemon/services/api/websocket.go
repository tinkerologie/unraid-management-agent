package api

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/constants"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(_ *http.Request) bool {
		return true // Allow all origins
	},
}

// broadcastMessage carries an event with its topic name through the broadcast channel.
type broadcastMessage struct {
	Topic string
	Data  any
}

// WSHub manages WebSocket client connections and broadcasts messages to all connected clients.
// It handles client registration, unregistration, and message broadcasting in a thread-safe manner.
type WSHub struct {
	clients    map[*WSClient]bool
	broadcast  chan broadcastMessage
	register   chan *WSClient
	unregister chan *WSClient
	mu         sync.RWMutex
}

// WSClient represents a single WebSocket client connection.
// It maintains the connection to the hub, the WebSocket connection, and a send channel for outgoing messages.
type WSClient struct {
	hub    *WSHub
	conn   *websocket.Conn
	send   chan dto.WSEvent
	topics map[string]bool // nil = all topics; non-nil = only matching topics
	topMu  sync.RWMutex
}

// NewWSHub creates and initializes a new WebSocket hub.
// The hub is ready to accept client connections and broadcast messages.
func NewWSHub() *WSHub {
	return &WSHub{
		clients:    make(map[*WSClient]bool),
		broadcast:  make(chan broadcastMessage, constants.WSBufferSize),
		register:   make(chan *WSClient),
		unregister: make(chan *WSClient),
	}
}

// Run starts the WebSocket hub's main event loop.
// It handles client registration, unregistration, and message broadcasting until the context is cancelled.
func (h *WSHub) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			logger.Info("WebSocket hub stopping due to context cancellation")
			// Close all client connections
			h.mu.Lock()
			for client := range h.clients {
				close(client.send)
				delete(h.clients, client)
			}
			h.mu.Unlock()
			return

		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()
			logger.Debug("WebSocket client connected")

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
				logger.Debug("WebSocket client disconnected")
			}
			h.mu.Unlock()

		case msg := <-h.broadcast:
			h.mu.RLock()
			event := dto.WSEvent{
				Event:     msg.Topic,
				Timestamp: time.Now(),
				Data:      msg.Data,
			}
			for client := range h.clients {
				if !client.wantsTopic(msg.Topic) {
					continue
				}
				select {
				case client.send <- event:
				default:
					close(client.send)
					delete(h.clients, client)
				}
			}
			h.mu.RUnlock()
		}
	}
}

// wantsTopic returns true if the client is subscribed to the given topic.
// A nil topics map means the client receives all topics (default).
func (c *WSClient) wantsTopic(topic string) bool {
	c.topMu.RLock()
	defer c.topMu.RUnlock()
	if c.topics == nil {
		return true
	}
	return c.topics[topic]
}

// setTopics updates the client's topic filter.
// A nil or empty list resets to receiving all topics.
func (c *WSClient) setTopics(topics []string) {
	c.topMu.Lock()
	defer c.topMu.Unlock()
	if len(topics) == 0 {
		c.topics = nil
		return
	}
	c.topics = make(map[string]bool, len(topics))
	for _, t := range topics {
		c.topics[t] = true
	}
}

// Broadcast sends a message to all connected WebSocket clients matching the topic filter.
func (h *WSHub) Broadcast(topic string, data any) {
	h.broadcast <- broadcastMessage{Topic: topic, Data: data}
}

// handleWebSocket godoc
//
//	@Summary		WebSocket connection
//	@Description	Establish a WebSocket connection for real-time system updates
//	@Description
//	@Description	**Connection:** `ws://localhost:8043/api/v1/ws`
//	@Description
//	@Description	**Event Format:**
//	@Description	```json
//	@Description	{
//	@Description	  "event": "update",
//	@Description	  "timestamp": "2025-01-01T00:00:00Z",
//	@Description	  "data": { ... }
//	@Description	}
//	@Description	```
//	@Description
//	@Description	**Supported Events:**
//	@Description	- system_update: System metrics (CPU, RAM, temps)
//	@Description	- array_status_update: Array status changes
//	@Description	- disk_list_update: Disk information updates
//	@Description	- container_list_update: Docker container updates
//	@Description	- vm_list_update: VM status updates
//	@Description	- ups_status_update: UPS status updates
//	@Description	- gpu_metrics_update: GPU metrics updates
//	@Description	- network_list_update: Network interface updates
//	@Description	- hardware_update: Hardware information updates
//	@Description	- notifications_update: Notification updates
//	@Description	- zfs_pools_update: ZFS pool updates
//	@Tags			WebSocket
//	@Router			/ws [get]
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.Error("WebSocket upgrade error: %v", err)
		return
	}

	client := &WSClient{
		hub:  s.wsHub,
		conn: conn,
		send: make(chan dto.WSEvent, constants.WSBufferSize),
	}

	client.hub.register <- client

	go client.writePump()
	go client.readPump()
}

func (c *WSClient) writePump() {
	ticker := time.NewTicker(time.Duration(constants.WSPingInterval) * time.Second)
	defer func() {
		ticker.Stop()
		if err := c.conn.Close(); err != nil {
			logger.Debug("Error closing WebSocket connection in writePump: %v", err)
		}
	}()

	for {
		select {
		case event, ok := <-c.send:
			if !ok {
				// Channel closed, send close message
				if err := c.conn.WriteMessage(websocket.CloseMessage, []byte{}); err != nil {
					logger.Debug("Error writing close message: %v", err)
				}
				return
			}

			if err := c.conn.WriteJSON(event); err != nil {
				return
			}

		case <-ticker.C:
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (c *WSClient) readPump() {
	defer func() {
		c.hub.unregister <- c
		if err := c.conn.Close(); err != nil {
			logger.Debug("Error closing WebSocket connection in readPump: %v", err)
		}
	}()

	if err := c.conn.SetReadDeadline(time.Now().Add(60 * time.Second)); err != nil {
		logger.Warning("Error setting initial read deadline: %v", err)
		return
	}
	c.conn.SetPongHandler(func(string) error {
		if err := c.conn.SetReadDeadline(time.Now().Add(60 * time.Second)); err != nil {
			logger.Debug("Error setting read deadline in pong handler: %v", err)
		}
		return nil
	})

	for {
		_, raw, err := c.conn.ReadMessage()
		if err != nil {
			break
		}
		// Try to parse subscribe message. Use a raw map to distinguish between
		// "subscribe" key absent vs. explicitly set to null (both unmarshal to
		// nil []string, but only the latter should reset topics to "all").
		var envelope map[string]json.RawMessage
		if json.Unmarshal(raw, &envelope) != nil {
			continue
		}
		rawSub, hasKey := envelope["subscribe"]
		if !hasKey {
			continue
		}
		var topics []string
		// json.Unmarshal handles both null → nil and ["a","b"] → slice.
		if err := json.Unmarshal(rawSub, &topics); err != nil {
			continue
		}
		c.setTopics(topics) // nil means "all topics"
		logger.Debug("WebSocket client updated topic filter: %v", topics)
	}
}
