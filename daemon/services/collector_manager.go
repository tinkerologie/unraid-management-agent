// Package services provides the collector manager for runtime control of collectors.
package services

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tinkerologie/unraid-management-agent/daemon/constants"
	"github.com/tinkerologie/unraid-management-agent/daemon/domain"
	"github.com/tinkerologie/unraid-management-agent/daemon/dto"
	"github.com/tinkerologie/unraid-management-agent/daemon/logger"
	"github.com/tinkerologie/unraid-management-agent/daemon/services/collectors"
	"github.com/tinkerologie/unraid-management-agent/daemon/services/controllers"
	"github.com/tinkerologie/unraid-management-agent/daemon/services/scheduler"
)

// ErrDynamicMode is returned by UpdateInterval in Dynamic collection mode,
// where the agent decides each collector's interval.
var ErrDynamicMode = domain.ErrDynamicCollectionMode

// CollectorFactory is a function that creates a new collector instance
type CollectorFactory func(ctx *domain.Context) Collector

// ManagedCollector wraps a collector with lifecycle management
type ManagedCollector struct {
	Name       string
	Enabled    bool
	Interval   int // seconds
	Status     string
	LastRun    *time.Time
	ErrorCount int
	Required   bool // Cannot be disabled (e.g., system)
	// Gated marks a hardware-gated collector the capability policy controls
	// (ApplyHardwareState): off while its hardware is absent. Auto additionally
	// runs it at the auto-enable interval while present; otherwise it runs at
	// configured. Manual runtime control clears both.
	Gated      bool
	Auto       bool
	configured int

	// Runtime management
	factory    CollectorFactory
	generation *collectorGeneration
	// runner is available for refresh requests only in active Dynamic mode.
	runner *collectorRunner
}

// collectorGeneration owns one lifetime until its sources and Close finish.
type collectorGeneration struct {
	cancel   context.CancelFunc
	done     chan struct{}
	stopping bool
}

// CollectorManager manages runtime enable/disable of collectors
type CollectorManager struct {
	shuttingDown bool
	psuPresent   bool
	psuAnnounced string
	mu           sync.RWMutex
	collectors   map[string]*ManagedCollector
	domainCtx    *domain.Context
	wg           *sync.WaitGroup

	// Dynamic collection mode (#280).
	dynamic bool
	clock   scheduler.Clock
	demand  *scheduler.Demand
	floors  atomic.Pointer[map[string]bool]
	// eventSources returns a collector's change-event sources in Dynamic mode.
	eventSources func(name string) []eventSource
}

func (cm *CollectorManager) stopCollectorLocked(mc *ManagedCollector) {
	wasRunning := mc.Status == "running"
	cm.cancelCollectorLocked(mc)
	if mc.Name == "psu" && wasRunning {
		state := dto.PSUStateDisabled
		if mc.Gated && !cm.psuPresent {
			state = dto.PSUStateNotDetected
		}
		cm.announcePSULocked(state)
	}
}

// cancelCollectorLocked also serves immediate restarts. Preserve the last
// snapshot across a restart so HA discovery is not withdrawn and recreated.
func (cm *CollectorManager) cancelCollectorLocked(mc *ManagedCollector) {
	if g := mc.generation; g != nil && !g.stopping {
		g.stopping = true
		g.cancel()
	}
	mc.runner = nil
}

// NewCollectorManager creates a new collector manager
func NewCollectorManager(domainCtx *domain.Context, wg *sync.WaitGroup) *CollectorManager {
	clock := scheduler.Clock(scheduler.RealClock{})
	cm := &CollectorManager{
		collectors:   make(map[string]*ManagedCollector),
		domainCtx:    domainCtx,
		wg:           wg,
		dynamic:      domainCtx != nil && domainCtx.DynamicCollection(),
		clock:        clock,
		demand:       scheduler.NewDemand(clock),
		eventSources: defaultEventSources,
	}
	cm.demand.SetOnChange(cm.wakeRunners)
	return cm
}

// Demand returns the tracker of collector consumers (WebSocket, MQTT, reads).
func (cm *CollectorManager) Demand() *scheduler.Demand {
	return cm.demand
}

// Dynamic reports whether the manager runs in Dynamic collection mode.
func (cm *CollectorManager) Dynamic() bool {
	return cm.dynamic
}

