package collectors

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
)

func writeTestFile(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestFileEventSource(t *testing.T) {
	dir := t.TempDir()
	varIni := filepath.Join(dir, "var.ini")
	disksIni := filepath.Join(dir, "disks.ini")
	writeTestFile(t, varIni)

	src := fastSource(NewFileEventSource(10*time.Millisecond, varIni, disksIni), 10*time.Millisecond)
	rec := newTriggerRecorder()
	runSource(t, src, rec.trigger)
	rec.wait(t) // catch-up once watching

	steps := []struct {
		name string
		do   func() error
	}{
		{"write", func() error { return os.WriteFile(varIni, []byte("x"), 0o600) }},
		{"create", func() error { return os.WriteFile(disksIni, []byte("x"), 0o600) }},
		{"remove", func() error { return os.Remove(disksIni) }},
		{"rename away", func() error { return os.Rename(varIni, varIni+".bak") }},
		{"rename into place", func() error { return os.Rename(varIni+".bak", varIni) }},
	}
	for _, s := range steps {
		if err := s.do(); err != nil {
			t.Fatalf("%s: %v", s.name, err)
		}
		rec.wait(t)
	}
}

func TestDirEventSource(t *testing.T) {
	dir := t.TempDir()
	src := fastSource(NewDirEventSource(10*time.Millisecond, dir, "*.plg"), 10*time.Millisecond)
	rec := newTriggerRecorder()
	runSource(t, src, rec.trigger)
	rec.wait(t) // catch-up once watching

	plg := filepath.Join(dir, "a.plg")
	writeTestFile(t, filepath.Join(dir, "notes.txt"))
	rec.none(t, 100*time.Millisecond)

	writeTestFile(t, plg)
	rec.wait(t)
	if err := os.Remove(plg); err != nil {
		t.Fatal(err)
	}
	rec.wait(t)
}

// TestDirEventSource_DirAppearsAndVanishes covers a missing directory being
// retried, and a removed directory being watched again once recreated.
func TestDirEventSource_DirAppearsAndVanishes(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "unassigned.devices")
	src := fastSource(NewDirEventSource(10*time.Millisecond, dir, ""), 10*time.Millisecond)
	rec := newTriggerRecorder()
	runSource(t, src, rec.trigger)

	rec.none(t, 50*time.Millisecond) // missing: retried, no trigger
	if err := os.Mkdir(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	rec.wait(t) // catch-up once the directory exists

	if err := os.Remove(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	rec.wait(t) // catch-up once watched again
}

func TestDirEntryChanged(t *testing.T) {
	dir := "/boot/config/plugins"
	tests := []struct {
		name string
		glob string
		path string
		op   fsnotify.Op
		want bool
	}{
		{"plg created", "*.plg", dir + "/a.plg", fsnotify.Create, true},
		{"plg written", "*.plg", dir + "/a.plg", fsnotify.Write, true},
		{"plg removed", "*.plg", dir + "/a.plg", fsnotify.Remove, true},
		{"plg renamed", "*.plg", dir + "/a.plg", fsnotify.Rename, true},
		{"plg chmod", "*.plg", dir + "/a.plg", fsnotify.Chmod, false},
		{"other extension", "*.plg", dir + "/a.txt", fsnotify.Create, false},
		{"nested entry", "*.plg", dir + "/sub/a.plg", fsnotify.Create, false},
		{"other directory", "*.plg", "/boot/config/a.plg", fsnotify.Create, false},
		{"empty glob matches all", "", dir + "/anything", fsnotify.Write, true},
		{"bad glob matches none", "[", dir + "/a.plg", fsnotify.Write, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := dirEntryChanged(dir, tt.glob, tt.path, tt.op); got != tt.want {
				t.Errorf("dirEntryChanged(%q, %q, %v) = %v, want %v", tt.glob, tt.path, tt.op, got, tt.want)
			}
		})
	}
}
