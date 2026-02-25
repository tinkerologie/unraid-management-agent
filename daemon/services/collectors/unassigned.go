package collectors

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/constants"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/lib"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
)

// UnassignedCollector collects information about unassigned devices
type UnassignedCollector struct {
	ctx *domain.Context
}

// NewUnassignedCollector creates a new unassigned devices collector
func NewUnassignedCollector(ctx *domain.Context) *UnassignedCollector {
	return &UnassignedCollector{ctx: ctx}
}

// Start begins collecting unassigned device information
func (c *UnassignedCollector) Start(ctx context.Context, interval time.Duration) {
	defer func() {
		if r := recover(); r != nil {
			logger.Error("Unassigned collector panic: %v", r)
		}
	}()

	logger.Info("Starting unassigned devices collector (interval: %v)", interval)

	// Initial collection
	c.collect()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info("Stopping unassigned devices collector")
			return
		case <-ticker.C:
			c.collect()
		}
	}
}

// collect gathers unassigned device information
func (c *UnassignedCollector) collect() {
	devices := c.collectUnassignedDevices()
	remoteShares := c.collectRemoteShares()

	deviceList := &dto.UnassignedDeviceList{
		Devices:      devices,
		RemoteShares: remoteShares,
		Timestamp:    time.Now(),
	}

	// Publish event
	domain.Publish(c.ctx.Hub, constants.TopicUnassignedDevicesUpdate, deviceList)
	logger.Debug("Published unassigned devices update - devices=%d, remote_shares=%d",
		len(devices), len(remoteShares))
}

// collectUnassignedDevices discovers and collects unassigned disk devices
func (c *UnassignedCollector) collectUnassignedDevices() []dto.UnassignedDevice {
	// Check if plugin is installed
	if !c.isPluginInstalled() {
		logger.Debug("Unassigned Devices plugin not installed")
		return []dto.UnassignedDevice{}
	}

	// Get array disks to filter them out
	arrayDisks := c.getArrayDisks()

	// Get all block devices
	allDevices := c.getAllBlockDevices()

	var unassignedDevices []dto.UnassignedDevice
	for _, device := range allDevices {
		// Skip if it's an array disk
		if c.isArrayDisk(device, arrayDisks) {
			continue
		}

		// Skip loop devices, md devices, zram, and partitions
		if strings.HasPrefix(device, "loop") ||
			strings.HasPrefix(device, "md") ||
			strings.HasPrefix(device, "zram") ||
			strings.Contains(device, "nvme0n1p") ||
			(len(device) > 3 && device[3] >= '1' && device[3] <= '9') {
			continue
		}

		unassignedDevice := c.getDeviceInfo(device)
		if unassignedDevice != nil {
			unassignedDevices = append(unassignedDevices, *unassignedDevice)
		}
	}

	return unassignedDevices
}

// collectRemoteShares collects remote SMB/NFS/ISO shares
func (c *UnassignedCollector) collectRemoteShares() []dto.UnassignedRemoteShare {
	if !c.isPluginInstalled() {
		return []dto.UnassignedRemoteShare{}
	}

	var shares []dto.UnassignedRemoteShare

	// Parse SMB mounts
	smbShares := c.parseSMBMounts()
	shares = append(shares, smbShares...)

	// Parse ISO mounts
	isoShares := c.parseISOMounts()
	shares = append(shares, isoShares...)

	return shares
}

// isPluginInstalled checks if the Unassigned Devices plugin is installed
func (c *UnassignedCollector) isPluginInstalled() bool {
	_, err := os.Stat("/boot/config/plugins/unassigned.devices")
	return err == nil
}

// getArrayDisks returns a map of array disk devices
func (c *UnassignedCollector) getArrayDisks() map[string]bool {
	arrayDisks := make(map[string]bool)

	// Read disks.ini file directly
	data, err := os.ReadFile("/var/local/emhttp/disks.ini")
	if err != nil {
		logger.Debug("Failed to read disks.ini: %v", err)
		return arrayDisks
	}

	// Parse the INI file to extract device names
	lines := strings.SplitSeq(string(data), "\n")
	for line := range lines {
		line = strings.TrimSpace(line)
		if device, found := strings.CutPrefix(line, "device="); found {
			device = strings.Trim(device, "\"")
			if device != "" {
				arrayDisks[device] = true
			}
		}
	}

	return arrayDisks
}

