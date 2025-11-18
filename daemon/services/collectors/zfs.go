package collectors

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/constants"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/lib"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
)

// ZFSCollector collects ZFS pool, dataset, and ARC statistics
type ZFSCollector struct {
	ctx *domain.Context
}

// NewZFSCollector creates a new ZFS collector
func NewZFSCollector(ctx *domain.Context) *ZFSCollector {
	return &ZFSCollector{ctx: ctx}
}

// Start begins the ZFS collection loop
func (c *ZFSCollector) Start(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	logger.Info("ZFS collector started", "interval", interval)

	// Collect immediately on start
	c.collect()

	for {
		select {
		case <-ctx.Done():
			logger.Info("ZFS collector stopped")
			return
		case <-ticker.C:
			c.collect()
		}
	}
}

// collect gathers all ZFS data and publishes events
func (c *ZFSCollector) collect() {
	defer func() {
		if r := recover(); r != nil {
			logger.Error("ZFS collector panic recovered", "error", r)
		}
	}()

	// Check if ZFS is available
	if !c.isZFSAvailable() {
		logger.Debug("ZFS not available, skipping collection")
		return
	}

	// Collect pools
	pools, err := c.collectPools()
	if err != nil {
		logger.Warning("Failed to collect ZFS pools", "error", err)
	} else if len(pools) > 0 {
		c.ctx.Hub.Pub(pools, "zfs_pools_update")
		logger.Debug("Published ZFS pools update", "count", len(pools))
	}

	// Collect datasets
	datasets, err := c.collectDatasets()
	if err != nil {
		logger.Warning("Failed to collect ZFS datasets", "error", err)
	} else if len(datasets) > 0 {
		c.ctx.Hub.Pub(datasets, "zfs_datasets_update")
		logger.Debug("Published ZFS datasets update", "count", len(datasets))
	}

	// Collect snapshots
	snapshots, err := c.collectSnapshots()
	if err != nil {
		logger.Warning("Failed to collect ZFS snapshots", "error", err)
	} else if len(snapshots) > 0 {
		c.ctx.Hub.Pub(snapshots, "zfs_snapshots_update")
		logger.Debug("Published ZFS snapshots update", "count", len(snapshots))
	}

	// Collect ARC stats
	arcStats, err := c.collectARCStats()
	if err != nil {
		logger.Warning("Failed to collect ZFS ARC stats", "error", err)
	} else {
		c.ctx.Hub.Pub(arcStats, "zfs_arc_stats_update")
		logger.Debug("Published ZFS ARC stats update")
	}
}

// isZFSAvailable checks if ZFS kernel module is loaded and binaries exist
func (c *ZFSCollector) isZFSAvailable() bool {
	// Check if zpool binary exists
	if _, err := os.Stat(constants.ZpoolBin); os.IsNotExist(err) {
		return false
	}

	// Try to execute zpool list to verify ZFS is functional
	_, err := lib.ExecCommandOutput(constants.ZpoolBin, "list", "-H")
	return err == nil
}

// collectPools collects information about all ZFS pools
func (c *ZFSCollector) collectPools() ([]dto.ZFSPool, error) {
	// Get list of pool names
	output, err := lib.ExecCommandOutput(constants.ZpoolBin, "list", "-H", "-o", "name")
	if err != nil {
		return nil, fmt.Errorf("failed to list pools: %w", err)
	}

	poolNames := strings.Split(strings.TrimSpace(output), "\n")
	if len(poolNames) == 0 || poolNames[0] == "" {
		return []dto.ZFSPool{}, nil
	}

	pools := make([]dto.ZFSPool, 0, len(poolNames))
	for _, name := range poolNames {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}

		pool, err := c.collectPoolDetails(name)
		if err != nil {
			logger.Warning("Failed to collect pool details", "pool", name, "error", err)
			continue
		}

		pools = append(pools, pool)
	}

	return pools, nil
}

