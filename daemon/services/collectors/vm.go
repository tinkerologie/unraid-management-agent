package collectors

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/lib"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
)

// cpuStats holds CPU usage tracking data for a VM
type cpuStats struct {
	guestCPUTime uint64    // Cumulative guest CPU time in nanoseconds
	hostCPUTime  uint64    // Cumulative host CPU time in clock ticks
	timestamp    time.Time // When this measurement was taken
}

// VMCollector collects information about virtual machines managed by libvirt/virsh.
// It gathers VM status, resource allocation, CPU usage, and configuration details.
type VMCollector struct {
	ctx           *domain.Context
	cpuStatsMutex sync.RWMutex
	previousStats map[string]*cpuStats // vmName -> previous CPU stats
}

// NewVMCollector creates a new virtual machine collector with the given context.
func NewVMCollector(ctx *domain.Context) *VMCollector {
	return &VMCollector{
		ctx:           ctx,
		previousStats: make(map[string]*cpuStats),
	}
}

// Start begins the VM collector's periodic data collection.
// It runs in a goroutine and publishes VM information updates at the specified interval until the context is cancelled.
func (c *VMCollector) Start(ctx context.Context, interval time.Duration) {
	logger.Info("Starting vm collector (interval: %v)", interval)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info("VM collector stopping due to context cancellation")
			return
		case <-ticker.C:
			c.Collect()
		}
	}
}

// Collect gathers virtual machine information and publishes it to the event bus.
// It uses virsh to query VM status and calculates CPU usage based on previous measurements.
func (c *VMCollector) Collect() {

	logger.Debug("Collecting vm data...")

	// Check if virsh is available
	if !lib.CommandExists("virsh") {
		logger.Warning("virsh command not found, skipping collection")
		return
	}

	// Collect VM information
	vms, err := c.collectVMs()
	if err != nil {
		logger.Error("Failed to collect VMs: %v", err)
		return
	}

	// Publish event
	c.ctx.Hub.Pub(vms, "vm_list_update")
	logger.Debug("Published vm_list_update event with %d VMs", len(vms))
}

func (c *VMCollector) collectVMs() ([]*dto.VMInfo, error) {
	// Get list of all VM names (one per line)
	// This approach handles VM names with spaces correctly
	output, err := lib.ExecCommandOutput("virsh", "list", "--all", "--name")
	if err != nil {
		return nil, fmt.Errorf("failed to list VMs: %w", err)
	}

	lines := strings.Split(output, "\n")
	vms := make([]*dto.VMInfo, 0)

	for _, line := range lines {
		vmName := strings.TrimSpace(line)
		if vmName == "" {
			continue
		}

		// Get VM state
		vmState, err := c.getVMState(vmName)
		if err != nil {
			logger.Warning("Failed to get state for VM %s: %v", vmName, err)
			continue
		}

		// Get VM UUID (stable identifier for all VM states)
		vmID := c.getVMID(vmName)

		vm := &dto.VMInfo{
			ID:        vmID,
			Name:      vmName,
			State:     vmState,
			Timestamp: time.Now(),
		}

		// Get detailed info for this VM
		if info, err := c.getVMInfo(vmName); err == nil {
			vm.CPUCount = info.CPUCount
			vm.MemoryAllocated = info.MemoryAllocated
			vm.Autostart = info.Autostart
			vm.PersistentState = info.PersistentState
		}

		// Get memory usage if running
		if strings.Contains(strings.ToLower(vmState), "running") {
			if memUsed, err := c.getVMMemoryUsage(vmName); err == nil {
				vm.MemoryUsed = memUsed
			}

			// Get CPU usage (pass number of vCPUs for percentage calculation)
			if vm.CPUCount > 0 {
				if guestCPU, hostCPU, err := c.getVMCPUUsage(vmName, vm.CPUCount); err == nil {
					vm.GuestCPUPercent = guestCPU
					vm.HostCPUPercent = hostCPU
				} else {
					logger.Debug("Failed to get CPU usage for VM %s: %v", vmName, err)
				}
			}

			// Get disk I/O stats
			if readBytes, writeBytes, err := c.getVMDiskIO(vmName); err == nil {
				vm.DiskReadBytes = readBytes
				vm.DiskWriteBytes = writeBytes
			}

			// Get network I/O stats
			if rxBytes, txBytes, err := c.getVMNetworkIO(vmName); err == nil {
				vm.NetworkRXBytes = rxBytes
				vm.NetworkTXBytes = txBytes
			}
		} else {
			// VM is not running, clear CPU stats history
			c.clearCPUStats(vmName)
		}

		// Format memory display
		vm.MemoryDisplay = c.formatMemoryDisplay(vm.MemoryUsed, vm.MemoryAllocated)

		vms = append(vms, vm)
	}

	return vms, nil
}

