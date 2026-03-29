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
