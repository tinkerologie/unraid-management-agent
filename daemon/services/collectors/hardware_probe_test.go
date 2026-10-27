package collectors

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tinkerologie/unraid-management-agent/daemon/constants"
	"github.com/tinkerologie/unraid-management-agent/daemon/dto"
	"github.com/tinkerologie/unraid-management-agent/daemon/lib"
)

// hwRoot is a fake filesystem root for HardwareProbe; sys is its /sys tree.
type hwRoot struct {
	t    *testing.T
	root string
	sys  sysfsTree
}

// newHWRoot creates an empty fake filesystem root.
func newHWRoot(t *testing.T) hwRoot {
	t.Helper()
	root := t.TempDir()
	return hwRoot{t: t, root: root, sys: sysfsTree{t: t, root: filepath.Join(root, "sys")}}
}

// file writes a file at an absolute system path below the root.
func (h hwRoot) file(abs, content string) {
	h.t.Helper()
	sysfsTree{t: h.t, root: h.root}.write(abs, content)
}

// dir creates a directory at an absolute system path below the root.
func (h hwRoot) dir(abs string) {
	h.t.Helper()
	sysfsTree{t: h.t, root: h.root}.mkdir(abs)
}

// probe returns a HardwareProbe on this root whose module reader fails the
// test unless reads are expected; reads counts the calls.
func (h hwRoot) probe(read func(string) (*lib.ModuleEEPROM, error)) (*HardwareProbe, *int) {
	reads := 0
	if read == nil {
		read = func(ifname string) (*lib.ModuleEEPROM, error) {
			h.t.Errorf("unexpected module EEPROM read on %s", ifname)
			return nil, lib.ErrModuleUnsupported
		}
	}
	return &HardwareProbe{Root: h.root, MaxModulePorts: defaultMaxModulePorts, ReadModuleFn: func(ifname string) (*lib.ModuleEEPROM, error) {
		reads++
		return read(ifname)
	}}, &reads
}

// Onboard NICs of the reference server without the hardware (#261).
// i219v adds an onboard Intel I219-V (e1000e) without sensors.
func (h hwRoot) i219v() {
	dev := h.sys.pciFunction("devices/pci0000:00/0000:00:1f.6", "8086", "15bc", "1849", "15bc", "e1000e")
	h.sys.iface("eth1", dev, up("1000", nil))
}

// i211 adds an onboard Intel I211 (igb) without sensors.
func (h hwRoot) i211() {
	dev := h.sys.pciFunction("devices/pci0000:00/0000:00:1c.0/0000:02:00.0", "8086", "1539", "1849", "1539", "igb")
	h.sys.iface("eth0", dev, up("1000", nil))
}

// wifi3168 adds an Intel 3168NGW Wi-Fi card with a hwmon temperature.
func (h hwRoot) wifi3168() {
	dev := h.sys.pciFunction("devices/pci0000:00/0000:00:1c.6/0000:03:00.0", "8086", "24fb", "8086", "2110", "iwlwifi")
	h.sys.iface("wlan0", dev, up("0", map[string]string{"wireless/.keep": ""}))
	h.sys.mkdir("class/net/wlan0/phy80211")
	h.sys.hwmon("hwmon3", dev, map[string]string{"name": "iwlwifi_1", "temp1_input": "45000"})
}

// moduleErr returns a module reader that always fails with err.
func moduleErr(err error) func(string) (*lib.ModuleEEPROM, error) {
	return func(string) (*lib.ModuleEEPROM, error) { return nil, err }
}

