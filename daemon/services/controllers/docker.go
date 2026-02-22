package controllers

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
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

	//nolint:staticcheck // SA1019: Updating to new API in future version
	dockerClient, err := client.NewClientWithOpts(
		client.FromEnv,
		client.WithAPIVersionNegotiation(), //nolint:staticcheck // SA1019
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

// ContainerLogs retrieves stdout/stderr logs from a specific Docker container.
// This is equivalent to `docker logs <container_id>`.
func (dc *DockerController) ContainerLogs(containerRef string, tail int, since string, timestamps bool) (*dto.ContainerLogs, error) {
	logger.Info("Getting logs for Docker container: %s (tail=%d)", containerRef, tail)

	if err := dc.initClient(); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Inspect container to get its name
	inspectResult, err := dc.client.ContainerInspect(ctx, containerRef, client.ContainerInspectOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to inspect container %s: %w", containerRef, err)
	}

	containerInfo := inspectResult.Container
	containerName := strings.TrimPrefix(containerInfo.Name, "/")

	// Build log options
	tailStr := "100"
	if tail > 0 {
		if tail > 5000 {
			tail = 5000
		}
		tailStr = fmt.Sprintf("%d", tail)
	}

	logOptions := client.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Tail:       tailStr,
		Timestamps: timestamps,
	}

	if since != "" {
		logOptions.Since = since
	}

	reader, err := dc.client.ContainerLogs(ctx, containerRef, logOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to get logs for container %s: %w", containerRef, err)
	}
	defer func() { _ = reader.Close() }()

	// Read log output — Docker multiplexes stdout/stderr with 8-byte headers.
	// We read raw bytes and strip the Docker stream headers.
	rawBytes, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read log stream for container %s: %w", containerRef, err)
	}

	// Strip Docker multiplexed stream headers (8 bytes per frame).
	// Each frame: [stream_type(1)][0][0][0][size(4)][payload(size)]
	logContent := stripDockerStreamHeaders(rawBytes)

	lineCount := 0
	for _, b := range logContent {
		if b == '\n' {
			lineCount++
		}
	}

	result := &dto.ContainerLogs{
		ContainerID:   containerInfo.ID[:12],
		ContainerName: containerName,
		Logs:          logContent,
		LineCount:     lineCount,
		Since:         since,
		Timestamp:     time.Now(),
	}

	logger.Debug("Retrieved %d log lines for container %s", lineCount, containerRef)
	return result, nil
}

// stripDockerStreamHeaders removes Docker multiplexed stream headers from raw log output.
// Docker stream format: [type(1)][0(3)][size(4 big-endian)][payload(size)]
func stripDockerStreamHeaders(raw []byte) string {
	var result strings.Builder
	i := 0
	for i < len(raw) {
		// Need at least 8 bytes for header
		if i+8 > len(raw) {
			// Remaining bytes without valid header — append as-is
			result.Write(raw[i:])
			break
		}

		// Check for valid stream type header (0=stdin, 1=stdout, 2=stderr)
		streamType := raw[i]
		if streamType > 2 {
			// Not a multiplexed stream — return the entire content as-is
			return string(raw)
		}

		// Read payload size (big-endian uint32)
		size := int(raw[i+4])<<24 | int(raw[i+5])<<16 | int(raw[i+6])<<8 | int(raw[i+7])
		i += 8

		// Read payload
		end := min(i+size, len(raw))
		result.Write(raw[i:end])
		i = end
	}
	return result.String()
}

