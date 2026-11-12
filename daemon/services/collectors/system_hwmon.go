package collectors

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/tinkerologie/unraid-management-agent/daemon/lib"
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

// hwmonFanIdentities maps lm-sensors chip names (and unique chip models) to
// the same physical identity used by the sysfs fan fallback.
func hwmonFanIdentities(sysfsRoot string) map[string]string {
	if sysfsRoot == "" {
		sysfsRoot = defaultSysfsRoot
	}
	identities := make(map[string]string)
	models := make(map[string]int)
	dirs, _ := filepath.Glob(filepath.Join(sysfsRoot, "class", "hwmon", "hwmon*"))
	for _, dir := range dirs {
		name := readSysfsValue(filepath.Join(dir, "name"))
		chip, identity := hwmonFanIdentity(dir, sysfsRoot)
		if chip != "" {
			identities[chip] = identity
		}
		models[name]++
		if models[name] == 1 {
			identities[name] = identity
		} else {
			delete(identities, name)
		}
	}
	return identities
}

// hwmonChipGPUs maps lm-sensors chip names to the PCI ID of a display-class
// PCI device. A chip name that matches more than one hwmon entry is left out.
func hwmonChipGPUs(sysfsRoot string) map[string]string {
	if sysfsRoot == "" {
		sysfsRoot = defaultSysfsRoot
	}
	gpus := make(map[string]string)
	entries := make(map[string]int)
	dirs, _ := filepath.Glob(filepath.Join(sysfsRoot, "class", "hwmon", "hwmon*"))
	for _, dir := range dirs {
		chip, _ := hwmonFanIdentity(dir, sysfsRoot)
		if chip == "" {
			continue
		}
		entries[chip]++
		if kind, pciID := lib.ResolveHwmonDisplayDevice(dir); kind == lib.HwmonDeviceDisplay {
			gpus[chip] = pciID
		}
	}
	for chip, n := range entries {
		if n > 1 {
			delete(gpus, chip)
		}
	}
	return gpus
}

// hwmonFanIdentity uses a physical device path, or a USB serial/port identity
// for HID chips that can re-enumerate during the quarantine hold.
func hwmonFanIdentity(dir, sysfsRoot string) (chip, identity string) {
	name := readSysfsValue(filepath.Join(dir, "name"))
	if hidChip, dev, ok := hidHwmonChip(dir); ok {
		if id := usbDeviceID(dev, sysfsRoot); id != "" {
			return hidChip, name + "/" + id
		}
		return hidChip, name + "/" + dev
	}
	dev, err := filepath.EvalSymlinks(filepath.Join(dir, "device"))
	if err != nil {
		dev, err = filepath.EvalSymlinks(dir)
		if err != nil {
			dev = dir
		}
	}
	// Native lm-sensors addresses: platform ISA ports, I2C devices and PCI.
	base := filepath.Base(dev)
	if _, address, ok := strings.Cut(base, "."); ok {
		if port, err := strconv.ParseUint(address, 10, 16); err == nil {
			chip = fmt.Sprintf("%s-isa-%04x", name, port)
		}
	}
	if bus, address, ok := strings.Cut(base, "-"); ok {
		busNum, busErr := strconv.ParseUint(bus, 10, 16)
		addr, addrErr := strconv.ParseUint(address, 16, 16)
		if busErr == nil && addrErr == nil {
			chip = fmt.Sprintf("%s-i2c-%d-%x", name, busNum, addr)
		}
	}
	var domain, bus, slot, function uint
	if n, err := fmt.Sscanf(base, "%x:%x:%x.%x", &domain, &bus, &slot, &function); err == nil && n == 4 {
		chip = fmt.Sprintf("%s-pci-%04x", name, bus<<8|slot<<3|function)
	}
	return chip, name + "/" + dev
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
