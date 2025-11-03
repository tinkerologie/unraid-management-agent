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
	// Get list of all VMs
	output, err := lib.ExecCommandOutput("virsh", "list", "--all")
	if err != nil {
		return nil, fmt.Errorf("failed to list VMs: %w", err)
	}

	lines := strings.Split(output, "\n")
	vms := make([]*dto.VMInfo, 0)

	// Skip header lines (first 2 lines)
	for i := 2; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}

		// Parse line format: " ID   Name   State"
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		// Determine if VM is running or not
		var vmID, vmName, vmState string
		if fields[0] == "-" {
			// Inactive VM: "- Name shut off"
			vmID = ""
			vmName = fields[1]
			vmState = strings.Join(fields[2:], " ")
		} else {
			// Active VM: "ID Name running"
			vmID = fields[0]
			vmName = fields[1]
			vmState = strings.Join(fields[2:], " ")
		}

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
		}

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