// wakeRunners asks the named collectors' runners to re-decide their cadence.
func (cm *CollectorManager) wakeRunners(names []string) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	for _, name := range names {
		if mc, ok := cm.collectors[name]; ok && mc.runner != nil {
			mc.runner.Wake()
		}
	}
}

// SetFloors sets the collectors that enabled alert rules read: in Dynamic
// mode they never run slower than their Manual default.
func (cm *CollectorManager) SetFloors(required map[string]bool) {
	var changed []string
	if old := cm.floors.Load(); old != nil {
		for name := range *old {
			if !required[name] {
				changed = append(changed, name)
			}
		}
	}
	for name := range required {
		changed = append(changed, name)
	}
	cm.floors.Store(&required)
	cm.wakeRunners(changed)
}

// floorRequired reports whether an enabled alert rule reads the collector.
func (cm *CollectorManager) floorRequired(name string) bool {
	f := cm.floors.Load()
	return f != nil && (*f)[name]
}

// RequestRefresh asks a running collector for a Dynamic-mode run and returns
// a channel that receives its outcome. It returns false in Manual mode and
// for collectors that are not running.
func (cm *CollectorManager) RequestRefresh(name string) (<-chan error, bool) {
	cm.mu.RLock()
	mc, ok := cm.collectors[name]
	var r *collectorRunner
	if ok {
		r = mc.runner
	}
	cm.mu.RUnlock()
	if r == nil {
		return nil, false
	}
	return r.RequestRefresh(), true
}

// Register registers a collector with the manager
func (cm *CollectorManager) Register(name string, factory CollectorFactory, interval int, required bool) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if old := cm.collectors[name]; old != nil && old.generation != nil {
		logger.Warning("Cannot register collector %s during an active lifetime", name)
		return
	}
	enabled := interval > 0
	if cm.dynamic {
		// INTERVAL_* values are kept but unused: only the disable list turns
		// a collector off.
		enabled = cm.domainCtx.DisabledCollectors == nil || !cm.domainCtx.DisabledCollectors[name]
	}
	var status string
	if enabled {
		status = "registered"
	} else {
		status = "disabled"
	}

	auto := interval == 0 && cm.domainCtx != nil && cm.domainCtx.AutoCollectors[name]
	gated := auto || (cm.domainCtx != nil && cm.domainCtx.GatedCollectors[name])
	if cm.dynamic {
		// Every gated collector follows its hardware; there is no auto interval.
		auto = false
		gated = enabled && cm.domainCtx.GatedCollectors[name]
	}

	cm.collectors[name] = &ManagedCollector{
		Name:       name,
		Enabled:    enabled,
		Interval:   interval,
		Status:     status,
		Required:   required,
		Gated:      gated,
		Auto:       auto,
		configured: interval,
		factory:    factory,
	}

	logger.Debug("Registered collector: %s (interval: %ds, required: %v, gated: %v, auto: %v)", name, interval, required, gated, auto)
}

// StartAll starts all enabled collectors
func (cm *CollectorManager) StartAll() int {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if cm.shuttingDown {
		return 0
	}
	enabledCount := 0
	for name, mc := range cm.collectors {
		if mc.Enabled && (mc.Interval > 0 || cm.dynamic) {
			cm.startCollectorLocked(name)
			enabledCount++
		}
	}

	return enabledCount
}

