package collectors

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestIntelNullableSample(t *testing.T) {
	tests := []struct {
		name, sample        string
		power, memory, util bool
		wantPower, wantUtil float64
	}{
		{"absent", `{"engines":{"Render/3D/0":{"busy":25}}}`, false, false, false, 0, 0},
		{"unavailable", `{"power":{"GPU":null},"memory":{"total":null,"shared":null}}`, false, false, false, 0, 0},
		{"measured zero", `{"power":{"GPU":0},"memory":{"total":100,"shared":0}}`, true, true, true, 0, 0},
		{"measured values", `{"power":{"GPU":12.5},"memory":{"total":100,"shared":25}}`, true, true, true, 12.5, 25},
		{"zero total", `{"power":{"GPU":0},"memory":{"total":0,"shared":0}}`, true, true, false, 0, 0},
		{"invalid memory", `{"memory":{"total":-1,"shared":"unknown"}}`, false, false, false, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var data map[string]any
			if err := json.Unmarshal([]byte(tt.sample), &data); err != nil {
				t.Fatal(err)
			}
			gpu := parseIntelGPUSample(data, "0000:00:02.0", "UHD", 0)
			if (gpu.PowerDraw != nil) != tt.power || (gpu.MemoryTotal != nil && gpu.MemoryUsed != nil) != tt.memory || (gpu.UtilizationMemory != nil) != tt.util {
				t.Fatalf("unexpected availability: %+v", gpu)
			}
			if gpu.Temperature != nil {
				t.Error("sample invented a temperature")
			}
			if tt.power && *gpu.PowerDraw != tt.wantPower {
				t.Errorf("power=%v, want %v", *gpu.PowerDraw, tt.wantPower)
			}
			if tt.util && *gpu.UtilizationMemory != tt.wantUtil {
				t.Errorf("memory utilisation=%v, want %v", *gpu.UtilizationMemory, tt.wantUtil)
			}
		})
	}
}

func TestIntelPCIBoundMetadata(t *testing.T) {
	for _, driver := range []string{"i915", "xe"} {
		t.Run(driver, func(t *testing.T) {
			root := t.TempDir()
			c := &GPUCollector{sysfsRoot: root}
			pci := "0000:00:02.0"
			dev := filepath.Join(root, "bus/pci/devices", pci)
			driverPath := filepath.Join(root, "bus/pci/drivers", driver)
			if err := os.MkdirAll(driverPath, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(dev, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(driverPath, filepath.Join(dev, "driver")); err != nil {
				t.Fatal(err)
			}
			version, err := c.getIntelDriverVersion(pci)
			if err != nil || version != driver {
				t.Fatalf("unversioned driver=%q, err=%v", version, err)
			}
			writeSensorFixture(t, filepath.Join(root, "module", driver, "version"), "1.2.3\n")
			version, err = c.getIntelDriverVersion(pci)
			if err != nil || version != driver+" 1.2.3" {
				t.Fatalf("versioned driver=%q, err=%v", version, err)
			}
			// A different GPU's hwmon is never used as this Intel device's reading.
			writeSensorFixture(t, filepath.Join(root, "bus/pci/devices/0000:01:00.0/hwmon/hwmon0/temp1_input"), "99000")
			if temp, err := c.getIntelGPUTemp(pci); err == nil {
				t.Errorf("borrowed temperature %v from another device", temp)
			}
			writeSensorFixture(t, filepath.Join(dev, "hwmon/hwmon1/temp1_input"), "garbled")
			if _, err := c.getIntelGPUTemp(pci); err == nil {
				t.Error("garbled temperature accepted")
			}
			writeSensorFixture(t, filepath.Join(dev, "hwmon/hwmon1/temp1_input"), "0")
			if temp, err := c.getIntelGPUTemp(pci); err != nil || temp != 0 {
				t.Errorf("measured zero=%v, err=%v", temp, err)
			}
			writeSensorFixture(t, filepath.Join(dev, "hwmon/hwmon1/temp1_input"), "42000")
			if temp, err := c.getIntelGPUTemp(pci); err != nil || temp != 42 {
				t.Errorf("bound temp=%v, err=%v", temp, err)
			}
		})
	}
}

func TestNvidiaNullableMeasurements(t *testing.T) {
	for _, marker := range []string{"[N/A]", "[Not Supported]", "", "garbled", "NaN", "Inf", "0"} {
		t.Run(marker, func(t *testing.T) {
			out := "0, pci, uuid, GPU, " + marker + ", 0, " + marker + ", " + marker + ", " + marker + ", 0\n"
			gpus, err := parseNvidiaGPUCSV(out)
			if err != nil || len(gpus) != 1 {
				t.Fatalf("parse: %v, %v", gpus, err)
			}
			gpu := gpus[0]
			known := marker == "0"
			if (gpu.Temperature != nil) != known || (gpu.PowerDraw != nil) != known || (gpu.MemoryTotal != nil) != known || (gpu.MemoryUsed != nil) != known {
				t.Errorf("unexpected nullable readings: %+v", gpu)
			}
			if known && (*gpu.Temperature != 0 || *gpu.PowerDraw != 0 || *gpu.MemoryTotal != 0 || *gpu.MemoryUsed != 0) {
				t.Error("zero measurement lost")
			}
			if gpu.UtilizationMemory != nil {
				t.Error("memory utilisation must be unknown without a positive total")
			}
		})
	}
}
