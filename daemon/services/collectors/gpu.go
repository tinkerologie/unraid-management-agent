package collectors

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/tinkerologie/unraid-management-agent/daemon/constants"
	"github.com/tinkerologie/unraid-management-agent/daemon/domain"
	"github.com/tinkerologie/unraid-management-agent/daemon/dto"
	"github.com/tinkerologie/unraid-management-agent/daemon/lib"
	"github.com/tinkerologie/unraid-management-agent/daemon/logger"
)

// GPUCollector collects GPU metrics from NVIDIA, AMD, and Intel GPUs.
// It gathers temperature, utilization, memory usage, and power consumption data.
type GPUCollector struct {
	ctx *domain.Context
	// sysfsRoot allows PCI-bound metadata reads against a fixture.
	sysfsRoot string
	// nvidiaExec runs nvidia-smi. Nil means lib.ExecCommandOutput; tests inject a fake.
	nvidiaExec func(name string, args ...string) (string, error)
}

// NewGPUCollector creates a new GPU metrics collector with the given context.
func NewGPUCollector(ctx *domain.Context) *GPUCollector {
	return &GPUCollector{ctx: ctx, sysfsRoot: defaultSysfsRoot}
}

// runNvidia invokes nvidia-smi via the injected executor, defaulting to lib.ExecCommandOutput.
func (c *GPUCollector) runNvidia(args ...string) (string, error) {
	if c.nvidiaExec != nil {
		return c.nvidiaExec("nvidia-smi", args...)
	}
	return lib.ExecCommandOutput("nvidia-smi", args...)
}

// Collect gathers GPU metrics from all available GPUs and publishes them to the event bus.
// It detects and collects data from NVIDIA, AMD, and Intel GPUs using vendor-specific tools.
func (c *GPUCollector) Collect() {
	logger.Debug("Collecting gpu data...")

	// Collect GPU metrics from all available GPU types
	var gpuMetrics []*dto.GPUMetrics

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

	// Reassign indices globally to ensure uniqueness across vendors.
	// Collection order is deterministic: Intel → NVIDIA → AMD.
	assignGlobalGPUIndices(gpuMetrics)

	// Publish event
	domain.Publish(c.ctx.Hub, constants.TopicGPUMetricsUpdate, gpuMetrics)
	logger.Debug("Published %s event for %d total GPU(s)", constants.TopicGPUMetricsUpdate.Name, len(gpuMetrics))
}

// assignGlobalGPUIndices reassigns GPU indices sequentially (0, 1, 2, ...)
// to ensure global uniqueness across all vendors.
func assignGlobalGPUIndices(gpus []*dto.GPUMetrics) {
	for i, gpu := range gpus {
		gpu.Index = i
	}
}

var (
	// lspciQuotedFieldRe extracts the quoted fields of an `lspci -Dmm` record.
	lspciQuotedFieldRe = regexp.MustCompile(`"([^"]*)"`)
	// lspciDisplayClassRe matches the numeric class that lspci prints for a
	// PCI base class 0x03 device when pci.ids has no name for it.
	lspciDisplayClassRe = regexp.MustCompile(`^Class 03[0-9a-fA-F]{2}$`)
)

// lspciRecord holds the fields of one `lspci -Dmm` line that GPU discovery uses.
type lspciRecord struct {
	PCIID  string
	Class  string
	Vendor string
	Device string
}

// parseLspciRecord splits an `lspci -Dmm` line into its unquoted PCI address
// and its first three quoted fields (class, vendor, device). Unquoted tokens
// such as -rXX and -pXX are ignored. It rejects records with an invalid
// address or fewer than three quoted fields.
func parseLspciRecord(line string) (lspciRecord, bool) {
	firstQuote := strings.Index(line, "\"")
	if firstQuote <= 0 {
		return lspciRecord{}, false
	}
	pciID := strings.TrimSpace(line[:firstQuote])
	if !pciAddressRe.MatchString(pciID) {
		return lspciRecord{}, false
	}
	matches := lspciQuotedFieldRe.FindAllStringSubmatch(line[firstQuote:], -1)
	if len(matches) < 3 {
		return lspciRecord{}, false
	}
	return lspciRecord{PCIID: pciID, Class: matches[0][1], Vendor: matches[1][1], Device: matches[2][1]}, true
}

// isDisplayClass reports whether an lspci class field names a PCI base class
// 0x03 (display controller) device. It matches whole field values only.
func isDisplayClass(class string) bool {
	switch class {
	case "VGA compatible controller", "XGA compatible controller", "3D controller", "Display controller":
		return true
	}
	return lspciDisplayClassRe.MatchString(class)
}

