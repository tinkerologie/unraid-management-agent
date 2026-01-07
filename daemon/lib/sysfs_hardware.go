package lib

import (
	"os"
	"strings"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

// SysfsDMIPath is the path to DMI information in sysfs
const SysfsDMIPath = "/sys/class/dmi/id"

// readSysfsFile reads a file from sysfs and returns its trimmed content
func readSysfsFile(path string) string {
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
