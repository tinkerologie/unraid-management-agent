package collectors

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tinkerologie/unraid-management-agent/daemon/constants"
	"github.com/tinkerologie/unraid-management-agent/daemon/domain"
	"github.com/tinkerologie/unraid-management-agent/daemon/dto"
	"github.com/tinkerologie/unraid-management-agent/daemon/lib"
)

// sysfsTree builds a fake /sys modelled on a server with a dual-port Mellanox
// ConnectX-6 Dx (0000:41:00.0/.1, eth0/eth1 in bond0), two single-port
// Aquantia AQC113CS cards (0000:d5:00.0, 0000:d6:00.0) and the virtual and
// non-PCI interfaces the collector must skip.
type sysfsTree struct {
	t    *testing.T
	root string
}

func (s sysfsTree) write(rel, content string) {
	s.t.Helper()
	p := filepath.Join(s.root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		s.t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content+"\n"), 0o644); err != nil {
		s.t.Fatal(err)
	}
}

func (s sysfsTree) mkdir(rel string) string {
	s.t.Helper()
	p := filepath.Join(s.root, rel)
	if err := os.MkdirAll(p, 0o755); err != nil {
		s.t.Fatal(err)
	}
	return p
}

func (s sysfsTree) link(rel, target string) {
	s.t.Helper()
	p := filepath.Join(s.root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		s.t.Fatal(err)
	}
	if err := os.Symlink(target, p); err != nil {
		s.t.Fatal(err)
	}
}

// pciFunction creates a PCI function directory with IDs and a bound driver.
func (s sysfsTree) pciFunction(rel, vendor, device, subVendor, subDevice, driver string) string {
	dir := s.mkdir(rel)
	s.write(filepath.Join(rel, "vendor"), "0x"+vendor)
	s.write(filepath.Join(rel, "device"), "0x"+device)
	s.write(filepath.Join(rel, "subsystem_vendor"), "0x"+subVendor)
	s.write(filepath.Join(rel, "subsystem_device"), "0x"+subDevice)
	if driver != "" {
		s.link(filepath.Join(rel, "driver"), s.mkdir("bus/pci/drivers/"+driver))
	}
	return dir
}

// iface creates /sys/class/net/<name> with the given attributes.
func (s sysfsTree) iface(name, device string, attrs map[string]string) {
	base := "class/net/" + name
	s.mkdir(base)
	if device != "" {
		s.link(base+"/device", device)
	}
	for k, v := range attrs {
		s.write(base+"/"+k, v)
	}
}

// hwmon creates /sys/class/hwmon/<name> pointing at device, with temp files.
func (s sysfsTree) hwmon(name, device string, files map[string]string) {
	base := "class/hwmon/" + name
	s.mkdir(base)
	if device != "" {
		s.link(base+"/device", device)
	}
	for k, v := range files {
		s.write(base+"/"+k, v)
	}
}

func up(speed string, extra map[string]string) map[string]string {
	attrs := map[string]string{"operstate": "up", "carrier": "1", "speed": speed, "duplex": "full", "mtu": "1500", "dev_port": "0"}
	for k, v := range extra {
		attrs[k] = v
	}
	return attrs
}

