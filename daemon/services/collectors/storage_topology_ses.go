package collectors

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

// SES element type codes (SES-4, table "Element type codes").
const (
	sesTypeDeviceSlot      = 0x01
	sesTypePowerSupply     = 0x02
	sesTypeCooling         = 0x03
	sesTypeTemperature     = 0x04
	sesTypeESCElectronics  = 0x07
	sesTypeEnclosure       = 0x0e
	sesTypeVoltage         = 0x12
	sesTypeCurrent         = 0x13
	sesTypeArrayDeviceSlot = 0x17
	sesTypeSASConnector    = 0x19
)

// sesInvalidReading is the all-ones 16-bit sensor value that enclosures use when
// they have no valid reading (sg_ses prints it as 655.35 V/A).
const sesInvalidReading = 0xffff

// sesFlag is a 0/1 field from sg_ses JSON; booleans are accepted too.
type sesFlag bool

// UnmarshalJSON accepts numbers, booleans and their string forms.
func (f *sesFlag) UnmarshalJSON(b []byte) error {
	switch strings.Trim(strings.TrimSpace(string(b)), `"`) {
	case "", "0", "false", "null":
		*f = false
	default:
		*f = true
	}
	return nil
}

// sesU64 is a 64-bit identifier (SAS address, logical ID) emitted as a JSON number
// by sg_ses; hex strings are accepted too.
type sesU64 uint64

// UnmarshalJSON accepts JSON numbers and hex/decimal strings.
func (u *sesU64) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(bytes.TrimSpace(b)), `"`)
	if s == "" || s == "null" {
		*u = 0
		return nil
	}
	base := 10
	if strings.HasPrefix(strings.ToLower(s), "0x") {
		s, base = s[2:], 16
	}
	v, err := strconv.ParseUint(s, base, 64)
	if err != nil {
		return fmt.Errorf("invalid 64-bit identifier %q: %w", s, err)
	}
	*u = sesU64(v)
	return nil
}

// sasAddress formats the identifier like normalizeSASAddress does.
func (u sesU64) sasAddress() string {
	if u == 0 {
		return ""
	}
	return fmt.Sprintf("0x%016x", uint64(u))
}

type sesCode struct {
	I       int    `json:"i"`
	Meaning string `json:"meaning"`
}

type sesReading struct {
	RawValue *int `json:"raw_value"`
}

type sesStatusDescriptor struct {
	PredictedFailure   sesFlag     `json:"prdfail"`
	Status             sesCode     `json:"status"`
	Ident              sesFlag     `json:"ident"`
	Fail               sesFlag     `json:"fail"`
	Off                sesFlag     `json:"off"`
	Report             sesFlag     `json:"report"`
	DCOverVoltage      sesFlag     `json:"dc_over_voltage"`
	DCUnderVoltage     sesFlag     `json:"dc_under_voltage"`
	DCOverCurrent      sesFlag     `json:"dc_over_current"`
	OverTempFail       sesFlag     `json:"overtmp_fail"`
	TempWarn           sesFlag     `json:"temp_warn"`
	ACFail             sesFlag     `json:"ac_fail"`
	DCFail             sesFlag     `json:"dc_fail"`
	ActualFanSpeed     *int        `json:"actual_fan_speed"`
	CalculatedFanSpeed *int        `json:"calculated_fan_speed"`
	ActualFanCode      sesCode     `json:"actual_fan_code"`
	Temperature        *sesCode    `json:"temperature"`
	OTFailure          sesFlag     `json:"ot_failure"`
	OTWarning          sesFlag     `json:"ot_warning"`
	UTFailure          sesFlag     `json:"ut_failure"`
	UTWarning          sesFlag     `json:"ut_warning"`
	WarnOver           sesFlag     `json:"warn_over"`
	WarnUnder          sesFlag     `json:"warn_under"`
	CritOver           sesFlag     `json:"crit_over"`
	CritUnder          sesFlag     `json:"crit_under"`
	Voltage            *sesReading `json:"voltage"`
	Current            *sesReading `json:"current"`
	FaultSensed        sesFlag     `json:"fault_sensed"`
	FaultRequested     sesFlag     `json:"fault_reqstd"`
	ConnectorType      sesCode     `json:"connector_type"`
}

