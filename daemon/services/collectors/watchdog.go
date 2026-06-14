package collectors

import (
	"context"
	"time"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
)

// watchdogFloor and watchdogCeil bound the per-cycle stall threshold derived
// from a collector's interval. The floor gives fast collectors (e.g. fancontrol
// at 5s) headroom so normal jitter never trips the watchdog; the ceiling ensures
// even long-interval collectors (e.g. hardware at 10m, the update checkers at
// hours) surface a stall within a few minutes rather than waiting a whole cycle.
const (
	watchdogFloor = 30 * time.Second
	watchdogCeil  = 5 * time.Minute
)

// watchdogThreshold derives the stall threshold for a collector from its
// collection interval, clamped to [watchdogFloor, watchdogCeil]. A healthy cycle
// completes well within its interval, so a cycle exceeding it is anomalous; the
// clamp keeps the signal useful for both very fast and very slow collectors.
func watchdogThreshold(interval time.Duration) time.Duration {
	switch {
	case interval < watchdogFloor:
		return watchdogFloor
	case interval > watchdogCeil:
		return watchdogCeil
	default:
		return interval
	}
}

// collectWithWatchdog runs a single collector cycle and observes how long it
// takes. Every cycle's start and duration are logged at debug; if a cycle does
// not finish within the interval-derived threshold, a warning plus a full
// goroutine stack dump are logged once, and a second warning is logged when the
// cycle eventually finishes. This makes a stalled cycle visible and diagnosable
// from the agent log: the gap between "starting" and "finished", and the
// goroutine dump, reveal exactly which call is blocked.
//
// Motivation: collectors run cycles serially on one goroutine, so one blocked
// cycle freezes that collector's updates until the call returns — e.g. the
// unassigned-devices collector stalling under concurrent SMB mount/unmount churn
// (ha-unraid-management-agent#83), or any collector that shells out to a command
// that hangs on a pathological system (issue #123). The watchdog is purely
// observational — it never cancels or alters the cycle, so enabling it cannot
// change behaviour, only surface it.
func collectWithWatchdog(ctx context.Context, name string, interval time.Duration, collect func()) {
	runCollectWithWatchdog(ctx, name, watchdogThreshold(interval), collect)
}

// runCollectWithWatchdog is the threshold-based core of collectWithWatchdog,
// separated so the watchdog behaviour can be tested with a small threshold
// without waiting for the interval-derived minimum.
func runCollectWithWatchdog(ctx context.Context, name string, threshold time.Duration, collect func()) {
	start := time.Now()
	done := make(chan struct{})

	logger.Debug("%s: collect cycle starting", name)

	go func() {
		select {
		case <-done:
		case <-ctx.Done():
			// Daemon shutting down — not a stall; exit without dumping.
		case <-time.After(threshold):
			logger.Warning("%s: collect cycle still running after %v — likely stalled; dumping goroutine stacks", name, threshold)
			logger.Warning("%s: goroutine dump follows:\n%s", name, logger.AllGoroutineStacks())
		}
	}()

	// Deferred so the watchdog is always stopped and the duration always logged,
	// even if collect panics (the caller's recover handles the panic itself).
	defer func() {
		close(done)
		elapsed := time.Since(start)
		if elapsed >= threshold {
			logger.Warning("%s: collect cycle finished after %v (was stalled)", name, elapsed)
		} else {
			logger.Debug("%s: collect cycle finished in %v", name, elapsed)
		}
	}()

	collect()
}
