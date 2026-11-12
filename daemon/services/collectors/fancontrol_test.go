package collectors

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/tinkerologie/unraid-management-agent/daemon/dto"
)

// The published fan status must always carry external-control state so the
// dashboard/MQTT feed can show when the agent is deferring to a third-party
// fan plugin (even when the agent's own control is disabled).
func TestCollectorBuildStatusIncludesExternalControl(t *testing.T) {
	c := &FanControlCollector{}
	st := c.buildStatus()
	if st.ExternalControl == nil {
		t.Fatal("expected buildStatus to populate ExternalControl")
	}
}

// TestCollectorBuildStatusGPUFans checks that a fan on a discrete GPU's hwmon
// device keeps its ID and controllability and gets a GPU name and metadata.
func TestCollectorBuildStatusGPUFans(t *testing.T) {
	installGPUFanLspci(t)
	root := buildFakeSysfs(t, []fakeHwmon{
		{hwmon: "hwmon2", name: "amdgpu", devRel: "pci0000:00/0000:00:01.1/0000:03:00.0"},
		{hwmon: "hwmon6", name: "xe", devRel: arcB70PCIPath},
		{hwmon: "hwmon7", name: "nct6687", devRel: "platform/nct6687.2592"},
	})
	writeHwmonFiles(t, root, "hwmon2", "0x030000", map[string]string{"fan1_input": "1500", "pwm1": "255", "pwm1_enable": "1"})
	writeHwmonFiles(t, root, "hwmon6", "0x030000", map[string]string{"fan1_input": "811"})
	writeHwmonFiles(t, root, "hwmon7", "", map[string]string{
		"fan1_input": "1005", "fan1_label": "CPU Fan", "pwm1": "128", "pwm1_enable": "2", "fan2_input": "0", "fan3_input": "818",
	})
	classDir := filepath.Join(root, "class", "hwmon")

	st := (&FanControlCollector{hwmonBasePath: classDir}).buildStatus()
	want := []dto.FanDevice{
		{ID: "hwmon2_fan1", Name: "AMD Radeon RX 7900 XTX Fan", RPM: 1500, PWMValue: 255, PWMPercent: 100, Mode: dto.FanModeManual,
			Controllable: true, HwmonPath: filepath.Join(classDir, "hwmon2"), HwmonIndex: 1, Role: dto.FanRoleGPU, PCIID: "0000:03:00.0"},
		{ID: "hwmon6_fan1", Name: "Intel Arc Pro B70 Fan", RPM: 811, Mode: dto.FanModeAutomatic,
			HwmonPath: filepath.Join(classDir, "hwmon6"), HwmonIndex: 1, Role: dto.FanRoleGPU, PCIID: "0000:04:00.0"},
		{ID: "hwmon7_fan1", Name: "CPU Fan", RPM: 1005, PWMValue: 128, PWMPercent: 50, Mode: dto.FanModeAutomatic,
			Controllable: true, HwmonPath: filepath.Join(classDir, "hwmon7"), HwmonIndex: 1},
		{ID: "hwmon7_fan2", Name: "Fan 2", Mode: dto.FanModeAutomatic, HwmonPath: filepath.Join(classDir, "hwmon7"), HwmonIndex: 2},
		{ID: "hwmon7_fan3", Name: "Fan 3", RPM: 818, Mode: dto.FanModeAutomatic, HwmonPath: filepath.Join(classDir, "hwmon7"), HwmonIndex: 3},
	}
	if !reflect.DeepEqual(st.Fans, want) {
		t.Errorf("buildStatus().Fans =\n  %+v\nwant\n  %+v", st.Fans, want)
	}
	if st.Summary.TotalFans != 5 || st.Summary.ControllableFans != 2 {
		t.Errorf("buildStatus().Summary = %+v, want 5 fans, 2 controllable", st.Summary)
	}
}