type sesPhyDescriptor struct {
	AttachedSASAddress sesU64 `json:"attached_sas_address"`
}

type sesAdditionalStatus struct {
	Phys []sesPhyDescriptor `json:"phy_descriptor_list"`
}

type sesElement struct {
	Type       sesCode              `json:"element_type"`
	Descriptor string               `json:"descriptor"`
	Number     int                  `json:"element_number"`
	Overall    sesFlag              `json:"overall"`
	Status     sesStatusDescriptor  `json:"status_descriptor"`
	Additional *sesAdditionalStatus `json:"additional_element_status_descriptor"`
}

// sesPath is everything read through one SES device (one I/O module path).
type sesPath struct {
	device    string
	vendor    string
	product   string
	revision  string
	logicalID string
	elements  []sesElement
}

// parseSESConfig parses "sg_ses --json --page=1" (configuration page) into the
// primary subenclosure's identity.
func parseSESConfig(out string, path *sesPath) error {
	var doc struct {
		Page *struct {
			Enclosures []struct {
				SubEnclosure sesCode `json:"subenclosure_identifier"`
				LogicalID    sesU64  `json:"enclosure_logical_identifier"`
				Vendor       string  `json:"enclosure_vendor_identification"`
				Product      string  `json:"product_identification"`
				Revision     string  `json:"product_revision_level"`
			} `json:"enclosure_descriptor_list"`
		} `json:"configuration_diagnostic_page"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		return fmt.Errorf("invalid sg_ses JSON: %w", err)
	}
	if doc.Page == nil {
		return errors.New("sg_ses JSON has no configuration page")
	}
	for _, e := range doc.Page.Enclosures {
		if e.SubEnclosure.I != 0 {
			continue // secondary subenclosures are not modelled
		}
		path.logicalID = e.LogicalID.sasAddress()
		if v := strings.TrimSpace(e.Vendor); v != "" {
			path.vendor = v
		}
		if v := strings.TrimSpace(e.Product); v != "" {
			path.product = v
		}
		if v := strings.TrimSpace(e.Revision); v != "" {
			path.revision = v
		}
	}
	return nil
}

// parseSESJoin parses "sg_ses --json --join" into the path's individual elements.
func parseSESJoin(out string, path *sesPath) error {
	var doc struct {
		Join *struct {
			Elements []sesElement `json:"element_list"`
		} `json:"join_of_diagnostic_pages"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		return fmt.Errorf("invalid sg_ses JSON: %w", err)
	}
	if doc.Join == nil {
		return errors.New("sg_ses JSON has no joined element list")
	}
	for _, e := range doc.Join.Elements {
		if bool(e.Overall) || e.Number < 0 {
			continue
		}
		path.elements = append(path.elements, e)
	}
	return nil
}

// sesSeverity ranks SES element status codes; higher is worse.
func sesSeverity(status string) int {
	switch strings.ToLower(status) {
	case "unrecoverable":
		return 3
	case "critical":
		return 2
	case "noncritical":
		return 1
	default:
		return 0
	}
}

// parseDescriptorKV parses vendor "KEY=value;KEY=value;" element descriptors.
// It returns nil when the descriptor is free text.
func parseDescriptorKV(desc string) map[string]string {
	if !strings.Contains(desc, "=") {
		return nil
	}
	kv := map[string]string{}
	for _, part := range strings.Split(desc, ";") {
		k, v, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		kv[strings.ToUpper(strings.TrimSpace(k))] = strings.TrimSpace(v)
	}
	return kv
}

// descriptorText returns a free-text descriptor (KEY=value descriptors return "").
func descriptorText(desc string) string {
	desc = strings.TrimSpace(desc)
	if desc == "<empty>" || parseDescriptorKV(desc) != nil {
		return ""
	}
	return desc
}