// startCollectorLocked starts a collector (must hold lock)
func (cm *CollectorManager) startCollectorLocked(name string) {
	mc, exists := cm.collectors[name]
	if !exists || cm.shuttingDown || mc.Status == "running" {
		return
	}
	policy, known := scheduler.PolicyFor(name)
	if cm.dynamic && !known {
		logger.Error("Collector %s has no Dynamic-mode policy, not started", name)
		return
	}
	mc.Status = "running"
	mc.Enabled = true
	if mc.generation != nil {
		// The retiring generation will start the latest requested configuration.
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	g := &collectorGeneration{cancel: cancel, done: make(chan struct{})}
	mc.generation = g
	interval := time.Duration(mc.Interval) * time.Second
	r := newCollectorRunner(name, policy, nil, cm.clock)
	if cm.dynamic {
		r.lowPower = cm.domainCtx.LowPowerMode
		r.active = func() bool { return cm.demand.Active(name) }
		r.floor = func() bool { return cm.floorRequired(name) }
		mc.runner = r
	} else {
		now := cm.clock.Now()
		mc.LastRun = &now
	}

	cm.wg.Go(func() {
		defer cm.finishGeneration(mc, g)
		defer cancel()
		// Factories may do setup and must never run while holding the manager lock.
		defer r.recoverPanic("generation")
		defer r.stop()
		if ctx.Err() != nil {
			return
		}
		r.coll = mc.factory(cm.domainCtx)
		if cm.dynamic {
			if cm.eventSources != nil {
				r.sources = cm.eventSources(name)
			}
			r.run(ctx)
			return
		}
		r.sources = manualEventSources(name, r.coll)
		r.runManual(ctx, interval)
	})
}

// finishGeneration releases the slot only after cleanup and honors a queued restart.
func (cm *CollectorManager) finishGeneration(mc *ManagedCollector, g *collectorGeneration) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	defer close(g.done)
	mc.generation = nil
	mc.runner = nil
	if mc.Status == "running" {
		mc.Status = "stopped"
		if g.stopping && mc.Enabled {
			cm.startCollectorLocked(mc.Name)
		}
	}
}

// EnableCollector enables a collector at runtime
func (cm *CollectorManager) EnableCollector(name string) error {
	var event dto.CollectorStateEvent

	cm.mu.Lock()
	mc, exists := cm.collectors[name]
	if !exists {
		cm.mu.Unlock()
		return fmt.Errorf("unknown collector: %s", name)
	}
	if cm.shuttingDown {
		cm.mu.Unlock()
		return errRunnerStopped
	}
	mc.Gated, mc.Auto = false, false // manual control ends hardware gating

	if mc.Status == "running" {
		cm.mu.Unlock()
		return nil // Already running
	}

	// Set default interval if not set
	if mc.Interval <= 0 {
		mc.Interval = cm.getDefaultInterval(name)
	}

	cm.startCollectorLocked(name)

	// Snapshot state while still holding the lock
	event = cm.buildStateEvent(name, true)
	cm.mu.Unlock()

	// Broadcast outside the lock to avoid deadlock with subscribers
	// that call GetStatus/GetAllStatus (which need RLock).
	domain.Publish(cm.domainCtx.Hub, constants.TopicCollectorStateChange, event)

	return nil
}

// DisableCollector disables a collector at runtime
func (cm *CollectorManager) DisableCollector(name string) error {
	var event dto.CollectorStateEvent

	cm.mu.Lock()
	mc, exists := cm.collectors[name]
	if !exists {
		cm.mu.Unlock()
		return fmt.Errorf("unknown collector: %s", name)
	}

	if mc.Required {
		cm.mu.Unlock()
		return fmt.Errorf("cannot disable %s collector (always required)", name)
	}
	mc.Gated, mc.Auto = false, false // manual control ends hardware gating

	if !mc.Enabled && mc.Status != "running" {
		if name == "psu" {
			cm.announcePSULocked(dto.PSUStateDisabled)
		}
		cm.mu.Unlock()
		return nil // Already stopped
	}

	// Cancel the collector's context
	cm.stopCollectorLocked(mc)

	mc.Status = "stopped"
	mc.Enabled = false

	logger.Info("Disabled collector: %s", name)

	// Snapshot state while still holding the lock
	event = cm.buildStateEvent(name, false)
	cm.mu.Unlock()

	// Broadcast outside the lock to avoid deadlock with subscribers
	// that call GetStatus/GetAllStatus (which need RLock).
	domain.Publish(cm.domainCtx.Hub, constants.TopicCollectorStateChange, event)

	return nil
}

// UpdateInterval updates the collection interval for a collector
func (cm *CollectorManager) UpdateInterval(name string, intervalSeconds int) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	mc, exists := cm.collectors[name]
	if !exists {
		return fmt.Errorf("unknown collector: %s", name)
	}

	if cm.dynamic {
		return ErrDynamicMode
	}

	if intervalSeconds < 5 || intervalSeconds > 86400 {
		return errors.New("invalid interval: must be between 5 and 86400 seconds")
	}
	mc.Gated, mc.Auto = false, false // manual control ends hardware gating

	wasRunning := mc.Status == "running"

	// Stop the collector if running
	if wasRunning {
		cm.cancelCollectorLocked(mc)
		mc.Status = "stopped"
	}

	// Update interval
	mc.Interval = intervalSeconds

	// Restart if it was running
	if wasRunning {
		cm.startCollectorLocked(name)
	}

	logger.Info("Updated collector %s interval to %d seconds", name, intervalSeconds)

	return nil
}

