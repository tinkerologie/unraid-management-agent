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

	if err := fw.WatchFile(testFile); err != nil {
		t.Fatal(err)
	}

	var callCount atomic.Int32
	ctx := t.Context()

	go func() {
		defer func() { _ = fw.Close() }()
		fw.Run(ctx, []string{testFile}, func() {
			callCount.Add(1)
		})
	}()

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

	// Watch the directory (via the watched file)
	if err := fw.WatchFile(watchedFile); err != nil {
		t.Fatal(err)
	}

	var callCount atomic.Int32
	ctx := t.Context()

	go func() {
		defer func() { _ = fw.Close() }()
		fw.Run(ctx, []string{watchedFile}, func() {
			callCount.Add(1)
		})
	}()

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

	if err := fw.WatchFile(testFile); err != nil {
		t.Fatal(err)
	}

	var callCount atomic.Int32
	ctx := t.Context()

	go func() {
		defer func() { _ = fw.Close() }()
		fw.Run(ctx, []string{testFile}, func() {
			callCount.Add(1)
		})
	}()

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

// TestFileWatcher_CloseInsideGoroutine verifies the pattern used by collectors
// where fw.Close() is deferred inside the same goroutine that calls fw.Run().
// This is a regression test for #84: previously fw.Close() was deferred in the
// parent Start() function, racing with fw.Run() and causing silent exits.
func TestFileWatcher_CloseInsideGoroutine(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.ini")
	if err := os.WriteFile(testFile, []byte("initial"), 0644); err != nil {
		t.Fatal(err)
	}

	fw, err := NewFileWatcher(50 * time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if err := fw.WatchFile(testFile); err != nil {
		t.Fatal(err)
	}

	var callCount atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())

	// Mimic the collector pattern: Close is deferred inside the goroutine
	done := make(chan struct{})
	go func() {
		defer func() { _ = fw.Close() }()
		defer close(done)
		fw.Run(ctx, []string{testFile}, func() {
			callCount.Add(1)
		})
	}()

	// Let the watcher start
	time.Sleep(100 * time.Millisecond)

	// Write to trigger a callback
	if err := os.WriteFile(testFile, []byte("modified"), 0644); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)

	if callCount.Load() == 0 {
		t.Error("expected callback before cancellation")
	}

	// Cancel context — Run should return, then Close fires in the same goroutine
	cancel()

	select {
	case <-done:
		// Success — goroutine exited cleanly, Close ran after Run
	case <-time.After(2 * time.Second):
		t.Fatal("goroutine did not exit after context cancellation")
	}

	// Verify Close was called: writing after close should not trigger callbacks
	countBefore := callCount.Load()
	// fw is closed — no more events should fire (ignore write errors on closed watcher)
	_ = os.WriteFile(testFile, []byte("after-close"), 0644)
	time.Sleep(200 * time.Millisecond)

	if callCount.Load() != countBefore {
		t.Error("callback triggered after fw.Close() — watcher not properly shut down")
	}
}

// TestFileWatcher_CloseWhileRunning verifies that if fw.Close() is called
// externally while fw.Run() is active, the Run loop exits (via closed channels).
// This documents the old bug from #84 — the watcher exits but does so through
// the channel-closed path rather than the context path.
func TestFileWatcher_CloseWhileRunning(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.ini")
	if err := os.WriteFile(testFile, []byte("initial"), 0644); err != nil {
		t.Fatal(err)
	}

	fw, err := NewFileWatcher(50 * time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if err := fw.WatchFile(testFile); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		fw.Run(ctx, []string{testFile}, func() {})
		close(done)
	}()

	time.Sleep(100 * time.Millisecond)

	// Close externally while Run is selecting — this was the #84 race condition
	_ = fw.Close()

	select {
	case <-done:
		// Run exited via channel-closed path (now with warning log)
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not exit after external Close()")
	}
}
