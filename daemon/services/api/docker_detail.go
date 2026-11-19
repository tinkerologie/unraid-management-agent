package api

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/containerd/errdefs"
	"github.com/moby/moby/client"

	"github.com/tinkerologie/unraid-management-agent/daemon/dto"
	"github.com/tinkerologie/unraid-management-agent/daemon/lib"
	"github.com/tinkerologie/unraid-management-agent/daemon/services/collectors"
)

// GetDockerContainer resolves the live identity before using collected data.
// Docker can replace a container under the same name between collections.
func (s *Server) GetDockerContainer(ctx context.Context, ref string) (*dto.ContainerDetail, error) {
	if lib.ValidateContainerRef(ref) != nil {
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	engine, err := client.New(client.FromEnv)
	if err != nil {
		return nil, fmt.Errorf("open Docker client: %w", err)
	}
	defer func() { _ = engine.Close() }()
	result, err := engine.ContainerInspect(ctx, ref, client.ContainerInspectOptions{})
	if errdefs.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("inspect Docker container: %w", err)
	}
	inspected := result.Container
	name := strings.TrimPrefix(inspected.Name, "/")
	if inspected.ID == "" || name == "" || inspected.Config == nil || inspected.State == nil {
		return nil, fmt.Errorf("incomplete container information from Docker")
	}

	var info dto.ContainerInfo
	for _, cached := range s.GetDockerCache() {
		if cached.FullID == inspected.ID && cached.Name == name && cached.State == string(inspected.State.Status) {
			info = cached
			break
		}
	}
	if info.ID == "" {
		info = collectors.DockerContainerInfoFromInspect(inspected)
	}

	detail := &dto.ContainerDetail{ContainerInfo: info}
	// Native user templates describe containers on this host.
	if strings.HasPrefix(engine.DaemonHost(), "unix://") {
		detail.CatalogLink = s.communityAppsService.InstalledCatalogLink(ctx, name, inspected.Config.Labels)
	}
	return detail, nil
}
