package collectors

import (
	"bufio"
	"context"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/common"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
)

type DiskCollector struct {
	ctx *domain.Context
}

func NewDiskCollector(ctx *domain.Context) *DiskCollector {
	return &DiskCollector{ctx: ctx}
}

func (c *DiskCollector) Start(ctx context.Context, interval time.Duration) {
	logger.Info("Starting disk collector (interval: %v)", interval)

	// Run once immediately with panic recovery
	func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("Disk collector PANIC on startup: %v", r)
			}
		}()
		c.Collect()
	}()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info("Disk collector stopping due to context cancellation")
			return
		case <-ticker.C:
			func() {
				defer func() {
					if r := recover(); r != nil {
						logger.Error("Disk collector PANIC in loop: %v", r)
					}
				}()
				c.Collect()
			}()
		}
	}
}

func (c *DiskCollector) Collect() {
	logger.Debug("Collecting disk data...")

	// Collect disk information
	disks, err := c.collectDisks()
	if err != nil {
		logger.Error("Disk: Failed to collect disk data: %v", err)
		return
	}

	logger.Debug("Disk: Successfully collected %d disks, publishing event", len(disks))
	// Publish event
	c.ctx.Hub.Pub(disks, "disk_list_update")
	logger.Debug("Disk: Published disk_list_update event with %d disks", len(disks))
}

func (c *DiskCollector) collectDisks() ([]dto.DiskInfo, error) {
	logger.Debug("Disk: Starting collection from %s", common.DisksIni)
	var disks []dto.DiskInfo

	// Parse disks.ini
	file, err := os.Open(common.DisksIni)
	if err != nil {
		logger.Error("Disk: Failed to open file: %v", err)
		return nil, err
	}
	defer func() {
		if err := file.Close(); err != nil {
			logger.Debug("Error closing disk file: %v", err)
		}
	}()
	logger.Debug("Disk: File opened successfully")

	scanner := bufio.NewScanner(file)
	var currentDisk *dto.DiskInfo

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Check for section header: ["diskname"]
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			// Save previous disk if exists
			if currentDisk != nil {
				disks = append(disks, *currentDisk)
			}

			// Start new disk
			currentDisk = &dto.DiskInfo{
				Timestamp: time.Now(),
			}
			continue
		}

		// Parse key=value pairs
		if currentDisk != nil && strings.Contains(line, "=") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) != 2 {
				continue
			}

			key := strings.TrimSpace(parts[0])
			value := strings.Trim(strings.TrimSpace(parts[1]), `"`)

			switch key {
			case "name":
				currentDisk.Name = value
			case "device":
				currentDisk.Device = value
			case "id":
				currentDisk.ID = value
			case "status":
				currentDisk.Status = value
			case "size":
				if size, err := strconv.ParseUint(value, 10, 64); err == nil {
					currentDisk.Size = size * 512 // Convert sectors to bytes
				}
			case "temp":
				// Temperature might be "*" if spun down
				if value != "*" && value != "" {
					if temp, err := strconv.ParseFloat(value, 64); err == nil {
						currentDisk.Temperature = temp
					}
				}
			case "numErrors":
				if errors, err := strconv.Atoi(value); err == nil {
					currentDisk.SMARTErrors = errors
				}
			case "spindownDelay":
				if delay, err := strconv.Atoi(value); err == nil {
					currentDisk.SpindownDelay = delay
				}
			case "format":
				currentDisk.FileSystem = value
			}
		}
	}

	// Save last disk
	if currentDisk != nil {
		disks = append(disks, *currentDisk)
	}

	if err := scanner.Err(); err != nil {
		logger.Error("Disk: Scanner error: %v", err)
		return disks, err
	}

	// Enhance each disk with additional stats
	for i := range disks {
		// Get I/O statistics
		c.enrichWithIOStats(&disks[i])

		// Get SMART attributes (if device is available)
		if disks[i].Device != "" {
			c.enrichWithSMARTData(&disks[i])
		}

		// Get mount information
		c.enrichWithMountInfo(&disks[i])

		// Get disk role
		c.enrichWithRole(&disks[i])

		// Get spin state
		if disks[i].Device != "" {
			c.enrichWithSpinState(&disks[i])
		}
	}

	logger.Debug("Disk: Parsed %d disks successfully", len(disks))
	return disks, nil
}

// enrichWithIOStats adds I/O statistics from /sys/block
func (c *DiskCollector) enrichWithIOStats(disk *dto.DiskInfo) {
	if disk.Device == "" {
		return
	}

	// Read from /sys/block/{device}/stat
	statPath := "/sys/block/" + disk.Device + "/stat"
	//nolint:gosec // G304: Path is constructed from /sys/block system directory, device name from trusted source
	data, err := os.ReadFile(statPath)
	if err != nil {
		return // Device might be spun down or not available
	}

	fields := strings.Fields(string(data))
	if len(fields) < 11 {
		return
	}

	// Parse fields (see Documentation/block/stat.txt in Linux kernel)
	// read I/Os, read merges, read sectors, read ticks,
	// write I/Os, write merges, write sectors, write ticks,
	// in_flight, io_ticks, time_in_queue
	if readOps, err := strconv.ParseUint(fields[0], 10, 64); err == nil {
		disk.ReadOps = readOps
	}
	if readSectors, err := strconv.ParseUint(fields[2], 10, 64); err == nil {
		disk.ReadBytes = readSectors * 512 // Sectors to bytes
	}
	if writeOps, err := strconv.ParseUint(fields[4], 10, 64); err == nil {
		disk.WriteOps = writeOps
	}
	if writeSectors, err := strconv.ParseUint(fields[6], 10, 64); err == nil {
		disk.WriteBytes = writeSectors * 512 // Sectors to bytes
	}
	if ioTicks, err := strconv.ParseUint(fields[9], 10, 64); err == nil {
		// io_ticks is in milliseconds, calculate utilization
		// This is a cumulative value, would need previous sample for rate
		disk.IOUtilization = float64(ioTicks) / 10.0 // Rough estimate
	}
}