// CheckContainerUpdate checks if a specific container has an update available.
// It uses DistributionInspect to compare the local image digest with the registry digest
// without pulling the image, making it significantly faster and bandwidth-free.
func (dc *DockerController) CheckContainerUpdate(containerRef string) (*dto.ContainerUpdateInfo, error) {
	logger.Info("Checking for update: container %s", containerRef)

	if err := dc.initClient(); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Inspect container to get image reference
	inspectResult, err := dc.client.ContainerInspect(ctx, containerRef, client.ContainerInspectOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to inspect container %s: %w", containerRef, err)
	}

	containerInfo := inspectResult.Container
	imageName := containerInfo.Config.Image
	containerName := strings.TrimPrefix(containerInfo.Name, "/")

	// Get local image info to extract current digest from RepoDigests
	imageResult, err := dc.client.ImageInspect(ctx, containerInfo.Image)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect local image for %s: %w", containerName, err)
	}

	// Extract current digest from RepoDigests (format: "repo@sha256:abc...")
	currentDigest := ""
	for _, rd := range imageResult.RepoDigests {
		if idx := strings.LastIndex(rd, "@"); idx != -1 {
			currentDigest = rd[idx+1:]
			break
		}
	}

	// If no RepoDigests (locally built image), fall back to image ID comparison
	if currentDigest == "" {
		currentDigest = imageResult.ID
	}

	// Use DistributionInspect to get the remote registry digest without pulling
	distResult, err := dc.client.DistributionInspect(ctx, imageName, client.DistributionInspectOptions{})
	if err != nil {
		// Fall back: if DistributionInspect fails (e.g., auth required, registry down),
		// report as unable to check rather than failing entirely
		logger.Warning("Docker: DistributionInspect failed for %s, cannot check update: %v", imageName, err)
		return &dto.ContainerUpdateInfo{
			ContainerID:     shortID(containerInfo.ID),
			ContainerName:   containerName,
			Image:           imageName,
			CurrentDigest:   currentDigest,
			UpdateAvailable: false,
			Timestamp:       time.Now(),
		}, nil
	}

	latestDigest := distResult.Descriptor.Digest.String()
	updateAvailable := currentDigest != latestDigest

	logger.Info("Update check for %s: current=%s, latest=%s, update=%v",
		containerName, shortDigest(currentDigest), shortDigest(latestDigest), updateAvailable)

	return &dto.ContainerUpdateInfo{
		ContainerID:     shortID(containerInfo.ID),
		ContainerName:   containerName,
		Image:           imageName,
		CurrentDigest:   currentDigest,
		LatestDigest:    latestDigest,
		UpdateAvailable: updateAvailable,
		Timestamp:       time.Now(),
	}, nil
}

// CheckAllContainerUpdates checks all running containers for available updates.
// Uses concurrent registry checks with a semaphore to avoid overwhelming the registry.
func (dc *DockerController) CheckAllContainerUpdates() (*dto.ContainerUpdatesResult, error) {
	logger.Info("Checking all containers for updates")

	if err := dc.initClient(); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// List all containers
	listResult, err := dc.client.ContainerList(ctx, client.ContainerListOptions{All: true})
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}

	// Check updates concurrently with a semaphore (max 5 concurrent registry calls)
	const maxConcurrency = 5
	sem := make(chan struct{}, maxConcurrency)

	results := make([]dto.ContainerUpdateInfo, len(listResult.Items))
	var mu sync.Mutex
	updatesAvailable := 0

	var wg sync.WaitGroup
	for i, c := range listResult.Items {
		wg.Go(func() {
			// Acquire semaphore
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				return
			}

			name := ""
			if len(c.Names) > 0 {
				name = strings.TrimPrefix(c.Names[0], "/")
			}

			updateInfo, err := dc.CheckContainerUpdate(c.ID)
			if err != nil {
				logger.Warning("Docker: Failed to check update for %s: %v", name, err)
				results[i] = dto.ContainerUpdateInfo{
					ContainerID:     shortID(c.ID),
					ContainerName:   name,
					Image:           c.Image,
					UpdateAvailable: false,
					Timestamp:       time.Now(),
				}
				return
			}

			results[i] = *updateInfo
			if updateInfo.UpdateAvailable {
				mu.Lock()
				updatesAvailable++
				mu.Unlock()
			}
		})
	}

	wg.Wait()

	result := &dto.ContainerUpdatesResult{
		Containers:       results,
		TotalCount:       len(results),
		UpdatesAvailable: updatesAvailable,
		Timestamp:        time.Now(),
	}

	logger.Info("Update check complete: %d containers, %d updates available", result.TotalCount, result.UpdatesAvailable)
	return result, nil
}