func newNICTree(t *testing.T) sysfsTree {
	t.Helper()
	s := sysfsTree{t: t, root: t.TempDir()}

	cx0 := s.pciFunction("devices/pci0000:40/0000:40:01.1/0000:41:00.0", "15b3", "101d", "15b3", "0016", "mlx5_core")
	cx1 := s.pciFunction("devices/pci0000:40/0000:40:01.1/0000:41:00.1", "15b3", "101d", "15b3", "0016", "mlx5_core")
	vf := s.pciFunction("devices/pci0000:40/0000:40:01.1/0000:41:00.2", "15b3", "101e", "15b3", "0016", "mlx5_core")
	s.link("devices/pci0000:40/0000:40:01.1/0000:41:00.2/physfn", cx0)
	aq0 := s.pciFunction("devices/pci0000:60/0000:60:03.3/0000:d5:00.0", "1d6a", "94c0", "1849", "d113", "atlantic")
	aq1 := s.pciFunction("devices/pci0000:60/0000:60:03.4/0000:d6:00.0", "1d6a", "94c0", "1849", "d113", "atlantic")
	// A ConnectX-3 style card: two ports on one PCI function (dev_port 0/1),
	// no driver link and a PHY hwmon chip below the function.
	mlx4 := s.pciFunction("devices/pci0000:00/0000:00:02.0/0000:03:00.0", "15b3", "1003", "15b3", "0050", "")
	phy := s.mkdir("devices/pci0000:00/0000:00:02.0/0000:03:00.0/mdio_bus/phy0")
	nvme := s.pciFunction("devices/pci0000:20/0000:20:01.1/0000:27:00.0", "144d", "a80a", "144d", "a801", "nvme")
	usb := s.mkdir("devices/pci0000:00/0000:00:14.0/usb1/1-1/1-1:1.0")

	s.iface("bond0", "", map[string]string{"operstate": "up"})
	s.iface("eth0", cx0, up("100000", map[string]string{"phys_port_name": "p0"}))
	s.link("class/net/eth0/master", filepath.Join(s.root, "class/net/bond0"))
	s.iface("eth1", cx1, up("100000", map[string]string{"phys_port_name": "p1"}))
	s.link("class/net/eth1/master", filepath.Join(s.root, "class/net/bond0"))
	s.iface("eth2", aq0, map[string]string{"operstate": "down", "carrier": "0", "speed": "0", "duplex": "unknown", "mtu": "1500"})
	// Admin-down interface: carrier/speed/duplex cannot be read.
	s.iface("eth3", aq1, map[string]string{"operstate": "down", "mtu": "1500"})
	s.iface("eth4", vf, up("100000", nil))                                              // SR-IOV VF
	s.iface("pf0vf0", cx0, up("100000", map[string]string{"phys_port_name": "pf0vf0"})) // representor
	s.iface("eth5", mlx4, up("40000", map[string]string{"dev_port": "0"}))
	s.iface("eth6", mlx4, up("-1", map[string]string{"dev_port": "1", "carrier": "0", "operstate": "down"}))
	s.iface("usb0", usb, up("1000", nil))
	s.iface("veth1234", "", up("10000", nil))
	s.iface("broken", filepath.Join(s.root, "devices/missing"), nil)

	s.hwmon("hwmon14", cx0, map[string]string{
		"name": "mlx5", "temp1_input": "88000", "temp1_label": "asic", "temp1_highest": "90000", "temp1_crit": "105000",
		"temp2_input": "50375", "temp2_label": "Module0", "temp2_highest": "55875", "temp2_crit": "80000",
	})
	s.hwmon("hwmon15", cx1, map[string]string{
		"name": "mlx5", "temp1_input": "89000", "temp1_label": "asic", "temp1_highest": "90000", "temp1_crit": "105000",
		"temp2_input": "52125", "temp2_label": "Module1", "temp2_crit": "80000",
	})
	s.hwmon("hwmon11", aq0, map[string]string{
		"name": "eth2", "temp1_input": "42000", "temp1_label": "PHY Temperature", "temp2_input": "41000", "temp2_label": "MAC Temperature",
	})
	s.hwmon("hwmon12", aq1, map[string]string{
		"name": "eth3", "temp1_input": "41000", "temp1_label": "PHY Temperature", "temp2_input": "bogus", "temp2_label": "MAC Temperature",
		"temp3_input": "39000", "temp3_label": "board", "temp3_max": "95000",
	})
	s.hwmon("hwmon20", phy, map[string]string{"name": "phy", "temp1_input": "61500"})
	s.hwmon("hwmon0", nvme, map[string]string{"name": "nvme", "temp1_input": "35850", "temp1_label": "Composite"})
	s.hwmon("hwmon16", "", map[string]string{"name": "nct6798", "temp1_input": "30000"})
	return s
}

const lspciCX6 = `0000:41:00.0 "Ethernet controller" "Mellanox Technologies" "MT2892 Family [ConnectX-6 Dx]" -p00 "Mellanox Technologies" "Device 0016"`
const lspciAQC = `0000:d5:00.0 "Ethernet controller" "Aquantia Corp." "AQtion AQC113CS NBase-T/IEEE 802.3an Ethernet Controller [Antigua 10G]" -r03 -p00 "ASRock Incorporation" "Device d113"`

func readSFFFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "lib", "testdata", "sff", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func newTestNICCollector(t *testing.T, s sysfsTree) (*NICCollector, *int) {
	t.Helper()
	port0 := readSFFFixture(t, "qsfp28_aoc_port0.bin")
	port1 := readSFFFixture(t, "qsfp28_aoc_port1.bin")
	lspciCalls := 0
	c := NewNICCollector(&domain.Context{Hub: domain.NewEventBus(10)})
	c.SysfsRoot = s.root
	c.Stagger = 0
	c.LspciFn = func(addr string) (string, error) {
		lspciCalls++
		switch addr {
		case "0000:41:00.0":
			return lspciCX6 + "\n", nil
		case "0000:d5:00.0", "0000:d6:00.0":
			return lspciAQC, nil
		}
		return "", errors.New("lspci: not found")
	}
	c.FirmwareFn = func(ifname string) (string, error) {
		if ifname == "eth0" {
			return "22.47.3576 (MT_0000000359)\n", nil
		}
		return "", errors.New("not supported")
	}
	c.ReadModuleFn = func(ifname string) (*lib.ModuleEEPROM, error) {
		switch ifname {
		case "eth0":
			return &lib.ModuleEEPROM{Type: lib.ModuleTypeSFF8636, Data: port0}, nil
		case "eth1":
			return &lib.ModuleEEPROM{Type: lib.ModuleTypeSFF8636, Data: port1}, nil
		case "eth5":
			return nil, fmt.Errorf("%w: ETHTOOL_GMODULEINFO: input/output error", lib.ErrNoModule)
		case "eth6":
			return nil, errors.New("ETHTOOL_GMODULEINFO: operation not permitted")
		}
		return nil, lib.ErrModuleUnsupported
	}
	return c, &lspciCalls
}

