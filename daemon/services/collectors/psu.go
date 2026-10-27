package collectors

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tinkerologie/unraid-management-agent/daemon/constants"
	"github.com/tinkerologie/unraid-management-agent/daemon/domain"
	"github.com/tinkerologie/unraid-management-agent/daemon/dto"
	"github.com/tinkerologie/unraid-management-agent/daemon/lib"
	"github.com/tinkerologie/unraid-management-agent/daemon/logger"
)

// corsairModels records exact IDs/capacities declared by the upstream
// drivers/hwmon/corsair-psu.c table. AXi and non-i RM/HX are different devices.
var corsairModels = map[string]string{
	"1c03": "HX550i", "1c04": "HX650i", "1c05": "HX750i", "1c06": "HX850i",
	"1c07": "HX1000i", "1c08": "HX1200i", "1c09": "RM550i", "1c0a": "RM650i",
	"1c0b": "RM750i", "1c0c": "RM850i", "1c0d": "RM1000i", "1c1e": "HX1000i",
	"1c1f": "HX1500i", "1c23": "HX1200i", "1c27": "HX1200i",
}

// Rated capacity is metadata, never a measurement. Leave the unusual/ambiguous
// HX550i, RM550i and HX1200i (2023) entries unknown pending manufacturer evidence.
// Model references: Corsair HXi/RMi manuals and the upstream USB model table.
// https://www.corsair.com/us/en/explorer/diy-builder/power-supply-units/corsair-hxi-series/
var corsairCapacities = map[string]float64{
	"1c04": 650, "1c05": 750, "1c06": 850, "1c07": 1000, "1c08": 1200,
	"1c0a": 650, "1c0b": 750, "1c0c": 850, "1c0d": 1000,
	"1c1e": 1000, "1c1f": 1500, "1c27": 1200,
}

type psuSource struct {
	device    dto.PSUDevice
	usbPath   string
	hwmonPath string
}

func psuIdentityValue(path string) (string, error) {
	// #nosec G304 -- caller supplies fixed sysfs identity attribute paths.
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	return strings.ToLower(strings.TrimSpace(string(data))), err
}

// discoverPSUs inventories known USB interfaces without reading sensor channels.
// No raw HID access, kernel module loading or I2C probing is performed.
func discoverPSUs(ctx context.Context, root string) ([]psuSource, []string) {
	var sources []psuSource
	var errs []string
	usbRoot := filepath.Join(root, "bus/usb/devices")
	entries, err := os.ReadDir(usbRoot)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, []string{fmt.Sprintf("PSU USB inventory: %v", err)}
	}
	for _, entry := range entries {
		if ctx.Err() != nil {
			return sources, []string{ctx.Err().Error()}
		}
		path := filepath.Join(usbRoot, entry.Name())
		vendor, vendorErr := psuIdentityValue(filepath.Join(path, "idVendor"))
		product, productErr := psuIdentityValue(filepath.Join(path, "idProduct"))
		if vendorErr != nil || productErr != nil {
			errs = append(errs, "PSU USB identity attributes unreadable")
			continue
		}
		model, known := corsairModels[product]
		if vendor != "1b1c" || !known {
			continue
		}
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil {
			errs = append(errs, "PSU USB identity: "+err.Error())
			continue
		}
		identity := readSysfsValue(filepath.Join(path, "serial"))
		if identity == "" {
			identity = resolved
		}
		digest := sha256.Sum256([]byte(vendor + ":" + product + ":" + identity))
		family := "HXi"
		if strings.HasPrefix(model, "RM") {
			family = "RMi"
		}
		var capacity *float64
		if watts, known := corsairCapacities[product]; known {
			capacity = &watts
		}
		sources = append(sources, psuSource{usbPath: resolved, device: dto.PSUDevice{
			ID: fmt.Sprintf("psu-%x", digest[:12]), Vendor: "Corsair", Model: model,
			Family: family, VendorID: vendor, ProductID: product, Backend: "hwmon",
			State: dto.PSUStateDriverUnavailable, CapacityWatts: capacity, Sensors: []dto.PSUSensor{},
			Message: "recognized USB PSU; corsairpsu hwmon interface unavailable",
		}})
	}
	// Some devices expose placeholder or duplicate serials. Disambiguate these
	// by physical port rather than publishing colliding cache/HA/metric IDs.
	idCounts := make(map[string]int)
	for _, source := range sources {
		idCounts[source.device.ID]++
	}
	for i := range sources {
		if idCounts[sources[i].device.ID] > 1 {
			digest := sha256.Sum256([]byte(sources[i].usbPath))
			sources[i].device.ID = fmt.Sprintf("psu-%x", digest[:12])
		}
	}
	dirs, err := os.ReadDir(filepath.Join(root, "class/hwmon"))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		errs = append(errs, "PSU hwmon inventory: "+err.Error())
	}
	for _, entry := range dirs {
		path := filepath.Join(root, "class/hwmon", entry.Name())
		if readSysfsValue(filepath.Join(path, "name")) != "corsairpsu" {
			continue
		}
		dev, err := filepath.EvalSymlinks(filepath.Join(path, "device"))
		if err != nil {
			errs = append(errs, "PSU hwmon identity: "+err.Error())
			continue
		}
		for i := range sources {
			// A path-component boundary prevents 1-1 from matching 1-10.
			if dev == sources[i].usbPath || strings.HasPrefix(dev, sources[i].usbPath+string(filepath.Separator)) {
				driver, driverErr := filepath.EvalSymlinks(filepath.Join(dev, "driver"))
				if driverErr != nil {
					if !errors.Is(driverErr, fs.ErrNotExist) {
						errs = append(errs, sources[i].device.ID+": cannot inspect HID driver binding")
					}
					continue
				}
				sources[i].device.Driver = filepath.Base(driver)
				if sources[i].device.Driver != "corsair-psu" {
					sources[i].device.Message = "recognized USB PSU; HID interface bound to another driver"
					continue
				}
				sources[i].hwmonPath = path
				sources[i].device.Driver = "corsair-psu"
			}
		}
	}
	sort.Slice(sources, func(i, j int) bool { return sources[i].device.ID < sources[j].device.ID })
	return sources, errs
}

