package dto

import "strings"

// DiskHealthClass is the class of a disk in the health summaries
// (get_health_status and get_diagnostic_summary).
type DiskHealthClass string

// Disk health classes returned by DiskInfo.HealthClass.
const (
	DiskHealthExcluded DiskHealthClass = "excluded"
	DiskHealthHealthy  DiskHealthClass = "healthy"
	DiskHealthUnknown  DiskHealthClass = "unknown"
	DiskHealthWarning  DiskHealthClass = "warning"
)

// diskHealthMaxTemperature is the highest healthy temperature in °C.
const diskHealthMaxTemperature = 50.0

// HealthClass classifies the disk for the health summaries. It is nil-safe.
//
// Excluded: a nil disk, the flash, docker_vdisk and log roles, and a vacant
// slot (native status with the prefix DISK_NP and no Device and no ID).
// The other disks are classified in this order of precedence:
//   - Warning: the native status is set and is not DISK_OK, the SMART status
//     is set and is not PASSED or UNKNOWN, or the temperature is above 50 °C.
//   - Unknown: the native status is empty, or the SMART status is UNKNOWN or
//     empty (for example, a disk in standby).
//   - Healthy: DISK_OK, SMART PASSED, and no temperature or at most 50 °C.
func (d *DiskInfo) HealthClass() DiskHealthClass {
	if d == nil {
		return DiskHealthExcluded
	}
	switch d.Role {
	case "flash", "docker_vdisk", "log":
		return DiskHealthExcluded
	}
	if strings.HasPrefix(d.Status, "DISK_NP") && d.Device == "" && d.ID == "" {
		return DiskHealthExcluded
	}
	if (d.Status != "" && d.Status != "DISK_OK") ||
		(d.SMARTStatus != "" && d.SMARTStatus != "PASSED" && d.SMARTStatus != "UNKNOWN") ||
		(d.Temperature != nil && *d.Temperature > diskHealthMaxTemperature) {
		return DiskHealthWarning
	}
	if d.Status == "" || d.SMARTStatus == "" || d.SMARTStatus == "UNKNOWN" {
		return DiskHealthUnknown
	}
	return DiskHealthHealthy
}
