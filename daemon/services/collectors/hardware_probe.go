package collectors

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/tinkerologie/unraid-management-agent/daemon/constants"
	"github.com/tinkerologie/unraid-management-agent/daemon/dto"
	"github.com/tinkerologie/unraid-management-agent/daemon/lib"
)

// Hardware capability names reported in the capability snapshot (#261).
const (
	CapabilityPSU             = "hardware.psu"
	CapabilityNICOptics       = "hardware.nic_optics"
	CapabilityStorageTopology = "hardware.storage_topology"
	CapabilityGPU             = "hardware.gpu"
	CapabilityUPS             = "hardware.ups"
	CapabilityNUT             = "hardware.nut"
	CapabilityZFS             = "hardware.zfs"
	CapabilityFanControl      = "hardware.fan_control"
)

// HardwareClass maps a hardware capability to the collector it gates.
type HardwareClass struct {
	Capability string
	// Collector is the gated collector, or "" when the class gates none.
	Collector string
}

// HardwareClasses is the single list of hardware classes and the collectors
// they gate. A gated collector in automatic mode runs only while its class is
// present. Fan control is reported only: it has no plugin interval setting.
var HardwareClasses = []HardwareClass{
	{Capability: CapabilityPSU, Collector: "psu"},
	{Capability: CapabilityNICOptics, Collector: "nic"},
	{Capability: CapabilityStorageTopology, Collector: "storage_topology"},
	{Capability: CapabilityGPU, Collector: "gpu"},
	{Capability: CapabilityUPS, Collector: "ups"},
	{Capability: CapabilityNUT, Collector: "nut"},
	{Capability: CapabilityZFS, Collector: "zfs"},
	{Capability: CapabilityFanControl},
}

// HardwareClassForCollector returns the hardware class gating a collector.
func HardwareClassForCollector(collector string) (HardwareClass, bool) {
	for _, c := range HardwareClasses {
		if c.Collector != "" && c.Collector == collector {
			return c, true
		}
	}
	return HardwareClass{}, false
}

// defaultMaxModulePorts caps the module EEPROM reads of one NIC check.
const defaultMaxModulePorts = 16

var (
	drmCardRe = regexp.MustCompile(`^card\d+$`)
	pwmRe     = regexp.MustCompile(`^pwm\d+$`)
)

// gpuDrivers are the kernel drivers of GPUs the gpu collector reports. Other
// DRM drivers (e.g. ast or mgag200 BMC display adapters) do not count.
var gpuDrivers = map[string]bool{"i915": true, "xe": true, "amdgpu": true, "nvidia": true}

// pluggableModuleIDs are the SFF-8024 identifiers of SFP and QSFP class modules.
var pluggableModuleIDs = map[byte]bool{
	0x03: true, 0x0C: true, 0x0D: true, 0x11: true, 0x18: true, 0x19: true, 0x1A: true, 0x1B: true, 0x1E: true,
}

// HardwareProbe runs the read-only hardware capability checks. Checks read
// sysfs, procfs and config files only: they run no commands, never touch
// disks, and the NIC check's only device access is a bounded number of
// module EEPROM ioctls when no NIC temperature sensor is found.
type HardwareProbe struct {
	// Root is the filesystem root ("/"); tests point it at a fixture tree.
	Root string
	// ReadModuleFn reads a port's module EEPROM; replaceable for tests.
	ReadModuleFn func(ifname string) (*lib.ModuleEEPROM, error)
	// MaxModulePorts caps the module EEPROM reads of one NIC check.
	MaxModulePorts int
}

// NewHardwareProbe creates a probe with production defaults.
func NewHardwareProbe() *HardwareProbe {
	return &HardwareProbe{Root: "/", ReadModuleFn: lib.ReadModuleEEPROM, MaxModulePorts: defaultMaxModulePorts}
}

// CheckFor returns the check of a hardware capability, or nil for an unknown name.
func (p *HardwareProbe) CheckFor(capability string) func(context.Context) (dto.CapabilityState, string) {
	switch capability {
	case CapabilityPSU:
		return p.CheckPSU
	case CapabilityNICOptics:
		return p.CheckNICOptics
	case CapabilityStorageTopology:
		return p.CheckStorageTopology
	case CapabilityGPU:
		return p.CheckGPU
	case CapabilityUPS:
		return p.CheckUPS
	case CapabilityNUT:
		return p.CheckNUT
	case CapabilityZFS:
		return p.CheckZFS
	case CapabilityFanControl:
		return p.CheckFanControl
	}
	return nil
}