type psuChannel struct {
	file, name, label, kind, unit string
	scale                         float64
}

// Channel semantics are specific to corsairpsu; power1 is DC output, not input.
var corsairChannels = []psuChannel{
	{"in0", "input_voltage", "AC input", "voltage", "V", 1000},
	{"curr1", "input_current", "AC input", "current", "A", 1000},
	{"power1", "output_power", "Total DC output", "power", "W", 1000000},
	{"in1", "12v_voltage", "+12V", "voltage", "V", 1000},
	{"in2", "5v_voltage", "+5V", "voltage", "V", 1000},
	{"in3", "3v3_voltage", "+3.3V", "voltage", "V", 1000},
	{"curr2", "12v_current", "+12V", "current", "A", 1000},
	{"curr3", "5v_current", "+5V", "current", "A", 1000},
	{"curr4", "3v3_current", "+3.3V", "current", "A", 1000},
	{"power2", "12v_power", "+12V", "power", "W", 1000000},
	{"power3", "5v_power", "+5V", "power", "W", 1000000},
	{"power4", "3v3_power", "+3.3V", "power", "W", 1000000},
	{"temp1", "vrm_temperature", "VRM", "temperature", "°C", 1000},
	{"temp2", "case_temperature", "Case", "temperature", "°C", 1000},
	{"fan1", "fan_speed", "Fan", "fan", "RPM", 1},
}

func readPSUDevice(ctx context.Context, source psuSource) (dto.PSUDevice, []string) {
	dev := source.device
	if source.hwmonPath == "" {
		return dev, nil
	}
	valid := 0
	var errs []string
	var output *float64
	for _, ch := range corsairChannels {
		if ctx.Err() != nil {
			break
		}
		sensor := dto.PSUSensor{Name: ch.name, Label: ch.label, Kind: ch.kind, Unit: ch.unit}
		if label := readSysfsValue(filepath.Join(source.hwmonPath, ch.file+"_label")); label != "" {
			sensor.Label = label
		}
		// #nosec G304 -- paths are discovered sysfs directories plus fixed attribute names.
		data, err := os.ReadFile(filepath.Join(source.hwmonPath, ch.file+"_input"))
		if err == nil {
			raw, parseErr := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
			value := float64(raw) / ch.scale
			if parseErr != nil || (ch.kind != "temperature" && value < 0) || (ch.kind == "temperature" && !lib.IsPlausibleTempC(value)) {
				errs = append(errs, dev.ID+": invalid "+ch.file+"_input")
			} else {
				sensor.Value = &value
				valid++
			}
		} else if !errors.Is(err, fs.ErrNotExist) {
			errs = append(errs, dev.ID+": cannot read "+ch.file+"_input")
		}
		if ch.name == "output_power" {
			output = sensor.Value
		}
		dev.Sensors = append(dev.Sensors, sensor)
	}
	// This backend does not measure real AC watts or efficiency.
	dev.Sensors = append(dev.Sensors,
		dto.PSUSensor{Name: "input_power", Label: "AC input power", Kind: "power", Unit: "W"},
		dto.PSUSensor{Name: "efficiency", Label: "Efficiency", Kind: "efficiency", Unit: "%"})
	load := dto.PSUSensor{Name: "load", Label: "DC output load", Kind: "load", Unit: "%", Derived: true}
	if output != nil && dev.CapacityWatts != nil && *dev.CapacityWatts > 0 {
		v := *output / *dev.CapacityWatts * 100
		load.Value = &v
	}
	dev.Sensors = append(dev.Sensors, load)
	dev.State, dev.Message = dto.PSUStateOK, ""
	if valid == 0 {
		dev.State, dev.Message = dto.PSUStateError, "hwmon interface returned no valid measurements"
	} else if len(errs) > 0 {
		dev.State, dev.Message = dto.PSUStatePartial, "some hwmon measurements could not be read"
	}
	return dev, errs
}