// GetContainerSize returns size information for a specific container.
func (dc *DockerController) GetContainerSize(containerRef string) (*dto.ContainerSizeInfo, error) {
	logger.Debug("Getting container size: %s", containerRef)

	if err := dc.initClient(); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Inspect with size option
	inspectResult, err := dc.client.ContainerInspect(ctx, containerRef, client.ContainerInspectOptions{Size: true})
	if err != nil {
		return nil, fmt.Errorf("failed to inspect container %s: %w", containerRef, err)
	}

	containerInfo := inspectResult.Container
	containerName := strings.TrimPrefix(containerInfo.Name, "/")

	// Get image size
	var imageSize int64
	imageResult, err := dc.client.ImageInspect(ctx, containerInfo.Image)
	if err == nil {
		imageSize = imageResult.Size
	}

	var sizeRw, sizeRootFs int64
	if containerInfo.SizeRw != nil {
		sizeRw = *containerInfo.SizeRw
	}
	if containerInfo.SizeRootFs != nil {
		sizeRootFs = *containerInfo.SizeRootFs
	}

	return &dto.ContainerSizeInfo{
		ContainerID:   shortID(containerInfo.ID),
		ContainerName: containerName,
		SizeRw:        sizeRw,
		SizeRootFs:    sizeRootFs,
		ImageSize:     imageSize,
		SizeDisplay:   formatBytes(sizeRootFs),
		Timestamp:     time.Now(),
	}, nil
}

// UpdateContainer updates a specific container by pulling the latest image and recreating it.
func (dc *DockerController) UpdateContainer(containerRef string, force bool) (*dto.ContainerUpdateResult, error) {
	logger.Info("Updating container: %s (force=%v)", containerRef, force)

	if err := dc.initClient(); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// Inspect container
	inspectResult, err := dc.client.ContainerInspect(ctx, containerRef, client.ContainerInspectOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to inspect container: %w", err)
	}

	containerInfo := inspectResult.Container
	imageName := containerInfo.Config.Image
	previousImageID := containerInfo.Image
	containerName := strings.TrimPrefix(containerInfo.Name, "/")
	wasRunning := containerInfo.State != nil && containerInfo.State.Running

	// Pull latest image
	pullResp, err := dc.client.ImagePull(ctx, imageName, client.ImagePullOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to pull image: %w", err)
	}
	defer func() { _ = pullResp.Close() }()
	if _, err := io.Copy(io.Discard, pullResp); err != nil {
		logger.Warning("Docker: Error draining pull response: %v", err)
	}

	// Check if update is available
	imageResult, err := dc.client.ImageInspect(ctx, imageName)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect image: %w", err)
	}

	if !force && previousImageID == imageResult.ID {
		return &dto.ContainerUpdateResult{
			ContainerID:   shortID(containerInfo.ID),
			ContainerName: containerName,
			Image:         imageName,
			Updated:       false,
			Recreated:     false,
			Message:       "Container is already up to date",
			Timestamp:     time.Now(),
		}, nil
	}

	// Stop container if running
	if wasRunning {
		logger.Info("Stopping container %s for update", containerName)
		if _, err := dc.client.ContainerStop(ctx, containerInfo.ID, client.ContainerStopOptions{}); err != nil {
			return nil, fmt.Errorf("failed to stop container: %w", err)
		}
	}

	// Remove old container
	logger.Info("Removing container %s for update", containerName)
	if _, err := dc.client.ContainerRemove(ctx, containerInfo.ID, client.ContainerRemoveOptions{}); err != nil {
		return nil, fmt.Errorf("failed to remove container: %w", err)
	}

	// Reconstruct networking config from the old container
	var networkConfig *network.NetworkingConfig
	if containerInfo.NetworkSettings != nil && containerInfo.NetworkSettings.Networks != nil {
		networkConfig = &network.NetworkingConfig{
			EndpointsConfig: containerInfo.NetworkSettings.Networks,
		}
	}

	// Create new container with same config but new image
	logger.Info("Creating updated container %s", containerName)
	createResult, err := dc.client.ContainerCreate(ctx, client.ContainerCreateOptions{
		Config:           containerInfo.Config,
		HostConfig:       containerInfo.HostConfig,
		NetworkingConfig: networkConfig,
		Name:             containerName,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create container: %w", err)
	}

	// Start new container if the old one was running
	if wasRunning {
		logger.Info("Starting updated container %s", containerName)
		if _, err := dc.client.ContainerStart(ctx, createResult.ID, client.ContainerStartOptions{}); err != nil {
			return nil, fmt.Errorf("failed to start container: %w", err)
		}
	}

	logger.Info("Successfully updated container %s", containerName)
	return &dto.ContainerUpdateResult{
		ContainerID:    shortID(createResult.ID),
		ContainerName:  containerName,
		Image:          imageName,
		PreviousDigest: previousImageID,
		NewDigest:      imageResult.ID,
		Updated:        true,
		Recreated:      true,
		Message:        "Container updated successfully",
		Timestamp:      time.Now(),
	}, nil
}

