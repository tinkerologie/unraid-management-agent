package collectors

import (
	"fmt"
	"sort"
	"strings"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

// storcliResult is everything collected through storcli in one cycle.
type storcliResult struct {
	controllers []dto.StorageController
	phys        map[int][]dto.StorageControllerPhy
	ports       map[int][]dto.StorageControllerPort
	enclosures  map[[2]int]*storcliEnclosure
	drives      []dto.StorageDrive
}

// iomRef identifies an enclosure I/O module.
type iomRef struct {
	enclosureID string
	iom         int
}

// assembleTopology merges storcli and SES data into topo (controllers,
// enclosures, drives, cross references, derived health and summary).
// driveDevices maps drive serial numbers to Linux block device names.
func assembleTopology(topo *dto.StorageTopology, sc *storcliResult, paths []sesPath, driveDevices map[string]string) {
	if sc == nil {
		sc = &storcliResult{}
	}

	// SES enclosures, one per logical ID.
	byLogical := map[string]int{}
	for _, group := range groupSESPaths(paths) {
		encl := buildSESEnclosure(group)
		if encl.LogicalID != "" {
			byLogical[encl.LogicalID] = len(topo.Enclosures)
		}
		topo.Enclosures = append(topo.Enclosures, encl)
	}

	// storcli enclosures: enrich the matching SES enclosure or stand alone.
	keys := make([][2]int, 0, len(sc.enclosures))
	for k := range sc.enclosures {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i][0] != keys[j][0] {
			return keys[i][0] < keys[j][0]
		}
		return keys[i][1] < keys[j][1]
	})
	byEID := map[[2]int]string{}
	for _, k := range keys {
		se := sc.enclosures[k]
		idx, ok := byLogical[se.logicalID]
		if !ok || se.logicalID == "" {
			topo.Enclosures = append(topo.Enclosures, storcliOnlyEnclosure(se))
			idx = len(topo.Enclosures) - 1
		}
		encl := &topo.Enclosures[idx]
		applyStorcliEnclosure(encl, se)
		byEID[k] = encl.ID
	}
	sortEnclosures(topo.Enclosures)

	// Expander SAS address -> enclosure I/O module, for cable resolution.
	expanders := map[string]iomRef{}
	for _, encl := range topo.Enclosures {
		for _, iom := range encl.IOMs {
			if iom.ExpanderSASAddress != "" {
				expanders[iom.ExpanderSASAddress] = iomRef{encl.ID, iom.Index}
			}
		}
	}

	// Controllers with their phys and ports.
	for _, ctrl := range sc.controllers {
		if phys, ok := sc.phys[ctrl.Index]; ok {
			ctrl.Phys = phys
		}
		if ports, ok := sc.ports[ctrl.Index]; ok {
			ctrl.Ports = ports
		}
		for i := range ctrl.Ports {
			if ref, ok := expanders[ctrl.Ports[i].AttachedSASAddress]; ok {
				ctrl.Ports[i].AttachedEnclosureID = ref.enclosureID
				ctrl.Ports[i].AttachedIOM = intPtr(ref.iom, true)
			}
		}
		topo.Controllers = append(topo.Controllers, ctrl)
	}
	resolveConnectors(topo, expanders)

	// Drives.
	for _, d := range sc.drives {
		if d.EnclosureDeviceID != nil {
			d.EnclosureID = byEID[[2]int{d.ControllerIndex, *d.EnclosureDeviceID}]
		}
		d.Device = driveDevices[d.SerialNumber]
		topo.Drives = append(topo.Drives, d)
	}

	applyFirmwareChecks(topo.Enclosures)
	for i := range topo.Enclosures {
		applyRedundancy(&topo.Enclosures[i], topo.Drives)
		topo.Enclosures[i].Problems = enclosureProblems(&topo.Enclosures[i])
	}
	topo.Summary = summarize(topo)
}

