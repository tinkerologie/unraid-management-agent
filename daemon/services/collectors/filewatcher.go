// Package collectors provides data collection services for Unraid system resources.
package collectors

import (
	"context"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
)

// FileWatcher watches files for changes using fsnotify and triggers callbacks.
// It provides debouncing to coalesce rapid successive fs events (e.g. truncate+write)
// into a single callback invocation.
type FileWatcher struct {
	watcher  *fsnotify.Watcher
	mu       sync.Mutex
	debounce time.Duration
	timers   map[string]*time.Timer
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

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-fw.watcher.Events:
			if !ok {
				return
			}
			// Only react to write and create events on watched files
			if event.Op&(fsnotify.Write|fsnotify.Create) == 0 {
				continue
			}
			abs, err := filepath.Abs(event.Name)
			if err != nil {
				abs = event.Name
			}
			if _, watched := fileSet[abs]; !watched {
				continue
			}
			logger.Debug("FileWatcher: change detected on %s (op=%s)", event.Name, event.Op)
			fw.debouncedCallback(abs, onChange)
		case err, ok := <-fw.watcher.Errors:
			if !ok {
				return
			}
			logger.Error("FileWatcher error: %v", err)
		}
	}
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
	fw.timers[key] = time.AfterFunc(fw.debounce, cb)
}
