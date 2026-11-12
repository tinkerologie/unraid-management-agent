package lib

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// arcB70PCIPath is the sysfs path of the Intel Arc Pro B70 from issue #347.
const arcB70PCIPath = "pci0000:00/0000:00:06.0/0000:02:00.0/0000:03:01.0/0000:04:00.0"

// fakeHwmonDevice describes one hwmon entry in a fake sysfs tree.
type fakeHwmonDevice struct {
	hwmon  string            // "hwmon6"
	devRel string            // parent device directory, relative to <root>/devices
	class  string            // the parent device's class attribute ("" = none)
	link   string            // device link target ("" = "../..", "-" = no link)
	files  map[string]string // hwmon attributes, e.g. fan1_input
}

// buildFakeHwmonSysfs lays out <root>/devices/<devRel>/hwmon/hwmonN with a
// relative device link and a relative <root>/class/hwmon/hwmonN link, the way
// the kernel does. It returns <root>/class/hwmon.
func buildFakeHwmonSysfs(t *testing.T, entries []fakeHwmonDevice) string {
	t.Helper()
	root := t.TempDir()
	classDir := filepath.Join(root, "class", "hwmon")
	write := func(path, content string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	symlink := func(target, path string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, path); err != nil {
			t.Fatal(err)
		}
	}

	for _, e := range entries {
		devDir := filepath.Join(root, "devices", e.devRel)
		hwmonDir := filepath.Join(devDir, "hwmon", e.hwmon)
		if err := os.MkdirAll(hwmonDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if e.class != "" {
			write(filepath.Join(devDir, "class"), e.class)
		}
		for name, value := range e.files {
			write(filepath.Join(hwmonDir, name), value)
		}
		switch e.link {
		case "":
			symlink(filepath.Join("..", ".."), filepath.Join(hwmonDir, "device"))
		case "-":
		default:
			symlink(e.link, filepath.Join(hwmonDir, "device"))
		}
		rel, err := filepath.Rel(classDir, hwmonDir)
		if err != nil {
			t.Fatal(err)
		}
		symlink(rel, filepath.Join(classDir, e.hwmon))
	}
	return classDir
}

// stubGPULspci replaces the lspci model lookup and empties the name cache for one test.
func stubGPULspci(t *testing.T, lspci func(pciID string) (string, error)) {
	t.Helper()
	oldLspci, oldNames := gpuLspci, gpuNames
	gpuLspci, gpuNames = lspci, map[string]string{}
	t.Cleanup(func() { gpuLspci, gpuNames = oldLspci, oldNames })
}

func TestResolveHwmonDisplayDevice(t *testing.T) {
	tests := []struct {
		name string
		// displayAncestor gives the PCI ancestor 0000:04:00.0 a display class.
		displayAncestor bool
		entry           fakeHwmonDevice
		wantKind        HwmonDeviceKind
		wantPCIID       string
	}{
		{name: "VGA class 0x030000", entry: fakeHwmonDevice{devRel: arcB70PCIPath, class: "0x030000"}, wantKind: HwmonDeviceDisplay, wantPCIID: "0000:04:00.0"},
		{name: "display class 0x038000", entry: fakeHwmonDevice{devRel: "pci0000:00/0000:00:01.0/0000:01:00.0", class: "0x038000"}, wantKind: HwmonDeviceDisplay, wantPCIID: "0000:01:00.0"},
		{name: "3D controller class 0x030200", entry: fakeHwmonDevice{devRel: "pci0000:00/0000:05:00.0", class: "0x030200"}, wantKind: HwmonDeviceDisplay, wantPCIID: "0000:05:00.0"},
		{name: "non-display NVMe class", entry: fakeHwmonDevice{devRel: "pci0000:00/0000:00:01.0/0000:01:00.0", class: "0x010802"}, wantKind: HwmonDeviceNonDisplay},
		{name: "ISA platform device", entry: fakeHwmonDevice{devRel: "platform/nct6687.2592"}, wantKind: HwmonDeviceNonDisplay},
		{name: "USB device below a display-class ancestor", displayAncestor: true, entry: fakeHwmonDevice{devRel: "pci0000:00/0000:04:00.0/usb3/3-6"}, wantKind: HwmonDeviceNonDisplay},
		{name: "short PCI address", entry: fakeHwmonDevice{devRel: "pci0000:00/04:00.0", class: "0x030000"}, wantKind: HwmonDeviceNonDisplay},
		{name: "missing device link", entry: fakeHwmonDevice{devRel: "virtual/thermal/thermal_zone0", link: "-"}, wantKind: HwmonDeviceUnknown},
		{name: "broken device link", entry: fakeHwmonDevice{devRel: "pci0000:00/0000:04:00.0", class: "0x030000", link: filepath.Join("..", "..", "missing")}, wantKind: HwmonDeviceUnknown},
		{name: "missing class attribute", entry: fakeHwmonDevice{devRel: "pci0000:00/0000:04:00.0"}, wantKind: HwmonDeviceUnknown},
		{name: "malformed class attribute", entry: fakeHwmonDevice{devRel: "pci0000:00/0000:04:00.0", class: "display"}, wantKind: HwmonDeviceUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.entry.hwmon = "hwmon6"
			classDir := buildFakeHwmonSysfs(t, []fakeHwmonDevice{tt.entry})
			if tt.displayAncestor {
				ancestor := filepath.Join(filepath.Dir(filepath.Dir(classDir)), "devices", "pci0000:00", "0000:04:00.0", "class")
				if err := os.WriteFile(ancestor, []byte("0x030000\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			kind, pciID := ResolveHwmonDisplayDevice(filepath.Join(classDir, "hwmon6"))
			if kind != tt.wantKind || pciID != tt.wantPCIID {
				t.Errorf("ResolveHwmonDisplayDevice() = %v, %q; want %v, %q", kind, pciID, tt.wantKind, tt.wantPCIID)
			}
		})
	}
}

func TestResolveHwmonDisplayDeviceSameBDFInTwoDomains(t *testing.T) {
	classDir := buildFakeHwmonSysfs(t, []fakeHwmonDevice{
		{hwmon: "hwmon1", devRel: "pci0000:00/0000:04:00.0", class: "0x030000"},
		{hwmon: "hwmon2", devRel: "pci0001:00/0001:04:00.0", class: "0x030000"},
	})
	for hwmon, want := range map[string]string{"hwmon1": "0000:04:00.0", "hwmon2": "0001:04:00.0"} {
		if kind, pciID := ResolveHwmonDisplayDevice(filepath.Join(classDir, hwmon)); kind != HwmonDeviceDisplay || pciID != want {
			t.Errorf("%s: ResolveHwmonDisplayDevice() = %v, %q; want display %q", hwmon, kind, pciID, want)
		}
	}
}

func TestGPUName(t *testing.T) {
	tests := []struct {
		name  string
		pciID string
		out   string
		err   error
		want  string
	}{
		{"Intel", "0000:04:00.0", `0000:04:00.0 "VGA compatible controller" "Intel Corporation" "Battlemage G31 [Arc Pro B70]" -p00 "Intel Corporation" "Device 1701"` + "\n", nil, "Intel Arc Pro B70"},
		{"AMD", "0000:03:00.0", `0000:03:00.0 "VGA compatible controller" "Advanced Micro Devices, Inc. [AMD/ATI]" "Navi 31 [Radeon RX 7900 XTX]" -rc8 "Sapphire Technology Limited" "Device 471e"`, nil, "AMD Radeon RX 7900 XTX"},
		{"ATI", "0000:06:00.0", `0000:06:00.0 "Display controller" "ATI Technologies Inc" "Device 1636"`, nil, "AMD Device 1636"},
		{"other vendor", "0000:01:00.0", `0000:01:00.0 "VGA compatible controller" "NVIDIA Corporation" "AD104 [GeForce RTX 4070]"`, nil, "GeForce RTX 4070"},
		{"lspci failure", "0000:07:00.0", "", errors.New("exit status 1"), "GPU 0000:07:00.0"},
		{"empty output", "0000:08:00.0", "", nil, "GPU 0000:08:00.0"},
		{"record for another device", "0000:09:00.0", `0001:09:00.0 "VGA compatible controller" "Intel Corporation" "Arc"`, nil, "GPU 0000:09:00.0"},
		{"malformed bracket", "0000:0a:00.0", `0000:0a:00.0 "VGA compatible controller" "Intel Corporation" "Broken [bracket"`, nil, "GPU 0000:0a:00.0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			stubGPULspci(t, func(pciID string) (string, error) {
				calls++
				if pciID != tt.pciID {
					t.Errorf("lspci -s %q, want %q", pciID, tt.pciID)
				}
				return tt.out, tt.err
			})
			for range 2 {
				if got := GPUName(tt.pciID); got != tt.want {
					t.Errorf("GPUName(%q) = %q, want %q", tt.pciID, got, tt.want)
				}
			}
			if calls != 1 {
				t.Errorf("lspci ran %d times, want 1 (cached, including the fallback)", calls)
			}
		})
	}
}

