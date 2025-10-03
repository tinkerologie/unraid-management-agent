package collectors

import (
	"strconv"
	"strings"
	"time"

	"github.com/ruaandeysel/unraid-management-agent/daemon/common"
	"github.com/ruaandeysel/unraid-management-agent/daemon/domain"
	"github.com/ruaandeysel/unraid-management-agent/daemon/dto"
	"github.com/ruaandeysel/unraid-management-agent/daemon/logger"
	"github.com/vaughan0/go-ini"
)

type ArrayCollector struct {
	ctx *domain.Context
}

func NewArrayCollector(ctx *domain.Context) *ArrayCollector {
	return &ArrayCollector{ctx: ctx}
}

func (c *ArrayCollector) Start(interval time.Duration) {
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

	for range ticker.C {
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

	if numParity, ok := file.Get("", "mdNumParity"); ok {
		numParity = strings.Trim(numParity, `"`)
		if n, err := strconv.Atoi(numParity); err == nil {
			status.NumParityDisks = n
		}
	}

	// Parity status
	if sbSynced, ok := file.Get("", "sbSynced"); ok {
		sbSynced = strings.Trim(sbSynced, `"`)
		status.ParityValid = (sbSynced == "yes" || sbSynced == "1")
	}

	if sbSyncErrs, ok := file.Get("", "sbSyncErrs"); ok {
		sbSyncErrs = strings.Trim(sbSyncErrs, `"`)
		if n, err := strconv.Atoi(sbSyncErrs); err == nil && n == 0 {
			status.ParityValid = status.ParityValid && true
		} else {
			status.ParityValid = false
		}
	}

	// Parity check status
	if sbSyncAction, ok := file.Get("", "sbSyncAction"); ok {
		status.ParityCheckStatus = strings.Trim(sbSyncAction, `"`)
	}

	// Array size information
	if mdNumDisks, ok := file.Get("", "mdNumDisks"); ok {
		// This would need mdcmd status or reading from disks
		// For now, just parse what we have
		_ = mdNumDisks
	}

	logger.Debug("Array: Parsed status - state=%s, disks=%d, parity=%v", status.State, status.NumDisks, status.ParityValid)
	return status, nil
}
