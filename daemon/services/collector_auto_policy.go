package services

import (
	"github.com/tinkerologie/unraid-management-agent/daemon/constants"
	"github.com/tinkerologie/unraid-management-agent/daemon/dto"
	"github.com/tinkerologie/unraid-management-agent/daemon/logger"
	"github.com/tinkerologie/unraid-management-agent/daemon/services/collectors"
)

// autoDisableAfter is the number of consecutive "absent" results that stop a
// collector whose hardware was seen before, so one flaky check does not stop it.
const autoDisableAfter = 2

// collectorAutoPolicy turns hardware-gated collectors on and off from each
// capability snapshot (#261). Absent hardware always wins: the collector is
// stopped whatever its configured interval — at once if the hardware was never
// seen, after two consecutive absent results if it was. Present hardware runs
// it at IntervalAutoEnable (x4 in low power mode) in automatic mode, or at its
// configured interval (a configured 0, "Disabled", keeps it off). Unknown or
// missing results change nothing. Collectors that are not gated (the disabled
// list, manual runtime control) are never touched. Apply is called with
// refreshes serialised.
type collectorAutoPolicy struct {
	cm       *CollectorManager
	lowPower bool
	absent   map[string]int
	seen     map[string]bool
}

// newCollectorAutoPolicy creates the policy for a collector manager.
func newCollectorAutoPolicy(cm *CollectorManager, lowPower bool) *collectorAutoPolicy {
	return &collectorAutoPolicy{cm: cm, lowPower: lowPower, absent: map[string]int{}, seen: map[string]bool{}}
}

// Apply applies the policy to one capability snapshot.
func (p *collectorAutoPolicy) Apply(caps dto.Capabilities) {
	interval := constants.IntervalAutoEnable
	if p.lowPower {
		interval *= 4
	}
	for _, class := range collectors.HardwareClasses {
		if class.Collector == "" {
			continue
		}
		status, err := p.cm.GetStatus(class.Collector)
		if err != nil || !status.HardwareGated {
			continue
		}
		item, ok := caps.Item(class.Capability)
		if !ok {
			continue
		}
		if class.Collector == "psu" {
			p.cm.setPSUPresence(item.State)
		}
		switch item.State {
		case dto.CapabilityPresent:
			p.absent[class.Collector] = 0
			p.seen[class.Collector] = true
			if p.cm.ApplyHardwareState(class.Collector, true, interval) {
				logger.Info("Hardware collectors: %s follows %s, detected (%s)", class.Collector, class.Capability, item.Detail)
			}
		case dto.CapabilityAbsent:
			p.absent[class.Collector]++
			if (!p.seen[class.Collector] || p.absent[class.Collector] >= autoDisableAfter) &&
				p.cm.ApplyHardwareState(class.Collector, false, interval) {
				logger.Info("Hardware collectors: stopped %s, %s not detected (%s)", class.Collector, class.Capability, item.Detail)
			}
		default:
			p.absent[class.Collector] = 0
		}
	}
}
