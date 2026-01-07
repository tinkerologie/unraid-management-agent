package controllers

import (
	"context"
	"fmt"
	"time"

	"github.com/moby/moby/client"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
)

// DockerController provides control operations for Docker containers using the Docker SDK.
// It handles container lifecycle operations including start, stop, restart, pause, and unpause.
type DockerController struct {
	client *client.Client
}

// NewDockerController creates a new Docker controller with SDK client.
func NewDockerController() *DockerController {
	return &DockerController{}
}

// initClient initializes the Docker client if not already done
func (dc *DockerController) initClient() error {
	if dc.client != nil {
		return nil
	}

	dockerClient, err := client.NewClientWithOpts(
		client.FromEnv,
		client.WithAPIVersionNegotiation(),
	)
	if err != nil {
		return fmt.Errorf("failed to create Docker client: %w", err)
	}

	dc.client = dockerClient
	return nil
}

// Start starts a Docker container by ID or name using the Docker SDK.
func (dc *DockerController) Start(containerID string) error {
	logger.Info("Starting Docker container: %s", containerID)

	if err := dc.initClient(); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if _, err := dc.client.ContainerStart(ctx, containerID, client.ContainerStartOptions{}); err != nil {
		return fmt.Errorf("failed to start container %s: %w", containerID, err)
	}

	logger.Info("Successfully started Docker container: %s", containerID)
	return nil
}

// Stop stops a Docker container by ID or name using the Docker SDK.
func (dc *DockerController) Stop(containerID string) error {
	logger.Info("Stopping Docker container: %s", containerID)

	if err := dc.initClient(); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Use default timeout (container's StopTimeout or 10 seconds)
	if _, err := dc.client.ContainerStop(ctx, containerID, client.ContainerStopOptions{}); err != nil {
		return fmt.Errorf("failed to stop container %s: %w", containerID, err)
	}

	logger.Info("Successfully stopped Docker container: %s", containerID)
	return nil
}

// Restart restarts a Docker container by ID or name using the Docker SDK.
func (dc *DockerController) Restart(containerID string) error {
	logger.Info("Restarting Docker container: %s", containerID)

	if err := dc.initClient(); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Use default timeout for restart
	if _, err := dc.client.ContainerRestart(ctx, containerID, client.ContainerRestartOptions{}); err != nil {
		return fmt.Errorf("failed to restart container %s: %w", containerID, err)
	}

	logger.Info("Successfully restarted Docker container: %s", containerID)
	return nil
}

// Pause pauses a running Docker container by ID or name using the Docker SDK.
func (dc *DockerController) Pause(containerID string) error {
	logger.Info("Pausing Docker container: %s", containerID)

	if err := dc.initClient(); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if _, err := dc.client.ContainerPause(ctx, containerID, client.ContainerPauseOptions{}); err != nil {
		return fmt.Errorf("failed to pause container %s: %w", containerID, err)
	}

	logger.Info("Successfully paused Docker container: %s", containerID)
	return nil
}

// Unpause resumes a paused Docker container by ID or name using the Docker SDK.
func (dc *DockerController) Unpause(containerID string) error {
	logger.Info("Unpausing Docker container: %s", containerID)

	if err := dc.initClient(); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if _, err := dc.client.ContainerUnpause(ctx, containerID, client.ContainerUnpauseOptions{}); err != nil {
		return fmt.Errorf("failed to unpause container %s: %w", containerID, err)
	}

	logger.Info("Successfully unpaused Docker container: %s", containerID)
	return nil
}

// Close closes the Docker client connection.
func (dc *DockerController) Close() error {
	if dc.client != nil {
		return dc.client.Close()
	}
	return nil
}