// UpdateAllContainers updates all containers that have updates available.
func (dc *DockerController) UpdateAllContainers() (*dto.ContainerBulkUpdateResult, error) {
	logger.Info("Updating all containers")

	// First check for updates
	updatesResult, err := dc.CheckAllContainerUpdates()
	if err != nil {
		return nil, fmt.Errorf("failed to check for updates: %w", err)
	}

	result := &dto.ContainerBulkUpdateResult{
		Results:   make([]dto.ContainerUpdateResult, 0),
		Timestamp: time.Now(),
	}

	for _, container := range updatesResult.Containers {
		if !container.UpdateAvailable {
			result.Skipped++
			continue
		}

		updateResult, err := dc.UpdateContainer(container.ContainerID, false)
		if err != nil {
			logger.Error("Docker: Failed to update container %s: %v", container.ContainerName, err)
			result.Results = append(result.Results, dto.ContainerUpdateResult{
				ContainerID:   container.ContainerID,
				ContainerName: container.ContainerName,
				Image:         container.Image,
				Updated:       false,
				Recreated:     false,
				Message:       fmt.Sprintf("Failed to update: %v", err),
				Timestamp:     time.Now(),
			})
			result.Failed++
			continue
		}

		result.Results = append(result.Results, *updateResult)
		if updateResult.Updated {
			result.Succeeded++
		} else {
			result.Skipped++
		}
	}

	logger.Info("Bulk update complete: %d succeeded, %d failed, %d skipped", result.Succeeded, result.Failed, result.Skipped)
	return result, nil
}

// shortID returns the first 12 characters of a Docker ID.
func shortID(id string) string {
	// Strip sha256: prefix if present
	id = strings.TrimPrefix(id, "sha256:")
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

// shortDigest returns a truncated digest for logging.
func shortDigest(digest string) string {
	digest = strings.TrimPrefix(digest, "sha256:")
	if len(digest) > 12 {
		return digest[:12]
	}
	return digest
}

// formatBytes converts bytes to a human-readable string.
func formatBytes(bytes int64) string {
	const (
		kb = 1024
		mb = kb * 1024
		gb = mb * 1024
	)

	switch {
	case bytes >= gb:
		return fmt.Sprintf("%.1f GiB", float64(bytes)/float64(gb))
	case bytes >= mb:
		return fmt.Sprintf("%.1f MiB", float64(bytes)/float64(mb))
	case bytes >= kb:
		return fmt.Sprintf("%.1f KiB", float64(bytes)/float64(kb))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}
