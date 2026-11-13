package collectors

import (
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestGetFansQuarantineSurvivesSysfsFallback(t *testing.T) {
	root := buildFakeSysfs(t, []fakeHwmon{{hwmon: "hwmon0", name: "nct6793", devRel: "platform/nct6775.656"}})
	hwmon := filepath.Join(root, "class", "hwmon", "hwmon0")
	writeSensorFixture(t, filepath.Join(hwmon, "fan5_input"), "1448")
	writeSensorFixture(t, filepath.Join(hwmon, "fan5_label"), "CPU Fan")
	bin := t.TempDir()
	output := filepath.Join(bin, "output")
	writeSensorFixture(t, output, "nct6793-isa-0290\nfan5:\n  fan5_input: 56784\n")
	if err := os.WriteFile(filepath.Join(bin, "sensors"), []byte("#!/bin/sh\n/bin/cat '"+output+"'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	now := time.Unix(1000, 0)
	c := &SystemCollector{sysfsRoot: root, now: func() time.Time { return now }}
	checks := []struct {
		name   string
		after  time.Duration
		output string
		fail   bool
	}{
		{"implausible sensors with plausible sysfs", 0, "nct6793-isa-0290\nfan5:\n  fan5_input: 56784\n", false},
		{"plausible sensors still quarantined", time.Minute, "nct6793-isa-0290\nfan5:\n  fan5_input: 1448\n", false},
		{"sensors absent during hold", time.Minute, "", false},
		{"sensors command fails during hold", time.Minute, "", true},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			now = now.Add(check.after)
			writeSensorFixture(t, output, check.output)
			if check.fail {
				if err := os.Remove(filepath.Join(bin, "sensors")); err != nil {
					t.Fatal(err)
				}
			}
			fans, _ := c.getFans(nil)
			if len(fans) != 0 {
				t.Errorf("getFans() = %+v, want quarantined channel hidden", fans)
			}
		})
	}
	now = now.Add(fanQuarantineHold - 3*time.Minute)
	fans, err := c.getFans(nil)
	if err != nil || len(fans) != 1 || fans[0].RPM != 1448 {
		t.Fatalf("getFans() after hold = %+v, %v, want recovered sysfs fan", fans, err)
	}
}

func TestPlausibleFanRPM(t *testing.T) {
	tests := []struct {
		value float64
		want  bool
	}{
		{0, true},
		{742, true},
		{25000, true},
		{25000.4, false},
		{25001, false},
		{56784, false},
		{-1, false},
		{math.NaN(), false},
		{math.Inf(1), false},
	}
	for _, tt := range tests {
		if got := plausibleFanRPM(tt.value); got != tt.want {
			t.Errorf("plausibleFanRPM(%v) = %v, want %v", tt.value, got, tt.want)
		}
	}
}

func TestFanQuarantineSharesDeviceAndChannelAcrossSources(t *testing.T) {
	tests := []struct {
		name, chip, otherChip string
		devices               []fakeHwmon
	}{
		{"ISA", "nct6793-isa-0290", "nct6793-isa-0298", []fakeHwmon{
			{hwmon: "hwmon0", name: "nct6793", devRel: "platform/nct6775.656"},
			{hwmon: "hwmon1", name: "nct6793", devRel: "platform/nct6775.664"},
		}},
		{"I2C", "nct6793-i2c-0-2d", "nct6793-i2c-1-2d", []fakeHwmon{
			{hwmon: "hwmon0", name: "nct6793", devRel: "i2c/0-002d"},
			{hwmon: "hwmon1", name: "nct6793", devRel: "i2c/1-002d"},
		}},
		{"PCI", "nct6793-pci-0300", "nct6793-pci-0400", []fakeHwmon{
			{hwmon: "hwmon0", name: "nct6793", devRel: "pci0000:00/0000:03:00.0"},
			{hwmon: "hwmon1", name: "nct6793", devRel: "pci0000:00/0000:04:00.0"},
		}},
		{"HID", "nct6793-hid-3-13", "nct6793-hid-3-14", []fakeHwmon{
			{hwmon: "hwmon0", name: "nct6793", devRel: "usb3/3-1/3-1:1.0/0003:0C70:F011.0013", usbRel: "usb3/3-1", usb: map[string]string{"idVendor": "0c70", "serial": "first"}},
			{hwmon: "hwmon1", name: "nct6793", devRel: "usb3/3-2/3-2:1.0/0003:0C70:F011.0014", usbRel: "usb3/3-2", usb: map[string]string{"idVendor": "0c70", "serial": "second"}},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := buildFakeSysfs(t, tt.devices)
			first := filepath.Join(root, "class/hwmon/hwmon0")
			second := filepath.Join(root, "class/hwmon/hwmon1")
			writeSensorFixture(t, filepath.Join(first, "fan1_input"), "1200")
			writeSensorFixture(t, filepath.Join(first, "fan1_label"), "Healthy channel")
			writeSensorFixture(t, filepath.Join(first, "fan5_input"), "1448")
			writeSensorFixture(t, filepath.Join(first, "fan5_label"), "Noisy channel")
			writeSensorFixture(t, filepath.Join(second, "fan5_input"), "1500")
			writeSensorFixture(t, filepath.Join(second, "fan5_label"), "Other device")
			c := &SystemCollector{sysfsRoot: root}
			c.parseFanSpeeds(tt.chip+"\nfan5:\n  fan5_input: 56784\n", hwmonChipDeviceIDs(root))
			fans, err := c.readHwmonFanSpeeds()
			if err != nil || len(fans) != 2 || fans["Healthy channel"].RPM != 1200 || fans["Other device"].RPM != 1500 {
				t.Fatalf("sysfs after bad sensors channel = %v, %v; want healthy channel and other device", fans, err)
			}
			// The reverse transition must use the same identity too.
			c = &SystemCollector{sysfsRoot: root}
			writeSensorFixture(t, filepath.Join(first, "fan5_input"), "56784")
			if _, err := c.readHwmonFanSpeeds(); err != nil {
				t.Fatal(err)
			}
			got := c.parseFanSpeeds(tt.chip+"\nfan5:\n  fan5_input: 1448\n\n"+tt.otherChip+"\nfan5:\n  fan5_input: 1500\n", hwmonChipDeviceIDs(root))
			if len(got) != 1 || got[0].Source != tt.otherChip || got[0].RPM != 1500 {
				t.Errorf("sensors after bad sysfs channel = %+v, want only other device", got)
			}
		})
	}
}

func TestParseFanSpeedsDropsImplausibleReadings(t *testing.T) {
	output := `nct6793-isa-0290
Adapter: ISA adapter
fan1:
  fan1_input: 796.000
fan2:
  fan2_input: 0.000
fan3:
  fan3_input: 25000.000
fan5:
  fan5_input: 56784.000
fan6:
  fan6_input: nan
fan7:
  fan7_input: -5.000
`
	c := &SystemCollector{}
	got := c.parseFanSpeeds(output, nil)

	want := map[string]int{"nct6793_fan1": 796, "nct6793_fan2": 0, "nct6793_fan3": 25000}
	if len(got) != len(want) {
		t.Fatalf("parseFanSpeeds() returned %d fans %+v, want %d", len(got), got, len(want))
	}
	for _, f := range got {
		if rpm, ok := want[f.Name]; !ok || rpm != f.RPM {
			t.Errorf("unexpected fan %s=%d", f.Name, f.RPM)
		}
	}
	if len(c.fanQuarantine) != 3 {
		t.Errorf("quarantined channels = %v", c.fanQuarantine)
	}
}

func TestParseFanSpeedsKeepsNamesWhenOneChipIsDropped(t *testing.T) {
	// Two chips of the same model: names carry the chip suffix even when one
	// chip's only reading is implausible, so the other fan's name is stable.
	output := `octo-hid-3-13
Adapter: HID adapter
fan1:
  fan1_input: 1200.000

octo-hid-3-14
Adapter: HID adapter
fan1:
  fan1_input: 65535.000
`
	c := &SystemCollector{}
	got := c.parseFanSpeeds(output, nil)
	if len(got) != 1 || got[0].Name != "octo-hid-3-13_fan1" || got[0].RPM != 1200 {
		t.Errorf("parseFanSpeeds() = %+v, want only octo-hid-3-13_fan1 at 1200 RPM", got)
	}
}

func TestReadHwmonFanSpeedsDropsImplausibleReadings(t *testing.T) {
	root := t.TempDir()
	hwmon := filepath.Join(root, "class", "hwmon", "hwmon0")
	if err := os.MkdirAll(hwmon, 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"fan1_input": "1500\n",
		"fan1_label": "CPU Fan\n",
		"fan2_input": "56784\n",
		"fan3_input": "0\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(hwmon, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	c := &SystemCollector{sysfsRoot: root}
	got, err := c.readHwmonFanSpeeds()
	if err != nil {
		t.Fatalf("readHwmonFanSpeeds() error = %v", err)
	}
	if len(got) != 1 || got["CPU Fan"].RPM != 1500 {
		t.Errorf("readHwmonFanSpeeds() = %v, want only CPU Fan at 1500", got)
	}
	if len(c.fanQuarantine) != 1 {
		t.Errorf("quarantined channels = %v", c.fanQuarantine)
	}
}
