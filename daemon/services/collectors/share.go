package collectors

import (
	"bufio"
	"context"
	"errors"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/tinkerologie/unraid-management-agent/daemon/constants"
	"github.com/tinkerologie/unraid-management-agent/daemon/domain"
	"github.com/tinkerologie/unraid-management-agent/daemon/dto"
	"github.com/tinkerologie/unraid-management-agent/daemon/lib"
	"github.com/tinkerologie/unraid-management-agent/daemon/logger"
)

// ShareCollector collects information about Unraid user shares.
// It gathers share configuration, usage statistics, and disk allocation details.
type ShareCollector struct {
	ctx *domain.Context

	// paths locates shares.ini, var.ini, disks.ini and /mnt; the zero value
	// uses the production paths.
	paths sharePaths

	// runCommand is overridden by tests; production uses the context-aware lib helper.
	runCommand func(context.Context, string, ...string) (string, error)
}

// NewShareCollector creates a new user share collector with the given context.
func NewShareCollector(ctx *domain.Context) *ShareCollector {
	return &ShareCollector{ctx: ctx}
}

// Collect gathers user share information and publishes it to the event bus.
// It reads emhttp storage counters and share configuration, then measures share
// content only on assigned ZFS pools whose members are all known spinning.
func (c *ShareCollector) Collect(ctx context.Context) {
	logger.Debug("Collecting share data...")

	// OS-resilience: validate shares.ini is readable before attempting the full parse.
	// A read error → unavailable (return without publishing); zero parsed shares → degraded
	// but still publish best-effort; otherwise report healthy.
	if c.ctx.Platform != nil {
		f, openErr := os.Open(c.paths.shares())
		if openErr != nil {
			c.ctx.Platform.Report("shares", dto.SourceUnavailable, "cannot read shares.ini", openErr)
			logger.Error("Share: Failed to open %s: %v", c.paths.shares(), openErr)
			return // nothing valid to publish
		}
		_ = f.Close()
	}

	// Collect share information
	shares, err := c.collectShares(ctx)
	if err != nil {
		logger.Error("Share: Failed to collect share data: %v", err)
		return
	}

	// Report source health and attach inline flag when not healthy.
	if c.ctx.Platform != nil {
		if len(shares) == 0 {
			c.ctx.Platform.Report("shares", dto.SourceDegraded, "shares.ini yielded zero shares", nil)
		} else {
			c.ctx.Platform.Healthy("shares")
		}
		if st := c.ctx.Platform.StatusFor("shares"); st != nil {
			for i := range shares {
				shares[i].SourceStatus = st
			}
		}
	}

	logger.Debug("Share: Successfully collected %d shares, publishing event", len(shares))
	// Publish event
	domain.Publish(c.ctx.Hub, constants.TopicShareListUpdate, shares)
	logger.Debug("Share: Published %s event with %d shares", constants.TopicShareListUpdate.Name, len(shares))
}