// ApplyHardwareState applies the hardware capability policy to a gated
// collector. Absent hardware stops and disables it whatever its configured
// interval; present hardware runs it at autoInterval in automatic mode and at
// its configured interval otherwise (a configured 0 keeps it off). It reports
// whether anything changed; collectors that are unknown or not gated are left
// alone.
func (cm *CollectorManager) ApplyHardwareState(name string, present bool, autoInterval int) bool {
	cm.mu.Lock()
	mc, exists := cm.collectors[name]
	if !exists || !mc.Gated || cm.shuttingDown {
		cm.mu.Unlock()
		return false
	}
	if cm.dynamic {
		return cm.applyHardwareStateDynamic(mc, present)
	}
	interval := 0
	if present {
		interval = mc.configured
		if mc.Auto {
			interval = autoInterval
		}
	}
	running := mc.Status == "running"
	if interval <= 0 {
		if !running && mc.Interval == 0 {
			cm.mu.Unlock()
			return false
		}
		cm.stopCollectorLocked(mc)
		mc.Interval = 0
		mc.Enabled = false
		mc.Status = "disabled"
		logger.Info("Hardware-gated collector stopped: %s", name)
	} else {
		if running && mc.Interval == interval {
			cm.mu.Unlock()
			return false
		}
		if running {
			cm.cancelCollectorLocked(mc)
			mc.Status = "stopped"
		}
		mc.Interval = interval
		cm.startCollectorLocked(name)
		logger.Info("Hardware-gated collector started: %s (interval: %ds)", name, interval)
	}
	event := cm.buildStateEvent(name, mc.Enabled)
	cm.mu.Unlock()

	domain.Publish(cm.domainCtx.Hub, constants.TopicCollectorStateChange, event)
	return true
}

// GetStatus returns the status of a specific collector
func (cm *CollectorManager) GetStatus(name string) (*dto.CollectorStatus, error) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	mc, exists := cm.collectors[name]
	if !exists {
		return nil, fmt.Errorf("unknown collector: %s", name)
	}

	status := cm.statusLocked(mc)
	return &status, nil
}

// GetAllStatus returns the status of all collectors
func (cm *CollectorManager) GetAllStatus() dto.CollectorsStatusResponse {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	var statuses []dto.CollectorStatus
	enabledCount := 0
	disabledCount := 0

	// Use consistent order
	collectorOrder := []string{
		"system", "array", "disk", "docker", "vm",
		"ups", "nut", "gpu", "shares", "network",
		"hardware", "zfs", "notification", "registration", "unassigned",
		"fancontrol", "tuning", "docker_update", "docker_networks", "plugin_update",
		"os_update", "mover", "storage_topology", "nic", "psu",
	}

	for _, name := range collectorOrder {
		mc, exists := cm.collectors[name]
		if !exists {
			continue
		}

		if mc.Enabled || mc.Status == "running" {
			enabledCount++
		} else {
			disabledCount++
		}

		statuses = append(statuses, cm.statusLocked(mc))
	}

	return dto.CollectorsStatusResponse{
		Collectors:    statuses,
		Mode:          cm.mode(),
		Total:         len(statuses),
		EnabledCount:  enabledCount,
		DisabledCount: disabledCount,
		Timestamp:     time.Now(),
	}
}

// mode returns the collection mode name.
func (cm *CollectorManager) mode() string {
	if cm.dynamic {
		return domain.CollectionModeDynamic
	}
	return domain.CollectionModeManual
}

