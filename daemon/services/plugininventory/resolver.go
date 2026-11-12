// Package plugininventory reads the installed user-plugin state used by Unraid.
package plugininventory

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/tinkerologie/unraid-management-agent/daemon/constants"
	"github.com/tinkerologie/unraid-management-agent/daemon/dto"
	"github.com/tinkerologie/unraid-management-agent/daemon/logger"
)

const (
	updateAvailable    = "update_available"
	updateIncompatible = "update_incompatible"
	validationFailed   = "validation_failed"
	statusCurrent      = "up_to_date"
	needCheck          = "need_check"
	notAvailable       = "not_available"
)

// Resolver reads Plugin Manager state without installing, removing or writing files.
type Resolver struct {
	RuntimeDir    string
	CandidateDir  string
	OSVersionPath string
	Now           func() time.Time
	changeTime    func(os.FileInfo) time.Time
}

// NewResolver returns a resolver for the running Unraid system.
func NewResolver() Resolver {
	return Resolver{
		RuntimeDir: constants.PluginsRuntimeDir, CandidateDir: constants.PluginsTempDir,
		OSVersionPath: constants.UnraidVersionFile, Now: time.Now, changeTime: fileChangeTime,
	}
}

// Resolve reads the installed inventory and each plugin's current update status.
func (r Resolver) Resolve(ctx context.Context) (*dto.PluginList, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	now := r.Now()
	list := &dto.PluginList{Plugins: []dto.PluginInfo{}, Timestamp: now}
	installed, err := r.installed(ctx)
	if err != nil {
		return nil, err
	}
	osVersion := r.osVersion()
	for _, item := range installed {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		plugin := dto.PluginInfo{
			Name: item.Name, Author: item.Author, Version: item.Version, URL: item.URL,
			SupportURL: item.Support, Icon: item.Icon, Enabled: true, UpdateStatus: notAvailable,
		}
		r.resolveUpdate(&plugin, item.target, osVersion, now)
		plugin.UpdateAvailable = plugin.UpdateStatus == updateAvailable
		if plugin.UpdateAvailable {
			list.UpdatesAvailable++
		}
		list.Plugins = append(list.Plugins, plugin)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	slices.SortFunc(list.Plugins, func(a, b dto.PluginInfo) int { return cmp.Compare(a.Name, b.Name) })
	list.TotalCount = len(list.Plugins)
	return list, nil
}

// CheckTargets returns registered user-plugin filenames with an update URL.
func (r Resolver) CheckTargets(ctx context.Context) ([]string, error) {
	installed, err := r.installed(ctx)
	if err != nil {
		return nil, err
	}
	var targets []string
	for _, item := range installed {
		if item.URL != "" {
			targets = append(targets, item.link)
		}
	}
	return targets, nil
}

type installedManifest struct {
	manifest
	link   string
	target string
}

func (r Resolver) installed(ctx context.Context) ([]installedManifest, error) {
	entries, err := os.ReadDir(r.RuntimeDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read runtime plugin inventory: %w", err)
	}
	installed := make([]installedManifest, 0, len(entries))
	seen := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if entry.Type()&os.ModeSymlink == 0 || filepath.Ext(entry.Name()) != ".plg" || isBuiltIn(entry.Name()) {
			continue
		}
		runtimePath := filepath.Join(r.RuntimeDir, entry.Name())
		target, err := os.Readlink(runtimePath)
		if err != nil || isBuiltIn(filepath.Base(target)) {
			continue
		}
		item, _, err := parseManifest(runtimePath)
		if err != nil {
			logger.Debug("Plugins: skipping unreadable runtime manifest %q", entry.Name())
			continue
		}
		target = filepath.Base(target)
		if item.Name == "" {
			item.Name = strings.TrimSuffix(target, ".plg")
		}
		if isBuiltIn(item.Name) {
			continue
		}
		if _, duplicate := seen[item.Name]; duplicate {
			continue
		}
		seen[item.Name] = struct{}{}
		installed = append(installed, installedManifest{manifest: item, link: entry.Name(), target: target})
	}
	return installed, nil
}

func isBuiltIn(name string) bool {
	name = strings.TrimSuffix(name, ".plg")
	return name == "unRAIDServer" || name == "unRAIDServer-"
}

func (r Resolver) resolveUpdate(plugin *dto.PluginInfo, target, osVersion string, now time.Time) {
	basename := filepath.Base(plugin.URL)
	if plugin.URL != "" && basename != "." && basename != ".." && basename != string(filepath.Separator) {
		candidate, info, err := parseManifest(filepath.Join(r.CandidateDir, basename))
		if err == nil {
			plugin.LatestVersion = candidate.Version
		}
		switch {
		case err == nil && plugin.Version != "" && candidate.Version > plugin.Version:
			plugin.UpdateStatus = updateIncompatible
			if compatible(candidate, osVersion) {
				plugin.UpdateStatus = updateAvailable
			}
		case err == nil && r.changeTime(info).After(now.Add(-24*time.Hour)):
			plugin.UpdateStatus = statusCurrent
		case info != nil:
			plugin.UpdateStatus = needCheck
		}
	}
	marker := readInvalidMarker(filepath.Join(r.CandidateDir, target+".invalid"))
	if marker.Version > plugin.Version {
		plugin.UpdateStatus = validationFailed
		plugin.LatestVersion = marker.Version
		for _, value := range []string{marker.Summary, marker.Reason, marker.Message} {
			if value = strings.TrimSpace(value); value != "" {
				plugin.ValidationMessage = value
				break
			}
		}
	}
}

func compatible(candidate manifest, osVersion string) bool {
	minimum, maximum := candidate.Min, candidate.Max
	if minimum == "" {
		minimum = "1.0"
	}
	if maximum == "" {
		maximum = "999.9.9"
	}
	return osVersion != "" && compareOSVersions(osVersion, minimum) >= 0 && compareOSVersions(osVersion, maximum) <= 0
}

func (r Resolver) osVersion() string {
	f, _, err := openRegular(r.OSVersionPath)
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, 1025))
	if err != nil || len(data) > 1024 {
		return ""
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		if value, ok := strings.CutPrefix(strings.TrimSpace(line), "version="); ok {
			return strings.Trim(strings.TrimSpace(value), "\"'")
		}
	}
	return ""
}

type invalidMarker struct {
	Version string `json:"version"`
	Summary string `json:"summary"`
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

func readInvalidMarker(path string) invalidMarker {
	f, _, err := openRegular(path)
	if err != nil {
		return invalidMarker{}
	}
	defer func() { _ = f.Close() }()
	const markerLimit = 16 << 10
	data, err := io.ReadAll(io.LimitReader(f, markerLimit+1))
	var marker invalidMarker
	if err != nil || len(data) > markerLimit || json.Unmarshal(data, &marker) != nil {
		return invalidMarker{}
	}
	return marker
}
