package collectors

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/constants"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
)

// osUpdateStartupStagger delays the first check so it does not pile onto boot
// alongside every other collector.
const osUpdateStartupStagger = 60 * time.Second

// osUpdateCandidatePaths is the ordered list of local files that may contain
// the "latest available" Unraid OS version.  Tests override this package-level
// variable to point at fixture files without touching the real filesystem.
//
// Supported file formats (one per file):
//   - JSON (Unraid unraidcheck): {"version":"7.3.2","isNewer":true}
//   - INI key-value:            version=7.2.1  (quoted or unquoted)
//   - Plain text:               7.2.1
//
// Candidate order reflects decreasing reliability:
//  1. /tmp/unraidcheck/result.json — written by the Unraid update-check cron job
//  2. /tmp/unraidcheck/result      — legacy format written by older scripts
//  3. /var/local/emhttp/update.ini — runtime update metadata exposed by emhttp
var osUpdateCandidatePaths = []string{
	"/tmp/unraidcheck/result.json",
	"/tmp/unraidcheck/result",
	"/var/local/emhttp/update.ini",
}

// osCurrentVersionPath is the primary path for reading the running version.
// Overridable in tests.
var osCurrentVersionPath = "/etc/unraid-version"

// OSUpdateCollector periodically checks whether a newer Unraid OS version is
// available by reading local files only.  It never makes outbound network calls.
type OSUpdateCollector struct {
	appCtx *domain.Context

	// CheckFn performs the actual check. The constructor installs a default
	// implementation; callers may replace it for testing.
	CheckFn func() (*dto.OSUpdateStatus, error)

	// NotifyFn is called (once) when the status transitions to update_available.
	// Injected by the collector factory in package services to avoid a
	// collectors→controllers import cycle.
	NotifyFn func(latest string)

	lastSig       string
	prevAvailable bool
	baselineSet   bool
}

// NewOSUpdateCollector creates a new OSUpdateCollector with the default
// local-file CheckFn pre-installed.
func NewOSUpdateCollector(ctx *domain.Context) *OSUpdateCollector {
	c := &OSUpdateCollector{appCtx: ctx}
	c.CheckFn = c.defaultCheck
	return c
}

// Start begins the periodic OS update check after a startup stagger.
func (c *OSUpdateCollector) Start(ctx context.Context, interval time.Duration) {
	logger.Info("Starting os_update collector (interval: %v)", interval)

	select {
	case <-ctx.Done():
		return
	case <-time.After(osUpdateStartupStagger):
	}

	func() {
		defer func() {
			if r := recover(); r != nil {
				logger.LogPanicWithStack("OSUpdate collector", r)
			}
		}()
		c.Collect()
	}()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info("OSUpdate collector stopping due to context cancellation")
			return
		case <-ticker.C:
			func() {
				defer func() {
					if r := recover(); r != nil {
						logger.LogPanicWithStack("OSUpdate collector", r)
					}
				}()
				c.Collect()
			}()
		}
	}
}

// Collect runs an OS update check and publishes the result only if it changed
// since the last publish (dedupe to avoid no-op WebSocket broadcasts).
func (c *OSUpdateCollector) Collect() {
	if c.CheckFn == nil {
		logger.Warning("OSUpdate: CheckFn not set, skipping collect")
		return
	}

	result, err := c.CheckFn()
	if err != nil {
		logger.Warning("OSUpdate: check failed: %v", err)
		return
	}
	if result == nil {
		return
	}

	// Notification: fire only on the first transition to update_available.
	if c.NotifyFn != nil {
		if !c.baselineSet {
			// First run establishes the baseline; do not notify.
			c.prevAvailable = result.UpdateAvailable
			c.baselineSet = true
		} else if result.UpdateAvailable && !c.prevAvailable {
			// Newly became available since last run.
			c.NotifyFn(result.LatestVersion)
			c.prevAvailable = true
		} else {
			c.prevAvailable = result.UpdateAvailable
		}
	}

	sig := result.CurrentVersion + "|" + result.LatestVersion + "|" + result.Status
	if sig == c.lastSig {
		logger.Debug("OSUpdate: no change (status=%s), skipping publish", result.Status)
		return
	}
	c.lastSig = sig

	domain.Publish(c.appCtx.Hub, constants.TopicOSUpdateUpdate, result)
	logger.Info("OSUpdate: published (status=%s, current=%s, latest=%s)",
		result.Status, result.CurrentVersion, result.LatestVersion)
}

type localOSUpdateInfo struct {
	Version string
	IsNewer *bool
}

