package watchdog

import (
	"context"
	"sync"
	"time"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
)

const (
	// MaxHistoryEvents is the maximum number of health check events kept in memory.
	MaxHistoryEvents = 100

	// TickInterval is how often the runner checks for health checks to execute.
	TickInterval = 5 * time.Second

	// RemediationCooldown is the minimum time between remediation actions for the same check.
	RemediationCooldown = 5 * time.Minute
)

// Runner orchestrates health check probes and remediation actions.
type Runner struct {
	store      *Store
	remediator *Remediator

	mu          sync.RWMutex
	statuses    map[string]*dto.HealthCheckStatus
	lastRun     map[string]time.Time
	history     []dto.HealthCheckEvent
	historyIdx  int
	historyFull bool
}

// NewRunner creates a new health check runner.
func NewRunner(store *Store) *Runner {
	return &Runner{
		store:      store,
		remediator: NewRemediator(),
		statuses:   make(map[string]*dto.HealthCheckStatus),
		lastRun:    make(map[string]time.Time),
		history:    make([]dto.HealthCheckEvent, MaxHistoryEvents),
	}
}

// Start runs the watchdog loop until context is cancelled.
func (r *Runner) Start(ctx context.Context) {
	if err := r.store.Load(); err != nil {
		logger.Error("Watchdog: Failed to load health checks: %v", err)
	}

	logger.Success("Watchdog started (%d health checks loaded)", len(r.store.GetChecks()))

	ticker := time.NewTicker(TickInterval)
	defer ticker.Stop()

	// Run once immediately
	func() {
		defer func() {
			if rec := recover(); rec != nil {
				logger.Error("Watchdog PANIC on startup: %v", rec)
			}
		}()
		r.tick(ctx)
	}()

	for {
		select {
		case <-ctx.Done():
			logger.Info("Watchdog stopped")
			return
		case <-ticker.C:
			func() {
				defer func() {
					if rec := recover(); rec != nil {
						logger.Error("Watchdog PANIC in loop: %v", rec)
					}
				}()
				r.tick(ctx)
			}()
		}
	}
}

// tick runs all due health checks.
func (r *Runner) tick(ctx context.Context) {
	checks := r.store.GetEnabledChecks()
	now := time.Now()

	for _, check := range checks {
		interval := time.Duration(check.IntervalSeconds) * time.Second

		r.mu.RLock()
		last, exists := r.lastRun[check.ID]
		r.mu.RUnlock()

		if exists && now.Sub(last) < interval {
			continue // Not due yet
		}

		r.runCheck(ctx, check, now)
	}
}

