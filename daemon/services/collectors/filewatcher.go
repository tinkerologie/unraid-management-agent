// Package collectors provides data collection services for Unraid system resources.
package collectors

import (
	"context"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/tinkerologie/unraid-management-agent/daemon/logger"
)

// FileWatcher watches files for changes using fsnotify and triggers callbacks.
// It provides debouncing to coalesce rapid successive fs events (e.g. truncate+write)
// into a single callback invocation.
type FileWatcher struct {
	watcher    *fsnotify.Watcher
	mu         sync.Mutex
	debounce   time.Duration
	timers     map[string]*time.Timer
	callbackMu sync.Mutex // guards the in-flight callback to prevent overlapping executions
}

// NewFileWatcher creates a new FileWatcher with the given debounce duration.
// The debounce duration prevents rapid-fire callbacks when a file is modified
// multiple times in quick succession (common for INI file writes).
func NewFileWatcher(debounce time.Duration) (*FileWatcher, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	return &FileWatcher{
		watcher:  w,
		debounce: debounce,
		timers:   make(map[string]*time.Timer),
	}, nil
}

// WatchFile adds a file to the watch list by watching its parent directory.
// fsnotify watches directories, not individual files, so we watch the dir
// and filter events by filename.
func (fw *FileWatcher) WatchFile(filePath string) error {
	dir := filepath.Dir(filePath)
	return fw.watcher.Add(dir)
}

// Run starts the event loop. It calls onChange when any of the watched files
// are written or created. The callback is debounced to avoid redundant triggers.
// Run blocks until the context is cancelled.
func (fw *FileWatcher) Run(ctx context.Context, watchedFiles []string, onChange func()) {
	// Build lookup set for fast matching
	fileSet := make(map[string]struct{}, len(watchedFiles))
	for _, f := range watchedFiles {
		abs, err := filepath.Abs(f)
		if err != nil {
			fileSet[f] = struct{}{}
		} else {
			fileSet[abs] = struct{}{}
		}
	}

	fw.loop(ctx, func(event fsnotify.Event, abs string) bool {
		// Only react to write and create events on watched files
		if event.Op&(fsnotify.Write|fsnotify.Create) == 0 {
			return true
		}
		if _, watched := fileSet[abs]; !watched {
			return true
		}
		logger.Debug("FileWatcher: change detected on %s (op=%s)", event.Name, event.Op)
		fw.debouncedCallback(abs, onChange)
		return true
	})
}

// WatchDir adds a directory to the watch list for RunDir.
func (fw *FileWatcher) WatchDir(dir string) error {
	return fw.watcher.Add(dir)
}

// RunDir starts the event loop in directory-entry mode. It calls onChange when
// an entry of dir whose name matches glob ("" matches all) is created, written,
// removed or renamed. Changes are debounced across the whole directory.
// RunDir blocks until the context is cancelled or dir itself is removed.
func (fw *FileWatcher) RunDir(ctx context.Context, dir, glob string, onChange func()) {
	dir = absPath(dir)
	fw.loop(ctx, func(event fsnotify.Event, abs string) bool {
		if abs == dir {
			return !isRemoval(event.Op)
		}
		if dirEntryChanged(dir, glob, abs, event.Op) {
			logger.Debug("FileWatcher: change detected on %s (op=%s)", event.Name, event.Op)
			fw.debouncedCallback(dir, onChange)
		}
		return true
	})
}

// isRemoval reports whether op removes or renames the path away.
func isRemoval(op fsnotify.Op) bool {
	return op&(fsnotify.Remove|fsnotify.Rename) != 0
}

// isEntryChange reports whether op creates, writes, removes or renames a path.
func isEntryChange(op fsnotify.Op) bool {
	return op&(fsnotify.Write|fsnotify.Create|fsnotify.Remove|fsnotify.Rename) != 0
}

// dirEntryChanged reports whether an event on abs is a create, write, remove
// or rename of an entry of dir whose name matches glob ("" matches all).
func dirEntryChanged(dir, glob, abs string, op fsnotify.Op) bool {
	if !isEntryChange(op) || filepath.Dir(abs) != dir {
		return false
	}
	if glob == "" {
		return true
	}
	ok, _ := filepath.Match(glob, filepath.Base(abs))
	return ok
}

// loop passes each event and its absolute path to handle until the context is
// cancelled, the watcher is closed or handle returns false.
func (fw *FileWatcher) loop(ctx context.Context, handle func(event fsnotify.Event, abs string) bool) {
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-fw.watcher.Events:
			if !ok {
				logger.Warning("FileWatcher: events channel closed, watcher exiting")
				return
			}
			if !handle(event, absPath(event.Name)) {
				return
			}
		case err, ok := <-fw.watcher.Errors:
			if !ok {
				logger.Warning("FileWatcher: errors channel closed, watcher exiting")
				return
			}
			logger.Error("FileWatcher error: %v", err)
		}
	}
}

// absPath returns the absolute form of path, or path itself if that fails.
func absPath(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return abs
}

// Close releases the underlying fsnotify watcher resources.
func (fw *FileWatcher) Close() error {
	return fw.watcher.Close()
}

// debouncedCallback ensures the callback fires at most once per debounce window
// for a given file path. If another event for the same file arrives within the
// window, the timer resets.
func (fw *FileWatcher) debouncedCallback(key string, cb func()) {
	fw.mu.Lock()
	defer fw.mu.Unlock()

	if t, exists := fw.timers[key]; exists {
		t.Stop()
	}
	fw.timers[key] = time.AfterFunc(fw.debounce, func() {
		// Guard against overlapping callback executions: if the previous callback
		// is still running, skip this firing rather than queuing behind it.
		if !fw.callbackMu.TryLock() {
			logger.Warning("FileWatcher: callback for %s skipped — previous run still in progress; change will be re-read on next collector tick", key)
			return
		}
		defer fw.callbackMu.Unlock()
		cb()
	})
}
