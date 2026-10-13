package controllers

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/tinkerologie/unraid-management-agent/daemon/constants"
	"github.com/tinkerologie/unraid-management-agent/daemon/dto"
	"github.com/tinkerologie/unraid-management-agent/daemon/lib"
	"github.com/tinkerologie/unraid-management-agent/daemon/logger"
)

// pluginCheckTimeout bounds the `plugin check` command, which downloads update
// metadata for every installed plugin. On networks without outbound internet
// access (issue #123) the command hangs, so it must fail fast; locally cached
// update files are still read afterwards.
const pluginCheckTimeout = 30 * time.Second

// pluginCheckExec runs the bounded update-metadata download; a variable so
// tests can stub it.
var pluginCheckExec = lib.ExecCommandOutputWithContext

// PluginController provides read-only plugin update checks. It never installs
// or updates plugins; users apply updates from Unraid's Plugins page (#244).
type PluginController struct{}

// NewPluginController creates a new plugin controller.
func NewPluginController() *PluginController {
	return &PluginController{}
}

// CheckPluginUpdates checks all plugins for available updates. The check is
// bounded by both the caller's context (cancellable on shutdown) and
// pluginCheckTimeout.
func (pc *PluginController) CheckPluginUpdates(parentCtx context.Context) ([]dto.PluginInfo, error) {
	logger.Info("Plugin: Checking for plugin updates")

	ctx, cancel := context.WithTimeout(parentCtx, pluginCheckTimeout)
	defer cancel()

	// Download update info for every installed plugin into /tmp/plugins.
	// "check" needs a plugin file argument (without one it only prints usage
	// and exits 1); "checkall" checks all installed plugins.
	_, err := pluginCheckExec(ctx, constants.PluginBin, "checkall")
	if err != nil {
		logger.Warning("Plugin: Check command returned error (may be normal): %v", err)
	}

	// Now read the plugin list to find which have updates
	pluginFiles, err := filepath.Glob(filepath.Join(constants.PluginsConfigDir, "*.plg"))
	if err != nil {
		return nil, fmt.Errorf("failed to list plugin files: %w", err)
	}

	var updatesAvailable []dto.PluginInfo
	for _, pluginFile := range pluginFiles {
		pluginName := strings.TrimSuffix(filepath.Base(pluginFile), ".plg")

		// Check if an update file exists in /tmp/plugins/
		updateFile := filepath.Join(constants.PluginsTempDir, filepath.Base(pluginFile))
		installedVersion := getPluginVersion(pluginFile)
		updateVersion := getPluginVersion(updateFile)

		if updateVersion != "" && updateVersion != installedVersion {
			updatesAvailable = append(updatesAvailable, dto.PluginInfo{
				Name:            pluginName,
				Version:         installedVersion,
				UpdateAvailable: true,
				LatestVersion:   updateVersion,
			})
		}
	}

	logger.Info("Plugin: Found %d plugins with updates available", len(updatesAvailable))
	return updatesAvailable, nil
}

// getPluginVersion extracts the version from a .plg file by parsing XML entities.
func getPluginVersion(path string) string {
	lines, err := lib.ExecCommand("/bin/grep", "-oP", `<!ENTITY\s+version\s+"\K[^"]+`, path)
	if err != nil || len(lines) == 0 {
		return ""
	}
	return strings.TrimSpace(lines[0])
}
