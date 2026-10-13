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