// statusLocked builds a collector's status; cm.mu must be held.
func (cm *CollectorManager) statusLocked(mc *ManagedCollector) dto.CollectorStatus {
	policy, _ := scheduler.PolicyFor(mc.Name)
	status := dto.CollectorStatus{
		Name:          mc.Name,
		Enabled:       mc.Enabled,
		Interval:      mc.Interval,
		Status:        mc.Status,
		LastRun:       mc.LastRun,
		ErrorCount:    mc.ErrorCount,
		Required:      mc.Required,
		HardwareGated: mc.Gated,
		Auto:          mc.Auto,
		Mode:          cm.mode(),
		Reason:        string(scheduler.BaselineReason(policy, !cm.dynamic)),
		EventDriven:   cm.dynamic && policy.EventDriven,
	}
	if len(policy.Topics) > 0 && cm.domainCtx != nil && cm.domainCtx.Hub != nil {
		status.PublishCount = cm.domainCtx.Hub.PublishCount(policy.Topics[0])
	}
	if mc.Status != "running" {
		return status
	}
	if !cm.dynamic {
		status.EffectiveInterval = mc.Interval
		return status
	}
	if mc.runner != nil {
		st := mc.runner.snapshot()
		status.EffectiveInterval = int(st.decision.Interval / time.Second)
		status.Reason = string(st.decision.Reason)
		status.ErrorCount = st.errorCount
		if !st.lastAttempt.IsZero() {
			at := st.lastAttempt
			status.LastRun = &at
		}
	}
	return status
}

// GetCollectorNames returns all registered collector names
func (cm *CollectorManager) GetCollectorNames() []string {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	names := make([]string, 0, len(cm.collectors))
	for name := range cm.collectors {
		names = append(names, name)
	}
	return names
}

// StopAll requests stop for every collector, retaining its configured enablement.
func (cm *CollectorManager) StopAll() {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.stopAllLocked()
}

func (cm *CollectorManager) stopAllLocked() map[string]<-chan struct{} {
	generations := make(map[string]<-chan struct{})
	for name, mc := range cm.collectors {
		if g := mc.generation; g != nil {
			cm.stopCollectorLocked(mc)
			mc.Status = "stopped"
			generations[name] = g.done
		}
	}
	return generations
}

// Shutdown prevents new generations and waits for cleanup up to ctx's deadline.
// The returned names are still stopping and continue to own their resources.
func (cm *CollectorManager) Shutdown(ctx context.Context) []string {
	cm.mu.Lock()
	cm.shuttingDown = true
	generations := cm.stopAllLocked()
	cm.mu.Unlock()

	var stopping []string
	for name, done := range generations {
		select {
		case <-done:
		case <-ctx.Done():
			select {
			case <-done:
			default:
				stopping = append(stopping, name)
			}
		}
	}
	slices.Sort(stopping)
	return stopping
}

// buildStateEvent creates a CollectorStateEvent snapshot. Must be called while cm.mu is held.
func (cm *CollectorManager) buildStateEvent(name string, enabled bool) dto.CollectorStateEvent {
	return dto.CollectorStateEvent{
		Event:     constants.TopicCollectorStateChange.Name,
		Collector: name,
		Enabled:   enabled,
		Status:    cm.collectors[name].Status,
		Interval:  cm.collectors[name].Interval,
		Timestamp: time.Now(),
	}
}

// getDefaultInterval returns the default interval for a collector
func (cm *CollectorManager) getDefaultInterval(name string) int {
	defaults := map[string]int{
		"system":          5,
		"array":           10,
		"disk":            30,
		"docker":          10,
		"vm":              10,
		"ups":             10,
		"nut":             10,
		"gpu":             10,
		"shares":          60,
		"network":         15,
		"hardware":        60,
		"zfs":             30,
		"notification":    30,
		"registration":    300,
		"unassigned":      60,
		"fancontrol":      constants.IntervalFanControl,
		"tuning":          120,
		"docker_update":   constants.IntervalDockerUpdate,
		"docker_networks": constants.IntervalDockerNetworks,
		"plugin_update":   constants.IntervalPluginUpdate,
		"os_update":       constants.IntervalOSUpdate,
		"mover":           constants.IntervalMover,

		"storage_topology": constants.IntervalStorageTopology,
		"nic":              constants.IntervalNIC,
		"psu":              constants.IntervalPSU,
	}

	if interval, ok := defaults[name]; ok {
		return interval
	}
	return 30 // Default fallback
}

