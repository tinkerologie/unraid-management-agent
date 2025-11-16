package collectors

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/lib"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
)

// SystemCollector collects overall system information including CPU, memory, uptime, and temperatures.
// It provides high-level system metrics and status information.
type SystemCollector struct {
	ctx *domain.Context
}

// NewSystemCollector creates a new system information collector with the given context.
func NewSystemCollector(ctx *domain.Context) *SystemCollector {
	return &SystemCollector{ctx: ctx}
}

// Start begins the system collector's periodic data collection.
// It runs in a goroutine and publishes system information updates at the specified interval until the context is cancelled.
func (c *SystemCollector) Start(ctx context.Context, interval time.Duration) {
	logger.Info("Starting system collector (interval: %v)", interval)

	// Run once immediately with panic recovery
	func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("System collector PANIC on startup: %v", r)
			}
		}()
		c.Collect()
	}()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info("System collector stopping due to context cancellation")
			return
		case <-ticker.C:
			func() {
				defer func() {
					if r := recover(); r != nil {
						logger.Error("System collector PANIC in loop: %v", r)
					}
				}()
				c.Collect()
			}()
		}
	}
}

// Collect gathers system information and publishes it to the event bus.
// It collects CPU, memory, uptime, and temperature data from /proc and /sys filesystems.
func (c *SystemCollector) Collect() {
	logger.Debug("Collecting system data...")

	// Collect system info
	systemInfo, err := c.collectSystemInfo()
	if err != nil {
		logger.Error("Failed to collect system info: %v", err)
		return
	}

	// Publish event
	c.ctx.Hub.Pub(systemInfo, "system_update")
	logger.Debug("Published system_update event")
}

func (c *SystemCollector) collectSystemInfo() (*dto.SystemInfo, error) {
	info := &dto.SystemInfo{}

	// Get hostname
	hostname, err := os.Hostname()
	if err != nil {
		logger.Warning("Failed to get hostname", "error", err)
		info.Hostname = "unknown"
	} else {
		info.Hostname = hostname
	}

	// Get uptime
	uptime, err := c.getUptime()
	if err != nil {
		logger.Warning("Failed to get uptime", "error", err)
	} else {
		info.Uptime = uptime
	}

	// Get CPU info
	cpuPercent, err := c.getCPUInfo()
	if err != nil {
		logger.Warning("Failed to get CPU info", "error", err)
	} else {
		info.CPUUsage = cpuPercent
	}

	// Get CPU model and specs
	cpuModel, cpuCores, cpuThreads, cpuMHz := c.getCPUSpecs()
	info.CPUModel = cpuModel
	info.CPUCores = cpuCores
	info.CPUThreads = cpuThreads
	info.CPUMHz = cpuMHz

	// Get per-core CPU usage
	perCoreUsage, err := c.getPerCoreCPUUsage()
	if err != nil {
		logger.Debug("Failed to get per-core CPU usage: %v", err)
	} else {
		info.CPUPerCore = perCoreUsage
	}

	// Get memory info
	memUsed, memTotal, memFree, memBuffers, memCached, err := c.getMemoryInfo()
	if err != nil {
		logger.Warning("Failed to get memory info", "error", err)
	} else {
		info.RAMUsed = memUsed
		info.RAMTotal = memTotal
		info.RAMFree = memFree
		info.RAMBuffers = memBuffers
		info.RAMCached = memCached
		if memTotal > 0 {
			info.RAMUsage = float64(memUsed) / float64(memTotal) * 100
		}
	}

	// Get server model and BIOS info
	serverModel, biosVersion, biosDate := c.getSystemHardwareInfo()
	info.ServerModel = serverModel
	info.BIOSVersion = biosVersion
	info.BIOSDate = biosDate

	// Get temperatures
	temperatures, err := c.getTemperatures()
	if err != nil {
		logger.Warning("Failed to get temperatures", "error", err)
	} else {
		// Extract CPU and motherboard temps if available
		for name, temp := range temperatures {
			nameLower := strings.ToLower(name)
			// CPU temperature - look for Core temps, Package, or CPUTIN
			if strings.Contains(nameLower, "core") || strings.Contains(nameLower, "package") || strings.Contains(nameLower, "cputin") {
				if info.CPUTemp == 0 || temp > info.CPUTemp {
					info.CPUTemp = temp
				}
			}
			// Motherboard temperature - look for "MB Temp" specifically from coretemp
			// Ignore SYSTIN and AUXTIN as they often have bogus readings
			if strings.Contains(nameLower, "mb_temp") || strings.Contains(name, "MB Temp") {
				// Sanity check: temperature should be reasonable (0-100°C)
				if temp > 0 && temp < 100 {
					info.MotherboardTemp = temp
				}
			}
		}
	}

	// Get fan speeds
	fans, err := c.getFans()
	if err != nil {
		logger.Warning("Failed to get fan speeds", "error", err)
	} else {
		info.Fans = fans
	}

	// Get virtualization features
	info.HVMEnabled = c.isHVMEnabled()
	info.IOMMUEnabled = c.isIOMMUEnabled()

	// Get additional system information
	info.OpenSSLVersion = c.getOpenSSLVersion()
	info.KernelVersion = c.getKernelVersion()
	info.ParityCheckSpeed = c.getParityCheckSpeed()

	// Set timestamp
	info.Timestamp = time.Now()

	return info, nil
}

