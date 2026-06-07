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
	for i := 0; i < 3; i++ {
		e.resolveTempForFan("hwmon0_fan1", src)
	}
	if n := strings.Count(buf.String(), "falling back"); n != 1 {
		t.Errorf("expected fallback logged once across 3 calls, got %d", n)
	}
}
