package controllers

import (
	"github.com/ruaan-deysel/unraid-management-agent/daemon/common"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/lib"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
)

type DockerController struct{}

func NewDockerController() *DockerController {
	return &DockerController{}
}

func (dc *DockerController) Start(containerID string) error {
	logger.Info("Starting Docker container: %s", containerID)
	_, err := lib.ExecCommand(common.DockerBin, "start", containerID)
	return err
}

func (dc *DockerController) Stop(containerID string) error {
	logger.Info("Stopping Docker container: %s", containerID)
	_, err := lib.ExecCommand(common.DockerBin, "stop", containerID)
	return err
}

func (dc *DockerController) Restart(containerID string) error {
	logger.Info("Restarting Docker container: %s", containerID)
	_, err := lib.ExecCommand(common.DockerBin, "restart", containerID)
	return err
}

func (dc *DockerController) Pause(containerID string) error {
	logger.Info("Pausing Docker container: %s", containerID)
	_, err := lib.ExecCommand(common.DockerBin, "pause", containerID)
	return err
}

func (dc *DockerController) Unpause(containerID string) error {
	logger.Info("Unpausing Docker container: %s", containerID)
	_, err := lib.ExecCommand(common.DockerBin, "unpause", containerID)
	return err
}
