package collectors

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/constants"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

const storcliNoControllers = `{"Controllers":[{"Command Status":{"CLI Version":"007.3305","Status Code":0,"Status":"Success"},` +
	`"Response Data":{"Number of Controllers":0}}]}`

func hasError(errs []string, substr string) bool {
	for _, e := range errs {
		if strings.Contains(e, substr) {
			return true
		}
	}
	return false
}

func TestStorageTopologyUnsupported(t *testing.T) {
	t.Run("no tools and no SES devices", func(t *testing.T) {
		c := newFixtureCollector(t, newFakeRunner(t))
		c.SysfsRoot = t.TempDir()
		topo := c.Gather(context.Background())
		if topo.State != dto.StorageTopologyStateUnsupported || len(topo.Errors) != 0 {
			t.Errorf("state=%q errors=%v", topo.State, topo.Errors)
		}
		if topo.Controllers == nil || topo.Enclosures == nil || topo.Drives == nil {
			t.Error("lists must be empty, not nil")
		}
	})

	t.Run("SES devices without sg_ses", func(t *testing.T) {
		c := newFixtureCollector(t, newFakeRunner(t))
		topo := c.Gather(context.Background())
		if topo.State != dto.StorageTopologyStateUnsupported || !hasError(topo.Errors, "found 5 SES device(s) but sg_ses") {
			t.Errorf("state=%q errors=%v", topo.State, topo.Errors)
		}
	})

	t.Run("storcli without controllers", func(t *testing.T) {
		runner := newFakeRunner(t)
		runner.outputs["storcli show J nolog"] = storcliNoControllers
		c := newFixtureCollector(t, runner, "/sbin/storcli", "/usr/bin/sg_ses")
		c.SysfsRoot = t.TempDir()
		topo := c.Gather(context.Background())
		if topo.State != dto.StorageTopologyStateUnsupported || topo.Sources.Storcli || runner.called("storcli /call") != 0 {
			t.Errorf("state=%q sources=%+v calls=%v", topo.State, topo.Sources, runner.calls)
		}
	})
}

func TestStorageTopologyStorcliOnly(t *testing.T) {
	c := newFixtureCollector(t, newFakeRunner(t), "/sbin/storcli")
	topo := c.Gather(context.Background())
	if topo.State != dto.StorageTopologyStateOK || topo.Sources.SES || !topo.Sources.Storcli {
		t.Fatalf("state=%q sources=%+v", topo.State, topo.Sources)
	}
	if len(topo.Enclosures) != 2 {
		t.Fatalf("enclosures = %d, want 2", len(topo.Enclosures))
	}
	e := topo.Enclosures[0]
	if e.ID == "" || strings.HasPrefix(e.ID, "c0-") || *e.EnclosureDeviceID != 242 || e.Status != "OK" ||
		e.Vendor != "NETAPP" || e.SerialNumber == "" || e.Slots != 24 || *e.SlotsPopulated != 22 {
		t.Errorf("unexpected storcli-only enclosure: %+v", e)
	}
	if len(e.PowerSupplies) != 4 || len(e.Fans) != 8 || len(e.TemperatureSensors) != 12 || len(e.IOMs) != 2 {
		t.Fatalf("unexpected element counts: %+v", e)
	}
	if e.Fans[0].RPM != nil || e.Fans[0].Speed != "Low Speed" || *e.TemperatureSensors[11].Value != 63 {
		t.Errorf("unexpected storcli fan/temperature: %+v %+v", e.Fans[0], e.TemperatureSensors[11])
	}
	if e.Redundancy.ExpectedPaths != 2 || e.Redundancy.ActivePaths != 2 || e.Redundancy.Degraded {
		t.Errorf("unexpected redundancy: %+v", e.Redundancy)
	}
	// Without SES the controller ports cannot be tied to an enclosure.
	if topo.Controllers[0].Ports[0].AttachedEnclosureID != "" {
		t.Errorf("port unexpectedly resolved: %+v", topo.Controllers[0].Ports[0])
	}
}