// lspciModelName returns the bracketed marketing name of an lspci device field,
// or the whole field when it has no brackets. A malformed bracket gives "".
func lspciModelName(device string) string {
	if !strings.Contains(device, "[") {
		return device
	}
	start := strings.Index(device, "[")
	end := strings.Index(device, "]")
	if start != -1 && end != -1 && end > start {
		return strings.TrimSpace(device[start+1 : end])
	}
	return ""
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
	var intelGPUs []intelGPUInfo

	for line := range strings.SplitSeq(output, "\n") {
		// Format: 0000:00:02.0 "VGA compatible controller" "Intel Corporation" "CoffeeLake-S GT2 [UHD Graphics 630]" -p00 "ASRock Incorporation" "Device 3e92"
		record, ok := parseLspciRecord(line)
		if !ok || !isDisplayClass(record.Class) || record.Vendor != "Intel Corporation" {
			continue
		}
		logger.Debug("Intel GPU: Found Intel GPU line: %s", line)
		logger.Debug("Intel GPU: Full model string: %s", record.Device)
		model := lspciModelName(record.Device)
		logger.Debug("Intel GPU: Parsed - ID: %s, Model: %s", record.PCIID, model)

		if model != "" {
			intelGPUs = append(intelGPUs, intelGPUInfo{PCIID: record.PCIID, Model: model})
		}
		// REMOVED: break statement - continue searching for more Intel GPUs
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

	// Run intel_gpu_top in JSON mode with 1 sample for power efficiency
	// Reduced timeout from 5s to 2s and samples from 2 to 1 to minimize CPU usage (Issue #8)
	// Note: intel_gpu_top auto-detects Intel GPU, doesn't support -d flag for specific device
	cmdOutput, err := lib.ExecCommandOutput("timeout", "2", "intel_gpu_top", "-J", "-s", "500", "-n", "1")
	switch {
	case err != nil && len(cmdOutput) == 0:
		logger.Debug("Intel GPU: intel_gpu_top query failed with no output: %v", err)
		return nil
	case err != nil:
		logger.Debug("Intel GPU: intel_gpu_top timed out (expected), got %d bytes output", len(cmdOutput))
	default:
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
	var intelData map[string]any
	if err := json.Unmarshal([]byte(sampleJSON), &intelData); err != nil {
		logger.Debug("Intel GPU: Failed to parse sample: %v", err)
		return nil
	}

	logger.Debug("Intel GPU: Successfully parsed sample for GPU %d", index)

	gpu := parseIntelGPUSample(intelData, pciID, model, index)
	// Read the driver bound to this PCI device, with its module version if available.
	if driverVersion, err := c.getIntelDriverVersion(pciID); err == nil {
		gpu.DriverVersion = driverVersion
	}

	// Intel iGPU typically doesn't report temperature via intel_gpu_top or sysfs hwmon
	// Most Intel integrated GPUs don't expose temperature sensors
	if temp, err := c.getIntelGPUTemp(pciID); err == nil {
		gpu.Temperature = new(temp)
	}
	// The xe driver labels the VRAM channel vram. i915 has no such channel.
	gpu.MemoryTemperature = c.getGPUMemoryTemp(pciID, "vram")

	// For Intel iGPUs, add CPU temperature as they share the die with the CPU
	// This provides useful thermal information since iGPUs don't have dedicated temp sensors
	if cpuTemp, err := c.getCPUTemp(); err == nil {
		gpu.CPUTemperature = cpuTemp
		logger.Debug("Intel GPU: CPU temperature: %.1f°C", cpuTemp)
	}

	return gpu
}

// parseIntelGPUSample preserves missing power and shared-memory measurements.
func parseIntelGPUSample(intelData map[string]any, pciID, model string, index int) *dto.GPUMetrics {
	gpu := &dto.GPUMetrics{
		Available: true,
		Index:     index,
		PCIID:     pciID,
		Vendor:    "intel",
		Name:      "Intel " + model,
		Timestamp: time.Now(),
	}

	// Extract utilization from engines
	if engines, ok := intelData["engines"].(map[string]any); ok {
		// Sum up all engine utilizations for overall GPU usage
		totalUtil := 0.0
		engineCount := 0
		for engineName, engineData := range engines {
			if engineMap, ok := engineData.(map[string]any); ok {
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
	if power, ok := intelData["power"].(map[string]any); ok {
		if gpuPower, ok := power["GPU"].(float64); ok {
			gpu.PowerDraw = new(gpuPower)
			logger.Debug("Intel GPU power: %.3f W", gpuPower)
		}
	}

	// Extract memory info (Note: Intel iGPU shares system RAM, intel_gpu_top doesn't report memory usage)
	// The "memory" field is not present in intel_gpu_top JSON output for integrated GPUs
	if memory, ok := intelData["memory"].(map[string]any); ok {
		if total, ok := memory["total"].(float64); ok && total >= 0 && total < float64(^uint64(0)) {
			gpu.MemoryTotal = new(uint64(total))
		}
		if shared, ok := memory["shared"].(float64); ok && shared >= 0 && shared < float64(^uint64(0)) {
			gpu.MemoryUsed = new(uint64(shared))
		}
		if gpu.MemoryTotal != nil && gpu.MemoryUsed != nil && *gpu.MemoryTotal > 0 {
			gpu.UtilizationMemory = new(float64(*gpu.MemoryUsed) / float64(*gpu.MemoryTotal) * 100)
		}
	}

	return gpu
}

// sysfs returns the mount point, including for zero-value collectors in tests.
func (c *GPUCollector) sysfs() string {
	if c.sysfsRoot != "" {
		return c.sysfsRoot
	}
	return defaultSysfsRoot
}

// getIntelGPUTemp reads only hwmon channels belonging to the detected PCI device.
// It uses the first usable channel labelled pkg or package. Otherwise it uses the
// first usable channel without a vram label. The xe driver has no temp1_input and
// exposes the package temperature on temp2.
func (c *GPUCollector) getIntelGPUTemp(pciID string) (float64, error) {
	channels, err := c.readPCIHwmonTemps(pciID)
	if err != nil {
		return 0, fmt.Errorf("finding Intel GPU temperature: %w", err)
	}
	fallback, haveFallback := 0.0, false
	for _, channel := range channels {
		if channel.label == "pkg" || channel.label == "package" {
			return channel.celsius, nil
		}
		if !haveFallback && !strings.HasPrefix(channel.label, "vram") {
			fallback, haveFallback = channel.celsius, true
		}
	}
	if haveFallback {
		return fallback, nil
	}
	return 0, fmt.Errorf("no Intel GPU temperature for PCI device %s", pciID)
}

// getGPUMemoryTemp returns the first usable channel with one of the labels in
// the PCI device's own hwmon. It is nil when no channel matches: another channel
// or device never supplies the memory temperature.
func (c *GPUCollector) getGPUMemoryTemp(pciID string, labels ...string) *float64 {
	channels, err := c.readPCIHwmonTemps(pciID)
	if err != nil {
		return nil
	}
	for _, channel := range channels {
		if slices.Contains(labels, channel.label) {
			return new(channel.celsius)
		}
	}
	return nil
}

// hwmonTempChannel is one usable tempN channel. The label is trimmed and lower case.
type hwmonTempChannel struct {
	label   string
	celsius float64
}

// readPCIHwmonTemps returns the usable hwmon temp*_input channels of one PCI
// device in numeric hwmon and channel order. It skips unreadable, malformed
// and non-finite inputs.
func (c *GPUCollector) readPCIHwmonTemps(pciID string) ([]hwmonTempChannel, error) {
	paths, err := filepath.Glob(filepath.Join(c.sysfs(), "bus/pci/devices", pciID, "hwmon/hwmon*/temp*_input"))
	if err != nil {
		return nil, err
	}
	sortHwmonTempInputs(paths)
	var channels []hwmonTempChannel
	for _, path := range paths {
		// #nosec G304 -- path is a hwmon temperature file under the internally detected PCI device.
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		temp, err := strconv.ParseFloat(strings.TrimSpace(string(data)), 64)
		if err != nil || math.IsNaN(temp) || math.IsInf(temp, 0) {
			continue
		}
		label := strings.ToLower(readTrimmed(strings.TrimSuffix(path, "_input") + "_label"))
		channels = append(channels, hwmonTempChannel{label: label, celsius: temp / 1000})
	}
	return channels, nil
}

// sortHwmonTempInputs orders hwmonN/tempM_input paths by N, then M, as numbers.
// filepath.Glob sorts lexically and puts temp10 before temp2.
func sortHwmonTempInputs(paths []string) {
	index := func(path string) (int, int) {
		hwmon, _ := strconv.Atoi(strings.TrimPrefix(filepath.Base(filepath.Dir(path)), "hwmon"))
		channel, _ := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(filepath.Base(path), "temp"), "_input"))
		return hwmon, channel
	}
	sort.SliceStable(paths, func(i, j int) bool {
		hwmonI, channelI := index(paths[i])
		hwmonJ, channelJ := index(paths[j])
		if hwmonI != hwmonJ {
			return hwmonI < hwmonJ
		}
		return channelI < channelJ
	})
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

// getIntelDriverVersion reports the bound driver name, plus its module version
// when the module exposes one. Kernel vermagic is not a driver version.
func (c *GPUCollector) getIntelDriverVersion(pciID string) (string, error) {
	driverPath, err := filepath.EvalSymlinks(filepath.Join(c.sysfs(), "bus/pci/devices", pciID, "driver"))
	if err != nil {
		return "", fmt.Errorf("resolving Intel GPU driver: %w", err)
	}
	driver := filepath.Base(driverPath)
	if driver != "i915" && driver != "xe" {
		return "", fmt.Errorf("unexpected Intel GPU driver %q", driver)
	}
	// #nosec G304 -- driver is restricted above to i915 or xe; the root is the sysfs mount.
	version, err := os.ReadFile(filepath.Join(c.sysfs(), "module", driver, "version"))
	if err == nil && strings.TrimSpace(string(version)) != "" {
		return driver + " " + strings.TrimSpace(string(version)), nil
	}
	return driver, nil
}

// NVIDIA GPU collection using nvidia-smi
func (c *GPUCollector) collectNvidiaGPU() ([]*dto.GPUMetrics, error) {
	// Query nvidia-smi with CSV output for easy parsing
	// Added: pci.bus_id, uuid, fan.speed
	output, err := c.runNvidia(
		"--query-gpu=index,pci.bus_id,uuid,name,temperature.gpu,utilization.gpu,memory.used,memory.total,power.draw,fan.speed",
		"--format=csv,noheader,nounits",
	)
	if err != nil {
		return nil, fmt.Errorf("nvidia-smi query failed: %w", err)
	}

	gpus, err := parseNvidiaGPUCSV(output)
	if err != nil {
		return nil, err
	}

	// Driver version is identical for all GPUs, so query it once and apply it.
	if len(gpus) > 0 {
		if driverVersion, err := c.getNvidiaDriverVersion(); err == nil {
			for _, gpu := range gpus {
				gpu.DriverVersion = driverVersion
			}
		}
	}

	return gpus, nil
}

// parseNvidiaGPUCSV parses nvidia-smi CSV output (noheader,nounits) with the
// query column order
// index,pci.bus_id,uuid,name,temperature.gpu,utilization.gpu,memory.used,
// memory.total,power.draw,fan.speed into GPU metrics. Memory values are
// converted from MiB to bytes and memory utilization is derived. Records with
// fewer than 10 fields are skipped; unparseable measurements are left nil.
func parseNvidiaGPUCSV(output string) ([]*dto.GPUMetrics, error) {
	reader := csv.NewReader(strings.NewReader(output))
	// Tolerate variable field counts so a single malformed/short row is skipped
	// per-record instead of failing the whole parse with a field-count error.
	reader.FieldsPerRecord = -1
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
		if temp, err := strconv.ParseFloat(strings.TrimSpace(record[4]), 64); err == nil && !math.IsNaN(temp) && !math.IsInf(temp, 0) {
			gpu.Temperature = new(temp)
		}

		// Utilization (%)
		if util, err := strconv.ParseFloat(strings.TrimSpace(record[5]), 64); err == nil {
			gpu.UtilizationGPU = util
		}

		// Memory Used (MiB)
		if memUsed, err := strconv.ParseFloat(strings.TrimSpace(record[6]), 64); err == nil && memUsed >= 0 && memUsed < float64(^uint64(0))/(1024*1024) {
			gpu.MemoryUsed = new(uint64(memUsed * 1024 * 1024)) // Convert MiB to bytes
		}

		// Memory Total (MiB)
		if memTotal, err := strconv.ParseFloat(strings.TrimSpace(record[7]), 64); err == nil && memTotal >= 0 && memTotal < float64(^uint64(0))/(1024*1024) {
			gpu.MemoryTotal = new(uint64(memTotal * 1024 * 1024)) // Convert MiB to bytes
			if gpu.MemoryTotal != nil && gpu.MemoryUsed != nil && *gpu.MemoryTotal > 0 {
				gpu.UtilizationMemory = new(float64(*gpu.MemoryUsed) / float64(*gpu.MemoryTotal) * 100)
			}
		}

		// Power Draw (W)
		if power, err := strconv.ParseFloat(strings.TrimSpace(record[8]), 64); err == nil && !math.IsNaN(power) && !math.IsInf(power, 0) {
			gpu.PowerDraw = new(power)
		}

		// Fan Speed (%)
		if fanSpeed, err := strconv.ParseFloat(strings.TrimSpace(record[9]), 64); err == nil {
			gpu.FanSpeed = fanSpeed
		}

		gpus = append(gpus, gpu)
	}

	return gpus, nil
}

// getNvidiaDriverVersion gets NVIDIA driver version
func (c *GPUCollector) getNvidiaDriverVersion() (string, error) {
	output, err := c.runNvidia("--query-gpu=driver_version", "--format=csv,noheader")
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
	var amdGPUs []amdGPUInfo

	for line := range strings.SplitSeq(output, "\n") {
		record, ok := parseLspciRecord(line)
		if !ok || !isDisplayClass(record.Class) {
			continue
		}
		if !strings.Contains(record.Vendor, "AMD") && !strings.Contains(record.Vendor, "Advanced Micro Devices") &&
			!strings.Contains(record.Vendor, "ATI") {
			continue
		}

		if model := lspciModelName(record.Device); model != "" {
			amdGPUs = append(amdGPUs, amdGPUInfo{PCIID: record.PCIID, Model: model})
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
				gpu.UtilizationMemory = new(vramPercent)
			}
			if vramUsedMB, err := strconv.ParseUint(matches[2], 10, 64); err == nil {
				gpu.MemoryUsed = new(vramUsedMB * 1024 * 1024) // Convert MB to bytes
				// Calculate total from percentage
				if gpu.UtilizationMemory != nil && *gpu.UtilizationMemory > 0 {
					gpu.MemoryTotal = new(uint64(float64(*gpu.MemoryUsed) / (*gpu.UtilizationMemory / 100.0)))
				}
			}
		}
	}

	// Get temperature from sysfs
	if temp, err := c.getAMDGPUTemp(index); err == nil {
		gpu.Temperature = new(temp)
	}
	// amdgpu labels the VRAM channel mem. Read it by PCI ID, not by card index.
	gpu.MemoryTemperature = c.getGPUMemoryTemp(pciID, "mem")

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

	tempMilliC := parseOptionalFloat(output)
	if tempMilliC == nil {
		return 0, fmt.Errorf("invalid AMD GPU temperature")
	}
	return *tempMilliC / 1000.0, nil
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
	for line := range strings.SplitSeq(output, "\n") {
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

	var rocmData map[string]any
	if err := json.Unmarshal([]byte(output), &rocmData); err != nil {
		return nil, fmt.Errorf("failed to parse rocm-smi JSON: %w", err)
	}

	var gpus []*dto.GPUMetrics
	index := 0

	// Extract and sort card identifiers numerically for deterministic ordering
	cardIDs := make([]string, 0, len(rocmData))
	for gpuID := range rocmData {
		if strings.HasPrefix(gpuID, "card") {
			cardIDs = append(cardIDs, gpuID)
		}
	}
	sort.Slice(cardIDs, func(i, j int) bool {
		ai, errI := strconv.Atoi(strings.TrimPrefix(cardIDs[i], "card"))
		aj, errJ := strconv.Atoi(strings.TrimPrefix(cardIDs[j], "card"))
		if errI == nil && errJ == nil {
			return ai < aj
		}
		return cardIDs[i] < cardIDs[j]
	})

	// Parse each GPU in deterministic card order
	for _, gpuID := range cardIDs {
		gpuDataInterface := rocmData[gpuID]

		gpuData, ok := gpuDataInterface.(map[string]any)
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
			gpu.Temperature = new(temp)
		}

		// Get GPU utilization
		if util, ok := gpuData["GPU use (%)"].(float64); ok {
			gpu.UtilizationGPU = util
		}

		// Get memory info
		if memUsed, ok := gpuData["VRAM Total Used Memory (B)"].(float64); ok && memUsed >= 0 && memUsed < float64(^uint64(0)) {
			gpu.MemoryUsed = new(uint64(memUsed))
		}
		if memTotal, ok := gpuData["VRAM Total Memory (B)"].(float64); ok && memTotal >= 0 && memTotal < float64(^uint64(0)) {
			gpu.MemoryTotal = new(uint64(memTotal))
			if gpu.MemoryTotal != nil && gpu.MemoryUsed != nil && *gpu.MemoryTotal > 0 {
				gpu.UtilizationMemory = new(float64(*gpu.MemoryUsed) / float64(*gpu.MemoryTotal) * 100)
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
