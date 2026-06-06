// Package platform provides OS-resilience primitives: a data-source health
// registry, capability/version detection, and path/binary resolution. It is
// deliberately Unraid-agnostic (callers supply probe lists) and imports only
// dto + logger, so any layer can use it without import cycles.
package platform

import (
	"sort"
	"sync"
	"time"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
)

// Notifier is invoked once per state transition for a subsystem.
type Notifier func(dto.SourceStatus)

// Registry is a thread-safe store of per-subsystem source health.
type Registry struct {
	mu       sync.RWMutex
	statuses map[string]dto.SourceStatus
	caps     dto.Capabilities
	notifier Notifier
	clock    func() time.Time
}

// NewRegistry creates an empty registry.
func NewRegistry() *Registry {
	return &Registry{statuses: make(map[string]dto.SourceStatus), clock: time.Now}
}

// SetNotifier sets the transition callback (wired to the event bus by the orchestrator).
func (r *Registry) SetNotifier(n Notifier) { r.mu.Lock(); r.notifier = n; r.mu.Unlock() }

// SetClock overrides the time source (tests).
func (r *Registry) SetClock(f func() time.Time) { r.mu.Lock(); r.clock = f; r.mu.Unlock() }

// SetCapabilities stores the startup capability snapshot.
func (r *Registry) SetCapabilities(c dto.Capabilities) { r.mu.Lock(); r.caps = c; r.mu.Unlock() }

// Capabilities returns the startup capability snapshot.
func (r *Registry) Capabilities() dto.Capabilities { r.mu.RLock(); defer r.mu.RUnlock(); return r.caps }

// Report records a subsystem's source state. On a state transition it logs once
// and invokes the notifier. err may be nil.
func (r *Registry) Report(subsystem string, state dto.SourceState, reason string, err error) {
	r.mu.Lock()
	prev, existed := r.statuses[subsystem]
	status := dto.SourceStatus{
		Subsystem:   subsystem,
		State:       state,
		Reason:      reason,
		LastChecked: r.clock(),
	}
	if err != nil {
		status.LastError = err.Error()
	}
	r.statuses[subsystem] = status
	transition := !existed || prev.State != state
	notifier := r.notifier
	r.mu.Unlock()

	if transition {
		if state == dto.SourceHealthy {
			logger.Info("Resilience: %s source recovered (healthy)", subsystem)
		} else {
			logger.Warning("Resilience: %s source %s: %s", subsystem, state, reason)
		}
		if notifier != nil {
			notifier(status)
		}
	}
}

// Healthy is a convenience for Report(subsystem, healthy, "", nil).
func (r *Registry) Healthy(subsystem string) { r.Report(subsystem, dto.SourceHealthy, "", nil) }

// Get returns the current status for a subsystem.
func (r *Registry) Get(subsystem string) (dto.SourceStatus, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.statuses[subsystem]
	return s, ok
}

// StatusFor returns a pointer to the status only when NOT healthy (for inline DTO flags).
func (r *Registry) StatusFor(subsystem string) *dto.SourceStatus {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.statuses[subsystem]
	if !ok || s.State == dto.SourceHealthy {
		return nil
	}
	cp := s
	return &cp
}

// Snapshot returns all statuses sorted by subsystem name.
func (r *Registry) Snapshot() []dto.SourceStatus {
	r.mu.RLock()
	out := make([]dto.SourceStatus, 0, len(r.statuses))
	for _, s := range r.statuses {
		out = append(out, s)
	}
	r.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].Subsystem < out[j].Subsystem })
	return out
}

// DegradedCount returns the number of subsystems not in the healthy state.
func (r *Registry) DegradedCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	n := 0
	for _, s := range r.statuses {
		if s.State != dto.SourceHealthy {
			n++
		}
	}
	return n
}

// OverallState returns the worst current state (healthy if empty).
func (r *Registry) OverallState() dto.SourceState {
	r.mu.RLock()
	defer r.mu.RUnlock()
	worst := dto.SourceHealthy
	for _, s := range r.statuses {
		if s.State.Severity() > worst.Severity() {
			worst = s.State
		}
	}
	return worst
}
