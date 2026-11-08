package services

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tinkerologie/unraid-management-agent/daemon/services/collectors"
)

func TestManualFileEvents(t *testing.T) {
	dir := t.TempDir()
	ini := filepath.Join(dir, "state.ini")
	missing := filepath.Join(dir, "missing", "state.ini")
	source := manualFileEvents(ini, missing)
	ctx, cancel := context.WithCancel(t.Context())
	triggered := make(chan struct{}, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		source.Run(ctx, func() { notify(triggered) })
	}()
	t.Cleanup(func() { cancel(); <-done })
	// Keep producing changes until the watcher has observed one. Space writes
	// farther apart than the debounce window so a continuous burst can settle.
	deadline := time.After(5 * time.Second)
	for {
		if err := os.WriteFile(ini, []byte("state=up\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		select {
		case <-triggered:
			cancel()
			waitChan(t, done, "manual watcher shutdown")
			return
		case <-deadline:
			t.Fatal("manual watcher did not trigger on a write")
		case <-time.After(fileEventDebounce + 50*time.Millisecond):
		}
	}
}

func TestManualEventSourceSelection(t *testing.T) {
	coll := collectors.NewNotificationCollector(createTestContext())
	for _, name := range []string{"array", "disk", "shares", "notification", "psu"} {
		if len(manualEventSources(name, coll)) != 1 {
			t.Errorf("expected one Manual event source for %s", name)
		}
	}
	if len(manualEventSources("mover", coll)) != 0 {
		t.Fatal("mover events belong to Dynamic mode")
	}
}
