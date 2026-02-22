package controllers

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/constants"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/lib"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
)

// PluginController provides operations for managing Unraid plugins.
type PluginController struct{}

// NewPluginController creates a new plugin controller.
func NewPluginController() *PluginController {
	return &PluginController{}
}

// CheckPluginUpdates checks all plugins for available updates.
func (pc *PluginController) CheckPluginUpdates() ([]dto.PluginInfo, error) {
	logger.Info("Plugin: Checking for plugin updates")

	// Run the plugin check command to download update info
	_, err := lib.ExecCommandWithTimeout(
		120*time.Second,
		constants.PluginBin, "check",
	)
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

// UpdatePlugin updates a specific plugin.
func (pc *PluginController) UpdatePlugin(pluginName string) error {
	logger.Info("Plugin: Updating plugin %s", pluginName)

	pluginFile := filepath.Join(constants.PluginsConfigDir, pluginName+".plg")

	// Use the plugin update command
	output, err := lib.ExecCommandOutput(constants.PluginBin, "update", pluginFile)
	if err != nil {
		return fmt.Errorf("failed to update plugin %s: %w (output: %s)", pluginName, err, output)
	}

	logger.Info("Plugin: Successfully updated %s", pluginName)
	return nil
}

// UpdateAllPlugins updates all plugins that have updates available.
func (pc *PluginController) UpdateAllPlugins() ([]dto.PluginUpdateResult, error) {
	logger.Info("Plugin: Updating all plugins with available updates")

	// First check for updates
	updatesAvailable, err := pc.CheckPluginUpdates()
	if err != nil {
		return nil, fmt.Errorf("failed to check for updates: %w", err)
	}

	if len(updatesAvailable) == 0 {
		logger.Info("Plugin: No updates available")
		return nil, nil
	}

	var results []dto.PluginUpdateResult
	for _, plugin := range updatesAvailable {
		result := dto.PluginUpdateResult{
			PluginName:      plugin.Name,
			PreviousVersion: plugin.Version,
			NewVersion:      plugin.LatestVersion,
			Timestamp:       time.Now(),
		}

		err := pc.UpdatePlugin(plugin.Name)
		if err != nil {
			result.Success = false
			result.Message = fmt.Sprintf("Failed to update: %v", err)
			logger.Error("Plugin: Failed to update %s: %v", plugin.Name, err)
		} else {
			result.Success = true
			result.Message = "Updated successfully"
		}

		results = append(results, result)
	}

	logger.Info("Plugin: Update complete (%d plugins processed)", len(results))
	return results, nil
}

// getPluginVersion extracts the version from a .plg file by parsing XML entities.
func getPluginVersion(path string) string {
	lines, err := lib.ExecCommand("/bin/grep", "-oP", `<!ENTITY\s+version\s+"\K[^"]+`, path)
	if err != nil || len(lines) == 0 {
		return ""
	}
	return strings.TrimSpace(lines[0])
}