func TestStorageTopologyStorcliTimeoutStopsStorcli(t *testing.T) {
	runner := newFakeRunner(t)
	runner.errs["storcli /call/pall show J nolog"] = fmt.Errorf("command did not finish: %w", context.DeadlineExceeded)
	c := newFixtureCollector(t, runner, "/sbin/storcli", "/usr/bin/sg_ses")
	topo := c.Gather(context.Background())

	if !hasError(topo.Errors, "storcli /call/pall show") || runner.called("storcli /call/eall") != 0 {
		t.Errorf("errors=%v calls=%v", topo.Errors, runner.calls)
	}
	if len(topo.Controllers) != 1 || len(topo.Controllers[0].Ports) != 0 || !topo.Sources.SES {
		t.Errorf("controller should be reported without ports and SES still read: %+v", topo.Sources)
	}
}

func TestStorageTopologyInflightGuard(t *testing.T) {
	runner := newFakeRunner(t)
	key := "storcli /call/eall/sall show all J nolog"
	stuck := make(chan struct{})
	runner.pending[key] = stuck
	runner.errs[key] = fmt.Errorf("killed: %w", context.DeadlineExceeded)
	c := newFixtureCollector(t, runner, "/sbin/storcli", "/usr/bin/sg_ses")

	first := c.Gather(context.Background())
	if !hasError(first.Errors, "storcli /call/eall/sall show all") {
		t.Fatalf("first cycle errors = %v", first.Errors)
	}

	// The killed storcli has not exited: the next cycle must not start another one.
	before := runner.called("storcli ")
	second := c.Gather(context.Background())
	if runner.called("storcli ") != before || !hasError(second.Errors, "previous invocation has not exited yet") {
		t.Errorf("storcli started while the previous call was running: errors=%v", second.Errors)
	}
	if second.State != dto.StorageTopologyStateOK || len(second.Enclosures) != 3 {
		t.Errorf("SES data must still be reported: state=%q enclosures=%d", second.State, len(second.Enclosures))
	}

	// Once it exits, storcli runs again.
	close(stuck)
	delete(runner.pending, key)
	delete(runner.errs, key)
	third := c.Gather(context.Background())
	if len(third.Errors) != 0 || len(third.Drives) != 43 {
		t.Errorf("third cycle errors=%v drives=%d", third.Errors, len(third.Drives))
	}
}

func TestStorageTopologyStorcliStepErrors(t *testing.T) {
	steps := []struct {
		key, wantErr string
		stops        bool
	}{
		{"show", "storcli show", true},
		{"/call show all", "storcli /call show all", true},
		{"/call/pall show", "storcli /call/pall show", false},
		{"/call/eall show all", "storcli /call/eall show all", false},
		{"/call/eall show status", "storcli /call/eall show status", false},
		{"/call/eall/sall show all", "storcli /call/eall/sall show all", false},
	}
	for _, step := range steps {
		t.Run("invalid JSON from "+step.key, func(t *testing.T) {
			runner := newFakeRunner(t)
			runner.outputs["storcli "+step.key+" J nolog"] = "not json"
			c := newFixtureCollector(t, runner, "/sbin/storcli", "/usr/bin/sg_ses")
			topo := c.Gather(context.Background())
			if !hasError(topo.Errors, step.wantErr) {
				t.Errorf("errors = %v, want %q", topo.Errors, step.wantErr)
			}
			if step.stops && runner.called("storcli /call/pall") != 0 {
				t.Errorf("collection continued after %s failed", step.key)
			}
		})
		t.Run("command failure without output from "+step.key, func(t *testing.T) {
			runner := newFakeRunner(t)
			k := "storcli " + step.key + " J nolog"
			runner.outputs[k] = ""
			runner.errs[k] = errors.New("exit status 1")
			c := newFixtureCollector(t, runner, "/sbin/storcli", "/usr/bin/sg_ses")
			topo := c.Gather(context.Background())
			if !hasError(topo.Errors, "exit status 1") {
				t.Errorf("errors = %v", topo.Errors)
			}
		})
	}

	t.Run("non-zero exit with JSON output is parsed", func(t *testing.T) {
		runner := newFakeRunner(t)
		runner.errs["storcli /call/eall show status J nolog"] = errors.New("exit status 46")
		c := newFixtureCollector(t, runner, "/sbin/storcli", "/usr/bin/sg_ses")
		topo := c.Gather(context.Background())
		if len(topo.Errors) != 0 || len(topo.Enclosures) != 3 {
			t.Errorf("errors=%v enclosures=%d", topo.Errors, len(topo.Enclosures))
		}
	})
}

