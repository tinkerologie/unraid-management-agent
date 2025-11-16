package collectors

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/lib"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
)

// GPUCollector collects GPU metrics from NVIDIA, AMD, and Intel GPUs.
// It gathers temperature, utilization, memory usage, and power consumption data.
type GPUCollector struct {
	ctx *domain.Context
}

// NewGPUCollector creates a new GPU metrics collector with the given context.
func NewGPUCollector(ctx *domain.Context) *GPUCollector {
	return &GPUCollector{ctx: ctx}
}

// Start begins the GPU collector's periodic data collection.
// It runs in a goroutine and publishes GPU metrics updates at the specified interval until the context is cancelled.
func (c *GPUCollector) Start(ctx context.Context, interval time.Duration) {
	logger.Info("Starting gpu collector (interval: %v)", interval)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info("GPU collector stopping due to context cancellation")
			return
		case <-ticker.C:
			c.Collect()
		}
	}
}

// Collect gathers GPU metrics from all available GPUs and publishes them to the event bus.
// It detects and collects data from NVIDIA, AMD, and Intel GPUs using vendor-specific tools.
func (c *GPUCollector) Collect() {
	logger.Debug("Collecting gpu data...")

	// Collect GPU metrics from all available GPU types
	gpuMetrics := make([]*dto.GPUMetrics, 0)

	// Try Intel iGPU
	logger.Debug("Attempting Intel GPU collection...")
	intelGPUs, err := c.collectIntelGPU()
	logger.Debug("Intel GPU collection returned: gpus=%d, err=%v", len(intelGPUs), err)
	if err == nil && len(intelGPUs) > 0 {
		gpuMetrics = append(gpuMetrics, intelGPUs...)
		logger.Debug("Collected %d Intel GPU(s)", len(intelGPUs))
	} else if err != nil {
		logger.Debug("Intel GPU collection failed: %v", err)
	}

	// Try NVIDIA GPU
	if lib.CommandExists("nvidia-smi") {
		if nvidiaGPUs, err := c.collectNvidiaGPU(); err == nil && len(nvidiaGPUs) > 0 {
			gpuMetrics = append(gpuMetrics, nvidiaGPUs...)
			logger.Debug("Collected %d NVIDIA GPU(s)", len(nvidiaGPUs))
		}
	}

	// Try AMD GPU (radeontop or rocm-smi)
	if lib.CommandExists("radeontop") || lib.CommandExists("rocm-smi") {
		if amdGPUs, err := c.collectAMDGPU(); err == nil && len(amdGPUs) > 0 {
			gpuMetrics = append(gpuMetrics, amdGPUs...)
			logger.Debug("Collected %d AMD GPU(s)", len(amdGPUs))
		}
	}

	if len(gpuMetrics) == 0 {
		logger.Debug("No GPUs detected or no monitoring tools available")
		return
	}

	// Publish event
	c.ctx.Hub.Pub(gpuMetrics, "gpu_metrics_update")
	logger.Debug("Published gpu_metrics_update event for %d total GPU(s)", len(gpuMetrics))
}

