package logger

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"sync"
	"time"

	"gopkg.in/natefinch/lumberjack.v2"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

// correlationKeyType is the context key type for storing correlation IDs.
// Exported so lib/correlation.go can share the same key.
type correlationKeyType struct{}

// CorrelationContextKey is the context key used for correlation IDs.
// Shared between logger and lib packages to ensure consistent context value access.
var CorrelationContextKey = correlationKeyType{}

// DiagnosticLogger writes structured JSON log entries to a dedicated diagnostic log file.
type DiagnosticLogger struct {
	writer      io.WriteCloser
	hostname    string
	serviceName string
	mu          sync.Mutex
}

// NewDiagnosticLogger creates a new diagnostic logger that writes JSON Lines to logPath.
// Uses lumberjack for log rotation consistent with the main application logger.
func NewDiagnosticLogger(logPath, serviceName string) *DiagnosticLogger {
	hostname, err := os.Hostname()
	if err != nil {
		Warning("failed to get hostname for diagnostic logger: %v", err)
	}

	writer := &lumberjack.Logger{
		Filename:   logPath,
		MaxSize:    5,     // 5 MB
		MaxBackups: 1,     // Keep only 1 backup
		MaxAge:     1,     // Delete backups older than 1 day
		Compress:   false, // No compression
	}

	return &DiagnosticLogger{
		writer:      writer,
		hostname:    hostname,
		serviceName: serviceName,
	}
}

// correlationIDFromContext retrieves the correlation ID from context.
func correlationIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if id, ok := ctx.Value(CorrelationContextKey).(string); ok {
		return id
	}
	return ""
}

// Log writes a structured diagnostic log entry.
func (d *DiagnosticLogger) Log(ctx context.Context, level, message string, fields map[string]any) {
	entry := dto.DiagnosticLogEntry{
		Timestamp:     time.Now().UTC().Format(time.RFC3339),
		Level:         level,
		Message:       message,
		CorrelationID: correlationIDFromContext(ctx),
		Service:       d.serviceName,
		Host:          d.hostname,
		Context:       fields,
	}

	data, err := json.Marshal(entry)
	if err != nil {
		Error("failed to marshal diagnostic log entry: %v", err)
		return
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	data = append(data, '\n')
	if _, err := d.writer.Write(data); err != nil {
		Error("failed to write diagnostic log entry: %v", err)
	}
}

// Error logs a diagnostic entry at ERROR level.
func (d *DiagnosticLogger) Error(ctx context.Context, message string, fields map[string]any) {
	d.Log(ctx, "ERROR", message, fields)
}

// Warn logs a diagnostic entry at WARN level.
func (d *DiagnosticLogger) Warn(ctx context.Context, message string, fields map[string]any) {
	d.Log(ctx, "WARN", message, fields)
}

// Info logs a diagnostic entry at INFO level.
func (d *DiagnosticLogger) Info(ctx context.Context, message string, fields map[string]any) {
	d.Log(ctx, "INFO", message, fields)
}

// Debug logs a diagnostic entry at DEBUG level.
func (d *DiagnosticLogger) Debug(ctx context.Context, message string, fields map[string]any) {
	d.Log(ctx, "DEBUG", message, fields)
}

// Close releases the underlying log file writer.
func (d *DiagnosticLogger) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.writer.Close()
}