func (c *ShareCollector) collectShares(ctx context.Context) ([]dto.ShareInfo, error) {
	const kibToBytes uint64 = 1024

	logger.Debug("Share: Starting collection from %s", c.paths.shares())
	var shares []dto.ShareInfo

	// Parse shares.ini
	file, err := os.Open(c.paths.shares())
	if err != nil {
		logger.Error("Share: Failed to open file: %v", err)
		return nil, err
	}
	defer func() {
		if err := file.Close(); err != nil {
			logger.Debug("Error closing share file: %v", err)
		}
	}()
	logger.Debug("Share: File opened successfully")

	scanner := bufio.NewScanner(file)
	var currentShare *dto.ShareInfo
	var currentShareName string

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Check for section header: [shareName="appdata"]
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			// Save previous share if exists
			if currentShare != nil {
				shares = append(shares, *currentShare)
			}

			// Extract share name from [shareName="appdata"]
			if strings.Contains(line, "=") {
				parts := strings.SplitN(line[1:len(line)-1], "=", 2)
				if len(parts) == 2 {
					currentShareName = strings.Trim(parts[1], `"`)
				}
			}

			// Start new share
			currentShare = &dto.ShareInfo{
				Name: currentShareName,
			}
			continue
		}

		// Parse key=value pairs
		if currentShare != nil && strings.Contains(line, "=") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) != 2 {
				continue
			}

			key := strings.TrimSpace(parts[0])
			value := strings.Trim(strings.TrimSpace(parts[1]), `"`)

			switch key {
			case "name":
				// Use the name field from the INI file
				currentShare.Name = value
			case "size":
				if size, err := strconv.ParseUint(value, 10, 64); err == nil {
					currentShare.StorageTotal = size * kibToBytes // shares.ini stores size in KiB (1024-byte blocks)
				}
			case "free":
				if free, err := strconv.ParseUint(value, 10, 64); err == nil {
					currentShare.StorageFree = free * kibToBytes // shares.ini stores free in KiB (1024-byte blocks)
				}
			case "used":
				if used, err := strconv.ParseUint(value, 10, 64); err == nil {
					currentShare.StorageUsed = used * kibToBytes // shares.ini stores used in KiB (1024-byte blocks)
				}
			// Cache settings from shares.ini (Issue #53)
			case "useCache":
				currentShare.UseCache = value
			case "cachePool":
				currentShare.CachePool = value
			case "cachePool2":
				currentShare.CachePool2 = value
			case "exclusive":
				// Set by emhttp (6.12+) once /mnt/user/<share> is a symlink
				// to the pool; completed by applyExclusiveState.
				exclusive := value == "yes"
				currentShare.Exclusive = &exclusive
			}
		}
	}

	// Save last share
	if currentShare != nil {
		shares = append(shares, *currentShare)
	}

	if err := scanner.Err(); err != nil {
		logger.Error("Share: Scanner error: %v", err)
		return shares, err
	}

	// Preserve the existing storage-total calculation and deprecated aliases.
	for i := range shares {
		share := &shares[i]
		if share.StorageTotal == 0 {
			share.StorageTotal = share.StorageUsed + share.StorageFree
		}
		if share.StorageTotal > 0 {
			share.StorageUsagePercent = float64(share.StorageUsed) / float64(share.StorageTotal) * 100
		}
		share.Free, share.Total = share.StorageFree, share.StorageTotal
		share.UsagePercent = share.StorageUsagePercent
		share.Timestamp = time.Now()
	}

	// Enrich shares with configuration data
	configCollector := &ConfigCollector{bootConfigDir: c.paths.bootConfigDir}
	exportUnknown := map[string]bool{}
	for i := range shares {
		if !c.enrichShareWithConfig(&shares[i], configCollector) {
			exportUnknown[shares[i].Name] = true
		}
	}
	c.enrichShareUsage(ctx, shares)
	// ZFS commands can block long enough for disks to spin down. Only a fresh
	// snapshot may authorize the exclusive-state filesystem probes.
	c.paths.applyExclusiveState(shares, exportUnknown, c.paths.readDisks())

	logger.Debug("Share: Parsed %d shares successfully", len(shares))
	return shares, nil
}

// enrichShareWithConfig enriches a share with configuration data. It returns
// false when the share's config exists but cannot be read, so its export
// settings are unknown (a missing config means Unraid's defaults: not exported).
func (c *ShareCollector) enrichShareWithConfig(share *dto.ShareInfo, configCollector *ConfigCollector) bool {
	config, err := configCollector.GetShareConfig(share.Name)
	if err != nil {
		logger.Debug("Share: Failed to get config for share %s: %v", share.Name, err)
		// Set default values for shares without config
		share.Storage = "unknown"
		share.SMBExport = false
		share.NFSExport = false
		return errors.Is(err, errShareConfigNotFound)
	}

	// Populate configuration fields
	share.Comment = config.Comment
	// Only override UseCache if not already set from shares.ini
	if share.UseCache == "" {
		share.UseCache = config.UseCache
	}
	share.Security = config.Security
	share.Storage = c.determineStorage(share.UseCache)
	share.SMBExport = c.isSMBExported(config.Export, config.Security)
	share.NFSExport = c.isNFSExported(config.ExportNFS)

	// Determine mover action based on cache settings (Issue #53)
	share.MoverAction = c.determineMoverAction(share.UseCache, share.CachePool, share.CachePool2)

	logger.Debug("Share: Enriched %s - Storage: %s, SMB: %v, NFS: %v, Cache: %s", share.Name, share.Storage, share.SMBExport, share.NFSExport, share.UseCache)
	return true
}

// determineStorage determines storage location based on UseCache setting
func (c *ShareCollector) determineStorage(useCache string) string {
	switch useCache {
	case "no":
		return "array"
	case "only":
		return "cache"
	case "yes", "prefer":
		return "cache+array"
	default:
		return "unknown"
	}
}

// isSMBExported checks if share is exported via SMB
func (c *ShareCollector) isSMBExported(export string, security string) bool {
	// Primary: use the authoritative "e"/"-" export flag.
	switch export {
	case "e":
		return true
	case "-":
		return false
	}

	if export == "" {
		// Fallback: if security is set, share is typically SMB exported.
		if security == "public" || security == "private" || security == "secure" {
			return true
		}
		return false
	}

	// Legacy fallback: substring checks for older config formats.
	return strings.Contains(export, "smb") || strings.Contains(export, "-e")
}

