package collectors

import (
	"strings"
	"testing"

	"github.com/tinkerologie/unraid-management-agent/daemon/dto"
)

// nonVGARecord is the chipset function from issue #346 that was reported as a GPU.
const nonVGARecord = `0000:80:14.5 "Non-VGA unclassified device" "Intel Corporation" "Device 7f2f" -r10 "Micro-Star International Co., Ltd. [MSI]" "Device 7e42"`

func TestParseLspciRecordAndDisplayClass(t *testing.T) {
	tests := []struct {
		name      string
		line      string
		wantOK    bool
		wantGPU   bool
		wantClass string
	}{
		{"VGA compatible controller", `0000:00:02.0 "VGA compatible controller" "Intel Corporation" "Arrow Lake-S [Intel Graphics]" -r06 "Micro-Star International Co., Ltd. [MSI]" "Device 7e42"`, true, true, "VGA compatible controller"},
		{"3D controller", `0000:05:00.0 "3D controller" "Intel Corporation" "Device e20b"`, true, true, "3D controller"},
		{"Display controller", `0000:04:00.0 "Display controller" "Intel Corporation" "Battlemage G31 [Arc Pro B70]" -p00 "Intel Corporation" "Device 1234"`, true, true, "Display controller"},
		{"XGA compatible controller", `0000:00:03.0 "XGA compatible controller" "Intel Corporation" "Device 0001"`, true, true, "XGA compatible controller"},
		{"numeric display class fallback", `0000:00:04.0 "Class 0380" "Intel Corporation" "Device 0002"`, true, true, "Class 0380"},
		{"non-VGA unclassified device from issue 346", nonVGARecord, true, false, "Non-VGA unclassified device"},
		{"audio device with Display in device name", `0000:03:00.1 "Audio device" "Advanced Micro Devices, Inc. [AMD/ATI]" "Navi 31 HDMI/DP Audio [Display Audio]"`, true, false, "Audio device"},
		{"numeric non-display class", `0000:00:1f.3 "Class 0480" "Intel Corporation" "Device 7f50"`, true, false, "Class 0480"},
		{"VGA only in subsystem fields", `0000:00:1f.4 "SMBus" "Intel Corporation" "Device 7f23" -r10 "VGA Corp" "VGA compatible controller"`, true, false, "SMBus"},
		{"empty line", "", false, false, ""},
		{"fewer than three quoted fields", `0000:00:02.0 "VGA compatible controller" "Intel Corporation"`, false, false, ""},
		{"unterminated quote", `0000:00:02.0 "VGA compatible controller" "Intel Corporation" "Arc`, false, false, ""},
		{"quoted address", `"0000:00:02.0" "VGA compatible controller" "Intel Corporation" "Arc"`, false, false, ""},
		{"address without domain", `00:02.0 "VGA compatible controller" "Intel Corporation" "Arc"`, false, false, ""},
		{"path traversal address", `../../etc/passwd "VGA compatible controller" "Intel Corporation" "Arc"`, false, false, ""},
		{"command injection address", `0000:00:02.0;rm -rf / "VGA compatible controller" "Intel Corporation" "Arc"`, false, false, ""},
		{"oversized class field", `0000:00:02.0 "` + strings.Repeat("VGA ", 1<<18) + `" "Intel Corporation" "Arc"`, true, false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			record, ok := parseLspciRecord(tt.line)
			if ok != tt.wantOK {
				t.Fatalf("parseLspciRecord ok = %v, want %v (record %+v)", ok, tt.wantOK, record)
			}
			if !ok {
				if record != (lspciRecord{}) {
					t.Errorf("rejected record returned fields: %+v", record)
				}
				return
			}
			if tt.wantClass != "" && record.Class != tt.wantClass {
				t.Errorf("class = %q, want %q", record.Class, tt.wantClass)
			}
			if got := isDisplayClass(record.Class); got != tt.wantGPU {
				t.Errorf("isDisplayClass(%.40q) = %v, want %v", record.Class, got, tt.wantGPU)
			}
		})
	}

	record, ok := parseLspciRecord(nonVGARecord)
	want := lspciRecord{PCIID: "0000:80:14.5", Class: "Non-VGA unclassified device", Vendor: "Intel Corporation", Device: "Device 7f2f"}
	if !ok || record != want {
		t.Errorf("parseLspciRecord(issue 346 record) = %+v, %v; want %+v", record, ok, want)
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
		if got := lspciModelName(tt.device); got != tt.want {
			t.Errorf("lspciModelName(%q) = %q, want %q", tt.device, got, tt.want)
		}
	}
}

