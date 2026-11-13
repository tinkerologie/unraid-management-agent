package lib

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// gpuLspciTimeout bounds one `lspci -Dmm -s <PCI>` model lookup.
const gpuLspciTimeout = 5 * time.Second

var (
	// pciAddressRe matches a PCI function address (domain:bus:device.function).
	pciAddressRe = regexp.MustCompile(`^[0-9a-f]{4}:[0-9a-f]{2}:[0-9a-f]{2}\.[0-7]$`)
	// lspciQuotedFieldRe extracts the quoted fields of an `lspci -Dmm` record.
	lspciQuotedFieldRe = regexp.MustCompile(`"([^"]*)"`)
)

// LspciRecord holds the fields of one `lspci -Dmm` line that GPU discovery uses.
type LspciRecord struct {
	PCIID  string
	Class  string
	Vendor string
	Device string
}

// ParseLspciRecord splits an `lspci -Dmm` line into its unquoted PCI address
// and its first three quoted fields (class, vendor, device). Unquoted tokens
// such as -rXX and -pXX are ignored. It rejects records with an invalid
// address or fewer than three quoted fields.
func ParseLspciRecord(line string) (LspciRecord, bool) {
	firstQuote := strings.Index(line, "\"")
	if firstQuote <= 0 {
		return LspciRecord{}, false
	}
	pciID := strings.TrimSpace(line[:firstQuote])
	if !pciAddressRe.MatchString(pciID) {
		return LspciRecord{}, false
	}
	matches := lspciQuotedFieldRe.FindAllStringSubmatch(line[firstQuote:], -1)
	if len(matches) < 3 {
		return LspciRecord{}, false
	}
	return LspciRecord{PCIID: pciID, Class: matches[0][1], Vendor: matches[1][1], Device: matches[2][1]}, true
}

// LspciModelName returns the bracketed marketing name of an lspci device field,
// or the whole field when it has no brackets. A malformed bracket gives "".
func LspciModelName(device string) string {
	if !strings.Contains(device, "[") {
		return device
	}
	start := strings.Index(device, "[")
	end := strings.Index(device, "]")
	if start != -1 && end != -1 && end > start {
		return strings.TrimSpace(device[start+1 : end])
	}
	return ""
}

// HwmonDeviceKind classifies the device that a hwmon entry belongs to.
type HwmonDeviceKind int

const (
	// HwmonDeviceUnknown means the device link is missing or broken, or the
	// PCI class cannot be read.
	HwmonDeviceUnknown HwmonDeviceKind = iota
	// HwmonDeviceNonDisplay means the device is not a display-class PCI function.
	HwmonDeviceNonDisplay
	// HwmonDeviceDisplay means the device is a PCI function with base class 0x03.
	HwmonDeviceDisplay
)

// ResolveHwmonDisplayDevice resolves the `device` link of a hwmon directory.
// It returns HwmonDeviceDisplay and the full PCI ID only when the link
// resolves to a PCI function whose `class` attribute has base class 0x03.
// PCI ancestors are not examined.
func ResolveHwmonDisplayDevice(hwmonDir string) (HwmonDeviceKind, string) {
	devDir, err := filepath.EvalSymlinks(filepath.Join(hwmonDir, "device"))
	if err != nil {
		return HwmonDeviceUnknown, ""
	}
	pciID := filepath.Base(devDir)
	if !pciAddressRe.MatchString(pciID) {
		return HwmonDeviceNonDisplay, ""
	}
	class := ReadSysfsString(filepath.Join(devDir, "class"))
	value, err := strconv.ParseUint(strings.TrimPrefix(class, "0x"), 16, 32)
	if err != nil {
		return HwmonDeviceUnknown, ""
	}
	if value>>16 != 0x03 {
		return HwmonDeviceNonDisplay, ""
	}
	return HwmonDeviceDisplay, pciID
}

var (
	// gpuLspci returns `lspci -Dmm -s <PCI>` output; replaceable for tests.
	gpuLspci = func(pciID string) (string, error) {
		return ExecCommandOutputWithTimeout(gpuLspciTimeout, "lspci", "-Dmm", "-s", pciID)
	}
	// gpuNamesMu guards gpuNames.
	gpuNamesMu sync.Mutex
	// gpuNames caches GPU names per full PCI ID, including the fallback
	// (lspci names do not change at runtime).
	gpuNames = map[string]string{}
)

// GPUName returns the name of the GPU at a full PCI ID, built like
// GPUMetrics.Name in collectors/gpu.go: "Intel <model>" or "AMD <model>"
// for those vendors, otherwise the model. It returns "GPU <pci_id>" when
// lspci fails or gives no model. Results are cached per PCI ID.
func GPUName(pciID string) string {
	gpuNamesMu.Lock()
	name, ok := gpuNames[pciID]
	gpuNamesMu.Unlock()
	if ok {
		return name
	}

	name = "GPU " + pciID
	if out, err := gpuLspci(pciID); err == nil {
		line, _, _ := strings.Cut(strings.TrimSpace(out), "\n")
		if record, ok := ParseLspciRecord(line); ok && record.PCIID == pciID {
			if model := LspciModelName(record.Device); model != "" {
				name = gpuDisplayName(record.Vendor, model)
			}
		}
	}

	gpuNamesMu.Lock()
	gpuNames[pciID] = name
	gpuNamesMu.Unlock()
	return name
}

// gpuDisplayName mirrors the vendor prefixes of collectors/gpu.go: Intel
// discovery accepts the vendor field "Intel Corporation" and AMD radeontop
// discovery accepts vendor fields that contain AMD, Advanced Micro Devices
// or ATI.
func gpuDisplayName(vendor, model string) string {
	switch {
	case vendor == "Intel Corporation":
		return "Intel " + model
	case strings.Contains(vendor, "AMD"), strings.Contains(vendor, "Advanced Micro Devices"),
		strings.Contains(vendor, "ATI"):
		return "AMD " + model
	}
	return model
}
