package collectors

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/tinkerologie/unraid-management-agent/daemon/constants"
	"github.com/tinkerologie/unraid-management-agent/daemon/domain"
	"github.com/tinkerologie/unraid-management-agent/daemon/dto"
	"github.com/tinkerologie/unraid-management-agent/daemon/lib"
	"github.com/tinkerologie/unraid-management-agent/daemon/logger"
)

const (
	// nicStartupStagger delays the first collection so it does not pile onto boot.
	nicStartupStagger = 10 * time.Second
	// lspciTimeout bounds one lspci call.
	lspciTimeout = 10 * time.Second
)

var (
	// pciAddressRe matches a PCI function address (domain:bus:device.function).
	pciAddressRe = regexp.MustCompile(`^[0-9a-f]{4}:[0-9a-f]{2}:[0-9a-f]{2}\.[0-7]$`)
	// representorPortRe matches the phys_port_name of switchdev VF/SF representors
	// ("pf0vf1", "c1pf0sf2"), which share the PF's PCI device but are not ports.
	representorPortRe = regexp.MustCompile(`pf\d+(vf|sf)\d+`)
	// hwmonModuleRe matches transceiver cage sensors such as mlx5's "Module0".
	hwmonModuleRe = regexp.MustCompile(`(?i)^module\s*\d+$`)
	// modelSkipRe matches tokens of a PCI device name that are not a model
	// number: link speeds ("10G", "2.5GbE", "10/40G", "10-Gigabit") and
	// parenthesized revisions ("(17)").
	modelSkipRe = regexp.MustCompile(`(?i)^\(|gigabit|^[0-9.]+(g|gb|gbe|gbit|mb|mbit)(/.*)?$|^[0-9.]+/`)
)

// pciNames holds the lspci names of a PCI device.
type pciNames struct {
	Vendor, Device, SubsystemVendor, Subsystem string
}

// NICCollector periodically reports physical network cards: the PCI functions
// behind /sys/class/net interfaces grouped per PCI slot, their hwmon
// temperatures, port link state and the diagnostics (DDM) of plugged optics.
// Interfaces without a PCI device (bridges, bonds, VLANs, veth, tunnels),
// SR-IOV virtual functions and switchdev representors are skipped.
type NICCollector struct {
	appCtx *domain.Context
	// SysfsRoot is the sysfs mount point ("/sys"); replaceable for tests.
	SysfsRoot string
	// ReadModuleFn reads a port's module EEPROM; replaceable for tests.
	ReadModuleFn func(ifname string) (*lib.ModuleEEPROM, error)
	// FirmwareFn returns a port's firmware version; replaceable for tests.
	FirmwareFn func(ifname string) (string, error)
	// LspciFn returns `lspci -Dmm -s <addr>` output; replaceable for tests.
	LspciFn func(addr string) (string, error)
	// Now returns the current time; replaceable for tests.
	Now func() time.Time
	// Stagger delays the first collection after Start.
	Stagger time.Duration

	// names caches lspci names per PCI function (they never change at runtime).
	names    map[string]pciNames
	lastErrs string
}

// NewNICCollector creates a collector with production defaults.
func NewNICCollector(ctx *domain.Context) *NICCollector {
	return &NICCollector{
		appCtx:       ctx,
		SysfsRoot:    "/sys",
		ReadModuleFn: lib.ReadModuleEEPROM,
		FirmwareFn:   lib.ReadFirmwareVersion,
		LspciFn: func(addr string) (string, error) {
			return lib.ExecCommandOutputWithTimeout(lspciTimeout, "lspci", "-Dmm", "-s", addr)
		},
		Now:     time.Now,
		Stagger: nicStartupStagger,
		names:   map[string]pciNames{},
	}
}

// Start runs the collector until ctx is cancelled.
func (c *NICCollector) Start(ctx context.Context, interval time.Duration) {
	logger.Info("Starting NIC collector (interval: %v)", interval)

	select {
	case <-ctx.Done():
		return
	case <-time.After(c.Stagger):
	}

	run := func() {
		defer func() {
			if r := recover(); r != nil {
				logger.LogPanicWithStack("NIC collector", r)
			}
		}()
		collectWithWatchdog(ctx, "NIC", interval, c.Collect)
	}
	run()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			logger.Info("NIC collector stopping due to context cancellation")
			return
		case <-ticker.C:
			run()
		}
	}
}

// Collect gathers the network cards and publishes them.
func (c *NICCollector) Collect() {
	list := c.Gather()
	domain.Publish(c.appCtx.Hub, constants.TopicNICUpdate, list)
	logger.Debug("NIC: published (cards=%d, errors=%d, %dms)", len(list.Cards), len(list.Errors),
		list.CollectionDurationMs)
}

// nicPortInfo is a discovered port plus the sysfs paths needed to finish it.
type nicPortInfo struct {
	port   dto.NICPort
	devDir string // resolved PCI function directory
}