// mixedLspciOutput contains Intel and AMD display devices plus records that
// older substring matching reported as GPUs.
var mixedLspciOutput = strings.Join([]string{
	`0000:00:02.0 "VGA compatible controller" "Intel Corporation" "Arrow Lake-S [Intel Graphics]" -r06 "Micro-Star International Co., Ltd. [MSI]" "Device 7e42"`,
	`0000:01:00.0 "VGA compatible controller" "NVIDIA Corporation" "AD104 [GeForce RTX 4070]" -ra1 "Intel Corporation" "Device 1234"`,
	`0000:03:00.0 "VGA compatible controller" "Advanced Micro Devices, Inc. [AMD/ATI]" "Navi 31 [Radeon RX 7900 XTX]" -rc8 "Sapphire Technology Limited" "Device 471e"`,
	`0000:03:00.1 "Audio device" "Advanced Micro Devices, Inc. [AMD/ATI]" "Navi 31 HDMI/DP Audio [Display Audio]"`,
	`0000:04:00.0 "Display controller" "Intel Corporation" "Battlemage G31 [Arc Pro B70]" -p00 "Intel Corporation" "Device 1234"`,
	`0000:05:00.0 "3D controller" "Intel Corporation" "Device e20b"`,
	`0000:06:00.0 "Display controller" "ATI Technologies Inc" "Device 1636"`,
	nonVGARecord,
	"",
}, "\n")

func TestCollectIntelGPUDiscovery(t *testing.T) {
	const intelSample = `[{"engines":{"Render/3D/0":{"busy":10}},"power":{"GPU":1}}]`
	tests := []struct {
		name      string
		lspci     string
		noGPUTop  bool
		wantErr   string
		wantIDs   []string
		wantNames []string
	}{
		{
			name:      "display class and vendor field only",
			lspci:     `printf '%s\n' "$LSPCI_OUTPUT"`,
			wantIDs:   []string{"0000:00:02.0", "0000:04:00.0", "0000:05:00.0"},
			wantNames: []string{"Intel Intel Graphics", "Intel Arc Pro B70", "Intel Device e20b"},
		},
		{name: "lspci failure", lspci: "exit 1", wantErr: "lspci query failed"},
		{name: "no display devices", lspci: `printf '%s\n' '` + nonVGARecord + `'`, wantErr: "no Intel GPU found"},
		{name: "missing intel_gpu_top", lspci: `printf '%s\n' "$LSPCI_OUTPUT"`, noGPUTop: true, wantErr: "intel_gpu_top not found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bin := t.TempDir()
			t.Setenv("PATH", bin)
			t.Setenv("LSPCI_OUTPUT", mixedLspciOutput)
			writeCollectorCommand(t, bin, "lspci", `[ "$*" = "-Dmm" ] || exit 1
`+tt.lspci)
			if !tt.noGPUTop {
				writeCollectorCommand(t, bin, "intel_gpu_top", "exit 1")
			}
			writeCollectorCommand(t, bin, "timeout", `[ "$*" = "2 intel_gpu_top -J -s 500 -n 1" ] || exit 1
printf '%s\n' '`+intelSample+`'`)

			gpus, err := (&GPUCollector{sysfsRoot: t.TempDir()}).collectIntelGPU()
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("collectIntelGPU() error = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("collectIntelGPU() error = %v", err)
			}
			assertDiscoveredGPUs(t, gpus, "intel", tt.wantIDs, tt.wantNames)
		})
	}
}

func TestCollectAMDGPUWithRadeontopDiscovery(t *testing.T) {
	tests := []struct {
		name      string
		lspci     string
		wantErr   string
		wantIDs   []string
		wantNames []string
	}{
		{
			name:      "display class and vendor field only",
			lspci:     `printf '%s\n' "$LSPCI_OUTPUT"`,
			wantIDs:   []string{"0000:03:00.0", "0000:06:00.0"},
			wantNames: []string{"AMD Radeon RX 7900 XTX", "AMD Device 1636"},
		},
		{name: "lspci failure", lspci: "exit 1", wantErr: "lspci query failed"},
		{name: "no display devices", lspci: `printf '%s\n' '` + nonVGARecord + `'`, wantErr: "no AMD GPU found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bin := t.TempDir()
			t.Setenv("PATH", bin)
			t.Setenv("LSPCI_OUTPUT", mixedLspciOutput)
			writeCollectorCommand(t, bin, "lspci", `[ "$*" = "-Dmm" ] || exit 1
`+tt.lspci)
			writeCollectorCommand(t, bin, "timeout", `[ "$*" = "3 radeontop -d - -l 1" ] || exit 1
printf '%s\n' 'gpu 12.50%'`)
			writeCollectorCommand(t, bin, "bash", "exit 1")

			gpus, err := (&GPUCollector{}).collectAMDGPUWithRadeontop()
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("collectAMDGPUWithRadeontop() error = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("collectAMDGPUWithRadeontop() error = %v", err)
			}
			assertDiscoveredGPUs(t, gpus, "amd", tt.wantIDs, tt.wantNames)
		})
	}
}

func assertDiscoveredGPUs(t *testing.T, gpus []*dto.GPUMetrics, vendor string, wantIDs, wantNames []string) {
	t.Helper()
	if len(gpus) != len(wantIDs) {
		ids := make([]string, 0, len(gpus))
		for _, gpu := range gpus {
			ids = append(ids, gpu.PCIID)
		}
		t.Fatalf("discovered %d GPUs %v, want %v", len(gpus), ids, wantIDs)
	}
	for i, gpu := range gpus {
		if gpu.PCIID != wantIDs[i] || gpu.Name != wantNames[i] || gpu.Vendor != vendor {
			t.Errorf("GPU %d = {PCIID:%q Name:%q Vendor:%q}, want {%q %q %q}", i, gpu.PCIID, gpu.Name, gpu.Vendor, wantIDs[i], wantNames[i], vendor)
		}
	}
}