// storcliOnlyEnclosure builds an enclosure from storcli data when SES is unavailable.
func storcliOnlyEnclosure(se *storcliEnclosure) dto.StorageEnclosure {
	encl := newEmptyEnclosure()
	encl.LogicalID = se.logicalID
	encl.ID = strings.TrimPrefix(se.logicalID, "0x")
	if encl.ID == "" {
		encl.ID = fmt.Sprintf("c%d-e%d", se.controller, se.eid)
	}
	encl.Status = se.state
	encl.Slots = se.slots
	encl.PowerSupplies = append(encl.PowerSupplies, se.psus...)
	encl.Fans = append(encl.Fans, se.fans...)
	encl.TemperatureSensors = append(encl.TemperatureSensors, se.temps...)
	encl.IOMs = append(encl.IOMs, se.sims...)
	return encl
}

// applyStorcliEnclosure adds storcli identity and state to an enclosure.
func applyStorcliEnclosure(encl *dto.StorageEnclosure, se *storcliEnclosure) {
	ctrl, eid := se.controller, se.eid
	encl.ControllerIndex = &ctrl
	encl.EnclosureDeviceID = &eid
	encl.PartnerDeviceID = se.partner
	encl.ConnectorName = se.connectorName
	encl.PortMode = se.portMode
	if encl.SerialNumber == "" {
		encl.SerialNumber = se.serial
	}
	if encl.Vendor == "" {
		encl.Vendor = se.vendor
	}
	if encl.Product == "" {
		encl.Product = se.product
	}
	if se.populated != nil {
		encl.SlotsPopulated = se.populated
	}
	if storcliElementProblem(se.state) && sesSeverity(encl.Status) == 0 {
		encl.Status = se.state
	}
	if encl.Status == "" {
		encl.Status = "Unknown"
	}
}

// sortEnclosures orders storcli-known enclosures by (controller, EID) first,
// then SES-only enclosures in discovery order.
func sortEnclosures(encls []dto.StorageEnclosure) {
	sort.SliceStable(encls, func(i, j int) bool {
		a, b := encls[i], encls[j]
		if (a.EnclosureDeviceID == nil) != (b.EnclosureDeviceID == nil) {
			return a.EnclosureDeviceID != nil
		}
		if a.EnclosureDeviceID == nil {
			return false
		}
		if *a.ControllerIndex != *b.ControllerIndex {
			return *a.ControllerIndex < *b.ControllerIndex
		}
		return *a.EnclosureDeviceID < *b.EnclosureDeviceID
	})
}

// resolveConnectors identifies what is plugged into each enclosure connector.
func resolveConnectors(topo *dto.StorageTopology, expanders map[string]iomRef) {
	for ei := range topo.Enclosures {
		for ci := range topo.Enclosures[ei].Connectors {
			conn := &topo.Enclosures[ei].Connectors[ci]
			if conn.AttachedSASAddress == "" {
				continue
			}
			if ref, ok := expanders[conn.AttachedSASAddress]; ok {
				conn.AttachedKind = "enclosure"
				conn.AttachedID = ref.enclosureID
				conn.AttachedIOM = intPtr(ref.iom, true)
				continue
			}
			for _, ctrl := range topo.Controllers {
				if !sameSASAddressBlock(ctrl.SASAddress, conn.AttachedSASAddress) {
					continue
				}
				conn.AttachedKind = "controller"
				conn.AttachedID = ctrl.ID
				if conn.AttachedPhy != nil {
					for _, phy := range ctrl.Phys {
						if phy.Phy == *conn.AttachedPhy && phy.Port != nil {
							conn.AttachedPort = intPtr(*phy.Port, true)
						}
					}
				}
			}
		}
	}
}

// sameSASAddressBlock reports whether two SAS addresses belong to the same
// device. Broadcom controllers present a different SAS address on each port
// (consecutive values in the lowest hex digit: 0x...a0 on port 0, 0x...a1 on
// port 1), while storcli reports only the base address.
func sameSASAddressBlock(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	return a[:len(a)-1] == b[:len(b)-1]
}

