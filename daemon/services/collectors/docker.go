package collectors

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/lib"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
)

type DockerCollector struct {
	ctx *domain.Context
}

func NewDockerCollector(ctx *domain.Context) *DockerCollector {
	return &DockerCollector{ctx: ctx}
}

func (c *DockerCollector) Start(ctx context.Context, interval time.Duration) {
	logger.Info("Starting docker collector (interval: %v)", interval)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

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

func (c *DockerCollector) Collect() {

	logger.Debug("Collecting docker data...")

	// Check if docker is available
	if !lib.CommandExists("docker") {
		logger.Warning("Docker command not found, skipping collection")
		return
	}

	// Collect container information
	containers, err := c.collectContainers()
	if err != nil {
		logger.Error("Failed to collect containers: %v", err)
		return
	}

	// Publish event
	c.ctx.Hub.Pub(containers, "container_list_update")
	logger.Debug("Published container_list_update event with %d containers", len(containers))
}

func (c *DockerCollector) collectContainers() ([]*dto.ContainerInfo, error) {
	// Get container list with JSON format
	output, err := lib.ExecCommandOutput("docker", "ps", "-a", "--format", "{{json .}}")
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}

	if strings.TrimSpace(output) == "" {
		return []*dto.ContainerInfo{}, nil
	}

	lines := strings.Split(strings.TrimSpace(output), "\n")
	containers := make([]*dto.ContainerInfo, 0, len(lines))

	for _, line := range lines {
		if line == "" {
			continue
		}

		var psOutput struct {
			ID     string `json:"ID"`
			Image  string `json:"Image"`
			Names  string `json:"Names"`
			State  string `json:"State"`
			Status string `json:"Status"`
			Ports  string `json:"Ports"`
		}

		if err := json.Unmarshal([]byte(line), &psOutput); err != nil {
			logger.Warning("Failed to parse container JSON", "error", err)
			continue
		}

		container := &dto.ContainerInfo{
			ID:        psOutput.ID,
			Name:      strings.TrimPrefix(psOutput.Names, "/"),
			Image:     psOutput.Image,
			State:     strings.ToLower(psOutput.State),
			Status:    psOutput.Status,
			Ports:     c.parsePorts(psOutput.Ports),
			Timestamp: time.Now(),
		}

		// Get container stats if running
		if container.State == "running" {
			if stats, err := c.getContainerStats(container.ID); err == nil {
				container.CPUPercent = stats.CPUPercent
				container.MemoryUsage = stats.MemoryUsage
				container.MemoryLimit = stats.MemoryLimit
				container.NetworkRX = stats.NetworkRX
				container.NetworkTX = stats.NetworkTX
			}
		}

		containers = append(containers, container)
	}

	return containers, nil
}

type containerStats struct {
	CPUPercent  float64
	MemoryUsage uint64
	MemoryLimit uint64
	NetworkRX   uint64
	NetworkTX   uint64
}

func (c *DockerCollector) getContainerStats(containerID string) (*containerStats, error) {
	// Get stats without streaming
	output, err := lib.ExecCommandOutput("docker", "stats", "--no-stream", "--format", "{{json .}}", containerID)
	if err != nil {
		return nil, err
	}

	var statsOutput struct {
		CPUPerc  string `json:"CPUPerc"`
		MemUsage string `json:"MemUsage"`
		MemPerc  string `json:"MemPerc"`
		NetIO    string `json:"NetIO"`
	}

	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &statsOutput); err != nil {
		return nil, err
	}

	stats := &containerStats{}

	// Parse CPU percentage (e.g., "0.50%")
	if cpuStr := strings.TrimSuffix(statsOutput.CPUPerc, "%"); cpuStr != "" {
		if cpu, err := strconv.ParseFloat(cpuStr, 64); err == nil {
			stats.CPUPercent = cpu
		}
	}

	// Parse memory usage (e.g., "1.5GiB / 8GiB")
	if parts := strings.Split(statsOutput.MemUsage, " / "); len(parts) == 2 {
		stats.MemoryUsage = c.parseSize(parts[0])
		stats.MemoryLimit = c.parseSize(parts[1])
	}

	// Parse network I/O (e.g., "1.2MB / 3.4MB")
	if parts := strings.Split(statsOutput.NetIO, " / "); len(parts) == 2 {
		stats.NetworkRX = c.parseSize(parts[0])
		stats.NetworkTX = c.parseSize(parts[1])
	}

	return stats, nil
}

func (c *DockerCollector) parseSize(sizeStr string) uint64 {
	sizeStr = strings.TrimSpace(sizeStr)
	if sizeStr == "" || sizeStr == "0B" {
		return 0
	}

	// Extract number and unit
	var value float64
	var unit string

	// Try to parse with unit
	if n, err := fmt.Sscanf(sizeStr, "%f%s", &value, &unit); n >= 1 && err == nil {
		unit = strings.ToUpper(unit)
		multiplier := uint64(1)

		switch {
		case strings.HasPrefix(unit, "K"):
			multiplier = 1024
		case strings.HasPrefix(unit, "M"):
			multiplier = 1024 * 1024
		case strings.HasPrefix(unit, "G"):
			multiplier = 1024 * 1024 * 1024
		case strings.HasPrefix(unit, "T"):
			multiplier = 1024 * 1024 * 1024 * 1024
		}

		return uint64(value * float64(multiplier))
	}

	return 0
}

func (c *DockerCollector) parsePorts(portsStr string) []dto.PortMapping {
	if portsStr == "" {
		return []dto.PortMapping{}
	}

	ports := []dto.PortMapping{}
	parts := strings.Split(portsStr, ", ")

	for _, part := range parts {
		// Format examples:
		// "0.0.0.0:8080->80/tcp"
		// "80/tcp"
		if strings.Contains(part, "->") {
			// Has port mapping
			mappingParts := strings.Split(part, "->")
			if len(mappingParts) == 2 {
				// Extract public port
				publicPart := mappingParts[0]
				if colonIdx := strings.LastIndex(publicPart, ":"); colonIdx >= 0 {
					publicPort, _ := strconv.Atoi(publicPart[colonIdx+1:])

					// Extract private port and type
					privatePart := mappingParts[1]
					if slashIdx := strings.Index(privatePart, "/"); slashIdx >= 0 {
						privatePort, _ := strconv.Atoi(privatePart[:slashIdx])
						portType := privatePart[slashIdx+1:]

						ports = append(ports, dto.PortMapping{
							PrivatePort: privatePort,
							PublicPort:  publicPort,
							Type:        portType,
						})
					}
				}
			}
		} else if strings.Contains(part, "/") {
			// Just exposed port, no mapping
			if slashIdx := strings.Index(part, "/"); slashIdx >= 0 {
				port, _ := strconv.Atoi(part[:slashIdx])
				portType := part[slashIdx+1:]

				ports = append(ports, dto.PortMapping{
					PrivatePort: port,
					PublicPort:  0,
					Type:        portType,
				})
			}
		}
	}

	return ports
}