// TestCheckNICOptics covers the NIC rule: onboard and Wi-Fi NICs, optics, sensors, caps, errors and cancellation.
func TestCheckNICOptics(t *testing.T) {
	ctx := context.Background()

	t.Run("i219v", func(t *testing.T) {
		h := newHWRoot(t)
		h.i219v()
		p, reads := h.probe(moduleErr(lib.ErrModuleUnsupported))
		assertState(t, p.CheckNICOptics, dto.CapabilityAbsent, "no NIC with optics")
		if *reads != 1 {
			t.Errorf("module reads = %d, want 1", *reads)
		}
	})

	t.Run("i211", func(t *testing.T) {
		h := newHWRoot(t)
		h.i211()
		p, _ := h.probe(moduleErr(lib.ErrNoModule))
		assertState(t, p.CheckNICOptics, dto.CapabilityAbsent, "")
	})

	t.Run("3168ngw wifi with temperature sensor", func(t *testing.T) {
		h := newHWRoot(t)
		h.wifi3168()
		p, _ := h.probe(nil)
		assertState(t, p.CheckNICOptics, dto.CapabilityAbsent, "no wired PCI network card")
	})

	t.Run("onboard mix", func(t *testing.T) {
		h := newHWRoot(t)
		h.i219v()
		h.i211()
		h.wifi3168()
		var read []string
		p, _ := h.probe(func(ifname string) (*lib.ModuleEEPROM, error) {
			read = append(read, ifname)
			if ifname == "eth0" {
				return nil, lib.ErrNoModule
			}
			return nil, lib.ErrModuleUnsupported
		})
		assertState(t, p.CheckNICOptics, dto.CapabilityAbsent, "")
		if strings.Join(read, ",") != "eth0,eth1" {
			t.Errorf("module reads = %v, want eth0,eth1 (never the Wi-Fi card)", read)
		}
	})

	t.Run("qsfp module", func(t *testing.T) {
		h := newHWRoot(t)
		dev := h.sys.pciFunction("devices/pci0000:40/0000:40:01.1/0000:41:00.0", "15b3", "101d", "15b3", "0016", "mlx5_core")
		h.sys.iface("eth2", dev, up("100000", nil))
		data := readSFFFixture(t, "qsfp28_aoc_port0.bin")
		p, _ := h.probe(func(string) (*lib.ModuleEEPROM, error) {
			return &lib.ModuleEEPROM{Type: lib.ModuleTypeSFF8636, Data: data}, nil
		})
		assertState(t, p.CheckNICOptics, dto.CapabilityPresent, "QSFP28 module on eth2 (0000:41:00.0)")
	})

	t.Run("hwmon sensor needs no module read", func(t *testing.T) {
		h := newHWRoot(t)
		dev := h.sys.pciFunction("devices/pci0000:60/0000:60:03.3/0000:d5:00.0", "1d6a", "94c0", "1849", "d113", "atlantic")
		h.sys.iface("eth2", dev, up("10000", nil))
		h.sys.hwmon("hwmon11", dev, map[string]string{"name": "eth2", "temp1_input": "42000", "temp1_label": "PHY Temperature"})
		p, reads := h.probe(nil)
		assertState(t, p.CheckNICOptics, dto.CapabilityPresent, "NIC temperature sensor on 0000:d5:00.0")
		if *reads != 0 {
			t.Errorf("module reads = %d, want 0", *reads)
		}
	})

	t.Run("empty cage is not detected", func(t *testing.T) {
		h := newHWRoot(t)
		dev := h.sys.pciFunction("devices/pci0000:00/0000:00:03.0/0000:05:00.0", "8086", "1572", "8086", "0001", "i40e")
		h.sys.iface("eth4", dev, up("0", nil))
		p, _ := h.probe(moduleErr(lib.ErrNoModule))
		assertState(t, p.CheckNICOptics, dto.CapabilityAbsent, "")
	})

	t.Run("invalid hwmon reading", func(t *testing.T) {
		h := newHWRoot(t)
		dev := h.sys.pciFunction("devices/pci0000:00/0000:00:03.0/0000:05:00.0", "8086", "1572", "8086", "0001", "i40e")
		h.sys.iface("eth4", dev, up("0", nil))
		h.sys.hwmon("hwmon9", dev, map[string]string{"name": "i40e", "temp1_input": "bogus"})
		p, _ := h.probe(moduleErr(lib.ErrNoModule))
		assertState(t, p.CheckNICOptics, dto.CapabilityAbsent, "")
	})

	t.Run("non pluggable module", func(t *testing.T) {
		h := newHWRoot(t)
		h.i211()
		p, _ := h.probe(func(string) (*lib.ModuleEEPROM, error) {
			return &lib.ModuleEEPROM{Type: lib.ModuleTypeSFF8079, Data: []byte{0x02}}, nil // soldered module
		})
		assertState(t, p.CheckNICOptics, dto.CapabilityAbsent, "")
	})

	t.Run("module reads stop at the port cap", func(t *testing.T) {
		h := newHWRoot(t)
		for i := range 5 {
			name := "eth" + string(rune('0'+i))
			dev := h.sys.pciFunction("devices/pci0000:00/0000:00:03.0/0000:0"+string(rune('5'+i))+":00.0", "8086", "1572", "8086", "0001", "i40e")
			h.sys.iface(name, dev, up("0", nil))
		}
		p, reads := h.probe(moduleErr(lib.ErrNoModule))
		p.MaxModulePorts = 2
		assertState(t, p.CheckNICOptics, dto.CapabilityAbsent, "")
		if *reads != 2 {
			t.Errorf("module reads = %d, want 2", *reads)
		}
	})

	t.Run("module error is unknown unless another port qualifies", func(t *testing.T) {
		h := newHWRoot(t)
		h.i211()
		h.i219v()
		p, _ := h.probe(moduleErr(errors.New("i2c timeout")))
		assertState(t, p.CheckNICOptics, dto.CapabilityUnknown, "eth0: i2c timeout")

		data := readSFFFixture(t, "qsfp28_aoc_port0.bin")
		p, _ = h.probe(func(ifname string) (*lib.ModuleEEPROM, error) {
			if ifname == "eth0" {
				return nil, errors.New("i2c timeout")
			}
			return &lib.ModuleEEPROM{Type: lib.ModuleTypeSFF8636, Data: data}, nil
		})
		assertState(t, p.CheckNICOptics, dto.CapabilityPresent, "on eth1")
	})

	t.Run("cancelled before module reads", func(t *testing.T) {
		h := newHWRoot(t)
		h.i211()
		p, reads := h.probe(moduleErr(lib.ErrNoModule))
		cctx, cancel := context.WithCancel(ctx)
		cancel()
		if state, _ := p.CheckNICOptics(cctx); state != dto.CapabilityUnknown {
			t.Errorf("state = %q, want unknown", state)
		}
		if *reads != 0 {
			t.Errorf("module reads = %d after cancel", *reads)
		}
	})

	t.Run("no network interfaces", func(t *testing.T) {
		h := newHWRoot(t)
		p, _ := h.probe(nil)
		assertState(t, p.CheckNICOptics, dto.CapabilityAbsent, "")
	})

	t.Run("unreadable interfaces", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("root ignores directory permissions")
		}
		h := newHWRoot(t)
		dir := h.sys.mkdir("class/net")
		if err := os.Chmod(dir, 0); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
		p, _ := h.probe(nil)
		assertState(t, p.CheckNICOptics, dto.CapabilityUnknown, "cannot read network interfaces")
	})
}