// sesReadingValue converts a raw 16-bit reading in hundredths to a value; the
// all-ones pattern means "no valid reading".
func sesReadingValue(r *sesReading) *float64 {
	if r == nil || r.RawValue == nil || *r.RawValue == sesInvalidReading {
		return nil
	}
	return floatPtr(float64(*r.RawValue) / 100)
}

// sesProblem reports whether an element's status or flags indicate a problem.
func sesProblem(st sesStatusDescriptor, flags ...sesFlag) bool {
	if sesSeverity(st.Status.Meaning) > 0 || bool(st.PredictedFailure) {
		return true
	}
	for _, f := range flags {
		if f {
			return true
		}
	}
	return false
}

// sesCandidate is an element together with the index of the path it came from.
type sesCandidate struct {
	el   sesElement
	path int
}

// mergeSESElements picks, per element type and number, the worst-status report
// across all paths (earlier paths win ties).
func mergeSESElements(paths []sesPath) map[int][]sesCandidate {
	best := map[[2]int]sesCandidate{}
	for pi, p := range paths {
		for _, el := range p.elements {
			key := [2]int{el.Type.I, el.Number}
			cur, ok := best[key]
			if !ok || sesSeverity(el.Status.Status.Meaning) > sesSeverity(cur.el.Status.Status.Meaning) {
				best[key] = sesCandidate{el: el, path: pi}
			}
		}
	}
	byType := map[int][]sesCandidate{}
	for key, c := range best {
		byType[key[0]] = append(byType[key[0]], c)
	}
	for t := range byType {
		list := byType[t]
		sort.Slice(list, func(i, j int) bool { return list[i].el.Number < list[j].el.Number })
	}
	return byType
}

// pathExpanderAddress returns the most common expander SAS address that the
// path's device slots are attached to: the expander of the reporting I/O module.
func pathExpanderAddress(p sesPath) string {
	counts := map[string]int{}
	for _, el := range p.elements {
		if el.Type.I != sesTypeDeviceSlot && el.Type.I != sesTypeArrayDeviceSlot {
			continue
		}
		if el.Additional == nil {
			continue
		}
		for _, phy := range el.Additional.Phys {
			if addr := phy.AttachedSASAddress.sasAddress(); addr != "" {
				counts[addr]++
			}
		}
	}
	best, bestN := "", 0
	for addr, n := range counts {
		if n > bestN || (n == bestN && addr < best) {
			best, bestN = addr, n
		}
	}
	return best
}

// pathReportingIOM returns the index of the I/O module serving the path.
func pathReportingIOM(p sesPath) *int {
	for _, el := range p.elements {
		if el.Type.I == sesTypeESCElectronics && bool(el.Status.Report) {
			n := el.Number
			return &n
		}
	}
	return nil
}

