// Package services provides the collector manager for runtime control of collectors.
package services

import (
	"context"
	"errors"
	"fmt"
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

// Collector is the interface that all collectors must implement for runtime management
type Collector interface {
	Start(ctx context.Context, interval time.Duration)
}

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
	ctx       context.Context
	cancel    context.CancelFunc
	factory   CollectorFactory
	domainCtx *domain.Context
	wg        *sync.WaitGroup
	// runner runs the collector in Dynamic mode while it is running.
	runner *collectorRunner
}

// CollectorManager manages runtime enable/disable of collectors
type CollectorManager struct {
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
	wasRunning := mc.cancel != nil
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
	if mc.cancel != nil {
		mc.cancel()
		mc.cancel = nil
	}
	mc.ctx = nil
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
		domainCtx:  cm.domainCtx,
		wg:         cm.wg,
	}

	logger.Debug("Registered collector: %s (interval: %ds, required: %v, gated: %v, auto: %v)", name, interval, required, gated, auto)
}

// StartAll starts all enabled collectors
func (cm *CollectorManager) StartAll() int {
	cm.mu.Lock()
	defer cm.mu.Unlock()

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
	if !exists || mc.Status == "running" {
		return
	}
	if cm.dynamic {
		cm.startRunnerLocked(mc)
		return
	}

	// Create new context for this collector
	// #nosec G118 -- cancel is stored on ManagedCollector and released by DisableCollector, UpdateInterval, StopAll, and collector exit.
	ctx, cancel := context.WithCancel(context.Background())
	mc.ctx = ctx
	mc.cancel = cancel

	// Create collector instance
	collector := mc.factory(mc.domainCtx)
	interval := time.Duration(mc.Interval) * time.Second

	// Start the collector goroutine
	mc.wg.Go(func() {
		defer cancel()
		collector.Start(ctx, interval)
	})

	mc.Status = "running"
	mc.Enabled = true
	now := time.Now()
	mc.LastRun = &now

	logger.Info("Started collector: %s (interval: %ds)", name, mc.Interval)
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

	if mc.Status != "running" {
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
		// Give time for graceful stop
		time.Sleep(100 * time.Millisecond)
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
	if !exists || !mc.Gated {
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

// StopAll stops all running collectors
func (cm *CollectorManager) StopAll() {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	for name, mc := range cm.collectors {
		if mc.cancel != nil {
			cm.stopCollectorLocked(mc)
			mc.Status = "stopped"
			logger.Debug("Stopped collector: %s", name)
		}
	}
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
		return collectors.NewSystemCollector(ctx)
	}, intervals.System, true)

	// Array collector
	cm.Register("array", func(ctx *domain.Context) Collector {
		return collectors.NewArrayCollector(ctx)
	}, intervals.Array, false)

	// Disk collector
	cm.Register("disk", func(ctx *domain.Context) Collector {
		return collectors.NewDiskCollector(ctx)
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
		return collectors.NewUPSCollector(ctx)
	}, intervals.UPS, false)

	// NUT collector
	cm.Register("nut", func(ctx *domain.Context) Collector {
		return collectors.NewNUTCollector(ctx)
	}, intervals.NUT, false)

	// NUT requires UPS to be enabled — UPS collector handles MQTT publishing
	// for both APC and NUT devices via upsc fallback.
	if intervals.NUT > 0 && intervals.UPS == 0 {
		logger.Warning("NUT interval is set but UPS interval is disabled. " +
			"UPS MQTT sensors will not update. Enable the UPS collector to report NUT data via MQTT.")
	}

	// GPU collector
	cm.Register("gpu", func(ctx *domain.Context) Collector {
		return collectors.NewGPUCollector(ctx)
	}, intervals.GPU, false)

	// Share collector
	cm.Register("shares", func(ctx *domain.Context) Collector {
		return collectors.NewShareCollector(ctx)
	}, intervals.Shares, false)

	// Network collector
	cm.Register("network", func(ctx *domain.Context) Collector {
		return collectors.NewNetworkCollector(ctx)
	}, intervals.Network, false)

	// Hardware collector
	cm.Register("hardware", func(ctx *domain.Context) Collector {
		return collectors.NewHardwareCollector(ctx)
	}, intervals.Hardware, false)

	// ZFS collector
	cm.Register("zfs", func(ctx *domain.Context) Collector {
		return collectors.NewZFSCollector(ctx)
	}, intervals.ZFS, false)

	// Notification collector
	cm.Register("notification", func(ctx *domain.Context) Collector {
		return collectors.NewNotificationCollector(ctx)
	}, intervals.Notification, false)

	// Registration collector
	cm.Register("registration", func(ctx *domain.Context) Collector {
		return collectors.NewRegistrationCollector(ctx)
	}, intervals.Registration, false)

	// Unassigned collector
	cm.Register("unassigned", func(ctx *domain.Context) Collector {
		return collectors.NewUnassignedCollector(ctx)
	}, intervals.Unassigned, false)

	// Fan control collector
	cm.Register("fancontrol", func(ctx *domain.Context) Collector {
		return collectors.NewFanControlCollector(ctx)
	}, intervals.FanControl, false)

	// Tuning collector (kernel parameters, NIC offloads, ring buffers)
	cm.Register("tuning", func(ctx *domain.Context) Collector {
		return collectors.NewTuningCollector(ctx)
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
		return c
	}, intervals.DockerUpdate, false)

	// DockerNetworks collector — lists all Docker networks.
	// ListFn is injected here (not in the collector) to avoid a
	// collectors→controllers import cycle.
	cm.Register("docker_networks", func(ctx *domain.Context) Collector {
		c := collectors.NewDockerNetworksCollector(ctx)
		c.ListFn = func() ([]dto.DockerNetworkInfo, error) {
			dc := controllers.NewDockerController()
			defer func() { _ = dc.Close() }()
			return dc.ListNetworks()
		}
		return c
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
		return c
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
		return c
	}, intervals.OSUpdate, false)

	// Mover collector — reads var.ini state + /var/log/mover.log (local files only).
	cm.Register("mover", func(ctx *domain.Context) Collector {
		return collectors.NewMoverCollector(ctx)
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
		return collectors.NewNICCollector(ctx)
	}, intervals.NIC, false)
}
