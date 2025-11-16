// Package collectors provides data collection services for Unraid system resources.
package collectors

import (
	"context"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/common"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
	"github.com/vaughan0/go-ini"
)

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
	c.ctx.Hub.Pub(arrayStatus, "array_status_update")
	logger.Debug("Array: Published array_status_update event - state=%s, disks=%d", arrayStatus.State, arrayStatus.NumDisks)
}

func (c *ArrayCollector) collectArrayStatus() (*dto.ArrayStatus, error) {
	defer func() {
		if r := recover(); r != nil {
			logger.Error("Array: PANIC during collection: %v", r)
		}
	}()

	logger.Debug("Array: Starting collection from %s", common.VarIni)
	status := &dto.ArrayStatus{
		Timestamp: time.Now(),
	}

	// Parse var.ini for array information
	file, err := ini.LoadFile(common.VarIni)
	if err != nil {
		logger.Error("Array: Failed to load file: %v", err)
		return nil, err
	}
	logger.Debug("Array: File loaded successfully")

	// Array state
	if mdState, ok := file.Get("", "mdState"); ok {
		status.State = strings.Trim(mdState, `"`)
	} else {
		status.State = "unknown"
	}

	// Number of disks
	if numDisks, ok := file.Get("", "mdNumDisks"); ok {
		numDisks = strings.Trim(numDisks, `"`)
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

	if numData, ok := file.Get("", "mdNumDisabled"); ok {
		numData = strings.Trim(numData, `"`)
		if n, err := strconv.Atoi(numData); err == nil {
			status.NumDataDisks = n
		}
	}

	// Count parity disks from disks.ini (mdNumParity doesn't exist in var.ini)
	status.NumParityDisks = c.countParityDisks()

	// Parity validity - check if parity sync has completed and has no errors
	// sbSynced contains a timestamp when parity was last synced, or "0" if never synced
	// sbSyncErrs contains the number of errors from the last parity check
	parityValid := false
	if sbSynced, ok := file.Get("", "sbSynced"); ok {
		sbSynced = strings.Trim(sbSynced, `"`)
		// If sbSynced is a non-zero number (timestamp), parity has been synced
		if sbSynced != "0" && sbSynced != "" {
			parityValid = true
		}
	}

	// Check for parity errors - if there are any errors, parity is not valid
	if sbSyncErrs, ok := file.Get("", "sbSyncErrs"); ok {
		sbSyncErrs = strings.Trim(sbSyncErrs, `"`)
		if n, err := strconv.Atoi(sbSyncErrs); err == nil && n > 0 {
			parityValid = false
		}
	}

	// Only mark parity as valid if we have at least one parity disk
	if status.NumParityDisks > 0 {
		status.ParityValid = parityValid
	} else {
		status.ParityValid = false
	}

	// Parity check status
	if sbSyncAction, ok := file.Get("", "sbSyncAction"); ok {
		status.ParityCheckStatus = strings.Trim(sbSyncAction, `"`)
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

	// Calculate sizes in bytes
	totalBytes := stat.Blocks * uint64(stat.Bsize)
	freeBytes := stat.Bfree * uint64(stat.Bsize)
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
	// Parse disks.ini to count parity disks
	file, err := ini.LoadFile(common.DisksIni)
	if err != nil {
		logger.Debug("Array: Failed to load disks.ini: %v", err)
		return 0
	}

	parityCount := 0
	// Iterate through all sections in disks.ini
	for sectionName := range file {
		// Check if this section has type="Parity"
		if diskType, ok := file.Get(sectionName, "type"); ok {
			diskType = strings.Trim(diskType, `"`)
			if diskType == "Parity" {
				parityCount++
			}
		}
	}

	logger.Debug("Array: Counted %d parity disk(s) from disks.ini", parityCount)
	return parityCount
}
