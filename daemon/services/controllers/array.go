// Package controllers provides control operations for Unraid system resources.
package controllers

import (
	"fmt"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/lib"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
)

// ArrayController provides control operations for the Unraid array.
// It handles array start/stop, parity check operations, and array management commands.
type ArrayController struct {
	ctx *domain.Context
}

// NewArrayController creates a new array controller with the given context.
func NewArrayController(ctx *domain.Context) *ArrayController {
	return &ArrayController{ctx: ctx}
}

// StartArray starts the Unraid array
func (c *ArrayController) StartArray() error {
	logger.Info("Array: Starting array...")

	// Use mdcmd to start the array
	_, err := lib.ExecCommand("/usr/local/sbin/mdcmd", "start")
	if err != nil {
		logger.Error("Array: Failed to start array: %v", err)
		return fmt.Errorf("failed to start array: %w", err)
	}

	logger.Info("Array: Array started successfully")
	return nil
}

// StopArray stops the Unraid array
func (c *ArrayController) StopArray() error {
	logger.Info("Array: Stopping array...")

	// Use mdcmd to stop the array
	_, err := lib.ExecCommand("/usr/local/sbin/mdcmd", "stop")
	if err != nil {
		logger.Error("Array: Failed to stop array: %v", err)
		return fmt.Errorf("failed to stop array: %w", err)
	}

	logger.Info("Array: Array stopped successfully")
	return nil
}

// StartParityCheck starts a parity check
func (c *ArrayController) StartParityCheck(correcting bool) error {
	logger.Info("Array: Starting parity check (correcting: %v)...", correcting)

	var mode string
	if correcting {
		mode = "check CORRECT"
	} else {
		mode = "check NOCORRECT"
	}

	// Use mdcmd to start parity check
	_, err := lib.ExecCommand("/usr/local/sbin/mdcmd", mode)
	if err != nil {
		logger.Error("Array: Failed to start parity check: %v", err)
		return fmt.Errorf("failed to start parity check: %w", err)
	}

	logger.Info("Array: Parity check started successfully")
	return nil
}

// StopParityCheck stops a running parity check
func (c *ArrayController) StopParityCheck() error {
	logger.Info("Array: Stopping parity check...")

	// Use mdcmd to stop parity check
	_, err := lib.ExecCommand("/usr/local/sbin/mdcmd", "nocheck")
	if err != nil {
		logger.Error("Array: Failed to stop parity check: %v", err)
		return fmt.Errorf("failed to stop parity check: %w", err)
	}

	logger.Info("Array: Parity check stopped successfully")
	return nil
}

// PauseParityCheck pauses a running parity check
func (c *ArrayController) PauseParityCheck() error {
	logger.Info("Array: Pausing parity check...")

	// Use mdcmd to pause parity check
	_, err := lib.ExecCommand("/usr/local/sbin/mdcmd", "pause")
	if err != nil {
		logger.Error("Array: Failed to pause parity check: %v", err)
		return fmt.Errorf("failed to pause parity check: %w", err)
	}

	logger.Info("Array: Parity check paused successfully")
	return nil
}

// ResumeParityCheck resumes a paused parity check
func (c *ArrayController) ResumeParityCheck() error {
	logger.Info("Array: Resuming parity check...")

	// Use mdcmd to resume parity check
	_, err := lib.ExecCommand("/usr/local/sbin/mdcmd", "resume")
	if err != nil {
		logger.Error("Array: Failed to resume parity check: %v", err)
		return fmt.Errorf("failed to resume parity check: %w", err)
	}

	logger.Info("Array: Parity check resumed successfully")
	return nil
}

// SpinDownDisk spins down a specific disk
func (c *ArrayController) SpinDownDisk(diskName string) error {
	logger.Info("Array: Spinning down disk %s...", diskName)

	// Use mdcmd to spin down disk
	_, err := lib.ExecCommand("/usr/local/sbin/mdcmd", "spindown", diskName)
	if err != nil {
		logger.Error("Array: Failed to spin down disk %s: %v", diskName, err)
		return fmt.Errorf("failed to spin down disk: %w", err)
	}

	logger.Info("Array: Disk %s spun down successfully", diskName)
	return nil
}

// SpinUpDisk spins up a specific disk
func (c *ArrayController) SpinUpDisk(diskName string) error {
	logger.Info("Array: Spinning up disk %s...", diskName)

	// Use mdcmd to spin up disk
	_, err := lib.ExecCommand("/usr/local/sbin/mdcmd", "spinup", diskName)
	if err != nil {
		logger.Error("Array: Failed to spin up disk %s: %v", diskName, err)
		return fmt.Errorf("failed to spin up disk: %w", err)
	}

	logger.Info("Array: Disk %s spun up successfully", diskName)
	return nil
}