// buildSESEnclosure merges all SES paths of one physical enclosure.
func buildSESEnclosure(paths []sesPath) dto.StorageEnclosure {
	first := paths[0]
	encl := newEmptyEnclosure()
	encl.LogicalID = first.logicalID
	encl.ID = strings.TrimPrefix(first.logicalID, "0x")
	if encl.ID == "" {
		encl.ID = first.device
	}
	encl.Vendor = first.vendor
	encl.Product = first.product

	iomExpander := map[int]string{}
	iomRevision := map[int]string{}
	for _, p := range paths {
		dev := dto.StorageSESDevice{
			Device:             p.device,
			Revision:           p.revision,
			ReportingIOM:       pathReportingIOM(p),
			ExpanderSASAddress: pathExpanderAddress(p),
		}
		if dev.ReportingIOM != nil {
			if dev.ExpanderSASAddress != "" {
				iomExpander[*dev.ReportingIOM] = dev.ExpanderSASAddress
			}
			if dev.Revision != "" {
				iomRevision[*dev.ReportingIOM] = dev.Revision
			}
		}
		encl.SESDevices = append(encl.SESDevices, dev)
	}

	worst := 0
	elements := mergeSESElements(paths)
	for t, list := range elements {
		for _, c := range list {
			el := c.el
			st := el.Status
			kv := parseDescriptorKV(el.Descriptor)
			status := st.Status.Meaning
			severity := sesSeverity(status)
			if t == sesTypeDeviceSlot || t == sesTypeArrayDeviceSlot {
				severity = 0 // slot status describes the drive, not the enclosure
			}
			switch t {
			case sesTypePowerSupply:
				psu := dto.EnclosurePowerSupply{
					Index: el.Number, Status: status, Description: descriptorText(el.Descriptor),
					SerialNumber: kv["SN"], Firmware: kv["FW"], PartNumber: kv["PN"],
					Fail: bool(st.Fail), ACFail: bool(st.ACFail), DCFail: bool(st.DCFail),
					OverTempFail: bool(st.OverTempFail), TempWarning: bool(st.TempWarn),
					DCOverVoltage: bool(st.DCOverVoltage), DCUnderVoltage: bool(st.DCUnderVoltage),
					DCOverCurrent: bool(st.DCOverCurrent), Off: bool(st.Off),
					PredictedFailure: bool(st.PredictedFailure),
				}
				if w, err := strconv.Atoi(kv["PW"]); err == nil {
					psu.RatedWatts = &w
				}
				psu.Problem = sesProblem(st, st.Fail, st.ACFail, st.DCFail, st.OverTempFail, st.TempWarn,
					st.DCOverVoltage, st.DCUnderVoltage, st.DCOverCurrent)
				encl.PowerSupplies = append(encl.PowerSupplies, psu)
			case sesTypeCooling:
				fan := dto.EnclosureFan{
					Index: el.Number, Status: status, Description: descriptorText(el.Descriptor),
					Speed: st.ActualFanCode.Meaning, Fail: bool(st.Fail), Off: bool(st.Off),
					Problem: sesProblem(st, st.Fail),
				}
				switch {
				case st.CalculatedFanSpeed != nil:
					fan.RPM = intPtr(*st.CalculatedFanSpeed, true)
				case st.ActualFanSpeed != nil:
					fan.RPM = intPtr(*st.ActualFanSpeed*10, true)
				}
				encl.Fans = append(encl.Fans, fan)
			case sesTypeTemperature:
				sensor := dto.EnclosureSensor{
					Index: el.Number, Status: status, Description: descriptorText(el.Descriptor),
					Fail: bool(st.Fail), WarnOver: bool(st.OTWarning), CritOver: bool(st.OTFailure),
					WarnUnder: bool(st.UTWarning), CritUnder: bool(st.UTFailure),
					Problem: sesProblem(st, st.Fail, st.OTFailure, st.OTWarning, st.UTFailure, st.UTWarning),
				}
				// SES reports temperature with a +20 offset; 0 is reserved (no reading).
				if st.Temperature != nil && st.Temperature.I > 0 {
					sensor.Value = floatPtr(float64(st.Temperature.I - 20))
				}
				encl.TemperatureSensors = append(encl.TemperatureSensors, sensor)
			case sesTypeVoltage, sesTypeCurrent:
				sensor := dto.EnclosureSensor{
					Index: el.Number, Status: status, Description: descriptorText(el.Descriptor),
					Fail: bool(st.Fail), WarnOver: bool(st.WarnOver), WarnUnder: bool(st.WarnUnder),
					CritOver: bool(st.CritOver), CritUnder: bool(st.CritUnder),
					Problem: sesProblem(st, st.Fail, st.WarnOver, st.WarnUnder, st.CritOver, st.CritUnder),
				}
				reading := st.Current
				if t == sesTypeVoltage {
					reading = st.Voltage
				}
				sensor.Value = sesReadingValue(reading)
				if sensor.Value == nil {
					// Without a valid reading the threshold flags only reflect the
					// invalid value (e.g. 0xFFFF compared against a limit); the
					// status is still reported, but it is not treated as a problem.
					sensor.Problem = false
					severity = 0
				}
				if t == sesTypeVoltage {
					encl.VoltageSensors = append(encl.VoltageSensors, sensor)
				} else {
					encl.CurrentSensors = append(encl.CurrentSensors, sensor)
				}
			case sesTypeESCElectronics:
				iom := dto.EnclosureIOM{
					Index: el.Number, Status: status, Description: descriptorText(el.Descriptor),
					SerialNumber: kv["SN"], Firmware: kv["FW"], PartNumber: kv["PN"],
					ExpanderSASAddress: iomExpander[el.Number], Fail: bool(st.Fail),
					Problem: sesProblem(st, st.Fail),
				}
				if iom.Firmware == "" {
					iom.Firmware = iomRevision[el.Number]
				}
				for _, dev := range encl.SESDevices {
					if dev.ReportingIOM != nil && *dev.ReportingIOM == el.Number {
						iom.HostVisible = true
					}
				}
				encl.IOMs = append(encl.IOMs, iom)
			case sesTypeSASConnector:
				conn := dto.EnclosureConnector{
					Index: el.Number, Status: status, Description: descriptorText(el.Descriptor),
					Installed: !strings.EqualFold(status, "Not installed"), Fail: bool(st.Fail),
					AttachedSASAddress: normalizeSASAddress(kv["AA"]),
					CableVendor:        kv["VN"], CablePartNumber: kv["PN"], CableSerialNumber: kv["SN"],
					Problem: sesProblem(st, st.Fail),
				}
				if conn.Installed {
					conn.Type = st.ConnectorType.Meaning
				}
				if ap, err := strconv.ParseUint(kv["AP"], 16, 8); err == nil && conn.AttachedSASAddress != "" {
					conn.AttachedPhy = intPtr(int(ap), true)
				}
				encl.Connectors = append(encl.Connectors, conn)
			case sesTypeDeviceSlot, sesTypeArrayDeviceSlot:
				slot := dto.EnclosureSlot{
					Index: el.Number, Status: status,
					Occupied: !strings.EqualFold(status, "Not installed") && !strings.EqualFold(status, "Unsupported"),
					Fault:    bool(st.FaultSensed) || bool(st.FaultRequested),
					Identify: bool(st.Ident),
				}
				slot.Problem = sesSeverity(status) > 0 || slot.Fault
				encl.SlotDetails = append(encl.SlotDetails, slot)
			case sesTypeEnclosure:
				if sn := kv["SN"]; sn != "" && encl.SerialNumber == "" {
					encl.SerialNumber = sn
				}
			}
			if severity > worst {
				worst = severity
			}
		}
	}
	encl.Slots = len(encl.SlotDetails)
	occupied := 0
	for _, s := range encl.SlotDetails {
		if s.Occupied {
			occupied++
		}
	}
	if encl.Slots > 0 {
		encl.SlotsPopulated = &occupied
	}
	encl.Status = []string{"OK", "Noncritical", "Critical", "Unrecoverable"}[worst]
	return encl
}