func (c *SystemCollector) getUptime() (int64, error) {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0, err
	}

	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return 0, fmt.Errorf("invalid uptime format")
	}

	uptime, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0, err
	}

	return int64(uptime), nil
}

func (c *SystemCollector) getCPUInfo() (float64, error) {
	// Get CPU usage by reading /proc/stat
	cpuPercent, err := c.calculateCPUPercent()
	if err != nil {
		logger.Warning("Failed to calculate CPU percent", "error", err)
		return 0, err
	}

	return cpuPercent, nil
}

func (c *SystemCollector) calculateCPUPercent() (float64, error) {
	// Read first snapshot
	stat1, err := c.readCPUStat()
	if err != nil {
		return 0, err
	}

	// Wait a short time
	time.Sleep(100 * time.Millisecond)

	// Read second snapshot
	stat2, err := c.readCPUStat()
	if err != nil {
		return 0, err
	}

	// Calculate usage
	total1 := stat1["user"] + stat1["nice"] + stat1["system"] + stat1["idle"] + stat1["iowait"] + stat1["irq"] + stat1["softirq"] + stat1["steal"]
	total2 := stat2["user"] + stat2["nice"] + stat2["system"] + stat2["idle"] + stat2["iowait"] + stat2["irq"] + stat2["softirq"] + stat2["steal"]

	idle1 := stat1["idle"] + stat1["iowait"]
	idle2 := stat2["idle"] + stat2["iowait"]

	totalDelta := total2 - total1
	idleDelta := idle2 - idle1

	if totalDelta == 0 {
		return 0, nil
	}

	usage := (float64(totalDelta-idleDelta) / float64(totalDelta)) * 100
	return usage, nil
}

func (c *SystemCollector) readCPUStat() (map[string]uint64, error) {
	file, err := os.Open("/proc/stat")
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := file.Close(); err != nil {
			logger.Debug("Error closing CPU stat file: %v", err)
		}
	}()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "cpu ") {
			fields := strings.Fields(line)
			if len(fields) < 9 {
				return nil, fmt.Errorf("invalid cpu stat format")
			}

			stat := make(map[string]uint64)
			var err error
			if stat["user"], err = strconv.ParseUint(fields[1], 10, 64); err != nil {
				logger.Warning("Failed to parse CPU user stat: %v", err)
			}
			if stat["nice"], err = strconv.ParseUint(fields[2], 10, 64); err != nil {
				logger.Warning("Failed to parse CPU nice stat: %v", err)
			}
			if stat["system"], err = strconv.ParseUint(fields[3], 10, 64); err != nil {
				logger.Warning("Failed to parse CPU system stat: %v", err)
			}
			if stat["idle"], err = strconv.ParseUint(fields[4], 10, 64); err != nil {
				logger.Warning("Failed to parse CPU idle stat: %v", err)
			}
			if stat["iowait"], err = strconv.ParseUint(fields[5], 10, 64); err != nil {
				logger.Warning("Failed to parse CPU iowait stat: %v", err)
			}
			if stat["irq"], err = strconv.ParseUint(fields[6], 10, 64); err != nil {
				logger.Warning("Failed to parse CPU irq stat: %v", err)
			}
			if stat["softirq"], err = strconv.ParseUint(fields[7], 10, 64); err != nil {
				logger.Warning("Failed to parse CPU softirq stat: %v", err)
			}
			if stat["steal"], err = strconv.ParseUint(fields[8], 10, 64); err != nil {
				logger.Warning("Failed to parse CPU steal stat: %v", err)
			}

			return stat, nil
		}
	}

	return nil, fmt.Errorf("cpu line not found in /proc/stat")
}

