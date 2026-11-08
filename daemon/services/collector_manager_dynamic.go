package services

import (
	"github.com/tinkerologie/unraid-management-agent/daemon/constants"
	"github.com/tinkerologie/unraid-management-agent/daemon/domain"
	"github.com/tinkerologie/unraid-management-agent/daemon/logger"
)

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
