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
	appCtx *domain.Context
	// CheckFn fetches container update status; the collector factory in package
	// services injects the controller-backed implementation to avoid a
	// collectors→controllers import cycle.
	CheckFn func() (*dto.ContainerUpdatesResult, error)
	// NotifyFn is called with the names of containers that newly became
	// update-available since the previous run. Injected by the factory in
	// package services to avoid a collectors→controllers import cycle.
	NotifyFn      func(names []string)
	lastSig       string
	prevAvailable map[string]bool
	baselineSet   bool
}

// NewDockerUpdateCollector creates a new DockerUpdate collector. CheckFn and
// NotifyFn must be set by the caller (the collector factory in package services)
// before Start or Collect is invoked — this avoids a collectors→controllers
// import cycle.
func NewDockerUpdateCollector(ctx *domain.Context) *DockerUpdateCollector {
	return &DockerUpdateCollector{
		appCtx:        ctx,
		prevAvailable: make(map[string]bool),
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
	if c.CheckFn == nil {
		logger.Warning("DockerUpdate: CheckFn not set, skipping collect")
		return
	}
	result, err := c.CheckFn()
	if err != nil {
		logger.Warning("DockerUpdate: check failed: %v", err)
		return
	}
	if result == nil {
		return
	}

	// Notification logic: fires only when opt-in is enabled and NotifyFn is set.
	// The first run establishes a baseline without notifying; subsequent runs
	// notify for containers that newly transitioned into update-available.
	if c.appCtx.DockerUpdateNotify && c.NotifyFn != nil {
		current := make(map[string]bool, len(result.Containers))
		for _, container := range result.Containers {
			if container.UpdateAvailable {
				current[container.ContainerID] = true
			}
		}

		if !c.baselineSet {
			// First run: record the baseline, do not notify.
			c.prevAvailable = current
			c.baselineSet = true
		} else {
			// Subsequent runs: find containers newly transitioned into available.
			var newlyAvailable []string
			for _, container := range result.Containers {
				if container.UpdateAvailable && !c.prevAvailable[container.ContainerID] {
					newlyAvailable = append(newlyAvailable, container.ContainerName)
				}
			}
			c.prevAvailable = current
			if len(newlyAvailable) > 0 {
				c.NotifyFn(newlyAvailable)
			}
		}
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
// (container ID + available flag + latest digest), ignoring the timestamp.
func updateSignature(r *dto.ContainerUpdatesResult) string {
	parts := make([]string, 0, len(r.Containers))
	for _, c := range r.Containers {
		parts = append(parts, fmt.Sprintf("%s=%t:%s", c.ContainerID, c.UpdateAvailable, c.LatestDigest))
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}