// Intel GPU collection using intel_gpu_top
func (c *GPUCollector) collectIntelGPU() ([]*dto.GPUMetrics, error) {
	logger.Debug("Intel GPU: Starting Intel GPU detection")

	// First check if Intel GPU exists using lspci
	output, err := lib.ExecCommandOutput("lspci", "-Dmm")
	if err != nil {
		logger.Debug("Intel GPU: lspci query failed: %v", err)
		return nil, fmt.Errorf("lspci query failed: %w", err)
	}

	logger.Debug("Intel GPU: Got lspci output, searching for Intel VGA")

	// Collect ALL Intel GPUs (not just the first one)
	type intelGPUInfo struct {
		PCIID string
		Model string
	}
	intelGPUs := make([]intelGPUInfo, 0)

	for _, line := range strings.Split(output, "\n") {
		if (strings.Contains(line, "VGA") || strings.Contains(line, "Display")) && strings.Contains(line, "Intel Corporation") {
			logger.Debug("Intel GPU: Found Intel GPU line: %s", line)
			// Parse PCI ID and model name using a more robust approach
			// Format: "0000:00:02.0" "VGA compatible controller" "Intel Corporation" "CoffeeLake-S GT2 [UHD Graphics 630]" -p00 "ASRock Incorporation" "Device 3e92"

			var pciID, model string

			// Extract PCI ID (everything before first quote)
			firstQuote := strings.Index(line, "\"")
			if firstQuote > 0 {
				pciID = strings.TrimSpace(line[:firstQuote])
			}

			// Extract all quoted strings using regex
			// Format: "VGA compatible controller" "Intel Corporation" "CoffeeLake-S GT2 [UHD Graphics 630]" -p00 "ASRock Incorporation" "Device 3e92"
			// Indices: [0]=class, [1]=vendor, [2]=device_name, [3]=subsys_vendor, [4]=subsys_device
			re := regexp.MustCompile(`"([^"]*)"`)
			matches := re.FindAllStringSubmatch(line, -1)

			// The 3rd quoted string (index 2) is the device name
			if len(matches) >= 3 {
				fullModel := matches[2][1] // matches[2][0] is the full match with quotes, [1] is the captured group
				logger.Debug("Intel GPU: Full model string: %s", fullModel)

				// Extract just the marketing name from brackets if present
				if strings.Contains(fullModel, "[") {
					start := strings.Index(fullModel, "[")
					end := strings.Index(fullModel, "]")
					if start != -1 && end != -1 && end > start {
						model = strings.TrimSpace(fullModel[start+1 : end])
					}
				} else {
					// No brackets, use the full model name
					model = fullModel
				}
				logger.Debug("Intel GPU: Parsed - ID: %s, Model: %s", pciID, model)
			} else {
				logger.Debug("Intel GPU: Failed to parse model name from line (found %d quoted strings, need at least 3)", len(matches))
			}

			if pciID != "" && model != "" {
				intelGPUs = append(intelGPUs, intelGPUInfo{PCIID: pciID, Model: model})
			}
			// REMOVED: break statement - continue searching for more Intel GPUs
		}
	}

	if len(intelGPUs) == 0 {
		logger.Debug("Intel GPU: No Intel GPU found in lspci output")
		return nil, fmt.Errorf("no Intel GPU found")
	}

	logger.Debug("Intel GPU: Found %d Intel GPU(s)", len(intelGPUs))

	// Check if intel_gpu_top is available
	if !lib.CommandExists("intel_gpu_top") {
		logger.Debug("Intel GPU: intel_gpu_top command not found")
		return nil, fmt.Errorf("intel_gpu_top not found")
	}

	logger.Debug("Intel GPU: intel_gpu_top found, collecting metrics for each GPU")

	// Collect metrics for each Intel GPU
	gpuMetrics := make([]*dto.GPUMetrics, 0, len(intelGPUs))
	for idx, intelGPU := range intelGPUs {
		gpu := c.collectSingleIntelGPU(intelGPU.PCIID, intelGPU.Model, idx)
		if gpu != nil {
			gpuMetrics = append(gpuMetrics, gpu)
		}
	}

	if len(gpuMetrics) == 0 {
		return nil, fmt.Errorf("failed to collect metrics for any Intel GPU")
	}

	return gpuMetrics, nil
}

