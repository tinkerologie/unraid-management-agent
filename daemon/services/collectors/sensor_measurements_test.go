package collectors

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tinkerologie/unraid-management-agent/daemon/dto"
)

func writeSensorFixture(t *testing.T, path, value string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestTelemetryTemperatureLabels(t *testing.T) {
	tests := []struct {
		label string
		value float64
		want  bool
	}{
		{"SYSTIN", 116, false}, {"AUXTIN1", 108, false}, {"AUXTIN2", 109, false}, {"AUXTIN3", 108, false},
		{"systin", 100, true}, {"auxTin12", 101, false}, {"SYSTIN", 33, true}, {"AUXTIN", 45, true},
		{"Agent0 Dimm0", 0, false}, {"Agent0 Dimm0", 38, true}, {"PECI Agent 0", 0, false}, {"PECI Agent 1", -1, false}, {"peci Agent 0", 42, true},
		{"External Probe", 0, true}, {"External Probe", -10, true}, {"CPUTIN", 116, true},
		{"Core 0", 110, true}, {"Package id 0", 120, true}, {"VRM", 110, true},
		{"AUXTIN extra", 110, false}, {"SYSTIN", 101, false}, {"VRM", 125, false},
		{"VRM", -40, false}, {"VRM", math.NaN(), false}, {"VRM", math.Inf(1), false},
	}
	c := &SystemCollector{sysfsRoot: t.TempDir()}
	var output strings.Builder
	output.WriteString("nct6793-isa-0290\nAdapter: ISA adapter\n")
	for i, tt := range tests {
		t.Run(fmt.Sprintf("%d-%s", i, tt.label), func(t *testing.T) {
			if got := plausibleTelemetryTemp(tt.label, tt.value); got != tt.want {
				t.Errorf("plausibility = %v, want %v", got, tt.want)
			}
			// Exercise raw label filtering in sensors -u, before names are normalised.
			out := fmt.Sprintf("nct6793-isa-0290\n%s:\n  temp1_input: %v\n", tt.label, tt.value)
			if got := len(c.parseSensorsOutput(out)) > 0; got != tt.want {
				t.Errorf("sensors retained = %v, want %v", got, tt.want)
			}
			// Exercise the same policy in sysfs with the original label.
			path := filepath.Join(c.sysfsRoot, "class/hwmon/hwmon0")
			writeSensorFixture(t, filepath.Join(path, "temp1_input"), fmt.Sprint(tt.value*1000))
			writeSensorFixture(t, filepath.Join(path, "temp1_label"), tt.label)
			temps, _ := c.readHwmonTemperatures()
			if got := len(temps) > 0; got != tt.want {
				t.Errorf("hwmon retained = %v, want %v", got, tt.want)
			}
		})
		fmt.Fprintf(&output, "%s:\n  temp%d_input: %v\n", tt.label, i+1, tt.value)
	}
	// A full board inventory still contains CPU, core and package channels.
	temps := c.parseSensorsOutput(output.String())
	for _, label := range []string{"CPUTIN", "Core_0", "Package_id_0"} {
		found := false
		for name := range temps {
			if strings.Contains(name, label) {
				found = true
			}
		}
		if !found {
			t.Errorf("lost healthy %s channel", label)
		}
	}
}

func TestFanChannelQuarantine(t *testing.T) {
	for _, source := range []string{"sensors", "sysfs"} {
		t.Run(source, func(t *testing.T) {
			now := time.Unix(1000, 0)
			c := &SystemCollector{sysfsRoot: t.TempDir(), now: func() time.Time { return now }}
			path := filepath.Join(c.sysfsRoot, "class/hwmon/hwmon0")
			writeSensorFixture(t, filepath.Join(path, "name"), "nct6793")
			writeSensorFixture(t, filepath.Join(path, "fan1_input"), "25000")
			read := func(rpm int) map[string]int {
				t.Helper()
				if source == "sysfs" {
					writeSensorFixture(t, filepath.Join(path, "fan5_input"), fmt.Sprint(rpm))
					// Changing a friendly label must not release the source channel.
					writeSensorFixture(t, filepath.Join(path, "fan5_label"), fmt.Sprintf("Fan five %d", rpm))
					fans, err := c.readHwmonFanSpeeds()
					if err != nil {
						t.Fatal(err)
					}
					return fans
				}
				fans := c.parseFanSpeeds(fmt.Sprintf("nct6793-isa-0290\nfan1:\n  fan1_input: 25000\nfan5:\n  fan5_input: %d\n", rpm), nil)
				result := map[string]int{}
				for _, fan := range fans {
					result[fan.Name] = fan.RPM
				}
				return result
			}
			healthyName := "nct6793_fan1"
			if source == "sysfs" {
				healthyName = "Fan 1"
			}
			checks := []struct {
				after   time.Duration
				rpm     int
				visible bool
			}{
				{0, 1448, true}, {time.Second, 2954, true}, {time.Second, 56784, false},
				{time.Minute, 1448, false}, {14*time.Minute - time.Second, 2954, false},
				// Another bad reading extends quarantine rather than allowing early recovery.
				{0, 56784, false}, {14 * time.Minute, 1448, false}, {time.Minute, 2954, true},
			}
			for i, check := range checks {
				now = now.Add(check.after)
				fans := read(check.rpm)
				if fans[healthyName] != 25000 {
					t.Errorf("step %d: healthy fan lost: %v", i, fans)
				}
				if got := len(fans) == 2; got != check.visible {
					t.Errorf("step %d: visible=%v, want %v (%v)", i, got, check.visible, fans)
				}
			}
			if len(c.fanQuarantine) != 0 {
				t.Errorf("expired quarantine not removed: %v", c.fanQuarantine)
			}
		})
	}
}

func TestNullableMotherboardTemperature(t *testing.T) {
	info := &dto.SystemInfo{}
	populateSystemTemperatures(info, map[string]float64{"coretemp_Core_0": 45}, nil)
	if info.MotherboardTemp != nil {
		t.Fatal("CPU reading fabricated motherboard temperature")
	}
	populateSystemTemperatures(info, map[string]float64{"chip_MB_Temp_temp1_input": 0}, nil)
	if info.MotherboardTemp == nil || *info.MotherboardTemp != 0 {
		t.Fatal("measured motherboard zero was lost")
	}
}

func TestDiskNullableTemperatureAndSpinState(t *testing.T) {
	c := &DiskCollector{}
	tests := []struct {
		value string
		known bool
		temp  float64
		spin  string
	}{
		{"*", false, 0, "standby"}, {"", false, 0, "standby"}, {"garbled", false, 0, "standby"},
		{"NaN", false, 0, "standby"}, {"0", true, 0, "active"}, {"35", true, 35, "active"},
	}
	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			// /dev/null is an existing device on both development and target platforms;
			// enrichWithSpinState only stats it and never issues a disk command.
			disk := &dto.DiskInfo{Device: "null"}
			c.parseDiskKeyValue(disk, "temp="+tt.value)
			if (disk.Temperature != nil) != tt.known {
				t.Fatalf("temperature=%v, known=%v", disk.Temperature, tt.known)
			}
			if tt.known && *disk.Temperature != tt.temp {
				t.Errorf("temperature=%v, want %v", *disk.Temperature, tt.temp)
			}
			c.enrichWithSpinState(disk)
			if disk.SpinState != tt.spin {
				t.Errorf("spin state=%s, want %s", disk.SpinState, tt.spin)
			}
		})
	}
}