// runCheck executes a single health check probe and handles the result.
func (r *Runner) runCheck(ctx context.Context, check dto.HealthCheck, now time.Time) {
	result := RunProbe(ctx, check)

	r.mu.Lock()
	r.lastRun[check.ID] = now

	status, exists := r.statuses[check.ID]
	if !exists {
		status = &dto.HealthCheckStatus{
			CheckID:           check.ID,
			CheckName:         check.Name,
			CheckType:         check.Type,
			Target:            check.Target,
			RemediationAction: check.OnFail,
		}
		r.statuses[check.ID] = status
	}

	wasHealthy := status.Healthy || !exists
	status.Healthy = result.Healthy
	status.LastCheck = now
	status.CheckName = check.Name
	status.Target = check.Target
	status.RemediationAction = check.OnFail

	if result.Healthy {
		status.LastError = ""
		status.ConsecutiveFails = 0
	} else {
		status.LastError = result.Error
		status.ConsecutiveFails++
	}

	// Detect state transitions
	transitionedToUnhealthy := wasHealthy && !result.Healthy
	transitionedToHealthy := !wasHealthy && result.Healthy

	// Check remediation cooldown
	canRemediate := status.LastRemediation == nil || now.Sub(*status.LastRemediation) >= RemediationCooldown
	needsRemediation := !result.Healthy && transitionedToUnhealthy && canRemediate && check.OnFail != ""

	if needsRemediation {
		ts := now
		status.LastRemediation = &ts
	}
	r.mu.Unlock()

	// Log and record state transitions
	if transitionedToUnhealthy {
		event := dto.HealthCheckEvent{
			CheckID:   check.ID,
			CheckName: check.Name,
			State:     "unhealthy",
			Message:   result.Error,
			Timestamp: now,
		}

		if needsRemediation {
			logger.Warning("Watchdog: '%s' failed (%s), executing remediation: %s",
				check.Name, result.Error, check.OnFail)

			if err := r.remediator.Execute(ctx, check, result); err != nil {
				logger.Error("Watchdog: Remediation failed for '%s': %v", check.Name, err)
			} else {
				event.RemediationTaken = check.OnFail
			}
		} else {
			logger.Warning("Watchdog: '%s' failed: %s", check.Name, result.Error)
		}

		r.addHistory(event)
	} else if transitionedToHealthy {
		logger.Success("Watchdog: '%s' recovered", check.Name)
		r.addHistory(dto.HealthCheckEvent{
			CheckID:   check.ID,
			CheckName: check.Name,
			State:     "healthy",
			Message:   "Check recovered",
			Timestamp: now,
		})
	}
}

// addHistory adds an event to the ring buffer.
func (r *Runner) addHistory(event dto.HealthCheckEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.history[r.historyIdx] = event
	r.historyIdx = (r.historyIdx + 1) % MaxHistoryEvents
	if r.historyIdx == 0 {
		r.historyFull = true
	}
}

// GetHistory returns health check events in reverse chronological order.
func (r *Runner) GetHistory() []dto.HealthCheckEvent {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var events []dto.HealthCheckEvent
	if r.historyFull {
		events = make([]dto.HealthCheckEvent, MaxHistoryEvents)
		// Newer events first
		for i := range MaxHistoryEvents {
			idx := (r.historyIdx - 1 - i + MaxHistoryEvents) % MaxHistoryEvents
			events[i] = r.history[idx]
		}
	} else {
		events = make([]dto.HealthCheckEvent, r.historyIdx)
		for i := 0; i < r.historyIdx; i++ {
			events[i] = r.history[r.historyIdx-1-i]
		}
	}
	return events
}

// GetStatuses returns the current status of all tracked health checks.
func (r *Runner) GetStatuses() []dto.HealthCheckStatus {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]dto.HealthCheckStatus, 0, len(r.statuses))
	for _, s := range r.statuses {
		result = append(result, *s)
	}
	return result
}

// GetStatus returns the status of a specific health check.
func (r *Runner) GetStatus(id string) (*dto.HealthCheckStatus, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	s, ok := r.statuses[id]
	if !ok {
		return nil, nil // Not yet run
	}
	result := *s
	return &result, nil
}

// GetUnhealthyChecks returns only checks that are currently unhealthy.
func (r *Runner) GetUnhealthyChecks() []dto.HealthCheckStatus {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]dto.HealthCheckStatus, 0)
	for _, s := range r.statuses {
		if !s.Healthy {
			result = append(result, *s)
		}
	}
	return result
}

// RunSingleCheck executes a specific health check immediately (for manual triggers).
func (r *Runner) RunSingleCheck(ctx context.Context, id string) (*dto.HealthCheckStatus, error) {
	check, err := r.store.GetCheck(id)
	if err != nil {
		return nil, err
	}

	r.runCheck(ctx, *check, time.Now())

	r.mu.RLock()
	defer r.mu.RUnlock()

	s, ok := r.statuses[id]
	if !ok {
		return nil, nil
	}
	result := *s
	return &result, nil
}

// CleanupCheck removes status tracking for a deleted health check.
func (r *Runner) CleanupCheck(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.statuses, id)
	delete(r.lastRun, id)
}
