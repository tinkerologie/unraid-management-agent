package collectors

import (
	"context"
	"fmt"
	"net/url"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/digitalocean/go-libvirt"

	"github.com/tinkerologie/unraid-management-agent/daemon/constants"
	"github.com/tinkerologie/unraid-management-agent/daemon/domain"
	"github.com/tinkerologie/unraid-management-agent/daemon/dto"
	"github.com/tinkerologie/unraid-management-agent/daemon/logger"
)

// vmCPUStats holds CPU usage tracking data for a VM
type vmCPUStats struct {
	guestCPUTime uint64    // Cumulative domain CPU time in nanoseconds
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

	// Run once immediately with panic recovery
	func() {
		defer func() {
			if r := recover(); r != nil {
				logger.LogPanicWithStack("VM collector", r)
			}
		}()
		c.Collect()
	}()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info("VM collector stopping due to context cancellation")
			return
		case <-ticker.C:
			func() {
				defer func() {
					if r := recover(); r != nil {
						logger.LogPanicWithStack("VM collector", r)
					}
				}()
				c.Collect()
			}()
		}
	}
}

// reportVMSourceFailure records a VM data-source failure, distinguishing an
// intentionally-disabled VM manager (reported as SourceDisabled, which is not
// counted as degraded) from a libvirt that should be running but cannot be
// reached (SourceUnavailable).
func (c *VMCollector) reportVMSourceFailure(reason string, err error) {
	if c.appCtx.Platform == nil {
		return
	}
	if vmServiceDisabled() {
		c.appCtx.Platform.Report("vm", dto.SourceDisabled, "VM manager disabled in Unraid settings", nil)
		return
	}
	c.appCtx.Platform.Report("vm", dto.SourceUnavailable, reason, err)
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
		c.reportVMSourceFailure("libvirt connection failed", err)
		// Publish empty list
		domain.Publish(c.appCtx.Hub, constants.TopicVMListUpdate, []*dto.VMInfo{})
		return
	}
	defer l.Disconnect() //nolint:errcheck

	// List all domains (active and inactive)
	flags := libvirt.ConnectListDomainsActive | libvirt.ConnectListDomainsInactive
	domains, _, err := l.ConnectListAllDomains(1, flags)
	if err != nil {
		logger.Error("Failed to list domains via libvirt: %v", err)
		c.reportVMSourceFailure("libvirt ListAllDomains failed", err)
		return
	}

	vms := make([]*dto.VMInfo, 0, len(domains))
	hostCPUs := c.hostCPUCount(l)

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
			c.getCPUUsage(l, domain, vm, hostCPUs)

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

	// Source healthy: connection + listing succeeded. A host with no VMs is
	// healthy, not unavailable. Attach the inline flag only when not healthy.
	if c.appCtx.Platform != nil {
		c.appCtx.Platform.Healthy("vm")
		if st := c.appCtx.Platform.StatusFor("vm"); st != nil {
			for _, vm := range vms {
				vm.SourceStatus = st
			}
		}
	}

	// Publish event
	domain.Publish(c.appCtx.Hub, constants.TopicVMListUpdate, vms)
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

// nodeInfoGetter is the libvirt call hostCPUCount needs; *libvirt.Libvirt implements it.
type nodeInfoGetter interface {
	NodeGetInfo() (rModel [32]int8, rMemory uint64, rCpus int32, rMhz int32, rNodes int32, rSockets int32, rCores int32, rThreads int32, err error)
}

// domainInfoGetter is the libvirt call getCPUUsage needs; *libvirt.Libvirt implements it.
type domainInfoGetter interface {
	DomainGetInfo(dom libvirt.Domain) (rState uint8, rMaxMem uint64, rMemory uint64, rNrVirtCPU uint16, rCPUTime uint64, err error)
}

// numCPU returns the CPUs the agent may run on; replaceable for tests.
var numCPU = runtime.NumCPU

// hostCPUCount returns the host's logical CPU count from libvirt (#175). It is
// independent of the agent's CPU affinity, which isolcpus shrinks on hosts
// that isolate cores for VMs.
func (c *VMCollector) hostCPUCount(l nodeInfoGetter) int {
	_, _, cpus, _, _, _, _, _, err := l.NodeGetInfo()
	return resolveHostCPUCount(cpus, err)
}

// resolveHostCPUCount picks the host CPU count: libvirt's active CPU count,
// else runtime.NumCPU(), else 1.
func resolveHostCPUCount(libvirtCPUs int32, err error) int {
	if err == nil && libvirtCPUs > 0 {
		return int(libvirtCPUs)
	}
	if n := numCPU(); n > 0 {
		return n
	}
	return 1
}

