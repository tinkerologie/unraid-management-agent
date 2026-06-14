package collectors

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
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
	// Top-level safety net for startup preamble panics
	defer func() {
		if r := recover(); r != nil {
			logger.LogPanicWithStack("Unassigned collector (top-level)", r)
		}
	}()

	logger.Info("Starting unassigned devices collector (interval: %v)", interval)

	// Initial collection with panic recovery
	func() {
		defer func() {
			if r := recover(); r != nil {
				logger.LogPanicWithStack("Unassigned collector", r)
			}
		}()
		collectWithWatchdog(ctx, "Unassigned", interval, c.collect)
	}()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info("Stopping unassigned devices collector")
			return
		case <-ticker.C:
			func() {
				defer func() {
					if r := recover(); r != nil {
						logger.LogPanicWithStack("Unassigned collector", r)
					}
				}()
				collectWithWatchdog(ctx, "Unassigned", interval, c.collect)
			}()
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
	// Log each remote share's reported status so a "HA shows mounted but Unraid
	// shows unmounted" report can be checked against what the agent actually
	// served (debug level only). ha-unraid-management-agent#83.
	for i := range remoteShares {
		s := &remoteShares[i]
		logger.Debug("Remote share status: source=%q type=%s status=%s mount=%q",
			s.Source, s.Type, s.Status, s.MountPoint)
	}
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

// collectRemoteShares collects remote SMB/NFS/ISO shares mounted by the
// Unassigned Devices plugin under /mnt/remotes/ and /mnt/disks/.
func (c *UnassignedCollector) collectRemoteShares() []dto.UnassignedRemoteShare {
	if !c.isPluginInstalled() {
		return []dto.UnassignedRemoteShare{}
	}

	// Read /proc/mounts once and parse all remote share types from it.
	mounts, err := os.ReadFile("/proc/mounts")
	if err != nil {
		logger.Debug("Failed to read /proc/mounts: %v", err)
		return []dto.UnassignedRemoteShare{}
	}
	procMounts := string(mounts)
	now := time.Now()

	// Parse currently-mounted SMB (CIFS) and NFS network shares.
	mounted := parseRemoteShareMounts(procMounts, now)

	// Enumerate configured shares from the plugin's samba_mount.cfg so that
	// shares which are configured but not currently mounted are still reported
	// (status "unmounted"), enabling mount/unmount toggles in consumers.
	configured := parseConfiguredRemoteShares(readSambaConfig(), now)

	// Merge: configured shares carry automount/read-only metadata and surface
	// unmounted entries; mounted shares carry live status and capacity.
	shares := mergeRemoteShares(configured, mounted)

	// Parse ISO mounts (loop devices) only when the plugin's ISO config exists,
	// preserving the historical gate that avoids false positives.
	if _, err := os.Stat("/boot/config/plugins/unassigned.devices/iso_mount.cfg"); err == nil {
		shares = append(shares, parseISOMountsFromProc(procMounts, now)...)
	}

	// Populate capacity information for mounted shares via statfs.
	for i := range shares {
		if shares[i].Status == "mounted" && shares[i].MountPoint != "" {
			c.getRemoteShareSizeInfo(&shares[i], shares[i].MountPoint)
		}
	}

	return shares
}

// readSambaConfig returns the contents of the Unassigned Devices SMB/NFS remote
// share configuration file, or an empty string if it is absent/unreadable.
func readSambaConfig() string {
	data, err := os.ReadFile(constants.UnassignedSambaMountCfg)
	if err != nil {
		return ""
	}
	return string(data)
}

