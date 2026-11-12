// Package collectors provides data collection services for Unraid system resources.
package collectors

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/tinkerologie/unraid-management-agent/daemon/constants"
	"github.com/tinkerologie/unraid-management-agent/daemon/domain"
	"github.com/tinkerologie/unraid-management-agent/daemon/dto"
	"github.com/tinkerologie/unraid-management-agent/daemon/logger"
	"gopkg.in/ini.v1"
)

// arrayRequiredKeys are the var.ini keys the array collector depends on for
// meaningful output. Their absence indicates a degraded (but not unavailable) source.
var arrayRequiredKeys = []string{"mdState", "mdNumDisks"}

// ArrayCollector collects Unraid array status information including state, parity status, and disk assignments.
// It publishes array status updates to the event bus at regular intervals.
type ArrayCollector struct {
	ctx      *domain.Context
	varIni   string
	disksIni string
	statfs   func(string, *syscall.Statfs_t) error
	capacity arrayCapacity
}

// NewArrayCollector creates a new array status collector with the given context.
func NewArrayCollector(ctx *domain.Context) *ArrayCollector {
	return &ArrayCollector{ctx: ctx, varIni: constants.VarIni, disksIni: constants.DisksIni, statfs: syscall.Statfs}
}

// Collect gathers current array status information and publishes it to the event bus.
// It reads native status files and samples user-share capacity while the array is idle.
func (c *ArrayCollector) Collect() {
	cfg, err := ini.Load(c.varIni)
	if err != nil {
		if c.ctx.Platform != nil {
			c.ctx.Platform.Report("array", dto.SourceUnavailable, "cannot read var.ini", err)
		}
		logger.Error("Array: Failed to load %s: %v", c.varIni, err)
		return
	}
	section := cfg.Section("")
	operation, position := arrayOperation(section)
	if c.ctx.Platform != nil {
		keys := make(map[string]string, len(arrayRequiredKeys))
		for _, key := range arrayRequiredKeys {
			if value, err := section.GetKey(key); err == nil {
				keys[key] = value.String()
			}
		}
		if ok, reason := validateRequiredKeys(keys, arrayRequiredKeys...); !ok {
			c.ctx.Platform.Report("array", dto.SourceDegraded, reason, nil)
		} else if operation == arrayOperationUnknown && strings.EqualFold(keys["mdState"], "STARTED") {
			c.ctx.Platform.Report("array", dto.SourceDegraded, "cannot determine native maintenance state", nil)
		} else {
			c.ctx.Platform.Healthy("array")
		}
	}

	arrayStatus, err := c.collectArrayStatus(section, operation, position)
	if err != nil {
		logger.Error("Array: Failed to collect array status: %v", err)
		return
	}

	// Attach inline source_status flag (nil when healthy — response unchanged).
	if c.ctx.Platform != nil {
		arrayStatus.SourceStatus = c.ctx.Platform.StatusFor("array")
	}

	logger.Debug("Array: Successfully collected, publishing event")
	// Publish event
	domain.Publish(c.ctx.Hub, constants.TopicArrayStatusUpdate, arrayStatus)
	logger.Debug("Array: Published %s event - state=%s, disks=%d", constants.TopicArrayStatusUpdate.Name, arrayStatus.State, arrayStatus.NumDisks)
}