// CheckPSU detects an exact supported USB PSU independently of driver readiness.
// A missing kernel driver must not hide a recognized device from diagnostics.
func (p *HardwareProbe) CheckPSU(ctx context.Context) (dto.CapabilityState, string) {
	sources, errs := discoverPSUs(ctx, p.path("/sys"))
	if len(sources) > 0 {
		return dto.CapabilityPresent, "recognized Corsair RMi/HXi USB PSU"
	}
	if len(errs) > 0 {
		return dto.CapabilityUnknown, strings.Join(errs, "; ")
	}
	return dto.CapabilityAbsent, "no recognized host-visible PSU telemetry interface"
}

// path maps an absolute system path below Root.
func (p *HardwareProbe) path(abs string) string {
	return filepath.Join(p.Root, abs)
}

// readDir lists a directory. A missing directory is not an error.
func readDir(dir string) ([]os.DirEntry, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	return entries, err
}

// exists reports whether path exists. Errors other than "not found" are returned.
func exists(path string) (bool, error) {
	_, err := os.Stat(path)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, fs.ErrNotExist):
		return false, nil
	}
	return false, err
}

// CheckNICOptics reports whether a wired PCI network card has a NIC-owned
// hwmon temperature sensor or a plugged SFP/QSFP module. Plain onboard NICs
// (e.g. Intel I219-V, I211) and Wi-Fi cards (e.g. Intel 3168NGW) do not count.
// An empty cage on a card without sensors is not detected until a module is
// inserted: sysfs does not expose cages.
func (p *HardwareProbe) CheckNICOptics(ctx context.Context) (dto.CapabilityState, string) {
	sys := p.path("/sys")
	if _, err := readDir(filepath.Join(sys, "class", "net")); err != nil {
		return dto.CapabilityUnknown, fmt.Sprintf("cannot read network interfaces: %v", err)
	}
	nc := &NICCollector{SysfsRoot: sys}
	var ports []nicPortInfo
	for _, port := range nc.discoverPorts() {
		if isWireless(sys, port.port.Interface) {
			continue
		}
		ports = append(ports, port)
	}
	if len(ports) == 0 {
		return dto.CapabilityAbsent, "no wired PCI network card"
	}
	sort.Slice(ports, func(i, j int) bool { return ports[i].port.Interface < ports[j].port.Interface })

	functions := map[string]string{}
	for _, port := range ports {
		if _, ok := functions[port.port.PCIAddress]; !ok {
			functions[port.port.PCIAddress] = port.port.Interface
		}
	}
	if temps := nc.discoverTemperatures(functions); len(temps) > 0 {
		return dto.CapabilityPresent, fmt.Sprintf("NIC temperature sensor on %s", temps[0].PCIAddress)
	}

	maxPorts := p.MaxModulePorts
	if maxPorts <= 0 {
		maxPorts = defaultMaxModulePorts
	}
	var errs []string
	for i, port := range ports {
		if i >= maxPorts {
			break
		}
		if err := ctx.Err(); err != nil {
			return dto.CapabilityUnknown, fmt.Sprintf("check interrupted: %v", err)
		}
		ifname := port.port.Interface
		eeprom, err := p.ReadModuleFn(ifname)
		switch {
		case err == nil:
			if eeprom != nil && len(eeprom.Data) > 0 && pluggableModuleIDs[eeprom.Data[0]] {
				id := lib.DecodeModuleEEPROM(eeprom.Type, eeprom.Data).Identifier
				return dto.CapabilityPresent, fmt.Sprintf("%s module on %s (%s)", id, ifname, port.port.PCIAddress)
			}
		case errors.Is(err, lib.ErrNoModule), errors.Is(err, lib.ErrModuleUnsupported):
		default:
			errs = append(errs, fmt.Sprintf("%s: %v", ifname, err))
		}
	}
	if len(errs) > 0 {
		return dto.CapabilityUnknown, "module EEPROM read failed: " + strings.Join(errs, "; ")
	}
	return dto.CapabilityAbsent, "no NIC with optics or temperature sensors"
}

// isWireless reports whether a network interface is a Wi-Fi device.
func isWireless(sys, ifname string) bool {
	for _, marker := range []string{"wireless", "phy80211"} {
		if ok, _ := exists(filepath.Join(sys, "class", "net", ifname, marker)); ok {
			return true
		}
	}
	return false
}

