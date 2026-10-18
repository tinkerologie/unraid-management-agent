package collectors

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
)

func TestFSEventSourceSessionErrors(t *testing.T) {
	accept := func(string, fsnotify.Op) bool { return true }
	noop := func() {}

	t.Run("watcher creation fails", func(t *testing.T) {
		old := newFileWatcher
		defer func() { newFileWatcher = old }()
		newFileWatcher = func(time.Duration) (*FileWatcher, error) { return nil, errors.New("too many open files") }
		src := newFSEventSource("test", 0, []string{t.TempDir()}, accept)
		if err := src.session(t.Context(), noop, noop); err == nil || !strings.Contains(err.Error(), "creating watcher") {
			t.Errorf("session = %v, want a watcher creation error", err)
		}
	})

	t.Run("watching a missing directory fails", func(t *testing.T) {
		missing := filepath.Join(t.TempDir(), "missing")
		src := newFSEventSource("test", 0, []string{missing}, accept)
		connected := false
		err := src.session(t.Context(), func() { connected = true }, noop)
		if err == nil || !strings.Contains(err.Error(), "watching "+missing) {
			t.Errorf("session = %v, want a watch error for %s", err, missing)
		}
		if connected {
			t.Error("a failed watch must not report a connection")
		}
	})
}

func TestFileWatcher_LoopLogsErrorsAndStopsWhenChannelsClose(t *testing.T) {
	for _, closeEvents := range []bool{true, false} {
		// A watcher whose channels the test owns, as fsnotify's backend owns real ones.
		w := &fsnotify.Watcher{Events: make(chan fsnotify.Event), Errors: make(chan error)}
		fw := &FileWatcher{watcher: w}
		done := make(chan struct{})
		go func() {
			defer close(done)
			fw.loop(context.Background(), func(fsnotify.Event, string) bool { return true })
		}()
		// The loop logs watcher errors and keeps going.
		w.Errors <- errors.New("queue overflow")
		// A closed events or errors channel ends it.
		if closeEvents {
			close(w.Events)
		} else {
			close(w.Errors)
		}
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatalf("loop did not stop after its channel closed (events: %v)", closeEvents)
		}
	}
}

func TestFileWatcher_SkipsOverlappingCallback(t *testing.T) {
	fw, err := NewFileWatcher(0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = fw.Close() }()

	started, release := make(chan struct{}), make(chan struct{})
	fw.debouncedCallback("first", func() {
		close(started)
		<-release
	})
	<-started
	second := make(chan struct{}, 1)
	fw.debouncedCallback("second", func() { second <- struct{}{} })
	select {
	case <-second:
		t.Error("a callback ran while the previous one was still running")
	case <-time.After(150 * time.Millisecond):
	}
	close(release)
}

func TestFileWatcher_RunDirReturnsWhenDirRenamed(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "watched")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	fw, err := NewFileWatcher(0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = fw.Close() }()
	if err := fw.WatchDir(dir); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		fw.RunDir(context.Background(), dir, "", func() {})
	}()
	if err := os.Rename(dir, filepath.Join(parent, "moved")); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("RunDir did not return after its directory was renamed")
	}
}
