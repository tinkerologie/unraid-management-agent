package lib

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
)

// SysPowercapPath is the default path to the powercap sysfs interface.
// This can be overridden in tests.
var SysPowercapPath = "/sys/class/powercap"

// RAPLZone represents a single RAPL power zone (package, core, uncore, or dram).
type RAPLZone struct {
	Name     string // e.g. "package-0", "core", "uncore", "dram"
	EnergyUJ uint64 // Energy counter in microjoules
	MaxRange uint64 // Max energy range before wraparound (microjoules)
}

// RAPLReading represents a snapshot of all RAPL energy counters at a point in time.
type RAPLReading struct {
	Packages []RAPLZone // Top-level package zones (one per socket)
	Core     []RAPLZone // CPU core zones
	Uncore   []RAPLZone // Uncore (GPU, memory controller) zones
	DRAM     []RAPLZone // DRAM zones
	Time     time.Time  // When this reading was taken
}

// RAPLPower represents calculated power consumption in watts.
type RAPLPower struct {
	PackageWatts float64 // Total CPU package power (cores + uncore)
	DRAMWatts    float64 // DRAM power consumption
}

// IsRAPLAvailable checks if the Intel RAPL powercap interface is available.
func IsRAPLAvailable() bool {
	entries, err := os.ReadDir(SysPowercapPath)
	if err != nil {
		return false
	}

	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "intel-rapl:") && !strings.Contains(entry.Name()[len("intel-rapl:"):], ":") {
			return true
		}
	}

	return false
}

// ReadRAPLEnergy reads the current energy counters from all RAPL zones.
// Returns nil if RAPL is not available or cannot be read.
func ReadRAPLEnergy() *RAPLReading {
	entries, err := os.ReadDir(SysPowercapPath)
	if err != nil {
		return nil
	}

	reading := &RAPLReading{
		Time: time.Now(),
	}

	// Find top-level package zones (intel-rapl:0, intel-rapl:1, etc.)
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, "intel-rapl:") {
			continue
		}

		suffix := name[len("intel-rapl:"):]

		// Top-level package zone has no colon in suffix (e.g. "0", "1")
		if !strings.Contains(suffix, ":") {
			zone := readRAPLZone(filepath.Join(SysPowercapPath, name))
			if zone != nil {
				reading.Packages = append(reading.Packages, *zone)
			}

			// Read sub-zones (intel-rapl:0:0, intel-rapl:0:1, etc.)
			readSubZones(reading, name, suffix)
		}
	}

	if len(reading.Packages) == 0 {
		return nil
	}

	return reading
}

// readSubZones reads the sub-zones of a RAPL package.
func readSubZones(reading *RAPLReading, _ string, parentSuffix string) {
	subEntries, err := os.ReadDir(SysPowercapPath)
	if err != nil {
		return
	}

	prefix := "intel-rapl:" + parentSuffix + ":"
	for _, entry := range subEntries {
		if !strings.HasPrefix(entry.Name(), prefix) {
			continue
		}

		zone := readRAPLZone(filepath.Join(SysPowercapPath, entry.Name()))
		if zone == nil {
			continue
		}

		switch zone.Name {
		case "core":
			reading.Core = append(reading.Core, *zone)
		case "uncore":
			reading.Uncore = append(reading.Uncore, *zone)
		case "dram":
			reading.DRAM = append(reading.DRAM, *zone)
		default:
			logger.Debug("Unknown RAPL sub-zone: %s", zone.Name)
		}
	}
}

// readRAPLZone reads a single RAPL zone's data.
func readRAPLZone(zonePath string) *RAPLZone {
	name := readSysfsFile(filepath.Join(zonePath, "name"))
	if name == "" {
		return nil
	}

	energyStr := readSysfsFile(filepath.Join(zonePath, "energy_uj"))
	if energyStr == "" {
		return nil
	}

	energy, err := strconv.ParseUint(energyStr, 10, 64)
	if err != nil {
		return nil
	}

	zone := &RAPLZone{
		Name:     name,
		EnergyUJ: energy,
	}

	// Read max energy range for wraparound detection
	maxRangeStr := readSysfsFile(filepath.Join(zonePath, "max_energy_range_uj"))
	if maxRangeStr != "" {
		maxRange, err := strconv.ParseUint(maxRangeStr, 10, 64)
		if err == nil {
			zone.MaxRange = maxRange
		}
	}

	return zone
}

// CalculateRAPLPower computes power in watts from two consecutive RAPL readings.
// Returns nil if either reading is nil or if the time delta is too small.
func CalculateRAPLPower(prev, curr *RAPLReading) *RAPLPower {
	if prev == nil || curr == nil {
		return nil
	}

	elapsed := curr.Time.Sub(prev.Time).Seconds()
	if elapsed <= 0 {
		return nil
	}

	power := &RAPLPower{}

	// Calculate package power (summed across sockets)
	power.PackageWatts = calculateZonePower(prev.Packages, curr.Packages, elapsed)

	// Calculate DRAM power (summed across sockets)
	power.DRAMWatts = calculateZonePower(prev.DRAM, curr.DRAM, elapsed)

	return power
}

// calculateZonePower computes total power for a set of zones matched by position.
// This is safe because Linux sysfs enumerates powercap zones in deterministic order.
func calculateZonePower(prev, curr []RAPLZone, elapsedSeconds float64) float64 {
	var totalWatts float64

	for i := range curr {
		if i >= len(prev) {
			break
		}

		deltaUJ := energyDelta(prev[i].EnergyUJ, curr[i].EnergyUJ, curr[i].MaxRange)
		watts := float64(deltaUJ) / (elapsedSeconds * 1_000_000) // µJ → J/s (watts)
		totalWatts += watts
	}

	return totalWatts
}

// energyDelta calculates the difference between two energy readings,
// handling counter wraparound using max_energy_range_uj.
func energyDelta(prev, curr, maxRange uint64) uint64 {
	if curr >= prev {
		return curr - prev
	}

	// Counter wrapped around
	if maxRange > 0 {
		return (maxRange - prev) + curr
	}

	// Fallback: assume 64-bit wraparound (shouldn't happen in practice)
	return curr + (^uint64(0) - prev) + 1
}

// FormatRAPLPower returns a human-readable string of the power reading.
func FormatRAPLPower(power *RAPLPower) string {
	if power == nil {
		return "RAPL power data unavailable"
	}

	return fmt.Sprintf("CPU Package: %.2f W, DRAM: %.2f W", power.PackageWatts, power.DRAMWatts)
}