// clearCPUStats removes CPU stats history for a VM (called when VM is shut off)
func (c *VMCollector) clearCPUStats(vmName string) {
	c.cpuStatsMutex.Lock()
	defer c.cpuStatsMutex.Unlock()
	delete(c.previousStats, vmName)
}

type vmInfo struct {
	CPUCount        int
	MemoryAllocated uint64
	Autostart       bool
	PersistentState bool
}

// getVMState returns the state of a VM (e.g., "running", "shut off", "paused")
func (c *VMCollector) getVMState(vmName string) (string, error) {
	output, err := lib.ExecCommandOutput("virsh", "domstate", vmName)
	if err != nil {
		return "", fmt.Errorf("failed to get VM state: %w", err)
	}
	return strings.TrimSpace(output), nil
}

// getVMID returns the UUID of a VM (stable identifier that works for all VM states)
func (c *VMCollector) getVMID(vmName string) string {
	output, err := lib.ExecCommandOutput("virsh", "domuuid", vmName)
	if err != nil {
		// Fallback to using VM name as ID if UUID is not available
		return vmName
	}
	uuid := strings.TrimSpace(output)
	if uuid == "" {
		// Fallback to using VM name as ID
		return vmName
	}
	return uuid
}

func (c *VMCollector) getVMInfo(vmName string) (*vmInfo, error) {
	output, err := lib.ExecCommandOutput("virsh", "dominfo", vmName)
	if err != nil {
		return nil, err
	}

	info := &vmInfo{}
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		switch key {
		case "CPU(s)":
			if cpu, err := strconv.Atoi(value); err == nil {
				info.CPUCount = cpu
			}
		case "Max memory":
			// Value is in KiB
			// Extract number before " KiB"
			if memStr := strings.Fields(value); len(memStr) > 0 {
				if mem, err := strconv.ParseUint(memStr[0], 10, 64); err == nil {
					info.MemoryAllocated = mem * 1024 // Convert KiB to bytes
				}
			}
		case "Autostart":
			info.Autostart = strings.ToLower(value) == "enable"
		case "Persistent":
			info.PersistentState = strings.ToLower(value) == "yes"
		}
	}

	return info, nil
}

func (c *VMCollector) getVMMemoryUsage(vmName string) (uint64, error) {
	output, err := lib.ExecCommandOutput("virsh", "dommemstat", vmName)
	if err != nil {
		return 0, err
	}

	// Parse output for actual memory usage
	// Format: "actual 4194304" (in KiB)
	re := regexp.MustCompile(`actual\s+(\d+)`)
	if matches := re.FindStringSubmatch(output); len(matches) > 1 {
		if mem, err := strconv.ParseUint(matches[1], 10, 64); err == nil {
			return mem * 1024, nil // Convert KiB to bytes
		}
	}

	// Fallback: look for rss (resident set size)
	re = regexp.MustCompile(`rss\s+(\d+)`)
	if matches := re.FindStringSubmatch(output); len(matches) > 1 {
		if mem, err := strconv.ParseUint(matches[1], 10, 64); err == nil {
			return mem * 1024, nil // Convert KiB to bytes
		}
	}

	return 0, nil
}

