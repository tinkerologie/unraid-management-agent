package collectors

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"gopkg.in/ini.v1"

	"github.com/tinkerologie/unraid-management-agent/daemon/constants"
	"github.com/tinkerologie/unraid-management-agent/daemon/domain"
	"github.com/tinkerologie/unraid-management-agent/daemon/dto"
	"github.com/tinkerologie/unraid-management-agent/daemon/platform"
)

func TestArrayCollectionAvoidsCapacityProbesDuringMaintenance(t *testing.T) {
	hub := domain.NewEventBus(8)
	sub := hub.Sub(constants.TopicArrayStatusUpdate.Name)
	t.Cleanup(func() { hub.Unsub(sub) })
	registry := platform.NewRegistry()
	c := NewArrayCollector(&domain.Context{Hub: hub, Platform: registry})
	dir := t.TempDir()
	c.varIni = filepath.Join(dir, "var.ini")
	c.disksIni = filepath.Join(dir, "disks.ini")
	if err := os.WriteFile(c.disksIni, []byte("[\"parity\"]\nstatus=DISK_OK\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	calls := 0
	c.statfs = func(name string, stat *syscall.Statfs_t) error {
		if name != "/mnt/user" {
			t.Fatalf("unexpected capacity source %q", name)
		}
		calls++
		if calls == 3 {
			return errors.New("capacity unavailable")
		}
		stat.Bsize = 4096
		stat.Blocks = uint64(100 * calls)
		stat.Bfree = 25
		return nil
	}
	for _, step := range []struct {
		name, native, status string
		probes               int
		capacity             uint64
	}{
		{"idle", "mdResync=0\nmdResyncPos=0\n", "", 1, 409600},
		{"check at start", "mdResync=1\nmdResyncPos=0\nmdResyncDt=0\nmdResyncAction=check P\n", "running", 1, 409600},
		{"paused", "mdResync=0\nmdResyncPos=8\nmdResyncDt=10\n", "paused", 1, 409600},
		{"rebuild", "mdResync=1\nmdResyncPos=10\nmdResyncAction=recon D1\n", "reconstructing", 1, 409600},
		{"complete", "mdResync=0\nmdResyncPos=0\n", "", 2, 819200},
		{"capacity read failed", "mdResync=0\nmdResyncPos=0\n", "", 3, 819200},
	} {
		if err := os.WriteFile(c.varIni, []byte("mdState=STARTED\nmdNumDisks=15\nsbSynced=1\nmdResyncSize=100\n"+step.native), 0o600); err != nil {
			t.Fatal(err)
		}
		c.Collect()
		select {
		case event := <-sub:
			status := event.(*dto.ArrayStatus)
			if status.ParityCheckStatus != step.status || status.TotalBytes != step.capacity || calls != step.probes {
				t.Fatalf("%s: status=%q capacity=%d probes=%d", step.name, status.ParityCheckStatus, status.TotalBytes, calls)
			}
		case <-time.After(time.Second):
			t.Fatal("array did not publish")
		}
	}
}

func TestArrayOperationUsesNativeActivityAndLegacyProgress(t *testing.T) {
	for _, step := range []struct{ source, status string }{
		{"mdResync=\"1\"\nmdResyncPos=\"0\"\nmdResyncAction=\"check P\"\n", "running"},
		{"mdResync=\"0\"\nmdResyncPos=\"15\"\nmdResyncDt=\"4\"\n", "paused"},
		{"mdResyncPos=\"15\"\nmdResyncDt=\"4\"\nsbSyncAction=\"clear D1\"\n", "clearing"},
		{"mdResyncPos=\"15\"\nmdResyncDt=\"0\"\n", "paused"},
		{"mdResyncPos=\"0\"\n", ""},
		{"mdResync=\"unknown\"\nmdResyncPos=\"0\"\n", "unknown"},
		{"mdState=\"STARTED\"\n", "unknown"},
	} {
		cfg, err := ini.Load([]byte(step.source))
		if err != nil {
			t.Fatal(err)
		}
		if got := ArrayOperationStatus(cfg.Section("")); got != step.status {
			t.Errorf("state %q: got %q, want %q", step.source, got, step.status)
		}
	}
}

func TestArrayCollectionReportsUnavailableNativeState(t *testing.T) {
	hub := domain.NewEventBus(8)
	sub := hub.Sub(constants.TopicArrayStatusUpdate.Name)
	t.Cleanup(func() { hub.Unsub(sub) })
	registry := platform.NewRegistry()
	c := NewArrayCollector(&domain.Context{Hub: hub, Platform: registry})
	dir := t.TempDir()
	c.varIni = filepath.Join(dir, "var.ini")
	c.disksIni = filepath.Join(dir, "disks.ini")
	c.statfs = func(string, *syscall.Statfs_t) error {
		t.Fatal("probed capacity without a known idle array")
		return nil
	}
	c.Collect()
	if status, _ := registry.Get("array"); status.State != dto.SourceUnavailable {
		t.Fatalf("missing native state: %+v", status)
	}
	for _, step := range []struct{ native, state string }{
		{"mdNumDisks=1\n", "unknown"},
		{"mdState=STARTED\nmdNumDisks=1\nmdResync=invalid\n", "STARTED"},
	} {
		if err := os.WriteFile(c.varIni, []byte(step.native), 0o600); err != nil {
			t.Fatal(err)
		}
		c.Collect()
		select {
		case event := <-sub:
			status := event.(*dto.ArrayStatus)
			if status.State != step.state || status.SourceStatus == nil || status.SourceStatus.State != dto.SourceDegraded {
				t.Fatalf("native state %q: %+v", step.native, status)
			}
		case <-time.After(time.Second):
			t.Fatal("array did not publish")
		}
	}
}
