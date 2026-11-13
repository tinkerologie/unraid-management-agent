package collectors

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Only these local fixtures are reachable through PATH; no hardware tools run.
func writeCollectorCommand(t *testing.T, bin, name, script string) {
	t.Helper()
	path := filepath.Join(bin, name)
	writeSensorFixture(t, path, "#!/bin/sh\n"+script+"\n")
	// #nosec G302 -- Owner-only execution is required for temporary command fixtures.
	if err := os.Chmod(path, 0o700); err != nil {
		t.Fatal(err)
	}
}

func TestCollectSingleIntelGPUMeasurements(t *testing.T) {
	for _, tt := range []struct {
		name, milliC string
		metadata     bool
	}{
		{"measured temperature", "42500", true},
		{"measured zero", "0", true},
		{"missing metadata", "", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			bin, root := t.TempDir(), t.TempDir()
			t.Setenv("PATH", bin)
			writeCollectorCommand(t, bin, "timeout", `
[ "$*" = "2 intel_gpu_top -J -s 500 -n 1" ] || exit 1
printf '%s\n' '[{"engines":{"Render/3D/0":{"busy":30}},"power":{"GPU":0}}]'
`)
			const pci = "0000:00:02.0"
			if tt.metadata {
				dev := filepath.Join(root, "bus/pci/devices", pci)
				writeSensorFixture(t, filepath.Join(dev, "hwmon/hwmon0/temp1_input"), tt.milliC)
				driver := filepath.Join(root, "bus/pci/drivers/xe")
				if err := os.MkdirAll(driver, 0o750); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(driver, filepath.Join(dev, "driver")); err != nil {
					t.Fatal(err)
				}
				writeSensorFixture(t, filepath.Join(root, "module/xe/version"), " 1.2.3\n")
				writeCollectorCommand(t, bin, "bash", "printf '51000\\n'")
			}
			c := &GPUCollector{sysfsRoot: root}
			gpu := c.collectSingleIntelGPU(pci, "Arc", 2)
			if gpu == nil {
				t.Fatal("valid Intel sample was discarded")
			}
			if !gpu.Available || gpu.Index != 2 || gpu.PCIID != pci || gpu.Name != "Intel Arc" || gpu.Vendor != "intel" || gpu.UtilizationGPU != 30 {
				t.Errorf("incorrect GPU identity/utilisation: %+v", gpu)
			}
			if gpu.PowerDraw == nil || *gpu.PowerDraw != 0 || gpu.MemoryUsed != nil || gpu.MemoryTotal != nil || gpu.UtilizationMemory != nil {
				t.Errorf("measured zero power or unavailable shared memory lost: %+v", gpu)
			}
			if tt.metadata {
				wantTemp := 42.5
				if tt.milliC == "0" {
					wantTemp = 0
				}
				if gpu.Temperature == nil || *gpu.Temperature != wantTemp || gpu.CPUTemperature != 51 || gpu.DriverVersion != "xe 1.2.3" {
					t.Errorf("incorrect bound metadata: %+v", gpu)
				}
			} else if gpu.Temperature != nil || gpu.CPUTemperature != 0 || gpu.DriverVersion != "" {
				t.Errorf("missing metadata fabricated measurements: %+v", gpu)
			}
		})
	}
}

func TestCollectSingleIntelGPUTemperatureChannels(t *testing.T) {
	const pci = "0000:04:00.0"
	for _, tt := range []struct {
		name  string
		files map[string]string
		want  *float64
	}{
		{"xe battlemage package channel", map[string]string{
			"temp2_input": "46000", "temp2_label": "pkg\n",
			"temp3_input": "48000", "temp3_label": "vram\n",
		}, new(46.0)},
		{"no usable GPU channel", map[string]string{
			"temp3_input": "48000", "temp3_label": "vram\n",
		}, nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			bin, root := t.TempDir(), t.TempDir()
			t.Setenv("PATH", bin)
			writeCollectorCommand(t, bin, "timeout", `
[ "$*" = "2 intel_gpu_top -J -s 500 -n 1" ] || exit 1
printf '%s\n' '[{"engines":{"Render/3D/0":{"busy":30}},"power":{"GPU":0}}]'
`)
			writeCollectorCommand(t, bin, "bash", "printf '51000\\n'")
			hwmon := filepath.Join(root, "bus/pci/devices", pci, "hwmon/hwmon3")
			for name, value := range tt.files {
				writeSensorFixture(t, filepath.Join(hwmon, name), value)
			}
			gpu := (&GPUCollector{sysfsRoot: root}).collectSingleIntelGPU(pci, "Arc Pro B70", 0)
			if gpu == nil {
				t.Fatal("valid Intel sample was discarded")
			}
			assertOptionalMeasurement(t, "temperature", gpu.Temperature, tt.want)
			if gpu.CPUTemperature != 51 {
				t.Errorf("CPU temperature = %v, want 51 from coretemp", gpu.CPUTemperature)
			}
		})
	}
}

