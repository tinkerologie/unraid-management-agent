package recovery

import (
	"context"
	"errors"
	"testing"
)

func TestWaitForStartRequestStopsOnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := waitForStartRequest(ctx, make(chan struct{})); !errors.Is(err, context.Canceled) {
		t.Fatalf("waiting for a start request returned %v, want cancellation", err)
	}
}
