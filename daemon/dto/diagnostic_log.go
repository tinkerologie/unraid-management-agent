package dto

// DiagnosticLogEntry represents a single structured diagnostic log entry in JSON Lines format.
type DiagnosticLogEntry struct {
	Timestamp     string         `json:"timestamp"`
	Level         string         `json:"level"`
	Message       string         `json:"message"`
	CorrelationID string         `json:"correlation_id,omitempty"`
	Service       string         `json:"service"`
	Host          string         `json:"host"`
	Context       map[string]any `json:"context,omitempty"`
}