// getVMCPUUsage returns guest and host CPU usage percentages
func (c *VMCollector) getVMCPUUsage(vmName string, numVCPUs int) (float64, float64, error) {
	currentTime := time.Now()

	// Get guest CPU time from virsh domstats
	guestCPUTime, err := c.getGuestCPUTime(vmName)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get guest CPU time: %w", err)
	}

	// Get host CPU time from QEMU process
	hostCPUTime, err := c.getHostCPUTime(vmName)
	if err != nil {
		// Host CPU might not be available, log but don't fail
		logger.Debug("Failed to get host CPU time for VM %s: %v", vmName, err)
		hostCPUTime = 0
	}

	// Calculate percentages using historical data
	c.cpuStatsMutex.Lock()
	defer c.cpuStatsMutex.Unlock()

	var guestCPUPercent, hostCPUPercent float64

	if prevStats, exists := c.previousStats[vmName]; exists {
		// Calculate time delta in seconds
		timeDelta := currentTime.Sub(prevStats.timestamp).Seconds()

		if timeDelta > 0 {
			// Calculate guest CPU percentage
			// Guest CPU time is in nanoseconds, convert to seconds
			guestCPUDelta := float64(guestCPUTime-prevStats.guestCPUTime) / 1e9
			guestCPUPercent = (guestCPUDelta / timeDelta / float64(numVCPUs)) * 100

			// Clamp to valid range [0, 100]
			if guestCPUPercent < 0 {
				guestCPUPercent = 0
			} else if guestCPUPercent > 100 {
				guestCPUPercent = 100
			}

			// Calculate host CPU percentage if available
			if hostCPUTime > 0 && prevStats.hostCPUTime > 0 {
				// Host CPU time is in clock ticks, need to convert
				// Clock ticks per second (typically 100 on Linux)
				clockTicksPerSec := 100.0
				hostCPUDelta := float64(hostCPUTime-prevStats.hostCPUTime) / clockTicksPerSec
				hostCPUPercent = (hostCPUDelta / timeDelta) * 100

				// Clamp to valid range [0, 100]
				if hostCPUPercent < 0 {
					hostCPUPercent = 0
				} else if hostCPUPercent > 100 {
					hostCPUPercent = 100
				}
			}
		}
	}

	// Store current stats for next calculation
	c.previousStats[vmName] = &cpuStats{
		guestCPUTime: guestCPUTime,
		hostCPUTime:  hostCPUTime,
		timestamp:    currentTime,
	}

	return guestCPUPercent, hostCPUPercent, nil
}

// getGuestCPUTime returns cumulative guest CPU time in nanoseconds
func (c *VMCollector) getGuestCPUTime(vmName string) (uint64, error) {
	output, err := lib.ExecCommandOutput("virsh", "domstats", vmName, "--cpu-total")
	if err != nil {
		return 0, err
	}

	// Parse cpu.time from output
	// Format: "cpu.time=123456789"
	re := regexp.MustCompile(`cpu\.time=(\d+)`)
	matches := re.FindStringSubmatch(output)
	if len(matches) < 2 {
		return 0, fmt.Errorf("failed to parse cpu.time from domstats output")
	}

	cpuTime, err := strconv.ParseUint(matches[1], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse cpu time value: %w", err)
	}

	return cpuTime, nil
}

// getHostCPUTime returns cumulative host CPU time in clock ticks for the QEMU process
func (c *VMCollector) getHostCPUTime(vmName string) (uint64, error) {
	// Get QEMU process PID
	pid, err := c.getQEMUProcessPID(vmName)
	if err != nil {
		return 0, err
	}

	// Read /proc/[pid]/stat
	output, err := lib.ExecCommandOutput("cat", fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return 0, fmt.Errorf("failed to read /proc/%d/stat: %w", pid, err)
	}

	// Parse /proc/[pid]/stat
	// Format: pid (comm) state ppid pgrp session tty_nr tpgid flags minflt cminflt majflt cmajflt utime stime ...
	// We need utime (field 14) + stime (field 15)
	fields := strings.Fields(output)
	if len(fields) < 15 {
		return 0, fmt.Errorf("unexpected /proc/stat format")
	}

	// utime is at index 13 (0-based), stime at index 14
	utime, err := strconv.ParseUint(fields[13], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse utime: %w", err)
	}

	stime, err := strconv.ParseUint(fields[14], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse stime: %w", err)
	}

	// Total CPU time = utime + stime
	return utime + stime, nil
}

