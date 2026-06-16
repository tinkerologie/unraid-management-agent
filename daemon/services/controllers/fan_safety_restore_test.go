package controllers

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

// newTempFan creates pwm + pwm_enable sysfs-style files for one fan and returns
// the paths plus the fan ID, wired into the provider's fanMap.
func newTempFan(t *testing.T, p *HwmonProvider, fanID string, pwm, enable string) (pwmPath, enablePath string) {
	t.Helper()
	dir := t.TempDir()
	pwmPath = filepath.Join(dir, "pwm")
	enablePath = filepath.Join(dir, "pwm_enable")
	if err := os.WriteFile(pwmPath, []byte(pwm), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(enablePath, []byte(enable), 0o644); err != nil {
		t.Fatal(err)
	}
	p.fanMap[fanID] = hwmonFanPaths{pwmPath: pwmPath, enablePath: enablePath, hwmonDir: dir, fanIndex: 1}
	return pwmPath, enablePath
}

func readTrim(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path) // #nosec G304 -- test path
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(b))
}

// RestoreAll must restore only the fans the agent actually modified, leaving
// fans it never touched (e.g. owned by an external controller) alone.
func TestRestoreAllRestoresOnlyModifiedFans(t *testing.T) {
	hw := NewHwmonProvider()
	// fanA: agent will modify it. fanB: agent never touches it.
	aPWM, aEnable := newTempFan(t, hw, "hwmon0_fan1", "100", "1")
	bPWM, bEnable := newTempFan(t, hw, "hwmon0_fan2", "50", "1")

	g := NewFanSafetyGuard(hw, dto.FanSafetyConfig{})
	g.CaptureState([]dto.FanDevice{
		{ID: "hwmon0_fan1", Controllable: true, Mode: dto.FanModeManual, PWMValue: 100},
		{ID: "hwmon0_fan2", Controllable: true, Mode: dto.FanModeManual, PWMValue: 50},
	})

	// Agent modifies only fanA.
	if err := hw.SetMode("hwmon0_fan1", dto.FanModeAutomatic); err != nil {
		t.Fatal(err)
	}
	if err := hw.SetPWM("hwmon0_fan1", 255); err != nil {
		t.Fatal(err)
	}

	// An external controller takes fanB to a distinctive state the agent must
	// not clobber on shutdown.
	if err := os.WriteFile(bEnable, []byte("2"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bPWM, []byte("200"), 0o644); err != nil {
		t.Fatal(err)
	}

	g.RestoreAll()

	// fanA restored to its captured original (manual / 100).
	if got := readTrim(t, aEnable); got != "1" {
		t.Errorf("fanA enable = %q, want \"1\" (restored to manual)", got)
	}
	if got := readTrim(t, aPWM); got != "100" {
		t.Errorf("fanA pwm = %q, want \"100\" (restored)", got)
	}

	// fanB untouched — still the external controller's values.
	if got := readTrim(t, bEnable); got != "2" {
		t.Errorf("fanB enable = %q, want \"2\" (agent must not restore unmodified fan)", got)
	}
	if got := readTrim(t, bPWM); got != "200" {
		t.Errorf("fanB pwm = %q, want \"200\" (agent must not restore unmodified fan)", got)
	}
}