// enrichWithSMARTData adds SMART attributes using smartctl
func (c *DiskCollector) enrichWithSMARTData(disk *dto.DiskInfo) {
	devicePath := "/dev/" + disk.Device

	// Check if device exists
	if _, err := os.Stat(devicePath); err != nil {
		return
	}

	// Get basic SMART health
	disk.SMARTStatus = "UNKNOWN"

	// Try to get serial number and model from smartctl -i
	data, err := os.ReadFile("/var/local/emhttp/smart/" + disk.Device)
	if err == nil {
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "Serial Number:") {
				disk.SerialNumber = strings.TrimSpace(strings.TrimPrefix(line, "Serial Number:"))
			} else if strings.HasPrefix(line, "Device Model:") || strings.HasPrefix(line, "Model Number:") {
				disk.Model = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(line, "Device Model:"), "Model Number:"))
			}
		}
	}

	// Note: Full SMART parsing would require smartctl execution
	// For now, we'll keep the existing SMART error count from disks.ini
	// Future enhancement: Parse smartctl -a output for detailed attributes
}

// enrichWithMountInfo adds mount point and usage information
func (c *DiskCollector) enrichWithMountInfo(disk *dto.DiskInfo) {
	if disk.Name == "" {
		return
	}

	// Read /proc/mounts to find mount point
	data, err := os.ReadFile("/proc/mounts")
	if err != nil {
		return
	}

	// For Unraid array disks, the mount point is /mnt/diskN where N is the disk number
	// The device in /proc/mounts is /dev/mdNp1 (e.g., /dev/md1p1 for disk1)
	// For cache/flash, it's the actual device (e.g., /dev/nvme0n1p1, /dev/sda1)

	var mountPoint string
	lines := strings.Split(string(data), "\n")

	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		// Check if mount point matches /mnt/{diskname}
		expectedMountPoint := "/mnt/" + disk.Name
		if fields[1] == expectedMountPoint {
			mountPoint = fields[1]
			break
		}

		// Also check for direct device match (for cache, flash, etc.)
		if disk.Device != "" {
			devicePath := "/dev/" + disk.Device
			if fields[0] == devicePath || strings.HasPrefix(fields[0], devicePath) {
				mountPoint = fields[1]
				break
			}
		}
	}

	if mountPoint == "" {
		return
	}

	disk.MountPoint = mountPoint

	// Get filesystem statistics using statfs
	var stat syscall.Statfs_t
	if err := syscall.Statfs(disk.MountPoint, &stat); err == nil {
		// Calculate sizes in bytes
		totalBytes := uint64(stat.Blocks) * uint64(stat.Bsize)
		freeBytes := uint64(stat.Bfree) * uint64(stat.Bsize)
		usedBytes := totalBytes - freeBytes

		disk.Used = usedBytes
		disk.Free = freeBytes

		// Calculate usage percentage
		if totalBytes > 0 {
			disk.UsagePercent = float64(usedBytes) / float64(totalBytes) * 100
		}
	}
}

// enrichWithRole determines the disk role (parity, parity2, data, cache, pool)
func (c *DiskCollector) enrichWithRole(disk *dto.DiskInfo) {
	// Determine role based on disk name/ID
	name := strings.ToLower(disk.Name)
	id := strings.ToLower(disk.ID)

	if strings.Contains(name, "parity") || strings.Contains(id, "parity") {
		if strings.Contains(name, "parity2") || strings.Contains(id, "parity2") {
			disk.Role = "parity2"
		} else {
			disk.Role = "parity"
		}
	} else if strings.Contains(name, "cache") || strings.Contains(id, "cache") {
		disk.Role = "cache"
	} else if strings.Contains(name, "pool") || strings.Contains(id, "pool") {
		disk.Role = "pool"
	} else if strings.Contains(name, "disk") || strings.Contains(id, "disk") {
		disk.Role = "data"
	} else {
		disk.Role = "unknown"
	}
}

// enrichWithSpinState checks the current spin state of the disk
func (c *DiskCollector) enrichWithSpinState(disk *dto.DiskInfo) {
	devicePath := "/dev/" + disk.Device

	// Check if device exists
	if _, err := os.Stat(devicePath); err != nil {
		disk.SpinState = "unknown"
		return
	}

	// Read spin state from /var/local/emhttp/var.ini or check temperature
	// If temperature is "*", disk is spun down
	if disk.Temperature == 0 {
		// Try to read from sysfs or use hdparm
		// For now, use a simple heuristic: if temp is 0, likely spun down
		disk.SpinState = "standby"
	} else {
		disk.SpinState = "active"
	}

	// Alternative: Could execute hdparm -C /dev/sdX to get actual state
	// But that requires executing external command which we want to minimize
}
