package collectors

import "github.com/tinkerologie/unraid-management-agent/daemon/dto"

type diskIdentity struct{ id, device string }

type diskSMARTSample struct {
	status                    string
	attributes                map[string]dto.SMARTAttribute
	powerOnHours, powerCycles uint64
}

// Keep the last measured SMART result for the same disk during maintenance.
// Exit bits describe the current probe only. Native errors, spin state and
// temperature still come from the current run.
func (c *DiskCollector) enrichDiskSMART(disk *dto.DiskInfo, idle bool, samples map[diskIdentity]diskSMARTSample) {
	if disk.Device == "" {
		return
	}
	key := diskIdentity{disk.ID, disk.Device}
	sample, measured := c.smart[key]
	if measured {
		samples[key] = sample
	}
	if idle {
		c.probeSMART(disk)
		if disk.ID != "" && disk.SMARTStatus != "" && disk.SMARTStatus != "UNKNOWN" {
			samples[key] = diskSMARTSample{
				status: disk.SMARTStatus, attributes: disk.SMARTAttributes,
				powerOnHours: disk.PowerOnHours, powerCycles: disk.PowerCycleCount,
			}
		}
		return
	}
	disk.SMARTStatus = "UNKNOWN"
	if measured {
		disk.SMARTStatus = sample.status
		disk.SMARTAttributes = sample.attributes
		disk.PowerOnHours, disk.PowerCycleCount = sample.powerOnHours, sample.powerCycles
	}
}
