package controllers

import (
	"github.com/ruaan-deysel/unraid-management-agent/daemon/common"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/lib"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
)

type VMController struct{}

func NewVMController() *VMController {
	return &VMController{}
}

func (vc *VMController) Start(vmName string) error {
	logger.Info("Starting VM: %s", vmName)
	_, err := lib.ExecCommand(common.VirshBin, "start", vmName)
	return err
}

func (vc *VMController) Stop(vmName string) error {
	logger.Info("Stopping VM: %s", vmName)
	_, err := lib.ExecCommand(common.VirshBin, "shutdown", vmName)
	return err
}

func (vc *VMController) Restart(vmName string) error {
	logger.Info("Restarting VM: %s", vmName)
	_, err := lib.ExecCommand(common.VirshBin, "reboot", vmName)
	return err
}

func (vc *VMController) Pause(vmName string) error {
	logger.Info("Pausing VM: %s", vmName)
	_, err := lib.ExecCommand(common.VirshBin, "suspend", vmName)
	return err
}

func (vc *VMController) Resume(vmName string) error {
	logger.Info("Resuming VM: %s", vmName)
	_, err := lib.ExecCommand(common.VirshBin, "resume", vmName)
	return err
}

func (vc *VMController) Hibernate(vmName string) error {
	logger.Info("Hibernating VM: %s", vmName)
	_, err := lib.ExecCommand(common.VirshBin, "managedsave", vmName)
	return err
}

func (vc *VMController) ForceStop(vmName string) error {
	logger.Info("Force stopping VM: %s", vmName)
	_, err := lib.ExecCommand(common.VirshBin, "destroy", vmName)
	return err
}
