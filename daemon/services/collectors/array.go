// Package collectors provides data collection services for Unraid system resources.
package collectors

import (
	"context"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/constants"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
	"gopkg.in/ini.v1"
)

// watchedArrayFiles are the INI files the array collector monitors for changes.
var watchedArrayFiles = []string{constants.VarIni, constants.DisksIni}

// ArrayCollector collects Unraid array status information including state, parity status, and disk assignments.
// It publishes array status updates to the event bus at regular intervals.
type ArrayCollector struct {
	ctx *domain.Context
}

// NewArrayCollector creates a new array status collector with the given context.
func NewArrayCollector(ctx *domain.Context) *ArrayCollector {
	return &ArrayCollector{ctx: ctx}
}

// Start begins the array collector's periodic data collection.
// It runs in a goroutine and publishes array status updates at the specified interval until the context is cancelled.
func (c *ArrayCollector) Start(ctx context.Context, interval time.Duration) {
	logger.Info("Starting array collector (interval: %v)", interval)

	// Run once immediately with panic recovery
	func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("Array collector PANIC on startup: %v", r)
			}
		}()
		c.Collect()
	}()

	// Set up fsnotify watcher for instant state updates on INI file changes
	fw, err := NewFileWatcher(500 * time.Millisecond)
	if err != nil {
		logger.Warning("Array collector: failed to create file watcher, using ticker only: %v", err)
	} else {
		for _, f := range watchedArrayFiles {
			if watchErr := fw.WatchFile(f); watchErr != nil {
				logger.Warning("Array collector: failed to watch %s: %v", f, watchErr)
			}
		}
		// Run watcher in background goroutine — triggers Collect on file changes
		// Close is deferred inside the goroutine to avoid racing with fw.Run()
		go func() {
			defer func() { _ = fw.Close() }()
			fw.Run(ctx, watchedArrayFiles, func() {
				func() {
					defer func() {
						if r := recover(); r != nil {
							logger.Error("Array collector PANIC on fsnotify: %v", r)
						}
					}()
					logger.Debug("Array collector: INI file changed, collecting immediately")
					c.Collect()
				}()
			})
		}()
		logger.Info("Array collector: fsnotify watching %v for instant updates", watchedArrayFiles)
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info("Array collector stopping due to context cancellation")
			return
		case <-ticker.C:
			func() {
				defer func() {
					if r := recover(); r != nil {
						logger.Error("Array collector PANIC in loop: %v", r)
					}
				}()
				c.Collect()
			}()
		}
	}
}

// Collect gathers current array status information and publishes it to the event bus.
// It reads array state from Unraid's mdcmd command and var.ini configuration file.
func (c *ArrayCollector) Collect() {
	logger.Debug("Collecting array data...")
	logger.Debug("TRACE: About to call collectArrayStatus()")

	// Collect array status
	arrayStatus, err := c.collectArrayStatus()
	logger.Debug("TRACE: Returned from collectArrayStatus, err=%v", err)
	if err != nil {
		logger.Error("Array: Failed to collect array status: %v", err)
		return
	}

	logger.Debug("Array: Successfully collected, publishing event")
	// Publish event
	domain.Publish(c.ctx.Hub, constants.TopicArrayStatusUpdate, arrayStatus)
	logger.Debug("Array: Published %s event - state=%s, disks=%d", constants.TopicArrayStatusUpdate.Name, arrayStatus.State, arrayStatus.NumDisks)
}