// TestCheckStorageTopology covers SES devices, enclosures, SAS and RAID controllers and tool detail.
func TestCheckStorageTopology(t *testing.T) {
	t.Run("SES devices from the storage topology fixture without sg_ses", func(t *testing.T) {
		root := t.TempDir()
		abs, err := filepath.Abs(filepath.Join("testdata", "storage_topology", "sysfs"))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(abs, filepath.Join(root, "sys")); err != nil {
			t.Fatal(err)
		}
		p := &HardwareProbe{Root: root}
		assertState(t, p.CheckStorageTopology, dto.CapabilityPresent, "SES device sg0")
		assertState(t, p.CheckStorageTopology, dto.CapabilityPresent, "storcli not found, sg_ses not found")
	})

	t.Run("enclosure class entry with tools", func(t *testing.T) {
		h := newHWRoot(t)
		h.sys.mkdir("class/enclosure/0:0:12:0")
		h.file("/sbin/storcli", "#!/bin/sh")
		if err := os.Chmod(filepath.Join(h.root, "sbin", "storcli"), 0o755); err != nil {
			t.Fatal(err)
		}
		p, _ := h.probe(nil)
		assertState(t, p.CheckStorageTopology, dto.CapabilityPresent, "SES enclosure 0:0:12:0; storcli found, sg_ses not found")
	})

	pci := func(t *testing.T, class, vendor string) *HardwareProbe {
		h := newHWRoot(t)
		h.sys.write("bus/pci/devices/0000:01:00.0/class", class)
		h.sys.write("bus/pci/devices/0000:01:00.0/vendor", vendor)
		h.sys.write("class/scsi_generic/sg0/device/type", "0")
		p, _ := h.probe(nil)
		return p
	}
	t.Run("SAS HBA", func(t *testing.T) {
		assertState(t, pci(t, "0x010700", "0x1000").CheckStorageTopology, dto.CapabilityPresent, "SAS controller at 0000:01:00.0")
	})
	t.Run("Broadcom RAID", func(t *testing.T) {
		assertState(t, pci(t, "0x010400", "0x1000").CheckStorageTopology, dto.CapabilityPresent, "RAID controller at 0000:01:00.0")
	})
	t.Run("Intel RST RAID is not SAS", func(t *testing.T) {
		assertState(t, pci(t, "0x010400", "0x8086").CheckStorageTopology, dto.CapabilityAbsent, "no SAS HBA")
	})
	t.Run("AHCI and NVMe only", func(t *testing.T) {
		assertState(t, pci(t, "0x010601", "0x8086").CheckStorageTopology, dto.CapabilityAbsent, "no SAS HBA")
	})
}