// mergeRemoteShares combines configured shares (which may be unmounted) with the
// live set parsed from /proc/mounts. A configured share that is currently
// mounted adopts the live mount data while retaining its automount metadata;
// configured-but-unmounted shares are reported as "unmounted". Mounted shares
// with no matching config entry (e.g. manually mounted) are preserved.
func mergeRemoteShares(configured, mounted []dto.UnassignedRemoteShare) []dto.UnassignedRemoteShare {
	matched := make(map[int]bool, len(mounted))
	result := make([]dto.UnassignedRemoteShare, 0, len(configured)+len(mounted))

	for _, cfg := range configured {
		live := -1
		for i := range mounted {
			if matched[i] {
				continue
			}
			if mounted[i].Source == cfg.Source ||
				(cfg.MountPoint != "" && mounted[i].MountPoint == cfg.MountPoint) {
				live = i
				break
			}
		}
		if live >= 0 {
			matched[live] = true
			m := mounted[live]
			m.AutoMount = cfg.AutoMount
			result = append(result, m)
		} else {
			result = append(result, cfg)
		}
	}

	// Preserve any mounted shares that were not present in the config.
	for i := range mounted {
		if !matched[i] {
			result = append(result, mounted[i])
		}
	}

	return result
}

// unassignedRemotePrefixes are the mount-point roots used by the Unassigned
// Devices plugin for remote (network) shares.
var unassignedRemotePrefixes = []string{"/mnt/remotes/", "/mnt/disks/"}

// rootShareMountPrefix is where the Unassigned Devices "Root Share" feature
// gathers local Unraid shares (one fuse.shfs mount per named root share).
const rootShareMountPrefix = "/mnt/rootshare/"

// isUnassignedRemoteMount reports whether a mount point belongs to one of the
// Unassigned Devices remote-share locations.
func isUnassignedRemoteMount(mountPoint string) bool {
	for _, prefix := range unassignedRemotePrefixes {
		if strings.HasPrefix(mountPoint, prefix) {
			return true
		}
	}
	return false
}

// mountHasOption reports whether a comma-separated /proc/mounts options field
// contains the given option token (exact match, not substring).
func mountHasOption(options, want string) bool {
	for opt := range strings.SplitSeq(options, ",") {
		if opt == want {
			return true
		}
	}
	return false
}