// getAllBlockDevices returns a list of all block device names
func (c *UnassignedCollector) getAllBlockDevices() []string {
	cmd := exec.Command("lsblk", "-d", "-n", "-o", "NAME")
	output, err := cmd.Output()
	if err != nil {
		logger.Error("Failed to list block devices: %v", err)
		return []string{}
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	return lines
}

// isArrayDisk checks if a device is part of the Unraid array
func (c *UnassignedCollector) isArrayDisk(device string, arrayDisks map[string]bool) bool {
	return arrayDisks[device]
}

// getDeviceInfo retrieves detailed information about a device
func (c *UnassignedCollector) getDeviceInfo(device string) *dto.UnassignedDevice {
	// Get device info using lsblk
	cmd := exec.Command("lsblk", "-J", "-o", "NAME,SIZE,TYPE,MOUNTPOINT,FSTYPE,LABEL,SERIAL,MODEL", "/dev/"+device) // #nosec G204 - device is validated from lsblk output
	output, err := cmd.Output()
	if err != nil {
		logger.Debug("Failed to get info for device %s: %v", device, err)
		return nil
	}

	var lsblkOutput struct {
		BlockDevices []struct {
			Name       string `json:"name"`
			Size       string `json:"size"`
			Type       string `json:"type"`
			MountPoint string `json:"mountpoint"`
			FSType     string `json:"fstype"`
			Label      string `json:"label"`
			Serial     string `json:"serial"`
			Model      string `json:"model"`
			Children   []struct {
				Name       string `json:"name"`
				Size       string `json:"size"`
				Type       string `json:"type"`
				MountPoint string `json:"mountpoint"`
				FSType     string `json:"fstype"`
				Label      string `json:"label"`
			} `json:"children"`
		} `json:"blockdevices"`
	}

	if err := json.Unmarshal(output, &lsblkOutput); err != nil {
		logger.Debug("Failed to parse lsblk output for %s: %v", device, err)
		return nil
	}

	if len(lsblkOutput.BlockDevices) == 0 {
		return nil
	}

	blockDev := lsblkOutput.BlockDevices[0]

	unassignedDevice := &dto.UnassignedDevice{
		Device:         blockDev.Name,
		SerialNumber:   blockDev.Serial,
		Model:          blockDev.Model,
		Identification: blockDev.Model,
		Status:         "unmounted",
		SpinState:      "unknown",
		AutoMount:      false,
		PassThrough:    false,
		DisableMount:   false,
		ScriptEnabled:  false,
		Timestamp:      time.Now(),
	}

	// Process partitions
	var partitions []dto.UnassignedPartition
	for i, child := range blockDev.Children {
		partition := dto.UnassignedPartition{
			PartitionNumber: i + 1,
			Label:           child.Label,
			FileSystem:      child.FSType,
			MountPoint:      child.MountPoint,
			ReadOnly:        false,
			SMBShare:        false,
			NFSShare:        false,
			Status:          "unmounted",
		}

		if child.MountPoint != "" {
			partition.Status = "mounted"
			unassignedDevice.Status = "mounted"

			// Get partition size info if mounted
			c.getPartitionSizeInfo(&partition, child.MountPoint)
		}

		partitions = append(partitions, partition)
	}

	unassignedDevice.Partitions = partitions

	return unassignedDevice
}

// getPartitionSizeInfo retrieves size information for a mounted partition
func (c *UnassignedCollector) getPartitionSizeInfo(partition *dto.UnassignedPartition, mountPoint string) {
	cmd := exec.Command("df", "-B1", mountPoint)
	output, err := cmd.Output()
	if err != nil {
		return
	}

	lines := strings.Split(string(output), "\n")
	if len(lines) < 2 {
		return
	}

	fields := strings.Fields(lines[1])
	if len(fields) < 6 {
		return
	}

	// Parse size, used, free
	size := lib.ParseUint64(fields[1])
	used := lib.ParseUint64(fields[2])
	free := lib.ParseUint64(fields[3])

	partition.Size = size
	partition.Used = used
	partition.Free = free

	// Calculate usage percent
	if size > 0 {
		partition.UsagePercent = float64(used) / float64(size) * 100.0
	}
}

// parseSMBMounts parses SMB mount configuration
func (c *UnassignedCollector) parseSMBMounts() []dto.UnassignedRemoteShare {
	configPath := "/boot/config/plugins/unassigned.devices/samba_mount.cfg"
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return []dto.UnassignedRemoteShare{}
	}

	// For now, return empty list - full implementation would parse the config file
	return []dto.UnassignedRemoteShare{}
}

// parseISOMounts parses ISO mount configuration
func (c *UnassignedCollector) parseISOMounts() []dto.UnassignedRemoteShare {
	configPath := "/boot/config/plugins/unassigned.devices/iso_mount.cfg"
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return []dto.UnassignedRemoteShare{}
	}

	// Check if any ISO files are mounted
	mounts, err := os.ReadFile("/proc/mounts")
	if err != nil {
		return []dto.UnassignedRemoteShare{}
	}

	var isoShares []dto.UnassignedRemoteShare
	lines := strings.SplitSeq(string(mounts), "\n")
	for line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}

		// Check if it's an ISO mount (loop device mounted under /mnt/disks/)
		if strings.HasPrefix(fields[0], "/dev/loop") && strings.HasPrefix(fields[1], "/mnt/disks/") {
			share := dto.UnassignedRemoteShare{
				Type:       "iso",
				Source:     fields[0],
				MountPoint: fields[1],
				Status:     "mounted",
				ReadOnly:   true,
				AutoMount:  false,
				Timestamp:  time.Now(),
			}

			// Get size info
			c.getRemoteShareSizeInfo(&share, fields[1])

			isoShares = append(isoShares, share)
		}
	}

	return isoShares
}

// getRemoteShareSizeInfo retrieves size information for a remote share
func (c *UnassignedCollector) getRemoteShareSizeInfo(share *dto.UnassignedRemoteShare, mountPoint string) {
	cmd := exec.Command("df", "-B1", mountPoint)
	output, err := cmd.Output()
	if err != nil {
		return
	}

	lines := strings.Split(string(output), "\n")
	if len(lines) < 2 {
		return
	}

	fields := strings.Fields(lines[1])
	if len(fields) < 6 {
		return
	}

	// Parse size, used, free
	size := lib.ParseUint64(fields[1])
	used := lib.ParseUint64(fields[2])
	free := lib.ParseUint64(fields[3])

	share.Size = size
	share.Used = used
	share.Free = free

	// Calculate usage percent
	if size > 0 {
		share.UsagePercent = float64(used) / float64(size) * 100.0
	}
}