func (c *ArrayCollector) collectArrayStatus() (*dto.ArrayStatus, error) {
	defer func() {
		if r := recover(); r != nil {
			logger.Error("Array: PANIC during collection: %v", r)
		}
	}()

	logger.Debug("Array: Starting collection from %s", constants.VarIni)
	status := &dto.ArrayStatus{
		Timestamp: time.Now(),
	}

	// Parse var.ini for array information
	cfg, err := ini.Load(constants.VarIni)
	if err != nil {
		logger.Error("Array: Failed to load file: %v", err)
		return nil, err
	}
	logger.Debug("Array: File loaded successfully")

	// Get the default section (unnamed section)
	section := cfg.Section("")

	// Array state
	if section.HasKey("mdState") {
		status.State = strings.Trim(section.Key("mdState").String(), `"`)
	} else {
		status.State = "unknown"
	}

	// Number of disks
	if section.HasKey("mdNumDisks") {
		numDisks := strings.Trim(section.Key("mdNumDisks").String(), `"`)
		logger.Debug("Array: Found mdNumDisks=%s", numDisks)
		if n, err := strconv.Atoi(numDisks); err == nil {
			status.NumDisks = n
			logger.Debug("Array: Parsed mdNumDisks=%d", n)
		} else {
			logger.Error("Array: Failed to parse mdNumDisks: %v", err)
		}
	} else {
		logger.Warning("Array: mdNumDisks not found in file")
	}

	// Count parity disks from disks.ini
	status.NumParityDisks = c.countParityDisks()

	// Calculate data disks: total disks minus parity disks
	// mdNumDisks includes all array disks (data + parity), excluding cache/flash
	status.NumDataDisks = status.NumDisks - status.NumParityDisks
	logger.Debug("Array: Calculated NumDataDisks=%d (total=%d - parity=%d)",
		status.NumDataDisks, status.NumDisks, status.NumParityDisks)

	// Parity validity — determine whether the array's parity data is intact.
	// Primary signal: sbSynced holds a Unix timestamp of the last successful parity sync;
	// a non-zero value means parity was synced at least once.
	// Fallback: mdNumInvalid counts disabled/missing disks in the array; if it is 0 and
	// parity disks exist, the array is in a healthy state even when sbSynced is absent
	// or zero (e.g. freshly cleared parity, or Unraid versions that omit the field).
	parityValid := false
	if section.HasKey("sbSynced") {
		sbSynced := strings.Trim(section.Key("sbSynced").String(), `"`)
		logger.Debug("Array: sbSynced=%q", sbSynced)
		if sbSynced != "0" && sbSynced != "" {
			parityValid = true
		}
	}

	// Fallback: if sbSynced is missing or zero, check mdNumInvalid.
	// A started array with parity disks and zero invalid disks has valid parity.
	if !parityValid && section.HasKey("mdNumInvalid") {
		mdNumInvalid := strings.Trim(section.Key("mdNumInvalid").String(), `"`)
		logger.Debug("Array: mdNumInvalid=%q (fallback parity check)", mdNumInvalid)
		if n, err := strconv.Atoi(mdNumInvalid); err == nil && n == 0 && status.NumParityDisks > 0 {
			parityValid = true
		}
	}

	// Parity errors override: any sync errors invalidate parity
	if section.HasKey("sbSyncErrs") {
		sbSyncErrs := strings.Trim(section.Key("sbSyncErrs").String(), `"`)
		if n, err := strconv.Atoi(sbSyncErrs); err == nil && n > 0 {
			logger.Debug("Array: sbSyncErrs=%d, marking parity as invalid", n)
			parityValid = false
		}
	}

	// Only mark parity as valid if we have at least one parity disk
	if status.NumParityDisks > 0 {
		status.ParityValid = parityValid
	} else {
		status.ParityValid = false
	}
	logger.Debug("Array: parityValid=%v (numParityDisks=%d)", status.ParityValid, status.NumParityDisks)

	// Parity check status - need to check multiple fields to detect state properly
	// Key fields:
	// - mdResyncPos: Current position in parity operation (>0 means operation in progress)
	// - mdResyncDt: Delta time (0 = paused, >0 = running)
	// - mdResyncSize: Total size for calculating progress
	// - sbSyncAction: Type of parity operation (e.g., "check P", "check NOCORRECT")
	var mdResyncPos, mdResyncSize uint64
	var mdResyncDt int64

	if section.HasKey("mdResyncPos") {
		posStr := strings.Trim(section.Key("mdResyncPos").String(), `"`)
		if pos, err := strconv.ParseUint(posStr, 10, 64); err == nil {
			mdResyncPos = pos
		}
	}

	if section.HasKey("mdResyncSize") {
		sizeStr := strings.Trim(section.Key("mdResyncSize").String(), `"`)
		if size, err := strconv.ParseUint(sizeStr, 10, 64); err == nil {
			mdResyncSize = size
		}
	}

	if section.HasKey("mdResyncDt") {
		dtStr := strings.Trim(section.Key("mdResyncDt").String(), `"`)
		if dt, err := strconv.ParseInt(dtStr, 10, 64); err == nil {
			mdResyncDt = dt
		}
	}

	// Determine parity check status based on mdResyncPos and mdResyncDt
	// - mdResyncPos > 0 AND mdResyncDt = 0 → PAUSED
	// - mdResyncPos > 0 AND mdResyncDt > 0 → RUNNING (check, correct, etc.)
	// - mdResyncPos = 0 → IDLE (no active operation)
	if mdResyncPos > 0 {
		// There is an active parity operation
		if mdResyncDt == 0 {
			// Operation is paused
			status.ParityCheckStatus = "paused"
		} else {
			// Operation is running - get the action type
			if section.HasKey("sbSyncAction") {
				action := strings.Trim(section.Key("sbSyncAction").String(), `"`)
				// Map common action values to user-friendly status
				switch {
				case strings.Contains(strings.ToLower(action), "check"):
					status.ParityCheckStatus = "running"
				case strings.Contains(strings.ToLower(action), "clear"):
					status.ParityCheckStatus = "clearing"
				case strings.Contains(strings.ToLower(action), "recon"):
					status.ParityCheckStatus = "reconstructing"
				default:
					status.ParityCheckStatus = "running"
				}
			} else {
				status.ParityCheckStatus = "running"
			}
		}

		// Calculate progress percentage
		if mdResyncSize > 0 {
			status.ParityCheckProgress = float64(mdResyncPos) / float64(mdResyncSize) * 100.0
			// Clamp to 0-100 range
			if status.ParityCheckProgress > 100 {
				status.ParityCheckProgress = 100
			}
		}

		logger.Debug("Array: Parity operation detected - pos=%d, size=%d, dt=%d, status=%s, progress=%.2f%%",
			mdResyncPos, mdResyncSize, mdResyncDt, status.ParityCheckStatus, status.ParityCheckProgress)
	} else {
		// No active parity operation
		status.ParityCheckStatus = ""
		status.ParityCheckProgress = 0
	}

	// Get array size information from /mnt/user filesystem
	// /mnt/user is the shfs (Unraid user share filesystem) that represents the entire array
	c.enrichWithArraySize(status)

	logger.Debug("Array: Parsed status - state=%s, disks=%d, parity=%v, used=%.1f%%",
		status.State, status.NumDisks, status.ParityValid, status.UsedPercent)
	return status, nil
}

