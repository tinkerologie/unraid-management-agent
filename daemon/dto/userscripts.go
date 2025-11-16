package dto

import "time"

// UserScriptInfo represents metadata about a user script
type UserScriptInfo struct {
	Name         string    `json:"name"`
	Description  string    `json:"description"`
	Path         string    `json:"path"`
	Executable   bool      `json:"executable"`
	LastModified time.Time `json:"last_modified"`
}

// UserScriptExecuteRequest represents a request to execute a user script
type UserScriptExecuteRequest struct {
	Background bool `json:"background"`
	Wait       bool `json:"wait"`
}

// UserScriptExecuteResponse represents the response from executing a user script
type UserScriptExecuteResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	PID     int    `json:"pid,omitempty"`
	Output  string `json:"output,omitempty"`
	Error   string `json:"error,omitempty"`
}