func (c *SystemCollector) getMemoryInfo() (uint64, uint64, uint64, uint64, uint64, error) {
	file, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, 0, 0, 0, 0, err
	}
	defer func() {
		if err := file.Close(); err != nil {
			logger.Debug("Error closing meminfo file: %v", err)
		}
	}()

	var memTotal, memFree, memBuffers, memCached uint64

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		key := strings.TrimSuffix(fields[0], ":")
		value, _ := strconv.ParseUint(fields[1], 10, 64)
		value *= 1024 // Convert from KB to bytes

		switch key {
		case "MemTotal":
			memTotal = value
		case "MemFree":
			memFree = value
		case "Buffers":
			memBuffers = value
		case "Cached":
			memCached = value
		}
	}

	if err := scanner.Err(); err != nil {
		return 0, 0, 0, 0, 0, err
	}

	// Calculate used memory (excluding buffers and cache)
	memUsed := memTotal - memFree - memBuffers - memCached
	// Calculate actual free (including buffers and cache)
	memActualFree := memFree + memBuffers + memCached

	return memUsed, memTotal, memActualFree, memBuffers, memCached, nil
}

func (c *SystemCollector) getTemperatures() (map[string]float64, error) {
	// Try using sensors command first
	output, err := lib.ExecCommandOutput("sensors", "-u")
	if err == nil {
		temperatures := c.parseSensorsOutput(output)
		if len(temperatures) > 0 {
			return temperatures, nil
		}
	}

	// Fallback: try reading from /sys/class/hwmon
	temperatures, err := c.readHwmonTemperatures()
	if err != nil {
		return nil, err
	}

	return temperatures, nil
}

func (c *SystemCollector) parseSensorsOutput(output string) map[string]float64 {
	temperatures := make(map[string]float64)
	lines := strings.Split(output, "\n")

	var currentChip string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// New chip/adapter
		if !strings.Contains(line, ":") && !strings.HasPrefix(line, " ") {
			currentChip = line
			continue
		}

		// Temperature input line
		if strings.Contains(line, "_input:") && currentChip != "" {
			parts := strings.Split(line, ":")
			if len(parts) == 2 {
				key := strings.TrimSpace(parts[0])
				valueStr := strings.TrimSpace(parts[1])
				if value, err := strconv.ParseFloat(valueStr, 64); err == nil {
					// Create a friendly name
					name := fmt.Sprintf("%s_%s", currentChip, key)
					name = strings.ReplaceAll(name, " ", "_")
					// sensors -u already outputs in degrees, no need to divide
					temperatures[name] = value
				}
			}
		}
	}

	return temperatures
}