func TestStorageTopologySESErrors(t *testing.T) {
	runner := newFakeRunner(t)
	runner.outputs["sg_ses --readonly --json --page=1 /dev/sg4"] = "sg_ses: unrecognized option '--json'"
	runner.errs["sg_ses --readonly --json --join /dev/sg6"] = errors.New("exit status 99")
	c := newFixtureCollector(t, runner, "/usr/bin/sg_ses")
	topo := c.Gather(context.Background())
	if !hasError(topo.Errors, "sg_ses sg4 configuration page") || !hasError(topo.Errors, "sg_ses sg6 status pages") {
		t.Errorf("errors = %v", topo.Errors)
	}
	if topo.Sources.SESDevices != 3 || len(topo.Enclosures) != 3 {
		t.Errorf("devices=%d enclosures=%d", topo.Sources.SESDevices, len(topo.Enclosures))
	}
	// Without storcli, enclosures keep SES discovery order (sg0 is the HighPoint
	// device). Each shelf is now reached through one path only: degraded.
	for _, e := range topo.Enclosures[1:] {
		if r := e.Redundancy; !r.Degraded || r.ActivePaths != 1 || r.ExpectedPaths != 2 ||
			!hasError(e.Problems, "only 1 of 2 host paths active") {
			t.Errorf("unexpected redundancy for %s: %+v problems=%v", e.ID, r, e.Problems)
		}
	}
}

func TestStorageTopologyCycleOutOfTime(t *testing.T) {
	c := newFixtureCollector(t, newFakeRunner(t), "/usr/bin/sg_ses")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	topo := c.Gather(ctx)
	if !hasError(topo.Errors, "ran out of time") || topo.Sources.SES {
		t.Errorf("errors=%v sources=%+v", topo.Errors, topo.Sources)
	}
}