// getQEMUProcessPID returns the PID of the QEMU process for a VM
func (c *VMCollector) getQEMUProcessPID(vmName string) (int, error) {
	// Use pgrep to find QEMU process with VM name
	output, err := lib.ExecCommandOutput("pgrep", "-f", fmt.Sprintf("qemu.*guest=%s", vmName))
	if err != nil {
		return 0, fmt.Errorf("failed to find QEMU process for VM %s: %w", vmName, err)
	}

	pidStr := strings.TrimSpace(output)
	if pidStr == "" {
		return 0, fmt.Errorf("no QEMU process found for VM %s", vmName)
	}

	// If multiple PIDs, take the first one
	pidStr = strings.Split(pidStr, "\n")[0]

	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return 0, fmt.Errorf("failed to parse PID: %w", err)
	}

	return pid, nil
}

// getVMDiskIO returns disk read and write bytes
func (c *VMCollector) getVMDiskIO(vmName string) (uint64, uint64, error) {
	// Get list of disk devices
	output, err := lib.ExecCommandOutput("virsh", "domblklist", vmName)
	if err != nil {
		return 0, 0, err
	}

	var totalRead, totalWrite uint64
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[0] == "Target" {
			continue
		}

		device := fields[0]
		stats, err := lib.ExecCommandOutput("virsh", "domblkstat", vmName, device)
		if err != nil {
			continue
		}

		// Parse read and write bytes
		// Format: "rd_bytes 123456"
		reRead := regexp.MustCompile(`rd_bytes\s+(\d+)`)
		if matches := reRead.FindStringSubmatch(stats); len(matches) > 1 {
			if bytes, err := strconv.ParseUint(matches[1], 10, 64); err == nil {
				totalRead += bytes
			}
		}

		reWrite := regexp.MustCompile(`wr_bytes\s+(\d+)`)
		if matches := reWrite.FindStringSubmatch(stats); len(matches) > 1 {
			if bytes, err := strconv.ParseUint(matches[1], 10, 64); err == nil {
				totalWrite += bytes
			}
		}
	}

	return totalRead, totalWrite, nil
}

// getVMNetworkIO returns network RX and TX bytes
func (c *VMCollector) getVMNetworkIO(vmName string) (uint64, uint64, error) {
	// Get list of network interfaces
	output, err := lib.ExecCommandOutput("virsh", "domiflist", vmName)
	if err != nil {
		return 0, 0, err
	}

	var totalRX, totalTX uint64
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 1 || fields[0] == "Interface" || fields[0] == "-" {
			continue
		}

		iface := fields[0]
		stats, err := lib.ExecCommandOutput("virsh", "domifstat", vmName, iface)
		if err != nil {
			continue
		}

		// Parse RX and TX bytes
		// Format: "rx_bytes 123456"
		reRX := regexp.MustCompile(`rx_bytes\s+(\d+)`)
		if matches := reRX.FindStringSubmatch(stats); len(matches) > 1 {
			if bytes, err := strconv.ParseUint(matches[1], 10, 64); err == nil {
				totalRX += bytes
			}
		}

		reTX := regexp.MustCompile(`tx_bytes\s+(\d+)`)
		if matches := reTX.FindStringSubmatch(stats); len(matches) > 1 {
			if bytes, err := strconv.ParseUint(matches[1], 10, 64); err == nil {
				totalTX += bytes
			}
		}
	}

	return totalRX, totalTX, nil
}

// formatMemoryDisplay formats memory usage as "used / allocated"
func (c *VMCollector) formatMemoryDisplay(used, allocated uint64) string {
	if allocated == 0 {
		return "0 / 0"
	}

	usedGB := float64(used) / (1024 * 1024 * 1024)
	allocatedGB := float64(allocated) / (1024 * 1024 * 1024)

	return fmt.Sprintf("%.2f GB / %.2f GB", usedGB, allocatedGB)
}
