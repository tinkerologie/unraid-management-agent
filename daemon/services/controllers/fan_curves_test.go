package controllers

import (
	"bytes"
	"log"
	"os"
	"strings"
	"testing"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/lib"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
)

type fakeDriveTemps struct{ m map[string]lib.DiskTemp }

func (f fakeDriveTemps) DriveTemps() (map[string]lib.DiskTemp, error) { return f.m, nil }

func newTestEngine(drives map[string]lib.DiskTemp) *FanCurveEngine {
	e := NewFanCurveEngine(nil, NewFanSafetyGuard(nil, dto.FanSafetyConfig{}))
	e.drives = fakeDriveTemps{m: drives}
	return e
}

func TestResolveTempDrivesMaxOfActive(t *testing.T) {
	e := newTestEngine(map[string]lib.DiskTemp{
		"disk1": {ID: "disk1", TempC: 36},
		"disk2": {ID: "disk2", TempC: 41},
		"disk3": {ID: "disk3", SpunDown: true}, // excluded
	})
	src := dto.FanTempSource{Type: dto.FanTempSourceDrives, DriveIDs: []string{"disk1", "disk2", "disk3"}}
	got, ok := e.resolveTemp(src)
	if !ok || got != 41 {
		t.Fatalf("max-of-active: got (%v,%v), want (41,true)", got, ok)
	}
}

func TestResolveTempAllSpunDownNoFallback(t *testing.T) {
	e := newTestEngine(map[string]lib.DiskTemp{"disk1": {ID: "disk1", SpunDown: true}})
	src := dto.FanTempSource{Type: dto.FanTempSourceDrives, DriveIDs: []string{"disk1"}}
	if _, ok := e.resolveTemp(src); ok {
		t.Fatal("all spun down with no fallback should yield ok=false")
	}
}

func TestDriveSourceFallbackLogsOnce(t *testing.T) {
	var buf bytes.Buffer
	prev := logger.GetLevel()
	log.SetOutput(&buf)
	logger.SetLevel(logger.LevelInfo)
	t.Cleanup(func() { log.SetOutput(os.Stderr); logger.SetLevel(prev) })

	e := newTestEngine(map[string]lib.DiskTemp{"disk1": {ID: "disk1", SpunDown: true}})
	src := dto.FanTempSource{
		Type: dto.FanTempSourceDrives, DriveIDs: []string{"disk1"},
		FallbackSensorPath: "/sys/class/hwmon/hwmon0/temp1_input", // may read 0 in CI; logging is what we assert
	}
	for range 3 {
		e.resolveTempForFan("hwmon0_fan1", src)
	}
	if n := strings.Count(buf.String(), "falling back"); n != 1 {
		t.Errorf("expected fallback logged once across 3 calls, got %d", n)
	}
}

func TestInterpolateSpeed(t *testing.T) {
	curve := []dto.FanCurvePoint{
		{TempCelsius: 30, SpeedPercent: 20},
		{TempCelsius: 50, SpeedPercent: 60},
		{TempCelsius: 70, SpeedPercent: 100},
	}

	tests := []struct {
		name   string
		points []dto.FanCurvePoint
		temp   float64
		want   int
	}{
		{"empty points defaults to full", nil, 40, 100},
		{"below lowest clamps to first", curve, 10, 20},
		{"exactly lowest", curve, 30, 20},
		{"above highest clamps to last", curve, 90, 100},
		{"exactly highest", curve, 70, 100},
		{"midpoint first segment", curve, 40, 40},
		{"midpoint second segment", curve, 60, 80},
		{"quarter into first segment", curve, 35, 30},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := interpolateSpeed(tt.points, tt.temp); got != tt.want {
				t.Errorf("interpolateSpeed(%v) = %d, want %d", tt.temp, got, tt.want)
			}
		})
	}
}

func TestInterpolateSpeedDuplicateLowestBoundary(t *testing.T) {
	// When tempC equals the lowest point's temperature, interpolateSpeed hits
	// the "at/below lowest" clamp and returns the first point's speed before any
	// interpolation runs — even if the next point shares the same temperature.
	points := []dto.FanCurvePoint{
		{TempCelsius: 40, SpeedPercent: 30},
		{TempCelsius: 40, SpeedPercent: 80},
		{TempCelsius: 60, SpeedPercent: 100},
	}
	if got := interpolateSpeed(points, 40); got != 30 {
		t.Errorf("at lowest boundary got %d, want 30", got)
	}
}
