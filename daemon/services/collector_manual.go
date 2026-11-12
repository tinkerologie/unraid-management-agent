package services

import (
	"context"
	"time"

	"github.com/tinkerologie/unraid-management-agent/daemon/services/scheduler"
)

// runManual polls at the configured interval. A change trigger gets a
// collection without shifting the fixed polling cadence.
func (r *collectorRunner) runManual(ctx context.Context, interval time.Duration) {
	r.mu.Lock()
	r.decision = scheduler.Decision{Interval: interval, Reason: scheduler.ReasonManual}
	r.mu.Unlock()

	r.runLifetime(ctx, func(ctx context.Context, startSources func()) {
		if delay := r.policy.InitialDelay; delay > 0 {
			timer := r.clock.NewTimer(delay)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return
			case <-timer.C():
			}
		}
		if ctx.Err() != nil {
			return
		}
		// Notification and PSU sources historically watch during the first run.
		watchBeforeRefresh := r.name == "notification" || r.name == "psu"
		if watchBeforeRefresh {
			startSources()
		}
		// Notifications historically start the ticker before the first refresh.
		next := r.clock.Now().Add(interval)
		r.collect(ctx)
		if ctx.Err() != nil {
			return
		}
		if !watchBeforeRefresh {
			startSources()
		}
		now := r.clock.Now()
		if r.name != "notification" {
			next = now.Add(interval)
		}
		timer := r.clock.NewTimer(max(next.Sub(now), 0))
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C():
				// Keep the phase of a ticker and coalesce any missed ticks.
				now := r.clock.Now()
				next = next.Add((max(now.Sub(next), 0)/interval + 1) * interval)
				timer.Reset(next.Sub(now))
			case <-r.trigger:
			}
			if ctx.Err() != nil {
				return
			}
			r.collect(ctx)
		}
	})
}
