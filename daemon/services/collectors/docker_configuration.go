package collectors

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/moby/moby/api/types/container"

	"github.com/tinkerologie/unraid-management-agent/daemon/dto"
)

// DockerContainerInfoFromInspect supplies configuration before the collector
// has sampled this identity. The live statistics arrive with the next sample.
func DockerContainerInfoFromInspect(inspected container.InspectResponse) dto.ContainerInfo {
	info := dto.ContainerInfo{
		FullID:       inspected.ID,
		ID:           inspected.ID[:min(12, len(inspected.ID))],
		Name:         strings.TrimPrefix(inspected.Name, "/"),
		Image:        inspected.Config.Image,
		Version:      dockerImageVersion(inspected.Config.Image),
		State:        string(inspected.State.Status),
		Status:       string(inspected.State.Status),
		Ports:        []dto.PortMapping{},
		UpdateStatus: dto.UpdateStatusUnknown,
		Timestamp:    time.Now(),
	}
	populateDockerConfiguration(&info, inspected)
	if inspected.HostConfig != nil {
		capacity := uint64(max(inspected.HostConfig.Memory, 0))
		info.ConfiguredMemoryLimit = &capacity
	}
	if inspected.NetworkSettings != nil {
		for port, bindings := range inspected.NetworkSettings.Ports {
			mapping := dto.PortMapping{PrivatePort: int(port.Num()), Type: string(port.Proto())}
			if len(bindings) == 0 {
				info.Ports = append(info.Ports, mapping)
			}
			for _, binding := range bindings {
				mapping.PublicPort, _ = strconv.Atoi(binding.HostPort)
				info.Ports = append(info.Ports, mapping)
			}
		}
	}
	return info
}

func dockerImageVersion(image string) string {
	_, suffix, found := strings.Cut(image, ":")
	if !found {
		return "latest"
	}
	version, _, _ := strings.Cut(suffix, ":")
	return version
}

func populateDockerConfiguration(info *dto.ContainerInfo, inspected container.InspectResponse) {
	if inspected.HostConfig != nil {
		info.NetworkMode = string(inspected.HostConfig.NetworkMode)
		info.PortMappings = []string{}
		for port, bindings := range inspected.HostConfig.PortBindings {
			for _, binding := range bindings {
				if binding.HostPort != "" {
					info.PortMappings = append(info.PortMappings, fmt.Sprintf("%s:%s", binding.HostPort, port))
				}
			}
		}
		info.RestartPolicy = string(inspected.HostConfig.RestartPolicy.Name)
		if info.RestartPolicy == "" {
			info.RestartPolicy = "no"
		}
	}
	if inspected.NetworkSettings != nil {
		info.Networks = buildNetworkAttachments(inspected.NetworkSettings.Networks)
		info.IPAddress, info.MACAddress = selectLegacyEndpoint(info.NetworkMode, info.Networks)
	}
	info.VolumeMappings = make([]dto.VolumeMapping, 0, len(inspected.Mounts))
	for _, mount := range inspected.Mounts {
		info.VolumeMappings = append(info.VolumeMappings, dto.VolumeMapping{
			HostPath: mount.Source, ContainerPath: mount.Destination, Mode: mount.Mode,
		})
	}
	if inspected.State != nil && inspected.State.StartedAt != "" {
		if started, err := time.Parse(time.RFC3339Nano, inspected.State.StartedAt); err == nil {
			info.Uptime = dockerFormatUptime(time.Since(started))
		}
	}
	info.RestartCount = inspected.RestartCount
}
