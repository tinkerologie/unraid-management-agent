package collectors

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/tinkerologie/unraid-management-agent/daemon/constants"
	"github.com/tinkerologie/unraid-management-agent/daemon/domain"
	"github.com/tinkerologie/unraid-management-agent/daemon/dto"
	"github.com/tinkerologie/unraid-management-agent/daemon/logger"
)

// DockerNetworksCollector periodically lists Docker networks and publishes the
// result. Networks change rarely so the default interval is 60 seconds.
type DockerNetworksCollector struct {
	appCtx *domain.Context
	// ListFn fetches the network list; the collector factory in package services
	// injects the controller-backed implementation to avoid a
	// collectors→controllers import cycle.
	ListFn    func(context.Context) ([]dto.DockerNetworkInfo, error)
	lastSig   string
	published bool
}

// NewDockerNetworksCollector creates a new DockerNetworks collector. ListFn must
// be set by the caller (the collector factory in package services) before Start
// or Collect is invoked.
func NewDockerNetworksCollector(ctx *domain.Context) *DockerNetworksCollector {
	return &DockerNetworksCollector{
		appCtx: ctx,
	}
}

// Collect fetches the current network list and publishes only when it changed
// since the last publish (dedupe to avoid no-op WebSocket broadcasts).
func (c *DockerNetworksCollector) Collect(ctx context.Context) {
	if c.ListFn == nil {
		logger.Warning("DockerNetworks: ListFn not set, skipping collect")
		return
	}
	networks, err := c.ListFn(ctx)
	if ctx.Err() != nil {
		return
	}
	if err != nil {
		logger.Warning("DockerNetworks: list failed: %v", err)
		return
	}

	result := &dto.DockerNetworkList{
		Networks:  networks,
		Count:     len(networks),
		Timestamp: time.Now(),
	}

	sig := networksSignature(networks)
	if c.published && sig == c.lastSig {
		logger.Debug("DockerNetworks: no change (%d networks), skipping publish", len(networks))
		return
	}
	c.lastSig = sig
	c.published = true

	domain.Publish(c.appCtx.Hub, constants.TopicDockerNetworksUpdate, result)
	logger.Info("DockerNetworks: published (%d networks)", len(networks))
}

// networksSignature builds an order-independent fingerprint of the network list
// (ID, driver and membership), ignoring timestamps and member order.
func networksSignature(networks []dto.DockerNetworkInfo) string {
	parts := make([]string, 0, len(networks))
	for _, n := range networks {
		members := slices.Clone(n.ContainerNames)
		slices.Sort(members)
		var key strings.Builder
		key.WriteString(strconv.Quote(n.ID))
		key.WriteString(strconv.Quote(n.Driver))
		key.WriteString(strconv.FormatBool(n.MembershipUnavailable))
		for _, name := range slices.Compact(members) {
			key.WriteString(strconv.Quote(name))
		}
		parts = append(parts, strconv.Quote(key.String()))
	}
	slices.Sort(parts)
	return strings.Join(parts, "")
}