// RegisterAllCollectors registers all collectors with the manager
func (cm *CollectorManager) RegisterAllCollectors() {
	intervals := cm.domainCtx.Intervals

	// System collector is required
	cm.Register("system", func(ctx *domain.Context) Collector {
		return collectOnce(collectors.NewSystemCollector(ctx).Collect)
	}, intervals.System, true)

	// Array collector
	cm.Register("array", func(ctx *domain.Context) Collector {
		return collectOnce(collectors.NewArrayCollector(ctx).Collect)
	}, intervals.Array, false)

	// Disk collector
	cm.Register("disk", func(ctx *domain.Context) Collector {
		return collectOnce(collectors.NewDiskCollector(ctx).Collect)
	}, intervals.Disk, false)

	// Docker collector - uses Docker SDK for fast container info collection
	cm.Register("docker", func(ctx *domain.Context) Collector {
		return collectors.NewDockerCollector(ctx)
	}, intervals.Docker, false)

	// VM collector - uses libvirt API for fast VM info collection
	cm.Register("vm", func(ctx *domain.Context) Collector {
		return collectors.NewVMCollector(ctx)
	}, intervals.VM, false)

	// UPS collector
	cm.Register("ups", func(ctx *domain.Context) Collector {
		return collectOnce(collectors.NewUPSCollector(ctx).Collect)
	}, intervals.UPS, false)

	// NUT collector
	cm.Register("nut", func(ctx *domain.Context) Collector {
		return collectOnce(collectors.NewNUTCollector(ctx).Collect)
	}, intervals.NUT, false)

	// NUT requires UPS to be enabled — UPS collector handles MQTT publishing
	// for both APC and NUT devices via upsc fallback.
	if intervals.NUT > 0 && intervals.UPS == 0 {
		logger.Warning("NUT interval is set but UPS interval is disabled. " +
			"UPS MQTT sensors will not update. Enable the UPS collector to report NUT data via MQTT.")
	}

	// GPU collector
	cm.Register("gpu", func(ctx *domain.Context) Collector {
		return collectOnce(collectors.NewGPUCollector(ctx).Collect)
	}, intervals.GPU, false)

	// Share collector
	cm.Register("shares", func(ctx *domain.Context) Collector {
		return collectFunc(collectors.NewShareCollector(ctx).Collect)
	}, intervals.Shares, false)

	// Network collector
	cm.Register("network", func(ctx *domain.Context) Collector {
		return collectOnce(collectors.NewNetworkCollector(ctx).Collect)
	}, intervals.Network, false)

	// Hardware collector
	cm.Register("hardware", func(ctx *domain.Context) Collector {
		return collectOnce(collectors.NewHardwareCollector(ctx).Collect)
	}, intervals.Hardware, false)

	// ZFS collector
	cm.Register("zfs", func(ctx *domain.Context) Collector {
		return collectors.NewZFSCollector(ctx)
	}, intervals.ZFS, false)

	// Notification collector
	cm.Register("notification", func(ctx *domain.Context) Collector {
		c := collectors.NewNotificationCollector(ctx)
		c.WatchManualEvents = !cm.dynamic
		return c
	}, intervals.Notification, false)

	// Registration collector
	cm.Register("registration", func(ctx *domain.Context) Collector {
		return collectOnce(collectors.NewRegistrationCollector(ctx).Collect)
	}, intervals.Registration, false)

	// Unassigned collector
	cm.Register("unassigned", func(ctx *domain.Context) Collector {
		return collectors.NewUnassignedCollector(ctx)
	}, intervals.Unassigned, false)

	// Fan control collector
	cm.Register("fancontrol", func(ctx *domain.Context) Collector {
		return collectOnce(collectors.NewFanControlCollector(ctx).Collect)
	}, intervals.FanControl, false)

	// Tuning collector (kernel parameters, NIC offloads, ring buffers)
	cm.Register("tuning", func(ctx *domain.Context) Collector {
		return collectOnce(collectors.NewTuningCollector(ctx).Collect)
	}, intervals.Tuning, false)

	// DockerUpdate collector — checks container images for available updates.
	// CheckFn and NotifyFn are injected here (not in the collector) to avoid a
	// collectors→controllers import cycle.
	cm.Register("docker_update", func(ctx *domain.Context) Collector {
		c := collectors.NewDockerUpdateCollector(ctx)
		c.CheckFn = func(checkCtx context.Context) (*dto.ContainerUpdatesResult, error) {
			dc := controllers.NewDockerController()
			defer func() { _ = dc.Close() }()
			return dc.CheckAllContainerUpdates(checkCtx)
		}
		c.NotifyFn = func(names []string) {
			if err := controllers.CreateNotification(
				"Container updates available",
				"Docker",
				fmt.Sprintf("Updates available for: %s", strings.Join(names, ", ")),
				"info",
				"",
			); err != nil {
				logger.Warning("docker_update: failed to create notification: %v", err)
			}
		}
		return collectFunc(c.Collect)
	}, intervals.DockerUpdate, false)

	// DockerNetworks collector — lists all Docker networks.
	// ListFn is injected here (not in the collector) to avoid a
	// collectors→controllers import cycle.
	cm.Register("docker_networks", func(ctx *domain.Context) Collector {
		c := collectors.NewDockerNetworksCollector(ctx)
		c.ListFn = func(ctx context.Context) ([]dto.DockerNetworkInfo, error) {
			dc := controllers.NewDockerController()
			defer func() { _ = dc.Close() }()
			return dc.ListNetworks(ctx)
		}
		return collectFunc(c.Collect)
	}, intervals.DockerNetworks, false)

	// PluginUpdate collector — checks installed plugins for available updates.
	// CheckFn and NotifyFn are injected here (not in the collector) to avoid a
	// collectors→controllers import cycle.
	cm.Register("plugin_update", func(ctx *domain.Context) Collector {
		c := collectors.NewPluginUpdateCollector(ctx)
		c.CheckFn = func(checkCtx context.Context) (*dto.PluginList, error) {
			pc := controllers.NewPluginController()
			updates, err := pc.CheckPluginUpdates(checkCtx)
			if err != nil {
				return nil, err
			}
			return &dto.PluginList{
				Plugins:          updates,
				TotalCount:       len(updates),
				UpdatesAvailable: len(updates),
			}, nil
		}
		c.NotifyFn = func(names []string) {
			if err := controllers.CreateNotification(
				"Plugin updates available",
				"Plugins",
				fmt.Sprintf("Updates available for: %s", strings.Join(names, ", ")),
				"info",
				"",
			); err != nil {
				logger.Warning("plugin_update: failed to create notification: %v", err)
			}
		}
		return collectFunc(c.Collect)
	}, intervals.PluginUpdate, false)

	// OSUpdate collector — best-effort local-file-only OS update availability check.
	// The default CheckFn reads only local files (no outbound network calls).
	// NotifyFn is injected here to avoid a collectors→controllers import cycle.
	cm.Register("os_update", func(ctx *domain.Context) Collector {
		c := collectors.NewOSUpdateCollector(ctx)
		c.NotifyFn = func(latest string) {
			if err := controllers.CreateNotification(
				"Unraid OS update available",
				"System",
				fmt.Sprintf("A new Unraid OS version is available: %s", latest),
				"info",
				"",
			); err != nil {
				logger.Warning("os_update: failed to create notification: %v", err)
			}
		}
		return collectOnce(c.Collect)
	}, intervals.OSUpdate, false)

	// Mover collector — reads native status and progress plus legacy settings and history.
	cm.Register("mover", func(ctx *domain.Context) Collector {
		return collectFunc(collectors.NewMoverCollector(ctx).Collect)
	}, intervals.Mover, false)

	// Storage topology collector — storcli (Broadcom/LSI) and sg_ses (SES enclosures).
	// Read-only queries, run sequentially; a no-op on systems without either tool.
	cm.Register("storage_topology", func(ctx *domain.Context) Collector {
		return collectors.NewStorageTopologyCollector(ctx)
	}, intervals.StorageTopology, false)

	// Shared PSU collector, initially using the passive Corsair hwmon backend.
	cm.Register("psu", func(ctx *domain.Context) Collector {
		c := collectors.NewPSUCollector(ctx)
		c.PublishGuard = cm.publishPSU
		return c
	}, intervals.PSU, false)

	// NIC collector — PCI network cards and optic diagnostics.
	cm.Register("nic", func(ctx *domain.Context) Collector {
		return collectOnce(collectors.NewNICCollector(ctx).Collect)
	}, intervals.NIC, false)
}
