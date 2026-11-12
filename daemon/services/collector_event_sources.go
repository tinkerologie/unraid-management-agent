package services

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"gopkg.in/ini.v1"

	"github.com/tinkerologie/unraid-management-agent/daemon/constants"
	"github.com/tinkerologie/unraid-management-agent/daemon/logger"
	"github.com/tinkerologie/unraid-management-agent/daemon/services/collectors"
)

// Debounce windows of the file-based change sources. Plugin installs write
// several files, so they get a longer window.
const (
	fileEventDebounce   = 500 * time.Millisecond
	pluginEventDebounce = 5 * time.Second
)

// defaultEventSources returns the change-event sources that trigger a
// collector in Dynamic mode. Each event-driven collector also keeps its
// safety-net poll, because events can be missed.
func defaultEventSources(name string) []eventSource {
	switch name {
	case "array":
		return []eventSource{arrayStateChanges(), disksIniChanges()}
	case "disk":
		return []eventSource{disksIniChanges()}
	case "shares":
		return []eventSource{iniChanges(constants.SharesIni, sharesIniVolatileKeys...)}
	case "mover":
		return []eventSource{
			collectors.NewFileEventSource(fileEventDebounce, constants.VarIni),
			collectors.NewFileEventSource(fileEventDebounce, constants.MoverPID),
			collectors.NewFileEventSource(fileEventDebounce, constants.MoverProgress),
		}
	case "notification":
		unread, archive := collectors.ResolveNotificationDirs(constants.DynamixCfg)
		return []eventSource{
			collectors.NewDirEventSource(fileEventDebounce, unread, ""),
			collectors.NewDirEventSource(fileEventDebounce, archive, ""),
		}
	case "unassigned":
		return []eventSource{
			disksIniChanges(),
			collectors.NewDirEventSource(fileEventDebounce, filepath.Dir(constants.UnassignedSambaMountCfg), ""),
			collectors.NewUeventSource(collectors.BlockDeviceUevent),
		}
	case "plugin_update":
		return []eventSource{collectors.NewDirEventSource(pluginEventDebounce, constants.PluginsRuntimeDir, "*.plg")}
	case "docker":
		return []eventSource{collectors.NewDockerEventSource()}
	case "docker_networks":
		return []eventSource{collectors.NewDockerNetworkEventSource()}
	case "vm":
		return []eventSource{collectors.NewLibvirtEventSource()}
	case "psu":
		return []eventSource{collectors.NewUeventSource(collectors.PSUDeviceUevent)}
	case "hardware":
		return []eventSource{collectors.NewUeventSource(collectors.HardwareDeviceUevent)}
	default:
		return nil
	}
}

// The keys emhttp rewrites disks.ini (every few seconds) and shares.ini
// (about every 30 s) to update: I/O counters, temperatures and space usage.
// Changes to them alone do not trigger a collection; the active and
// safety-net polls pick them up.
var (
	disksIniVolatileKeys  = []string{"numReads", "numWrites", "temp", "fsFree", "fsUsed"}
	sharesIniVolatileKeys = []string{"free", "used"}
)

// disksIniChanges triggers on disks.ini state changes, not its counters.
func disksIniChanges() eventSource {
	return iniChanges(constants.DisksIni, disksIniVolatileKeys...)
}

// iniChanges triggers when the INI file changes in any key but the ignored ones.
func iniChanges(file string, ignore ...string) eventSource {
	return &contentChanges{
		src:    collectors.NewFileEventSource(fileEventDebounce, file),
		file:   file,
		ignore: ignore,
	}
}

// contentChanges passes on its source's triggers only when the file's
// content changed since the last trigger, after projecting it or filtering
// the ignored INI keys.
type contentChanges struct {
	src     eventSource
	file    string
	ignore  []string
	project func([]byte) []byte

	mu   sync.Mutex
	last [sha256.Size]byte
}

// Run runs the source until ctx is cancelled.
func (c *contentChanges) Run(ctx context.Context, trigger func()) {
	c.mu.Lock()
	c.last = c.digest()
	c.mu.Unlock()
	c.src.Run(ctx, func() {
		c.mu.Lock()
		d := c.digest()
		changed := d != c.last
		c.last = d
		c.mu.Unlock()
		if changed {
			trigger()
		}
	})
}

// digest hashes the file without the ignored keys; an unreadable file hashes
// as empty, so it changing to or from unreadable is a change.
func (c *contentChanges) digest() [sha256.Size]byte {
	data, err := os.ReadFile(c.file)
	if err != nil {
		return sha256.Sum256(nil)
	}
	if c.project != nil {
		return sha256.Sum256(c.project(data))
	}
	var kept bytes.Buffer
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		line := sc.Text()
		key, _, _ := strings.Cut(line, "=")
		if !slices.Contains(c.ignore, strings.TrimSpace(key)) {
			kept.WriteString(line)
			kept.WriteByte('\n')
		}
	}
	return sha256.Sum256(kept.Bytes())
}

func arrayStateChanges() eventSource {
	return &contentChanges{
		src:     collectors.NewFileEventSource(fileEventDebounce, constants.VarIni),
		file:    constants.VarIni,
		project: arrayStateProjection,
	}
}

// Progress counters are sampled by periodic collection. File events carry
// changes to state and parity validity, including pause, resume and finish.
func arrayStateProjection(data []byte) []byte {
	cfg, err := ini.Load(data)
	if err != nil {
		return nil
	}
	section := cfg.Section("")
	var state strings.Builder
	for _, key := range []string{"mdState", "mdNumDisks", "mdNumInvalid", "sbSynced"} {
		state.WriteString(section.Key(key).String())
		state.WriteByte(0)
	}
	if count, _ := strconv.Atoi(section.Key("sbSyncErrs").String()); count > 0 {
		state.WriteByte(1)
	}
	state.WriteString(collectors.ArrayOperationStatus(section))
	return []byte(state.String())
}

type eventSourceFunc func(context.Context, func())

func (f eventSourceFunc) Run(ctx context.Context, trigger func()) { f(ctx, trigger) }

// manualEventSources keeps the remaining legacy triggers. Array and disk
// collection honor their configured interval, including during native progress rewrites.
func manualEventSources(name string, coll Collector) []eventSource {
	switch name {
	case "shares":
		return []eventSource{manualFileEvents(constants.SharesIni)}
	case "notification":
		if notification, ok := coll.(*collectors.NotificationCollector); ok {
			return []eventSource{eventSourceFunc(notification.ManualEvents)}
		}
	case "psu":
		return []eventSource{collectors.NewUeventSource(collectors.PSUDeviceUevent)}
	}
	return nil
}

func manualFileEvents(paths ...string) eventSource {
	return eventSourceFunc(func(ctx context.Context, trigger func()) {
		w, err := collectors.NewFileWatcher(fileEventDebounce)
		if err != nil {
			logger.Warning("Manual collector watcher: %v", err)
			return
		}
		defer func() { _ = w.Close() }()
		for _, path := range paths {
			if err := w.WatchFile(path); err != nil {
				logger.Warning("Manual collector watcher cannot watch %s: %v", path, err)
			}
		}
		w.Run(ctx, paths, trigger)
	})
}