// collectPoolDetails collects detailed information about a specific pool
func (c *ZFSCollector) collectPoolDetails(name string) (dto.ZFSPool, error) {
	pool := dto.ZFSPool{
		Name:      name,
		Timestamp: time.Now(),
	}

	// Get basic pool info (parseable format)
	// Fields: name, size, allocated, free, fragmentation, capacity, dedupratio, health, altroot
	output, err := lib.ExecCommandOutput(constants.ZpoolBin, "list", "-Hp", "-o",
		"name,size,allocated,free,fragmentation,capacity,dedupratio,health,altroot", name)
	if err != nil {
		return pool, fmt.Errorf("failed to get pool info: %w", err)
	}

	// Parse tab-separated values
	fields := strings.Split(strings.TrimSpace(output), "\t")
	if len(fields) < 9 {
		return pool, fmt.Errorf("unexpected pool info format: got %d fields", len(fields))
	}

	pool.SizeBytes, _ = strconv.ParseUint(fields[1], 10, 64)
	pool.AllocatedBytes, _ = strconv.ParseUint(fields[2], 10, 64)
	pool.FreeBytes, _ = strconv.ParseUint(fields[3], 10, 64)

	// Parse fragmentation and capacity (can be "-" if not available)
	if fields[4] != "-" {
		pool.FragmentationPct, _ = strconv.ParseFloat(fields[4], 64)
	}
	if fields[5] != "-" {
		pool.CapacityPct, _ = strconv.ParseFloat(fields[5], 64)
	}

	// Parse dedup ratio (format: "1.00x" or "1.00")
	dedupStr := strings.TrimSuffix(fields[6], "x")
	pool.DedupRatio, _ = strconv.ParseFloat(dedupStr, 64)

	pool.Health = fields[7]

	// Altroot (can be "-" if not set)
	if fields[8] != "-" {
		pool.Altroot = fields[8]
	}

	// Get pool properties for additional details
	if err := c.enrichPoolProperties(&pool); err != nil {
		logger.Warning("Failed to enrich pool properties", "pool", name, "error", err)
	}

	// Get pool status (vdevs, errors, scrub info)
	if err := c.parsePoolStatus(&pool); err != nil {
		logger.Warning("Failed to parse pool status", "pool", name, "error", err)
	}

	return pool, nil
}

// enrichPoolProperties adds additional properties from 'zpool get all'
func (c *ZFSCollector) enrichPoolProperties(pool *dto.ZFSPool) error {
	output, err := lib.ExecCommandOutput(constants.ZpoolBin, "get", "-Hp", "-o", "property,value",
		"guid,readonly,autoexpand,autotrim", pool.Name)
	if err != nil {
		return err
	}

	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		fields := strings.Split(line, "\t")
		if len(fields) < 2 {
			continue
		}

		property := fields[0]
		value := fields[1]

		switch property {
		case "guid":
			pool.GUID = value
		case "readonly":
			pool.Readonly = value == "on"
		case "autoexpand":
			pool.Autoexpand = value == "on"
		case "autotrim":
			pool.Autotrim = value
		}
	}

	return scanner.Err()
}

// parsePoolStatus parses 'zpool status' output for vdevs, errors, and scrub info
func (c *ZFSCollector) parsePoolStatus(pool *dto.ZFSPool) error {
	output, err := lib.ExecCommandOutput(constants.ZpoolBin, "status", "-v", pool.Name)
	if err != nil {
		return err
	}

	scanner := bufio.NewScanner(strings.NewReader(output))
	inConfig := false
	var currentVdev *dto.ZFSVdev

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Parse state
		if state, found := strings.CutPrefix(trimmed, "state:"); found {
			pool.State = strings.TrimSpace(state)
		}

		// Parse scan/scrub info
		if strings.HasPrefix(trimmed, "scan:") {
			c.parseScanInfo(pool, trimmed)
		}

		// Parse errors line
		if strings.HasPrefix(trimmed, "errors:") {
			// Error summary is in the line, but individual errors are in vdev stats
			continue
		}

		// Parse config section (vdev tree)
		if strings.HasPrefix(trimmed, "config:") {
			inConfig = true
			continue
		}

		if inConfig && trimmed != "" && !strings.HasPrefix(trimmed, "NAME") {
			// Parse vdev line
			vdev := c.parseVdevLine(line)
			if vdev != nil {
				// Determine if this is a top-level vdev or a device
				indent := len(line) - len(strings.TrimLeft(line, "\t "))

				if indent <= 1 {
					// Top-level vdev (pool itself)
					pool.ReadErrors = vdev.ReadErrors
					pool.WriteErrors = vdev.WriteErrors
					pool.ChecksumErrors = vdev.ChecksumErrors
				} else if indent <= 3 {
					// Mid-level vdev (raidz, mirror, etc.)
					if currentVdev != nil {
						pool.VDEVs = append(pool.VDEVs, *currentVdev)
					}
					currentVdev = vdev
				} else {
					// Device within a vdev
					if currentVdev != nil {
						device := dto.ZFSDevice{
							Name:           vdev.Name,
							State:          vdev.State,
							ReadErrors:     vdev.ReadErrors,
							WriteErrors:    vdev.WriteErrors,
							ChecksumErrors: vdev.ChecksumErrors,
						}
						currentVdev.Devices = append(currentVdev.Devices, device)
					}
				}
			}
		}
	}

	// Add last vdev if exists
	if currentVdev != nil {
		pool.VDEVs = append(pool.VDEVs, *currentVdev)
	}

	return scanner.Err()
}