// collectSingleIntelGPU collects metrics for a single Intel GPU
func (c *GPUCollector) collectSingleIntelGPU(pciID, model string, index int) *dto.GPUMetrics {
	logger.Debug("Intel GPU: Collecting metrics for GPU %d (%s)", index, pciID)

	// Run intel_gpu_top in JSON mode with 2 samples
	// Note: intel_gpu_top auto-detects Intel GPU, doesn't support -d flag for specific device
	// For multi-GPU systems, we run it once and it reports the first GPU
	// This is a limitation of intel_gpu_top
	cmdOutput, err := lib.ExecCommandOutput("timeout", "5", "intel_gpu_top", "-J", "-s", "1000", "-n", "2")
	if err != nil && len(cmdOutput) == 0 {
		logger.Debug("Intel GPU: intel_gpu_top query failed with no output: %v", err)
		return nil
	} else if err != nil {
		logger.Debug("Intel GPU: intel_gpu_top timed out (expected), got %d bytes output", len(cmdOutput))
	} else {
		logger.Debug("Intel GPU: Got output from intel_gpu_top (%d bytes)", len(cmdOutput))
	}

	// Parse JSON output - intel_gpu_top returns malformed JSON array with -n 2
	stdout := strings.TrimSpace(cmdOutput)
	stdout = strings.ReplaceAll(stdout, "\n", "")
	stdout = strings.ReplaceAll(stdout, "\t", "")

	// Find the first { and match its closing }
	startIdx := strings.Index(stdout, "{")
	if startIdx == -1 {
		logger.Debug("Intel GPU: No JSON object found in output")
		return nil
	}

	// Simple brace matching to find the complete first JSON object
	braceCount := 0
	endIdx := -1
	for i := startIdx; i < len(stdout); i++ {
		if stdout[i] == '{' {
			braceCount++
		} else if stdout[i] == '}' {
			braceCount--
			if braceCount == 0 {
				endIdx = i + 1
				break
			}
		}
	}

	if endIdx == -1 {
		logger.Debug("Intel GPU: Incomplete JSON object in output")
		return nil
	}

	sampleJSON := stdout[startIdx:endIdx]
	logger.Debug("Intel GPU: Extracted JSON object of %d chars", len(sampleJSON))

	// Parse the sample
	var intelData map[string]interface{}
	if err := json.Unmarshal([]byte(sampleJSON), &intelData); err != nil {
		logger.Debug("Intel GPU: Failed to parse sample: %v", err)
		return nil
	}

	logger.Debug("Intel GPU: Successfully parsed sample for GPU %d", index)

	gpu := &dto.GPUMetrics{
		Available: true,
		Index:     index,
		PCIID:     pciID,
		Vendor:    "intel",
		Name:      "Intel " + model,
		Timestamp: time.Now(),
	}

	// Extract driver version from modinfo
	if driverVersion, err := c.getIntelDriverVersion(); err == nil {
		gpu.DriverVersion = driverVersion
	}

	// Extract utilization from engines
	if engines, ok := intelData["engines"].(map[string]interface{}); ok {
		// Sum up all engine utilizations for overall GPU usage
		totalUtil := 0.0
		engineCount := 0
		for engineName, engineData := range engines {
			if engineMap, ok := engineData.(map[string]interface{}); ok {
				if busy, ok := engineMap["busy"].(float64); ok {
					totalUtil += busy
					engineCount++
					logger.Debug("Intel GPU engine %s: %.2f%%", engineName, busy)
				}
			}
		}
		if engineCount > 0 {
			gpu.UtilizationGPU = totalUtil / float64(engineCount)
		}
	}

	// Extract power consumption (GPU power, not package power)
	if power, ok := intelData["power"].(map[string]interface{}); ok {
		if gpuPower, ok := power["GPU"].(float64); ok {
			gpu.PowerDraw = gpuPower
			logger.Debug("Intel GPU power: %.3f W", gpuPower)
		}
	}

	// Extract memory info (Note: Intel iGPU shares system RAM, intel_gpu_top doesn't report memory usage)
	// The "memory" field is not present in intel_gpu_top JSON output for integrated GPUs
	if memory, ok := intelData["memory"].(map[string]interface{}); ok {
		if total, ok := memory["total"].(float64); ok {
			gpu.MemoryTotal = uint64(total)
		}
		if shared, ok := memory["shared"].(float64); ok {
			gpu.MemoryUsed = uint64(shared)
		}
		if gpu.MemoryTotal > 0 {
			gpu.UtilizationMemory = float64(gpu.MemoryUsed) / float64(gpu.MemoryTotal) * 100
		}
	}

	// Intel iGPU typically doesn't report temperature via intel_gpu_top or sysfs hwmon
	// Most Intel integrated GPUs don't expose temperature sensors
	if temp, err := c.getIntelGPUTemp(); err == nil {
		gpu.Temperature = temp
	}

	// For Intel iGPUs, add CPU temperature as they share the die with the CPU
	// This provides useful thermal information since iGPUs don't have dedicated temp sensors
	if cpuTemp, err := c.getCPUTemp(); err == nil {
		gpu.CPUTemperature = cpuTemp
		logger.Debug("Intel GPU: CPU temperature: %.1f°C", cpuTemp)
	}

	return gpu
}