// TestCheckGPU covers supported GPU drivers, BMC adapters and NVIDIA device nodes.
func TestCheckGPU(t *testing.T) {
	gpu := func(t *testing.T, driver string) hwRoot {
		h := newHWRoot(t)
		dev := h.sys.pciFunction("devices/pci0000:00/0000:00:02.0", "8086", "3e92", "1849", "3e92", driver)
		h.sys.link("class/drm/card0/device", dev)
		h.sys.mkdir("class/drm/card0-HDMI-A-1")
		return h
	}
	t.Run("intel", func(t *testing.T) {
		p, _ := gpu(t, "i915").probe(nil)
		assertState(t, p.CheckGPU, dto.CapabilityPresent, "GPU card0 (i915)")
	})
	t.Run("BMC display adapter", func(t *testing.T) {
		p, _ := gpu(t, "ast").probe(nil)
		assertState(t, p.CheckGPU, dto.CapabilityAbsent, "")
	})
	t.Run("nvidia device node", func(t *testing.T) {
		h := newHWRoot(t)
		h.file("/dev/nvidia0", "")
		h.file("/dev/nvidiactl", "")
		p, _ := h.probe(nil)
		assertState(t, p.CheckGPU, dto.CapabilityPresent, "NVIDIA device nvidia0")
	})
	t.Run("none", func(t *testing.T) {
		p, _ := newHWRoot(t).probe(nil)
		assertState(t, p.CheckGPU, dto.CapabilityAbsent, "")
	})
}

// TestCheckUPSAndNUT covers apcupsd and NUT detection without contacting a UPS.
func TestCheckUPSAndNUT(t *testing.T) {
	t.Run("apcupsd running", func(t *testing.T) {
		h := newHWRoot(t)
		h.file(constants.ApcPidFile, "1234")
		p, _ := h.probe(nil)
		assertState(t, p.CheckUPS, dto.CapabilityPresent, "apcupsd running")
	})
	t.Run("apcupsd enabled", func(t *testing.T) {
		h := newHWRoot(t)
		h.file(constants.ApcupsdPluginCfg, `SERVICE="enable"`+"\nUPSCABLE=\"usb\"")
		p, _ := h.probe(nil)
		assertState(t, p.CheckUPS, dto.CapabilityPresent, "apcupsd enabled")
	})
	t.Run("apcupsd disabled", func(t *testing.T) {
		h := newHWRoot(t)
		h.file(constants.ApcupsdPluginCfg, `SERVICE="disable"`)
		p, _ := h.probe(nil)
		assertState(t, p.CheckUPS, dto.CapabilityAbsent, "no apcupsd or NUT")
	})
	t.Run("stale NUT settings without the plugin", func(t *testing.T) {
		h := newHWRoot(t)
		h.file(constants.NutPluginCfg, `SERVICE="enable"`)
		p, _ := h.probe(nil)
		assertState(t, p.CheckNUT, dto.CapabilityAbsent, "NUT plugin not installed")
		assertState(t, p.CheckUPS, dto.CapabilityAbsent, "")
	})
	t.Run("NUT enabled", func(t *testing.T) {
		h := newHWRoot(t)
		h.dir(constants.NutPluginDir)
		h.file(constants.NutPluginCfg, `SERVICE="enable"`)
		p, _ := h.probe(nil)
		assertState(t, p.CheckNUT, dto.CapabilityPresent, "NUT enabled")
		assertState(t, p.CheckUPS, dto.CapabilityPresent, "NUT enabled")
	})
	t.Run("NUT running", func(t *testing.T) {
		h := newHWRoot(t)
		h.dir(constants.NutPluginDir)
		h.file(constants.NutPidFile, "99")
		p, _ := h.probe(nil)
		assertState(t, p.CheckNUT, dto.CapabilityPresent, "NUT upsmon running")
	})
	t.Run("NUT installed but disabled", func(t *testing.T) {
		h := newHWRoot(t)
		h.dir(constants.NutPluginDir)
		h.file(constants.NutPluginCfg, `SERVICE="disable"`)
		p, _ := h.probe(nil)
		assertState(t, p.CheckNUT, dto.CapabilityAbsent, "not enabled")
	})
	t.Run("unreadable apcupsd settings", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("root ignores file permissions")
		}
		h := newHWRoot(t)
		h.file(constants.ApcupsdPluginCfg, `SERVICE="enable"`)
		if err := os.Chmod(filepath.Join(h.root, constants.ApcupsdPluginCfg), 0); err != nil {
			t.Fatal(err)
		}
		p, _ := h.probe(nil)
		assertState(t, p.CheckUPS, dto.CapabilityUnknown, "cannot read UPS configuration")
	})
}

