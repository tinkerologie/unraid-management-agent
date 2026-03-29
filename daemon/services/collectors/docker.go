package collectors

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/constants"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
)

// cpuSnapshot holds a point-in-time cgroup CPU usage reading for delta calculation.
type cpuSnapshot struct {
	usageUsec int64
	readAt    time.Time
}

// DockerCollector collects Docker container information using the Docker SDK.
// This is significantly faster than CLI commands as it avoids process spawning.
type DockerCollector struct {
	appCtx       *domain.Context
	dockerClient *client.Client
	initialized  bool
	prevCPU      map[string]cpuSnapshot // keyed by full container ID
}

// NewDockerCollector creates a new Docker SDK-based collector
func NewDockerCollector(ctx *domain.Context) *DockerCollector {
	return &DockerCollector{
		appCtx:      ctx,
		initialized: false,
		prevCPU:     make(map[string]cpuSnapshot),
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

	// Run once immediately with panic recovery
	func() {
		defer func() {
			if r := recover(); r != nil {
				logger.LogPanicWithStack("Docker collector", r)
			}
		}()
		c.Collect()
	}()

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
			func() {
				defer func() {
					if r := recover(); r != nil {
						logger.LogPanicWithStack("Docker collector", r)
					}
				}()
				c.Collect()
			}()
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
		domain.Publish(c.appCtx.Hub, constants.TopicContainerListUpdate, []*dto.ContainerInfo{})
		return
	}

	ctx := context.Background()

	// List all containers (including stopped) - SDK is much faster than CLI
	startList := time.Now()
	result, err := c.dockerClient.ContainerList(ctx, client.ContainerListOptions{All: true})
	if err != nil {
		logger.Debug("Failed to list containers via SDK: %v", err)
		domain.Publish(c.appCtx.Hub, constants.TopicContainerListUpdate, []*dto.ContainerInfo{})
		return
	}
	apiContainers := result.Items
	logger.Debug("Docker SDK: ContainerList took %v for %d containers", time.Since(startList), len(apiContainers))

	containers := make([]*dto.ContainerInfo, 0, len(apiContainers))
	var runningContainers []container.Summary

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

				// CPU stats from cgroups (delta between collections)
				c.getCPUFromCgroups(apiContainer.ID, cont)
			}
		}
		logger.Debug("Docker SDK: Inspect + cgroup stats took %v for %d containers", time.Since(startInspect), len(runningContainers))
	}

	// Prune stale CPU snapshots for containers that no longer exist
	c.pruneStaleSnapshots(runningContainers)

	// Publish event
	domain.Publish(c.appCtx.Hub, constants.TopicContainerListUpdate, containers)
	logger.Debug("Docker SDK: Total collection took %v, published %d containers", time.Since(startTotal), len(containers))
}

// getMemoryFromCgroups reads memory stats directly from cgroup v2 filesystem
// This is much faster than using Docker's ContainerStats API
func (c *DockerCollector) getMemoryFromCgroups(fullID string, cont *dto.ContainerInfo) {
	cgroupPath := "/sys/fs/cgroup/docker/" + fullID

	// Read memory.current
	// #nosec G304 -- cgroupPath is constructed from a trusted Docker container ID under /sys/fs/cgroup/docker.
	if data, err := os.ReadFile(cgroupPath + "/memory.current"); err == nil {
		var memUsage uint64
		if _, err := fmt.Sscanf(strings.TrimSpace(string(data)), "%d", &memUsage); err == nil {
			cont.MemoryUsage = memUsage
		}
	}

	// Read memory.max
	// #nosec G304 -- cgroupPath is constructed from a trusted Docker container ID under /sys/fs/cgroup/docker.
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
		cont.MemoryPercent = float64(cont.MemoryUsage) / float64(cont.MemoryLimit) * 100
	}
	cont.MemoryUsageMB = float64(cont.MemoryUsage) / (1024 * 1024)
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

// getCPUFromCgroups reads cpu.stat from cgroup v2 to compute per-container CPU%.
// Requires two consecutive readings; the first call for a container records the
// baseline and leaves CPUPercent at 0.
func (c *DockerCollector) getCPUFromCgroups(fullID string, cont *dto.ContainerInfo) {
	cgroupPath := "/sys/fs/cgroup/docker/" + fullID + "/cpu.stat"

	// #nosec G304 -- cgroupPath is constructed from a trusted Docker container ID under /sys/fs/cgroup/docker.
	data, err := os.ReadFile(cgroupPath)
	if err != nil {
		return
	}

	var usageUsec int64
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "usage_usec ") {
			if _, err := fmt.Sscanf(line, "usage_usec %d", &usageUsec); err != nil {
				return
			}
			break
		}
	}

	now := time.Now()
	snap := cpuSnapshot{usageUsec: usageUsec, readAt: now}

	prev, hasPrev := c.prevCPU[fullID]
	c.prevCPU[fullID] = snap

	if !hasPrev {
		return
	}

	elapsedUsec := now.Sub(prev.readAt).Microseconds()
	if elapsedUsec <= 0 {
		return
	}

	deltaUsec := usageUsec - prev.usageUsec
	if deltaUsec < 0 {
		return
	}

	// CPU% = (cpu time used / wall time) / num_cpus * 100
	numCPU := float64(runtime.NumCPU())
	if numCPU < 1 {
		numCPU = 1
	}
	cont.CPUPercent = (float64(deltaUsec) / float64(elapsedUsec)) / numCPU * 100
}

// pruneStaleSnapshots removes CPU snapshots for containers that are no longer running.
func (c *DockerCollector) pruneStaleSnapshots(running []container.Summary) {
	active := make(map[string]struct{}, len(running))
	for _, rc := range running {
		active[rc.ID] = struct{}{}
	}
	for id := range c.prevCPU {
		if _, ok := active[id]; !ok {
			delete(c.prevCPU, id)
		}
	}
}