func TestIntelTemperatureReadFailures(t *testing.T) {
	t.Run("default sysfs mount", func(t *testing.T) {
		if got := (&GPUCollector{}).sysfs(); got != "/sys" {
			t.Errorf("zero-value sysfs mount = %q, want /sys", got)
		}
	})
	t.Run("malformed glob reports cause", func(t *testing.T) {
		c := &GPUCollector{sysfsRoot: filepath.Join(t.TempDir(), "[")}
		temp, err := c.getIntelGPUTemp("0000:00:02.0")
		if temp != 0 || !errors.Is(err, filepath.ErrBadPattern) || !strings.Contains(err.Error(), "finding Intel GPU temperature") {
			t.Fatalf("temperature = %v, error = %v; want contextual bad-pattern error", temp, err)
		}
	})
	t.Run("unreadable channel does not hide next reading", func(t *testing.T) {
		root := t.TempDir()
		dev := filepath.Join(root, "bus/pci/devices/0000:00:02.0/hwmon")
		// A directory is unreadable as a temperature file, even when run as root.
		if err := os.MkdirAll(filepath.Join(dev, "hwmon0/temp1_input"), 0o750); err != nil {
			t.Fatal(err)
		}
		c := &GPUCollector{sysfsRoot: root}
		if _, err := c.getIntelGPUTemp("0000:00:02.0"); err == nil {
			t.Fatal("unreadable channel reported as a measurement")
		}
		writeSensorFixture(t, filepath.Join(dev, "hwmon1/temp1_input"), "37500")
		if temp, err := c.getIntelGPUTemp("0000:00:02.0"); err != nil || temp != 37.5 {
			t.Errorf("temperature = %v, error = %v; want 37.5 from next channel", temp, err)
		}
	})
}

func TestIntelDriverReadFailures(t *testing.T) {
	root := t.TempDir()
	c := &GPUCollector{sysfsRoot: root}
	const pci = "0000:00:02.0"
	if driver, err := c.getIntelDriverVersion(pci); driver != "" || !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing driver = %q, %v; want unknown and not-exist error", driver, err)
	}
	driver := filepath.Join(root, "bus/pci/drivers/vfio-pci")
	if err := os.MkdirAll(driver, 0o750); err != nil {
		t.Fatal(err)
	}
	dev := filepath.Join(root, "bus/pci/devices", pci)
	if err := os.MkdirAll(dev, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(driver, filepath.Join(dev, "driver")); err != nil {
		t.Fatal(err)
	}
	if got, err := c.getIntelDriverVersion(pci); got != "" || err == nil || !strings.Contains(err.Error(), `unexpected Intel GPU driver "vfio-pci"`) {
		t.Errorf("non-Intel driver = %q, %v; want unknown and explicit rejection", got, err)
	}
}

func TestCollectSingleAMDGPUMeasurements(t *testing.T) {
	bin := t.TempDir()
	t.Setenv("PATH", bin)
	writeCollectorCommand(t, bin, "timeout", `
[ "$*" = "3 radeontop -d - -l 1" ] || exit 1
printf '%s\n' "$RADEONTOP_SAMPLE"
`)
	writeCollectorCommand(t, bin, "bash", `
case "$2" in
  *card2/device/hwmon/hwmon\*/temp1_input*) printf '%s\n' "$AMD_TEMP" ;;
  *) exit 1 ;;
esac
`)
	for _, tt := range []struct {
		name, sample, temp string
		used, total        *uint64
		util, temperature  *float64
	}{
		{"measured", "gpu 12.50%, vram 25.00% 2048mb", "42500", new(uint64(2147483648)), new(uint64(8589934592)), new(25.0), new(42.5)},
		{"zero usage", "gpu 12.50%, vram 0.00% 0mb", "0", new(uint64(0)), nil, new(0.0), new(0.0)},
		{"missing", "gpu 12.50%", "", nil, nil, nil, nil},
		{"invalid temperature", "gpu 12.50%", "NaN", nil, nil, nil, nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("RADEONTOP_SAMPLE", tt.sample)
			t.Setenv("AMD_TEMP", tt.temp)
			gpu := (&GPUCollector{}).collectSingleAMDGPU("0000:03:00.0", "Radeon", 2)
			if gpu == nil {
				t.Fatal("valid radeontop sample discarded")
			}
			if gpu.PCIID != "0000:03:00.0" || gpu.Index != 2 || gpu.Vendor != "amd" || gpu.Name != "AMD Radeon" || !gpu.Available || gpu.UtilizationGPU != 12.5 {
				t.Errorf("incorrect GPU identity/utilisation: %+v", gpu)
			}
			assertOptionalMeasurement(t, "used bytes", gpu.MemoryUsed, tt.used)
			assertOptionalMeasurement(t, "total bytes", gpu.MemoryTotal, tt.total)
			assertOptionalMeasurement(t, "memory utilisation", gpu.UtilizationMemory, tt.util)
			assertOptionalMeasurement(t, "temperature", gpu.Temperature, tt.temperature)
			if gpu.PowerDraw != nil {
				t.Error("radeontop invented a power measurement")
			}
		})
	}
}