// unescapeMountField decodes the octal escapes (\040 space, \011 tab,
// \012 newline, \134 backslash) that the kernel uses in /proc/mounts fields.
func unescapeMountField(field string) string {
	if !strings.Contains(field, `\`) {
		return field
	}
	var b strings.Builder
	for i := 0; i < len(field); i++ {
		// A valid single-byte octal escape is \ followed by three octal digits
		// whose value fits in a byte (0..255), so the leading digit is 0..3.
		if field[i] == '\\' && i+3 < len(field) &&
			field[i+1] >= '0' && field[i+1] <= '3' &&
			field[i+2] >= '0' && field[i+2] <= '7' &&
			field[i+3] >= '0' && field[i+3] <= '7' {
			val := (int(field[i+1]-'0') << 6) | (int(field[i+2]-'0') << 3) | int(field[i+3]-'0')
			b.WriteByte(byte(val & 0xFF))
			i += 3
			continue
		}
		b.WriteByte(field[i])
	}
	return b.String()
}

// parseSMBSource splits a CIFS source ("//server/share") into server and share.
func parseSMBSource(source string) (server, share string) {
	trimmed := strings.TrimPrefix(source, "//")
	if idx := strings.Index(trimmed, "/"); idx >= 0 {
		return trimmed[:idx], trimmed[idx+1:]
	}
	return trimmed, ""
}

// parseNFSSource splits an NFS source ("server:/export") into server and export.
// It splits on the ":/" delimiter so bracketed IPv6 hosts (e.g.
// "[fe80::1]:/export") are handled correctly rather than splitting inside the
// address.
func parseNFSSource(source string) (server, export string) {
	if idx := strings.Index(source, ":/"); idx >= 0 {
		return source[:idx], source[idx+1:]
	}
	return source, ""
}

// parseRemoteShareMounts parses /proc/mounts content for CIFS and NFS remote
// shares mounted under the Unassigned Devices locations. Capacity fields are
// left zero; callers populate them via getRemoteShareSizeInfo.
func parseRemoteShareMounts(procMounts string, now time.Time) []dto.UnassignedRemoteShare {
	var shares []dto.UnassignedRemoteShare
	for line := range strings.SplitSeq(procMounts, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		source := unescapeMountField(fields[0])
		mountPoint := unescapeMountField(fields[1])
		fsType := fields[2]
		options := fields[3]

		// Root Shares (the UD "Root Share" feature) gather local Unraid shares
		// under /mnt/rootshare/<name> as a fuse.shfs mount, so they are matched by
		// mount point rather than fsType. The bare /mnt/rootshare tmpfs (no name)
		// is skipped.
		if name, ok := strings.CutPrefix(mountPoint, rootShareMountPrefix); ok && name != "" {
			shares = append(shares, dto.UnassignedRemoteShare{
				Type:       "root",
				Source:     name,
				MountPoint: mountPoint,
				Status:     "mounted",
				ReadOnly:   mountHasOption(options, "ro"),
				Timestamp:  now,
			})
			continue
		}

		if !isUnassignedRemoteMount(mountPoint) {
			continue
		}
		readOnly := mountHasOption(options, "ro")

		switch fsType {
		case "cifs", "smb3", "smbfs":
			server, share := parseSMBSource(source)
			shares = append(shares, dto.UnassignedRemoteShare{
				Type:       "smb",
				Source:     source,
				MountPoint: mountPoint,
				Status:     "mounted",
				ReadOnly:   readOnly,
				SMBServer:  server,
				SMBShare:   share,
				Timestamp:  now,
			})
		case "nfs", "nfs4":
			server, export := parseNFSSource(source)
			shares = append(shares, dto.UnassignedRemoteShare{
				Type:       "nfs",
				Source:     source,
				MountPoint: mountPoint,
				Status:     "mounted",
				ReadOnly:   readOnly,
				NFSServer:  server,
				NFSExport:  export,
				NFSOptions: options,
				Timestamp:  now,
			})
		}
	}
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
	output, err := lib.ExecCommandOutput("lsblk", "-d", "-n", "-o", "NAME")
	if err != nil {
		logger.Error("Failed to list block devices: %v", err)
		return []string{}
	}

	lines := strings.Split(strings.TrimSpace(output), "\n")
	return lines
}

// isArrayDisk checks if a device is part of the Unraid array
func (c *UnassignedCollector) isArrayDisk(device string, arrayDisks map[string]bool) bool {
	return arrayDisks[device]
}

// getDeviceInfo retrieves detailed information about a device
func (c *UnassignedCollector) getDeviceInfo(device string) *dto.UnassignedDevice {
	// Get device info using lsblk
	// Use stdout-only helper so stderr warnings don't contaminate JSON output.
	output, err := lib.ExecCommandStdout("lsblk", "-J", "-o", "NAME,SIZE,TYPE,MOUNTPOINT,FSTYPE,LABEL,SERIAL,MODEL", "/dev/"+device)
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

	if err := json.Unmarshal([]byte(output), &lsblkOutput); err != nil {
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

// remoteStatfsTimeout bounds statfs probes on remote (network) mount points. A
// CIFS/NFS mount whose server is unreachable blocks statfs in uninterruptible
// sleep for minutes; with many remote shares this wedged the whole collector
// for hours after boot on isolated networks (issue #123).
const remoteStatfsTimeout = 5 * time.Second

// statfsFn is swappable in tests to simulate hung network mounts.
var statfsFn = syscall.Statfs

func getFilesystemUsage(path string) (size, used, free uint64, usagePercent float64, err error) {
	var stat syscall.Statfs_t
	if err := statfsFn(path, &stat); err != nil {
		return 0, 0, 0, 0, err
	}

	blockSize := uint64(stat.Bsize)
	size = stat.Blocks * blockSize
	used = (stat.Blocks - stat.Bfree) * blockSize
	free = stat.Bavail * blockSize

	if size > 0 {
		usagePercent = float64(used) / float64(size) * 100.0
	}

	return size, used, free, usagePercent, nil
}

// getPartitionSizeInfo retrieves size information for a mounted partition
func (c *UnassignedCollector) getPartitionSizeInfo(partition *dto.UnassignedPartition, mountPoint string) {
	size, used, free, usagePercent, err := getFilesystemUsage(mountPoint)
	if err != nil {
		return
	}

	partition.Size = size
	partition.Used = used
	partition.Free = free
	partition.UsagePercent = usagePercent
}

// parseINISections parses a simple INI document into a map of
// section -> key -> value. Section headers may optionally be quoted
// (e.g. ["//server/share"]); surrounding double quotes on both section names
// and values are stripped. This matches the format the Unassigned Devices
// plugin writes via Unraid's save_ini_file.
func parseINISections(data string) map[string]map[string]string {
	sections := make(map[string]map[string]string)
	var current string
	for line := range strings.SplitSeq(data, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			name := strings.TrimSpace(line[1 : len(line)-1])
			name = strings.Trim(name, "\"")
			current = name
			if _, ok := sections[current]; !ok {
				sections[current] = make(map[string]string)
			}
			continue
		}
		if current == "" {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), "\"")
		sections[current][key] = value
	}
	return sections
}

// parseConfiguredRemoteShares builds the list of SMB/NFS remote shares defined
// in samba_mount.cfg. Each section key is the share source (//server/share for
// SMB, server:/export for NFS) — the same identifier the Unassigned Devices
// plugin's rc.unassigned script accepts for mount/unmount. Shares are reported
// as "unmounted"; collectRemoteShares overlays live mount status and capacity.
func parseConfiguredRemoteShares(cfgData string, now time.Time) []dto.UnassignedRemoteShare {
	if strings.TrimSpace(cfgData) == "" {
		return nil
	}

	var shares []dto.UnassignedRemoteShare
	for source, cfg := range parseINISections(cfgData) {
		if source == "" {
			continue
		}

		// Determine the share type from the configured protocol, falling back
		// to the source string format.
		shareType := ""
		switch strings.ToUpper(cfg["protocol"]) {
		case "SMB", "CIFS":
			shareType = "smb"
		case "NFS":
			shareType = "nfs"
		case "ROOT":
			// Unassigned Devices "Root Share" — surfaced (type "root") so consumers
			// can see it. The configured source is an SMB-style //server/path.
			shareType = "root"
		default:
			switch {
			case strings.HasPrefix(source, "//"):
				shareType = "smb"
			case strings.Contains(source, ":/"):
				shareType = "nfs"
			default:
				continue
			}
		}

		share := dto.UnassignedRemoteShare{
			Type:       shareType,
			Source:     source,
			MountPoint: configuredRemoteMountPoint(source, shareType, cfg["mountpoint"]),
			Status:     "unmounted",
			AutoMount:  cfg["automount"] == "yes",
			ReadOnly:   cfg["read_only"] == "yes",
			Timestamp:  now,
		}
		if shareType == "smb" || shareType == "root" {
			share.SMBServer, share.SMBShare = parseSMBSource(source)
		} else {
			share.NFSServer, share.NFSExport = parseNFSSource(source)
		}
		shares = append(shares, share)
	}
	return shares
}

// configuredRemoteMountPoint computes the expected mount point for a configured
// remote share, mirroring the Unassigned Devices convention of mounting under
// /mnt/remotes/. This is best-effort for display of unmounted shares; mounted
// shares use their actual mount point from /proc/mounts.
func configuredRemoteMountPoint(source, shareType, override string) string {
	// Root Shares mount under /mnt/rootshare/, all other remote shares under
	// /mnt/remotes/. For unmounted shares this is best-effort display only;
	// mounted shares use their actual mount point from /proc/mounts.
	prefix := "/mnt/remotes/"
	if shareType == "root" {
		prefix = rootShareMountPrefix
	}
	if override != "" {
		return prefix + filepath.Base(override)
	}
	var server, path string
	if shareType == "smb" || shareType == "root" {
		server, path = parseSMBSource(source)
	} else {
		server, path = parseNFSSource(source)
	}
	path = strings.Trim(path, "/")
	name := strings.ReplaceAll(path, "/", "_")
	if server == "" && name == "" {
		return ""
	}
	return prefix + server + "_" + name
}

// parseISOMountsFromProc parses /proc/mounts content for ISO files mounted as
// loop devices under /mnt/disks/ by the Unassigned Devices plugin. Capacity
// fields are left zero; callers populate them via getRemoteShareSizeInfo.
func parseISOMountsFromProc(procMounts string, now time.Time) []dto.UnassignedRemoteShare {
	var isoShares []dto.UnassignedRemoteShare
	for line := range strings.SplitSeq(procMounts, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		source := unescapeMountField(fields[0])
		mountPoint := unescapeMountField(fields[1])

		// An ISO mount is a loop device mounted under /mnt/disks/.
		if strings.HasPrefix(source, "/dev/loop") && strings.HasPrefix(mountPoint, "/mnt/disks/") {
			isoShares = append(isoShares, dto.UnassignedRemoteShare{
				Type:       "iso",
				Source:     source,
				MountPoint: mountPoint,
				Status:     "mounted",
				ReadOnly:   true,
				AutoMount:  false,
				Timestamp:  now,
			})
		}
	}
	return isoShares
}

// getFilesystemUsageTimed runs getFilesystemUsage with a timeout. statfs on a
// dead network mount blocks in uninterruptible sleep and cannot be cancelled,
// so on timeout the probe goroutine is abandoned (it exits once the kernel
// call eventually returns) and an error is returned so the caller skips
// capacity data instead of blocking the collector.
// maxConcurrentStatfsProbes bounds how many statfs probe goroutines may be
// outstanding at once. A probe that hangs on a dead network mount cannot be
// cancelled — its goroutine is held until the kernel call eventually returns —
// so without a cap these abandoned goroutines (and the OS threads they pin in
// the blocking syscall) would grow without bound while a remote server is
// unreachable, eventually exhausting memory and crashing the daemon. Healthy
// probes return in microseconds and release their slot immediately, so this
// cap only ever engages when mounts are genuinely wedged.
const maxConcurrentStatfsProbes = 16

var statfsProbeSlots = make(chan struct{}, maxConcurrentStatfsProbes)

func getFilesystemUsageTimed(path string, timeout time.Duration) (size, used, free uint64, usagePercent float64, err error) {
	// Refuse to start a new probe if too many earlier ones are still wedged.
	select {
	case statfsProbeSlots <- struct{}{}:
	default:
		return 0, 0, 0, 0, fmt.Errorf("statfs on %s skipped: %d probes already stalled (unreachable mounts?)", path, maxConcurrentStatfsProbes)
	}

	type usageResult struct {
		size, used, free uint64
		usagePercent     float64
		err              error
	}
	ch := make(chan usageResult, 1)
	go func() {
		// Release the slot when (if) the kernel returns. A leaked/hung probe holds
		// its slot until then, which is exactly what bounds the abandoned count.
		defer func() { <-statfsProbeSlots }()
		var r usageResult
		r.size, r.used, r.free, r.usagePercent, r.err = getFilesystemUsage(path)
		ch <- r
	}()

	select {
	case r := <-ch:
		return r.size, r.used, r.free, r.usagePercent, r.err
	case <-time.After(timeout):
		return 0, 0, 0, 0, fmt.Errorf("statfs on %s timed out after %v (unreachable network mount?)", path, timeout)
	}
}

// getRemoteShareSizeInfo retrieves size information for a remote share. The
// statfs probe is bounded so an unreachable SMB/NFS server cannot wedge the
// collector (issue #123); on timeout the share keeps zero capacity fields.
func (c *UnassignedCollector) getRemoteShareSizeInfo(share *dto.UnassignedRemoteShare, mountPoint string) {
	size, used, free, usagePercent, err := getFilesystemUsageTimed(mountPoint, remoteStatfsTimeout)
	if err != nil {
		logger.Debug("Unassigned: skipping capacity for %s: %v", mountPoint, err)
		return
	}

	share.Size = size
	share.Used = used
	share.Free = free
	share.UsagePercent = usagePercent
}
