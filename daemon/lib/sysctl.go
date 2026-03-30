package lib

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// procSysPath is the base path for sysctl parameters via procfs.
// Exposed as var so tests can override it.
var procSysPath = "/proc/sys"

// ReadSysctl reads a sysctl parameter value as a trimmed string.
// The key uses dot notation (e.g. "vm.dirty_ratio") which maps to
// /proc/sys/vm/dirty_ratio.
func ReadSysctl(key string) (string, error) {
	path, err := sysctlKeyToPath(key)
	if err != nil {
		return "", fmt.Errorf("invalid sysctl key %q: %w", key, err)
	}
	data, err := os.ReadFile(path) // #nosec G304 -- path validated by sysctlKeyToPath
	if err != nil {
		return "", fmt.Errorf("reading sysctl %s: %w", key, err)
	}
	return strings.TrimSpace(string(data)), nil
}

// ReadSysctlInt reads a sysctl parameter as an integer.
func ReadSysctlInt(key string) (int, error) {
	val, err := ReadSysctl(key)
	if err != nil {
		return 0, err
	}
	n, err := strconv.Atoi(val)
	if err != nil {
		return 0, fmt.Errorf("parsing sysctl %s value %q: %w", key, val, err)
	}
	return n, nil
}

// WriteSysctl writes a value to a sysctl parameter.
// The key uses dot notation (e.g. "vm.dirty_ratio").
func WriteSysctl(key, value string) error {
	path, err := sysctlKeyToPath(key)
	if err != nil {
		return fmt.Errorf("invalid sysctl key %q: %w", key, err)
	}
	// #nosec G306 -- /proc/sys files require specific permissions.
	if err := os.WriteFile(path, []byte(value), 0o644); err != nil {
		return fmt.Errorf("writing sysctl %s: %w", key, err)
	}
	return nil
}

// sysctlKeyToPath converts a dot-notation sysctl key to its /proc/sys path.
// Returns an error if the key contains path traversal or invalid segments.
func sysctlKeyToPath(key string) (string, error) {
	if key == "" {
		return "", fmt.Errorf("empty sysctl key")
	}

	segments := strings.Split(key, ".")
	for _, seg := range segments {
		if seg == "" || seg == ".." || strings.ContainsAny(seg, "/\\") {
			return "", fmt.Errorf("invalid sysctl key segment %q", seg)
		}
	}

	path := filepath.Join(procSysPath, filepath.Join(segments...))
	if !strings.HasPrefix(filepath.Clean(path), filepath.Clean(procSysPath)) {
		return "", fmt.Errorf("sysctl path escapes base directory")
	}
	return path, nil
}

// DiskCacheSettings contains Linux VM dirty page writeback parameters.
type DiskCacheSettings struct {
	DirtyBackgroundRatio int `json:"dirty_background_ratio"`
	DirtyRatio           int `json:"dirty_ratio"`
	DirtyWritebackCenti  int `json:"dirty_writeback_centisecs"`
	DirtyExpireCenti     int `json:"dirty_expire_centisecs"`
}

// ReadDiskCacheSettings reads all vm.dirty_* kernel parameters.
func ReadDiskCacheSettings() (*DiskCacheSettings, error) {
	bgRatio, err := ReadSysctlInt("vm.dirty_background_ratio")
	if err != nil {
		return nil, fmt.Errorf("reading disk cache settings: %w", err)
	}
	ratio, err := ReadSysctlInt("vm.dirty_ratio")
	if err != nil {
		return nil, fmt.Errorf("reading disk cache settings: %w", err)
	}
	writeback, err := ReadSysctlInt("vm.dirty_writeback_centisecs")
	if err != nil {
		return nil, fmt.Errorf("reading disk cache settings: %w", err)
	}
	expire, err := ReadSysctlInt("vm.dirty_expire_centisecs")
	if err != nil {
		return nil, fmt.Errorf("reading disk cache settings: %w", err)
	}
	return &DiskCacheSettings{
		DirtyBackgroundRatio: bgRatio,
		DirtyRatio:           ratio,
		DirtyWritebackCenti:  writeback,
		DirtyExpireCenti:     expire,
	}, nil
}

// InotifySettings contains Linux inotify limits and usage.
type InotifySettings struct {
	MaxUserWatches   int `json:"max_user_watches"`
	MaxUserInstances int `json:"max_user_instances"`
	MaxQueuedEvents  int `json:"max_queued_events"`
}

// ReadInotifySettings reads inotify limits from /proc/sys/fs/inotify.
func ReadInotifySettings() (*InotifySettings, error) {
	watches, err := ReadSysctlInt("fs.inotify.max_user_watches")
	if err != nil {
		return nil, fmt.Errorf("reading inotify settings: %w", err)
	}
	instances, err := ReadSysctlInt("fs.inotify.max_user_instances")
	if err != nil {
		return nil, fmt.Errorf("reading inotify settings: %w", err)
	}
	events, err := ReadSysctlInt("fs.inotify.max_queued_events")
	if err != nil {
		return nil, fmt.Errorf("reading inotify settings: %w", err)
	}
	return &InotifySettings{
		MaxUserWatches:   watches,
		MaxUserInstances: instances,
		MaxQueuedEvents:  events,
	}, nil
}