// CheckStorageTopology reports whether a SAS HBA, a Broadcom/LSI RAID
// controller or an SES enclosure exists. It reads sysfs only: it neither runs
// storcli nor sends SCSI commands. Tool availability is noted in the detail.
func (p *HardwareProbe) CheckStorageTopology(_ context.Context) (dto.CapabilityState, string) {
	sys := p.path("/sys")
	sc := NewStorageTopologyCollector(nil)
	sc.SysfsRoot = sys
	tools := fmt.Sprintf("storcli %s, sg_ses %s", foundWord(p.anyExecutable(sc.StorcliCandidates)), foundWord(p.anyExecutable(sc.SgSesCandidates)))

	var readErr error
	enclosures, err := readDir(filepath.Join(sys, "class", "enclosure"))
	if err != nil {
		readErr = err
	}
	if len(enclosures) > 0 {
		return dto.CapabilityPresent, fmt.Sprintf("SES enclosure %s; %s", enclosures[0].Name(), tools)
	}
	if devs := sc.discoverSESDevices(); len(devs) > 0 {
		d := devs[0]
		return dto.CapabilityPresent, fmt.Sprintf("SES device %s (%s %s); %s", d.name, d.vendor, d.product, tools)
	}
	devices, err := readDir(filepath.Join(sys, "bus", "pci", "devices"))
	if err != nil {
		readErr = err
	}
	for _, e := range devices {
		dir := filepath.Join(sys, "bus", "pci", "devices", e.Name())
		class := readTrimmed(filepath.Join(dir, "class"))
		switch {
		case strings.HasPrefix(class, "0x0107"):
			return dto.CapabilityPresent, fmt.Sprintf("SAS controller at %s; %s", e.Name(), tools)
		case strings.HasPrefix(class, "0x0104") && readTrimmed(filepath.Join(dir, "vendor")) == "0x1000":
			return dto.CapabilityPresent, fmt.Sprintf("Broadcom/LSI RAID controller at %s; %s", e.Name(), tools)
		}
	}
	if readErr != nil {
		return dto.CapabilityUnknown, fmt.Sprintf("cannot read storage controllers: %v", readErr)
	}
	return dto.CapabilityAbsent, "no SAS HBA, RAID controller or SES enclosure; " + tools
}

func (p *HardwareProbe) anyExecutable(candidates []string) bool {
	for _, c := range candidates {
		if isExecutableFile(p.path(c)) {
			return true
		}
	}
	return false
}

// foundWord renders a tool lookup for a detail string.
func foundWord(found bool) string {
	if found {
		return "found"
	}
	return "not found"
}

// CheckGPU reports whether a GPU driven by i915, xe, amdgpu or nvidia exists.
// GPUs bound to vfio-pci (passed through to a VM) do not count.
func (p *HardwareProbe) CheckGPU(_ context.Context) (dto.CapabilityState, string) {
	drm := p.path("/sys/class/drm")
	cards, readErr := readDir(drm)
	for _, e := range cards {
		if !drmCardRe.MatchString(e.Name()) {
			continue // connectors such as card0-HDMI-A-1
		}
		driver, err := filepath.EvalSymlinks(filepath.Join(drm, e.Name(), "device", "driver"))
		if err != nil {
			continue
		}
		if name := filepath.Base(driver); gpuDrivers[name] {
			return dto.CapabilityPresent, fmt.Sprintf("GPU %s (%s)", e.Name(), name)
		}
	}
	if matches, _ := filepath.Glob(p.path("/dev/nvidia[0-9]*")); len(matches) > 0 {
		return dto.CapabilityPresent, "NVIDIA device " + filepath.Base(matches[0])
	}
	if readErr != nil {
		return dto.CapabilityUnknown, fmt.Sprintf("cannot read GPUs: %v", readErr)
	}
	return dto.CapabilityAbsent, "no Intel, AMD or NVIDIA GPU"
}