func TestStorageTopologyCollectPublishes(t *testing.T) {
	hub := domain.NewEventBus(10)
	ch := hub.Sub(constants.TopicStorageTopologyUpdate.Name)
	defer hub.Unsub(ch, constants.TopicStorageTopologyUpdate.Name)

	c := newFixtureCollector(t, newFakeRunner(t), "/sbin/storcli", "/usr/bin/sg_ses")
	c.appCtx = &domain.Context{Hub: hub}
	c.Collect(context.Background())
	select {
	case msg := <-ch:
		topo, ok := msg.(*dto.StorageTopology)
		if !ok || topo.Summary.Drives != 43 {
			t.Errorf("unexpected message %T", msg)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no topology published")
	}

	// A cancelled context means shutdown: nothing is published.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c.Collect(ctx)
	select {
	case msg := <-ch:
		t.Errorf("published during shutdown: %T", msg)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestStorageTopologyStart(t *testing.T) {
	t.Run("collects, recovers from panics and stops on cancel", func(t *testing.T) {
		hub := domain.NewEventBus(10)
		ch := hub.Sub(constants.TopicStorageTopologyUpdate.Name)
		defer hub.Unsub(ch, constants.TopicStorageTopologyUpdate.Name)

		runner := newFakeRunner(t)
		c := newFixtureCollector(t, runner, "/usr/bin/sg_ses")
		c.appCtx = &domain.Context{Hub: hub}
		c.Stagger = 0
		calls := 0
		c.RunFn = func(ctx context.Context, timeout time.Duration, name string, args ...string) (string, <-chan struct{}, error) {
			calls++
			if calls == 1 {
				panic("boom")
			}
			return runner.run(ctx, timeout, name, args...)
		}

		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() {
			c.Start(ctx, 20*time.Millisecond)
			close(done)
		}()
		select {
		case <-ch:
		case <-time.After(5 * time.Second):
			t.Fatal("no topology published after a panicking cycle")
		}
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("Start did not return after cancel")
		}
	})

	t.Run("returns during the startup stagger", func(t *testing.T) {
		c := newFixtureCollector(t, newFakeRunner(t))
		c.Stagger = time.Hour
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		finished := make(chan struct{})
		go func() {
			c.Start(ctx, time.Minute)
			close(finished)
		}()
		select {
		case <-finished:
		case <-time.After(5 * time.Second):
			t.Fatal("Start ignored cancellation during the stagger")
		}
	})
}

func TestStorageTopologyHelpers(t *testing.T) {
	t.Run("isExecutableFile", func(t *testing.T) {
		dir := t.TempDir()
		exe := filepath.Join(dir, "exe")
		plain := filepath.Join(dir, "plain")
		if err := os.WriteFile(exe, []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(plain, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		cases := map[string]bool{exe: true, plain: false, dir: false, filepath.Join(dir, "missing"): false}
		for path, want := range cases {
			if got := isExecutableFile(path); got != want {
				t.Errorf("isExecutableFile(%s) = %v, want %v", path, got, want)
			}
		}
	})

	t.Run("findBinary picks the first executable candidate", func(t *testing.T) {
		c := newFixtureCollector(t, newFakeRunner(t), "/b")
		if got := c.findBinary([]string{"/a", "/b"}); got != "/b" {
			t.Errorf("findBinary = %q", got)
		}
		if got := c.findBinary([]string{"/a"}); got != "" {
			t.Errorf("findBinary = %q, want empty", got)
		}
	})

	t.Run("logErrors only logs changes", func(t *testing.T) {
		c := newFixtureCollector(t, newFakeRunner(t))
		c.logErrors([]string{"a"})
		if c.lastErrs != "a" {
			t.Errorf("lastErrs = %q", c.lastErrs)
		}
		c.logErrors([]string{"a"})
		c.logErrors(nil)
		if c.lastErrs != "" {
			t.Errorf("lastErrs = %q, want empty", c.lastErrs)
		}
	})

	t.Run("sysfs helpers tolerate missing and odd entries", func(t *testing.T) {
		if sgNumber("sg12") != 12 || sgNumber("bogus") <= 1000 {
			t.Error("sgNumber ordering is wrong")
		}
		if readTrimmed(filepath.Join(t.TempDir(), "missing")) != "" {
			t.Error("readTrimmed should return empty for missing files")
		}
		root := t.TempDir()
		c := newFixtureCollector(t, newFakeRunner(t))
		c.SysfsRoot = root
		if c.discoverSESDevices() != nil {
			t.Error("expected no SES devices without a scsi_generic class")
		}
		short := filepath.Join(root, "block", "sdz", "device")
		if err := os.MkdirAll(short, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(short, "vpd_pg80"), []byte{0, 0x80, 0, 0}, 0o644); err != nil {
			t.Fatal(err)
		}
		if got := c.driveDevices([]dto.StorageDrive{{SerialNumber: "X"}}); len(got) != 0 {
			t.Errorf("driveDevices = %v", got)
		}
		if got := c.driveDevices(nil); len(got) != 0 {
			t.Errorf("driveDevices(nil) = %v", got)
		}
		ctrl := dto.StorageController{}
		c.applyPCIeLink(&ctrl)
		if ctrl.PCIeLinkSpeed != "" || ctrl.PCIeLinkWidth != nil {
			t.Errorf("PCIe link set without a PCI address: %+v", ctrl)
		}
	})

	t.Run("isTimeout", func(t *testing.T) {
		if !isTimeout(fmt.Errorf("x: %w", context.Canceled)) || isTimeout(errors.New("exit status 1")) {
			t.Error("isTimeout misclassifies errors")
		}
	})
}