func (c *SystemCollector) readHwmonTemperatures() (map[string]float64, error) {
	temperatures := make(map[string]float64)

	// Read from /sys/class/hwmon/hwmon*/temp*_input
	for i := 0; i < 10; i++ {
		for j := 1; j < 20; j++ {
			path := fmt.Sprintf("/sys/class/hwmon/hwmon%d/temp%d_input", i, j)
			//nolint:gosec // G304: Path is constructed from /sys/class/hwmon system directory with numeric indices
			data, err := os.ReadFile(path)
			if err != nil {
				continue
			}

			value, err := strconv.ParseFloat(strings.TrimSpace(string(data)), 64)
			if err != nil {
				continue
			}

			// Try to get label
			labelPath := fmt.Sprintf("/sys/class/hwmon/hwmon%d/temp%d_label", i, j)
			//nolint:gosec // G304: Path is constructed from /sys/class/hwmon system directory with numeric indices
			labelData, err := os.ReadFile(labelPath)
			label := fmt.Sprintf("hwmon%d_temp%d", i, j)
			if err == nil {
				label = strings.TrimSpace(string(labelData))
			}

			temperatures[label] = value / 1000.0 // Convert from millidegrees
		}
	}

	if len(temperatures) == 0 {
		return nil, fmt.Errorf("no temperature sensors found")
	}

	return temperatures, nil
}

func (c *SystemCollector) getFans() ([]dto.FanInfo, error) {
	fanMap := make(map[string]int)

	// Try using sensors command first
	output, err := lib.ExecCommandOutput("sensors", "-u")
	if err == nil {
		fanMap = c.parseFanSpeeds(output)
	}

	// If no fans found, try fallback
	if len(fanMap) == 0 {
		fanMap, err = c.readHwmonFanSpeeds()
		if err != nil {
			return nil, err
		}
	}

	// Convert map to slice
	fans := make([]dto.FanInfo, 0, len(fanMap))
	for name, rpm := range fanMap {
		fans = append(fans, dto.FanInfo{
			Name: name,
			RPM:  rpm,
		})
	}

	return fans, nil
}

func (c *SystemCollector) parseFanSpeeds(output string) map[string]int {
	fanSpeeds := make(map[string]int)
	lines := strings.Split(output, "\n")

	var currentChip string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// New chip/adapter
		if !strings.Contains(line, ":") && !strings.HasPrefix(line, " ") {
			currentChip = line
			continue
		}

		// Fan input line
		if strings.Contains(line, "fan") && strings.Contains(line, "_input:") && currentChip != "" {
			parts := strings.Split(line, ":")
			if len(parts) == 2 {
				key := strings.TrimSpace(parts[0])
				valueStr := strings.TrimSpace(parts[1])
				if value, err := strconv.Atoi(valueStr); err == nil {
					name := fmt.Sprintf("%s_%s", currentChip, key)
					name = strings.ReplaceAll(name, " ", "_")
					fanSpeeds[name] = value
				}
			}
		}
	}

	return fanSpeeds
}

func (c *SystemCollector) readHwmonFanSpeeds() (map[string]int, error) {
	fanSpeeds := make(map[string]int)

	// Read from /sys/class/hwmon/hwmon*/fan*_input
	for i := 0; i < 10; i++ {
		for j := 1; j < 20; j++ {
			path := fmt.Sprintf("/sys/class/hwmon/hwmon%d/fan%d_input", i, j)
			//nolint:gosec // G304: Path is constructed from /sys/class/hwmon system directory with numeric indices
			data, err := os.ReadFile(path)
			if err != nil {
				continue
			}

			value, err := strconv.Atoi(strings.TrimSpace(string(data)))
			if err != nil {
				continue
			}

			// Try to get label
			labelPath := fmt.Sprintf("/sys/class/hwmon/hwmon%d/fan%d_label", i, j)
			//nolint:gosec // G304: Path is constructed from /sys/class/hwmon system directory with numeric indices
			labelData, err := os.ReadFile(labelPath)
			label := fmt.Sprintf("hwmon%d_fan%d", i, j)
			if err == nil {
				label = strings.TrimSpace(string(labelData))
			}

			fanSpeeds[label] = value
		}
	}

	if len(fanSpeeds) == 0 {
		return nil, fmt.Errorf("no fan sensors found")
	}

	return fanSpeeds, nil
}

