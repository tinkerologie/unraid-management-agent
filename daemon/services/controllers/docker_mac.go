package controllers

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
)

const dockerContainerStateLimit = 8 << 20

// Docker's public inspect response mixes configured and generated endpoint
// MACs. Read the configured values from the local daemon's persisted state.
// An Engine reached over TCP may use a different filesystem.
func (dc *DockerController) configuredDockerMACs(ctx context.Context, original container.InspectResponse) (map[string]network.HardwareAddr, error) {
	mode := original.HostConfig.NetworkMode
	if original.NetworkSettings == nil || len(original.NetworkSettings.Networks) == 0 ||
		mode.IsHost() || mode.IsNone() || mode.IsContainer() || !strings.HasPrefix(dc.client.DaemonHost(), "unix://") {
		return nil, nil
	}
	info, err := dc.client.Info(ctx, client.InfoOptions{})
	if err != nil {
		return nil, fmt.Errorf("find local Docker state directory: %w", err)
	}
	return readDockerConfiguredMACs(info.Info.DockerRootDir, original.ID)
}

func readDockerConfiguredMACs(dockerRoot, id string) (map[string]network.HardwareAddr, error) {
	if len(id) != 64 || !filepath.IsAbs(dockerRoot) {
		return nil, fmt.Errorf("invalid Docker root directory or container ID")
	}
	if _, err := hex.DecodeString(id); err != nil {
		return nil, fmt.Errorf("invalid Docker container ID: %w", err)
	}
	root, err := os.OpenRoot(dockerRoot)
	if err != nil {
		return nil, fmt.Errorf("open local Docker state directory: %w", err)
	}
	defer func() { _ = root.Close() }()

	name := filepath.Join("containers", id, "config.v2.json")
	info, err := root.Stat(name)
	if err != nil {
		return nil, fmt.Errorf("stat Docker container state: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() > dockerContainerStateLimit {
		return nil, fmt.Errorf("docker container state is not a bounded regular file")
	}
	file, err := root.Open(name)
	if err != nil {
		return nil, fmt.Errorf("open Docker container state: %w", err)
	}
	defer func() { _ = file.Close() }()

	var state struct {
		ID              string
		NetworkSettings struct {
			Networks map[string]struct{ DesiredMacAddress network.HardwareAddr }
		}
	}
	if err := json.NewDecoder(io.LimitReader(file, dockerContainerStateLimit)).Decode(&state); err != nil {
		return nil, fmt.Errorf("decode Docker container state: %w", err)
	}
	if state.ID != id {
		return nil, fmt.Errorf("docker state ID does not match the inspected container")
	}
	addresses := make(map[string]network.HardwareAddr, len(state.NetworkSettings.Networks))
	for name, endpoint := range state.NetworkSettings.Networks {
		if len(endpoint.DesiredMacAddress) > 0 {
			addresses[name] = endpoint.DesiredMacAddress
		}
	}
	return addresses, nil
}
