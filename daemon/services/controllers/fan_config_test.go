package controllers

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

func TestFanConfigMigrationLegacyAssignment(t *testing.T) {
	var a fanCurveAssignment
	legacy := []byte(`{"ProfileName":"balanced","TempSensorPath":"/sys/class/hwmon/hwmon0/temp1_input"}`)
	if err := json.Unmarshal(legacy, &a); err != nil {
		t.Fatal(err)
	}
	if a.ProfileName != "balanced" || a.Source.Type != dto.FanTempSourceHwmon ||
		a.Source.SensorPath != "/sys/class/hwmon/hwmon0/temp1_input" {
		t.Fatalf("legacy migration failed: %+v", a)
	}
}

func TestFanConfigNewShapeAssignment(t *testing.T) {
	var a fanCurveAssignment
	newShape := []byte(`{"profile_name":"balanced","source":{"type":"drives","drive_ids":["disk1"]}}`)
	if err := json.Unmarshal(newShape, &a); err != nil {
		t.Fatal(err)
	}
	if a.ProfileName != "balanced" || a.Source.Type != dto.FanTempSourceDrives || len(a.Source.DriveIDs) != 1 {
		t.Fatalf("new-shape parse failed: %+v", a)
	}
}

func TestFanConfigRoundTrip(t *testing.T) {
	store := NewFanConfigStore(t.TempDir())
	in := fanConfigData{
		Config: defaultFanControlConfig(),
		Assignments: map[string]fanCurveAssignment{
			"hwmon0_fan1": {ProfileName: "balanced", Source: dto.FanTempSource{Type: dto.FanTempSourceDrives, DriveIDs: []string{"disk1"}}},
		},
	}
	if err := store.Save(in); err != nil {
		t.Fatal(err)
	}
	out, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	got := out.Assignments["hwmon0_fan1"]
	if got.ProfileName != "balanced" || got.Source.Type != dto.FanTempSourceDrives || len(got.Source.DriveIDs) != 1 || got.Source.DriveIDs[0] != "disk1" {
		t.Fatalf("round-trip failed: %+v", got)
	}
}

func TestFanConfigLoadMigratesLegacyFile(t *testing.T) {
	dir := t.TempDir()
	// Legacy on-disk shape: capitalized keys, no "source" object.
	legacy := `{
  "config": {"enabled": true, "control_enabled": true, "control_method": "hwmon", "poll_interval_seconds": 5, "safety": {"min_speed_percent": 20, "critical_temp_celsius": 90, "failure_rpm_threshold": 100}},
  "assignments": {"hwmon0_fan1": {"ProfileName": "balanced", "TempSensorPath": "/sys/class/hwmon/hwmon0/temp1_input"}}
}`
	if err := os.WriteFile(filepath.Join(dir, "fancontrol.json"), []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	store := NewFanConfigStore(dir)
	out, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	got := out.Assignments["hwmon0_fan1"]
	if got.ProfileName != "balanced" || got.Source.Type != dto.FanTempSourceHwmon ||
		got.Source.SensorPath != "/sys/class/hwmon/hwmon0/temp1_input" {
		t.Fatalf("legacy file did not migrate via Load: %+v", got)
	}
}
