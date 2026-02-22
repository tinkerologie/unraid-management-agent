package collectors

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
)

// DockerCollector collects Docker container information using the Docker SDK.
// This is significantly faster than CLI commands as it avoids process spawning.
type DockerCollector struct {
	appCtx       *domain.Context
	dockerClient *client.Client
	initialized  bool
}

// NewDockerCollector creates a new Docker SDK-based collector
func NewDockerCollector(ctx *domain.Context) *DockerCollector {
	return &DockerCollector{
		appCtx:      ctx,
		initialized: false,
	}
}

// initClient initializes the Docker client if not already done
func (c *DockerCollector) initClient() error {
	if c.dockerClient != nil {
		return nil
	}

	//nolint:staticcheck // SA1019: Updating to new API in future version
	dockerClient, err := client.NewClientWithOpts(
		client.FromEnv,
		client.WithAPIVersionNegotiation(), //nolint:staticcheck // SA1019
	)
	if err != nil {
		return err
	}

	c.dockerClient = dockerClient
	c.initialized = true
	return nil
}

// Start begins the Docker collector's periodic data collection
func (c *DockerCollector) Start(ctx context.Context, interval time.Duration) {
	logger.Info("Starting docker collector (interval: %v)", interval)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	defer func() {
		if c.dockerClient != nil {
			if err := c.dockerClient.Close(); err != nil {
				logger.Debug("Docker: Error closing client: %v", err)
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			logger.Info("Docker collector stopping due to context cancellation")
			return
		case <-ticker.C:
			c.Collect()
		}
	}
}

// Collect gathers Docker container information using the SDK and publishes to event bus
func (c *DockerCollector) Collect() {
	startTotal := time.Now()
	logger.Debug("Collecting docker data via SDK...")

	// Initialize client if needed
	if err := c.initClient(); err != nil {
		logger.Debug("Failed to initialize Docker client: %v (Docker may not be running)", err)
		// Publish empty list
		c.appCtx.Hub.Pub([]*dto.ContainerInfo{}, "container_list_update")
		return
	}

	ctx := context.Background()

	// List all containers (including stopped) - SDK is much faster than CLI
	startList := time.Now()
	result, err := c.dockerClient.ContainerList(ctx, client.ContainerListOptions{All: true})
	if err != nil {
		logger.Debug("Failed to list containers via SDK: %v", err)
		c.appCtx.Hub.Pub([]*dto.ContainerInfo{}, "container_list_update")
		return
	}
	apiContainers := result.Items
	logger.Debug("Docker SDK: ContainerList took %v for %d containers", time.Since(startList), len(apiContainers))

	containers := make([]*dto.ContainerInfo, 0, len(apiContainers))
	runningContainers := make([]container.Summary, 0)

	// First pass: create basic container info
	for _, apiContainer := range apiContainers {
		shortID := apiContainer.ID[:12]
		state := strings.ToLower(string(apiContainer.State))

		cont := &dto.ContainerInfo{
			ID:        shortID,
			Name:      strings.TrimPrefix(apiContainer.Names[0], "/"),
			Image:     apiContainer.Image,
			State:     state,
			Status:    apiContainer.Status,
			Ports:     c.convertPorts(apiContainer.Ports),
			Timestamp: time.Now(),
		}

		// Extract version from image tag
		imageParts := strings.Split(apiContainer.Image, ":")
		if len(imageParts) > 1 {
			cont.Version = imageParts[1]
		} else {
			cont.Version = "latest"
		}

		containers = append(containers, cont)

		if state == "running" {
			runningContainers = append(runningContainers, apiContainer)
		}
	}

	// Batch inspect running containers for detailed info
	if len(runningContainers) > 0 {
		startInspect := time.Now()
		containerMap := make(map[string]*dto.ContainerInfo)
		for _, cont := range containers {
			containerMap[cont.ID] = cont
		}

		for _, apiContainer := range runningContainers {
			shortID := apiContainer.ID[:12]
			inspectResult, err := c.dockerClient.ContainerInspect(ctx, apiContainer.ID, client.ContainerInspectOptions{})
			if err != nil {
				logger.Debug("Docker SDK: Failed to inspect container %s: %v", shortID, err)
				continue
			}

			inspectData := inspectResult.Container

			if cont, ok := containerMap[shortID]; ok {
				// Network mode
				if inspectData.HostConfig != nil {
					cont.NetworkMode = string(inspectData.HostConfig.NetworkMode)
				}

				// IP Address (get first available)
				if inspectData.NetworkSettings != nil {
					for _, network := range inspectData.NetworkSettings.Networks {
						if network.IPAddress.IsValid() {
							cont.IPAddress = network.IPAddress.String()
							break
						}
					}
				}

				// Port mappings
				if inspectData.HostConfig != nil {
					portMappings := []string{}
					for containerPort, bindings := range inspectData.HostConfig.PortBindings {
						for _, binding := range bindings {
							if binding.HostPort != "" {
								portMappings = append(portMappings, fmt.Sprintf("%s:%s", binding.HostPort, containerPort))
							}
						}
					}
					cont.PortMappings = portMappings

					// Restart policy
					cont.RestartPolicy = string(inspectData.HostConfig.RestartPolicy.Name)
					if cont.RestartPolicy == "" {
						cont.RestartPolicy = "no"
					}
				}

				// Volume mappings
				volumeMappings := []dto.VolumeMapping{}
				for _, mount := range inspectData.Mounts {
					volumeMappings = append(volumeMappings, dto.VolumeMapping{
						HostPath:      mount.Source,
						ContainerPath: mount.Destination,
						Mode:          mount.Mode,
					})
				}
				cont.VolumeMappings = volumeMappings

				// Uptime
				if inspectData.State != nil && inspectData.State.StartedAt != "" {
					startTime, err := time.Parse(time.RFC3339Nano, inspectData.State.StartedAt)
					if err == nil {
						cont.Uptime = dockerFormatUptime(time.Since(startTime))
					}
				}

				// Memory stats from cgroups (much faster than ContainerStats API)
				c.getMemoryFromCgroups(apiContainer.ID, cont)
			}
		}
		logger.Debug("Docker SDK: Inspect + cgroup stats took %v for %d containers", time.Since(startInspect), len(runningContainers))
	}

	// Publish event
	c.appCtx.Hub.Pub(containers, "container_list_update")
	logger.Debug("Docker SDK: Total collection took %v, published %d containers", time.Since(startTotal), len(containers))
}

// getMemoryFromCgroups reads memory stats directly from cgroup v2 filesystem
// This is much faster than using Docker's ContainerStats API
func (c *DockerCollector) getMemoryFromCgroups(fullID string, cont *dto.ContainerInfo) {
	cgroupPath := "/sys/fs/cgroup/docker/" + fullID

	// Read memory.current
	//nolint:gosec // G304: cgroupPath is constructed from trusted Docker container ID
	if data, err := os.ReadFile(cgroupPath + "/memory.current"); err == nil {
		var memUsage uint64
		if _, err := fmt.Sscanf(strings.TrimSpace(string(data)), "%d", &memUsage); err == nil {
			cont.MemoryUsage = memUsage
		}
	}

	// Read memory.max
	//nolint:gosec // G304: cgroupPath is constructed from trusted Docker container ID
	if data, err := os.ReadFile(cgroupPath + "/memory.max"); err == nil {
		content := strings.TrimSpace(string(data))
		if content != "max" {
			var memLimit uint64
			if _, err := fmt.Sscanf(content, "%d", &memLimit); err == nil {
				cont.MemoryLimit = memLimit
			}
		} else {
			// "max" means unlimited - use system memory
			cont.MemoryLimit = dockerGetSystemMemoryTotal()
		}
	}

	// Format memory display
	if cont.MemoryLimit > 0 {
		cont.MemoryDisplay = dockerFormatMemoryDisplay(cont.MemoryUsage, cont.MemoryLimit)
	}
}

// convertPorts converts Docker API port format to our DTO format
func (c *DockerCollector) convertPorts(apiPorts []container.PortSummary) []dto.PortMapping {
	ports := make([]dto.PortMapping, 0, len(apiPorts))
	for _, p := range apiPorts {
		ports = append(ports, dto.PortMapping{
			PrivatePort: int(p.PrivatePort),
			PublicPort:  int(p.PublicPort),
			Type:        p.Type,
		})
	}
	return ports
}

// dockerFormatUptime formats a duration as human-readable uptime string
func dockerFormatUptime(d time.Duration) string {
	days := int(d.Hours() / 24)
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60

	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, hours, minutes)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
	return fmt.Sprintf("%dm", minutes)
}

// dockerFormatMemoryDisplay formats memory as human-readable string
func dockerFormatMemoryDisplay(used, limit uint64) string {
	if limit == 0 {
		return "0 / 0"
	}

	usedMB := float64(used) / (1024 * 1024)
	limitMB := float64(limit) / (1024 * 1024)

	if limitMB >= 1024 {
		usedGB := usedMB / 1024
		limitGB := limitMB / 1024
		return fmt.Sprintf("%.2f GB / %.2f GB", usedGB, limitGB)
	}

	return fmt.Sprintf("%.2f MB / %.2f MB", usedMB, limitMB)
}

// dockerGetSystemMemoryTotal reads total system memory from /proc/meminfo
func dockerGetSystemMemoryTotal() uint64 {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0
	}
	lines := strings.SplitSeq(string(data), "\n")
	for line := range lines {
		if strings.HasPrefix(line, "MemTotal:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				var val uint64
				if _, err := fmt.Sscanf(fields[1], "%d", &val); err == nil {
					return val * 1024 // Convert from kB to bytes
				}
			}
		}
	}
	return 0
}
