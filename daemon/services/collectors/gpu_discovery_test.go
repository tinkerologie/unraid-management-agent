package collectors

import (
	"strings"
	"testing"

	"github.com/tinkerologie/unraid-management-agent/daemon/dto"
)

// nonVGARecord is the chipset function from issue #346 that was reported as a GPU.
const nonVGARecord = `0000:80:14.5 "Non-VGA unclassified device" "Intel Corporation" "Device 7f2f" -r10 "Micro-Star International Co., Ltd. [MSI]" "Device 7e42"`

func TestIsDisplayClass(t *testing.T) {
	tests := []struct {
		class string
		want  bool
	}{
		{"VGA compatible controller", true},
		{"3D controller", true},
		{"Display controller", true},
		{"XGA compatible controller", true},
		{"Class 0380", true},
		{"Non-VGA unclassified device", false},
		{"Audio device", false},
		{"Class 0480", false},
		{"SMBus", false},
		{"", false},
		{strings.Repeat("VGA ", 1<<18), false},
	}
	for _, tt := range tests {
		if got := isDisplayClass(tt.class); got != tt.want {
			t.Errorf("isDisplayClass(%.40q) = %v, want %v", tt.class, got, tt.want)
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
