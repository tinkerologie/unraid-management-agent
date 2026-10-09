package controllers

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/constants"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/lib"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
)

// pluginCheckTimeout bounds the `plugin check` command, which downloads update
// metadata for every installed plugin. On networks without outbound internet
// access (issue #123) the command hangs, so it must fail fast; locally cached
// update files are still read afterwards.
const pluginCheckTimeout = 30 * time.Second

// pluginCheckExec runs the bounded update-metadata download; a variable so
// tests can stub it.
var pluginCheckExec = lib.ExecCommandOutputWithContext

// pluginsConfigDir holds the installed .plg files; a variable so tests can
// point it at a temporary directory.
var pluginsConfigDir = constants.PluginsConfigDir

// pluginNameEntityRe matches the "name" entity in a .plg file header, which is
// the name GET /plugins reports for the plugin.
var pluginNameEntityRe = regexp.MustCompile(`<!ENTITY\s+name\s+"([^"]*)"`)

// PluginController provides operations for managing Unraid plugins.
type PluginController struct {
	execOutput func(string, ...string) (string, error)
}

// NewPluginController creates a new plugin controller.
func NewPluginController() *PluginController {
	return &PluginController{
		execOutput: lib.ExecCommandOutput,
	}
}

// NewPluginControllerWithExec creates a new plugin controller with a custom command output executor.
func NewPluginControllerWithExec(execFn func(string, ...string) (string, error)) *PluginController {
	return &PluginController{
		execOutput: execFn,
	}
}

// SetExec sets a custom command output executor for the plugin controller.
func (pc *PluginController) SetExec(execFn func(string, ...string) (string, error)) {
	pc.execOutput = execFn
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

// UpdatePlugin updates a specific plugin.
func (pc *PluginController) UpdatePlugin(pluginName string) error {
	logger.Info("Plugin: Updating plugin %s", pluginName)

	baseName := strings.TrimSuffix(filepath.Base(pluginName), ".plg")
	bareFile := resolvePluginFile(baseName)

	execFn := pc.execOutput
	if execFn == nil {
		execFn = lib.ExecCommandOutput
	}

	// Use the plugin update command with bare .plg filename
	output, err := execFn(constants.PluginBin, "update", bareFile)
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
	updatesAvailable, err := pc.CheckPluginUpdates(context.Background())
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

// resolvePluginFile returns the bare .plg file name for a plugin. GET /plugins
// names a plugin by the "name" entity in its .plg file, which can differ from
// the file name (e.g. "disklocation" is installed as disklocation-master.plg),
// so when no file has the given name, the file whose name entity matches is
// used instead. Falls back to "<baseName>.plg".
func resolvePluginFile(baseName string) string {
	fileName := baseName + ".plg"
	if _, err := os.Stat(filepath.Join(pluginsConfigDir, fileName)); err == nil {
		return fileName
	}
	// Glob only fails on a malformed pattern, and this one is fixed.
	files, _ := filepath.Glob(filepath.Join(pluginsConfigDir, "*.plg"))
	for _, file := range files {
		if pluginEntityName(file) == baseName {
			logger.Info("Plugin: %s is installed as %s", baseName, filepath.Base(file))
			return filepath.Base(file)
		}
	}
	return fileName
}

// pluginEntityName returns the "name" entity declared in a .plg file header,
// or "" when the file cannot be read or declares none. Like the plugin list
// parser, it stops at the end of the DOCTYPE section.
func pluginEntityName(path string) string {
	// #nosec G304 -- path comes from globbing the trusted plugin directory.
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close() //nolint:errcheck // Error checking not needed for defer Close

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if matches := pluginNameEntityRe.FindStringSubmatch(line); matches != nil {
			return matches[1]
		}
		if strings.Contains(line, "]>") {
			break
		}
	}
	return ""
}

// getPluginVersion extracts the version from a .plg file by parsing XML entities.
func getPluginVersion(path string) string {
	lines, err := lib.ExecCommand("/bin/grep", "-oP", `<!ENTITY\s+version\s+"\K[^"]+`, path)
	if err != nil || len(lines) == 0 {
		return ""
	}
	return strings.TrimSpace(lines[0])
}
