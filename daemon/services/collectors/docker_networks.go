package collectors

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/constants"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
)

// dockerNetworksStartupStagger delays the first collection so it does not
// pile onto boot alongside every other collector.
const dockerNetworksStartupStagger = 5 * time.Second

// DockerNetworksCollector periodically lists Docker networks and publishes the
// result. Networks change rarely so the default interval is 60 seconds.
type DockerNetworksCollector struct {
	appCtx *domain.Context
	// ListFn fetches the network list; the collector factory in package services
	// injects the controller-backed implementation to avoid a
	// collectors→controllers import cycle.
	ListFn  func() ([]dto.DockerNetworkInfo, error)
	lastSig string
}

// NewDockerNetworksCollector creates a new DockerNetworks collector. ListFn must
// be set by the caller (the collector factory in package services) before Start
// or Collect is invoked.
func NewDockerNetworksCollector(ctx *domain.Context) *DockerNetworksCollector {
	return &DockerNetworksCollector{
		appCtx: ctx,
	}
}

// Start begins the periodic network listing after a startup stagger.
func (c *DockerNetworksCollector) Start(ctx context.Context, interval time.Duration) {
	logger.Info("Starting docker_networks collector (interval: %v)", interval)

	select {
	case <-ctx.Done():
		return
	case <-time.After(dockerNetworksStartupStagger):
	}

	func() {
		defer func() {
			if r := recover(); r != nil {
				logger.LogPanicWithStack("DockerNetworks collector", r)
			}
		}()
		c.Collect()
	}()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info("DockerNetworks collector stopping due to context cancellation")
			return
		case <-ticker.C:
			func() {
				defer func() {
					if r := recover(); r != nil {
						logger.LogPanicWithStack("DockerNetworks collector", r)
					}
				}()
				c.Collect()
			}()
		}
	}
}

// Collect fetches the current network list and publishes only when it changed
// since the last publish (dedupe to avoid no-op WebSocket broadcasts).
func (c *DockerNetworksCollector) Collect() {
	if c.ListFn == nil {
		logger.Warning("DockerNetworks: ListFn not set, skipping collect")
		return
	}
	networks, err := c.ListFn()
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
	if sig == c.lastSig {
		logger.Debug("DockerNetworks: no change (%d networks), skipping publish", len(networks))
		return
	}
	c.lastSig = sig

	domain.Publish(c.appCtx.Hub, constants.TopicDockerNetworksUpdate, result)
	logger.Info("DockerNetworks: published (%d networks)", len(networks))
}

// networksSignature builds an order-independent fingerprint of the network list
// (ID + driver), ignoring timestamps.
func networksSignature(networks []dto.DockerNetworkInfo) string {
	parts := make([]string, 0, len(networks))
	for _, n := range networks {
		parts = append(parts, fmt.Sprintf("%s:%s", n.ID, n.Driver))
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}