// PSUCollector publishes one immutable snapshot for all supported PSU backends.
type PSUCollector struct {
	appCtx    *domain.Context
	SysfsRoot string
	// PublishGuard serializes a publication with collector disable/removal.
	PublishGuard func(context.Context, *dto.PSUStatus)
	lastErrors   string
}

// NewPSUCollector creates a passive Corsair hwmon reader.
func NewPSUCollector(ctx *domain.Context) *PSUCollector {
	return &PSUCollector{appCtx: ctx, SysfsRoot: "/sys"}
}

// Gather discovers interfaces afresh and reads only known kernel channels.
func (c *PSUCollector) Gather(ctx context.Context) *dto.PSUStatus {
	status := dto.NewPSUStatus(dto.PSUStateNotDetected, time.Now())
	sources, errs := discoverPSUs(ctx, c.SysfsRoot)
	status.Errors = errs
	working := 0
	for _, source := range sources {
		if ctx.Err() != nil {
			break
		}
		dev, readErrs := readPSUDevice(ctx, source)
		status.Devices = append(status.Devices, dev)
		status.Errors = append(status.Errors, readErrs...)
		if dev.State == dto.PSUStateOK || dev.State == dto.PSUStatePartial {
			working++
		}
	}
	switch {
	case working > 0:
		status.State = dto.PSUStateOK
		if working < len(status.Devices) || len(status.Errors) > 0 {
			status.State = dto.PSUStatePartial
		}
		for _, dev := range status.Devices {
			if dev.State == dto.PSUStatePartial {
				status.State = dto.PSUStatePartial
			}
		}
	case len(status.Errors) > 0:
		status.State = dto.PSUStateError
	case len(status.Devices) > 0:
		status.State = dto.PSUStateDriverUnavailable
		for _, dev := range status.Devices {
			if dev.State == dto.PSUStateError {
				status.State = dto.PSUStateError
			}
		}
	}
	return status
}

// Collect gathers and publishes unless the run has been cancelled.
func (c *PSUCollector) Collect(ctx context.Context) {
	status := c.Gather(ctx)
	if ctx.Err() != nil {
		return
	}
	if c.PublishGuard != nil {
		c.PublishGuard(ctx, status)
	} else {
		domain.Publish(c.appCtx.Hub, constants.TopicPSUStatusUpdate, status)
	}
	errText := strings.Join(status.Errors, "; ")
	if errText != "" && errText != c.lastErrors {
		logger.Warning("PSU collector: %s", errText)
	}
	c.lastErrors = errText
}

// Start runs passive collection until cancelled, guarding both startup and polls.
func (c *PSUCollector) Start(ctx context.Context, interval time.Duration) {
	if ctx.Err() != nil || interval <= 0 {
		return
	}
	// Manual mode also reacts to a late hwmon bind, without waiting for a
	// potentially long configured interval. Dynamic mode uses its event source.
	changed := make(chan struct{}, 1)
	var sources sync.WaitGroup
	sources.Go(func() {
		defer func() {
			if r := recover(); r != nil {
				logger.LogPanicWithStack("PSU collector events", r)
			}
		}()
		NewUeventSource(PSUDeviceUevent).Run(ctx, func() {
			select {
			case changed <- struct{}{}:
			default:
			}
		})
	})
	defer sources.Wait()
	run := func() {
		defer func() {
			if r := recover(); r != nil {
				logger.LogPanicWithStack("PSU collector", r)
			}
		}()
		c.Collect(ctx)
	}
	run()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run()
		case <-changed:
			run()
		}
	}
}