// TestCheckZFS covers the ZFS module and imported pool detection.
func TestCheckZFS(t *testing.T) {
	t.Run("module not loaded", func(t *testing.T) {
		p, _ := newHWRoot(t).probe(nil)
		assertState(t, p.CheckZFS, dto.CapabilityAbsent, "ZFS module not loaded")
	})
	t.Run("module without pools", func(t *testing.T) {
		h := newHWRoot(t)
		h.dir("/sys/module/zfs")
		h.file("/proc/spl/kstat/zfs/arcstats", "")
		h.file("/proc/spl/kstat/zfs/zil", "")
		p, _ := h.probe(nil)
		assertState(t, p.CheckZFS, dto.CapabilityAbsent, "no imported ZFS pools")
	})
	t.Run("imported pool", func(t *testing.T) {
		h := newHWRoot(t)
		h.dir("/sys/module/zfs")
		h.file("/proc/spl/kstat/zfs/arcstats", "")
		h.file("/proc/spl/kstat/zfs/tank/state", "ONLINE")
		p, _ := h.probe(nil)
		assertState(t, p.CheckZFS, dto.CapabilityPresent, "ZFS pool tank")
	})
}

// TestCheckFanControl covers PWM outputs and IPMI devices.
func TestCheckFanControl(t *testing.T) {
	t.Run("pwm with enable", func(t *testing.T) {
		h := newHWRoot(t)
		h.sys.hwmon("hwmon4", "", map[string]string{"name": "nct6793", "pwm1": "128", "pwm1_enable": "5", "temp1_input": "30000"})
		p, _ := h.probe(nil)
		assertState(t, p.CheckFanControl, dto.CapabilityPresent, "PWM fan control on nct6793 (pwm1)")
	})
	t.Run("pwm without enable", func(t *testing.T) {
		h := newHWRoot(t)
		h.sys.hwmon("hwmon4", "", map[string]string{"pwm1": "128"})
		p, _ := h.probe(nil)
		assertState(t, p.CheckFanControl, dto.CapabilityAbsent, "")
	})
	t.Run("ipmi", func(t *testing.T) {
		h := newHWRoot(t)
		h.file("/dev/ipmi0", "")
		p, _ := h.probe(nil)
		assertState(t, p.CheckFanControl, dto.CapabilityPresent, "IPMI device ipmi0")
	})
}

