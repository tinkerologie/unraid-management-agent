package services

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/tinkerologie/unraid-management-agent/daemon/constants"
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
	case "array", "disk":
		return []eventSource{
			collectors.NewFileEventSource(fileEventDebounce, constants.VarIni),
			disksIniChanges(),
		}
	case "shares":
		return []eventSource{iniChanges(constants.SharesIni, sharesIniVolatileKeys...)}
	case "mover":
		return []eventSource{collectors.NewFileEventSource(fileEventDebounce, constants.VarIni)}
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
		return []eventSource{collectors.NewDirEventSource(pluginEventDebounce, constants.PluginsConfigDir, "*.plg")}
	case "docker":
		return []eventSource{collectors.NewDockerEventSource()}
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
// content, without the ignored INI keys, changed since the last trigger.
type contentChanges struct {
	src    eventSource
	file   string
	ignore []string

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
