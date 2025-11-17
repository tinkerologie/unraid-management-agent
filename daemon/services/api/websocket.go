package api

import (
	"context"
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

// WSHub manages WebSocket client connections and broadcasts messages to all connected clients.
// It handles client registration, unregistration, and message broadcasting in a thread-safe manner.
type WSHub struct {
	clients    map[*WSClient]bool
	broadcast  chan interface{}
	register   chan *WSClient
	unregister chan *WSClient
	mu         sync.RWMutex
}

// WSClient represents a single WebSocket client connection.
// It maintains the connection to the hub, the WebSocket connection, and a send channel for outgoing messages.
type WSClient struct {
	hub  *WSHub
	conn *websocket.Conn
	send chan dto.WSEvent
}

// NewWSHub creates and initializes a new WebSocket hub.
// The hub is ready to accept client connections and broadcast messages.
func NewWSHub() *WSHub {
	return &WSHub{
		clients:    make(map[*WSClient]bool),
		broadcast:  make(chan interface{}, constants.WSBufferSize),
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

		case message := <-h.broadcast:
			h.mu.RLock()
			event := dto.WSEvent{
				Event:     "update",
				Timestamp: time.Now(),
				Data:      message,
			}
			for client := range h.clients {
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

// Broadcast sends a message to all connected WebSocket clients.
// The message is wrapped in a WSEvent and sent asynchronously to each client.
func (h *WSHub) Broadcast(message interface{}) {
	h.broadcast <- message
}

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
		_, _, err := c.conn.ReadMessage()
		if err != nil {
			break
		}
	}
}
