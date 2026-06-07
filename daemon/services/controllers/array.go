// Package controllers provides control operations for Unraid system resources.
package controllers

import (
	"fmt"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/constants"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/lib"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
)

// mdcmdExec writes a command to /proc/mdcmd directly for zero shell overhead.
// Falls back to the mdcmd binary via ExecCommand if /proc/mdcmd is unavailable.
func mdcmdExec(args ...string) error {
	if lib.IsProcMdcmdAvailable() {
		return lib.MdcmdWrite(args...)
	}
	// Capability gate: if neither /proc/mdcmd nor the mdcmd binary is present,
	// return a clear "unavailable" error instead of a cryptic exec failure.
	if err := requireBinary("array", constants.MdcmdBin); err != nil {
		return err
	}
	logger.Debug("Array: /proc/mdcmd not available, falling back to mdcmd binary")
	_, err := lib.ExecCommand(constants.MdcmdBin, args...)
	return err
}

// ArrayController provides control operations for the Unraid array.
// It handles array start/stop, parity check operations, and array management commands.
type ArrayController struct {
	ctx *domain.Context
}

// NewArrayController creates a new array controller with the given context.
func NewArrayController(ctx *domain.Context) *ArrayController {
	return &ArrayController{ctx: ctx}
}

// StartArray starts the Unraid array.
// Uses direct /proc/mdcmd write for zero shell overhead with fallback to mdcmd binary.
func (c *ArrayController) StartArray() error {
	logger.Info("Array: Starting array...")

	if err := mdcmdExec("start"); err != nil {
		logger.Error("Array: Failed to start array: %v", err)
		return fmt.Errorf("failed to start array: %w", err)
	}

	logger.Info("Array: Array started successfully")
	return nil
}

// StopArray stops the Unraid array.
// Uses direct /proc/mdcmd write for zero shell overhead with fallback to mdcmd binary.
func (c *ArrayController) StopArray() error {
	logger.Info("Array: Stopping array...")

	if err := mdcmdExec("stop"); err != nil {
		logger.Error("Array: Failed to stop array: %v", err)
		return fmt.Errorf("failed to stop array: %w", err)
	}

	logger.Info("Array: Array stopped successfully")
	return nil
}

// StartParityCheck starts a parity check.
// Uses direct /proc/mdcmd write for zero shell overhead with fallback to mdcmd binary.
func (c *ArrayController) StartParityCheck(correcting bool) error {
	logger.Info("Array: Starting parity check (correcting: %v)...", correcting)

	var err error
	if correcting {
		err = mdcmdExec("check", "CORRECT")
	} else {
		err = mdcmdExec("check", "NOCORRECT")
	}
	if err != nil {
		logger.Error("Array: Failed to start parity check: %v", err)
		return fmt.Errorf("failed to start parity check: %w", err)
	}

	logger.Info("Array: Parity check started successfully")
	return nil
}

// StopParityCheck stops a running parity check.
// Uses direct /proc/mdcmd write for zero shell overhead with fallback to mdcmd binary.
func (c *ArrayController) StopParityCheck() error {
	logger.Info("Array: Stopping parity check...")

	if err := mdcmdExec("nocheck"); err != nil {
		logger.Error("Array: Failed to stop parity check: %v", err)
		return fmt.Errorf("failed to stop parity check: %w", err)
	}

	logger.Info("Array: Parity check stopped successfully")
	return nil
}

// PauseParityCheck pauses a running parity check.
// Uses direct /proc/mdcmd write for zero shell overhead with fallback to mdcmd binary.
func (c *ArrayController) PauseParityCheck() error {
	logger.Info("Array: Pausing parity check...")

	if err := mdcmdExec("pause"); err != nil {
		logger.Error("Array: Failed to pause parity check: %v", err)
		return fmt.Errorf("failed to pause parity check: %w", err)
	}

	logger.Info("Array: Parity check paused successfully")
	return nil
}

// ResumeParityCheck resumes a paused parity check.
// Uses direct /proc/mdcmd write for zero shell overhead with fallback to mdcmd binary.
func (c *ArrayController) ResumeParityCheck() error {
	logger.Info("Array: Resuming parity check...")

	if err := mdcmdExec("resume"); err != nil {
		logger.Error("Array: Failed to resume parity check: %v", err)
		return fmt.Errorf("failed to resume parity check: %w", err)
	}

	logger.Info("Array: Parity check resumed successfully")
	return nil
}

// SpinDownDisk spins down a specific disk.
// Uses direct /proc/mdcmd write for zero shell overhead with fallback to mdcmd binary.
func (c *ArrayController) SpinDownDisk(diskName string) error {
	logger.Info("Array: Spinning down disk %s...", diskName)

	if err := mdcmdExec("spindown", diskName); err != nil {
		logger.Error("Array: Failed to spin down disk %s: %v", diskName, err)
		return fmt.Errorf("failed to spin down disk: %w", err)
	}

	logger.Info("Array: Disk %s spun down successfully", diskName)
	return nil
}

// SpinUpDisk spins up a specific disk.
// Uses direct /proc/mdcmd write for zero shell overhead with fallback to mdcmd binary.
func (c *ArrayController) SpinUpDisk(diskName string) error {
	logger.Info("Array: Spinning up disk %s...", diskName)

	if err := mdcmdExec("spinup", diskName); err != nil {
		logger.Error("Array: Failed to spin up disk %s: %v", diskName, err)
		return fmt.Errorf("failed to spin up disk: %w", err)
	}

	logger.Info("Array: Disk %s spun up successfully", diskName)
	return nil
}

// ClearDiskStats clears all array disk I/O statistics system-wide.
//
// Mechanism: issues "clearStatistics=true" via the emhttpd Unix socket — the same
// call the Unraid WebUI makes when the user clicks "Clear Stats" on the Main page
// (ArrayOperation.page → ToggleState.php → emcmd clearStatistics=true).  The
// operation is safe and reversible: statistics simply reset to zero and accumulate
// again as normal.
//
// Note: the operation clears ALL disk statistics globally; there is no per-disk
// variant exposed by the emhttpd API.
//
// Capability gate: requires the emhttpd socket (/var/run/emhttpd.socket).
func (c *ArrayController) ClearDiskStats() error {
	logger.Info("Array: Clearing disk statistics...")

	if err := requireBinary("array", constants.EmcmdBin); err != nil {
		return err
	}

	if err := lib.EmhttpdRequest(map[string]string{"clearStatistics": "true"}); err != nil {
		logger.Error("Array: Failed to clear disk statistics: %v", err)
		return fmt.Errorf("failed to clear disk statistics: %w", err)
	}

	logger.Info("Array: Disk statistics cleared successfully")
	return nil
}
