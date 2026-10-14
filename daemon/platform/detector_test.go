package platform

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/tinkerologie/unraid-management-agent/daemon/dto"
)

// TestDetectMarksMissingProbes checks a missing path is absent and unavailable.
func TestDetectMarksMissingProbes(t *testing.T) {
	caps := Detect([]Probe{{Name: "ghost", Target: "/no/such/path", Kind: ProbePath}})
	if len(caps.Items) != 1 || caps.Items[0].Available {
		t.Fatalf("expected missing probe to be unavailable: %+v", caps.Items)
	}
	if caps.Items[0].State != dto.CapabilityAbsent {
		t.Fatalf("state = %q, want absent", caps.Items[0].State)
	}
}

// TestDetectPathAndBinaryPresent checks path and binary probes report present and absent.
func TestDetectPathAndBinaryPresent(t *testing.T) {
	dir := t.TempDir()
	caps := Detect([]Probe{
		{Name: "dir", Target: dir, Kind: ProbePath},
		{Name: "sh", Target: "sh", Kind: ProbeBinary},
		{Name: "nobin", Target: "definitely-not-a-binary-xyz", Kind: ProbeBinary},
	})
	want := []dto.CapabilityState{dto.CapabilityPresent, dto.CapabilityPresent, dto.CapabilityAbsent}
	for i, w := range want {
		if caps.Items[i].State != w || caps.Items[i].Available != (w == dto.CapabilityPresent) {
			t.Errorf("%s: state=%q available=%v, want %q", caps.Items[i].Name, caps.Items[i].State, caps.Items[i].Available, w)
		}
	}
}

// TestDetectContextCheckProbes covers check probe states, invalid states, nil checks, panics and CheckedAt.
func TestDetectContextCheckProbes(t *testing.T) {
	fixed := time.Date(2026, 10, 14, 9, 0, 0, 0, time.UTC)
	orig := now
	now = func() time.Time { return fixed }
	t.Cleanup(func() { now = orig })

	check := func(s dto.CapabilityState, d string) CheckFunc {
		return func(context.Context) (dto.CapabilityState, string) { return s, d }
	}
	caps := DetectContext(context.Background(), []Probe{
		{Name: "present", Kind: ProbeCheck, Check: check(dto.CapabilityPresent, "found it")},
		{Name: "absent", Kind: ProbeCheck, Check: check(dto.CapabilityAbsent, "nothing")},
		{Name: "unknown", Kind: ProbeCheck, Check: check(dto.CapabilityUnknown, "permission denied")},
		{Name: "invalid", Kind: ProbeCheck, Check: check("maybe", "")},
		{Name: "nil", Kind: ProbeCheck},
		{Name: "panic", Kind: ProbeCheck, Check: func(context.Context) (dto.CapabilityState, string) { panic("boom") }},
	})
	if !caps.CheckedAt.Equal(fixed) {
		t.Errorf("CheckedAt = %v, want %v", caps.CheckedAt, fixed)
	}
	tests := []struct {
		name   string
		state  dto.CapabilityState
		detail string
	}{
		{"present", dto.CapabilityPresent, "found it"},
		{"absent", dto.CapabilityAbsent, "nothing"},
		{"unknown", dto.CapabilityUnknown, "permission denied"},
		{"invalid", dto.CapabilityUnknown, "invalid check state"},
		{"nil", dto.CapabilityUnknown, "no check function"},
		{"panic", dto.CapabilityUnknown, "check panicked: boom"},
	}
	for _, tt := range tests {
		it, ok := caps.Item(tt.name)
		if !ok {
			t.Fatalf("missing item %q", tt.name)
		}
		if it.State != tt.state || !strings.Contains(it.Detail, tt.detail) {
			t.Errorf("%s: got %q/%q, want %q/%q", tt.name, it.State, it.Detail, tt.state, tt.detail)
		}
		if it.Available != (tt.state == dto.CapabilityPresent) {
			t.Errorf("%s: Available=%v inconsistent with state %q", tt.name, it.Available, it.State)
		}
	}
	if _, ok := caps.Item("missing"); ok {
		t.Error("Item(missing) reported found")
	}
}

// TestDetectContextCancelled checks a cancelled context skips checks and reports unknown.
func TestDetectContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	called := false
	caps := DetectContext(ctx, []Probe{{Name: "hw", Kind: ProbeCheck, Check: func(context.Context) (dto.CapabilityState, string) {
		called = true
		return dto.CapabilityAbsent, ""
	}}})
	if called {
		t.Error("check ran with a cancelled context")
	}
	if caps.Items[0].State != dto.CapabilityUnknown {
		t.Errorf("state = %q, want unknown", caps.Items[0].State)
	}
}

// TestDetectContextInterruptedAbsentIsUnknown checks an interrupted check cannot prove absence.
func TestDetectContextInterruptedAbsentIsUnknown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	caps := DetectContext(ctx, []Probe{
		{Name: "absent", Kind: ProbeCheck, Check: func(context.Context) (dto.CapabilityState, string) {
			cancel()
			return dto.CapabilityAbsent, "gave up"
		}},
	})
	if caps.Items[0].State != dto.CapabilityUnknown {
		t.Errorf("interrupted absent: state = %q, want unknown", caps.Items[0].State)
	}

	ctx2, cancel2 := context.WithCancel(context.Background())
	caps = DetectContext(ctx2, []Probe{
		{Name: "present", Kind: ProbeCheck, Check: func(context.Context) (dto.CapabilityState, string) {
			cancel2()
			return dto.CapabilityPresent, "found before cancel"
		}},
	})
	if caps.Items[0].State != dto.CapabilityPresent {
		t.Errorf("interrupted present: state = %q, want present", caps.Items[0].State)
	}
}

// TestDetectBuildsNewItemsSlice checks snapshots do not share their Items backing array.
func TestDetectBuildsNewItemsSlice(t *testing.T) {
	probes := []Probe{{Name: "a", Target: "/no/such", Kind: ProbePath}}
	first := Detect(probes)
	second := Detect(probes)
	second.Items[0].Name = "changed"
	if first.Items[0].Name != "a" {
		t.Fatal("snapshots share their Items backing array")
	}
}