// Gather collects the current network cards.
func (c *NICCollector) Gather() *dto.NICList {
	start := c.Now()
	list := &dto.NICList{State: dto.NICStateOK, Cards: []dto.NICCard{}, Timestamp: start}
	var errs []string

	ports := c.discoverPorts()
	byCard := map[string][]nicPortInfo{}
	for _, p := range ports {
		slot := pciSlot(p.port.PCIAddress)
		byCard[slot] = append(byCard[slot], p)
	}
	functions := map[string]string{} // PCI function -> first interface
	for _, p := range ports {
		if _, ok := functions[p.port.PCIAddress]; !ok {
			functions[p.port.PCIAddress] = p.port.Interface
		}
	}
	temps := c.discoverTemperatures(functions)

	for slot, cardPorts := range byCard {
		sort.Slice(cardPorts, func(i, j int) bool {
			if cardPorts[i].port.PCIAddress != cardPorts[j].port.PCIAddress {
				return cardPorts[i].port.PCIAddress < cardPorts[j].port.PCIAddress
			}
			return cardPorts[i].port.DevPort < cardPorts[j].port.DevPort
		})
		card := c.buildCard(slot, cardPorts[0])
		for _, p := range cardPorts {
			port := p.port
			port.Module, errs = c.readModule(port.Interface, errs)
			card.Ports = append(card.Ports, port)
		}
		card.Temperatures = cardTemperatures(temps, cardPorts)
		list.Cards = append(list.Cards, card)
	}
	sort.Slice(list.Cards, func(i, j int) bool { return list.Cards[i].ID < list.Cards[j].ID })

	list.Errors = errs
	list.CollectionDurationMs = c.Now().Sub(start).Milliseconds()
	c.logErrors(errs)
	return list
}

// discoverPorts returns every interface backed by a PCI physical function.
func (c *NICCollector) discoverPorts() []nicPortInfo {
	netDir := filepath.Join(c.SysfsRoot, "class", "net")
	entries, err := os.ReadDir(netDir)
	if err != nil {
		logger.Debug("NIC: cannot read %s: %v", netDir, err)
		return nil
	}
	var ports []nicPortInfo
	for _, e := range entries {
		ifDir := filepath.Join(netDir, e.Name())
		devDir, err := filepath.EvalSymlinks(filepath.Join(ifDir, "device"))
		if err != nil || !pciAddressRe.MatchString(filepath.Base(devDir)) {
			continue // virtual interface, or not a PCI device (e.g. USB)
		}
		if _, err := os.Stat(filepath.Join(devDir, "physfn")); err == nil {
			continue // SR-IOV virtual function
		}
		if representorPortRe.MatchString(readTrimmed(filepath.Join(ifDir, "phys_port_name"))) {
			continue
		}
		addr := filepath.Base(devDir)
		devPort, _ := strconv.Atoi(readTrimmed(filepath.Join(ifDir, "dev_port")))
		port := dto.NICPort{
			ID:         addr,
			Interface:  e.Name(),
			PCIAddress: addr,
			DevPort:    devPort,
			OperState:  readTrimmed(filepath.Join(ifDir, "operstate")),
			Carrier:    readTrimmed(filepath.Join(ifDir, "carrier")) == "1",
		}
		if devPort > 0 {
			port.ID = fmt.Sprintf("%s-%d", addr, devPort)
		}
		if speed, err := strconv.Atoi(readTrimmed(filepath.Join(ifDir, "speed"))); err == nil && speed > 0 {
			port.SpeedMbps = &speed
		}
		if duplex := readTrimmed(filepath.Join(ifDir, "duplex")); duplex == "full" || duplex == "half" {
			port.Duplex = duplex
		}
		port.MTU, _ = strconv.Atoi(readTrimmed(filepath.Join(ifDir, "mtu")))
		if master, err := filepath.EvalSymlinks(filepath.Join(ifDir, "master")); err == nil {
			port.Master = filepath.Base(master)
		}
		ports = append(ports, nicPortInfo{port: port, devDir: devDir})
	}
	return ports
}

