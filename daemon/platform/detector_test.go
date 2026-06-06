package platform

import "testing"

func TestDetectMarksMissingProbes(t *testing.T) {
	caps := Detect([]Probe{{Name: "ghost", Target: "/no/such/path", Kind: ProbePath}})
	if len(caps.Items) != 1 || caps.Items[0].Available {
		t.Fatalf("expected missing probe to be unavailable: %+v", caps.Items)
	}
}
