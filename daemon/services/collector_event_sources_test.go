package services

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/tinkerologie/unraid-management-agent/daemon/services/scheduler"
)

// TestEventSourcesMatchPolicies keeps the event-source wiring and the
// EventDriven policy flag in step.
func TestEventSourcesMatchPolicies(t *testing.T) {
	for _, name := range scheduler.CollectorNames() {
		p, _ := scheduler.PolicyFor(name)
		if got := len(defaultEventSources(name)) > 0; got != p.EventDriven {
			t.Errorf("%s: has event sources = %v, policy EventDriven = %v", name, got, p.EventDriven)
		}
	}
}

// fakeFileSource calls trigger once per request on fire and then closes the
// request's channel, until ctx is done.
type fakeFileSource struct{ fire chan chan struct{} }

func (f fakeFileSource) Run(ctx context.Context, trigger func()) {
	for {
		select {
		case <-ctx.Done():
			return
		case ack := <-f.fire:
			trigger()
			close(ack)
		}
	}
}

// fireAndWait makes the source fire once and waits for the trigger decision.
func (f fakeFileSource) fireAndWait() {
	ack := make(chan struct{})
	f.fire <- ack
	<-ack
}

func TestContentChangesIgnoresVolatileKeys(t *testing.T) {
	file := filepath.Join(t.TempDir(), "disks.ini")
	write := func(s string) {
		t.Helper()
		if err := os.WriteFile(file, []byte(s), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("[\"disk1\"]\nstatus=\"DISK_OK\"\nnumReads=\"1\"\ntemp=\"30\"\n")

	src := fakeFileSource{fire: make(chan chan struct{})}
	c := &contentChanges{src: src, file: file, ignore: disksIniVolatileKeys}
	triggered := make(chan struct{}, 10)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		c.Run(ctx, func() { triggered <- struct{}{} })
	}()
	// The source receives only after Run took its first digest.
	src.fireAndWait()
	if len(triggered) > 0 {
		t.Fatal("unchanged content should not trigger")
	}

	steps := []struct {
		name    string
		content string
		want    bool
	}{
		{"counters only", "[\"disk1\"]\nstatus=\"DISK_OK\"\nnumReads=\"9\"\ntemp=\"31\"\n", false},
		{"status changed", "[\"disk1\"]\nstatus=\"DISK_DSBL\"\nnumReads=\"9\"\ntemp=\"31\"\n", true},
		{"same again", "[\"disk1\"]\nstatus=\"DISK_DSBL\"\nnumReads=\"10\"\ntemp=\"31\"\n", false},
	}
	for _, step := range steps {
		write(step.content)
		src.fireAndWait()
		got := len(triggered) > 0
		for len(triggered) > 0 {
			<-triggered
		}
		if got != step.want {
			t.Errorf("%s: triggered = %v, want %v", step.name, got, step.want)
		}
	}

	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	src.fireAndWait()
	if len(triggered) == 0 {
		t.Error("removing the file should trigger")
	}
	cancel()
	<-done
}

func TestSharesIniChangesIgnoreSpaceUsage(t *testing.T) {
	file := filepath.Join(t.TempDir(), "shares.ini")
	write := func(s string) {
		t.Helper()
		if err := os.WriteFile(file, []byte(s), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("[\"appdata\"]\nfree=\"100\"\nused=\"5\"\nuseCache=\"yes\"\n")
	src := fakeFileSource{fire: make(chan chan struct{})}
	c := iniChanges(file, sharesIniVolatileKeys...).(*contentChanges)
	c.src = src
	triggered := make(chan struct{}, 10)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		c.Run(ctx, func() { triggered <- struct{}{} })
	}()
	src.fireAndWait()

	write("[\"appdata\"]\nfree=\"90\"\nused=\"15\"\nuseCache=\"yes\"\n")
	src.fireAndWait()
	if len(triggered) > 0 {
		t.Error("a free/used rewrite should not trigger")
	}
	write("[\"appdata\"]\nfree=\"90\"\nused=\"15\"\nuseCache=\"no\"\n")
	src.fireAndWait()
	if len(triggered) != 1 {
		t.Errorf("a share setting change triggered %d times, want 1", len(triggered))
	}
	cancel()
	<-done
}

func TestDynamicArrayEventsIgnoreProgressAndKeepStateTransitions(t *testing.T) {
	source, ok := defaultEventSources("array")[0].(*contentChanges)
	if !ok {
		t.Fatal("Dynamic array still refreshes on every native state file rewrite")
	}
	file := filepath.Join(t.TempDir(), "var.ini")
	write := func(contents string) {
		t.Helper()
		if err := os.WriteFile(file, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("mdState=STARTED\nmdNumDisks=15\nmdResync=1\nmdResyncPos=1\nmdResyncDt=1\nmdResyncAction=check P\n")
	src := fakeFileSource{fire: make(chan chan struct{})}
	source.file, source.src = file, src
	triggered := make(chan struct{}, 8)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		defer close(done)
		source.Run(ctx, func() { triggered <- struct{}{} })
	}()
	t.Cleanup(func() { cancel(); <-done })
	src.fireAndWait()

	for _, step := range []struct {
		state string
		want  bool
	}{
		{"mdResync=1\nmdResyncPos=400\nmdResyncDt=0\nmdResyncDb=12\nmdResyncAction=check P\n", false},
		{"mdResync=0\nmdResyncPos=400\nmdResyncDt=0\nmdResyncAction=check P\n", true},
		{"mdResync=1\nmdResyncPos=400\nmdResyncDt=0\nmdResyncAction=check P\n", true},
		{"mdResync=1\nmdResyncPos=500\nsbSyncErrs=1\n", true},
		{"mdResync=1\nmdResyncPos=600\nsbSyncErrs=2\n", false},
		{"mdResync=0\nmdResyncPos=0\nmdResyncDt=0\nmdResyncAction=check P\n", true},
	} {
		write("mdState=STARTED\nmdNumDisks=15\n" + step.state)
		src.fireAndWait()
		if got := len(triggered) > 0; got != step.want {
			t.Errorf("trigger=%t want %t for %q", got, step.want, step.state)
		}
		for len(triggered) > 0 {
			<-triggered
		}
	}
}
