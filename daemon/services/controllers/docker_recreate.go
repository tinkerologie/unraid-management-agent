package controllers

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/containerd/errdefs"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"

	"github.com/tinkerologie/unraid-management-agent/daemon/logger"
)

const dockerUpdateAttemptLabel = "io.tinkerologie.uma.update-attempt"

// Updates can enter through different controllers and use either a name or an
// ID. Keep both reserved until cleanup or rollback finishes.
var dockerUpdatesInProgress sync.Map

func reserveDockerUpdate(id, name string) (func(), error) {
	idKey, nameKey := "id:"+id, "name:"+name
	if _, busy := dockerUpdatesInProgress.LoadOrStore(idKey, struct{}{}); busy {
		return nil, fmt.Errorf("an update is already in progress for container %q", name)
	}
	if _, busy := dockerUpdatesInProgress.LoadOrStore(nameKey, struct{}{}); busy {
		dockerUpdatesInProgress.Delete(idKey)
		return nil, fmt.Errorf("an update is already in progress for container %q", name)
	}
	return func() {
		dockerUpdatesInProgress.Delete(nameKey)
		dockerUpdatesInProgress.Delete(idKey)
	}, nil
}

type dockerReplacement struct {
	originalID    string
	name          string
	retainedName  string
	wasRunning    bool
	attempt       string
	replacementID string
	restartPolicy container.RestartPolicy
}

func (dc *DockerController) recreateDockerContainer(ctx context.Context, original container.InspectResponse) (id, message string, err error) {
	macs, err := dc.configuredDockerMACs(ctx, original)
	if err != nil {
		return "", "", fmt.Errorf("read configured Docker MAC addresses: %w", err)
	}
	change := dockerReplacement{
		originalID:    original.ID,
		name:          strings.TrimPrefix(original.Name, "/"),
		wasRunning:    original.State != nil && original.State.Running,
		attempt:       rand.Text(),
		restartPolicy: original.HostConfig.RestartPolicy,
	}
	change.retainedName = "uma-prev-" + shortID(original.ID) + "-" + strings.ToLower(change.attempt[:8])

	// A retained container must not restart after a Docker daemon restart.
	if !change.restartPolicy.IsNone() {
		disabled := container.RestartPolicy{Name: container.RestartPolicyDisabled}
		if _, err := dc.client.ContainerUpdate(ctx, change.originalID, client.ContainerUpdateOptions{RestartPolicy: &disabled}); err != nil {
			return "", "", dc.recoverDockerUpdate(change, "disable original restart policy", err)
		}
	}

	if change.wasRunning {
		if _, err := dc.client.ContainerStop(ctx, original.ID, client.ContainerStopOptions{}); err != nil {
			return "", "", dc.recoverDockerUpdate(change, "stop original", err)
		}
	}
	if _, err := dc.client.ContainerRename(ctx, original.ID, client.ContainerRenameOptions{NewName: change.retainedName}); err != nil {
		return "", "", dc.recoverDockerUpdate(change, "retain original", err)
	}

	cfg := *original.Config
	cfg.Labels = maps.Clone(cfg.Labels)
	if cfg.Labels == nil {
		cfg.Labels = make(map[string]string, 1)
	}
	// On a lost create response, the name alone cannot identify our replacement.
	cfg.Labels[dockerUpdateAttemptLabel] = change.attempt
	result, err := dc.client.ContainerCreate(ctx, client.ContainerCreateOptions{
		Config: &cfg, HostConfig: dockerReplacementHostConfig(original), Name: change.name,
		NetworkingConfig: dockerReplacementNetworks(original.NetworkSettings, shortID(original.ID), macs),
	})
	if err != nil {
		return "", "", dc.recoverDockerUpdate(change, "create replacement", err)
	}
	if result.ID == "" || result.ID == original.ID {
		return "", "", dc.recoverDockerUpdate(change, "create replacement", fmt.Errorf("invalid replacement ID from Docker"))
	}
	change.replacementID = result.ID

	if change.wasRunning {
		if _, err := dc.client.ContainerStart(ctx, result.ID, client.ContainerStartOptions{}); err != nil {
			return "", "", dc.recoverDockerUpdate(change, "start replacement", err)
		}
	}
	message = "Container updated successfully"
	if _, err := dc.client.ContainerRemove(ctx, original.ID, client.ContainerRemoveOptions{}); err != nil && !errdefs.IsNotFound(err) {
		logger.Warning("Docker: retained original %s (%s) after updating %s: %v", change.retainedName, shortID(original.ID), change.name, err)
		message += fmt.Sprintf(". Original retained as %s (%s) for manual cleanup", change.retainedName, shortID(original.ID))
	}
	return result.ID, message, nil
}

