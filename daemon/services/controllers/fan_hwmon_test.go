package controllers

import (
	"testing"

	"github.com/tinkerologie/unraid-management-agent/daemon/dto"
	"github.com/tinkerologie/unraid-management-agent/daemon/lib"
)

func TestFanDevicesFromHwmon(t *testing.T) {
	tests := []struct {
		name string
		fan  lib.HwmonFan
		want dto.FanDevice
	}{
		{
			name: "GPU fan without PWM",
			fan: lib.HwmonFan{
				ID: "hwmon6_fan1", Name: "Intel Arc Pro B70 Fan", RPM: 813, HwmonDir: "/sys/class/hwmon/hwmon6",
				FanIndex: 1, Role: dto.FanRoleGPU, PCIID: "0000:04:00.0",
			},
			want: dto.FanDevice{
				ID: "hwmon6_fan1", Name: "Intel Arc Pro B70 Fan", RPM: 813, HwmonPath: "/sys/class/hwmon/hwmon6",
				HwmonIndex: 1, Role: dto.FanRoleGPU, PCIID: "0000:04:00.0", Mode: dto.FanModeAutomatic,
			},
		},
		{
			name: "GPU fan with PWM in manual mode",
			fan: lib.HwmonFan{
				ID: "hwmon3_fan1", Name: "AMD Radeon RX 7900 XTX Fan", RPM: 1200, HwmonDir: "/sys/class/hwmon/hwmon3",
				FanIndex: 1, HasPWM: true, PWMValue: 128, PWMPercent: 50, Mode: 1, Role: dto.FanRoleGPU, PCIID: "0000:03:00.0",
			},
			want: dto.FanDevice{
				ID: "hwmon3_fan1", Name: "AMD Radeon RX 7900 XTX Fan", RPM: 1200, HwmonPath: "/sys/class/hwmon/hwmon3",
				HwmonIndex: 1, Controllable: true, PWMValue: 128, PWMPercent: 50, Mode: dto.FanModeManual,
				Role: dto.FanRoleGPU, PCIID: "0000:03:00.0",
			},
		},
		{
			name: "motherboard fan with PWM in automatic mode",
			fan: lib.HwmonFan{
				ID: "hwmon7_fan1", Name: "CPU Fan", RPM: 960, HwmonDir: "/sys/class/hwmon/hwmon7",
				FanIndex: 1, HasPWM: true, PWMValue: 90, PWMPercent: 35, Mode: 2,
			},
			want: dto.FanDevice{
				ID: "hwmon7_fan1", Name: "CPU Fan", RPM: 960, HwmonPath: "/sys/class/hwmon/hwmon7",
				HwmonIndex: 1, Controllable: true, PWMValue: 90, PWMPercent: 35, Mode: dto.FanModeAutomatic,
			},
		},
		{
			name: "motherboard fan with PWM off",
			fan:  lib.HwmonFan{ID: "hwmon7_fan3", Name: "System Fan #1", HwmonDir: "/sys/class/hwmon/hwmon7", FanIndex: 3, HasPWM: true, Mode: 0},
			want: dto.FanDevice{ID: "hwmon7_fan3", Name: "System Fan #1", HwmonPath: "/sys/class/hwmon/hwmon7", HwmonIndex: 3, Controllable: true, Mode: dto.FanModeOff},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fanDevicesFromHwmon([]lib.HwmonFan{tt.fan})
			if len(got) != 1 || got[0] != tt.want {
				t.Errorf("fanDevicesFromHwmon() = %+v, want [%+v]", got, tt.want)
			}
		})
	}
	if got := fanDevicesFromHwmon(nil); got == nil || len(got) != 0 {
		t.Errorf("fanDevicesFromHwmon(nil) = %#v, want an empty non-nil slice", got)
	}
}
