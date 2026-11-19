package communityapps

import (
	"context"
	"path"
	"regexp"
	"slices"
	"strings"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/client"
)

type volumeMetadataReader interface {
	VolumeInspect(context.Context, string, client.VolumeInspectOptions) (client.VolumeInspectResult, error)
}

type volumeBindMetadata struct {
	driver string
	source string
}

var dockerVolumeName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,254}$`)

// withNamedVolumeBinds adds writable local bind-backed volume paths to the
// same conflict checks as direct binds. Docker's container list reports the
// volume's _data directory and does not identify the local driver's device.
func withNamedVolumeBinds(ctx context.Context, docker PlanningDocker, mounts []Mount, containers []container.Summary) ([]container.Summary, error) {
	if len(mounts) == 0 {
		return containers, nil
	}
	reader, _ := docker.(volumeMetadataReader)
	sources := make(map[string]volumeBindMetadata)
	normalized := slices.Clone(containers)
	for i, existing := range containers {
		normalized[i].Mounts = slices.Clone(existing.Mounts)
		for j, mounted := range existing.Mounts {
			if !mounted.RW || mounted.Type != mount.TypeVolume {
				continue
			}
			metadata, ok := sources[mounted.Name]
			if !ok {
				var err error
				metadata, err = inspectVolumeBind(ctx, reader, mounted.Name)
				if err != nil {
					return nil, err
				}
				sources[mounted.Name] = metadata
			}
			if mounted.Driver != "" && mounted.Driver != metadata.driver {
				return nil, ErrDockerState
			}
			if metadata.source != "" {
				normalized[i].Mounts[j].Type = mount.TypeBind
				normalized[i].Mounts[j].Source = metadata.source
			}
		}
	}
	return normalized, nil
}

func inspectVolumeBind(ctx context.Context, reader volumeMetadataReader, name string) (volumeBindMetadata, error) {
	if reader == nil || !dockerVolumeName.MatchString(name) {
		return volumeBindMetadata{}, ErrDockerState
	}
	result, err := reader.VolumeInspect(ctx, name, client.VolumeInspectOptions{})
	v := result.Volume
	if err != nil || v.Name != name || v.Driver == "" {
		return volumeBindMetadata{}, ErrDockerState
	}
	metadata := volumeBindMetadata{driver: v.Driver}
	if v.Driver != "local" {
		return metadata, nil
	}
	if v.Scope != "local" {
		return volumeBindMetadata{}, ErrDockerState
	}
	for option := range strings.SplitSeq(v.Options["o"], ",") {
		switch strings.TrimSpace(option) {
		case "bind", "rbind":
			device := v.Options["device"]
			if !path.IsAbs(device) || strings.ContainsRune(device, 0) {
				return volumeBindMetadata{}, ErrDockerState
			}
			metadata.source = device
			return metadata, nil
		}
	}
	return metadata, nil
}
