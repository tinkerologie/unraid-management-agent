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

// pluginUpdateStartupStagger delays the first scheduled check so plugin update
// detection does not pile onto boot alongside every other collector.
const pluginUpdateStartupStagger = 30 * time.Second

// pluginUpdateCheckTimeout bounds a full plugin update check. The check shells
// out to `plugin check`, which downloads update metadata; on networks without
// outbound internet access (issue #123) it must fail fast instead of wedging
// the collector for minutes per cycle.
const pluginUpdateCheckTimeout = 30 * time.Second

// PluginUpdateCollector periodically checks all plugins for available updates
// and publishes the result. It runs on a long interval because the check
// command downloads update metadata from the network.
type PluginUpdateCollector struct {
	appCtx *domain.Context
	// CheckFn fetches plugin update status; the collector factory in package
	// services injects the controller-backed implementation to avoid a
	// collectors→controllers import cycle. The context carries a deadline and
	// is cancelled on shutdown so the check command never outlives the collector.
	CheckFn func(ctx context.Context) (*dto.PluginList, error)
	// NotifyFn is called with the names of plugins that newly became
	// update-available since the previous run. Injected by the factory in
	// package services to avoid a collectors→controllers import cycle.
	NotifyFn      func(names []string)
	lastSig       string
	prevAvailable map[string]bool
	baselineSet   bool
}

// NewPluginUpdateCollector creates a new PluginUpdate collector. CheckFn and
// NotifyFn must be set by the caller (the collector factory in package services)
// before Start or Collect is invoked — this avoids a collectors→controllers
// import cycle.
func NewPluginUpdateCollector(ctx *domain.Context) *PluginUpdateCollector {
	return &PluginUpdateCollector{
		appCtx:        ctx,
		prevAvailable: make(map[string]bool),
	}
}

// Start begins the periodic plugin update check after a startup stagger.
func (c *PluginUpdateCollector) Start(ctx context.Context, interval time.Duration) {
	logger.Info("Starting plugin_update collector (interval: %v)", interval)

	select {
	case <-ctx.Done():
		return
	case <-time.After(pluginUpdateStartupStagger):
	}

	func() {
		defer func() {
			if r := recover(); r != nil {
				logger.LogPanicWithStack("PluginUpdate collector", r)
			}
		}()
		c.Collect(ctx)
	}()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info("PluginUpdate collector stopping due to context cancellation")
			return
		case <-ticker.C:
			func() {
				defer func() {
					if r := recover(); r != nil {
						logger.LogPanicWithStack("PluginUpdate collector", r)
					}
				}()
				c.Collect(ctx)
			}()
		}
	}
}

// Collect runs a plugin update check and publishes the result only if it
// changed since the last publish (dedupe to avoid no-op WebSocket broadcasts).
// The passed lifecycle context bounds the check so it is cancelled on shutdown.
func (c *PluginUpdateCollector) Collect(parentCtx context.Context) {
	if c.CheckFn == nil {
		logger.Warning("PluginUpdate: CheckFn not set, skipping collect")
		return
	}
	ctx, cancel := context.WithTimeout(parentCtx, pluginUpdateCheckTimeout)
	defer cancel()
	result, err := c.CheckFn(ctx)
	if err != nil {
		logger.Warning("PluginUpdate: check failed: %v", err)
		return
	}
	if result == nil {
		return
	}

	// Notification logic: fires only when NotifyFn is set.
	// The first run establishes a baseline without notifying; subsequent runs
	// notify for plugins that newly transitioned into update-available.
	if c.NotifyFn != nil {
		current := make(map[string]bool, len(result.Plugins))
		for _, plugin := range result.Plugins {
			if plugin.UpdateAvailable {
				current[plugin.Name] = true
			}
		}

		if !c.baselineSet {
			// First run: record the baseline, do not notify.
			c.prevAvailable = current
			c.baselineSet = true
		} else {
			// Subsequent runs: find plugins newly transitioned into available.
			var newlyAvailable []string
			for _, plugin := range result.Plugins {
				if plugin.UpdateAvailable && !c.prevAvailable[plugin.Name] {
					newlyAvailable = append(newlyAvailable, plugin.Name)
				}
			}
			c.prevAvailable = current
			if len(newlyAvailable) > 0 {
				c.NotifyFn(newlyAvailable)
			}
		}
	}

	sig := pluginUpdateSignature(result)
	if sig == c.lastSig {
		logger.Debug("PluginUpdate: no change (%d updates available), skipping publish", result.UpdatesAvailable)
		return
	}
	c.lastSig = sig

	domain.Publish(c.appCtx.Hub, constants.TopicPluginUpdatesUpdate, result)
	logger.Info("PluginUpdate: published (%d/%d plugins have updates)", result.UpdatesAvailable, result.TotalCount)
}

// pluginUpdateSignature builds an order-independent fingerprint of update
// status (plugin name + available flag + latest version), ignoring timestamp.
func pluginUpdateSignature(r *dto.PluginList) string {
	parts := make([]string, 0, len(r.Plugins))
	for _, p := range r.Plugins {
		parts = append(parts, fmt.Sprintf("%s=%t:%s", p.Name, p.UpdateAvailable, p.LatestVersion))
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}
