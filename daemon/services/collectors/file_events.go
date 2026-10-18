package collectors

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
)

// NewFileEventSource returns an EventSource that reports writes, creates,
// removes and renames of the given files.
func NewFileEventSource(debounce time.Duration, files ...string) EventSource {
	set := make(map[string]struct{}, len(files))
	var dirs []string
	for _, f := range files {
		abs := absPath(f)
		set[abs] = struct{}{}
		if dir := filepath.Dir(abs); !slices.Contains(dirs, dir) {
			dirs = append(dirs, dir)
		}
	}
	name := "File event source (" + strings.Join(files, ", ") + ")"
	return newFSEventSource(name, debounce, dirs, func(abs string, op fsnotify.Op) bool {
		_, watched := set[abs]
		return watched && isEntryChange(op)
	})
}

// NewDirEventSource returns an EventSource that reports creates, writes,
// removes and renames of the entries of dir whose name matches glob ("" matches
// all).
func NewDirEventSource(debounce time.Duration, dir, glob string) EventSource {
	dir = absPath(dir)
	name := "Directory event source (" + filepath.Join(dir, glob) + ")"
	return newFSEventSource(name, debounce, []string{dir}, func(abs string, op fsnotify.Op) bool {
		return dirEntryChanged(dir, glob, abs, op)
	})
}

// newFileWatcher creates the file event sources' watchers; tests replace it.
var newFileWatcher = NewFileWatcher

// newFSEventSource watches dirs and reports the events that match accepts. A
// missing directory is retried with backoff, and a session ends when one of
// dirs is removed so it is watched again once it reappears.
func newFSEventSource(name string, debounce time.Duration, dirs []string, accepts func(abs string, op fsnotify.Op) bool) *sessionSource {
	return newSessionSource(name, debounce, func(ctx context.Context, connected, changed func()) error {
		fw, err := newFileWatcher(0)
		if err != nil {
			return fmt.Errorf("creating watcher: %w", err)
		}
		defer func() { _ = fw.Close() }()
		for _, dir := range dirs {
			if err := fw.WatchDir(dir); err != nil {
				return fmt.Errorf("watching %s: %w", dir, err)
			}
		}
		connected()

		lost := errors.New("watcher closed")
		fw.loop(ctx, func(event fsnotify.Event, abs string) bool {
			if isRemoval(event.Op) && slices.Contains(dirs, abs) {
				lost = fmt.Errorf("%s was removed", abs)
				return false
			}
			if accepts(abs, event.Op) {
				changed()
			}
			return true
		})
		return lost
	})
}