// iomFirmwareSet returns the distinct, sorted I/O module firmware versions of an enclosure.
func iomFirmwareSet(encl dto.StorageEnclosure) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, iom := range encl.IOMs {
		if iom.Firmware != "" && !seen[iom.Firmware] {
			seen[iom.Firmware] = true
			out = append(out, iom.Firmware)
		}
	}
	sort.Strings(out)
	return out
}

// applyFirmwareChecks flags I/O module firmware differences within an enclosure
// and between enclosures of the same vendor and product.
func applyFirmwareChecks(encls []dto.StorageEnclosure) {
	groups := map[string]map[string]bool{}
	members := map[string]int{}
	for i := range encls {
		fw := iomFirmwareSet(encls[i])
		encls[i].IOMFirmwareMismatch = len(fw) > 1
		if len(fw) == 0 || encls[i].Product == "" {
			continue
		}
		key := encls[i].Vendor + "|" + encls[i].Product
		if groups[key] == nil {
			groups[key] = map[string]bool{}
		}
		for _, v := range fw {
			groups[key][v] = true
		}
		members[key]++
	}
	for i := range encls {
		key := encls[i].Vendor + "|" + encls[i].Product
		if members[key] > 1 && len(groups[key]) > 1 && len(iomFirmwareSet(encls[i])) > 0 {
			encls[i].IOMFirmwareDiffersFromPeers = true
		}
	}
}

// applyRedundancy derives the host path redundancy of an enclosure.
func applyRedundancy(encl *dto.StorageEnclosure, drives []dto.StorageDrive) {
	r := dto.EnclosureRedundancy{Reasons: []string{}}
	for _, iom := range encl.IOMs {
		if !strings.EqualFold(iom.Status, "Not installed") {
			r.ExpectedPaths++
		}
	}
	ports := map[int]bool{}
	for _, d := range drives {
		if d.EnclosureID != encl.ID || d.EnclosureID == "" {
			continue
		}
		for _, p := range d.ControllerPorts {
			ports[p] = true
		}
		if r.ExpectedPaths >= 2 && len(d.Ports) >= 2 && d.ActivePaths < 2 {
			r.SinglePathDrives++
		}
	}
	if len(ports) > 0 {
		r.ActivePaths = len(ports)
	} else {
		r.ActivePaths = len(encl.SESDevices)
	}
	if r.ExpectedPaths >= 2 {
		if r.ActivePaths > 0 && r.ActivePaths < r.ExpectedPaths {
			r.Reasons = append(r.Reasons, fmt.Sprintf("only %d of %d host paths active", r.ActivePaths, r.ExpectedPaths))
		}
		if r.SinglePathDrives > 0 {
			r.Reasons = append(r.Reasons, fmt.Sprintf("%d dual-ported drives have a single active path", r.SinglePathDrives))
		}
		for _, iom := range encl.IOMs {
			if iom.Problem {
				r.Reasons = append(r.Reasons, fmt.Sprintf("I/O module %d is %s", iom.Index, iom.Status))
			}
		}
	}
	r.Degraded = len(r.Reasons) > 0
	encl.Redundancy = r
}

