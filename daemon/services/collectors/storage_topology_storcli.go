package collectors

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

// storcli JSON output is not stable between storcli versions: keys gain or lose
// trailing spaces ("Device_Type "), numbers are sometimes strings, and sections
// move around. Everything here therefore works on generic maps and looks keys up
// tolerantly (whitespace-trimmed, case-insensitive).

// normKey normalises a storcli JSON key for comparison.
func normKey(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}

// jsonField returns the value of the first matching key in m.
func jsonField(m map[string]any, names ...string) (any, bool) {
	for _, name := range names {
		want := normKey(name)
		for k, v := range m {
			if normKey(k) == want {
				return v, true
			}
		}
	}
	return nil, false
}

// jsonMap returns the object stored under one of names.
func jsonMap(m map[string]any, names ...string) map[string]any {
	v, _ := jsonField(m, names...)
	out, _ := v.(map[string]any)
	return out
}

// jsonList returns the array stored under one of names, keeping only objects.
func jsonList(m map[string]any, names ...string) []map[string]any {
	v, _ := jsonField(m, names...)
	items, _ := v.([]any)
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if obj, ok := item.(map[string]any); ok {
			out = append(out, obj)
		}
	}
	return out
}

// jsonString returns a trimmed string for one of names; numbers are formatted.
func jsonString(m map[string]any, names ...string) string {
	v, ok := jsonField(m, names...)
	if !ok {
		return ""
	}
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	default:
		return ""
	}
}

// jsonInt returns an integer for one of names, accepting numbers and numeric strings.
func jsonInt(m map[string]any, names ...string) (int, bool) {
	v, ok := jsonField(m, names...)
	if !ok {
		return 0, false
	}
	return anyInt(v)
}

// anyInt converts a JSON number or numeric string to int.
func anyInt(v any) (int, bool) {
	switch t := v.(type) {
	case float64:
		if t != math.Trunc(t) {
			return 0, false
		}
		return int(t), true
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(t))
		if err != nil {
			return 0, false
		}
		return n, true
	default:
		return 0, false
	}
}

// intPtr returns a pointer to v when ok.
func intPtr(v int, ok bool) *int {
	if !ok {
		return nil
	}
	return &v
}

// floatPtr returns a pointer to v.
func floatPtr(v float64) *float64 { return &v }

var gbpsRe = regexp.MustCompile(`(?i)^\s*([0-9]+(?:\.[0-9]+)?)\s*Gb/?s`)

// parseGbps parses a storcli link speed such as "12.0Gb/s "; anything else is nil.
func parseGbps(s string) *float64 {
	m := gbpsRe.FindStringSubmatch(s)
	if m == nil {
		return nil
	}
	v, _ := strconv.ParseFloat(m[1], 64)
	return &v
}

var leadingNumberRe = regexp.MustCompile(`^\s*(-?[0-9]+(?:\.[0-9]+)?)`)

// leadingNumber parses the number at the start of s (e.g. " 37C (98.60 F)").
func leadingNumber(s string) *float64 {
	m := leadingNumberRe.FindStringSubmatch(s)
	if m == nil {
		return nil
	}
	v, _ := strconv.ParseFloat(m[1], 64)
	return &v
}

// normalizeSASAddress returns "0x" + 16 lowercase hex digits, or "" for empty/zero addresses.
func normalizeSASAddress(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.TrimPrefix(s, "0x")
	if s == "" {
		return ""
	}
	v, err := strconv.ParseUint(s, 16, 64)
	if err != nil || v == 0 {
		return ""
	}
	return fmt.Sprintf("0x%016x", v)
}

// storcliControllerEntry is one element of storcli's top-level "Controllers" array.
type storcliControllerEntry struct {
	index    int
	ok       bool
	errText  string
	response map[string]any
	status   map[string]any
}

