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
	prevLevel := logger.GetLevel()
	log.SetOutput(&buf)
	logger.SetLevel(logger.LevelInfo)
	t.Cleanup(func() {
		log.SetOutput(os.Stderr)
		logger.SetLevel(prevLevel)
	})

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
	for i := range 2 {
		got := g.DetectFailures(fans)
		if len(got) != 1 || got[0] != "a" {
			t.Fatalf("call %d: expected [a], got %v", i, got)
		}
	}
}

func TestValidatePWM(t *testing.T) {
	g := NewFanSafetyGuard(nil, dto.FanSafetyConfig{MinSpeedPercent: 20})

	tests := []struct {
		name string
		pct  int
		want int
	}{
		{"below minimum clamps up", 10, 20},
		{"far below minimum clamps up", -50, 20},
		{"exactly at minimum", 20, 20},
		{"above minimum unchanged", 55, 55},
		{"max unchanged", 100, 100},
		{"above 100 unchanged (no upper clamp here)", 150, 150},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := g.ValidatePWM(tt.pct); got != tt.want {
				t.Errorf("ValidatePWM(%d) = %d, want %d", tt.pct, got, tt.want)
			}
		})
	}
}

func TestValidatePWMDefaultMinimum(t *testing.T) {
	// A zero MinSpeedPercent is normalized to DefaultMinSpeedPercent by the constructor.
	g := NewFanSafetyGuard(nil, dto.FanSafetyConfig{})
	if got := g.ValidatePWM(0); got != DefaultMinSpeedPercent {
		t.Errorf("ValidatePWM(0) = %d, want default %d", got, DefaultMinSpeedPercent)
	}
	if got := g.Config().MinSpeedPercent; got != DefaultMinSpeedPercent {
		t.Errorf("Config().MinSpeedPercent = %d, want %d", got, DefaultMinSpeedPercent)
	}
}

func TestCheckTemperatureSafety(t *testing.T) {
	tests := []struct {
		name     string
		temp     float64
		critical float64
		want     bool
	}{
		{"below critical", 65.0, 90.0, false},
		{"equal to critical", 90.0, 90.0, true},
		{"above critical", 95.5, 90.0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewFanSafetyGuard(nil, dto.FanSafetyConfig{
				CriticalTempC: tt.critical,
			})
			g.readMaxTemp = func() float64 {
				return tt.temp
			}
			if got := g.CheckTemperatureSafety(); got != tt.want {
				t.Errorf("CheckTemperatureSafety() = %v, want %v", got, tt.want)
			}
		})
	}
}