// flagNames lists the names of the set flags.
func flagNames(flags map[string]bool) string {
	names := []string{}
	for name, set := range flags {
		if set {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// problemText formats one element problem.
func problemText(kind string, index int, status, details string) string {
	text := fmt.Sprintf("%s %d: %s", kind, index, status)
	if details != "" {
		text += " (" + details + ")"
	}
	return text
}

// sensorDetails describes a sensor reading and its threshold flags.
func sensorDetails(s dto.EnclosureSensor, unit string) string {
	parts := []string{}
	if s.Value != nil {
		parts = append(parts, fmt.Sprintf("%g %s", *s.Value, unit))
	}
	if f := flagNames(map[string]bool{"fail": s.Fail, "over critical": s.CritOver, "over warning": s.WarnOver,
		"under critical": s.CritUnder, "under warning": s.WarnUnder}); f != "" {
		parts = append(parts, f)
	}
	return strings.Join(parts, ", ")
}

// enclosureProblems lists the enclosure's current problems in a stable order.
func enclosureProblems(encl *dto.StorageEnclosure) []string {
	problems := []string{}
	for _, p := range encl.PowerSupplies {
		if p.Problem {
			problems = append(problems, problemText("power supply", p.Index, p.Status, flagNames(map[string]bool{
				"fail": p.Fail, "AC fail": p.ACFail, "DC fail": p.DCFail, "over temperature": p.OverTempFail,
				"temperature warning": p.TempWarning, "DC overvoltage": p.DCOverVoltage,
				"DC undervoltage": p.DCUnderVoltage, "DC overcurrent": p.DCOverCurrent,
				"predicted failure": p.PredictedFailure,
			})))
		}
	}
	for _, f := range encl.Fans {
		if f.Problem {
			problems = append(problems, problemText("fan", f.Index, f.Status, flagNames(map[string]bool{"fail": f.Fail})))
		}
	}
	for _, group := range []struct {
		kind, unit string
		list       []dto.EnclosureSensor
	}{
		{"temperature sensor", "°C", encl.TemperatureSensors},
		{"voltage sensor", "V", encl.VoltageSensors},
		{"current sensor", "A", encl.CurrentSensors},
	} {
		for _, s := range group.list {
			if s.Problem {
				problems = append(problems, problemText(group.kind, s.Index, s.Status, sensorDetails(s, group.unit)))
			}
		}
	}
	for _, iom := range encl.IOMs {
		if iom.Problem {
			problems = append(problems, problemText("I/O module", iom.Index, iom.Status, flagNames(map[string]bool{"fail": iom.Fail})))
		}
	}
	for _, c := range encl.Connectors {
		if c.Problem && c.Installed {
			problems = append(problems, problemText("connector", c.Index, c.Status, flagNames(map[string]bool{"fail": c.Fail})))
		}
	}
	for _, s := range encl.SlotDetails {
		if s.Problem {
			problems = append(problems, problemText("slot", s.Index, s.Status, flagNames(map[string]bool{"fault": s.Fault})))
		}
	}
	if encl.IOMFirmwareMismatch {
		problems = append(problems, "I/O module firmware mismatch: "+strings.Join(iomFirmwareSet(*encl), ", "))
	}
	for _, reason := range encl.Redundancy.Reasons {
		problems = append(problems, "path redundancy degraded: "+reason)
	}
	if storcliElementProblem(encl.Status) && sesSeverity(encl.Status) == 0 {
		problems = append(problems, "enclosure state: "+encl.Status)
	}
	return problems
}

// summarize computes topology-wide counts.
func summarize(topo *dto.StorageTopology) dto.StorageTopologySummary {
	s := dto.StorageTopologySummary{
		Controllers: len(topo.Controllers),
		Enclosures:  len(topo.Enclosures),
		Drives:      len(topo.Drives),
	}
	for _, d := range topo.Drives {
		if d.BelowMaxLinkRate {
			s.DrivesBelowMaxLinkRate++
		}
		if derefInt(d.MediaErrors, 0) > 0 {
			s.DrivesWithMediaErrors++
		}
		if derefInt(d.OtherErrors, 0) > 0 {
			s.DrivesWithOtherErrors++
		}
		if derefInt(d.PredictiveFailures, 0) > 0 || d.SMARTAlert {
			s.DrivesWithPredictiveFailure++
		}
	}
	for _, e := range topo.Enclosures {
		s.SinglePathDrives += e.Redundancy.SinglePathDrives
		if len(e.Problems) > 0 {
			s.EnclosuresWithProblems++
		}
	}
	return s
}
