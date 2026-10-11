package collectors

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// defaultSysfsRoot is the sysfs mount point used when a collector does not override it.
const defaultSysfsRoot = "/sys"

// hidDeviceNameRe matches a HID device directory name, e.g. "0003:0C70:F011.0013"
// (bus type : vendor : product . id, all hexadecimal). The kernel prints the id
// with "%04X", a minimum width: its global counter grows past 0xFFFF on a host
// that has registered many HID devices ("0003:0C70:F011.10000").
var hidDeviceNameRe = regexp.MustCompile(`^([0-9A-Fa-f]{4}):[0-9A-Fa-f]{4}:[0-9A-Fa-f]{4}\.([0-9A-Fa-f]{4,8})$`)

// maxUSBParentDepth bounds the walk from a HID device up to its USB device.
const maxUSBParentDepth = 8

// hwmonChipDeviceIDs maps lm-sensors chip names of USB HID hwmon chips to a
// stable identity of the physical USB device.
//
// lm-sensors names HID chips "<name>-hid-<bus type>-<hid id>" (lib/data.c:
// "%s-hid-%hd-%x"), where the HID id changes every time the USB device
// re-enumerates. The returned ID is the USB device's serial number when it
// has one, otherwise "usb-<busnum>-<devpath>" (its physical port path).
// Chips on other buses are not included: their lm-sensors address is stable.
func hwmonChipDeviceIDs(sysfsRoot string) map[string]string {
	if sysfsRoot == "" {
		sysfsRoot = defaultSysfsRoot
	}
	ids := make(map[string]string)

	hwmonDirs, err := filepath.Glob(filepath.Join(sysfsRoot, "class", "hwmon", "hwmon*"))
	if err != nil {
		return ids
	}

	for _, hwmonDir := range hwmonDirs {
		chip, devDir, ok := hidHwmonChip(hwmonDir)
		if !ok {
			continue
		}
		if id := usbDeviceID(devDir, sysfsRoot); id != "" {
			ids[chip] = id
		}
	}

	return ids
}

// hidHwmonChip returns the lm-sensors chip name and the resolved device
// directory of a hwmon entry whose parent device is a HID device.
func hidHwmonChip(hwmonDir string) (chip, devDir string, ok bool) {
	name := readSysfsValue(filepath.Join(hwmonDir, "name"))
	if name == "" {
		return "", "", false
	}

	devDir, err := filepath.EvalSymlinks(filepath.Join(hwmonDir, "device"))
	if err != nil {
		return "", "", false
	}

	m := hidDeviceNameRe.FindStringSubmatch(filepath.Base(devDir))
	if m == nil {
		return "", "", false
	}

	bus, err := strconv.ParseUint(m[1], 16, 16)
	if err != nil {
		return "", "", false
	}
	id, err := strconv.ParseUint(m[2], 16, 32)
	if err != nil {
		return "", "", false
	}

	return fmt.Sprintf("%s-hid-%d-%x", name, bus, id), devDir, true
}

// usbDeviceID walks up from a HID device directory to the USB device that
// owns it (the first ancestor with an idVendor attribute) and returns its
// serial number, or "usb-<busnum>-<devpath>" when it has none.
func usbDeviceID(devDir, sysfsRoot string) string {
	stop := filepath.Clean(sysfsRoot)
	if resolved, err := filepath.EvalSymlinks(sysfsRoot); err == nil {
		stop = resolved
	}

	dir := filepath.Dir(devDir)
	for range maxUSBParentDepth {
		if dir == stop || dir == filepath.Dir(dir) {
			return ""
		}
		if _, err := os.Stat(filepath.Join(dir, "idVendor")); err == nil {
			if serial := readSysfsValue(filepath.Join(dir, "serial")); serial != "" {
				return serial
			}
			busnum := readSysfsValue(filepath.Join(dir, "busnum"))
			devpath := readSysfsValue(filepath.Join(dir, "devpath"))
			if busnum == "" || devpath == "" {
				return ""
			}
			return "usb-" + busnum + "-" + devpath
		}
		dir = filepath.Dir(dir)
	}

	return ""
}

// readSysfsValue returns the trimmed contents of a sysfs attribute, or "" if it cannot be read.
func readSysfsValue(path string) string {
	// #nosec G304 -- path is built from the sysfs root and hwmon/USB attribute names.
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}