// getCPUSpecs reads CPU model, cores, threads, and frequency from /proc/cpuinfo
func (c *SystemCollector) getCPUSpecs() (string, int, int, float64) {
	file, err := os.Open("/proc/cpuinfo")
	if err != nil {
		return "Unknown", 0, 0, 0.0
	}
	defer func() {
		if err := file.Close(); err != nil {
			logger.Debug("Error closing cpuinfo file: %v", err)
		}
	}()

	var cpuModel string
	var cpuMHz float64
	physicalIDs := make(map[string]bool)
	processors := 0

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		switch key {
		case "model name":
			if cpuModel == "" {
				cpuModel = value
			}
		case "cpu MHz":
			if mhz, err := strconv.ParseFloat(value, 64); err == nil && cpuMHz == 0 {
				cpuMHz = mhz
			}
		case "physical id":
			physicalIDs[value] = true
		case "processor":
			processors++
		}
	}

	cpuCores := len(physicalIDs)
	if cpuCores == 0 {
		cpuCores = 1 // Fallback to at least 1 core
	}

	return cpuModel, cpuCores, processors, cpuMHz
}

// getSystemHardwareInfo uses dmidecode to get server model and BIOS info
func (c *SystemCollector) getSystemHardwareInfo() (string, string, string) {
	var serverModel, biosVersion, biosDate string

	// Get system product name (server model)
	if output, err := lib.ExecCommandOutput("dmidecode", "-s", "system-product-name"); err == nil {
		serverModel = strings.TrimSpace(output)
	}

	// Get BIOS version
	if output, err := lib.ExecCommandOutput("dmidecode", "-s", "bios-version"); err == nil {
		biosVersion = strings.TrimSpace(output)
	}

	// Get BIOS release date
	if output, err := lib.ExecCommandOutput("dmidecode", "-s", "bios-release-date"); err == nil {
		biosDate = strings.TrimSpace(output)
	}

	return serverModel, biosVersion, biosDate
}

// getPerCoreCPUUsage calculates per-core CPU usage
func (c *SystemCollector) getPerCoreCPUUsage() (map[string]float64, error) {
	// Read first snapshot
	stat1, err := c.readPerCoreCPUStat()
	if err != nil {
		return nil, err
	}

	// Wait a short time
	time.Sleep(100 * time.Millisecond)

	// Read second snapshot
	stat2, err := c.readPerCoreCPUStat()
	if err != nil {
		return nil, err
	}

	// Calculate usage per core
	perCoreUsage := make(map[string]float64)
	for core, values1 := range stat1 {
		if values2, exists := stat2[core]; exists {
			total1 := values1["user"] + values1["nice"] + values1["system"] + values1["idle"] + values1["iowait"] + values1["irq"] + values1["softirq"] + values1["steal"]
			total2 := values2["user"] + values2["nice"] + values2["system"] + values2["idle"] + values2["iowait"] + values2["irq"] + values2["softirq"] + values2["steal"]

			idle1 := values1["idle"] + values1["iowait"]
			idle2 := values2["idle"] + values2["iowait"]

			totalDelta := total2 - total1
			idleDelta := idle2 - idle1

			if totalDelta > 0 {
				usage := (float64(totalDelta-idleDelta) / float64(totalDelta)) * 100
				perCoreUsage[core] = usage
			}
		}
	}

	return perCoreUsage, nil
}