func TestNICCollectorGather(t *testing.T) {
	s := newNICTree(t)
	c, lspciCalls := newTestNICCollector(t, s)
	list := c.Gather()

	if list.State != dto.NICStateOK {
		t.Errorf("state = %q", list.State)
	}
	ids := make([]string, 0, len(list.Cards))
	for _, card := range list.Cards {
		ids = append(ids, card.ID)
	}
	if want := []string{"0000:03:00", "0000:41:00", "0000:d5:00", "0000:d6:00"}; !slices.Equal(ids, want) {
		t.Fatalf("cards = %v, want %v", ids, want)
	}

	cx := list.Cards[1]
	if cx.Vendor != "Mellanox Technologies" || cx.Model != "ConnectX-6 Dx" || cx.DeviceName != "MT2892 Family [ConnectX-6 Dx]" ||
		cx.SubsystemName != "Device 0016" || cx.Driver != "mlx5_core" || cx.FirmwareVersion != "22.47.3576 (MT_0000000359)" {
		t.Errorf("card identity: %+v", cx)
	}
	if cx.VendorID != "15b3" || cx.DeviceID != "101d" || cx.SubsystemVendorID != "15b3" || cx.SubsystemDeviceID != "0016" {
		t.Errorf("card IDs: %+v", cx)
	}

	// Card temperatures: one ASIC (highest of both functions), then per-port module sensors.
	if len(cx.Temperatures) != 3 {
		t.Fatalf("temperatures = %+v", cx.Temperatures)
	}
	asic := cx.Temperatures[0]
	if asic.Sensor != dto.NICSensorASIC || asic.Celsius != 89 || *asic.HighestCelsius != 90 || *asic.CritCelsius != 105 ||
		asic.PCIAddress != "0000:41:00.1" || asic.Interface != "" || asic.MaxCelsius != nil {
		t.Errorf("asic = %+v", asic)
	}
	m0, m1 := cx.Temperatures[1], cx.Temperatures[2]
	if m0.Sensor != dto.NICSensorModule || m0.Label != "Module0" || m0.Interface != "eth0" || m0.Celsius != 50.375 {
		t.Errorf("module 0 = %+v", m0)
	}
	if m1.Label != "Module1" || m1.Interface != "eth1" || m1.HighestCelsius != nil {
		t.Errorf("module 1 = %+v", m1)
	}

	if len(cx.Ports) != 2 {
		t.Fatalf("ports = %+v", cx.Ports)
	}
	p0 := cx.Ports[0]
	if p0.ID != "0000:41:00.0" || p0.Interface != "eth0" || p0.OperState != "up" || !p0.Carrier || *p0.SpeedMbps != 100000 ||
		p0.Duplex != "full" || p0.MTU != 1500 || p0.Master != "bond0" || p0.DevPort != 0 {
		t.Errorf("port 0 = %+v", p0)
	}
	if p0.Module == nil || p0.Module.VendorPN != "QSFP-100G-AOC-3M" || len(p0.Module.Lanes) != 4 {
		t.Errorf("port 0 module = %+v", p0.Module)
	}
	if cx.Ports[1].Module == nil || cx.Ports[1].Interface != "eth1" {
		t.Errorf("port 1 = %+v", cx.Ports[1])
	}

	aq := list.Cards[2]
	if aq.Model != "AQC113CS" || aq.Vendor != "Aquantia Corp." || aq.Driver != "atlantic" || aq.FirmwareVersion != "" {
		t.Errorf("aquantia card: %+v", aq)
	}
	sensors := make([]string, 0, len(aq.Temperatures))
	for _, tt := range aq.Temperatures {
		sensors = append(sensors, tt.Sensor+"="+tt.Label)
	}
	if want := []string{"phy=PHY Temperature", "mac=MAC Temperature"}; !slices.Equal(sensors, want) {
		t.Errorf("aquantia sensors = %v, want %v", sensors, want)
	}
	if aq.Temperatures[0].Interface != "eth2" {
		t.Errorf("phy interface = %q", aq.Temperatures[0].Interface)
	}
	eth2 := aq.Ports[0]
	if eth2.Carrier || eth2.SpeedMbps != nil || eth2.Duplex != "" || eth2.Module != nil || eth2.Master != "" {
		t.Errorf("eth2 = %+v", eth2)
	}

	// Unparsable reading skipped; an extra labelled sensor is "other" with its limit.
	aq2 := list.Cards[3]
	if len(aq2.Temperatures) != 2 || aq2.Temperatures[1].Sensor != dto.NICSensorOther || *aq2.Temperatures[1].MaxCelsius != 95 {
		t.Errorf("second aquantia temperatures = %+v", aq2.Temperatures)
	}
	if eth3 := aq2.Ports[0]; eth3.OperState != "down" || eth3.Carrier || eth3.SpeedMbps != nil {
		t.Errorf("eth3 = %+v", eth3)
	}

	// Two ports on one PCI function; unknown to lspci; PHY hwmon below the function.
	mlx4 := list.Cards[0]
	if mlx4.Vendor != "15b3" || mlx4.Model != "15b3:1003" || mlx4.Driver != "" {
		t.Errorf("mlx4 card: %+v", mlx4)
	}
	if len(mlx4.Ports) != 2 || mlx4.Ports[0].ID != "0000:03:00.0" || mlx4.Ports[1].ID != "0000:03:00.0-1" || mlx4.Ports[1].SpeedMbps != nil {
		t.Errorf("mlx4 ports: %+v", mlx4.Ports)
	}
	if len(mlx4.Temperatures) != 1 || mlx4.Temperatures[0].Sensor != dto.NICSensorASIC || mlx4.Temperatures[0].Label != "temp1" {
		t.Errorf("mlx4 temperatures: %+v", mlx4.Temperatures)
	}

	// Only real errors are reported (eth5's empty cage is not one).
	if len(list.Errors) != 1 || !strings.HasPrefix(list.Errors[0], "eth6: module EEPROM") {
		t.Errorf("errors = %v", list.Errors)
	}

	// lspci is run once per card and cached, failures included.
	if *lspciCalls != 4 {
		t.Errorf("lspci calls = %d, want 4", *lspciCalls)
	}
	c.Gather()
	if *lspciCalls != 4 {
		t.Errorf("lspci calls after second cycle = %d, want 4", *lspciCalls)
	}
}

func TestNICCollectorErrorLogging(t *testing.T) {
	c, _ := newTestNICCollector(t, newNICTree(t))
	c.logErrors([]string{"a"})
	c.logErrors([]string{"a"}) // unchanged: not logged again
	if c.lastErrs != "a" {
		t.Errorf("lastErrs = %q", c.lastErrs)
	}
	c.logErrors(nil)
	if c.lastErrs != "" {
		t.Errorf("lastErrs = %q", c.lastErrs)
	}
}