// Get Intel GPU temperature from sysfs
func (c *GPUCollector) getIntelGPUTemp() (float64, error) {
	// Intel iGPU temp is usually in hwmon under i915
	output, err := lib.ExecCommandOutput("bash", "-c", "cat /sys/class/drm/card*/device/hwmon/hwmon*/temp1_input 2>/dev/null | head -1")
	if err != nil || output == "" {
		return 0, fmt.Errorf("failed to read Intel GPU temperature")
	}

	tempMilliC, err := strconv.ParseFloat(strings.TrimSpace(output), 64)
	if err != nil {
		return 0, err
	}

	// Convert from millidegrees to degrees
	return tempMilliC / 1000.0, nil
}

// Get CPU temperature from coretemp hwmon
// This is useful for Intel iGPUs since they share the die with the CPU
func (c *GPUCollector) getCPUTemp() (float64, error) {
	// Try to find coretemp hwmon device
	// Look for hwmon device with name "coretemp"
	output, err := lib.ExecCommandOutput("bash", "-c", "for d in /sys/class/hwmon/hwmon*; do if [ -f $d/name ] && grep -q coretemp $d/name 2>/dev/null; then cat $d/temp1_input 2>/dev/null && exit 0; fi; done")
	if err != nil || output == "" {
		return 0, fmt.Errorf("failed to read CPU temperature from coretemp")
	}

	tempMilliC, err := strconv.ParseFloat(strings.TrimSpace(output), 64)
	if err != nil {
		return 0, err
	}

	// Convert from millidegrees to degrees
	// temp1 is typically the package temperature (overall CPU temp)
	return tempMilliC / 1000.0, nil
}

// Get Intel GPU driver version from modinfo
func (c *GPUCollector) getIntelDriverVersion() (string, error) {
	// Get vermagic from modinfo i915 (contains kernel version)
	output, err := lib.ExecCommandOutput("modinfo", "i915")
	if err != nil {
		return "", fmt.Errorf("modinfo i915 failed: %w", err)
	}

	// Parse vermagic line: "vermagic:       6.12.24-Unraid SMP preempt mod_unload"
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "vermagic:") {
			// Extract kernel version from vermagic
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				// Return kernel version (e.g., "6.12.24-Unraid")
				return parts[1], nil
			}
		}
	}

	return "", fmt.Errorf("failed to parse driver version from modinfo")
}

// NVIDIA GPU collection using nvidia-smi
func (c *GPUCollector) collectNvidiaGPU() ([]*dto.GPUMetrics, error) {
	// Query nvidia-smi with CSV output for easy parsing
	// Added: pci.bus_id, uuid, fan.speed
	output, err := lib.ExecCommandOutput(
		"nvidia-smi",
		"--query-gpu=index,pci.bus_id,uuid,name,temperature.gpu,utilization.gpu,memory.used,memory.total,power.draw,fan.speed",
		"--format=csv,noheader,nounits",
	)
	if err != nil {
		return nil, fmt.Errorf("nvidia-smi query failed: %w", err)
	}

	// Parse CSV output
	reader := csv.NewReader(strings.NewReader(output))
	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("failed to parse CSV output: %w", err)
	}

	gpus := make([]*dto.GPUMetrics, 0, len(records))

	for _, record := range records {
		if len(record) < 10 {
			continue
		}

		gpu := &dto.GPUMetrics{
			Available: true,
			Vendor:    "nvidia",
			Timestamp: time.Now(),
		}

		// Index
		if idx, err := strconv.Atoi(strings.TrimSpace(record[0])); err == nil {
			gpu.Index = idx
		}

		// PCI Bus ID
		gpu.PCIID = strings.TrimSpace(record[1])

		// UUID
		gpu.UUID = strings.TrimSpace(record[2])

		// Name
		gpu.Name = strings.TrimSpace(record[3])

		// Temperature (°C)
		if temp, err := strconv.ParseFloat(strings.TrimSpace(record[4]), 64); err == nil {
			gpu.Temperature = temp
		}

		// Utilization (%)
		if util, err := strconv.ParseFloat(strings.TrimSpace(record[5]), 64); err == nil {
			gpu.UtilizationGPU = util
		}

		// Memory Used (MiB)
		if memUsed, err := strconv.ParseFloat(strings.TrimSpace(record[6]), 64); err == nil {
			gpu.MemoryUsed = uint64(memUsed * 1024 * 1024) // Convert MiB to bytes
		}

		// Memory Total (MiB)
		if memTotal, err := strconv.ParseFloat(strings.TrimSpace(record[7]), 64); err == nil {
			gpu.MemoryTotal = uint64(memTotal * 1024 * 1024) // Convert MiB to bytes
			if gpu.MemoryTotal > 0 {
				gpu.UtilizationMemory = float64(gpu.MemoryUsed) / float64(gpu.MemoryTotal) * 100
			}
		}

		// Power Draw (W)
		if power, err := strconv.ParseFloat(strings.TrimSpace(record[8]), 64); err == nil {
			gpu.PowerDraw = power
		}

		// Fan Speed (%)
		if fanSpeed, err := strconv.ParseFloat(strings.TrimSpace(record[9]), 64); err == nil {
			gpu.FanSpeed = fanSpeed
		}

		// Get driver version (same for all GPUs, only query once)
		if len(gpus) == 0 {
			if driverVersion, err := c.getNvidiaDriverVersion(); err == nil {
				gpu.DriverVersion = driverVersion
			}
		} else {
			gpu.DriverVersion = gpus[0].DriverVersion
		}

		gpus = append(gpus, gpu)
	}

	return gpus, nil
}

