//go:build linux

package lib

import (
	"encoding/binary"
	"errors"
	"strings"
	"testing"
	"unsafe"

	"golang.org/x/sys/unix"
)

// fakeModule answers ETHTOOL_GMODULEINFO / ETHTOOL_GMODULEEEPROM like a driver.
type fakeModule struct {
	typ, eepromLen uint32
	data           []byte
	infoErr        error
	eepromErr      error
	// shortRead makes the driver return fewer bytes than requested.
	shortRead uint32
	// requested is the length asked for in ETHTOOL_GMODULEEEPROM.
	requested uint32
	ifname    string
}

func (f *fakeModule) ioctl(_ int, ifr *ifreqData) error {
	f.ifname = unix.ByteSliceToString(ifr.Name[:])
	switch cmd := *(*uint32)(ifr.Data); cmd {
	case ethtoolGModuleInfo:
		if f.infoErr != nil {
			return f.infoErr
		}
		info := (*ethtoolModInfo)(ifr.Data)
		info.Type, info.EEPROMLen = f.typ, f.eepromLen
		return nil
	case ethtoolGModuleEEPROM:
		if f.eepromErr != nil {
			return f.eepromErr
		}
		hdr := unsafe.Slice((*byte)(ifr.Data), ethtoolEEPROMHeader)
		f.requested = binary.NativeEndian.Uint32(hdr[12:])
		n := f.requested
		if f.shortRead != 0 {
			n = f.shortRead
			binary.NativeEndian.PutUint32(hdr[12:], n)
		}
		buf := unsafe.Slice((*byte)(ifr.Data), ethtoolEEPROMHeader+int(f.requested))
		copy(buf[ethtoolEEPROMHeader:ethtoolEEPROMHeader+int(n)], f.data)
		return nil
	default:
		return unix.EINVAL
	}
}

func withFakeEthtool(t *testing.T, f *fakeModule) {
	t.Helper()
	origIoctl, origSocket := ethtoolIoctl, openEthtoolSocket
	ethtoolIoctl = f.ioctl
	openEthtoolSocket = func() (int, error) { return unix.Socket(unix.AF_UNIX, unix.SOCK_DGRAM|unix.SOCK_CLOEXEC, 0) }
	t.Cleanup(func() { ethtoolIoctl, openEthtoolSocket = origIoctl, origSocket })
}

func TestReadModuleEEPROM(t *testing.T) {
	qsfp := make([]byte, 640)
	for i := range qsfp {
		qsfp[i] = byte(i)
	}

	t.Run("reads the whole EEPROM", func(t *testing.T) {
		f := &fakeModule{typ: ModuleTypeSFF8636, eepromLen: 640, data: qsfp}
		withFakeEthtool(t, f)
		got, err := ReadModuleEEPROM("eth0")
		if err != nil {
			t.Fatal(err)
		}
		if got.Type != ModuleTypeSFF8636 || len(got.Data) != 640 || got.Data[639] != byte(639%256) || f.ifname != "eth0" {
			t.Errorf("type=%d len=%d last=%d ifname=%q", got.Type, len(got.Data), got.Data[639], f.ifname)
		}
	})

	t.Run("caps the length and honours short reads", func(t *testing.T) {
		f := &fakeModule{typ: ModuleTypeSFF8472, eepromLen: 4096, data: qsfp, shortRead: 512}
		withFakeEthtool(t, f)
		got, err := ReadModuleEEPROM("eth1")
		if err != nil {
			t.Fatal(err)
		}
		if f.requested != 640 || len(got.Data) != 512 {
			t.Errorf("requested=%d len=%d", f.requested, len(got.Data))
		}
	})

	t.Run("zero-length EEPROM means no module", func(t *testing.T) {
		withFakeEthtool(t, &fakeModule{typ: ModuleTypeSFF8636})
		if _, err := ReadModuleEEPROM("eth0"); !errors.Is(err, ErrNoModule) {
			t.Errorf("err = %v", err)
		}
	})

	errorCases := []struct {
		name string
		f    *fakeModule
		want error
		text string
	}{
		{"driver without module support", &fakeModule{infoErr: unix.EOPNOTSUPP}, ErrModuleUnsupported, ""},
		{"empty cage", &fakeModule{infoErr: unix.EIO}, ErrNoModule, "ETHTOOL_GMODULEINFO"},
		{"unreadable module", &fakeModule{typ: 3, eepromLen: 256, eepromErr: unix.EINVAL}, ErrNoModule, "ETHTOOL_GMODULEEEPROM"},
		{"permission denied", &fakeModule{infoErr: unix.EPERM}, unix.EPERM, "ETHTOOL_GMODULEINFO"},
	}
	for _, tc := range errorCases {
		t.Run(tc.name, func(t *testing.T) {
			withFakeEthtool(t, tc.f)
			_, err := ReadModuleEEPROM("eth0")
			if !errors.Is(err, tc.want) || !strings.Contains(err.Error(), tc.text) {
				t.Errorf("err = %v, want %v", err, tc.want)
			}
		})
	}

	t.Run("rejects names longer than IFNAMSIZ", func(t *testing.T) {
		if _, err := ReadModuleEEPROM(strings.Repeat("x", unix.IFNAMSIZ)); err == nil {
			t.Error("expected an error")
		}
	})

	t.Run("socket failure", func(t *testing.T) {
		withFakeEthtool(t, &fakeModule{})
		openEthtoolSocket = func() (int, error) { return -1, unix.EMFILE }
		if _, err := ReadModuleEEPROM("eth0"); !errors.Is(err, unix.EMFILE) {
			t.Errorf("err = %v", err)
		}
	})

	t.Run("real ioctl on loopback", func(t *testing.T) {
		// lo has no module: EOPNOTSUPP as root, EPERM without CAP_NET_ADMIN.
		if _, err := ReadModuleEEPROM("lo"); err == nil {
			t.Error("expected an error for lo")
		}
	})
}

func TestErrnoErr(t *testing.T) {
	if errnoErr(0) != nil || !errors.Is(errnoErr(unix.EIO), unix.EIO) {
		t.Error("errnoErr")
	}
}

func TestReadFirmwareVersion(t *testing.T) {
	origDrv, origSocket := getDrvinfo, openEthtoolSocket
	t.Cleanup(func() { getDrvinfo, openEthtoolSocket = origDrv, origSocket })

	getDrvinfo = func(_ int, ifname string) (*unix.EthtoolDrvinfo, error) {
		if ifname != "eth0" {
			return nil, unix.ENODEV
		}
		info := &unix.EthtoolDrvinfo{}
		copy(info.Fw_version[:], "22.47.3576 (MT_0000000359)")
		return info, nil
	}
	if fw, err := ReadFirmwareVersion("eth0"); err != nil || fw != "22.47.3576 (MT_0000000359)" {
		t.Errorf("fw=%q err=%v", fw, err)
	}
	if _, err := ReadFirmwareVersion("eth9"); !errors.Is(err, unix.ENODEV) {
		t.Errorf("err = %v", err)
	}
	openEthtoolSocket = func() (int, error) { return -1, unix.EMFILE }
	if _, err := ReadFirmwareVersion("eth0"); !errors.Is(err, unix.EMFILE) {
		t.Errorf("err = %v", err)
	}
}
