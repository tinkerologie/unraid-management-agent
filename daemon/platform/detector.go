package platform

import (
	"os"
	"regexp"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

// ProbeKind distinguishes a path probe from a binary probe.
type ProbeKind int

const (
	ProbePath ProbeKind = iota
	ProbeBinary
)

// Probe is one capability to check at startup. Name is a stable key; Target is
// the path or binary. Callers (which know Unraid paths) build the probe list,
// keeping this package Unraid-agnostic and cycle-free.
type Probe struct {
	Name   string
	Target string
	Kind   ProbeKind
}

var unraidVersionRe = regexp.MustCompile(`version="?([^"\n]+)"?`)

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
	caps := dto.Capabilities{UnraidVersion: DetectUnraidVersion()}
	for _, p := range probes {
		available := false
		switch p.Kind {
		case ProbeBinary:
			available = BinaryExists(p.Target)
		default:
			available = PathExists(p.Target)
		}
		caps.Items = append(caps.Items, dto.Capability{
			Name:      p.Name,
			Available: available,
			Target:    p.Target,
		})
	}
	return caps
}