// getNvidiaDriverVersion gets NVIDIA driver version
func (c *GPUCollector) getNvidiaDriverVersion() (string, error) {
	output, err := lib.ExecCommandOutput("nvidia-smi", "--query-gpu=driver_version", "--format=csv,noheader")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(output), nil
}

// AMD GPU collection using radeontop (broader AMD GPU compatibility)
func (c *GPUCollector) collectAMDGPU() ([]*dto.GPUMetrics, error) {
	// Try radeontop first (supports consumer Radeon GPUs)
	if lib.CommandExists("radeontop") {
		return c.collectAMDGPUWithRadeontop()
	}

	// Fallback to rocm-smi for datacenter GPUs
	if lib.CommandExists("rocm-smi") {
		return c.collectAMDGPUWithROCm()
	}

	return nil, fmt.Errorf("neither radeontop nor rocm-smi found")
}

// collectAMDGPUWithRadeontop uses radeontop for consumer AMD GPUs
func (c *GPUCollector) collectAMDGPUWithRadeontop() ([]*dto.GPUMetrics, error) {
	// First, detect AMD GPUs using lspci
	output, err := lib.ExecCommandOutput("lspci", "-Dmm")
	if err != nil {
		return nil, fmt.Errorf("lspci query failed: %w", err)
	}

	type amdGPUInfo struct {
		PCIID string
		Model string
	}
	amdGPUs := make([]amdGPUInfo, 0)

	for _, line := range strings.Split(output, "\n") {
		if (strings.Contains(line, "VGA") || strings.Contains(line, "Display")) &&
			(strings.Contains(line, "AMD") || strings.Contains(line, "Advanced Micro Devices") || strings.Contains(line, "ATI")) {

			var pciID, model string

			// Extract PCI ID
			firstQuote := strings.Index(line, "\"")
			if firstQuote > 0 {
				pciID = strings.TrimSpace(line[:firstQuote])
			}

			// Extract model name
			re := regexp.MustCompile(`"([^"]*)"`)
			matches := re.FindAllStringSubmatch(line, -1)
			if len(matches) >= 3 {
				fullModel := matches[2][1]
				// Extract marketing name from brackets if present
				if strings.Contains(fullModel, "[") {
					start := strings.Index(fullModel, "[")
					end := strings.Index(fullModel, "]")
					if start != -1 && end != -1 && end > start {
						model = strings.TrimSpace(fullModel[start+1 : end])
					}
				} else {
					model = fullModel
				}
			}

			if pciID != "" && model != "" {
				amdGPUs = append(amdGPUs, amdGPUInfo{PCIID: pciID, Model: model})
			}
		}
	}

	if len(amdGPUs) == 0 {
		return nil, fmt.Errorf("no AMD GPU found")
	}

	logger.Debug("AMD GPU: Found %d AMD GPU(s)", len(amdGPUs))

	// Collect metrics for each AMD GPU
	gpuMetrics := make([]*dto.GPUMetrics, 0, len(amdGPUs))
	for idx, amdGPU := range amdGPUs {
		gpu := c.collectSingleAMDGPU(amdGPU.PCIID, amdGPU.Model, idx)
		if gpu != nil {
			gpuMetrics = append(gpuMetrics, gpu)
		}
	}

	return gpuMetrics, nil
}

