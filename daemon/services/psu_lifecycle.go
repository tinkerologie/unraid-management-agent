package services

import (
	"context"
	"sync"
	"time"

	"github.com/tinkerologie/unraid-management-agent/daemon/constants"
	"github.com/tinkerologie/unraid-management-agent/daemon/domain"
	"github.com/tinkerologie/unraid-management-agent/daemon/dto"
	"github.com/tinkerologie/unraid-management-agent/daemon/logger"
	"github.com/tinkerologie/unraid-management-agent/daemon/services/collectors"
)

// publishPSU serializes publication with removal and disable. A cancelled run
// cannot overwrite the unavailable snapshot after its collector has stopped.
func (cm *CollectorManager) publishPSU(ctx context.Context, status *dto.PSUStatus) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	if ctx.Err() != nil {
		return
	}
	mc := cm.collectors["psu"]
	if mc == nil || mc.Status != "running" {
		return
	}
	if mc.Gated && !cm.psuPresent && len(status.Devices) > 0 {
		return
	}
	cm.psuAnnounced = status.State
	domain.Publish(cm.domainCtx.Hub, constants.TopicPSUStatusUpdate, status)
}

// announcePSULocked publishes availability transitions only; cm.mu is held.
// Empty-hardware safety scans must not create periodic WebSocket/MQTT traffic.
func (cm *CollectorManager) announcePSULocked(state string) {
	if state == cm.psuAnnounced {
		return
	}
	cm.psuAnnounced = state
	domain.Publish(cm.domainCtx.Hub, constants.TopicPSUStatusUpdate, dto.NewPSUStatus(state, time.Now()))
}

// initializePSUAvailability runs after API/MQTT subscriptions are ready. Register
// runs earlier, when a boot-time disabled snapshot would be silently lost.
func (cm *CollectorManager) initializePSUAvailability() {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	mc := cm.collectors["psu"]
	if mc != nil && (cm.domainCtx.DisabledCollectors["psu"] || (!cm.dynamic && mc.configured == 0 && !mc.Auto)) {
		cm.announcePSULocked(dto.PSUStateDisabled)
	}
}

func (cm *CollectorManager) setPSUPresence(state dto.CapabilityState) {
	if state == dto.CapabilityUnknown {
		return
	}
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.psuPresent = state == dto.CapabilityPresent
	mc := cm.collectors["psu"]
	if mc == nil || !mc.Gated {
		return
	}
	if !cm.dynamic && mc.configured == 0 && !mc.Auto {
		cm.announcePSULocked(dto.PSUStateDisabled)
	} else if !cm.psuPresent {
		cm.announcePSULocked(dto.PSUStateNotDetected)
	} else {
		cm.psuAnnounced = ""
	}
}

// watchPSUInventory runs independently of the telemetry collector so an unplugged
// collector can discover reconnection. Scoped refreshes do not re-probe other
// hardware or advance their absence counters. The poll catches late driver binds.
func watchPSUInventory(ctx context.Context, refresher *CapabilityRefresher) {
	runPSUInventory(ctx, refresher, 30*time.Second, collectors.NewUeventSource(collectors.PSUDeviceUevent))
}

func runPSUInventory(ctx context.Context, refresher *CapabilityRefresher, interval time.Duration, source eventSource) {
	changed := make(chan struct{}, 1)
	var wg sync.WaitGroup
	wg.Go(func() {
		defer func() {
			if r := recover(); r != nil {
				logger.LogPanicWithStack("PSU inventory events", r)
			}
		}()
		source.Run(ctx, func() {
			select {
			case changed <- struct{}{}:
			default:
			}
		})
	})
	defer wg.Wait()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	refresh := func() {
		defer func() {
			if r := recover(); r != nil {
				logger.LogPanicWithStack("PSU inventory refresh", r)
			}
		}()
		refresher.RefreshCapability(ctx, collectors.CapabilityPSU)
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			refresh()
		case <-changed:
			refresh()
		}
	}
}
