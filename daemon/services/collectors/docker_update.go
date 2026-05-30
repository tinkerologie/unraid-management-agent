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

// dockerUpdateStartupStagger delays the first scheduled check so update
// detection does not pile onto boot alongside every other collector.
const dockerUpdateStartupStagger = 45 * time.Second

// DockerUpdateCollector periodically checks all containers for available image
// updates (registry digest comparison) and publishes the result. It runs on a
// long interval because DistributionInspect hits the registry and Docker Hub
// rate-limits anonymous manifest requests.
type DockerUpdateCollector struct {
	appCtx  *domain.Context
	checkFn func() (*dto.ContainerUpdatesResult, error)
	lastSig string
}

// NewDockerUpdateCollector creates a new DockerUpdate collector.
// checkFn is the function used to fetch update status; callers (e.g. the
// orchestrator) must inject a real implementation before calling Start or
// Collect, as this package must not import the controllers package to avoid
// an import cycle.
func NewDockerUpdateCollector(ctx *domain.Context) *DockerUpdateCollector {
	return &DockerUpdateCollector{
		appCtx: ctx,
	}
}

// Start begins the periodic update check after a startup stagger.
func (c *DockerUpdateCollector) Start(ctx context.Context, interval time.Duration) {
	logger.Info("Starting docker_update collector (interval: %v)", interval)

	select {
	case <-ctx.Done():
		return
	case <-time.After(dockerUpdateStartupStagger):
	}

	func() {
		defer func() {
			if r := recover(); r != nil {
				logger.LogPanicWithStack("DockerUpdate collector", r)
			}
		}()
		c.Collect()
	}()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info("DockerUpdate collector stopping due to context cancellation")
			return
		case <-ticker.C:
			func() {
				defer func() {
					if r := recover(); r != nil {
						logger.LogPanicWithStack("DockerUpdate collector", r)
					}
				}()
				c.Collect()
			}()
		}
	}
}

// Collect runs an update check and publishes the result only if it changed
// since the last publish (dedupe to avoid no-op WebSocket broadcasts).
func (c *DockerUpdateCollector) Collect() {
	if c.checkFn == nil {
		logger.Warning("DockerUpdate: checkFn not set, skipping collect")
		return
	}
	result, err := c.checkFn()
	if err != nil {
		logger.Warning("DockerUpdate: check failed: %v", err)
		return
	}
	if result == nil {
		return
	}

	sig := updateSignature(result)
	if sig == c.lastSig {
		logger.Debug("DockerUpdate: no change (%d updates available), skipping publish", result.UpdatesAvailable)
		return
	}
	c.lastSig = sig

	domain.Publish(c.appCtx.Hub, constants.TopicDockerUpdatesUpdate, result)
	logger.Info("DockerUpdate: published (%d/%d containers have updates)", result.UpdatesAvailable, result.TotalCount)
}

// updateSignature builds an order-independent fingerprint of update status
// (container ID + available flag), ignoring the timestamp.
func updateSignature(r *dto.ContainerUpdatesResult) string {
	parts := make([]string, 0, len(r.Containers))
	for _, c := range r.Containers {
		parts = append(parts, fmt.Sprintf("%s=%t", c.ContainerID, c.UpdateAvailable))
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}