// collectSingleAMDGPU collects metrics for a single AMD GPU using radeontop
func (c *GPUCollector) collectSingleAMDGPU(pciID, model string, index int) *dto.GPUMetrics {
	logger.Debug("AMD GPU: Collecting metrics for GPU %d (%s)", index, pciID)

	// Run radeontop with dump mode: radeontop -d - -l 1
	// Output format: bus 0000:01:00.0, gpu 45.00%, ee 0.00%, vgt 0.00%, ta 0.00%, tc 0.00%, sx 0.00%, sh 0.00%, spi 0.00%, sc 0.00%, pa 0.00%, db 0.00%, cb 0.00%, vram 15.00% 1234mb, gtt 5.00% 123mb, mclk 100.00% 1.750ghz, sclk 50.00% 1.200ghz
	cmdOutput, err := lib.ExecCommandOutput("timeout", "3", "radeontop", "-d", "-", "-l", "1")
	if err != nil && len(cmdOutput) == 0 {
		logger.Debug("AMD GPU: radeontop query failed: %v", err)
		return nil
	}

	gpu := &dto.GPUMetrics{
		Available: true,
		Index:     index,
		PCIID:     pciID,
		Vendor:    "amd",
		Name:      "AMD " + model,
		Timestamp: time.Now(),
	}

	// Parse radeontop output
	// Example: bus 0000:03:00.0, gpu 12.34%, vram 25.50% 2048mb, sclk 50.00% 1.200ghz
	output := strings.TrimSpace(cmdOutput)
	if output != "" {
		// Extract GPU utilization
		if matches := regexp.MustCompile(`gpu\s+([\d.]+)%`).FindStringSubmatch(output); len(matches) > 1 {
			if util, err := strconv.ParseFloat(matches[1], 64); err == nil {
				gpu.UtilizationGPU = util
			}
		}

		// Extract VRAM usage: "vram 15.00% 1234mb"
		if matches := regexp.MustCompile(`vram\s+([\d.]+)%\s+([\d]+)mb`).FindStringSubmatch(output); len(matches) > 2 {
			if vramPercent, err := strconv.ParseFloat(matches[1], 64); err == nil {
				gpu.UtilizationMemory = vramPercent
			}
			if vramUsedMB, err := strconv.ParseUint(matches[2], 10, 64); err == nil {
				gpu.MemoryUsed = vramUsedMB * 1024 * 1024 // Convert MB to bytes
				// Calculate total from percentage
				if gpu.UtilizationMemory > 0 {
					gpu.MemoryTotal = uint64(float64(gpu.MemoryUsed) / (gpu.UtilizationMemory / 100.0))
				}
			}
		}
	}

	// Get temperature from sysfs
	if temp, err := c.getAMDGPUTemp(index); err == nil {
		gpu.Temperature = temp
	}

	// Get fan speed from sysfs (discrete GPUs only)
	if fanRPM, fanMaxRPM, err := c.getAMDGPUFanSpeed(index); err == nil {
		gpu.FanRPM = fanRPM
		gpu.FanMaxRPM = fanMaxRPM
	}

	// Get driver version
	if driverVersion, err := c.getAMDDriverVersion(); err == nil {
		gpu.DriverVersion = driverVersion
	}

	return gpu
}

// getAMDGPUTemp gets AMD GPU temperature from sysfs
func (c *GPUCollector) getAMDGPUTemp(cardIndex int) (float64, error) {
	// AMD GPU temp is in hwmon
	output, err := lib.ExecCommandOutput("bash", "-c", fmt.Sprintf("cat /sys/class/drm/card%d/device/hwmon/hwmon*/temp1_input 2>/dev/null | head -1", cardIndex))
	if err != nil || output == "" {
		return 0, fmt.Errorf("failed to read AMD GPU temperature")
	}

	tempMilliC, err := strconv.ParseFloat(strings.TrimSpace(output), 64)
	if err != nil {
		return 0, err
	}

	return tempMilliC / 1000.0, nil
}

