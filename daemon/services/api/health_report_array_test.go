package api

import (
	"testing"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

func hasFinding(report dto.HealthReport, title string) bool {
	for _, f := range report.Findings {
		if f.Title == title {
			return true
		}
	}
	return false
}

// TestBuildHealthReport_StartedArrayUsesUnraidCase guards against the false
// "Array not started" critical finding: Unraid reports mdState as "STARTED".
func TestBuildHealthReport_StartedArrayUsesUnraidCase(t *testing.T) {
	report := BuildHealthReport(nil, &dto.ArrayStatus{State: "STARTED"}, nil, nil)
	if hasFinding(report, "Array not started") {
		t.Errorf("STARTED array reported as not started: %+v", report.Findings)
	}
}

func TestBuildHealthReport_StoppedArrayIsCritical(t *testing.T) {
	report := BuildHealthReport(nil, &dto.ArrayStatus{State: "STOPPED"}, nil, nil)
	if !hasFinding(report, "Array not started") {
		t.Errorf("expected an Array not started finding, got %+v", report.Findings)
	}
}
