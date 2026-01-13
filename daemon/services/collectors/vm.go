package collectors

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/digitalocean/go-libvirt"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
)

// vmCPUStats holds CPU usage tracking data for a VM
type vmCPUStats struct {
	guestCPUTime uint64    // Cumulative guest CPU time in nanoseconds
	hostCPUTime  uint64    // Cumulative host CPU time in clock ticks
	timestamp    time.Time // When this measurement was taken
}

// VMCollector collects VM information using the libvirt Go API directly.
// This is significantly faster than virsh CLI commands.
type VMCollector struct {
	appCtx        *domain.Context
	cpuStatsMutex sync.RWMutex
	previousStats map[string]*vmCPUStats // vmName -> previous CPU stats
}

// NewVMCollector creates a new libvirt-based VM collector
func NewVMCollector(ctx *domain.Context) *VMCollector {
	return &VMCollector{
		appCtx:        ctx,
		previousStats: make(map[string]*vmCPUStats),
	}
}

// Start begins the VM collector's periodic data collection
func (c *VMCollector) Start(ctx context.Context, interval time.Duration) {
	logger.Info("Starting VM collector (interval: %v)", interval)
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

// Collect gathers VM information using libvirt API and publishes to event bus
func (c *VMCollector) Collect() {
	startTotal := time.Now()
	logger.Debug("Collecting VM data via libvirt API...")

	// Connect to libvirt
	uri, _ := url.Parse(string(libvirt.QEMUSystem))
	l, err := libvirt.ConnectToURI(uri)
	if err != nil {
		logger.Debug("Failed to connect to libvirt: %v (libvirt may not be running)", err)
		// Publish empty list
		c.appCtx.Hub.Pub([]*dto.VMInfo{}, "vm_list_update")
		return
	}
	defer l.Disconnect()

	// List all domains (active and inactive)
	flags := libvirt.ConnectListDomainsActive | libvirt.ConnectListDomainsInactive
	domains, _, err := l.ConnectListAllDomains(1, flags)
	if err != nil {
		logger.Error("Failed to list domains via libvirt: %v", err)
		return
	}

	vms := make([]*dto.VMInfo, 0, len(domains))

	for _, domain := range domains {
		vm := &dto.VMInfo{
			ID:        fmt.Sprintf("%x", domain.UUID),
			Name:      domain.Name,
			Timestamp: time.Now(),
		}

		// Get domain state
		state, _, err := l.DomainGetState(domain, 0)
		if err != nil {
			logger.Debug("Failed to get state for VM %s: %v", domain.Name, err)
			vm.State = "unknown"
		} else {
			vm.State = c.stateToString(libvirt.DomainState(state))
		}

		// Get domain info (vCPUs, memory)
		// DomainGetInfo returns: rState, rMaxMem, rMemory, rNrVirtCPU, rCPUTime, err
		_, maxMem, memory, nrVirtCPU, _, err := l.DomainGetInfo(domain)
		if err == nil {
			vm.CPUCount = int(nrVirtCPU)
			vm.MemoryAllocated = maxMem * 1024 // Convert from KiB to bytes
			vm.MemoryUsed = memory * 1024      // Convert from KiB to bytes
		}

		// Check autostart
		autostart, err := l.DomainGetAutostart(domain)
		if err == nil {
			vm.Autostart = autostart != 0
		}

		// Get persistent state
		persistent, err := l.DomainIsPersistent(domain)
		if err == nil {
			vm.PersistentState = persistent != 0
		}

		// For running VMs, get more stats
		if vm.State == "running" {
			// Get CPU usage (using CPU stats)
			c.getCPUUsage(l, domain, vm)

			// Get block I/O stats
			c.getBlockIO(l, domain, vm)

			// Get network I/O stats
			c.getNetworkIO(l, domain, vm)
		} else {
			// VM is not running, clear CPU stats history
			c.clearCPUStats(domain.Name)
		}

		// Format memory display
		vm.MemoryDisplay = c.formatMemoryDisplay(vm.MemoryUsed, vm.MemoryAllocated)

		vms = append(vms, vm)
	}

	// Publish event
	c.appCtx.Hub.Pub(vms, "vm_list_update")
	logger.Debug("VM libvirt: Total collection took %v, published %d VMs", time.Since(startTotal), len(vms))
}

// stateToString converts libvirt domain state to string
func (c *VMCollector) stateToString(state libvirt.DomainState) string {
	switch state {
	case libvirt.DomainRunning:
		return "running"
	case libvirt.DomainBlocked:
		return "blocked"
	case libvirt.DomainPaused:
		return "paused"
	case libvirt.DomainShutdown:
		return "shutdown"
	case libvirt.DomainShutoff:
		return "shut off"
	case libvirt.DomainCrashed:
		return "crashed"
	case libvirt.DomainPmsuspended:
		return "suspended"
	default:
		return "unknown"
	}
}

// getCPUUsage gets CPU usage for a running VM
func (c *VMCollector) getCPUUsage(l *libvirt.Libvirt, domain libvirt.Domain, vm *dto.VMInfo) {
	// Get domain info for CPU time
	// DomainGetInfo returns: rState, rMaxMem, rMemory, rNrVirtCPU, rCPUTime, err
	_, _, _, _, cpuTime, err := l.DomainGetInfo(domain)
	if err != nil {
		return
	}

	currentTime := time.Now()
	guestCPUTime := cpuTime // nanoseconds

	c.cpuStatsMutex.Lock()
	defer c.cpuStatsMutex.Unlock()

	prevStats, hasPrev := c.previousStats[domain.Name]

	if hasPrev && currentTime.Sub(prevStats.timestamp) > 0 {
		// Calculate CPU percentage
		timeDelta := currentTime.Sub(prevStats.timestamp).Seconds()
		cpuTimeDelta := float64(guestCPUTime - prevStats.guestCPUTime)

		// CPU percentage = (CPU time delta in ns) / (wall time delta in s * 1e9 * num_vcpus) * 100
		if vm.CPUCount > 0 && timeDelta > 0 {
			vm.GuestCPUPercent = (cpuTimeDelta / (timeDelta * 1e9 * float64(vm.CPUCount))) * 100
			if vm.GuestCPUPercent < 0 {
				vm.GuestCPUPercent = 0
			}
			if vm.GuestCPUPercent > 100 {
				vm.GuestCPUPercent = 100
			}
		}
	}

	// Store current stats for next calculation
	c.previousStats[domain.Name] = &vmCPUStats{
		guestCPUTime: guestCPUTime,
		timestamp:    currentTime,
	}
}

// getBlockIO gets disk I/O stats for a running VM
func (c *VMCollector) getBlockIO(l *libvirt.Libvirt, domain libvirt.Domain, vm *dto.VMInfo) {
	// Get domain XML to find block devices
	xml, err := l.DomainGetXMLDesc(domain, 0)
	if err != nil {
		return
	}

	// Simple parsing to find disk targets (e.g., vda, sda)
	// Look for <target dev="vda"/> patterns
	diskTargets := extractDiskTargets(xml)

	var totalRead, totalWrite uint64
	for _, target := range diskTargets {
		// DomainBlockStats returns: rRdReq, rRdBytes, rWrReq, rWrBytes, rErrs, err
		_, rdBytes, _, wrBytes, _, err := l.DomainBlockStats(domain, target)
		if err != nil {
			continue
		}
		// Safe conversion: negative values indicate errors or unsupported
		if rdBytes >= 0 {
			totalRead += uint64(rdBytes) //nolint:gosec // G115: rdBytes checked >= 0
		}
		if wrBytes >= 0 {
			totalWrite += uint64(wrBytes) //nolint:gosec // G115: wrBytes checked >= 0
		}
	}

	vm.DiskReadBytes = totalRead
	vm.DiskWriteBytes = totalWrite
}

// getNetworkIO gets network I/O stats for a running VM
func (c *VMCollector) getNetworkIO(l *libvirt.Libvirt, domain libvirt.Domain, vm *dto.VMInfo) {
	// Get domain XML to find network interfaces
	xml, err := l.DomainGetXMLDesc(domain, 0)
	if err != nil {
		return
	}

	// Simple parsing to find interface targets (e.g., vnet0)
	ifaceTargets := extractInterfaceTargets(xml)

	var totalRX, totalTX uint64
	for _, target := range ifaceTargets {
		// DomainInterfaceStats returns: rRxBytes, rRxPackets, rRxErrs, rRxDrop, rTxBytes, rTxPackets, rTxErrs, rTxDrop, err
		rxBytes, _, _, _, txBytes, _, _, _, err := l.DomainInterfaceStats(domain, target)
		if err != nil {
			continue
		}
		// Safe conversion: negative values indicate errors or unsupported
		if rxBytes >= 0 {
			totalRX += uint64(rxBytes) //nolint:gosec // G115: rxBytes checked >= 0
		}
		if txBytes >= 0 {
			totalTX += uint64(txBytes) //nolint:gosec // G115: txBytes checked >= 0
		}
	}

	vm.NetworkRXBytes = totalRX
	vm.NetworkTXBytes = totalTX
}

// clearCPUStats removes CPU stats history for a VM
func (c *VMCollector) clearCPUStats(vmName string) {
	c.cpuStatsMutex.Lock()
	defer c.cpuStatsMutex.Unlock()
	delete(c.previousStats, vmName)
}

// formatMemoryDisplay formats memory as human-readable string
func (c *VMCollector) formatMemoryDisplay(used, allocated uint64) string {
	if allocated == 0 {
		return "0 B / 0 B"
	}

	usedMB := float64(used) / (1024 * 1024)
	allocMB := float64(allocated) / (1024 * 1024)

	if allocMB >= 1024 {
		usedGB := usedMB / 1024
		allocGB := allocMB / 1024
		return fmt.Sprintf("%.2f GB / %.2f GB", usedGB, allocGB)
	}

	return fmt.Sprintf("%.2f MB / %.2f MB", usedMB, allocMB)
}

// extractDiskTargets parses XML to find disk device targets
func extractDiskTargets(xml string) []string {
	targets := []string{}
	// Simple string parsing for <target dev="xxx"/> patterns
	parts := strings.Split(xml, "<target dev=\"")
	for i := 1; i < len(parts); i++ {
		if idx := strings.Index(parts[i], "\""); idx > 0 {
			target := parts[i][:idx]
			targets = append(targets, target)
		}
	}
	return targets
}

// extractInterfaceTargets parses XML to find network interface targets
func extractInterfaceTargets(xml string) []string {
	targets := []string{}
	// Look for interface sections and their targets
	// Pattern: <interface type='...'> ... <target dev='vnetX'/> ... </interface>
	parts := strings.Split(xml, "<interface ")
	for i := 1; i < len(parts); i++ {
		ifaceXML := parts[i]
		if endIdx := strings.Index(ifaceXML, "</interface>"); endIdx > 0 {
			ifaceXML = ifaceXML[:endIdx]
			// Find target dev within this interface
			if devIdx := strings.Index(ifaceXML, "<target dev=\""); devIdx >= 0 {
				devPart := ifaceXML[devIdx+len("<target dev=\""):]
				if quoteIdx := strings.Index(devPart, "\""); quoteIdx > 0 {
					targets = append(targets, devPart[:quoteIdx])
				}
			} else if devIdx := strings.Index(ifaceXML, "<target dev='"); devIdx >= 0 {
				devPart := ifaceXML[devIdx+len("<target dev='"):]
				if quoteIdx := strings.Index(devPart, "'"); quoteIdx > 0 {
					targets = append(targets, devPart[:quoteIdx])
				}
			}
		}
	}
	return targets
}