// cpuPercent converts a CPU-time delta to a percentage of divisor CPUs over
// elapsedSeconds, clamped to 0–100. It reports false when there is no valid
// value: no elapsed time, no CPUs, or a counter that went backwards (reset).
func cpuPercent(prevCPUTime, curCPUTime uint64, elapsedSeconds float64, divisor int) (float64, bool) {
	if elapsedSeconds <= 0 || divisor <= 0 || curCPUTime < prevCPUTime {
		return 0, false
	}
	pct := float64(curCPUTime-prevCPUTime) / (elapsedSeconds * 1e9 * float64(divisor)) * 100
	return min(max(pct, 0), 100), true
}

// getCPUUsage gets CPU usage for a running VM
func (c *VMCollector) getCPUUsage(l domainInfoGetter, domain libvirt.Domain, vm *dto.VMInfo, hostCPUs int) {
	// Get domain info for CPU time
	// DomainGetInfo returns: rState, rMaxMem, rMemory, rNrVirtCPU, rCPUTime, err
	_, _, _, _, cpuTime, err := l.DomainGetInfo(domain)
	if err != nil {
		return
	}
	c.recordCPUSample(domain.Name, cpuTime, time.Now(), hostCPUs, vm)
}

// recordCPUSample sets the guest and host CPU percentages from the previous
// sample of the VM, then stores this sample. The guest share is relative to
// the VM's vCPUs, the host share to the host's logical CPUs. Both stay 0 on
// the first sample and after a counter reset.
func (c *VMCollector) recordCPUSample(name string, cpuTime uint64, now time.Time, hostCPUs int, vm *dto.VMInfo) {
	c.cpuStatsMutex.Lock()
	defer c.cpuStatsMutex.Unlock()

	if prev, ok := c.previousStats[name]; ok {
		elapsed := now.Sub(prev.timestamp).Seconds()
		if pct, valid := cpuPercent(prev.guestCPUTime, cpuTime, elapsed, vm.CPUCount); valid {
			vm.GuestCPUPercent = pct
		}
		if pct, valid := cpuPercent(prev.guestCPUTime, cpuTime, elapsed, hostCPUs); valid {
			vm.HostCPUPercent = pct
		}
	}

	// Store current stats for next calculation
	c.previousStats[name] = &vmCPUStats{
		guestCPUTime: cpuTime,
		timestamp:    now,
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

// extractDiskTargets parses domain XML for the target device of each disk
// (e.g. "vda", "hdc"). libvirt writes attributes with single quotes
// (<target dev='hdc' bus='sata'/>), so both quote styles are accepted. Only
// <disk> elements are considered, so interface targets (vnetN) are skipped.
func extractDiskTargets(xml string) []string {
	var targets []string
	parts := strings.Split(xml, "<disk")
	for i := 1; i < len(parts); i++ {
		diskXML := parts[i]
		// "<disk" must be the whole tag name (<disk ...> or <disk>)
		if diskXML == "" || (diskXML[0] != ' ' && diskXML[0] != '>') {
			continue
		}
		if endIdx := strings.Index(diskXML, "</disk>"); endIdx > 0 {
			diskXML = diskXML[:endIdx]
		}
		if dev := targetDev(diskXML); dev != "" {
			targets = append(targets, dev)
		}
	}
	return targets
}

// targetDev returns the dev attribute of the first <target> element in an XML
// fragment, accepting double or single quotes.
func targetDev(fragment string) string {
	for _, quote := range []string{`"`, "'"} {
		if _, after, ok := strings.Cut(fragment, "<target dev="+quote); ok {
			if idx := strings.Index(after, quote); idx > 0 {
				return after[:idx]
			}
		}
	}
	return ""
}

// extractInterfaceTargets parses XML to find network interface targets
func extractInterfaceTargets(xml string) []string {
	var targets []string
	// Look for interface sections and their targets
	// Pattern: <interface type='...'> ... <target dev='vnetX'/> ... </interface>
	parts := strings.Split(xml, "<interface ")
	for i := 1; i < len(parts); i++ {
		ifaceXML := parts[i]
		if endIdx := strings.Index(ifaceXML, "</interface>"); endIdx > 0 {
			ifaceXML = ifaceXML[:endIdx]
			// Find target dev within this interface
			if dev := targetDev(ifaceXML); dev != "" {
				targets = append(targets, dev)
			}
		}
	}
	return targets
}