// CheckUPS reports whether apcupsd or NUT is configured. It never contacts the UPS.
func (p *HardwareProbe) CheckUPS(ctx context.Context) (dto.CapabilityState, string) {
	running, err := exists(p.path(constants.ApcPidFile))
	if running {
		return dto.CapabilityPresent, "apcupsd running"
	}
	enabled, cfgErr := p.serviceEnabled(constants.ApcupsdPluginCfg)
	if enabled {
		return dto.CapabilityPresent, "apcupsd enabled in UPS settings"
	}
	if state, detail := p.CheckNUT(ctx); state == dto.CapabilityPresent {
		return state, detail
	}
	if err = errors.Join(err, cfgErr); err != nil {
		return dto.CapabilityUnknown, fmt.Sprintf("cannot read UPS configuration: %v", err)
	}
	return dto.CapabilityAbsent, "no apcupsd or NUT UPS configured"
}

// CheckNUT reports whether the NUT plugin is installed and enabled or running.
func (p *HardwareProbe) CheckNUT(_ context.Context) (dto.CapabilityState, string) {
	installed, err := exists(p.path(constants.NutPluginDir))
	if err != nil {
		return dto.CapabilityUnknown, fmt.Sprintf("cannot read NUT plugin: %v", err)
	}
	if !installed {
		return dto.CapabilityAbsent, "NUT plugin not installed"
	}
	if running, _ := exists(p.path(constants.NutPidFile)); running {
		return dto.CapabilityPresent, "NUT upsmon running"
	}
	enabled, err := p.serviceEnabled(constants.NutPluginCfg)
	if enabled {
		return dto.CapabilityPresent, "NUT enabled in plugin settings"
	}
	if err != nil {
		return dto.CapabilityUnknown, fmt.Sprintf("cannot read NUT settings: %v", err)
	}
	return dto.CapabilityAbsent, "NUT plugin installed but not enabled"
}

// serviceEnabled reports whether an Unraid plugin .cfg has SERVICE="enable".
// A missing file is not an error.
func (p *HardwareProbe) serviceEnabled(cfgPath string) (bool, error) {
	f, err := os.Open(p.path(cfgPath))
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer f.Close() //nolint:errcheck // read-only file
	// parseNUTConfig is a generic KEY="value" parser; SERVICE is all we read.
	cfg, err := parseNUTConfig(f)
	if err != nil {
		return false, err
	}
	return cfg.ServiceEnabled, nil
}

// CheckZFS reports whether the ZFS module is loaded with at least one imported
// pool (a pool directory under /proc/spl/kstat/zfs). It does not run zpool.
func (p *HardwareProbe) CheckZFS(_ context.Context) (dto.CapabilityState, string) {
	loaded, err := exists(p.path("/sys/module/zfs"))
	if err != nil {
		return dto.CapabilityUnknown, fmt.Sprintf("cannot read ZFS module: %v", err)
	}
	if !loaded {
		return dto.CapabilityAbsent, "ZFS module not loaded"
	}
	entries, err := readDir(p.path("/proc/spl/kstat/zfs"))
	if err != nil {
		return dto.CapabilityUnknown, fmt.Sprintf("cannot read ZFS pools: %v", err)
	}
	for _, e := range entries {
		if e.IsDir() {
			return dto.CapabilityPresent, "ZFS pool " + e.Name()
		}
	}
	return dto.CapabilityAbsent, "no imported ZFS pools"
}

// CheckFanControl reports whether a hwmon chip has a controllable PWM output
// (pwmN with pwmN_enable) or an IPMI device exists. It does not run ipmitool.
func (p *HardwareProbe) CheckFanControl(_ context.Context) (dto.CapabilityState, string) {
	hwmon := p.path("/sys/class/hwmon")
	chips, readErr := readDir(hwmon)
	for _, chip := range chips {
		dir := filepath.Join(hwmon, chip.Name())
		files, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, f := range files {
			if !pwmRe.MatchString(f.Name()) {
				continue
			}
			if ok, _ := exists(filepath.Join(dir, f.Name()+"_enable")); ok {
				name := readTrimmed(filepath.Join(dir, "name"))
				if name == "" {
					name = chip.Name()
				}
				return dto.CapabilityPresent, fmt.Sprintf("PWM fan control on %s (%s)", name, f.Name())
			}
		}
	}
	if matches, _ := filepath.Glob(p.path("/dev/ipmi*")); len(matches) > 0 {
		return dto.CapabilityPresent, "IPMI device " + filepath.Base(matches[0])
	}
	if readErr != nil {
		return dto.CapabilityUnknown, fmt.Sprintf("cannot read hwmon: %v", readErr)
	}
	return dto.CapabilityAbsent, "no PWM fan controller or IPMI device"
}