// readPerCoreCPUStat reads CPU statistics for each core from /proc/stat
func (c *SystemCollector) readPerCoreCPUStat() (map[string]map[string]uint64, error) {
	file, err := os.Open("/proc/stat")
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := file.Close(); err != nil {
			logger.Debug("Error closing per-core CPU stat file: %v", err)
		}
	}()

	coreStats := make(map[string]map[string]uint64)
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := scanner.Text()
		// Look for lines starting with "cpu" followed by a number (cpu0, cpu1, etc.)
		if strings.HasPrefix(line, "cpu") && len(line) > 3 {
			fields := strings.Fields(line)
			if len(fields) < 9 {
				continue
			}

			coreName := fields[0]
			// Skip the aggregate "cpu" line
			if coreName == "cpu" {
				continue
			}

			stat := make(map[string]uint64)
			var parseErr error
			if stat["user"], parseErr = strconv.ParseUint(fields[1], 10, 64); parseErr != nil {
				logger.Debug("Failed to parse per-core CPU user stat for %s: %v", coreName, parseErr)
			}
			if stat["nice"], parseErr = strconv.ParseUint(fields[2], 10, 64); parseErr != nil {
				logger.Debug("Failed to parse per-core CPU nice stat for %s: %v", coreName, parseErr)
			}
			if stat["system"], parseErr = strconv.ParseUint(fields[3], 10, 64); parseErr != nil {
				logger.Debug("Failed to parse per-core CPU system stat for %s: %v", coreName, parseErr)
			}
			if stat["idle"], parseErr = strconv.ParseUint(fields[4], 10, 64); parseErr != nil {
				logger.Debug("Failed to parse per-core CPU idle stat for %s: %v", coreName, parseErr)
			}
			if stat["iowait"], parseErr = strconv.ParseUint(fields[5], 10, 64); parseErr != nil {
				logger.Debug("Failed to parse per-core CPU iowait stat for %s: %v", coreName, parseErr)
			}
			if stat["irq"], parseErr = strconv.ParseUint(fields[6], 10, 64); parseErr != nil {
				logger.Debug("Failed to parse per-core CPU irq stat for %s: %v", coreName, parseErr)
			}
			if stat["softirq"], parseErr = strconv.ParseUint(fields[7], 10, 64); parseErr != nil {
				logger.Debug("Failed to parse per-core CPU softirq stat for %s: %v", coreName, parseErr)
			}
			if stat["steal"], parseErr = strconv.ParseUint(fields[8], 10, 64); parseErr != nil {
				logger.Debug("Failed to parse per-core CPU steal stat for %s: %v", coreName, parseErr)
			}

			coreStats[coreName] = stat
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	if len(coreStats) == 0 {
		return nil, fmt.Errorf("no per-core CPU stats found")
	}

	return coreStats, nil
}

// isHVMEnabled checks if hardware virtualization (HVM) is enabled
// Checks for vmx (Intel) or svm (AMD) flags in /proc/cpuinfo
func (c *SystemCollector) isHVMEnabled() bool {
	data, err := os.ReadFile("/proc/cpuinfo")
	if err != nil {
		return false
	}

	content := string(data)
	// Check for Intel VT-x (vmx) or AMD-V (svm)
	return strings.Contains(content, " vmx ") || strings.Contains(content, " svm ")
}

// isIOMMUEnabled checks if IOMMU is enabled
// Checks kernel command line and /sys/class/iommu/
func (c *SystemCollector) isIOMMUEnabled() bool {
	// Check kernel command line for IOMMU parameters
	cmdline, err := os.ReadFile("/proc/cmdline")
	if err == nil {
		content := string(cmdline)
		if strings.Contains(content, "intel_iommu=on") || strings.Contains(content, "amd_iommu=on") {
			return true
		}
	}

	// Check if /sys/class/iommu/ exists and has entries
	entries, err := os.ReadDir("/sys/class/iommu")
	if err == nil && len(entries) > 0 {
		return true
	}

	return false
}

// getOpenSSLVersion gets the OpenSSL version
func (c *SystemCollector) getOpenSSLVersion() string {
	output, err := lib.ExecCommandOutput("openssl", "version")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(output)
}

// getKernelVersion gets the kernel version
func (c *SystemCollector) getKernelVersion() string {
	output, err := lib.ExecCommandOutput("uname", "-r")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(output)
}

// getParityCheckSpeed gets the parity check speed from var.ini
func (c *SystemCollector) getParityCheckSpeed() string {
	// Try to read from /var/local/emhttp/var.ini
	data, err := os.ReadFile("/var/local/emhttp/var.ini")
	if err != nil {
		return ""
	}

	// Parse for sbSynced line which contains parity check speed
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "sbSynced=") {
			// Extract the speed part (e.g., "18645 MB/s + 38044 MB/s")
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				value := strings.Trim(parts[1], "\"")
				// Look for the speed pattern
				if strings.Contains(value, "MB/s") {
					return value
				}
			}
		}
	}

	return ""
}