// TestHardwareProbeReferenceServer models the maintainer's server without the
// optional hardware (#261): I219-V, I211 and 3168NGW NICs without sensors or
// optics, AHCI and NVMe storage only, an Intel iGPU, apcupsd, a stale NUT
// config without the plugin, the ZFS module without pools, and a Nuvoton
// Super I/O with PWM fans.
func TestHardwareProbeReferenceServer(t *testing.T) {
	h := newHWRoot(t)
	h.i219v()
	h.i211()
	h.wifi3168()
	h.sys.write("bus/pci/devices/0000:00:17.0/class", "0x010601")
	h.sys.write("bus/pci/devices/0000:01:00.0/class", "0x010802")
	for _, sg := range []string{"sg0", "sg1"} {
		h.sys.write("class/scsi_generic/"+sg+"/device/type", "0")
	}
	igpu := h.sys.pciFunction("devices/pci0000:00/0000:00:02.0", "8086", "3e92", "1849", "3e92", "i915")
	h.sys.link("class/drm/card0/device", igpu)
	h.file(constants.ApcPidFile, "1234")
	h.file(constants.NutPluginCfg, `SERVICE="enable"`)
	h.dir("/sys/module/zfs")
	h.file("/proc/spl/kstat/zfs/arcstats", "")
	h.sys.hwmon("hwmon4", "", map[string]string{"name": "nct6793", "pwm1": "128", "pwm1_enable": "5"})
	p, _ := h.probe(func(ifname string) (*lib.ModuleEEPROM, error) {
		if ifname == "eth0" {
			return nil, lib.ErrNoModule
		}
		return nil, lib.ErrModuleUnsupported
	})

	want := map[string]dto.CapabilityState{
		CapabilityPSU:             dto.CapabilityAbsent,
		CapabilityNICOptics:       dto.CapabilityAbsent,
		CapabilityStorageTopology: dto.CapabilityAbsent,
		CapabilityGPU:             dto.CapabilityPresent,
		CapabilityUPS:             dto.CapabilityPresent,
		CapabilityNUT:             dto.CapabilityAbsent,
		CapabilityZFS:             dto.CapabilityAbsent,
		CapabilityFanControl:      dto.CapabilityPresent,
	}
	for _, class := range HardwareClasses {
		check := p.CheckFor(class.Capability)
		if check == nil {
			t.Fatalf("no check for %s", class.Capability)
		}
		if state, detail := check(context.Background()); state != want[class.Capability] {
			t.Errorf("%s = %q (%s), want %q", class.Capability, state, detail, want[class.Capability])
		}
	}
}

// TestHardwareClasses checks probe defaults and the class-to-collector mapping.
func TestHardwareClasses(t *testing.T) {
	p := NewHardwareProbe()
	if p.Root != "/" || p.ReadModuleFn == nil || p.MaxModulePorts != defaultMaxModulePorts {
		t.Errorf("unexpected defaults: %+v", p)
	}
	if p.CheckFor("hardware.nope") != nil {
		t.Error("CheckFor returned a check for an unknown capability")
	}
	gated := map[string]string{}
	for _, c := range HardwareClasses {
		if c.Collector != "" {
			gated[c.Collector] = c.Capability
		}
	}
	for _, name := range []string{"nic", "storage_topology", "gpu", "ups", "nut", "zfs"} {
		class, ok := HardwareClassForCollector(name)
		if !ok || class.Capability != gated[name] {
			t.Errorf("HardwareClassForCollector(%q) = %+v, %v", name, class, ok)
		}
	}
	for _, name := range []string{"", "system", "fancontrol"} {
		if _, ok := HardwareClassForCollector(name); ok {
			t.Errorf("%q must not be hardware-gated", name)
		}
	}
}

// assertState runs a check and asserts its state and detail.
func assertState(t *testing.T, check func(context.Context) (dto.CapabilityState, string), want dto.CapabilityState, detail string) {
	t.Helper()
	state, got := check(context.Background())
	if state != want {
		t.Errorf("state = %q (%s), want %q", state, got, want)
	}
	if !strings.Contains(got, detail) {
		t.Errorf("detail = %q, want it to contain %q", got, detail)
	}
}

// lockDir makes a directory below the root unreadable for the rest of the test.
func (h hwRoot) lockDir(abs string) {
	h.t.Helper()
	if os.Geteuid() == 0 {
		h.t.Skip("root ignores directory permissions")
	}
	dir := filepath.Join(h.root, abs)
	sysfsTree{t: h.t, root: h.root}.mkdir(abs)
	if err := os.Chmod(dir, 0); err != nil {
		h.t.Fatal(err)
	}
	h.t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
}