// defaultCheck implements the local-file-only OS update check.
func (c *OSUpdateCollector) defaultCheck() (*dto.OSUpdateStatus, error) {
	current := readCurrentOSVersion()
	info, found := readLocalLatestVersion()

	result := &dto.OSUpdateStatus{
		CurrentVersion: current,
		Timestamp:      time.Now(),
	}

	if !found || info == nil || info.Version == "" {
		result.Status = dto.OSUpdateStatusUnknown
		result.UpdateAvailable = false
		return result, nil
	}

	result.LatestVersion = info.Version
	if info.IsNewer != nil {
		result.UpdateAvailable = *info.IsNewer
		if *info.IsNewer {
			result.Status = dto.OSUpdateStatusAvailable
		} else {
			result.Status = dto.OSUpdateStatusUpToDate
		}
	} else if isVersionNewer(info.Version, current) {
		result.UpdateAvailable = true
		result.Status = dto.OSUpdateStatusAvailable
	} else {
		result.UpdateAvailable = false
		result.Status = dto.OSUpdateStatusUpToDate
	}

	return result, nil
}

// readCurrentOSVersion reads the running Unraid OS version from the local
// filesystem.  It mirrors the logic in SystemCollector.getUnraidVersion().
func readCurrentOSVersion() string {
	// Primary: /etc/unraid-version  (format: version="7.2.0" or 7.2.0)
	if data, err := os.ReadFile(osCurrentVersionPath); err == nil {
		content := strings.TrimSpace(string(data))
		if after, ok := strings.CutPrefix(content, "version="); ok {
			return strings.Trim(after, `"`)
		}
		return content
	}

	// Fallback: /var/local/emhttp/var.ini
	if data, err := os.ReadFile(constants.VarIni); err == nil {
		for line := range strings.SplitSeq(string(data), "\n") {
			line = strings.TrimSpace(line)
			if after, ok := strings.CutPrefix(line, "version="); ok {
				return strings.Trim(after, `"`)
			}
		}
	}

	return ""
}

// readLocalLatestVersion iterates osUpdateCandidatePaths and returns the first
// parseable version info it finds.  Returns (nil, false) if none are available.
func readLocalLatestVersion() (*localOSUpdateInfo, bool) {
	for _, path := range osUpdateCandidatePaths {
		if info, ok := parseVersionFile(path); ok {
			return info, true
		}
	}
	return nil, false
}

// parseVersionFile reads a single candidate file and extracts version information.
// Supported formats:
//   - JSON:          {"version": "7.3.2", "isNewer": true}
//   - INI key-value: version=7.2.1  (quoted or unquoted)
//   - VERSION=7.2.1
//   - Plain text:    7.2.1
func parseVersionFile(path string) (*localOSUpdateInfo, bool) {
	// #nosec G304 -- path comes from the package-level osUpdateCandidatePaths
	// variable which is only overridden in tests using safe temp-dir paths.
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}

	trimmed := strings.TrimSpace(string(data))
	if strings.HasPrefix(trimmed, "{") {
		var jsonResult struct {
			Version string `json:"version"`
			IsNewer *bool  `json:"isNewer"`
		}
		if err := json.Unmarshal([]byte(trimmed), &jsonResult); err == nil && jsonResult.Version != "" {
			return &localOSUpdateInfo{
				Version: jsonResult.Version,
				IsNewer: jsonResult.IsNewer,
			}, true
		}
	}

	scanner := bufio.NewScanner(strings.NewReader(trimmed))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Try key=value forms: version= or VERSION=
		lower := strings.ToLower(line)
		if strings.HasPrefix(lower, "version=") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				v := strings.Trim(strings.TrimSpace(parts[1]), `"`)
				if v != "" {
					return &localOSUpdateInfo{Version: v}, true
				}
			}
			continue
		}

		// Plain version string (e.g. "7.2.1")
		if looksLikeVersion(line) {
			return &localOSUpdateInfo{Version: line}, true
		}
	}

	return nil, false
}

// looksLikeVersion returns true if s matches a simple N.N.N-style version token.
// It is intentionally lenient — the goal is to avoid treating full sentences as
// versions, not to validate semver strictly.
func looksLikeVersion(s string) bool {
	if s == "" || len(s) > 32 {
		return false
	}
	// Must start with a digit
	if s[0] < '0' || s[0] > '9' {
		return false
	}
	// Must contain at least one dot
	return strings.ContainsRune(s, '.')
}

// isVersionNewer compares semver-like version strings (e.g. "7.2.1" vs "7.2.0").
// Returns true if latest is strictly newer than current.
func isVersionNewer(latest, current string) bool {
	latest = strings.TrimPrefix(strings.TrimSpace(latest), "v")
	current = strings.TrimPrefix(strings.TrimSpace(current), "v")
	if latest == "" || latest == current {
		return false
	}
	if current == "" {
		return true
	}

	latestBase, _, _ := strings.Cut(latest, "-")
	currentBase, _, _ := strings.Cut(current, "-")

	lParts := strings.Split(latestBase, ".")
	cParts := strings.Split(currentBase, ".")

	maxLen := max(len(lParts), len(cParts))
	for i := range maxLen {
		var lNum, cNum int
		if i < len(lParts) {
			_, _ = fmt.Sscanf(lParts[i], "%d", &lNum)
		}
		if i < len(cParts) {
			_, _ = fmt.Sscanf(cParts[i], "%d", &cNum)
		}
		if lNum > cNum {
			return true
		}
		if lNum < cNum {
			return false
		}
	}
	return false
}
