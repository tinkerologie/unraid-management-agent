package dto

import "time"

// WSEvent represents a WebSocket event
type WSEvent struct {
	Event     string      `json:"event" example:"system_update"`
	Timestamp time.Time   `json:"timestamp"`
	Data      interface{} `json:"data"`
}

// Response represents a standard API response
type Response struct {
	Success   bool        `json:"success" example:"true"`
	Message   string      `json:"message,omitempty" example:"Operation completed successfully"`
	Error     string      `json:"error,omitempty" example:""`
	Data      interface{} `json:"data,omitempty"`
	Timestamp time.Time   `json:"timestamp"`
}