func TestNICCollectorWithoutSysfs(t *testing.T) {
	c, _ := newTestNICCollector(t, sysfsTree{t: t, root: t.TempDir()})
	list := c.Gather()
	if list.State != dto.NICStateOK || len(list.Cards) != 0 || list.Cards == nil {
		t.Errorf("list = %+v", list)
	}
	if temps := c.discoverTemperatures(map[string]string{"0000:41:00.0": "eth0"}); temps != nil {
		t.Errorf("temperatures = %+v", temps)
	}
}

func TestNICCollectorDefaults(t *testing.T) {
	c := NewNICCollector(&domain.Context{})
	if c.SysfsRoot != "/sys" || c.ReadModuleFn == nil || c.FirmwareFn == nil || c.Now == nil || c.Stagger != nicStartupStagger {
		t.Errorf("defaults: %+v", c)
	}
	// The default lspci runner returns output or an error, never panics.
	_, _ = c.LspciFn("0000:00:00.0")
}

func TestNICCollectorCollectPublishes(t *testing.T) {
	hub := domain.NewEventBus(10)
	ch := hub.Sub(constants.TopicNICUpdate.Name)
	defer hub.Unsub(ch, constants.TopicNICUpdate.Name)

	c, _ := newTestNICCollector(t, newNICTree(t))
	c.appCtx = &domain.Context{Hub: hub}
	c.Collect()
	select {
	case msg := <-ch:
		if list, ok := msg.(*dto.NICList); !ok || len(list.Cards) != 4 {
			t.Errorf("unexpected message %T", msg)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("nothing published")
	}
}

func TestNICCollectorStart(t *testing.T) {
	t.Run("collects, recovers from panics and stops on cancel", func(t *testing.T) {
		hub := domain.NewEventBus(10)
		ch := hub.Sub(constants.TopicNICUpdate.Name)
		defer hub.Unsub(ch, constants.TopicNICUpdate.Name)

		c, _ := newTestNICCollector(t, newNICTree(t))
		c.appCtx = &domain.Context{Hub: hub}
		read := c.ReadModuleFn
		calls := 0
		c.ReadModuleFn = func(ifname string) (*lib.ModuleEEPROM, error) {
			calls++
			if calls == 1 {
				panic("boom")
			}
			return read(ifname)
		}

		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() {
			c.Start(ctx, 20*time.Millisecond)
			close(done)
		}()
		select {
		case <-ch:
		case <-time.After(5 * time.Second):
			t.Fatal("nothing published after a panicking cycle")
		}
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("Start did not return after cancel")
		}
	})

	t.Run("returns during the startup stagger", func(t *testing.T) {
		c := NewNICCollector(&domain.Context{})
		c.Stagger = time.Hour
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		finished := make(chan struct{})
		go func() {
			c.Start(ctx, time.Minute)
			close(finished)
		}()
		select {
		case <-finished:
		case <-time.After(5 * time.Second):
			t.Fatal("Start ignored cancellation during the stagger")
		}
	})
}

func TestParseLspciMachine(t *testing.T) {
	got := parseLspciMachine(lspciAQC)
	want := pciNames{Vendor: "Aquantia Corp.", Device: "AQtion AQC113CS NBase-T/IEEE 802.3an Ethernet Controller [Antigua 10G]",
		SubsystemVendor: "ASRock Incorporation", Subsystem: "Device d113"}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
	if got := parseLspciMachine(`0000:01:00.0 "Ethernet controller" "Intel Corporation"`); got.Vendor != "Intel Corporation" || got.Device != "" {
		t.Errorf("partial: %+v", got)
	}
	if got := parseLspciMachine(`0000:01:00.0 "Ethernet controller" "Intel`); got.Vendor != "" {
		t.Errorf("unterminated: %+v", got)
	}
	if got := parseLspciMachine(""); got != (pciNames{}) {
		t.Errorf("empty: %+v", got)
	}
}

