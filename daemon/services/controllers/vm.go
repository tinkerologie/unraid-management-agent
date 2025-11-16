package controllers

import (
	"github.com/ruaan-deysel/unraid-management-agent/daemon/common"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/lib"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
)

// VMController provides control operations for virtual machines managed by libvirt.
// It handles VM lifecycle operations including start, stop, restart, pause, resume, hibernate, and force stop.
type VMController struct{}

// NewVMController creates a new VM controller.
func NewVMController() *VMController {
	return &VMController{}
}

// Start starts a virtual machine by name.
func (vc *VMController) Start(vmName string) error {
	logger.Info("Starting VM: %s", vmName)
	_, err := lib.ExecCommand(common.VirshBin, "start", vmName)
	return err
}

// Stop gracefully shuts down a virtual machine by name.
func (vc *VMController) Stop(vmName string) error {
	logger.Info("Stopping VM: %s", vmName)
	_, err := lib.ExecCommand(common.VirshBin, "shutdown", vmName)
	return err
}

// Restart reboots a virtual machine by name.
func (vc *VMController) Restart(vmName string) error {
	logger.Info("Restarting VM: %s", vmName)
	_, err := lib.ExecCommand(common.VirshBin, "reboot", vmName)
	return err
}

// Pause suspends a running virtual machine by name.
func (vc *VMController) Pause(vmName string) error {
	logger.Info("Pausing VM: %s", vmName)
	_, err := lib.ExecCommand(common.VirshBin, "suspend", vmName)
	return err
}

// Resume resumes a paused virtual machine by name.
func (vc *VMController) Resume(vmName string) error {
	logger.Info("Resuming VM: %s", vmName)
	_, err := lib.ExecCommand(common.VirshBin, "resume", vmName)
	return err
}

// Hibernate saves the VM state to disk and stops it.
func (vc *VMController) Hibernate(vmName string) error {
	logger.Info("Hibernating VM: %s", vmName)
	_, err := lib.ExecCommand(common.VirshBin, "managedsave", vmName)
	return err
}

// ForceStop immediately terminates a virtual machine by name without graceful shutdown.
func (vc *VMController) ForceStop(vmName string) error {
	logger.Info("Force stopping VM: %s", vmName)
	_, err := lib.ExecCommand(common.VirshBin, "destroy", vmName)
	return err
}
