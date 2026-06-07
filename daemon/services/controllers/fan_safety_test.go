package controllers

import (
	"bytes"
	"log"
	"os"
	"strings"
	"testing"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
)

func TestDetectFailuresLogsOncePerTransition(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	logger.SetLevel(logger.LevelInfo)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	g := NewFanSafetyGuard(nil, dto.FanSafetyConfig{FailureRPMThreshold: 100, MinSpeedPercent: 20, CriticalTempC: 90})
	stalled := []dto.FanDevice{{ID: "hwmon4_fan2", Controllable: true, PWMPercent: 50, RPM: 0}}

	// Three consecutive stalled polls must log the warning only ONCE.
	g.DetectFailures(stalled)
	g.DetectFailures(stalled)
	g.DetectFailures(stalled)
	if n := strings.Count(buf.String(), "appears stalled"); n != 1 {
		t.Errorf("expected stall warning once across 3 cycles, got %d", n)
	}

	// Recovery is logged once.
	recovered := []dto.FanDevice{{ID: "hwmon4_fan2", Controllable: true, PWMPercent: 50, RPM: 1200}}
	g.DetectFailures(recovered)
	if n := strings.Count(buf.String(), "recovered"); n != 1 {
		t.Errorf("expected recovery logged once, got %d", n)
	}

	// Re-entering the stalled state logs the warning again (new transition).
	g.DetectFailures(stalled)
	if n := strings.Count(buf.String(), "appears stalled"); n != 2 {
		t.Errorf("expected stall warning to log again after recovery (2 total), got %d", n)
	}
}

func TestDetectFailuresReturnsAllFailedEachCall(t *testing.T) {
	g := NewFanSafetyGuard(nil, dto.FanSafetyConfig{FailureRPMThreshold: 100})
	fans := []dto.FanDevice{
		{ID: "a", Controllable: true, PWMPercent: 50, RPM: 0},
		{ID: "b", Controllable: true, PWMPercent: 50, RPM: 1500}, // healthy
	}
	for i := 0; i < 2; i++ {
		got := g.DetectFailures(fans)
		if len(got) != 1 || got[0] != "a" {
			t.Fatalf("call %d: expected [a], got %v", i, got)
		}
	}
}
