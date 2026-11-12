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

// A disk can be reassigned or remounted while the agent is running. Only reuse
// capacity for the same disk, slot and filesystem mount.
type diskCapacityIdentity struct {
	diskIdentity
	name, mountPoint, filesystem string
}

type diskCapacitySample struct {
	size, used, free uint64
	usagePercent     float64
}

func (c *DiskCollector) retainDiskCapacity(disk *dto.DiskInfo, idle, measured bool, samples map[diskCapacityIdentity]diskCapacitySample) {
	if disk.ID == "" || disk.Device == "" {
		return
	}
	key := diskCapacityIdentity{diskIdentity{disk.ID, disk.Device}, disk.Name, disk.MountPoint, disk.FileSystem}
	if measured || disk.FreeFromSource {
		samples[key] = diskCapacitySample{disk.Size, disk.Used, disk.Free, disk.UsagePercent}
		return
	}
	if sample, ok := c.capacity[key]; ok {
		samples[key] = sample
		if !idle {
			disk.Size, disk.Used, disk.Free = sample.size, sample.used, sample.free
			disk.UsagePercent = sample.usagePercent
		}
	}
}
