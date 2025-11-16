package collectors

import (
	"context"
	"time"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/lib"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
)

// HardwareCollector collects detailed hardware information using dmidecode.
// It gathers BIOS, baseboard, CPU, cache, and memory information from the system's DMI tables.
type HardwareCollector struct {
	ctx *domain.Context
}

// NewHardwareCollector creates a new hardware information collector with the given context.
func NewHardwareCollector(ctx *domain.Context) *HardwareCollector {
	return &HardwareCollector{ctx: ctx}
}

// Start begins the hardware collector's periodic data collection.
// It runs in a goroutine and publishes hardware information updates at the specified interval until the context is cancelled.
func (c *HardwareCollector) Start(ctx context.Context, interval time.Duration) {
	logger.Info("Starting hardware collector (interval: %v)", interval)

	// Run once immediately with panic recovery
	func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("Hardware collector PANIC on startup: %v", r)
			}
		}()
		c.Collect()
	}()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info("Hardware collector stopping due to context cancellation")
			return
		case <-ticker.C:
			func() {
				defer func() {
					if r := recover(); r != nil {
						logger.Error("Hardware collector PANIC in loop: %v", r)
					}
				}()
				c.Collect()
			}()
		}
	}
}

// Collect gathers hardware information from DMI tables and publishes it to the event bus.
// It uses dmidecode to extract BIOS, baseboard, CPU, cache, and memory device information.
func (c *HardwareCollector) Collect() {
	logger.Debug("Collecting hardware data...")

	// Collect hardware information
	hardwareInfo, err := c.collectHardwareInfo()
	if err != nil {
		logger.Error("Hardware: Failed to collect hardware data: %v", err)
		return
	}

	logger.Debug("Hardware: Successfully collected hardware info, publishing event")
	// Publish event
	c.ctx.Hub.Pub(hardwareInfo, "hardware_update")
	logger.Debug("Hardware: Published hardware_update event")
}

func (c *HardwareCollector) collectHardwareInfo() (*dto.HardwareInfo, error) {
	info := &dto.HardwareInfo{
		Timestamp: time.Now(),
	}

	// Check if dmidecode is available
	if !lib.CommandExists("dmidecode") {
		logger.Warning("dmidecode command not found, skipping hardware collection")
		return info, nil
	}

	// Collect BIOS information
	if bios, err := lib.ParseBIOSInfo(); err == nil {
		info.BIOS = bios
		logger.Debug("Hardware: Collected BIOS info - Vendor: %s, Version: %s", bios.Vendor, bios.Version)
	} else {
		logger.Debug("Hardware: Failed to collect BIOS info: %v", err)
	}

	// Collect baseboard information
	if baseboard, err := lib.ParseBaseboardInfo(); err == nil {
		info.Baseboard = baseboard
		logger.Debug("Hardware: Collected baseboard info - Manufacturer: %s, Product: %s", baseboard.Manufacturer, baseboard.ProductName)
	} else {
		logger.Debug("Hardware: Failed to collect baseboard info: %v", err)
	}

	// Collect CPU hardware information
	if cpu, err := lib.ParseCPUInfo(); err == nil {
		info.CPU = cpu
		logger.Debug("Hardware: Collected CPU hardware info - Socket: %s, Manufacturer: %s", cpu.SocketDesignation, cpu.Manufacturer)
	} else {
		logger.Debug("Hardware: Failed to collect CPU hardware info: %v", err)
	}

	// Collect CPU cache information
	if caches, err := lib.ParseCPUCacheInfo(); err == nil {
		info.Cache = caches
		logger.Debug("Hardware: Collected %d CPU cache levels", len(caches))
	} else {
		logger.Debug("Hardware: Failed to collect CPU cache info: %v", err)
	}

	// Collect memory array information
	if memArray, err := lib.ParseMemoryArrayInfo(); err == nil {
		info.MemoryArray = memArray
		logger.Debug("Hardware: Collected memory array info - Max Capacity: %s, Devices: %d", memArray.MaximumCapacity, memArray.NumberOfDevices)
	} else {
		logger.Debug("Hardware: Failed to collect memory array info: %v", err)
	}

	// Collect memory device information
	if memDevices, err := lib.ParseMemoryDevices(); err == nil {
		info.MemoryDevices = memDevices
		logger.Debug("Hardware: Collected %d memory devices", len(memDevices))
	} else {
		logger.Debug("Hardware: Failed to collect memory devices: %v", err)
	}

	return info, nil
}
