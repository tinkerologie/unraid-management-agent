package lib

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestDiscoverHwmonTempSensorsPlausibility(t *testing.T) {
	// Pure-logic guard: ensures unreliable labels and out-of-range temps are
	// flagged (not silently dropped). Real sysfs scanning is integration-tested
	// on hardware.
	if !classifyTempSensorPlausible("Tctl", 45.0) {
		t.Errorf("normal CPU sensor should be plausible")
	}
	if classifyTempSensorPlausible("AUXTIN", 45.0) {
		t.Errorf("unreliable label should be flagged implausible")
	}
	if classifyTempSensorPlausible("Core 0", 200.0) {
		t.Errorf("out-of-range temp should be flagged implausible")
	}
}

func TestGPUFanName(t *testing.T) {
	tests := []struct {
		name         string
		label        string
		channel      int
		channelCount int
		want         string
	}{
		{"labelled channel", "Pump", 2, 3, "AMD Radeon RX 7900 XTX Pump"},
		{"only unlabelled channel", "", 1, 1, "Intel Arc Pro B70 Fan"},
		{"one of several unlabelled channels", "", 2, 3, "Intel Arc Pro B70 Fan 2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gpu := "Intel Arc Pro B70"
			if tt.label != "" {
				gpu = "AMD Radeon RX 7900 XTX"
			}
			if got := GPUFanName(gpu, tt.label, tt.channel, tt.channelCount); got != tt.want {
				t.Errorf("GPUFanName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDiscoverHwmonFansAt(t *testing.T) {
	lspciNames := map[string]string{
		"0000:04:00.0": `0000:04:00.0 "VGA compatible controller" "Intel Corporation" "Battlemage G31 [Arc Pro B70]" -p00 "Intel Corporation" "Device 1701"`,
		"0000:03:00.0": `0000:03:00.0 "VGA compatible controller" "Advanced Micro Devices, Inc. [AMD/ATI]" "Navi 31 [Radeon RX 7900 XTX]"`,
	}
	var lookups []string
	stubGPULspci(t, func(pciID string) (string, error) {
		lookups = append(lookups, pciID)
		return lspciNames[pciID], nil
	})
	classDir := buildFakeHwmonSysfs(t, []fakeHwmonDevice{
		// Intel Arc Pro B70 on xe: one fan channel without PWM control.
		{hwmon: "hwmon6", devRel: arcB70PCIPath, class: "0x030000", files: map[string]string{
			"name": "xe", "fan1_input": "811",
		}},
		// Motherboard Super I/O on an ISA platform device.
		{hwmon: "hwmon7", devRel: "platform/nct6687.2592", files: map[string]string{
			"name": "nct6687", "fan1_input": "1005", "fan1_label": "CPU Fan", "pwm1": "128", "pwm1_enable": "2",
			"fan2_input": "0", "fan3_input": "818",
		}},
		// AMD GPU with PWM control and two unlabelled fans.
		{hwmon: "hwmon2", devRel: "pci0000:00/0000:00:01.1/0000:03:00.0", class: "0x030000", files: map[string]string{
			"name": "amdgpu", "fan1_input": "1500", "pwm1": "255", "pwm1_enable": "1", "fan2_input": "1400",
		}},
		// NVMe drive: PCI but not display class.
		{hwmon: "hwmon0", devRel: "pci0000:00/0000:00:01.0/0000:01:00.0", class: "0x010802", files: map[string]string{
			"name": "nvme", "fan1_input": "900",
		}},
		// Integrated GPU without fan channels: no fans and no lspci lookup.
		{hwmon: "hwmon3", devRel: "pci0000:00/0000:00:02.0", class: "0x030000", files: map[string]string{
			"name": "i915", "temp1_input": "45000",
		}},
		// Virtual device without a device link: unknown, so unchanged.
		{hwmon: "hwmon1", devRel: "virtual/thermal/thermal_zone0", link: "-", files: map[string]string{
			"name": "acpitz", "fan1_input": "700", "fan1_label": "Case",
		}},
	})

	want := []HwmonFan{
		{ID: "hwmon0_fan1", Name: "Fan 1", RPM: 900, FanIndex: 1},
		{ID: "hwmon1_fan1", Name: "Case", RPM: 700, FanIndex: 1},
		{ID: "hwmon2_fan1", Name: "AMD Radeon RX 7900 XTX Fan 1", RPM: 1500, PWMValue: 255, PWMPercent: 100, Mode: 1, HasPWM: true, FanIndex: 1, Role: "gpu", PCIID: "0000:03:00.0"},
		{ID: "hwmon2_fan2", Name: "AMD Radeon RX 7900 XTX Fan 2", RPM: 1400, FanIndex: 2, Role: "gpu", PCIID: "0000:03:00.0"},
		{ID: "hwmon6_fan1", Name: "Intel Arc Pro B70 Fan", RPM: 811, FanIndex: 1, Role: "gpu", PCIID: "0000:04:00.0"},
		{ID: "hwmon7_fan1", Name: "CPU Fan", RPM: 1005, PWMValue: 128, PWMPercent: 50, Mode: 2, HasPWM: true, FanIndex: 1},
		{ID: "hwmon7_fan2", Name: "Fan 2", FanIndex: 2},
		{ID: "hwmon7_fan3", Name: "Fan 3", RPM: 818, FanIndex: 3},
	}
	got := DiscoverHwmonFansAt(classDir)
	if len(got) != len(want) {
		t.Fatalf("DiscoverHwmonFansAt() returned %d fans %+v, want %d", len(got), got, len(want))
	}
	for i := range want {
		hwmon, _, _ := strings.Cut(want[i].ID, "_")
		want[i].HwmonDir = filepath.Join(classDir, hwmon)
		if got[i] != want[i] {
			t.Errorf("fan %d =\n  %+v\nwant\n  %+v", i, got[i], want[i])
		}
	}
	// lspci runs once per GPU with fan channels; the GPU without fans is not looked up.
	if want := []string{"0000:03:00.0", "0000:04:00.0"}; !reflect.DeepEqual(lookups, want) {
		t.Errorf("lspci lookups = %v, want %v", lookups, want)
	}
}

func TestDiscoverHwmonFansUsesDefaultPath(t *testing.T) {
	ids := func(fans []HwmonFan) []string {
		out := make([]string, 0, len(fans))
		for _, fan := range fans {
			out = append(out, fan.ID)
		}
		return out
	}
	got, want := ids(DiscoverHwmonFans()), ids(DiscoverHwmonFansAt(HwmonBasePath))
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("DiscoverHwmonFans() IDs = %v, want %v from DiscoverHwmonFansAt(HwmonBasePath)", got, want)
	}
}
