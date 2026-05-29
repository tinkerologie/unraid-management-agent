package lib

import (
	"os"
	"strings"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

// SysfsDMIPath is the path to DMI information in sysfs.
// It is a var (not const) so tests can override it with a temp directory.
var SysfsDMIPath = "/sys/class/dmi/id"

// readSysfsFile reads a file from sysfs and returns its trimmed content
// This function only reads from /sys paths which are safe system directories
func readSysfsFile(path string) string {
	// #nosec G304 -- path is always built from the fixed /sys system directory.
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// IsSysfsDMIAvailable checks if sysfs DMI info is available
func IsSysfsDMIAvailable() bool {
	_, err := os.Stat(SysfsDMIPath)
	return err == nil
}

// ParseBIOSInfoSysfs reads BIOS information from sysfs instead of dmidecode
// This is faster as it doesn't spawn a process
func ParseBIOSInfoSysfs() (*dto.BIOSInfo, error) {
	bios := &dto.BIOSInfo{
		Vendor:      readSysfsFile(SysfsDMIPath + "/bios_vendor"),
		Version:     readSysfsFile(SysfsDMIPath + "/bios_version"),
		ReleaseDate: readSysfsFile(SysfsDMIPath + "/bios_date"),
	}

	// Read BIOS release if available (format: major.minor)
	if release := readSysfsFile(SysfsDMIPath + "/bios_release"); release != "" {
		bios.Revision = release
	}

	return bios, nil
}

// ParseBaseboardInfoSysfs reads baseboard information from sysfs instead of dmidecode
// This is faster as it doesn't spawn a process
func ParseBaseboardInfoSysfs() (*dto.BaseboardInfo, error) {
	baseboard := &dto.BaseboardInfo{
		Manufacturer: readSysfsFile(SysfsDMIPath + "/board_vendor"),
		ProductName:  readSysfsFile(SysfsDMIPath + "/board_name"),
		Version:      readSysfsFile(SysfsDMIPath + "/board_version"),
		SerialNumber: readSysfsFile(SysfsDMIPath + "/board_serial"),
		AssetTag:     readSysfsFile(SysfsDMIPath + "/board_asset_tag"),
	}

	return baseboard, nil
}

// ParseChassisInfoSysfs reads chassis information from sysfs DMI.
// Returns nil values gracefully when fields are absent.
func ParseChassisInfoSysfs() *dto.ChassisInfo {
	return &dto.ChassisInfo{
		Manufacturer: readSysfsFile(SysfsDMIPath + "/chassis_vendor"),
		// sysfs exposes chassis_type as the raw SMBIOS numeric code; map it to a
		// human-readable label (dmidecode's type 3 already returns the label).
		Type:         chassisTypeName(readSysfsFile(SysfsDMIPath + "/chassis_type")),
		Version:      readSysfsFile(SysfsDMIPath + "/chassis_version"),
		SerialNumber: readSysfsFile(SysfsDMIPath + "/chassis_serial"),
		AssetTag:     readSysfsFile(SysfsDMIPath + "/chassis_asset_tag"),
	}
}

// chassisTypeName maps an SMBIOS System Enclosure type code (DMI type 3) to a
// human-readable label. Non-numeric input (e.g. dmidecode's already-decoded
// label) is returned unchanged.
func chassisTypeName(code string) string {
	names := map[string]string{
		"1": "Other", "2": "Unknown", "3": "Desktop", "4": "Low Profile Desktop",
		"5": "Pizza Box", "6": "Mini Tower", "7": "Tower", "8": "Portable",
		"9": "Laptop", "10": "Notebook", "11": "Hand Held", "12": "Docking Station",
		"13": "All In One", "14": "Sub Notebook", "15": "Space-saving",
		"16": "Lunch Box", "17": "Main Server Chassis", "18": "Expansion Chassis",
		"19": "SubChassis", "20": "Bus Expansion Chassis", "21": "Peripheral Chassis",
		"22": "RAID Chassis", "23": "Rack Mount Chassis", "24": "Sealed-case PC",
		"25": "Multi-system Chassis", "26": "Compact PCI", "27": "Advanced TCA",
		"28": "Blade", "29": "Blade Enclosure", "30": "Tablet", "31": "Convertible",
		"32": "Detachable", "33": "IoT Gateway", "34": "Embedded PC",
		"35": "Mini PC", "36": "Stick PC",
	}
	if name, ok := names[strings.TrimSpace(code)]; ok {
		return name
	}
	return code
}

// ParseSystemInfoSysfs reads system information from sysfs
func ParseSystemInfoSysfs() map[string]string {
	return map[string]string{
		"product_name":   readSysfsFile(SysfsDMIPath + "/product_name"),
		"product_family": readSysfsFile(SysfsDMIPath + "/product_family"),
		"product_serial": readSysfsFile(SysfsDMIPath + "/product_serial"),
		"product_uuid":   readSysfsFile(SysfsDMIPath + "/product_uuid"),
		"product_sku":    readSysfsFile(SysfsDMIPath + "/product_sku"),
		"sys_vendor":     readSysfsFile(SysfsDMIPath + "/sys_vendor"),
		"chassis_vendor": readSysfsFile(SysfsDMIPath + "/chassis_vendor"),
		"chassis_type":   readSysfsFile(SysfsDMIPath + "/chassis_type"),
	}
}
