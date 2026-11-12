package controllers

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"github.com/tinkerologie/unraid-management-agent/daemon/constants"
	"github.com/tinkerologie/unraid-management-agent/daemon/dto"
	"github.com/tinkerologie/unraid-management-agent/daemon/lib"
	"github.com/tinkerologie/unraid-management-agent/daemon/logger"
	"github.com/tinkerologie/unraid-management-agent/daemon/services/plugininventory"
)

const (
	pluginCheckTimeout   = 30 * time.Second
	pluginDownloadBudget = 25 * time.Second
	pluginTargetTimeout  = 10 * time.Second
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
	// Reserve time for reading native metadata after network checks finish.
	downloadCtx, cancelDownloads := context.WithTimeout(ctx, pluginDownloadBudget)
	refreshErr := refreshPluginTargets(downloadCtx, targets, pluginTargetTimeout)
	cancelDownloads()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if refreshErr != nil {
		return nil, refreshErr
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
func refreshPluginTargets(ctx context.Context, targets []string, targetTimeout time.Duration) error {
	var firstFailure error
	checked := false
	for _, target := range targets {
		if ctx.Err() != nil {
			break
		}
		if !pluginCheckFilename.MatchString(target) {
			continue
		}
		// Match the native checkall command's serial execution of plugin hooks.
		// A rejected candidate still counts as a successful native check when
		// the manager publishes a validation marker and exits successfully.
		checkCtx, cancel := context.WithTimeout(ctx, targetTimeout)
		_, err := pluginCheckExec(checkCtx, constants.PluginBin, "check", target)
		cancel()
		if err != nil {
			logger.Debug("Plugin: refresh failed for %q: %v", target, err)
			if firstFailure == nil {
				firstFailure = fmt.Errorf("check %q: %w", target, err)
			}
			continue
		}
		checked = true
	}
	if checked {
		return nil
	}
	if firstFailure != nil {
		return fmt.Errorf("plugin refresh failed: %w", firstFailure)
	}
	return ctx.Err()
}
