package controllers

import (
	"context"
	"regexp"
	"sync"
	"time"

	"github.com/tinkerologie/unraid-management-agent/daemon/constants"
	"github.com/tinkerologie/unraid-management-agent/daemon/dto"
	"github.com/tinkerologie/unraid-management-agent/daemon/lib"
	"github.com/tinkerologie/unraid-management-agent/daemon/logger"
	"github.com/tinkerologie/unraid-management-agent/daemon/services/plugininventory"
)

const (
	pluginCheckTimeout     = 30 * time.Second
	pluginCheckConcurrency = 4
)

var (
	pluginCheckExec     = lib.ExecCommandOutputWithContext
	pluginCheckSlot     = make(chan struct{}, 1)
	pluginCheckFilename = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]*\.plg$`)
)

// PluginController refreshes and reads update metadata for installed plugins.
type PluginController struct {
	resolve func(context.Context) (*dto.PluginList, error)
	targets func(context.Context) ([]string, error)
}

// NewPluginController creates a controller for the native Plugin Manager cache.
func NewPluginController() *PluginController {
	resolver := plugininventory.NewResolver()
	return &PluginController{resolve: resolver.Resolve, targets: resolver.CheckTargets}
}

// CheckPluginUpdates refreshes cached metadata and returns installable updates.
// Checks share one slot because the native command writes to a shared cache.
func (pc *PluginController) CheckPluginUpdates(parentCtx context.Context) ([]dto.PluginInfo, error) {
	ctx, cancel := context.WithTimeout(parentCtx, pluginCheckTimeout)
	defer cancel()

	select {
	case pluginCheckSlot <- struct{}{}:
		defer func() { <-pluginCheckSlot }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	targets, err := pc.targets(ctx)
	if err != nil {
		return nil, err
	}
	refreshPluginTargets(ctx, targets)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	list, err := pc.resolve(ctx)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	available := make([]dto.PluginInfo, 0, list.UpdatesAvailable)
	for _, plugin := range list.Plugins {
		if plugin.UpdateAvailable {
			available = append(available, plugin)
		}
	}
	return available, nil
}

// Check only valid registered plugins. The native checkall command also prunes
// dangling runtime registrations, which is outside an agent refresh.
func refreshPluginTargets(ctx context.Context, targets []string) {
	jobs := make(chan string, len(targets))
	for _, target := range targets {
		if pluginCheckFilename.MatchString(target) {
			jobs <- target
		}
	}
	close(jobs)

	var checks sync.WaitGroup
	for range min(pluginCheckConcurrency, len(jobs)) {
		checks.Go(func() {
			for target := range jobs {
				if ctx.Err() != nil {
					return
				}
				if _, err := pluginCheckExec(ctx, constants.PluginBin, "check", target); err != nil && ctx.Err() == nil {
					logger.Debug("Plugin: refresh failed for %q; using cached metadata: %v", target, err)
				}
			}
		})
	}
	checks.Wait()
}