// buildCard fills in a card's identity from its first PCI function.
func (c *NICCollector) buildCard(slot string, first nicPortInfo) dto.NICCard {
	dev := first.devDir
	card := dto.NICCard{
		ID:                slot,
		VendorID:          pciID(filepath.Join(dev, "vendor")),
		DeviceID:          pciID(filepath.Join(dev, "device")),
		SubsystemVendorID: pciID(filepath.Join(dev, "subsystem_vendor")),
		SubsystemDeviceID: pciID(filepath.Join(dev, "subsystem_device")),
		Temperatures:      []dto.NICTemperature{},
		Ports:             []dto.NICPort{},
	}
	if driver, err := filepath.EvalSymlinks(filepath.Join(dev, "driver")); err == nil {
		card.Driver = filepath.Base(driver)
	}
	names := c.pciNames(first.port.PCIAddress)
	card.Vendor = names.Vendor
	if card.Vendor == "" {
		card.Vendor = card.VendorID
	}
	card.DeviceName = names.Device
	card.SubsystemName = names.Subsystem
	card.Model = nicModel(names.Device)
	if card.Model == "" && card.DeviceID != "" {
		card.Model = card.VendorID + ":" + card.DeviceID
	}
	if fw, err := c.FirmwareFn(first.port.Interface); err == nil {
		card.FirmwareVersion = strings.TrimSpace(fw)
	} else {
		logger.Debug("NIC: %s: firmware version: %v", first.port.Interface, err)
	}
	return card
}

// pciNames returns the cached lspci names of a PCI function.
func (c *NICCollector) pciNames(addr string) pciNames {
	if names, ok := c.names[addr]; ok {
		return names
	}
	out, err := c.LspciFn(addr)
	var names pciNames
	if err != nil {
		logger.Debug("NIC: lspci %s: %v", addr, err)
	} else {
		names = parseLspciMachine(out)
	}
	c.names[addr] = names
	return names
}

// parseLspciMachine parses one line of `lspci -Dmm` output:
// slot "class" "vendor" "device" [-rXX] [-pXX] "subsystem vendor" "subsystem".
func parseLspciMachine(out string) pciNames {
	line, _, _ := strings.Cut(strings.TrimSpace(out), "\n")
	var fields []string
	for rest := line; ; {
		start := strings.IndexByte(rest, '"')
		if start < 0 {
			break
		}
		end := strings.IndexByte(rest[start+1:], '"')
		if end < 0 {
			break
		}
		fields = append(fields, rest[start+1:start+1+end])
		rest = rest[start+end+2:]
	}
	var names pciNames
	for i, dst := range []*string{nil, &names.Vendor, &names.Device, &names.SubsystemVendor, &names.Subsystem} {
		if dst != nil && i < len(fields) {
			*dst = fields[i]
		}
	}
	return names
}

// nicModel derives a short product name from a PCI device name:
// "MT2892 Family [ConnectX-6 Dx]" -> "ConnectX-6 Dx",
// "AQtion AQC113CS NBase-T/IEEE 802.3an Ethernet Controller [Antigua 10G]" -> "AQC113CS",
// "Ethernet Controller E810-C for QSFP" -> "E810-C". Names without a model
// number are returned without their bracketed suffix.
func nicModel(device string) string {
	base, bracket, hasBracket := strings.Cut(device, " [")
	base = strings.TrimSpace(base)
	bracket = strings.TrimSuffix(strings.TrimSpace(bracket), "]")
	if hasBracket && strings.Contains(base, "Family") && bracket != "" {
		return bracket
	}
	if strings.HasPrefix(base, "Device ") {
		return base
	}
	for _, token := range strings.Fields(base) {
		if strings.ContainsAny(token, "0123456789") && !modelSkipRe.MatchString(token) {
			return token
		}
	}
	return base
}

// discoverTemperatures reads the hwmon chips that belong to the given PCI
// functions (mapped to their first interface).
func (c *NICCollector) discoverTemperatures(functions map[string]string) []dto.NICTemperature {
	hwmonDir := filepath.Join(c.SysfsRoot, "class", "hwmon")
	entries, err := os.ReadDir(hwmonDir)
	if err != nil {
		logger.Debug("NIC: cannot read %s: %v", hwmonDir, err)
		return nil
	}
	var temps []dto.NICTemperature
	for _, e := range entries {
		chip := filepath.Join(hwmonDir, e.Name())
		addr := owningPCIFunction(filepath.Join(chip, "device"))
		ifname, ok := functions[addr]
		if !ok {
			continue
		}
		inputs, _ := filepath.Glob(filepath.Join(chip, "temp*_input"))
		sort.Strings(inputs)
		for _, input := range inputs {
			raw, err := strconv.ParseFloat(readTrimmed(input), 64)
			if err != nil {
				continue
			}
			prefix := strings.TrimSuffix(input, "_input")
			label := readTrimmed(prefix + "_label")
			t := dto.NICTemperature{
				Label:          label,
				PCIAddress:     addr,
				Celsius:        raw / 1000,
				HighestCelsius: readMilliCelsius(prefix + "_highest"),
				MaxCelsius:     readMilliCelsius(prefix + "_max"),
				CritCelsius:    readMilliCelsius(prefix + "_crit"),
			}
			if label == "" {
				t.Label = filepath.Base(prefix)
			}
			t.Sensor = classifyNICSensor(label, len(inputs) == 1)
			if t.Sensor != dto.NICSensorASIC {
				t.Interface = ifname // per-port sensor: name its port
			}
			temps = append(temps, t)
		}
	}
	return temps
}

