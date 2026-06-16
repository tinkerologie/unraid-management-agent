package controllers

import (
	"strings"
	"testing"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

// activeExternal injects a detector that reports the named plugin as active.
func activeExternal() func() dto.ExternalFanControl {
	return func() dto.ExternalFanControl {
		return dto.ExternalFanControl{Active: true, Controllers: []string{"FanCTRL Plus"}}
	}
}

// newDeferringController builds a minimal initialized controller that believes a
// third-party fan plugin is active.
func newDeferringController() *FanController {
	c := &FanController{
		hwmon:          NewHwmonProvider(),
		config:         dto.FanControlConfig{ControlEnabled: true},
		detectExternal: activeExternal(),
	}
	c.safety = NewFanSafetyGuard(c.hwmon, c.config.Safety)
	c.curves = NewFanCurveEngine(c.hwmon, c.safety)
	c.initialized = true
	return c
}

func TestWritesRefusedWhenExternalControlActive(t *testing.T) {
	c := newDeferringController()

	tests := []struct {
		name string
		fn   func() error
	}{
		{"SetSpeed", func() error { return c.SetSpeed("hwmon0_fan1", 50) }},
		{"SetMode", func() error { return c.SetMode("hwmon0_fan1", "manual") }},
		{"SetProfile", func() error {
			return c.SetProfile("hwmon0_fan1", "balanced", dto.FanTempSource{Type: dto.FanTempSourceHwmon, SensorPath: "/x"})
		}},
		{"RestoreDefaults", c.RestoreDefaults},
	}
	for _, tt := range tests {
		if err := tt.fn(); err == nil || !strings.Contains(err.Error(), "FanCTRL Plus") {
			t.Errorf("%s: expected deferral error naming the active plugin, got %v", tt.name, err)
		}
	}

	if mods := c.hwmon.ModifiedFans(); len(mods) != 0 {
		t.Errorf("expected zero fan writes while deferring, got %v", mods)
	}
}

func TestGetStatusReportsExternalControl(t *testing.T) {
	c := newDeferringController()

	st := c.GetStatus()
	if st.ExternalControl == nil || !st.ExternalControl.Active {
		t.Fatalf("expected ExternalControl.Active=true, got %+v", st.ExternalControl)
	}
	if len(st.ExternalControl.Controllers) != 1 || st.ExternalControl.Controllers[0] != "FanCTRL Plus" {
		t.Errorf("Controllers = %v, want [FanCTRL Plus]", st.ExternalControl.Controllers)
	}
}