func TestNICModel(t *testing.T) {
	for device, want := range map[string]string{
		"MT2892 Family [ConnectX-6 Dx]":                                          "ConnectX-6 Dx",
		"MT27500 Family [ConnectX-3]":                                            "ConnectX-3",
		"AQtion AQC113CS NBase-T/IEEE 802.3an Ethernet Controller [Antigua 10G]": "AQC113CS",
		"AQC107 NBase-T/IEEE 802.3bz Ethernet Controller [AQtion]":               "AQC107",
		"Ethernet Controller E810-C for QSFP":                                    "E810-C",
		"Ethernet Controller 10-Gigabit X540-AT2":                                "X540-AT2",
		"I350 Gigabit Network Connection":                                        "I350",
		"82576 Gigabit Network Connection":                                       "82576",
		"82599ES 10-Gigabit SFI/SFP+ Network Connection":                         "82599ES",
		"Ethernet Connection (17) I219-LM":                                       "I219-LM",
		"BCM57414 NetXtreme-E 10Gb/25Gb RDMA Ethernet Controller":                "BCM57414",
		"NetXtreme BCM5720 Gigabit Ethernet PCIe":                                "BCM5720",
		"RTL8125 2.5GbE Controller":                                              "RTL8125",
		"SFC9220 10/40G Ethernet Controller":                                     "SFC9220",
		"10G Ethernet Adapter":                                                   "10G Ethernet Adapter",
		"Device 101d":                                                            "Device 101d",
		"Ethernet Controller [Something]":                                        "Ethernet Controller",
		"":                                                                       "",
	} {
		if got := nicModel(device); got != want {
			t.Errorf("nicModel(%q) = %q, want %q", device, got, want)
		}
	}
}

func TestClassifyNICSensor(t *testing.T) {
	for _, tc := range []struct {
		label string
		sole  bool
		want  string
	}{
		{"asic", false, dto.NICSensorASIC},
		{"", true, dto.NICSensorASIC},
		{"", false, dto.NICSensorOther},
		{"PHY Temperature", false, dto.NICSensorPHY},
		{"MAC Temperature", false, dto.NICSensorMAC},
		{"Module0", false, dto.NICSensorModule},
		{"module 12", false, dto.NICSensorModule},
		{"loc1", false, dto.NICSensorOther},
	} {
		if got := classifyNICSensor(tc.label, tc.sole); got != tc.want {
			t.Errorf("classifyNICSensor(%q, %v) = %q, want %q", tc.label, tc.sole, got, tc.want)
		}
	}
}

func TestOwningPCIFunction(t *testing.T) {
	s := sysfsTree{t: t, root: t.TempDir()}
	dev := s.mkdir("devices/pci0000:00/0000:00:02.0/0000:03:00.0/mdio_bus/phy0")
	s.link("a", dev)
	s.link("b", s.mkdir("devices/platform/nct6775.672"))
	if got := owningPCIFunction(filepath.Join(s.root, "a")); got != "0000:03:00.0" {
		t.Errorf("phy = %q", got)
	}
	if got := owningPCIFunction(filepath.Join(s.root, "b")); got != "" {
		t.Errorf("platform = %q", got)
	}
	if got := owningPCIFunction(filepath.Join(s.root, "missing")); got != "" {
		t.Errorf("missing = %q", got)
	}
}

func TestCardTemperaturesModuleSensorsPerFunction(t *testing.T) {
	ports := []nicPortInfo{
		{port: dto.NICPort{PCIAddress: "0000:05:00.0"}},
		{port: dto.NICPort{PCIAddress: "0000:05:00.1"}},
	}
	crit := 90.0
	temps := []dto.NICTemperature{
		{Sensor: dto.NICSensorModule, Label: "Module0", PCIAddress: "0000:05:00.1", Celsius: 40},
		{Sensor: dto.NICSensorModule, Label: "Module0", PCIAddress: "0000:05:00.0", Celsius: 41},
		// Independent per-port PHYs with the same label are both kept.
		{Sensor: dto.NICSensorPHY, Label: "PHY Temperature", PCIAddress: "0000:05:00.1", Celsius: 50},
		{Sensor: dto.NICSensorPHY, Label: "PHY Temperature", PCIAddress: "0000:05:00.0", Celsius: 60, CritCelsius: &crit},
		{Sensor: dto.NICSensorASIC, Label: "temp1", PCIAddress: "0000:06:00.0", Celsius: 70}, // another card
	}
	got := cardTemperatures(temps, ports)
	if len(got) != 4 {
		t.Fatalf("got %+v", got)
	}
	if got[0].Sensor != dto.NICSensorPHY || got[0].PCIAddress != "0000:05:00.0" || got[0].CritCelsius == nil || got[1].PCIAddress != "0000:05:00.1" {
		t.Errorf("phy sensors = %+v", got[:2])
	}
	if got[2].PCIAddress != "0000:05:00.0" || got[3].PCIAddress != "0000:05:00.1" {
		t.Errorf("module sensors = %+v", got[2:])
	}
}
