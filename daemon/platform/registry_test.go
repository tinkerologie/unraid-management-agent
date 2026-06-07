package platform

import (
	"errors"
	"testing"
	"time"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

func TestRegistryTransitionNotifier(t *testing.T) {
	var calls []dto.SourceStatus
	r := NewRegistry()
	r.SetClock(func() time.Time { return time.Unix(0, 0) })
	r.SetNotifier(func(s dto.SourceStatus) { calls = append(calls, s) })

	r.Report("array", dto.SourceHealthy, "", nil)                      // transition nil->healthy
	r.Report("array", dto.SourceHealthy, "", nil)                      // no transition
	r.Report("array", dto.SourceDegraded, "stale", errors.New("boom")) // transition

	if len(calls) != 2 {
		t.Fatalf("notifier called %d times, want 2", len(calls))
	}
	if calls[1].State != dto.SourceDegraded || calls[1].Reason != "stale" || calls[1].LastError != "boom" {
		t.Fatalf("unexpected last notification: %+v", calls[1])
	}
}

func TestRegistrySnapshotAndCounts(t *testing.T) {
	r := NewRegistry()
	r.Report("b", dto.SourceDegraded, "x", nil)
	r.Report("a", dto.SourceHealthy, "", nil)
	r.Report("c", dto.SourceUnavailable, "missing", nil)

	snap := r.Snapshot()
	if len(snap) != 3 || snap[0].Subsystem != "a" || snap[1].Subsystem != "b" {
		t.Fatalf("snapshot not sorted by name: %+v", snap)
	}
	if r.DegradedCount() != 2 {
		t.Fatalf("DegradedCount = %d, want 2", r.DegradedCount())
	}
	if r.StatusFor("a") != nil {
		t.Fatalf("StatusFor healthy subsystem must be nil")
	}
	if r.StatusFor("b") == nil {
		t.Fatalf("StatusFor degraded subsystem must be non-nil")
	}
}

func TestRegistryDisabledNotCountedAsDegraded(t *testing.T) {
	r := NewRegistry()
	r.Report("docker", dto.SourceDisabled, "Docker service disabled in Unraid settings", nil)
	r.Report("vm", dto.SourceDisabled, "VM manager disabled in Unraid settings", nil)
	r.Report("array", dto.SourceHealthy, "", nil)

	if got := r.DegradedCount(); got != 0 {
		t.Fatalf("DegradedCount with only disabled/healthy subsystems = %d, want 0", got)
	}
	if got := r.OverallState(); got != dto.SourceHealthy {
		t.Fatalf("OverallState with only disabled/healthy subsystems = %q, want %q", got, dto.SourceHealthy)
	}

	// A genuinely faulted subsystem must still be counted alongside disabled ones.
	r.Report("disk", dto.SourceUnavailable, "cannot read disks.ini", nil)
	if got := r.DegradedCount(); got != 1 {
		t.Fatalf("DegradedCount with one unavailable subsystem = %d, want 1", got)
	}
}