func TestAMDTemperatureRejectsInvalidMeasurements(t *testing.T) {
	bin := t.TempDir()
	t.Setenv("PATH", bin)
	writeCollectorCommand(t, bin, "bash", `printf '%s\n' "$AMD_TEMP"`)
	for _, value := range []string{"garbled", "NaN", "Inf", "-Inf", "[N/A]"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("AMD_TEMP", value)
			if temp, err := (&GPUCollector{}).getAMDGPUTemp(0); temp != 0 || err == nil || !strings.Contains(err.Error(), "invalid AMD GPU temperature") {
				t.Errorf("temperature = %v, error = %v; want invalid measurement", temp, err)
			}
		})
	}
}

func TestROCmNullableMeasurements(t *testing.T) {
	bin := t.TempDir()
	t.Setenv("PATH", bin)
	writeCollectorCommand(t, bin, "rocm-smi", `
[ "$*" = "--showid --showtemp --showuse --showmeminfo vram --json" ] || exit 1
printf '%s\n' "$ROCM_SAMPLE"
`)
	for _, tt := range []struct {
		name, sample      string
		temperature, util *float64
		used, total       *uint64
	}{
		{"measured", `{"Temperature (Sensor edge) (C)":42.5,"GPU use (%)":30,"VRAM Total Used Memory (B)":2147483648,"VRAM Total Memory (B)":8589934592}`, new(42.5), new(25.0), new(uint64(2147483648)), new(uint64(8589934592))},
		{"measured zero", `{"Temperature (Sensor edge) (C)":0,"GPU use (%)":30,"VRAM Total Used Memory (B)":0,"VRAM Total Memory (B)":100}`, new(0.0), new(0.0), new(uint64(0)), new(uint64(100))},
		{"zero total", `{"GPU use (%)":30,"VRAM Total Used Memory (B)":0,"VRAM Total Memory (B)":0}`, nil, nil, new(uint64(0)), new(uint64(0))},
		{"absent", `{"GPU use (%)":30}`, nil, nil, nil, nil},
		{"unavailable", `{"GPU use (%)":30,"Temperature (Sensor edge) (C)":null,"VRAM Total Used Memory (B)":null,"VRAM Total Memory (B)":null}`, nil, nil, nil, nil},
		{"negative memory", `{"GPU use (%)":30,"VRAM Total Used Memory (B)":-1,"VRAM Total Memory (B)":-1}`, nil, nil, nil, nil},
		{"overflow memory", `{"GPU use (%)":30,"VRAM Total Used Memory (B)":18446744073709551616,"VRAM Total Memory (B)":18446744073709551616}`, nil, nil, nil, nil},
		{"missing used", `{"GPU use (%)":30,"VRAM Total Memory (B)":100}`, nil, nil, nil, new(uint64(100))},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("ROCM_SAMPLE", `{"card0":`+tt.sample+`}`)
			gpus, err := (&GPUCollector{}).collectAMDGPUWithROCm()
			if err != nil || len(gpus) != 1 {
				t.Fatalf("ROCm GPUs = %+v, error = %v; want one GPU", gpus, err)
			}
			gpu := gpus[0]
			if !gpu.Available || gpu.Vendor != "amd" || gpu.Index != 0 || gpu.UtilizationGPU != 30 {
				t.Errorf("incorrect GPU identity/utilisation: %+v", gpu)
			}
			assertOptionalMeasurement(t, "temperature", gpu.Temperature, tt.temperature)
			assertOptionalMeasurement(t, "used bytes", gpu.MemoryUsed, tt.used)
			assertOptionalMeasurement(t, "total bytes", gpu.MemoryTotal, tt.total)
			assertOptionalMeasurement(t, "memory utilisation", gpu.UtilizationMemory, tt.util)
		})
	}
}

func assertOptionalMeasurement[T comparable](t *testing.T, name string, got, want *T) {
	t.Helper()
	if want == nil {
		if got != nil {
			t.Errorf("%s = %v, want unknown", name, *got)
		}
		return
	}
	if got == nil {
		t.Errorf("%s is unknown, want %v", name, *want)
	} else if *got != *want {
		t.Errorf("%s = %v, want %v", name, *got, *want)
	}
}
