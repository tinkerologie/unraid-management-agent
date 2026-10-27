package services

import (
	"context"
	"slices"
	"sync"
	"time"

	"github.com/tinkerologie/unraid-management-agent/daemon/constants"
	"github.com/tinkerologie/unraid-management-agent/daemon/dto"
	"github.com/tinkerologie/unraid-management-agent/daemon/logger"
	"github.com/tinkerologie/unraid-management-agent/daemon/platform"
)

// CapabilityRefresher keeps the capability snapshot in the platform registry
// current: once at startup, then every CapabilityRefreshInterval, and on
// demand (rate-limited). Refreshes are serialised, so concurrent callers wait
// for the running one and then get its snapshot.
type CapabilityRefresher struct {
	registry *platform.Registry
	probes   func() []platform.Probe

	// Interval, MinInterval and Timeout default to the constants; replaceable for tests.
	Interval    time.Duration
	MinInterval time.Duration
	Timeout     time.Duration
	// Now returns the current time; replaceable for tests.
	Now func() time.Time

	mu   sync.Mutex
	last time.Time
	hook func(dto.Capabilities)
}

// NewCapabilityRefresher creates a refresher that stores snapshots of probes in registry.
func NewCapabilityRefresher(registry *platform.Registry, probes func() []platform.Probe) *CapabilityRefresher {
	return &CapabilityRefresher{
		registry:    registry,
		probes:      probes,
		Interval:    constants.CapabilityRefreshInterval,
		MinInterval: constants.CapabilityMinRefreshInterval,
		Timeout:     constants.CapabilityRefreshTimeout,
		Now:         time.Now,
	}
}

// SetHook sets a function called with every new snapshot (not with cached
// ones), while refreshes are still serialised.
func (r *CapabilityRefresher) SetHook(hook func(dto.Capabilities)) {
	r.mu.Lock()
	r.hook = hook
	r.mu.Unlock()
}

// RefreshCapability rechecks one hardware class without running unrelated probes.
// The hook receives only this class so absent counters for others do not advance.
func (r *CapabilityRefresher) RefreshCapability(ctx context.Context, name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if ctx.Err() != nil {
		return
	}
	for _, probe := range r.probes() {
		if probe.Name != name {
			continue
		}
		checkCtx, cancel := context.WithTimeout(ctx, r.Timeout)
		fresh := platform.DetectContext(checkCtx, []platform.Probe{probe})
		cancel()
		if ctx.Err() != nil {
			return
		}
		previous := r.registry.Capabilities()
		logCapabilityChanges(previous, fresh)
		previous.Items = slices.Clone(previous.Items)
		found := false
		for i := range previous.Items {
			if previous.Items[i].Name == name {
				previous.Items[i] = fresh.Items[0]
				found = true
			}
		}
		if !found {
			previous.Items = append(previous.Items, fresh.Items[0])
		}
		r.registry.SetCapabilities(previous)
		if r.hook != nil {
			r.hook(fresh)
		}
		return
	}
}

// Refresh re-checks capabilities unless the last check is younger than
// MinInterval, in which case the cached snapshot is returned. The check is
// detached from ctx's cancellation (a client hanging up must not leave a
// half-checked snapshot behind) and bounded by Timeout.
func (r *CapabilityRefresher) Refresh(ctx context.Context) dto.Capabilities {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.last.IsZero() && r.Now().Sub(r.last) < r.MinInterval {
		return r.registry.Capabilities()
	}

	checkCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), r.Timeout)
	defer cancel()
	caps := platform.DetectContext(checkCtx, r.probes())
	logCapabilityChanges(r.registry.Capabilities(), caps)
	r.registry.SetCapabilities(caps)
	r.last = r.Now()
	if r.hook != nil {
		r.hook(caps)
	}
	return caps
}

// Start re-checks capabilities every Interval until ctx is cancelled.
func (r *CapabilityRefresher) Start(ctx context.Context) {
	ticker := time.NewTicker(r.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			func() {
				defer func() {
					if rec := recover(); rec != nil {
						logger.LogPanicWithStack("Capability refresher", rec)
					}
				}()
				r.Refresh(ctx)
			}()
		}
	}
}

// logCapabilityChanges logs every check-probe item (hardware classes) that is
// new or changed state.
func logCapabilityChanges(prev, next dto.Capabilities) {
	for _, item := range next.Items {
		old, seen := prev.Item(item.Name)
		if seen && old.State == item.State {
			continue
		}
		if item.Detail == "" {
			continue // path and binary probes are summarised by the caller
		}
		logger.Info("Capabilities: %s is %s (%s)", item.Name, item.State, item.Detail)
	}
}