// classifyNICSensor maps a hwmon label to a sensor kind. The only, unlabeled
// sensor of a NIC's hwmon chip (ice, bnxt_en) is the controller die.
func classifyNICSensor(label string, sole bool) string {
	lower := strings.ToLower(label)
	switch {
	case label == "" && sole:
		return dto.NICSensorASIC
	case strings.Contains(lower, "asic"):
		return dto.NICSensorASIC
	case strings.Contains(lower, "phy"):
		return dto.NICSensorPHY
	case strings.Contains(lower, "mac"):
		return dto.NICSensorMAC
	case hwmonModuleRe.MatchString(label):
		return dto.NICSensorModule
	}
	return dto.NICSensorOther
}

// nicSensorOrder sorts card temperatures: ASIC, PHY, MAC, other, module.
var nicSensorOrder = map[string]int{
	dto.NICSensorASIC: 0, dto.NICSensorPHY: 1, dto.NICSensorMAC: 2, dto.NICSensorOther: 3, dto.NICSensorModule: 4,
}

// cardTemperatures returns the temperatures of a card's functions. The ASIC
// sensor every function of a multi-function card exposes is the one shared
// die (functions in one PCI slot are one chip), so it is reported once, with
// the highest reading. Every other sensor (PHY, MAC, module, ...) can be per
// port, so it is kept per PCI function even when labels repeat.
func cardTemperatures(temps []dto.NICTemperature, ports []nicPortInfo) []dto.NICTemperature {
	inCard := map[string]bool{}
	for _, p := range ports {
		inCard[p.port.PCIAddress] = true
	}
	out := []dto.NICTemperature{}
	index := map[string]int{}
	for _, t := range temps {
		if !inCard[t.PCIAddress] {
			continue
		}
		if t.Sensor != dto.NICSensorASIC {
			out = append(out, t)
			continue
		}
		if i, ok := index[t.Label]; ok {
			if t.Celsius > out[i].Celsius {
				out[i] = t
			}
			continue
		}
		index[t.Label] = len(out)
		out = append(out, t)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if nicSensorOrder[out[i].Sensor] != nicSensorOrder[out[j].Sensor] {
			return nicSensorOrder[out[i].Sensor] < nicSensorOrder[out[j].Sensor]
		}
		if out[i].Label != out[j].Label {
			return out[i].Label < out[j].Label
		}
		return out[i].PCIAddress < out[j].PCIAddress
	})
	return out
}

// readModule reads and decodes a port's module EEPROM. Ports whose driver
// has no module support, and empty cages, have no module.
func (c *NICCollector) readModule(ifname string, errs []string) (*dto.NICModule, []string) {
	eeprom, err := c.ReadModuleFn(ifname)
	switch {
	case err == nil:
		return lib.DecodeModuleEEPROM(eeprom.Type, eeprom.Data), errs
	case errors.Is(err, lib.ErrModuleUnsupported), errors.Is(err, lib.ErrNoModule):
		logger.Debug("NIC: %s: no module: %v", ifname, err)
		return nil, errs
	}
	return nil, append(errs, fmt.Sprintf("%s: module EEPROM: %v", ifname, err))
}

// logErrors logs collection errors once per distinct set.
func (c *NICCollector) logErrors(errs []string) {
	sig := strings.Join(errs, "\n")
	if sig == c.lastErrs {
		return
	}
	c.lastErrs = sig
	if len(errs) == 0 {
		logger.Info("NIC: collection is healthy again")
		return
	}
	logger.Warning("NIC: %d collection problem(s): %s", len(errs), strings.Join(errs, "; "))
}

// owningPCIFunction resolves a sysfs device link and returns the nearest PCI
// function at or above it (hwmon chips of PHYs hang below the NIC), or "".
func owningPCIFunction(link string) string {
	dir, err := filepath.EvalSymlinks(link)
	if err != nil {
		return ""
	}
	for ; dir != "/" && dir != "."; dir = filepath.Dir(dir) {
		if pciAddressRe.MatchString(filepath.Base(dir)) {
			return filepath.Base(dir)
		}
	}
	return ""
}

// pciSlot strips the function from a PCI address ("0000:41:00.1" -> "0000:41:00").
func pciSlot(addr string) string {
	slot, _, _ := strings.Cut(addr, ".")
	return slot
}

// pciID reads a sysfs PCI ID file ("0x15b3") as bare hex ("15b3").
func pciID(path string) string {
	return strings.TrimPrefix(readTrimmed(path), "0x")
}

// readMilliCelsius reads an optional hwmon millidegree file.
func readMilliCelsius(path string) *float64 {
	v, err := strconv.ParseFloat(readTrimmed(path), 64)
	if err != nil {
		return nil
	}
	c := v / 1000
	return &c
}
