package lib

import (
	"context"

	"github.com/google/uuid"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
)

// NewCorrelationID returns a new UUID string for use as a correlation ID.
func NewCorrelationID() string {
	return uuid.New().String()
}

// ContextWithCorrelationID attaches a correlation ID to a context.
func ContextWithCorrelationID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, logger.CorrelationContextKey, id)
}

// CorrelationIDFromContext retrieves the correlation ID from a context.
// Returns an empty string if no correlation ID is present.
func CorrelationIDFromContext(ctx context.Context) string {
	if id, ok := ctx.Value(logger.CorrelationContextKey).(string); ok {
		return id
	}
	return ""
}