// Reuse Docker-assigned volume names when the original create request left the
// source implicit. Docker would otherwise create empty volumes on recreation.
func dockerReplacementHostConfig(original container.InspectResponse) *container.HostConfig {
	config := *original.HostConfig
	config.Mounts = slices.Clone(config.Mounts)
	config.Binds = slices.Clone(config.Binds)
	bound := make(map[string]struct{}, len(config.Binds)+len(config.Mounts))
	anonymousBinds := make(map[string]int)
	for index, bind := range config.Binds {
		sourceOrTarget, rest, _ := strings.Cut(bind, ":")
		if !strings.HasPrefix(rest, "/") {
			// Linux bind targets are absolute paths. A suffix with no
			// leading slash is the mode list of an anonymous volume.
			anonymousBinds[sourceOrTarget] = index
			continue
		}
		target, _, _ := strings.Cut(rest, ":")
		bound[target] = struct{}{}
	}
	for _, existing := range config.Mounts {
		if existing.Type == mount.TypeVolume && existing.Source == "" {
			// Resolve the source below while retaining the original mount options.
			continue
		}
		bound[existing.Target] = struct{}{}
	}
	for _, existing := range original.Mounts {
		if existing.Type != mount.TypeVolume || existing.Name == "" {
			continue
		}
		if _, found := bound[existing.Destination]; found {
			continue
		}
		if index := slices.IndexFunc(config.Mounts, func(entry mount.Mount) bool { return entry.Target == existing.Destination }); index >= 0 {
			entry := &config.Mounts[index]
			entry.Source = existing.Name
			options := mount.VolumeOptions{}
			if entry.VolumeOptions != nil {
				options = *entry.VolumeOptions
			}
			options.NoCopy = true
			entry.VolumeOptions = &options
			continue
		}
		if index, found := anonymousBinds[existing.Destination]; found {
			bind := config.Binds[index]
			_, modes, hasModes := strings.Cut(bind, ":")
			switch {
			case !hasModes:
				bind += ":nocopy"
			case !slices.Contains(strings.Split(modes, ","), "nocopy"):
				bind += ",nocopy"
			}
			config.Binds[index] = existing.Name + ":" + bind
			continue
		}
		config.Mounts = append(config.Mounts, mount.Mount{
			Type: mount.TypeVolume, Source: existing.Name, Target: existing.Destination, ReadOnly: !existing.RW,
			VolumeOptions: &mount.VolumeOptions{NoCopy: true},
		})
	}
	return &config
}

func dockerReplacementNetworks(settings *container.NetworkSettings, oldShortID string, macs map[string]network.HardwareAddr) *network.NetworkingConfig {
	if settings == nil || len(settings.Networks) == 0 {
		return nil
	}
	endpoints := make(map[string]*network.EndpointSettings, len(settings.Networks))
	for name, previous := range settings.Networks {
		if previous == nil {
			continue
		}
		aliases := slices.DeleteFunc(slices.Clone(previous.Aliases), func(alias string) bool {
			return alias == oldShortID
		})
		endpoints[name] = &network.EndpointSettings{
			IPAMConfig: previous.IPAMConfig.Copy(),
			Links:      slices.Clone(previous.Links), Aliases: aliases, DriverOpts: maps.Clone(previous.DriverOpts),
			MacAddress: macs[name], GwPriority: previous.GwPriority,
		}
	}
	if len(endpoints) == 0 {
		return nil
	}
	return &network.NetworkingConfig{EndpointsConfig: endpoints}
}

func (dc *DockerController) recoverDockerUpdate(change dockerReplacement, stage string, cause error) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := dc.restoreDockerOriginal(ctx, change); err != nil {
		return fmt.Errorf("update failed at %s: %w. Recovery failed: %w. Check original %s (retained name %s, ID %s)",
			stage, cause, err, change.name, change.retainedName, change.originalID)
	}
	return fmt.Errorf("update failed at %s (original restored): %w", stage, cause)
}

func (dc *DockerController) restoreDockerOriginal(ctx context.Context, change dockerReplacement) error {
	original, err := dc.client.ContainerInspect(ctx, change.originalID, client.ContainerInspectOptions{})
	if err != nil {
		return fmt.Errorf("inspect original: %w", err)
	}
	nameChanged := strings.TrimPrefix(original.Container.Name, "/") != change.name
	if nameChanged || change.replacementID != "" {
		if err := dc.removeFailedDockerReplacement(ctx, change); err != nil {
			return err
		}
	}
	if nameChanged {
		if _, err := dc.client.ContainerRename(ctx, change.originalID, client.ContainerRenameOptions{NewName: change.name}); err != nil {
			return fmt.Errorf("restore original name: %w", err)
		}
	}
	var policyErr, startErr error
	if !change.restartPolicy.IsNone() &&
		(original.Container.HostConfig == nil || !change.restartPolicy.IsSame(&original.Container.HostConfig.RestartPolicy)) {
		if _, err := dc.client.ContainerUpdate(ctx, change.originalID, client.ContainerUpdateOptions{RestartPolicy: &change.restartPolicy}); err != nil {
			policyErr = fmt.Errorf("restore original restart policy: %w", err)
		}
	}
	if change.wasRunning && (original.Container.State == nil || !original.Container.State.Running) {
		if _, err := dc.client.ContainerStart(ctx, change.originalID, client.ContainerStartOptions{}); err != nil {
			startErr = fmt.Errorf("restart original: %w", err)
		}
	}
	return errors.Join(policyErr, startErr)
}

func (dc *DockerController) removeFailedDockerReplacement(ctx context.Context, change dockerReplacement) error {
	id := change.replacementID
	if id == "" {
		result, err := dc.client.ContainerInspect(ctx, change.name, client.ContainerInspectOptions{})
		if errdefs.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("check for a partial replacement: %w", err)
		}
		candidate := result.Container
		if candidate.Config == nil || candidate.Config.Labels[dockerUpdateAttemptLabel] != change.attempt {
			return fmt.Errorf("name %q is occupied by a container outside this update", change.name)
		}
		id = candidate.ID
	}
	if id == "" || id == change.originalID {
		return fmt.Errorf("refuse to remove a replacement without a distinct Docker ID")
	}
	if _, err := dc.client.ContainerRemove(ctx, id, client.ContainerRemoveOptions{Force: true}); err != nil && !errdefs.IsNotFound(err) {
		return fmt.Errorf("remove failed replacement %s: %w", shortID(id), err)
	}
	return nil
}