// isNFSExported checks if share is exported via NFS using the shareExportNFS flag
func (c *ShareCollector) isNFSExported(exportNFS string) bool {
	return exportNFS == "e"
}

// enrichShareUsage queries each eligible, awake ZFS pool once. No filesystem
// walks or probes of sleeping/unknown devices are used to measure share content.
func (c *ShareCollector) enrichShareUsage(ctx context.Context, shares []dto.ShareInfo) {
	if c.ctx.Intervals.ZFS <= 0 {
		return
	}
	pools := map[string][]int{}
	for i, share := range shares {
		if share.UseCache == "only" && share.CachePool2 == "" &&
			validateShareName(share.Name) == nil && validateShareName(share.CachePool) == nil {
			pools[share.CachePool] = append(pools[share.CachePool], i)
		}
	}
	run := c.runCommand
	if run == nil {
		run = lib.ExecCommandOutputWithContext
	}
	for pool, indexes := range pools {
		// A preceding pool query may have blocked for up to a minute. Refresh
		// every member's spin state immediately before querying this pool.
		disks := c.paths.readDisks()
		spinning := diskSpinStates(disks)
		awake := false
		for section, kv := range disks {
			name := kv["name"]
			if name == "" {
				name = section
			}
			if name == pool && kv["type"] == "Cache" && kv["fsType"] == "zfs" &&
				kv["fsStatus"] == "Mounted" && kv["shareEnabled"] != "no" {
				awake = branchAwake(kv, spinning, true)
				break
			}
		}
		if !awake {
			continue
		}
		cmdCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
		out, err := run(cmdCtx, "zfs", "list", "-Hp", "-t", "filesystem", "-o", "name,refer,mounted,mountpoint", "-r", pool)
		cancel()
		if err != nil {
			logger.Debug("Share: zfs list for pool %s failed: %v", pool, err)
			continue
		}
		for _, i := range indexes {
			if total, valid := shareDatasetUsage(out, pool+"/"+shares[i].Name); valid {
				shares[i].Used = &total
			}
		}
	}
}

// shareDatasetUsage counts referenced bytes only for mounted filesystems at or
// beneath the share dataset's mountpoint. Dataset ancestry alone does not imply
// visibility. Missing or ambiguous mount metadata leaves usage unknown.
func shareDatasetUsage(out, dataset string) (uint64, bool) {
	var rows [][]string
	var mountpoint string
	for line := range strings.SplitSeq(out, "\n") {
		// zfs list -H separates columns with tabs; mountpoints may contain spaces.
		fields := strings.Split(line, "\t")
		if fields[0] != dataset && !strings.HasPrefix(fields[0], dataset+"/") {
			continue
		}
		if len(fields) != 4 {
			return 0, false
		}
		if fields[0] == dataset {
			if mountpoint != "" || fields[2] != "yes" || !path.IsAbs(fields[3]) {
				return 0, false
			}
			mountpoint = path.Clean(fields[3])
		}
		rows = append(rows, fields)
	}
	if mountpoint == "" {
		return 0, false
	}
	var total uint64
	seen := make(map[string]bool, len(rows))
	for _, fields := range rows {
		if fields[2] == "no" {
			continue
		}
		if fields[2] != "yes" || !path.IsAbs(fields[3]) {
			return 0, false
		}
		childMountpoint := path.Clean(fields[3])
		// Filesystems stacked on one mountpoint hide each other, so the visible size is unknown.
		if seen[childMountpoint] {
			return 0, false
		}
		seen[childMountpoint] = true
		if childMountpoint != mountpoint && !strings.HasPrefix(childMountpoint, strings.TrimSuffix(mountpoint, "/")+"/") {
			continue
		}
		bytes, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil || bytes > ^uint64(0)-total {
			return 0, false
		}
		total += bytes
	}
	return total, true
}

// determineMoverAction determines the mover action based on cache settings
// This addresses Issue #53: Expose share cache settings via API
func (c *ShareCollector) determineMoverAction(useCache, cachePool, cachePool2 string) string {
	// If cachePool2 is set, there's a secondary destination (pool-to-pool movement)
	if cachePool2 != "" && cachePool != "" {
		return cachePool + "->" + cachePool2
	}

	switch useCache {
	case "yes", "prefer":
		// Cache preferred - mover moves from cache to array
		if cachePool != "" {
			return "cache->array"
		}
	case "only":
		// Cache only - no mover action (data stays on cache)
		return ""
	case "no":
		// Array only - no mover action (data never on cache)
		return ""
	}

	return ""
}
