package controllers

import (
	"github.com/ruaan-deysel/unraid-management-agent/daemon/constants"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/lib"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
)

// DockerController provides control operations for Docker containers.
// It handles container lifecycle operations including start, stop, restart, pause, and unpause.
type DockerController struct{}

// NewDockerController creates a new Docker controller.
func NewDockerController() *DockerController {
	return &DockerController{}
}

// Start starts a Docker container by ID or name.
func (dc *DockerController) Start(containerID string) error {
	logger.Info("Starting Docker container: %s", containerID)
	_, err := lib.ExecCommand(constants.DockerBin, "start", containerID)
	return err
}

// Stop stops a Docker container by ID or name.
func (dc *DockerController) Stop(containerID string) error {
	logger.Info("Stopping Docker container: %s", containerID)
	_, err := lib.ExecCommand(constants.DockerBin, "stop", containerID)
	return err
}

// Restart restarts a Docker container by ID or name.
func (dc *DockerController) Restart(containerID string) error {
	logger.Info("Restarting Docker container: %s", containerID)
	_, err := lib.ExecCommand(constants.DockerBin, "restart", containerID)
	return err
}

// Pause pauses a running Docker container by ID or name.
func (dc *DockerController) Pause(containerID string) error {
	logger.Info("Pausing Docker container: %s", containerID)
	_, err := lib.ExecCommand(constants.DockerBin, "pause", containerID)
	return err
}

// Unpause resumes a paused Docker container by ID or name.
func (dc *DockerController) Unpause(containerID string) error {
	logger.Info("Unpausing Docker container: %s", containerID)
	_, err := lib.ExecCommand(constants.DockerBin, "unpause", containerID)
	return err
}
