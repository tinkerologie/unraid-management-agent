package lib

import (
	"bufio"
	"os"
	"strings"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

// sysClassTPM is the sysfs TPM class directory. var (not const) so tests can override.
var sysClassTPM = "/sys/class/tpm"

// sysBlockPath is the sysfs block device directory. var so tests can override.
var sysBlockPath = "/sys/block"

// procMountsPath is the kernel mounts file. var so tests can override.
var procMountsPath = "/proc/mounts"

// ReadTPMInfo detects a Trusted Platform Module via sysfs.
// Returns Present=false (and no error) when no TPM device exists.
func ReadTPMInfo() *dto.TPMInfo {
	info := &dto.TPMInfo{}

	entries, err := os.ReadDir(sysClassTPM)
	if err != nil || len(entries) == 0 {
		return info
	}

	// Use the first TPM device (typically tpm0).
	dev := entries[0].Name()
	info.Present = true

	// tpm_version_major exists on modern kernels: "2" -> 2.0, "1" -> 1.2.
	switch strings.TrimSpace(readSysfsFile(sysClassTPM + "/" + dev + "/tpm_version_major")) {
	case "2":
		info.Version = "2.0"
	case "1":
		info.Version = "1.2"
	}

	// Manufacturer / vendor identifier when exposed.
	if mfr := readSysfsFile(sysClassTPM + "/" + dev + "/device/description"); mfr != "" {
		info.Manufacturer = mfr
	}

	return info
}

// DetectBootInfo determines the Unraid boot device by inspecting /proc/mounts.
// Unraid 7.3 supports booting from internal media (NVMe/SSD/eMMC) and ZFS boot
// pools in addition to the traditional USB flash drive.
func DetectBootInfo() *dto.BootInfo {
	info := &dto.BootInfo{DeviceType: "unknown"}

	mountDev, fsType := bootMount()
	if mountDev == "" {
		return info
	}
	info.FileSystem = fsType

	// A ZFS-backed /boot is a boot pool; the pool name is the first path segment.
	if fsType == "zfs" {
		info.DeviceType = "internal"
		info.BootPool = strings.SplitN(mountDev, "/", 2)[0]
		return info
	}

	info.Device = strings.TrimPrefix(mountDev, "/dev/")

	// Resolve the parent block device and check its "removable" flag.
	base := baseBlockDevice(info.Device)
	switch strings.TrimSpace(readSysfsFile(sysBlockPath + "/" + base + "/removable")) {
	case "1":
		info.DeviceType = "usb"
	case "0":
		info.DeviceType = "internal"
	}

	return info
}

// bootMount returns the device and filesystem type mounted at /boot.
func bootMount() (device, fsType string) {
	// #nosec G304 -- procMountsPath is the fixed /proc/mounts kernel file.
	file, err := os.Open(procMountsPath)
	if err != nil {
		return "", ""
	}
	defer func() { _ = file.Close() }()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 3 && fields[1] == "/boot" {
			return fields[0], fields[2]
		}
	}
	return "", ""
}

// baseBlockDevice strips a partition suffix to get the parent block device name.
// e.g. "sda1" -> "sda", "nvme0n1p1" -> "nvme0n1", "mmcblk0p1" -> "mmcblk0".
func baseBlockDevice(part string) string {
	// NVMe/eMMC use a "p" partition separator (nvme0n1p1, mmcblk0p1).
	if strings.Contains(part, "nvme") || strings.HasPrefix(part, "mmcblk") {
		if idx := strings.LastIndex(part, "p"); idx > 0 {
			return part[:idx]
		}
		return part
	}
	// SCSI/SATA: trim trailing digits (sda1 -> sda).
	return strings.TrimRight(part, "0123456789")
}