// getAMDGPUFanSpeed gets AMD GPU fan speed from sysfs (discrete GPUs only)
func (c *GPUCollector) getAMDGPUFanSpeed(cardIndex int) (int, int, error) {
	// Read current fan RPM
	rpmOutput, err := lib.ExecCommandOutput("bash", "-c", fmt.Sprintf("cat /sys/class/drm/card%d/device/hwmon/hwmon*/fan1_input 2>/dev/null | head -1", cardIndex))
	if err != nil || rpmOutput == "" {
		return 0, 0, fmt.Errorf("fan speed not available (integrated GPU or no fan sensor)")
	}

	rpm, err := strconv.Atoi(strings.TrimSpace(rpmOutput))
	if err != nil {
		return 0, 0, err
	}

	// Read max fan RPM
	maxRPMOutput, err := lib.ExecCommandOutput("bash", "-c", fmt.Sprintf("cat /sys/class/drm/card%d/device/hwmon/hwmon*/fan1_max 2>/dev/null | head -1", cardIndex))
	maxRPM := 0
	if err == nil && maxRPMOutput != "" {
		if val, err := strconv.Atoi(strings.TrimSpace(maxRPMOutput)); err == nil {
			maxRPM = val
		}
	}

	return rpm, maxRPM, nil
}

// getAMDDriverVersion gets AMD driver version
func (c *GPUCollector) getAMDDriverVersion() (string, error) {
	output, err := lib.ExecCommandOutput("modinfo", "amdgpu")
	if err != nil {
		return "", err
	}

	// Parse modinfo output for version
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, "version:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1]), nil
			}
		}
	}

	return "", fmt.Errorf("failed to parse driver version")
}

// collectAMDGPUWithROCm uses rocm-smi for datacenter AMD GPUs (fallback)
func (c *GPUCollector) collectAMDGPUWithROCm() ([]*dto.GPUMetrics, error) {
	// Query rocm-smi with JSON output
	output, err := lib.ExecCommandOutput("rocm-smi", "--showid", "--showtemp", "--showuse", "--showmeminfo", "vram", "--json")
	if err != nil {
		return nil, fmt.Errorf("rocm-smi query failed: %w", err)
	}

	var rocmData map[string]interface{}
	if err := json.Unmarshal([]byte(output), &rocmData); err != nil {
		return nil, fmt.Errorf("failed to parse rocm-smi JSON: %w", err)
	}

	gpus := make([]*dto.GPUMetrics, 0)
	index := 0

	// Parse each GPU
	for gpuID, gpuDataInterface := range rocmData {
		if !strings.HasPrefix(gpuID, "card") {
			continue
		}

		gpuData, ok := gpuDataInterface.(map[string]interface{})
		if !ok {
			continue
		}

		gpu := &dto.GPUMetrics{
			Available: true,
			Index:     index,
			Vendor:    "amd",
			Timestamp: time.Now(),
		}

		// Get GPU name/model
		if cardSeries, ok := gpuData["Card series"].(string); ok {
			gpu.Name = "AMD " + cardSeries
		}

		// Get temperature
		if temp, ok := gpuData["Temperature (Sensor edge) (C)"].(float64); ok {
			gpu.Temperature = temp
		}

		// Get GPU utilization
		if util, ok := gpuData["GPU use (%)"].(float64); ok {
			gpu.UtilizationGPU = util
		}

		// Get memory info
		if memUsed, ok := gpuData["VRAM Total Used Memory (B)"].(float64); ok {
			gpu.MemoryUsed = uint64(memUsed)
		}
		if memTotal, ok := gpuData["VRAM Total Memory (B)"].(float64); ok {
			gpu.MemoryTotal = uint64(memTotal)
			if gpu.MemoryTotal > 0 {
				gpu.UtilizationMemory = float64(gpu.MemoryUsed) / float64(gpu.MemoryTotal) * 100
			}
		}

		// Get driver version
		if index == 0 {
			if driverVersion, err := c.getAMDDriverVersion(); err == nil {
				gpu.DriverVersion = driverVersion
			}
		}

		gpus = append(gpus, gpu)
		index++
	}

	return gpus, nil
}
