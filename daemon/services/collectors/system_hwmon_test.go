package collectors

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/tinkerologie/unraid-management-agent/daemon/domain"
	"github.com/tinkerologie/unraid-management-agent/daemon/dto"
)

// fakeHwmon describes one hwmon entry in a fake sysfs tree.
type fakeHwmon struct {
	hwmon  string            // "hwmon5"
	name   string            // contents of the name attribute
	devRel string            // device directory, relative to <root>/devices
	usbRel string            // USB device directory relative to <root>/devices ("" = none)
	usb    map[string]string // USB device attributes (idVendor, serial, busnum, devpath)
}

// buildFakeSysfs lays out <root>/devices/... with the real devices and
// <root>/class/hwmon/hwmonN -> device symlinks the way the kernel does.
func buildFakeSysfs(t *testing.T, entries []fakeHwmon) string {
	t.Helper()
	root := t.TempDir()

	write := func(path, content string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	for _, e := range entries {
		devDir := filepath.Join(root, "devices", e.devRel)
		hwmonDir := filepath.Join(devDir, "hwmon", e.hwmon)
		write(filepath.Join(hwmonDir, "name"), e.name)
		// hwmonN/device -> ../../ (the parent device), relative like in sysfs.
		if err := os.Symlink(filepath.Join("..", ".."), filepath.Join(hwmonDir, "device")); err != nil {
			t.Fatal(err)
		}
		if e.usbRel != "" {
			for attr, val := range e.usb {
				write(filepath.Join(root, "devices", e.usbRel, attr), val)
			}
		}

		classDir := filepath.Join(root, "class", "hwmon")
		if err := os.MkdirAll(classDir, 0o755); err != nil {
			t.Fatal(err)
		}
		rel, err := filepath.Rel(classDir, hwmonDir)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(rel, filepath.Join(classDir, e.hwmon)); err != nil {
			t.Fatal(err)
		}
	}

	return root
}

func TestHwmonChipDeviceIDs(t *testing.T) {
	const usbPort = "pci0000:00/0000:00:14.0/usb3/3-6/3-6.1"

	tests := []struct {
		name    string
		entries []fakeHwmon
		want    map[string]string
	}{
		{
			name: "HID chip with USB serial uses the serial",
			entries: []fakeHwmon{{
				hwmon:  "hwmon5",
				name:   "octo",
				devRel: usbPort + "/3-6.1:1.0/0003:0C70:F011.0013",
				usbRel: usbPort,
				usb:    map[string]string{"idVendor": "0c70", "serial": "07274-50017", "busnum": "3", "devpath": "6.1"},
			}},
			want: map[string]string{"octo-hid-3-13": "07274-50017"},
		},
		{
			name: "HID id wider than four digits after many HID registrations",
			entries: []fakeHwmon{{
				hwmon:  "hwmon7",
				name:   "octo",
				devRel: usbPort + "/3-6.1:1.0/0003:0C70:F011.10000",
				usbRel: usbPort,
				usb:    map[string]string{"idVendor": "0c70", "serial": "07274-50017", "busnum": "3", "devpath": "6.1"},
			}},
			want: map[string]string{"octo-hid-3-10000": "07274-50017"},
		},
		{
			name: "HID chip without serial uses the USB port path",
			entries: []fakeHwmon{{
				hwmon:  "hwmon6",
				name:   "corsaircpro",
				devRel: usbPort + "/3-6.1:1.0/0003:1B1C:0C10.00A2",
				usbRel: usbPort,
				usb:    map[string]string{"idVendor": "1b1c", "busnum": "3", "devpath": "6.1"},
			}},
			want: map[string]string{"corsaircpro-hid-3-a2": "usb-3-6.1"},
		},
		{
			name: "HID chip with empty serial uses the USB port path",
			entries: []fakeHwmon{{
				hwmon:  "hwmon6",
				name:   "octo",
				devRel: usbPort + "/3-6.1:1.0/0003:0C70:F011.0001",
				usbRel: usbPort,
				usb:    map[string]string{"idVendor": "0c70", "serial": "", "busnum": "3", "devpath": "6.1"},
			}},
			want: map[string]string{"octo-hid-3-1": "usb-3-6.1"},
		},
		{
			name: "non-HID chips get no device ID",
			entries: []fakeHwmon{
				{hwmon: "hwmon0", name: "nct6798", devRel: "platform/nct6775.656"},
				{hwmon: "hwmon1", name: "k10temp", devRel: "pci0000:00/0000:00:18.3"},
				{hwmon: "hwmon2", name: "nvme", devRel: "pci0000:00/0000:01:00.0/nvme/nvme0"},
			},
			want: map[string]string{},
		},
		{
			name: "HID chip without a USB parent gets no device ID",
			entries: []fakeHwmon{{
				hwmon:  "hwmon7",
				name:   "octo",
				devRel: "virtual/0003:0C70:F011.0013",
			}},
			want: map[string]string{},
		},
		{
			name: "two identical HID chips are told apart",
			entries: []fakeHwmon{
				{
					hwmon:  "hwmon5",
					name:   "octo",
					devRel: usbPort + "/3-6.1:1.0/0003:0C70:F011.0013",
					usbRel: usbPort,
					usb:    map[string]string{"idVendor": "0c70", "serial": "07274-50017", "busnum": "3", "devpath": "6.1"},
				},
				{
					hwmon:  "hwmon8",
					name:   "octo",
					devRel: "pci0000:00/0000:00:14.0/usb3/3-7/3-7:1.0/0003:0C70:F011.0014",
					usbRel: "pci0000:00/0000:00:14.0/usb3/3-7",
					usb:    map[string]string{"idVendor": "0c70", "serial": "07274-61234", "busnum": "3", "devpath": "7"},
				},
			},
			want: map[string]string{"octo-hid-3-13": "07274-50017", "octo-hid-3-14": "07274-61234"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := buildFakeSysfs(t, tt.entries)
			got := hwmonChipDeviceIDs(root)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("hwmonChipDeviceIDs() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHwmonChipDeviceIDsMissingSysfs(t *testing.T) {
	got := hwmonChipDeviceIDs(filepath.Join(t.TempDir(), "missing"))
	if len(got) != 0 {
		t.Errorf("expected no device IDs, got %v", got)
	}
}

func TestHwmonFanIdentityWithoutUSBIdentity(t *testing.T) {
	root := buildFakeSysfs(t, []fakeHwmon{
		{hwmon: "hwmon0", name: "octo", devRel: "virtual/0003:0C70:F011.0013"},
		{hwmon: "hwmon1", name: "octo", devRel: "virtual/0003:0C70:F011.0014"},
	})
	first := filepath.Join(root, "class/hwmon/hwmon0")
	second := filepath.Join(root, "class/hwmon/hwmon1")
	physicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	chip, identity := hwmonFanIdentity(first, root)
	if chip != "octo-hid-3-13" || identity != "octo/"+filepath.Join(physicalRoot, "devices/virtual/0003:0C70:F011.0013") {
		t.Fatalf("HID fallback identity = %q/%q; want chip and physical device path", chip, identity)
	}
	writeSensorFixture(t, filepath.Join(first, "fan1_input"), "1200")
	writeSensorFixture(t, filepath.Join(first, "fan1_label"), "First")
	writeSensorFixture(t, filepath.Join(second, "fan1_input"), "1500")
	writeSensorFixture(t, filepath.Join(second, "fan1_label"), "Second")
	c := &SystemCollector{sysfsRoot: root}
	c.parseFanSpeeds("octo-hid-3-13\nfan1:\n  fan1_input: 56784\n", nil)
	fans, err := c.readHwmonFanSpeeds()
	if err != nil || len(fans) != 1 || fans["Second"].RPM != 1500 {
		t.Errorf("fans after HID quarantine = %v, %v; want only second device", fans, err)
	}
}

func TestHwmonFanIdentityForDisappearedDevice(t *testing.T) {
	root := t.TempDir()
	// Dangling class links model devices removed during a sysfs scan.
	for _, name := range []string{"hwmon0", "hwmon1"} {
		dir := filepath.Join(root, name)
		if err := os.Symlink(filepath.Join(root, "removed", name), dir); err != nil {
			t.Fatal(err)
		}
		chip, identity := hwmonFanIdentity(dir, root)
		if chip != "" || identity != "/"+dir {
			t.Errorf("removed device identity = %q/%q; want no chip and original path /%s", chip, identity, dir)
		}
	}
}

func TestParseFanSpeedsChipIdentity(t *testing.T) {
	c := NewSystemCollector(&domain.Context{Hub: domain.NewEventBus(10)})

	const octoA = "octo-hid-3-13\nAdapter: HID adapter\nFan 1:\n  fan1_input: 1200.000\nFan 2:\n  fan2_input: 800.000\n"
	const octoB = "octo-hid-3-14\nAdapter: HID adapter\nFan 1:\n  fan1_input: 950.000\nFan 2:\n  fan2_input: 600.000\n"
	const nct = "nct6798-isa-0290\nAdapter: ISA adapter\nfan1:\n  fan1_input: 700.000\n"

	tests := []struct {
		name      string
		output    string
		deviceIDs map[string]string
		want      []dto.FanInfo
	}{
		{
			name:      "single octo keeps the short name",
			output:    octoA + "\n" + nct,
			deviceIDs: map[string]string{"octo-hid-3-13": "07274-50017"},
			want: []dto.FanInfo{
				{Name: "octo_fan1", RPM: 1200, Source: "octo-hid-3-13", DeviceID: "07274-50017", Label: "Fan 1"},
				{Name: "octo_fan2", RPM: 800, Source: "octo-hid-3-13", DeviceID: "07274-50017", Label: "Fan 2"},
				{Name: "nct6798_fan1", RPM: 700, Source: "nct6798-isa-0290"},
			},
		},
		{
			name:      "two octos are named by device ID",
			output:    octoA + "\n" + octoB,
			deviceIDs: map[string]string{"octo-hid-3-13": "07274-50017", "octo-hid-3-14": "usb-3-7"},
			want: []dto.FanInfo{
				{Name: "octo-07274-50017_fan1", RPM: 1200, Source: "octo-hid-3-13", DeviceID: "07274-50017", Label: "Fan 1"},
				{Name: "octo-07274-50017_fan2", RPM: 800, Source: "octo-hid-3-13", DeviceID: "07274-50017", Label: "Fan 2"},
				{Name: "octo-usb-3-7_fan1", RPM: 950, Source: "octo-hid-3-14", DeviceID: "usb-3-7", Label: "Fan 1"},
				{Name: "octo-usb-3-7_fan2", RPM: 600, Source: "octo-hid-3-14", DeviceID: "usb-3-7", Label: "Fan 2"},
			},
		},
		{
			name:   "two octos without device IDs use the full chip name",
			output: octoA + "\n" + octoB,
			want: []dto.FanInfo{
				{Name: "octo-hid-3-13_fan1", RPM: 1200, Source: "octo-hid-3-13", Label: "Fan 1"},
				{Name: "octo-hid-3-13_fan2", RPM: 800, Source: "octo-hid-3-13", Label: "Fan 2"},
				{Name: "octo-hid-3-14_fan1", RPM: 950, Source: "octo-hid-3-14", Label: "Fan 1"},
				{Name: "octo-hid-3-14_fan2", RPM: 600, Source: "octo-hid-3-14", Label: "Fan 2"},
			},
		},
		{
			name:      "same device ID on two chips falls back to the full chip name",
			output:    octoA + "\n" + octoB,
			deviceIDs: map[string]string{"octo-hid-3-13": "07274-50017", "octo-hid-3-14": "07274-50017"},
			want: []dto.FanInfo{
				{Name: "octo-hid-3-13_fan1", RPM: 1200, Source: "octo-hid-3-13", DeviceID: "07274-50017", Label: "Fan 1"},
				{Name: "octo-hid-3-13_fan2", RPM: 800, Source: "octo-hid-3-13", DeviceID: "07274-50017", Label: "Fan 2"},
				{Name: "octo-hid-3-14_fan1", RPM: 950, Source: "octo-hid-3-14", DeviceID: "07274-50017", Label: "Fan 1"},
				{Name: "octo-hid-3-14_fan2", RPM: 600, Source: "octo-hid-3-14", DeviceID: "07274-50017", Label: "Fan 2"},
			},
		},
		{
			name: "two non-HID chips sharing a model use the full chip name",
			output: "nct6798-isa-0290\nAdapter: ISA adapter\nfan1:\n  fan1_input: 700.000\n\n" +
				"nct6798-isa-0a20\nAdapter: ISA adapter\nfan1:\n  fan1_input: 900.000\n\n" +
				"amdgpu-pci-0300\nAdapter: PCI adapter\nfan1:\n  fan1_input: 0.000\n",
			want: []dto.FanInfo{
				{Name: "nct6798-isa-0290_fan1", RPM: 700, Source: "nct6798-isa-0290"},
				{Name: "nct6798-isa-0a20_fan1", RPM: 900, Source: "nct6798-isa-0a20"},
				{Name: "amdgpu_fan1", RPM: 0, Source: "amdgpu-pci-0300"},
			},
		},
		{
			name:   "chip without fans does not affect naming",
			output: "nct6798-isa-0290\nAdapter: ISA adapter\nfan1:\n  fan1_input: 700.000\n\nnct6798-isa-0a20\nAdapter: ISA adapter\ntemp1:\n  temp1_input: 40.000\n",
			want: []dto.FanInfo{
				{Name: "nct6798_fan1", RPM: 700, Source: "nct6798-isa-0290"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := c.parseFanSpeeds(tt.output, tt.deviceIDs)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseFanSpeeds() =\n  %+v\nwant\n  %+v", got, tt.want)
			}
		})
	}
}

// TestParseFanSpeedsLabels checks that each fan carries its lm-sensors label
// (a sensors.conf override or the driver's fanN_label), taken from the
// feature line that precedes the fanN_input line, and that the labels of
// other features (voltages, temperatures) are not attached to fans.
func TestParseFanSpeedsLabels(t *testing.T) {
	c := NewSystemCollector(&domain.Context{Hub: domain.NewEventBus(10)})
	// Layout of `sensors -u` for an Aquacomputer Octo with sensors.conf labels.
	const output = `octo-hid-3-13
Adapter: HID adapter
Fan 1 voltage:
  in0_input: 12.020
Array Fan1:
  fan1_input: 2889.000
  fan1_min: 0.000
Coolant Flow (rpm):
  fan8_input: 742.000
Coolant Temp:
  temp1_input: 35.830

nct6798-isa-0290
Adapter: ISA adapter
fan1:
  fan1_input: 700.000
in0:
  in0_input: 1.000
  fan2_input: 650.000
`
	got := c.parseFanSpeeds(output, map[string]string{"octo-hid-3-13": "00000-00001"})
	want := []dto.FanInfo{
		{Name: "octo_fan1", RPM: 2889, Source: "octo-hid-3-13", DeviceID: "00000-00001", Label: "Array Fan1"},
		{Name: "octo_fan8", RPM: 742, Source: "octo-hid-3-13", DeviceID: "00000-00001", Label: "Coolant Flow (rpm)"},
		{Name: "nct6798_fan1", RPM: 700, Source: "nct6798-isa-0290"},
		// An input without its own feature line gets no label.
		{Name: "nct6798_fan2", RPM: 650, Source: "nct6798-isa-0290"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseFanSpeeds() =\n  %+v\nwant\n  %+v", got, want)
	}
}

// arcB70PCIPath is the sysfs path of the Intel Arc Pro B70 from issue #347.
const arcB70PCIPath = "pci0000:00/0000:00:06.0/0000:02:00.0/0000:03:01.0/0000:04:00.0"

// installGPUFanLspci puts a fake lspci on PATH that names the GPUs of the fan
// fixtures. lib caches GPU names per PCI ID, so every test uses the same names.
func installGPUFanLspci(t *testing.T) {
	t.Helper()
	bin := t.TempDir()
	t.Setenv("PATH", bin)
	writeCollectorCommand(t, bin, "lspci", `case "$*" in
"-Dmm -s 0000:04:00.0") echo '0000:04:00.0 "VGA compatible controller" "Intel Corporation" "Battlemage G31 [Arc Pro B70]" -p00 "Intel Corporation" "Device 1701"' ;;
"-Dmm -s 0000:03:00.0") echo '0000:03:00.0 "VGA compatible controller" "Advanced Micro Devices, Inc. [AMD/ATI]" "Navi 31 [Radeon RX 7900 XTX]"' ;;
*) exit 1 ;;
esac`)
}

// writeHwmonFiles writes attributes of <root>/class/hwmon/<hwmon> and, when
// class is not empty, the class attribute of its parent device.
func writeHwmonFiles(t *testing.T, root, hwmon, class string, files map[string]string) {
	t.Helper()
	dir := filepath.Join(root, "class", "hwmon", hwmon)
	for name, value := range files {
		writeSensorFixture(t, filepath.Join(dir, name), value+"\n")
	}
	if class != "" {
		writeSensorFixture(t, filepath.Join(dir, "device", "class"), class+"\n")
	}
}

func TestParseFanSpeedsGPUMetadata(t *testing.T) {
	installGPUFanLspci(t)
	const xe = "xe-pci-0400\nAdapter: PCI adapter\nfan1:\n  fan1_input: 811.000\n"
	const nct = "nct6687-isa-0a20\nAdapter: ISA adapter\nCPU Fan:\n  fan1_input: 1005.000\nfan2:\n  fan2_input: 818.000\n"
	const amdgpuPair = "amdgpu-pci-0300\nAdapter: PCI adapter\nfan1:\n  fan1_input: 1500.000\n\n" +
		"amdgpu-pci-0400\nAdapter: PCI adapter\nfan1:\n  fan1_input: 1400.000\n"

	tests := []struct {
		name    string
		entries []fakeHwmon
		classes map[string]string // hwmon -> PCI class
		output  string
		want    []dto.FanInfo
	}{
		{
			name: "GPU chip with one hwmon entry gets metadata and keeps its name",
			entries: []fakeHwmon{
				{hwmon: "hwmon6", name: "xe", devRel: arcB70PCIPath},
				{hwmon: "hwmon7", name: "nct6687", devRel: "platform/nct6687.2592"},
			},
			classes: map[string]string{"hwmon6": "0x030000"},
			output:  xe + "\n" + nct,
			want: []dto.FanInfo{
				{Name: "xe_fan1", RPM: 811, Source: "xe-pci-0400", Role: dto.FanRoleGPU, PCIID: "0000:04:00.0"},
				{Name: "nct6687_fan1", RPM: 1005, Source: "nct6687-isa-0a20", Label: "CPU Fan"},
				{Name: "nct6687_fan2", RPM: 818, Source: "nct6687-isa-0a20"},
			},
		},
		{
			name: "chip name of two hwmon entries gets no metadata",
			entries: []fakeHwmon{
				{hwmon: "hwmon5", name: "xe", devRel: "pci0001:00/0001:04:00.0"},
				{hwmon: "hwmon6", name: "xe", devRel: arcB70PCIPath},
			},
			classes: map[string]string{"hwmon5": "0x030000", "hwmon6": "0x030000"},
			output:  xe,
			want:    []dto.FanInfo{{Name: "xe_fan1", RPM: 811, Source: "xe-pci-0400"}},
		},
		{
			name: "non-display PCI chip gets no metadata",
			entries: []fakeHwmon{
				{hwmon: "hwmon6", name: "xe", devRel: arcB70PCIPath},
			},
			classes: map[string]string{"hwmon6": "0x040300"},
			output:  xe,
			want:    []dto.FanInfo{{Name: "xe_fan1", RPM: 811, Source: "xe-pci-0400"}},
		},
		{
			name: "two GPUs of the same model keep their full chip names",
			entries: []fakeHwmon{
				{hwmon: "hwmon2", name: "amdgpu", devRel: "pci0000:00/0000:00:01.1/0000:03:00.0"},
				{hwmon: "hwmon3", name: "amdgpu", devRel: "pci0000:00/0000:00:01.2/0000:04:00.0"},
			},
			classes: map[string]string{"hwmon2": "0x030000", "hwmon3": "0x030000"},
			output:  amdgpuPair,
			want: []dto.FanInfo{
				{Name: "amdgpu-pci-0300_fan1", RPM: 1500, Source: "amdgpu-pci-0300", Role: dto.FanRoleGPU, PCIID: "0000:03:00.0"},
				{Name: "amdgpu-pci-0400_fan1", RPM: 1400, Source: "amdgpu-pci-0400", Role: dto.FanRoleGPU, PCIID: "0000:04:00.0"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := buildFakeSysfs(t, tt.entries)
			for hwmon, class := range tt.classes {
				writeHwmonFiles(t, root, hwmon, class, nil)
			}
			c := &SystemCollector{sysfsRoot: root}
			if got := c.parseFanSpeeds(tt.output, nil); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseFanSpeeds() =\n  %+v\nwant\n  %+v", got, tt.want)
			}
		})
	}
}

func TestReadHwmonFanSpeedsGPUFans(t *testing.T) {
	installGPUFanLspci(t)
	root := buildFakeSysfs(t, []fakeHwmon{
		{hwmon: "hwmon2", name: "amdgpu", devRel: "pci0000:00/0000:00:01.1/0000:03:00.0"},
		{hwmon: "hwmon6", name: "xe", devRel: arcB70PCIPath},
		{hwmon: "hwmon7", name: "nct6687", devRel: "platform/nct6687.2592"},
	})
	writeHwmonFiles(t, root, "hwmon2", "0x030000", map[string]string{"fan1_input": "1500", "fan2_input": "1400", "fan2_label": "Pump"})
	writeHwmonFiles(t, root, "hwmon6", "0x030000", map[string]string{"fan1_input": "811"})
	writeHwmonFiles(t, root, "hwmon7", "", map[string]string{"fan1_input": "1005", "fan3_input": "818", "fan3_label": "System Fan #1"})

	c := &SystemCollector{sysfsRoot: root}
	got, err := c.readHwmonFanSpeeds()
	if err != nil {
		t.Fatalf("readHwmonFanSpeeds() error = %v", err)
	}
	// The GPU fan1 and the motherboard fan1 no longer share the name "Fan 1".
	want := map[string]dto.FanInfo{
		"AMD Radeon RX 7900 XTX Fan 1": {Name: "AMD Radeon RX 7900 XTX Fan 1", RPM: 1500, Role: dto.FanRoleGPU, PCIID: "0000:03:00.0"},
		"AMD Radeon RX 7900 XTX Pump":  {Name: "AMD Radeon RX 7900 XTX Pump", RPM: 1400, Role: dto.FanRoleGPU, PCIID: "0000:03:00.0"},
		"Intel Arc Pro B70 Fan":        {Name: "Intel Arc Pro B70 Fan", RPM: 811, Role: dto.FanRoleGPU, PCIID: "0000:04:00.0"},
		"Fan 1":                        {Name: "Fan 1", RPM: 1005},
		"System Fan #1":                {Name: "System Fan #1", RPM: 818},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("readHwmonFanSpeeds() =\n  %+v\nwant\n  %+v", got, want)
	}
}
