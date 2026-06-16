package lib

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

// Detection roots. var (not const) so tests can point them at temp dirs.
var (
	emhttpPluginsDir = "/usr/local/emhttp/plugins"
	flashPluginsDir  = "/boot/config/plugins"
	procDirPath      = "/proc"
)

// externalFanPlugin describes a known third-party fan controller and how to
// recognise it.
type externalFanPlugin struct {
	// name is the human-readable controller name surfaced to the user.
	name string
	// dir is the plugin directory name under both the emhttp plugins dir
	// (install marker) and the flash plugins config dir (.cfg files).
	dir string
	// procHint is a substring unique to the plugin's control scripts as they
	// appear in /proc/<pid>/cmdline.
	procHint string
}

// knownExternalFanPlugins lists the third-party fan controllers the agent
// defers to. Order is preserved in the reported controller list.
var knownExternalFanPlugins = []externalFanPlugin{
	{name: "FanCTRL Plus", dir: "fanctrlplus", procHint: "/plugins/fanctrlplus/"},
	{name: "Dynamix Auto Fan Control", dir: "dynamix.system.autofan", procHint: "/plugins/dynamix.system.autofan/"},
}

// DetectExternalFanControl reports whether a known third-party fan-control
// plugin is installed AND enabled. A plugin counts as enabled when any of its
// .cfg files has service="1" or one of its control processes is running.
//
// Detection is best-effort: unreadable files or proc entries are skipped and
// never fatal. When nothing is detected the returned status is inactive, which
// preserves the agent's normal behaviour.
func DetectExternalFanControl() dto.ExternalFanControl {
	var result dto.ExternalFanControl
	for _, p := range knownExternalFanPlugins {
		if !dirExists(filepath.Join(emhttpPluginsDir, p.dir)) {
			continue // not installed
		}
		if !pluginEnabledViaConfig(filepath.Join(flashPluginsDir, p.dir)) && !pluginProcessRunning(p.procHint) {
			continue // installed but not enabled
		}
		result.Active = true
		result.Controllers = append(result.Controllers, p.name)
	}
	return result
}

// dirExists reports whether path is an existing directory.
func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// pluginEnabledViaConfig reports whether any *.cfg in cfgDir has service="1".
func pluginEnabledViaConfig(cfgDir string) bool {
	matches, err := filepath.Glob(filepath.Join(cfgDir, "*.cfg"))
	if err != nil {
		return false
	}
	for _, cfg := range matches {
		if cfgServiceEnabled(cfg) {
			return true
		}
	}
	return false
}

// cfgServiceEnabled reports whether the cfg file contains a service="1" line.
func cfgServiceEnabled(path string) bool {
	// #nosec G304 -- path comes from a bounded glob of the plugins config dir.
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close() //nolint:errcheck

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "service=") {
			continue
		}
		val := strings.Trim(strings.TrimPrefix(line, "service="), "\"'")
		return val == "1"
	}
	return false
}

// pluginProcessRunning reports whether any process has hint in its cmdline.
func pluginProcessRunning(hint string) bool {
	entries, err := os.ReadDir(procDirPath)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if _, err := strconv.Atoi(entry.Name()); err != nil {
			continue // not a PID
		}
		// #nosec G304 -- path is /proc/<numeric-pid>/cmdline.
		raw, err := os.ReadFile(filepath.Join(procDirPath, entry.Name(), "cmdline"))
		if err != nil {
			continue
		}
		// cmdline is NUL-separated argv; normalise to spaces before matching.
		cmdline := strings.ReplaceAll(string(raw), "\x00", " ")
		if strings.Contains(cmdline, hint) {
			return true
		}
	}
	return false
}
