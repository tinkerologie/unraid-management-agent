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

func TestIntelTemperatureChannelSelection(t *testing.T) {
	const pci = "0000:04:00.0"
	dev := "bus/pci/devices/" + pci + "/hwmon/"
	for _, tt := range []struct {
		name  string
		files map[string]string
		want  *float64
	}{
		{"xe package and vram without temp1", map[string]string{
			dev + "hwmon3/temp2_input": "46000", dev + "hwmon3/temp2_label": "pkg\n",
			dev + "hwmon3/temp3_input": "48000", dev + "hwmon3/temp3_label": "vram\n",
		}, new(46.0)},
		{"unlabelled temp1", map[string]string{dev + "hwmon0/temp1_input": "42500"}, new(42.5)},
		{"padded mixed-case package label", map[string]string{
			dev + "hwmon0/temp1_input": "30000",
			dev + "hwmon0/temp2_input": "45000", dev + "hwmon0/temp2_label": "  Package \n",
		}, new(45.0)},
		{"NaN package falls back to non-vram channel", map[string]string{
			dev + "hwmon0/temp2_input": "NaN", dev + "hwmon0/temp2_label": "pkg",
			dev + "hwmon0/temp3_input": "48000", dev + "hwmon0/temp3_label": "vram",
			dev + "hwmon0/temp4_input": "44000",
		}, new(44.0)},
		{"garbled package falls back to non-vram channel", map[string]string{
			dev + "hwmon0/temp2_input": "garbled", dev + "hwmon0/temp2_label": "pkg",
			dev + "hwmon0/temp3_input": "48000", dev + "hwmon0/temp3_label": "vram",
			dev + "hwmon0/temp4_input": "44000", dev + "hwmon0/temp4_label": "gt",
		}, new(44.0)},
		{"vram only", map[string]string{
			dev + "hwmon0/temp3_input": "48000", dev + "hwmon0/temp3_label": "vram",
		}, nil},
		{"vram channel only", map[string]string{
			dev + "hwmon0/temp3_input": "48000", dev + "hwmon0/temp3_label": "vram_ch_0",
		}, nil},
		{"temp2 before temp10", map[string]string{
			dev + "hwmon0/temp10_input": "50000",
			dev + "hwmon0/temp2_input":  "41000",
		}, new(41.0)},
		{"hwmon2 before hwmon10", map[string]string{
			dev + "hwmon10/temp1_input": "50000",
			dev + "hwmon2/temp1_input":  "41000",
		}, new(41.0)},
		{"measured zero package", map[string]string{
			dev + "hwmon0/temp1_input": "30000",
			dev + "hwmon0/temp2_input": "0", dev + "hwmon0/temp2_label": "pkg",
		}, new(0.0)},
		{"package on another PCI device", map[string]string{
			"bus/pci/devices/0000:03:00.0/hwmon/hwmon0/temp2_input": "99000",
			"bus/pci/devices/0000:03:00.0/hwmon/hwmon0/temp2_label": "pkg",
			dev + "hwmon1/temp1_input":                              "41000",
		}, new(41.0)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			for path, value := range tt.files {
				writeSensorFixture(t, filepath.Join(root, path), value)
			}
			temp, err := (&GPUCollector{sysfsRoot: root}).getIntelGPUTemp(pci)
			if tt.want == nil {
				if err == nil {
					t.Errorf("temperature = %v, want unknown", temp)
				}
				return
			}
			if err != nil || temp != *tt.want {
				t.Errorf("temperature = %v, error = %v; want %v", temp, err, *tt.want)
			}
		})
	}
}

func TestGPUMemoryTemperatureChannel(t *testing.T) {
	const pci = "0000:04:00.0"
	dev := "bus/pci/devices/" + pci + "/hwmon/"
	for _, tt := range []struct {
		name  string
		label string
		files map[string]string
		want  *float64
	}{
		{"xe package and vram", "vram", map[string]string{
			dev + "hwmon6/temp2_input": "46000", dev + "hwmon6/temp2_label": "pkg\n",
			dev + "hwmon6/temp3_input": "48000", dev + "hwmon6/temp3_label": "vram\n",
		}, new(48.0)},
		{"padded mixed-case label", "vram", map[string]string{
			dev + "hwmon0/temp3_input": "48500", dev + "hwmon0/temp3_label": "  VRAM \n",
		}, new(48.5)},
		{"missing label", "vram", map[string]string{
			dev + "hwmon0/temp1_input": "42000",
			dev + "hwmon0/temp2_input": "46000", dev + "hwmon0/temp2_label": "pkg",
		}, nil},
		{"measured zero", "vram", map[string]string{
			dev + "hwmon0/temp3_input": "0", dev + "hwmon0/temp3_label": "vram",
		}, new(0.0)},
		{"malformed", "vram", map[string]string{
			dev + "hwmon0/temp2_input": "46000", dev + "hwmon0/temp2_label": "pkg",
			dev + "hwmon0/temp3_input": "garbled", dev + "hwmon0/temp3_label": "vram",
		}, nil},
		{"NaN", "vram", map[string]string{
			dev + "hwmon0/temp3_input": "NaN", dev + "hwmon0/temp3_label": "vram",
		}, nil},
		{"Inf", "vram", map[string]string{
			dev + "hwmon0/temp3_input": "Inf", dev + "hwmon0/temp3_label": "vram",
		}, nil},
		{"vram on another PCI device", "vram", map[string]string{
			"bus/pci/devices/0000:03:00.0/hwmon/hwmon0/temp3_input": "99000",
			"bus/pci/devices/0000:03:00.0/hwmon/hwmon0/temp3_label": "vram",
			dev + "hwmon1/temp2_input":                              "46000", dev + "hwmon1/temp2_label": "pkg",
		}, nil},
		{"own vram beside another PCI device", "vram", map[string]string{
			"bus/pci/devices/0000:03:00.0/hwmon/hwmon0/temp3_input": "99000",
			"bus/pci/devices/0000:03:00.0/hwmon/hwmon0/temp3_label": "vram",
			dev + "hwmon1/temp3_input":                              "51000", dev + "hwmon1/temp3_label": "vram",
		}, new(51.0)},
		{"amdgpu mem over junction", "mem", map[string]string{
			dev + "hwmon2/temp1_input": "45000", dev + "hwmon2/temp1_label": "edge\n",
			dev + "hwmon2/temp2_input": "60000", dev + "hwmon2/temp2_label": "junction\n",
			dev + "hwmon2/temp3_input": "52000", dev + "hwmon2/temp3_label": "mem\n",
		}, new(52.0)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			for path, value := range tt.files {
				writeSensorFixture(t, filepath.Join(root, path), value)
			}
			got := (&GPUCollector{sysfsRoot: root}).getGPUMemoryTemp(pci, tt.label)
			assertOptionalMeasurement(t, "memory temperature", got, tt.want)
		})
	}
	t.Run("malformed glob", func(t *testing.T) {
		c := &GPUCollector{sysfsRoot: filepath.Join(t.TempDir(), "[")}
		if got := c.getGPUMemoryTemp(pci, "vram"); got != nil {
			t.Errorf("memory temperature = %v, want unknown", *got)
		}
	})
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
			if gpu.MemoryTemperature != nil {
				t.Error("nvidia-smi output has no memory temperature")
			}
		})
	}
}
