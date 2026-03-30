package lib

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// cpufreqPath is a helper to construct cpufreq sysfs paths.
func cpufreqPath(cpu, file string) string {
	return filepath.Join(cpufreqBasePath, cpu, "cpufreq", file)
}

const cpufreqBasePath = "/sys/devices/system/cpu"

// ReadCPUGovernor reads the current CPU scaling governor from cpu0.
func ReadCPUGovernor() (string, error) {
	gov := ReadSysfsString(cpufreqPath("cpu0", "scaling_governor"))
	if gov == "" {
		return "", fmt.Errorf("reading CPU governor: cpufreq not available")
	}
	return gov, nil
}

// ReadAvailableGovernors reads the available CPU scaling governors from cpu0.
func ReadAvailableGovernors() ([]string, error) {
	content := ReadSysfsString(cpufreqPath("cpu0", "scaling_available_governors"))
	if content == "" {
		return nil, fmt.Errorf("reading available governors: cpufreq not available")
	}
	govs := strings.Fields(content)
	if len(govs) == 0 {
		return nil, fmt.Errorf("no available governors found")
	}
	return govs, nil
}

// ReadCPUFreqDriver reads the current cpufreq driver name.
func ReadCPUFreqDriver() string {
	return ReadSysfsString(cpufreqPath("cpu0", "scaling_driver"))
}

// ReadCPUFreqLimits reads the min, max, and current CPU frequencies in kHz,
// then returns them converted to MHz.
func ReadCPUFreqLimits() (minMHz, maxMHz, curMHz int) {
	minMHz = ReadSysfsInt(cpufreqPath("cpu0", "scaling_min_freq")) / 1000
	maxMHz = ReadSysfsInt(cpufreqPath("cpu0", "scaling_max_freq")) / 1000
	curMHz = ReadSysfsInt(cpufreqPath("cpu0", "scaling_cur_freq")) / 1000
	return minMHz, maxMHz, curMHz
}

// WriteCPUGovernor sets the scaling governor for all online CPU cores.
func WriteCPUGovernor(governor string) error {
	// Enumerate all cpuN directories
	entries, err := os.ReadDir(cpufreqBasePath)
	if err != nil {
		return fmt.Errorf("listing CPU directories: %w", err)
	}

	written := 0
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, "cpu") || !entry.IsDir() {
			continue
		}
		// Skip non-numeric suffixes (e.g. cpufreq, cpuidle)
		suffix := strings.TrimPrefix(name, "cpu")
		if len(suffix) == 0 || suffix[0] < '0' || suffix[0] > '9' {
			continue
		}

		govPath := filepath.Join(cpufreqBasePath, name, "cpufreq", "scaling_governor")
		if err := WriteSysfs(govPath, governor); err != nil {
			// Some offline CPUs may not have cpufreq — skip them
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return fmt.Errorf("setting governor on %s: %w", name, err)
		}
		written++
	}

	if written == 0 {
		return fmt.Errorf("no CPU cores accepted the governor change")
	}
	return nil
}

// Turbo boost sysfs paths for Intel and AMD processors.
var (
	intelTurboPath = filepath.Join(cpufreqBasePath, "intel_pstate", "no_turbo")
	amdBoostPath   = filepath.Join(cpufreqBasePath, "cpufreq", "boost")
)

// TurboBoostStatus represents the current turbo/boost state.
type TurboBoostStatus struct {
	Available bool   // Whether turbo/boost control is available
	Enabled   bool   // Whether turbo/boost is currently enabled
	Vendor    string // "intel", "amd", or ""
}

// ReadTurboBoost reads the current Intel Turbo Boost or AMD Performance Boost state.
func ReadTurboBoost() TurboBoostStatus {
	// Try Intel pstate first: no_turbo = 0 means turbo ON, 1 means turbo OFF
	val := ReadSysfsString(intelTurboPath)
	if val != "" {
		return TurboBoostStatus{
			Available: true,
			Enabled:   val == "0",
			Vendor:    "intel",
		}
	}

	// Try AMD boost: boost = 1 means boost ON, 0 means boost OFF
	val = ReadSysfsString(amdBoostPath)
	if val != "" {
		return TurboBoostStatus{
			Available: true,
			Enabled:   val == "1",
			Vendor:    "amd",
		}
	}

	return TurboBoostStatus{}
}

// WriteTurboBoost enables or disables Intel Turbo Boost / AMD Performance Boost.
func WriteTurboBoost(enabled bool) error {
	// Try Intel pstate: write "0" to enable turbo (no_turbo=0), "1" to disable
	if ReadSysfsString(intelTurboPath) != "" {
		val := "0"
		if !enabled {
			val = "1"
		}
		return WriteSysfs(intelTurboPath, val)
	}

	// Try AMD boost: write "1" to enable, "0" to disable
	if ReadSysfsString(amdBoostPath) != "" {
		val := "1"
		if !enabled {
			val = "0"
		}
		return WriteSysfs(amdBoostPath, val)
	}

	return fmt.Errorf("turbo/boost control not available on this system")
}
