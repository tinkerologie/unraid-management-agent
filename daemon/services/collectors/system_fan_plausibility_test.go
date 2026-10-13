package collectors

import (
	"math"
	"os"
	"path/filepath"
	"testing"
)

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
	if !c.droppedFans["nct6793_fan5"] || !c.droppedFans["nct6793_fan6"] || !c.droppedFans["nct6793_fan7"] {
		t.Errorf("dropped channels not recorded: %v", c.droppedFans)
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

func TestLogDroppedFanOncePerChannel(t *testing.T) {
	c := &SystemCollector{}
	c.logDroppedFan("nct6793_fan5", 56784)
	c.logDroppedFan("nct6793_fan5", 56784)
	if len(c.droppedFans) != 1 {
		t.Errorf("droppedFans = %v, want one entry", c.droppedFans)
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
	if len(got) != 1 || got["CPU Fan"] != 1500 {
		t.Errorf("readHwmonFanSpeeds() = %v, want only CPU Fan at 1500", got)
	}
	if !c.droppedFans["Fan 2"] {
		t.Errorf("implausible sysfs reading not recorded as dropped: %v", c.droppedFans)
	}
}
