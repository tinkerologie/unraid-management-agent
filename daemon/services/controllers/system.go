// Package controllers provides control operations for Unraid system resources.
package controllers

import (
	"fmt"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/lib"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
)

// SystemController provides control operations for the Unraid system.
// It handles system reboot and shutdown operations.
type SystemController struct {
	ctx  *domain.Context
	exec func(string, ...string) ([]string, error)
}

// NewSystemController creates a new system controller with the given context.
func NewSystemController(ctx *domain.Context) *SystemController {
	return &SystemController{
		ctx:  ctx,
		exec: lib.ExecCommand,
	}
}

// NewSystemControllerWithExec creates a new system controller with a custom command executor.
func NewSystemControllerWithExec(ctx *domain.Context, execFn func(string, ...string) ([]string, error)) *SystemController {
	return &SystemController{
		ctx:  ctx,
		exec: execFn,
	}
}

// SetExec sets a custom command executor for the system controller.
func (c *SystemController) SetExec(execFn func(string, ...string) ([]string, error)) {
	c.exec = execFn
}

// Reboot initiates a system reboot.
// This will gracefully stop services and reboot the Unraid server.
func (c *SystemController) Reboot() error {
	logger.Info("System: Initiating reboot...")

	execFn := c.exec
	if execFn == nil {
		execFn = lib.ExecCommand
	}

	// Use the shutdown command with -r flag for reboot
	// The command runs in background so we can return a response before reboot occurs
	_, err := execFn("/sbin/shutdown", "-r", "now")
	if err != nil {
		logger.Error("System: Failed to initiate reboot: %v", err)
		return fmt.Errorf("failed to initiate reboot: %w", err)
	}

	logger.Info("System: Reboot initiated successfully")
	return nil
}

// Shutdown initiates a system shutdown.
// This will gracefully stop services and power off the Unraid server.
func (c *SystemController) Shutdown() error {
	logger.Info("System: Initiating shutdown...")

	execFn := c.exec
	if execFn == nil {
		execFn = lib.ExecCommand
	}

	// Use the shutdown command with -h flag for halt/poweroff
	// The command runs in background so we can return a response before shutdown occurs
	_, err := execFn("/sbin/shutdown", "-h", "now")
	if err != nil {
		logger.Error("System: Failed to initiate shutdown: %v", err)
		return fmt.Errorf("failed to initiate shutdown: %w", err)
	}

	logger.Info("System: Shutdown initiated successfully")
	return nil
}