// enrichWithArraySize gets total array size and usage from /mnt/user
func (c *ArrayCollector) enrichWithArraySize(status *dto.ArrayStatus) {
	// Use syscall.Statfs to get filesystem statistics for /mnt/user
	var stat syscall.Statfs_t
	if err := syscall.Statfs("/mnt/user", &stat); err != nil {
		logger.Debug("Array: Failed to get /mnt/user stats: %v", err)
		return
	}

	// Calculate sizes in bytes (safe conversion - Bsize is always positive)
	//nolint:gosec // G115: Bsize is always positive on Linux systems
	bsize := uint64(stat.Bsize)
	totalBytes := stat.Blocks * bsize
	freeBytes := stat.Bfree * bsize
	usedBytes := totalBytes - freeBytes

	status.TotalBytes = totalBytes
	status.FreeBytes = freeBytes

	// Calculate usage percentage
	if totalBytes > 0 {
		status.UsedPercent = float64(usedBytes) / float64(totalBytes) * 100
	}

	logger.Debug("Array: Size - total=%d bytes (%.2f TB), used=%.1f%%",
		totalBytes, float64(totalBytes)/(1024*1024*1024*1024), status.UsedPercent)
}

// countParityDisks counts the number of parity disks from disks.ini
func (c *ArrayCollector) countParityDisks() int {
	// Parse disks.ini to count active parity disks
	cfg, err := ini.Load(constants.DisksIni)
	if err != nil {
		logger.Debug("Array: Failed to load disks.ini: %v", err)
		return 0
	}

	parityCount := 0
	// Iterate through all sections in disks.ini
	for _, section := range cfg.Sections() {
		// Check if this section has type="Parity" and is active
		if section.HasKey("type") && section.HasKey("status") {
			diskType := strings.Trim(section.Key("type").String(), `"`)
			diskStatus := strings.Trim(section.Key("status").String(), `"`)

			// Only count parity disks that are active (not disabled)
			// DISK_NP_DSBL = Not Present/Disabled, DISK_NP = Not Present, DISK_DSBL = Disabled
			if diskType == "Parity" && diskStatus != "DISK_NP_DSBL" && diskStatus != "DISK_NP" && diskStatus != "DISK_DSBL" {
				parityCount++
				logger.Debug("Array: Found active parity disk in section [%s] with status=%s", section.Name(), diskStatus)
			} else if diskType == "Parity" {
				logger.Debug("Array: Skipping disabled/missing parity disk in section [%s] with status=%s", section.Name(), diskStatus)
			}
		}
	}

	logger.Debug("Array: Counted %d active parity disk(s) from disks.ini", parityCount)
	return parityCount
}
