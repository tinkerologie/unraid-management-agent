package collectors

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestFileWatcher_DetectsWrite(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.ini")
	if err := os.WriteFile(testFile, []byte("initial"), 0644); err != nil {
		t.Fatal(err)
	}

	fw, err := NewFileWatcher(50 * time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = fw.Close() }()

	if err := fw.WatchFile(testFile); err != nil {
		t.Fatal(err)
	}

	var callCount atomic.Int32
	ctx := t.Context()

	go fw.Run(ctx, []string{testFile}, func() {
		callCount.Add(1)
	})

	// Give watcher time to start
	time.Sleep(100 * time.Millisecond)

	// Write to the file
	if err := os.WriteFile(testFile, []byte("modified"), 0644); err != nil {
		t.Fatal(err)
	}

	// Wait for debounce + processing
	time.Sleep(300 * time.Millisecond)

	count := callCount.Load()
	if count == 0 {
		t.Error("expected callback to be triggered on file write, got 0 calls")
	}
}

func TestFileWatcher_IgnoresUnwatchedFiles(t *testing.T) {
	tmpDir := t.TempDir()
	watchedFile := filepath.Join(tmpDir, "watched.ini")
	unwatchedFile := filepath.Join(tmpDir, "unwatched.ini")

	if err := os.WriteFile(watchedFile, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(unwatchedFile, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}

	fw, err := NewFileWatcher(50 * time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = fw.Close() }()

	// Watch the directory (via the watched file)
	if err := fw.WatchFile(watchedFile); err != nil {
		t.Fatal(err)
	}

	var callCount atomic.Int32
	ctx := t.Context()

	go fw.Run(ctx, []string{watchedFile}, func() {
		callCount.Add(1)
	})

	time.Sleep(100 * time.Millisecond)

	// Write to the unwatched file only
	if err := os.WriteFile(unwatchedFile, []byte("changed"), 0644); err != nil {
		t.Fatal(err)
	}

	time.Sleep(300 * time.Millisecond)

	count := callCount.Load()
	if count != 0 {
		t.Errorf("expected 0 callbacks for unwatched file, got %d", count)
	}
}

func TestFileWatcher_Debounce(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.ini")
	if err := os.WriteFile(testFile, []byte("initial"), 0644); err != nil {
		t.Fatal(err)
	}

	fw, err := NewFileWatcher(200 * time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = fw.Close() }()

	if err := fw.WatchFile(testFile); err != nil {
		t.Fatal(err)
	}

	var callCount atomic.Int32
	ctx := t.Context()

	go fw.Run(ctx, []string{testFile}, func() {
		callCount.Add(1)
	})

	time.Sleep(100 * time.Millisecond)

	// Rapid-fire writes (should be debounced to 1 callback)
	for range 5 {
		if err := os.WriteFile(testFile, []byte("write"), 0644); err != nil {
			t.Fatal(err)
		}
		time.Sleep(20 * time.Millisecond)
	}

	// Wait for debounce to settle
	time.Sleep(500 * time.Millisecond)

	count := callCount.Load()
	if count != 1 {
		t.Errorf("expected 1 debounced callback, got %d", count)
	}
}

func TestFileWatcher_ContextCancel(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.ini")
	if err := os.WriteFile(testFile, []byte("initial"), 0644); err != nil {
		t.Fatal(err)
	}

	fw, err := NewFileWatcher(50 * time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = fw.Close() }()

	if err := fw.WatchFile(testFile); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		fw.Run(ctx, []string{testFile}, func() {})
		close(done)
	}()

	// Cancel should cause Run to exit
	cancel()

	select {
	case <-done:
		// Success — Run exited
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not exit after context cancellation")
	}
}