func TestGPUNameRunsLspci(t *testing.T) {
	bin := t.TempDir()
	t.Setenv("PATH", bin)
	script := "#!/bin/sh\n[ \"$*\" = \"-Dmm -s 0000:04:00.0\" ] || exit 1\n" +
		`echo '0000:04:00.0 "VGA compatible controller" "Intel Corporation" "Battlemage G31 [Arc Pro B70]"'` + "\n"
	// #nosec G306 -- Owner-only execution is required for the temporary command fixture.
	if err := os.WriteFile(filepath.Join(bin, "lspci"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	oldNames := gpuNames
	gpuNames = map[string]string{}
	t.Cleanup(func() { gpuNames = oldNames })

	if got := GPUName("0000:04:00.0"); got != "Intel Arc Pro B70" {
		t.Errorf("GPUName() = %q, want %q", got, "Intel Arc Pro B70")
	}
}

func TestParseLspciRecord(t *testing.T) {
	tests := []struct {
		name      string
		line      string
		wantOK    bool
		wantClass string
	}{
		{"VGA compatible controller", `0000:00:02.0 "VGA compatible controller" "Intel Corporation" "Arrow Lake-S [Intel Graphics]" -r06 "Micro-Star International Co., Ltd. [MSI]" "Device 7e42"`, true, "VGA compatible controller"},
		{"3D controller", `0000:05:00.0 "3D controller" "Intel Corporation" "Device e20b"`, true, "3D controller"},
		{"Display controller", `0000:04:00.0 "Display controller" "Intel Corporation" "Battlemage G31 [Arc Pro B70]" -p00 "Intel Corporation" "Device 1234"`, true, "Display controller"},
		{"numeric class", `0000:00:04.0 "Class 0380" "Intel Corporation" "Device 0002"`, true, "Class 0380"},
		{"audio device with Display in device name", `0000:03:00.1 "Audio device" "Advanced Micro Devices, Inc. [AMD/ATI]" "Navi 31 HDMI/DP Audio [Display Audio]"`, true, "Audio device"},
		{"VGA only in subsystem fields", `0000:00:1f.4 "SMBus" "Intel Corporation" "Device 7f23" -r10 "VGA Corp" "VGA compatible controller"`, true, "SMBus"},
		{"empty line", "", false, ""},
		{"fewer than three quoted fields", `0000:00:02.0 "VGA compatible controller" "Intel Corporation"`, false, ""},
		{"unterminated quote", `0000:00:02.0 "VGA compatible controller" "Intel Corporation" "Arc`, false, ""},
		{"quoted address", `"0000:00:02.0" "VGA compatible controller" "Intel Corporation" "Arc"`, false, ""},
		{"address without domain", `00:02.0 "VGA compatible controller" "Intel Corporation" "Arc"`, false, ""},
		{"path traversal address", `../../etc/passwd "VGA compatible controller" "Intel Corporation" "Arc"`, false, ""},
		{"command injection address", `0000:00:02.0;rm -rf / "VGA compatible controller" "Intel Corporation" "Arc"`, false, ""},
		{"oversized class field", `0000:00:02.0 "` + strings.Repeat("VGA ", 1<<18) + `" "Intel Corporation" "Arc"`, true, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			record, ok := ParseLspciRecord(tt.line)
			if ok != tt.wantOK {
				t.Fatalf("ParseLspciRecord ok = %v, want %v (record %+v)", ok, tt.wantOK, record)
			}
			if !ok {
				if record != (LspciRecord{}) {
					t.Errorf("rejected record returned fields: %+v", record)
				}
				return
			}
			if tt.wantClass != "" && record.Class != tt.wantClass {
				t.Errorf("class = %q, want %q", record.Class, tt.wantClass)
			}
		})
	}

	const nonVGARecord = `0000:80:14.5 "Non-VGA unclassified device" "Intel Corporation" "Device 7f2f" -r10 "Micro-Star International Co., Ltd. [MSI]" "Device 7e42"`
	record, ok := ParseLspciRecord(nonVGARecord)
	want := LspciRecord{PCIID: "0000:80:14.5", Class: "Non-VGA unclassified device", Vendor: "Intel Corporation", Device: "Device 7f2f"}
	if !ok || record != want {
		t.Errorf("ParseLspciRecord(issue 346 record) = %+v, %v; want %+v", record, ok, want)
	}
}

func TestLspciModelName(t *testing.T) {
	for _, tt := range []struct{ device, want string }{
		{"Battlemage G31 [Arc Pro B70]", "Arc Pro B70"},
		{"Navi 31 [ Radeon RX 7900 XTX ]", "Radeon RX 7900 XTX"},
		{"Device 7f2f", "Device 7f2f"},
		{"Broken [bracket", ""},
		{"Reversed ]bracket[", ""},
	} {
		if got := LspciModelName(tt.device); got != tt.want {
			t.Errorf("LspciModelName(%q) = %q, want %q", tt.device, got, tt.want)
		}
	}
}