// TestHardwareProbeReadErrors checks that a read error other than "not found"
// reports unknown, so the policy never acts on it.
func TestHardwareProbeReadErrors(t *testing.T) {
	tests := []struct {
		name   string
		locked string
		check  func(*HardwareProbe) func(context.Context) (dto.CapabilityState, string)
		detail string
	}{
		{"storage enclosures", "/sys/class/enclosure", func(p *HardwareProbe) func(context.Context) (dto.CapabilityState, string) {
			return p.CheckStorageTopology
		}, "cannot read storage controllers"},
		{"storage PCI devices", "/sys/bus/pci/devices", func(p *HardwareProbe) func(context.Context) (dto.CapabilityState, string) {
			return p.CheckStorageTopology
		}, "cannot read storage controllers"},
		{"GPU DRM cards", "/sys/class/drm", func(p *HardwareProbe) func(context.Context) (dto.CapabilityState, string) { return p.CheckGPU }, "cannot read GPUs"},
		{"ZFS module", "/sys/module", func(p *HardwareProbe) func(context.Context) (dto.CapabilityState, string) { return p.CheckZFS }, "cannot read ZFS module"},
		{"fan hwmon", "/sys/class/hwmon", func(p *HardwareProbe) func(context.Context) (dto.CapabilityState, string) { return p.CheckFanControl }, "cannot read hwmon"},
		{"NUT plugin", "/usr/local/emhttp/plugins", func(p *HardwareProbe) func(context.Context) (dto.CapabilityState, string) { return p.CheckNUT }, "cannot read NUT plugin"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHWRoot(t)
			h.lockDir(tt.locked)
			p, _ := h.probe(nil)
			assertState(t, tt.check(p), dto.CapabilityUnknown, tt.detail)
		})
	}

	t.Run("ZFS pools", func(t *testing.T) {
		h := newHWRoot(t)
		h.dir("/sys/module/zfs")
		h.lockDir("/proc/spl/kstat/zfs")
		p, _ := h.probe(nil)
		assertState(t, p.CheckZFS, dto.CapabilityUnknown, "cannot read ZFS pools")
	})
	t.Run("NUT settings", func(t *testing.T) {
		h := newHWRoot(t)
		h.dir(constants.NutPluginDir)
		h.file(constants.NutPluginCfg, `SERVICE="enable"`)
		if os.Geteuid() == 0 {
			t.Skip("root ignores file permissions")
		}
		if err := os.Chmod(filepath.Join(h.root, constants.NutPluginCfg), 0); err != nil {
			t.Fatal(err)
		}
		p, _ := h.probe(nil)
		assertState(t, p.CheckNUT, dto.CapabilityUnknown, "cannot read NUT settings")
	})
	t.Run("unreadable hwmon chip is skipped", func(t *testing.T) {
		h := newHWRoot(t)
		h.lockDir("/sys/class/hwmon/hwmon0")
		h.sys.hwmon("hwmon1", "", map[string]string{"pwm2": "100", "pwm2_enable": "1"})
		p, _ := h.probe(nil)
		assertState(t, p.CheckFanControl, dto.CapabilityPresent, "PWM fan control on hwmon1 (pwm2)")
	})
}

// TestHardwareProbeFallbacks covers the module-read cap default, a DRM card
// without a driver, and a hwmon chip without a name.
func TestHardwareProbeFallbacks(t *testing.T) {
	t.Run("zero port cap uses the default", func(t *testing.T) {
		h := newHWRoot(t)
		h.i211()
		p, reads := h.probe(moduleErr(lib.ErrNoModule))
		p.MaxModulePorts = 0
		assertState(t, p.CheckNICOptics, dto.CapabilityAbsent, "")
		if *reads != 1 {
			t.Errorf("module reads = %d, want 1", *reads)
		}
	})
	t.Run("DRM card without a driver", func(t *testing.T) {
		h := newHWRoot(t)
		h.sys.mkdir("class/drm/card0/device")
		p, _ := h.probe(nil)
		assertState(t, p.CheckGPU, dto.CapabilityAbsent, "no Intel, AMD or NVIDIA GPU")
	})
	t.Run("hwmon chip without a name", func(t *testing.T) {
		h := newHWRoot(t)
		h.sys.hwmon("hwmon7", "", map[string]string{"pwm1": "100", "pwm1_enable": "2"})
		p, _ := h.probe(nil)
		assertState(t, p.CheckFanControl, dto.CapabilityPresent, "PWM fan control on hwmon7 (pwm1)")
	})
}