// parseScanInfo parses scrub/resilver information from status output
func (c *ZFSCollector) parseScanInfo(pool *dto.ZFSPool, line string) {
	// Example: "scan: scrub repaired 0B in 00:00:01 with 0 errors on Sun Nov 10 02:39:43 2025"
	// Example: "scan: scrub in progress since Sun Nov 10 02:39:43 2025"
	line = strings.TrimPrefix(line, "scan:")
	line = strings.TrimSpace(line)

	if strings.Contains(line, "in progress") {
		pool.ScanStatus = "in progress"
		pool.ScanState = "scanning"
	} else if strings.Contains(line, "scrub repaired") {
		pool.ScanStatus = "scrub completed"
		pool.ScanState = "finished"

		// Try to parse "with X errors"
		if strings.Contains(line, "with") && strings.Contains(line, "errors") {
			parts := strings.Split(line, "with")
			if len(parts) > 1 {
				errorPart := strings.TrimSpace(parts[1])
				errorFields := strings.Fields(errorPart)
				if len(errorFields) > 0 {
					pool.ScanErrors, _ = strconv.Atoi(errorFields[0])
				}
			}
		}
	} else if strings.Contains(line, "resilver") {
		pool.ScanStatus = "resilver in progress"
		pool.ScanState = "scanning"
	}
}

// parseVdevLine parses a single vdev line from zpool status output
// Format: "NAME        STATE     READ WRITE CKSUM"
// Example: "  sdg1      ONLINE       0     0     0"
func (c *ZFSCollector) parseVdevLine(line string) *dto.ZFSVdev {
	fields := strings.Fields(line)
	if len(fields) < 5 {
		return nil
	}

	vdev := &dto.ZFSVdev{
		Name:  fields[0],
		State: fields[1],
	}

	// Determine vdev type based on name
	if strings.Contains(vdev.Name, "raidz1") {
		vdev.Type = "raidz1"
	} else if strings.Contains(vdev.Name, "raidz2") {
		vdev.Type = "raidz2"
	} else if strings.Contains(vdev.Name, "raidz3") {
		vdev.Type = "raidz3"
	} else if strings.Contains(vdev.Name, "mirror") {
		vdev.Type = "mirror"
	} else if strings.Contains(vdev.Name, "spare") {
		vdev.Type = "spare"
	} else if strings.Contains(vdev.Name, "cache") {
		vdev.Type = "cache"
	} else if strings.Contains(vdev.Name, "log") {
		vdev.Type = "log"
	} else {
		vdev.Type = "disk"
	}

	vdev.ReadErrors, _ = strconv.ParseUint(fields[2], 10, 64)
	vdev.WriteErrors, _ = strconv.ParseUint(fields[3], 10, 64)
	vdev.ChecksumErrors, _ = strconv.ParseUint(fields[4], 10, 64)

	return vdev
}

