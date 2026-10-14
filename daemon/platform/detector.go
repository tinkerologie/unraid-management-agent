package platform

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"time"

	"github.com/tinkerologie/unraid-management-agent/daemon/dto"
)

// ProbeKind distinguishes a path probe, a binary probe and a check probe.
type ProbeKind int

const (
	ProbePath ProbeKind = iota
	ProbeBinary
	// ProbeCheck runs Probe.Check, for capabilities a path or binary cannot
	// express (e.g. "a NIC has an SFP module").
	ProbeCheck
)

// CheckFunc decides a ProbeCheck capability. It returns the state and a short
// human-readable reason. It must be read-only and honour ctx.
type CheckFunc func(ctx context.Context) (dto.CapabilityState, string)

// Probe is one capability to check. Name is a stable key; Target is the path
// or binary (unused for ProbeCheck). Callers (which know Unraid paths) build
// the probe list, keeping this package Unraid-agnostic and cycle-free.
type Probe struct {
	Name   string
	Target string
	Kind   ProbeKind
	Check  CheckFunc
}

var unraidVersionRe = regexp.MustCompile(`version="?([^"\n]+)"?`)

// now is the snapshot clock; replaceable for tests.
var now = time.Now

// DetectUnraidVersion reads /etc/unraid-version; returns "" if unavailable.
func DetectUnraidVersion() string {
	data, err := os.ReadFile("/etc/unraid-version")
	if err != nil {
		return ""
	}
	if m := unraidVersionRe.FindStringSubmatch(string(data)); m != nil {
		return m[1]
	}
	return ""
}

// Detect runs all probes and returns a capability snapshot. Never panics.
func Detect(probes []Probe) dto.Capabilities {
	return DetectContext(context.Background(), probes)
}

// DetectContext is Detect with a context for check probes. Every run builds a
// new Items slice, so earlier snapshots are never modified. Never panics.
func DetectContext(ctx context.Context, probes []Probe) dto.Capabilities {
	caps := dto.Capabilities{
		UnraidVersion: DetectUnraidVersion(),
		Items:         make([]dto.Capability, 0, len(probes)),
	}
	for _, p := range probes {
		var state dto.CapabilityState
		var detail string
		switch p.Kind {
		case ProbeBinary:
			state = presence(BinaryExists(p.Target))
		case ProbeCheck:
			state, detail = runCheck(ctx, p)
		default:
			state = presence(PathExists(p.Target))
		}
		caps.Items = append(caps.Items, dto.Capability{
			Name:      p.Name,
			Available: state == dto.CapabilityPresent,
			State:     state,
			Target:    p.Target,
			Detail:    detail,
		})
	}
	caps.CheckedAt = now()
	return caps
}

// presence maps a path or binary lookup to present or absent.
func presence(found bool) dto.CapabilityState {
	if found {
		return dto.CapabilityPresent
	}
	return dto.CapabilityAbsent
}

// runCheck runs one check probe. A missing check, a cancelled context or a
// panic reports unknown.
func runCheck(ctx context.Context, p Probe) (state dto.CapabilityState, detail string) {
	if p.Check == nil {
		return dto.CapabilityUnknown, "no check function"
	}
	if err := ctx.Err(); err != nil {
		return dto.CapabilityUnknown, fmt.Sprintf("check skipped: %v", err)
	}
	defer func() {
		if r := recover(); r != nil {
			state, detail = dto.CapabilityUnknown, fmt.Sprintf("check panicked: %v", r)
		}
	}()
	state, detail = p.Check(ctx)
	if err := ctx.Err(); err != nil && state != dto.CapabilityPresent {
		// An interrupted check may have missed evidence; absent is not proven.
		return dto.CapabilityUnknown, fmt.Sprintf("check interrupted: %v", err)
	}
	switch state {
	case dto.CapabilityPresent, dto.CapabilityAbsent, dto.CapabilityUnknown:
		return state, detail
	}
	return dto.CapabilityUnknown, fmt.Sprintf("invalid check state %q", state)
}