// parseStorcliOutput decodes storcli JSON output into per-controller entries.
func parseStorcliOutput(out string) ([]storcliControllerEntry, error) {
	var doc map[string]any
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		return nil, fmt.Errorf("invalid storcli JSON: %w", err)
	}
	controllers := jsonList(doc, "Controllers")
	if len(controllers) == 0 {
		return nil, errors.New("storcli JSON has no Controllers array")
	}
	entries := make([]storcliControllerEntry, 0, len(controllers))
	for _, ctrl := range controllers {
		status := jsonMap(ctrl, "Command Status")
		idx, _ := jsonInt(status, "Controller")
		entry := storcliControllerEntry{
			index:    idx,
			ok:       strings.EqualFold(jsonString(status, "Status"), "Success"),
			response: jsonMap(ctrl, "Response Data"),
			status:   status,
		}
		if !entry.ok {
			entry.errText = storcliErrorText(status)
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// storcliErrorText extracts a readable failure description from a Command Status.
func storcliErrorText(status map[string]any) string {
	parts := []string{}
	if d := jsonString(status, "Description"); d != "" && !strings.EqualFold(d, "None") {
		parts = append(parts, d)
	}
	for _, detail := range jsonList(status, "Detailed Status") {
		if msg := jsonString(detail, "ErrMsg"); msg != "" {
			parts = append(parts, msg)
		}
	}
	if len(parts) == 0 {
		return "storcli reported a failure"
	}
	return strings.Join(parts, ": ")
}

// storcliVersionInfo holds the result of "storcli show J".
type storcliVersionInfo struct {
	controllers int
	version     string
}

// parseStorcliShow parses "storcli show J" (controller count and CLI version).
func parseStorcliShow(out string) (storcliVersionInfo, error) {
	entries, err := parseStorcliOutput(out)
	if err != nil {
		return storcliVersionInfo{}, err
	}
	info := storcliVersionInfo{version: jsonString(entries[0].status, "CLI Version")}
	info.controllers, _ = jsonInt(entries[0].response, "Number of Controllers")
	return info, nil
}

// pciAddressFromStorcli builds a sysfs PCI address from the storcli Bus section,
// falling back to the "PCI Address" field ("00:02:00:00" = domain:bus:device:function).
func pciAddressFromStorcli(basics, bus map[string]any) string {
	busNo, okBus := jsonInt(bus, "Bus Number")
	devNo, okDev := jsonInt(bus, "Device Number")
	fnNo, okFn := jsonInt(bus, "Function Number")
	if okBus && okDev && okFn {
		domain, _ := jsonInt(bus, "Domain ID")
		return fmt.Sprintf("%04x:%02x:%02x.%x", domain, busNo, devNo, fnNo)
	}
	parts := strings.Split(jsonString(basics, "PCI Address"), ":")
	if len(parts) != 4 {
		return ""
	}
	nums := make([]uint64, 4)
	for i, p := range parts {
		n, err := strconv.ParseUint(p, 16, 32)
		if err != nil {
			return ""
		}
		nums[i] = n
	}
	return fmt.Sprintf("%04x:%02x:%02x.%x", nums[0], nums[1], nums[2], nums[3])
}

// controllerTemperature finds the ROC/controller temperature in the HwCfg section.
func controllerTemperature(hw map[string]any) *float64 {
	for _, key := range []string{"ROC temperature(Degree Celsius)", "Ctrl temperature(Degree Celsius)",
		"Controller temperature(Degree Celsius)"} {
		if v, ok := jsonField(hw, key); ok {
			if n, ok := anyInt(v); ok {
				return floatPtr(float64(n))
			}
			if s, ok := v.(string); ok {
				if f := leadingNumber(s); f != nil {
					return f
				}
			}
		}
	}
	return nil
}

// parseStorcliControllers parses "storcli /call show all J".
func parseStorcliControllers(out string) ([]dto.StorageController, []string, error) {
	entries, err := parseStorcliOutput(out)
	if err != nil {
		return nil, nil, err
	}
	var errs []string
	controllers := make([]dto.StorageController, 0, len(entries))
	for _, e := range entries {
		if !e.ok {
			errs = append(errs, fmt.Sprintf("storcli /c%d show all: %s", e.index, e.errText))
			continue
		}
		basics := jsonMap(e.response, "Basics")
		version := jsonMap(e.response, "Version")
		status := jsonMap(e.response, "Status")
		hw := jsonMap(e.response, "HwCfg")
		ctrl := dto.StorageController{
			Index:           e.index,
			Model:           jsonString(basics, "Model"),
			SerialNumber:    jsonString(basics, "Serial Number"),
			SASAddress:      normalizeSASAddress(jsonString(basics, "SAS Address")),
			FirmwareVersion: jsonString(version, "Firmware Version"),
			FirmwarePackage: jsonString(version, "Firmware Package Build"),
			BIOSVersion:     jsonString(version, "Bios Version"),
			DriverName:      jsonString(version, "Driver Name"),
			DriverVersion:   jsonString(version, "Driver Version"),
			Personality:     jsonString(status, "Current Personality"),
			Status:          jsonString(status, "Controller Status"),
			BBUStatus:       jsonString(status, "BBU Status"),
			PCIAddress:      pciAddressFromStorcli(basics, jsonMap(e.response, "Bus")),
			Ports:           []dto.StorageControllerPort{},
			Phys:            []dto.StorageControllerPhy{},
		}
		if idx, ok := jsonInt(basics, "Controller"); ok {
			ctrl.Index = idx
		}
		ctrl.ID = ctrl.SerialNumber
		if ctrl.ID == "" {
			ctrl.ID = fmt.Sprintf("c%d", ctrl.Index)
		}
		ctrl.MemoryCorrectableErrors = intPtr(jsonInt(status, "Memory Correctable Errors"))
		ctrl.MemoryUncorrectableErrors = intPtr(jsonInt(status, "Memory Uncorrectable Errors"))
		ctrl.TemperatureCelsius = controllerTemperature(hw)
		if n, ok := jsonInt(e.response, "Physical Drives"); ok {
			ctrl.PhysicalDrives = n
		} else {
			ctrl.PhysicalDrives = len(jsonList(e.response, "PD LIST"))
		}
		controllers = append(controllers, ctrl)
	}
	return controllers, errs, nil
}

// parseStorcliPhys parses "storcli /call/pall show J" into phys and ports per controller index.
func parseStorcliPhys(out string) (map[int][]dto.StorageControllerPhy, map[int][]dto.StorageControllerPort, []string, error) {
	entries, err := parseStorcliOutput(out)
	if err != nil {
		return nil, nil, nil, err
	}
	var errs []string
	physBy := map[int][]dto.StorageControllerPhy{}
	portsBy := map[int][]dto.StorageControllerPort{}
	for _, e := range entries {
		if !e.ok {
			errs = append(errs, fmt.Sprintf("storcli /c%d/pall show: %s", e.index, e.errText))
			continue
		}
		phys := []dto.StorageControllerPhy{}
		for _, p := range jsonList(e.response, "PhyInfo") {
			phyNo, ok := jsonInt(p, "PhyNo")
			if !ok {
				continue
			}
			phy := dto.StorageControllerPhy{
				Phy:     phyNo,
				Enabled: !strings.EqualFold(jsonString(p, "Enbl"), "N"),
			}
			addr := normalizeSASAddress(jsonString(p, "SAS_Addr"))
			valid, _ := jsonInt(p, "Port_valid")
			rate := parseGbps(jsonString(p, "Link_Speed"))
			if addr != "" && valid == 1 && rate != nil {
				phy.Connected = true
				phy.LinkRateGbps = rate
				phy.AttachedSASAddress = addr
				phy.AttachedDeviceType = jsonString(p, "Device_Type")
				phy.AttachedPhy = intPtr(jsonInt(p, "Phy_Identifier"))
				phy.Port = intPtr(jsonInt(p, "Port"))
			}
			phys = append(phys, phy)
		}
		sort.Slice(phys, func(i, j int) bool { return phys[i].Phy < phys[j].Phy })
		physBy[e.index] = phys
		portsBy[e.index] = groupPorts(phys)
	}
	return physBy, portsBy, errs, nil
}

// groupPorts groups connected phys into ports.
func groupPorts(phys []dto.StorageControllerPhy) []dto.StorageControllerPort {
	byPort := map[int]*dto.StorageControllerPort{}
	order := []int{}
	for _, phy := range phys {
		if !phy.Connected || phy.Port == nil {
			continue
		}
		port, ok := byPort[*phy.Port]
		if !ok {
			port = &dto.StorageControllerPort{
				Port:               *phy.Port,
				Phys:               []int{},
				AttachedSASAddress: phy.AttachedSASAddress,
				AttachedDeviceType: phy.AttachedDeviceType,
			}
			byPort[*phy.Port] = port
			order = append(order, *phy.Port)
		}
		port.Phys = append(port.Phys, phy.Phy)
		port.Width++
		if port.LinkRateGbps == nil || *phy.LinkRateGbps < *port.LinkRateGbps {
			port.LinkRateGbps = floatPtr(*phy.LinkRateGbps)
		}
	}
	sort.Ints(order)
	ports := make([]dto.StorageControllerPort, 0, len(order))
	for _, p := range order {
		ports = append(ports, *byPort[p])
	}
	return ports
}

// storcliEnclosure is an enclosure as reported by storcli.
type storcliEnclosure struct {
	controller    int
	eid           int
	partner       *int
	logicalID     string // normalized "0x..." or ""
	serial        string
	vendor        string
	product       string
	revision      string
	connectorName string
	portMode      string
	state         string
	slots         int
	populated     *int
	psus          []dto.EnclosurePowerSupply
	fans          []dto.EnclosureFan
	temps         []dto.EnclosureSensor
	sims          []dto.EnclosureIOM
	hasStatus     bool
}

var storcliEnclosureKeyRe = regexp.MustCompile(`(?i)^Enclosure\s+/c(\d+)/e(\d+)$`)

// storcliEnclosureSections returns the "Enclosure /cN/eM" objects of a response, keyed by (controller, eid).
func storcliEnclosureSections(response map[string]any) map[[2]int]map[string]any {
	out := map[[2]int]map[string]any{}
	for k, v := range response {
		m := storcliEnclosureKeyRe.FindStringSubmatch(strings.TrimSpace(k))
		obj, ok := v.(map[string]any)
		if m == nil || !ok {
			continue
		}
		c, _ := strconv.Atoi(m[1])
		e, _ := strconv.Atoi(m[2])
		out[[2]int{c, e}] = obj
	}
	return out
}

// parseStorcliEnclosures parses "storcli /call/eall show all J".
func parseStorcliEnclosures(out string) (map[[2]int]*storcliEnclosure, error) {
	entries, err := parseStorcliOutput(out)
	if err != nil {
		return nil, err
	}
	encls := map[[2]int]*storcliEnclosure{}
	for _, e := range entries {
		if !e.ok {
			// A controller without enclosures reports a failure here; that is normal.
			continue
		}
		for key, obj := range storcliEnclosureSections(e.response) {
			info := jsonMap(obj, "Information")
			inq := jsonMap(obj, "Inquiry Data")
			encl := &storcliEnclosure{
				controller:    key[0],
				eid:           key[1],
				logicalID:     normalizeSASAddress(jsonString(info, "EnclLogicalID")),
				serial:        jsonString(info, "Enclosure Serial Number"),
				vendor:        jsonString(inq, "Vendor Identification"),
				product:       jsonString(inq, "Product Identification"),
				revision:      jsonString(inq, "Product Revision Level"),
				connectorName: jsonString(info, "Connector Name"),
				state:         jsonString(info, "Status"),
			}
			if strings.EqualFold(encl.serial, "N/A") {
				encl.serial = ""
			}
			if p, ok := jsonInt(info, "Partner Device ID"); ok {
				encl.partner = &p
			}
			if props := jsonList(obj, "Properties"); len(props) > 0 {
				encl.slots, _ = jsonInt(props[0], "Slots")
				encl.populated = intPtr(jsonInt(props[0], "PD"))
				encl.portMode = jsonString(props[0], "Port#")
				if s := jsonString(props[0], "State"); s != "" {
					encl.state = s
				}
				if encl.product == "" {
					encl.product = jsonString(props[0], "ProdID")
				}
			}
			encls[key] = encl
		}
	}
	return encls, nil
}

// storcliElementProblem reports whether a storcli element status indicates a problem.
func storcliElementProblem(status string) bool {
	s := strings.ToLower(strings.TrimSpace(status))
	return s != "" && s != "ok" && s != "unknown" && !strings.Contains(s, "not installed") && !strings.Contains(s, "not available")
}

// applyStorcliEnclosureStatus merges "storcli /call/eall show status J" into encls.
func applyStorcliEnclosureStatus(out string, encls map[[2]int]*storcliEnclosure) error {
	entries, err := parseStorcliOutput(out)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if !e.ok {
			continue
		}
		for key, obj := range storcliEnclosureSections(e.response) {
			encl, ok := encls[key]
			if !ok {
				encl = &storcliEnclosure{controller: key[0], eid: key[1]}
				encls[key] = encl
			}
			encl.hasStatus = true
			for _, p := range jsonList(obj, "Power Supply Info") {
				idx, _ := jsonInt(p, "Power Supply")
				status := jsonString(p, "Status")
				encl.psus = append(encl.psus, dto.EnclosurePowerSupply{
					Index: idx, Status: status, Problem: storcliElementProblem(status),
				})
			}
			for _, f := range jsonList(obj, "Fan Info") {
				idx, _ := jsonInt(f, "Fan")
				status := jsonString(f, "Status")
				encl.fans = append(encl.fans, dto.EnclosureFan{
					Index: idx, Status: status, Problem: storcliElementProblem(status),
					Speed: jsonString(f, "Fan Speed"),
				})
			}
			for _, t := range jsonList(obj, "Temperature Sensor Info") {
				idx, _ := jsonInt(t, "Temperature Sensor")
				status := jsonString(t, "Status")
				sensor := dto.EnclosureSensor{Index: idx, Status: status, Problem: storcliElementProblem(status)}
				if v, ok := jsonInt(t, "Temperature"); ok {
					sensor.Value = floatPtr(float64(v))
				}
				encl.temps = append(encl.temps, sensor)
			}
			for _, s := range jsonList(obj, "SIM Info") {
				idx, _ := jsonInt(s, "SIM Module")
				status := jsonString(s, "Status")
				encl.sims = append(encl.sims, dto.EnclosureIOM{
					Index: idx, Status: status, Problem: storcliElementProblem(status),
				})
			}
		}
	}
	return nil
}

var (
	storcliDriveKeyRe = regexp.MustCompile(`(?i)^Drive\s+/c(\d+)(?:/e(\d+))?/s(\d+)$`)
	connectedPortRe   = regexp.MustCompile(`(\d+)\s*\(path(\d+)\)`)
)

// parseConnectedPorts parses "1(path0) 0(path1) " into controller ports in path order.
func parseConnectedPorts(s string) []int {
	type pathPort struct{ path, port int }
	var found []pathPort
	for _, m := range connectedPortRe.FindAllStringSubmatch(s, -1) {
		port, _ := strconv.Atoi(m[1])
		path, _ := strconv.Atoi(m[2])
		found = append(found, pathPort{path, port})
	}
	sort.Slice(found, func(i, j int) bool { return found[i].path < found[j].path })
	ports := make([]int, 0, len(found))
	for _, f := range found {
		ports = append(ports, f.port)
	}
	return ports
}

// parseStorcliDrives parses "storcli /call/eall/sall show all J".
func parseStorcliDrives(out string) ([]dto.StorageDrive, error) {
	entries, err := parseStorcliOutput(out)
	if err != nil {
		return nil, err
	}
	drives := []dto.StorageDrive{}
	for _, e := range entries {
		if !e.ok {
			continue
		}
		for key, v := range e.response {
			m := storcliDriveKeyRe.FindStringSubmatch(strings.TrimSpace(key))
			summaryList, ok := v.([]any)
			if m == nil || !ok || len(summaryList) == 0 {
				continue
			}
			summary, _ := summaryList[0].(map[string]any)
			ctrl, _ := strconv.Atoi(m[1])
			slot, _ := strconv.Atoi(m[3])
			drive := dto.StorageDrive{
				ControllerIndex: ctrl,
				Slot:            slot,
				State:           jsonString(summary, "State"),
				Interface:       jsonString(summary, "Intf"),
				Media:           jsonString(summary, "Med"),
				Model:           jsonString(summary, "Model"),
				Size:            jsonString(summary, "Size"),
				DeviceID:        intPtr(jsonInt(summary, "DID")),
				ControllerPorts: []int{},
				Ports:           []dto.StorageDrivePort{},
			}
			if m[2] != "" {
				eid, _ := strconv.Atoi(m[2])
				drive.EnclosureDeviceID = &eid
			}
			prefix := strings.TrimSpace(key)
			detail := jsonMap(e.response, prefix+" - Detailed Information")
			state := jsonMap(detail, prefix+" State")
			attrs := jsonMap(detail, prefix+" Device attributes")
			policies := jsonMap(detail, prefix+" Policies/Settings")

			drive.MediaErrors = intPtr(jsonInt(state, "Media Error Count"))
			drive.OtherErrors = intPtr(jsonInt(state, "Other Error Count"))
			drive.PredictiveFailures = intPtr(jsonInt(state, "Predictive Failure Count"))
			drive.UnrecoveredMediumErrors = intPtr(jsonInt(state, "Unrecovered Medium Error Count"))
			drive.ShieldCounter = intPtr(jsonInt(state, "Shield Counter"))
			drive.SMARTAlert = strings.EqualFold(jsonString(state, "S.M.A.R.T alert flagged by drive"), "Yes")
			drive.TemperatureCelsius = leadingNumber(jsonString(state, "Drive Temperature"))

			drive.SerialNumber = jsonString(attrs, "SN")
			drive.Vendor = jsonString(attrs, "Manufacturer Id")
			if model := jsonString(attrs, "Model Number"); model != "" {
				drive.Model = model
			}
			if wwn := jsonString(attrs, "WWN"); wwn != "" && !strings.EqualFold(wwn, "NA") {
				drive.WWN = strings.ToLower(wwn)
			}
			drive.Firmware = jsonString(attrs, "Firmware Revision")
			drive.MaxLinkRateGbps = parseGbps(jsonString(attrs, "Device Speed"))
			drive.LinkRateGbps = parseGbps(jsonString(attrs, "Link Speed"))
			drive.BelowMaxLinkRate = drive.MaxLinkRateGbps != nil && drive.LinkRateGbps != nil &&
				*drive.LinkRateGbps < *drive.MaxLinkRateGbps

			drive.Multipath = strings.EqualFold(jsonString(policies, "Multipath"), "Yes")
			drive.ControllerPorts = parseConnectedPorts(jsonString(policies, "Connected Port Number"))
			for _, p := range jsonList(policies, "Port Information") {
				port, _ := jsonInt(p, "Port")
				dp := dto.StorageDrivePort{
					Port:         port,
					Status:       jsonString(p, "Status"),
					LinkRateGbps: parseGbps(jsonString(p, "Linkspeed")),
					SASAddress:   normalizeSASAddress(jsonString(p, "SAS address")),
				}
				if strings.EqualFold(dp.Status, "Active") {
					drive.ActivePaths++
				}
				drive.Ports = append(drive.Ports, dp)
			}
			drives = append(drives, drive)
		}
	}
	sort.Slice(drives, func(i, j int) bool {
		a, b := drives[i], drives[j]
		if a.ControllerIndex != b.ControllerIndex {
			return a.ControllerIndex < b.ControllerIndex
		}
		ae, be := derefInt(a.EnclosureDeviceID, -1), derefInt(b.EnclosureDeviceID, -1)
		if ae != be {
			return ae < be
		}
		return a.Slot < b.Slot
	})
	return drives, nil
}

// derefInt returns *p or def when p is nil.
func derefInt(p *int, def int) int {
	if p == nil {
		return def
	}
	return *p
}
