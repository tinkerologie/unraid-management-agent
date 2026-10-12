//go:build linux

package lib

import (
	"encoding/binary"
	"errors"
	"fmt"
	"unsafe"

	"golang.org/x/sys/unix"
)

// ethtool commands (linux/ethtool.h).
const (
	ethtoolGModuleInfo   = 0x42
	ethtoolGModuleEEPROM = 0x43
	// ethtoolEEPROMHeader is sizeof(struct ethtool_eeprom) without its data.
	ethtoolEEPROMHeader = 16
)

// ethtoolModInfo mirrors struct ethtool_modinfo.
type ethtoolModInfo struct {
	Cmd       uint32
	Type      uint32
	EEPROMLen uint32
	Reserved  [8]uint32
}

// ifreqData mirrors struct ifreq with ifr_data set: the interface name and a
// pointer, padded to the size of the ifreq union. Keeping the pointer as an
// unsafe.Pointer lets the garbage collector see the referenced buffer.
type ifreqData struct {
	Name [unix.IFNAMSIZ]byte
	Data unsafe.Pointer
	_    [16]byte
}

// ethtoolIoctl issues SIOCETHTOOL on fd; replaceable for tests.
var ethtoolIoctl = sysEthtoolIoctl

// openEthtoolSocket opens the socket SIOCETHTOOL is issued on; replaceable for tests.
var openEthtoolSocket = func() (int, error) {
	return unix.Socket(unix.AF_INET, unix.SOCK_DGRAM|unix.SOCK_CLOEXEC, 0)
}

// getDrvinfo fetches ETHTOOL_GDRVINFO; replaceable for tests.
var getDrvinfo = unix.IoctlGetEthtoolDrvinfo

func sysEthtoolIoctl(fd int, ifr *ifreqData) error {
	//nolint:gosec // G103: SIOCETHTOOL takes a struct ifreq pointer.
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), unix.SIOCETHTOOL, uintptr(unsafe.Pointer(ifr)))
	return errnoErr(errno)
}

func errnoErr(errno unix.Errno) error {
	if errno == 0 {
		return nil
	}
	return errno
}

// ReadModuleEEPROM reads the EEPROM of the module plugged into ifname with
// the SIOCETHTOOL ETHTOOL_GMODULEINFO and ETHTOOL_GMODULEEEPROM ioctls. It
// returns ErrModuleUnsupported when the driver has no module support and
// ErrNoModule when no readable module is present. Reading needs CAP_NET_ADMIN.
func ReadModuleEEPROM(ifname string) (*ModuleEEPROM, error) {
	if len(ifname) >= unix.IFNAMSIZ {
		return nil, fmt.Errorf("interface name too long: %q", ifname)
	}
	fd, err := openEthtoolSocket()
	if err != nil {
		return nil, fmt.Errorf("open socket: %w", err)
	}
	defer func() { _ = unix.Close(fd) }()

	info := ethtoolModInfo{Cmd: ethtoolGModuleInfo}
	ifr := ifreqData{Data: unsafe.Pointer(&info)} //nolint:gosec // G103: ifr_data points at struct ethtool_modinfo.
	copy(ifr.Name[:], ifname)
	if err := ethtoolIoctl(fd, &ifr); err != nil {
		return nil, classifyModuleError("ETHTOOL_GMODULEINFO", err)
	}
	length := min(info.EEPROMLen, sff8636MaxLen)
	if length == 0 {
		return nil, ErrNoModule
	}

	// struct ethtool_eeprom {cmd, magic, offset, len; data[len]}.
	buf := make([]byte, ethtoolEEPROMHeader+int(length))
	binary.NativeEndian.PutUint32(buf[0:], ethtoolGModuleEEPROM)
	binary.NativeEndian.PutUint32(buf[12:], length)
	ifr.Data = unsafe.Pointer(&buf[0]) //nolint:gosec // G103: ifr_data points at struct ethtool_eeprom.
	if err := ethtoolIoctl(fd, &ifr); err != nil {
		return nil, classifyModuleError("ETHTOOL_GMODULEEEPROM", err)
	}
	got := min(binary.NativeEndian.Uint32(buf[12:]), length)
	return &ModuleEEPROM{Type: info.Type, Data: buf[ethtoolEEPROMHeader : ethtoolEEPROMHeader+int(got)]}, nil
}

// classifyModuleError maps driver errors to ErrModuleUnsupported (no module
// support) and ErrNoModule (empty cage or unreadable module: drivers return
// EIO, EINVAL, ENXIO or EAGAIN for these).
func classifyModuleError(op string, err error) error {
	switch {
	case errors.Is(err, unix.EOPNOTSUPP):
		return ErrModuleUnsupported
	case errors.Is(err, unix.EIO), errors.Is(err, unix.EINVAL),
		errors.Is(err, unix.ENXIO), errors.Is(err, unix.EAGAIN):
		return fmt.Errorf("%w: %s: %w", ErrNoModule, op, err)
	}
	return fmt.Errorf("%s: %w", op, err)
}

// ReadFirmwareVersion returns the firmware version the driver of ifname
// reports (ETHTOOL_GDRVINFO, as shown by `ethtool -i`).
func ReadFirmwareVersion(ifname string) (string, error) {
	fd, err := openEthtoolSocket()
	if err != nil {
		return "", fmt.Errorf("open socket: %w", err)
	}
	defer func() { _ = unix.Close(fd) }()
	info, err := getDrvinfo(fd, ifname)
	if err != nil {
		return "", fmt.Errorf("ETHTOOL_GDRVINFO: %w", err)
	}
	return unix.ByteSliceToString(info.Fw_version[:]), nil
}