// collectDatasets collects information about all ZFS datasets
func (c *ZFSCollector) collectDatasets() ([]dto.ZFSDataset, error) {
	// Get all datasets across all pools
	// Fields: name, type, used, available, referenced, compressratio, mountpoint, quota, reservation, compression, readonly
	output, err := lib.ExecCommandOutput(constants.ZfsBin, "list", "-Hp", "-o",
		"name,type,used,available,referenced,compressratio,mountpoint,quota,reservation,compression,readonly")
	if err != nil {
		return nil, fmt.Errorf("failed to list datasets: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(output), "\n")
	datasets := make([]dto.ZFSDataset, 0, len(lines))

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		dataset := c.parseDatasetLine(line)
		if dataset != nil {
			datasets = append(datasets, *dataset)
		}
	}

	return datasets, nil
}

// parseDatasetLine parses a single dataset line from zfs list output
func (c *ZFSCollector) parseDatasetLine(line string) *dto.ZFSDataset {
	fields := strings.Split(line, "\t")
	if len(fields) < 11 {
		return nil
	}

	dataset := &dto.ZFSDataset{
		Name:      fields[0],
		Type:      fields[1],
		Timestamp: time.Now(),
	}

	dataset.UsedBytes, _ = strconv.ParseUint(fields[2], 10, 64)
	dataset.AvailableBytes, _ = strconv.ParseUint(fields[3], 10, 64)
	dataset.ReferencedBytes, _ = strconv.ParseUint(fields[4], 10, 64)

	// Parse compression ratio (format: "1.00x" or "1.00")
	compressStr := strings.TrimSuffix(fields[5], "x")
	dataset.CompressRatio, _ = strconv.ParseFloat(compressStr, 64)

	if fields[6] != "-" {
		dataset.Mountpoint = fields[6]
	}

	dataset.QuotaBytes, _ = strconv.ParseUint(fields[7], 10, 64)
	dataset.ReservationBytes, _ = strconv.ParseUint(fields[8], 10, 64)
	dataset.Compression = fields[9]
	dataset.Readonly = fields[10] == "on"

	return dataset
}

// collectSnapshots collects information about all ZFS snapshots
func (c *ZFSCollector) collectSnapshots() ([]dto.ZFSSnapshot, error) {
	// Get all snapshots
	// Fields: name, used, referenced, creation
	output, err := lib.ExecCommandOutput(constants.ZfsBin, "list", "-t", "snapshot", "-Hp", "-o",
		"name,used,referenced,creation")
	if err != nil {
		// No snapshots is not an error
		if strings.Contains(err.Error(), "no datasets available") {
			return []dto.ZFSSnapshot{}, nil
		}
		return nil, fmt.Errorf("failed to list snapshots: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(output), "\n")
	snapshots := make([]dto.ZFSSnapshot, 0, len(lines))

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		snapshot := c.parseSnapshotLine(line)
		if snapshot != nil {
			snapshots = append(snapshots, *snapshot)
		}
	}

	return snapshots, nil
}

// parseSnapshotLine parses a single snapshot line from zfs list output
func (c *ZFSCollector) parseSnapshotLine(line string) *dto.ZFSSnapshot {
	fields := strings.Split(line, "\t")
	if len(fields) < 4 {
		return nil
	}

	// Parse snapshot name (format: dataset@snapshot)
	parts := strings.Split(fields[0], "@")
	if len(parts) != 2 {
		return nil
	}

	snapshot := &dto.ZFSSnapshot{
		Name:      fields[0],
		Dataset:   parts[0],
		Timestamp: time.Now(),
	}

	snapshot.UsedBytes, _ = strconv.ParseUint(fields[1], 10, 64)
	snapshot.ReferencedBytes, _ = strconv.ParseUint(fields[2], 10, 64)

	// Parse creation time (Unix timestamp)
	creationUnix, _ := strconv.ParseInt(fields[3], 10, 64)
	snapshot.CreationTime = time.Unix(creationUnix, 0)

	return snapshot
}

// collectARCStats collects ZFS ARC (Adaptive Replacement Cache) statistics
func (c *ZFSCollector) collectARCStats() (dto.ZFSARCStats, error) {
	stats := dto.ZFSARCStats{
		Timestamp: time.Now(),
	}

	// Check if ARC stats file exists
	if _, err := os.Stat(constants.ProcSPLARCStats); os.IsNotExist(err) {
		return stats, fmt.Errorf("ARC stats file not found: %w", err)
	}

	// Read ARC stats file
	file, err := os.Open(constants.ProcSPLARCStats)
	if err != nil {
		return stats, fmt.Errorf("failed to open ARC stats file: %w", err)
	}
	defer file.Close()

	// Parse ARC stats (format: "name type data")
	scanner := bufio.NewScanner(file)
	arcData := make(map[string]uint64)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}

		name := fields[0]
		// type is fields[1], but we don't need it
		value, _ := strconv.ParseUint(fields[2], 10, 64)
		arcData[name] = value
	}

	if err := scanner.Err(); err != nil {
		return stats, fmt.Errorf("error reading ARC stats: %w", err)
	}

	// Extract relevant stats
	stats.SizeBytes = arcData["size"]
	stats.TargetSizeBytes = arcData["c"]
	stats.MinSizeBytes = arcData["c_min"]
	stats.MaxSizeBytes = arcData["c_max"]
	stats.Hits = arcData["hits"]
	stats.Misses = arcData["misses"]

	// Calculate hit ratio
	totalAccesses := stats.Hits + stats.Misses
	if totalAccesses > 0 {
		stats.HitRatioPct = (float64(stats.Hits) / float64(totalAccesses)) * 100.0
	}

	// MRU/MFU hit ratios (if available)
	mruHits := arcData["mru_hits"]
	mfuHits := arcData["mfu_hits"]
	if mruHits > 0 || mfuHits > 0 {
		mruTotal := mruHits + arcData["mru_ghost_hits"]
		mfuTotal := mfuHits + arcData["mfu_ghost_hits"]

		if mruTotal > 0 {
			stats.MRUHitRatioPct = (float64(mruHits) / float64(mruTotal)) * 100.0
		}
		if mfuTotal > 0 {
			stats.MFUHitRatioPct = (float64(mfuHits) / float64(mfuTotal)) * 100.0
		}
	}

	// L2ARC stats (if available)
	stats.L2SizeBytes = arcData["l2_size"]
	stats.L2Hits = arcData["l2_hits"]
	stats.L2Misses = arcData["l2_misses"]

	return stats, nil
}