// newEmptyEnclosure returns an enclosure with all lists initialised (never null in JSON).
func newEmptyEnclosure() dto.StorageEnclosure {
	return dto.StorageEnclosure{
		SESDevices:         []dto.StorageSESDevice{},
		PowerSupplies:      []dto.EnclosurePowerSupply{},
		Fans:               []dto.EnclosureFan{},
		TemperatureSensors: []dto.EnclosureSensor{},
		VoltageSensors:     []dto.EnclosureSensor{},
		CurrentSensors:     []dto.EnclosureSensor{},
		IOMs:               []dto.EnclosureIOM{},
		Connectors:         []dto.EnclosureConnector{},
		SlotDetails:        []dto.EnclosureSlot{},
		Problems:           []string{},
		Redundancy:         dto.EnclosureRedundancy{Reasons: []string{}},
	}
}

// groupSESPaths groups paths by enclosure logical ID (paths without one stand alone),
// preserving discovery order.
func groupSESPaths(paths []sesPath) [][]sesPath {
	var groups [][]sesPath
	index := map[string]int{}
	for _, p := range paths {
		if p.logicalID == "" {
			groups = append(groups, []sesPath{p})
			continue
		}
		if i, ok := index[p.logicalID]; ok {
			groups[i] = append(groups[i], p)
			continue
		}
		index[p.logicalID] = len(groups)
		groups = append(groups, []sesPath{p})
	}
	return groups
}
