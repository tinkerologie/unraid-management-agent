package collectors

import (
	"context"
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

type VMCollector struct {
	ctx *domain.Context
}

func NewVMCollector(ctx *domain.Context) *VMCollector {
	return &VMCollector{ctx: ctx}
}

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

		// Get VM ID (only for running VMs)
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

			// Get CPU usage
			if guestCPU, hostCPU, err := c.getVMCPUUsage(vmName); err == nil {
				vm.GuestCPUPercent = guestCPU
				vm.HostCPUPercent = hostCPU
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
		}

		// Format memory display
		vm.MemoryDisplay = c.formatMemoryDisplay(vm.MemoryUsed, vm.MemoryAllocated)

		vms = append(vms, vm)
	}

	return vms, nil
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

// getVMID returns the ID of a running VM, or empty string if not running
func (c *VMCollector) getVMID(vmName string) string {
	output, err := lib.ExecCommandOutput("virsh", "domid", vmName)
	if err != nil {
		return ""
	}
	id := strings.TrimSpace(output)
	// virsh domid returns "-" for shut off VMs
	if id == "-" || id == "" {
		return ""
	}
	return id
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
func (c *VMCollector) getVMCPUUsage(vmName string) (float64, float64, error) {
	output, err := lib.ExecCommandOutput("virsh", "cpu-stats", vmName, "--total")
	if err != nil {
		return 0, 0, err
	}

	// Parse CPU time from output
	// Format: "cpu_time          123456789 ns"
	re := regexp.MustCompile(`cpu_time\s+(\d+)`)
	if matches := re.FindStringSubmatch(output); len(matches) > 1 {
		_ = matches[1] // CPU time available but needs historical data for percentage calculation
	}

	// For now, return 0 as we need historical data to calculate percentage
	// This would require storing previous values and calculating delta
	return 0, 0, nil
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