func (c *ArrayCollector) collectArrayStatus(section *ini.Section, operation string, position uint64) (status *dto.ArrayStatus, err error) {
	defer func() {
		if r := recover(); r != nil {
			logger.LogPanicWithStack("Array collector (collect)", r)
			err = fmt.Errorf("panic in collectArrayStatus: %v", r)
			status = nil
		}
	}()

	status = &dto.ArrayStatus{Timestamp: time.Now()}

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
	hasSyncErrors := false
	if section.HasKey("sbSyncErrs") {
		sbSyncErrs := strings.Trim(section.Key("sbSyncErrs").String(), `"`)
		if n, err := strconv.Atoi(sbSyncErrs); err == nil && n > 0 {
			logger.Debug("Array: sbSyncErrs=%d, marking parity as invalid", n)
			hasSyncErrors = true
			parityValid = false
		}
	}

	// Corroboration via parity-checks.log (issue #114): some Unraid versions omit
	// or zero out sbSynced in var.ini even when parity is valid. If the var.ini
	// signals didn't confirm validity but the array is started with parity disks
	// and the most recent parity check/sync completed successfully (exit 0, zero
	// errors), treat parity as valid. Skipped when the superblock reports sync
	// errors so this can never mask a real problem.
	if !parityValid && !hasSyncErrors && status.State == "STARTED" && status.NumParityDisks > 0 {
		if NewParityCollector().LastCheckValid() {
			logger.Debug("Array: parity marked valid via most recent successful parity-check log entry (sbSynced absent/zero)")
			parityValid = true
		}
	}

	// Only mark parity as valid if we have at least one parity disk
	if status.NumParityDisks > 0 {
		status.ParityValid = parityValid
	} else {
		status.ParityValid = false
	}

	// Comprehensive diagnostic logging — captures every signal that feeds the
	// parity validity decision so future false-positive/negative reports
	// (issues #98, #114) have the raw data needed to reproduce.
	logger.Debug("Array: parity decision: parityValid=%v state=%q numParityDisks=%d sbSynced=%q sbSynced2=%q sbSyncErrs=%q sbSyncExit=%q mdNumInvalid=%q mdResync=%q",
		status.ParityValid, status.State, status.NumParityDisks,
		section.Key("sbSynced").String(), section.Key("sbSynced2").String(),
		section.Key("sbSyncErrs").String(), section.Key("sbSyncExit").String(),
		section.Key("mdNumInvalid").String(), section.Key("mdResync").String())

	if operation != arrayOperationUnknown {
		status.ParityCheckStatus = operation
	}
	if size, err := strconv.ParseUint(section.Key("mdResyncSize").String(), 10, 64); operation != "" && size > 0 && err == nil {
		status.ParityCheckProgress = min(100, float64(position)/float64(size)*100)
	}

	c.enrichWithArraySize(status, operation)

	logger.Debug("Array: Parsed status - state=%s, disks=%d, parity=%v, used=%.1f%%",
		status.State, status.NumDisks, status.ParityValid, status.UsedPercent)
	return status, nil
}

type arrayCapacity struct {
	total, free uint64
	sampledAt   time.Time
}

// Reuse the last idle sample during an array operation. The shfs statfs
// request can fan out to the participating disks.
func (c *ArrayCollector) enrichWithArraySize(status *dto.ArrayStatus, operation string) {
	if !status.IsStarted() {
		c.capacity = arrayCapacity{}
		return
	}
	if operation == "" {
		var stat syscall.Statfs_t
		if err := c.statfs("/mnt/user", &stat); err != nil {
			logger.Debug("Array: Failed to get /mnt/user stats: %v", err)
		} else if stat.Bsize > 0 && stat.Blocks <= math.MaxUint64/uint64(stat.Bsize) && stat.Bfree <= stat.Blocks {
			size := uint64(stat.Bsize) // #nosec G115 -- Bsize is checked above.
			c.capacity = arrayCapacity{total: stat.Blocks * size, free: stat.Bfree * size, sampledAt: status.Timestamp}
		}
	}
	if c.capacity.sampledAt.IsZero() {
		return
	}
	status.CapacitySampledAt = new(c.capacity.sampledAt)
	status.TotalBytes, status.FreeBytes = c.capacity.total, c.capacity.free
	if status.TotalBytes > 0 {
		status.UsedPercent = float64(status.TotalBytes-status.FreeBytes) / float64(status.TotalBytes) * 100
	}
}

// countParityDisks counts the number of parity disks from disks.ini
func (c *ArrayCollector) countParityDisks() int {
	// Parse disks.ini to count active parity disks
	cfg, err := ini.Load(c.disksIni)
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
