package services

import (
	"context"

	"github.com/tinkerologie/unraid-management-agent/daemon/constants"
	"github.com/tinkerologie/unraid-management-agent/daemon/domain"
	"github.com/tinkerologie/unraid-management-agent/daemon/logger"
	"github.com/tinkerologie/unraid-management-agent/daemon/services/scheduler"
)

// startRunnerLocked starts a collector's Dynamic-mode runner; cm.mu must be held.
func (cm *CollectorManager) startRunnerLocked(mc *ManagedCollector) {
	policy, ok := scheduler.PolicyFor(mc.Name)
	if !ok {
		logger.Error("Collector %s has no Dynamic-mode policy, not started", mc.Name)
		return
	}
	coll, err := dynamicCollectorFor(mc.factory(mc.domainCtx))
	if err != nil {
		logger.Error("Collector %s not started: %v", mc.Name, err)
		return
	}

	// #nosec G118 -- cancel is stored on ManagedCollector and released by DisableCollector, StopAll, and runner exit.
	ctx, cancel := context.WithCancel(context.Background())
	name := mc.Name
	r := newCollectorRunner(name, policy, coll, cm.clock)
	r.lowPower = cm.domainCtx.LowPowerMode
	r.active = func() bool { return cm.demand.Active(name) }
	r.floor = func() bool { return cm.floorRequired(name) }
	if cm.eventSources != nil {
		r.sources = cm.eventSources(name)
	}
	mc.ctx, mc.cancel, mc.runner = ctx, cancel, r

	mc.wg.Go(func() {
		defer cancel()
		r.run(ctx)
	})

	mc.Status = "running"
	mc.Enabled = true
	logger.Info("Started collector: %s (dynamic)", name)
}

// applyHardwareStateDynamic runs a gated collector while its hardware is
// present and stops it while absent. It takes over cm.mu, held by the caller,
// and releases it.
func (cm *CollectorManager) applyHardwareStateDynamic(mc *ManagedCollector, present bool) bool {
	running := mc.Status == "running"
	switch {
	case present && !running:
		cm.startCollectorLocked(mc.Name)
		logger.Info("Hardware-gated collector started: %s (dynamic)", mc.Name)
	case !present && (running || mc.Enabled):
		cm.stopCollectorLocked(mc)
		mc.Enabled = false
		mc.Status = "disabled"
		logger.Info("Hardware-gated collector stopped: %s", mc.Name)
	default:
		cm.mu.Unlock()
		return false
	}
	event := cm.buildStateEvent(mc.Name, mc.Enabled)
	cm.mu.Unlock()

	domain.Publish(cm.domainCtx.Hub, constants.TopicCollectorStateChange, event)
	return true
}
