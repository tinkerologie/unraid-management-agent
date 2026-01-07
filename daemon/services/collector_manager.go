// Package services provides the collector manager for runtime control of collectors.
package services

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/services/collectors"
)

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

	// Runtime management
	ctx       context.Context
	cancel    context.CancelFunc
	factory   CollectorFactory
	domainCtx *domain.Context
	wg        *sync.WaitGroup
}

// CollectorManager manages runtime enable/disable of collectors
type CollectorManager struct {
	mu         sync.RWMutex
	collectors map[string]*ManagedCollector
	domainCtx  *domain.Context
	wg         *sync.WaitGroup
}

// NewCollectorManager creates a new collector manager
func NewCollectorManager(domainCtx *domain.Context, wg *sync.WaitGroup) *CollectorManager {
	return &CollectorManager{
		collectors: make(map[string]*ManagedCollector),
		domainCtx:  domainCtx,
		wg:         wg,
	}
}

// Register registers a collector with the manager
func (cm *CollectorManager) Register(name string, factory CollectorFactory, interval int, required bool) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	enabled := interval > 0
	status := "stopped"
	if enabled {
		status = "registered"
	} else {
		status = "disabled"
	}

	cm.collectors[name] = &ManagedCollector{
		Name:      name,
		Enabled:   enabled,
		Interval:  interval,
		Status:    status,
		Required:  required,
		factory:   factory,
		domainCtx: cm.domainCtx,
		wg:        cm.wg,
	}

	logger.Debug("Registered collector: %s (interval: %ds, required: %v)", name, interval, required)
}

// StartAll starts all enabled collectors
func (cm *CollectorManager) StartAll() int {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	enabledCount := 0
	for name, mc := range cm.collectors {
		if mc.Enabled && mc.Interval > 0 {
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

	// Create new context for this collector
	ctx, cancel := context.WithCancel(context.Background())
	mc.ctx = ctx
	mc.cancel = cancel

	// Create collector instance
	collector := mc.factory(mc.domainCtx)

	// Start the collector goroutine
	mc.wg.Add(1)
	go func() {
		defer mc.wg.Done()
		collector.Start(ctx, time.Duration(mc.Interval)*time.Second)
	}()

	mc.Status = "running"
	mc.Enabled = true
	now := time.Now()
	mc.LastRun = &now

	logger.Info("Started collector: %s (interval: %ds)", name, mc.Interval)
}

// EnableCollector enables a collector at runtime
func (cm *CollectorManager) EnableCollector(name string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	mc, exists := cm.collectors[name]
	if !exists {
		return fmt.Errorf("unknown collector: %s", name)
	}

	if mc.Status == "running" {
		return nil // Already running
	}

	// Set default interval if not set
	if mc.Interval <= 0 {
		mc.Interval = cm.getDefaultInterval(name)
	}

	cm.startCollectorLocked(name)

	// Broadcast state change
	cm.broadcastStateChange(name, true)

	return nil
}

// DisableCollector disables a collector at runtime
func (cm *CollectorManager) DisableCollector(name string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	mc, exists := cm.collectors[name]
	if !exists {
		return fmt.Errorf("unknown collector: %s", name)
	}

	if mc.Required {
		return fmt.Errorf("cannot disable %s collector (always required)", name)
	}

	if mc.Status != "running" {
		return nil // Already stopped
	}

	// Cancel the collector's context
	if mc.cancel != nil {
		mc.cancel()
		mc.cancel = nil
	}

	mc.Status = "stopped"
	mc.Enabled = false

	logger.Info("Disabled collector: %s", name)

	// Broadcast state change
	cm.broadcastStateChange(name, false)

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

	if intervalSeconds < 5 || intervalSeconds > 3600 {
		return fmt.Errorf("invalid interval: must be between 5 and 3600 seconds")
	}

	wasRunning := mc.Status == "running"

	// Stop the collector if running
	if wasRunning && mc.cancel != nil {
		mc.cancel()
		mc.cancel = nil
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

// GetStatus returns the status of a specific collector
func (cm *CollectorManager) GetStatus(name string) (*dto.CollectorStatus, error) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	mc, exists := cm.collectors[name]
	if !exists {
		return nil, fmt.Errorf("unknown collector: %s", name)
	}

	return &dto.CollectorStatus{
		Name:       mc.Name,
		Enabled:    mc.Enabled,
		Interval:   mc.Interval,
		Status:     mc.Status,
		LastRun:    mc.LastRun,
		ErrorCount: mc.ErrorCount,
		Required:   mc.Required,
	}, nil
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

		statuses = append(statuses, dto.CollectorStatus{
			Name:       mc.Name,
			Enabled:    mc.Enabled,
			Interval:   mc.Interval,
			Status:     mc.Status,
			LastRun:    mc.LastRun,
			ErrorCount: mc.ErrorCount,
			Required:   mc.Required,
		})
	}

	return dto.CollectorsStatusResponse{
		Collectors:    statuses,
		Total:         len(statuses),
		EnabledCount:  enabledCount,
		DisabledCount: disabledCount,
		Timestamp:     time.Now(),
	}
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
			mc.cancel()
			mc.cancel = nil
			mc.Status = "stopped"
			logger.Debug("Stopped collector: %s", name)
		}
	}
}

// broadcastStateChange publishes a collector state change event
func (cm *CollectorManager) broadcastStateChange(name string, enabled bool) {
	event := dto.CollectorStateEvent{
		Event:     "collector_state_change",
		Collector: name,
		Enabled:   enabled,
		Status:    cm.collectors[name].Status,
		Interval:  cm.collectors[name].Interval,
		Timestamp: time.Now(),
	}
	cm.domainCtx.Hub.Pub(event, "collector_state_change")
}

// getDefaultInterval returns the default interval for a collector
func (cm *CollectorManager) getDefaultInterval(name string) int {
	defaults := map[string]int{
		"system":       5,
		"array":        10,
		"disk":         30,
		"docker":       10,
		"vm":           10,
		"ups":          10,
		"nut":          10,
		"gpu":          10,
		"shares":       60,
		"network":      15,
		"hardware":     60,
		"zfs":          30,
		"notification": 30,
		"registration": 300,
		"unassigned":   60,
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
}
