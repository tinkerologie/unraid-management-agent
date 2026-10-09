package collectors

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

// storcliDoc wraps per-controller JSON objects in storcli's top-level envelope.
func storcliDoc(controllers ...string) string {
	return `{"Controllers":[` + strings.Join(controllers, ",") + `]}`
}

const storcliFailure = `{"Command Status":{"Controller":1,"Status":"Failure","Description":"Controller 1 not found"}}`

func TestStorcliJSONHelpers(t *testing.T) {
	m := map[string]any{"Num": float64(7), "Frac": 1.5, "Str": " 42 ", "Bad": "x", "List": []any{}, "Flag": true}
	if jsonString(m, "Num") != "7" || jsonString(m, "List") != "" || jsonString(m, "Missing") != "" {
		t.Error("jsonString conversions are wrong")
	}
	if n, ok := jsonInt(m, "Str"); !ok || n != 42 {
		t.Errorf("jsonInt(Str) = %d, %v", n, ok)
	}
	for _, key := range []string{"Frac", "Bad", "Flag", "Missing"} {
		if _, ok := jsonInt(m, key); ok {
			t.Errorf("jsonInt(%s) should fail", key)
		}
	}
	if parseGbps("No limit ") != nil || leadingNumber("n/a") != nil || derefInt(nil, -1) != -1 {
		t.Error("parse helpers should reject non-numeric input")
	}
	for in, want := range map[string]string{"": "", "0x0000000000000000": "", "zz": "", "500062B20B26BD40": "0x500062b20b26bd40"} {
		if got := normalizeSASAddress(in); got != want {
			t.Errorf("normalizeSASAddress(%q) = %q, want %q", in, got, want)
		}
	}
	if sameSASAddressBlock("", "0x500062b20b26bd40") || !sameSASAddressBlock("0x500062b20b26bd40", "0x500062b20b26bd41") {
		t.Error("sameSASAddressBlock is wrong")
	}
	for status, want := range map[string]bool{"OK": false, "": false, "Not Installed": false, "Not Available": false, "Unknown": false, "Failed": true} {
		if storcliElementProblem(status) != want {
			t.Errorf("storcliElementProblem(%q) != %v", status, want)
		}
	}
}

func TestParseStorcliOutputErrors(t *testing.T) {
	if _, err := parseStorcliOutput("{"); err == nil {
		t.Error("expected invalid JSON error")
	}
	if _, err := parseStorcliOutput(`{"Controllers":[]}`); err == nil {
		t.Error("expected missing controllers error")
	}
	entries, err := parseStorcliOutput(storcliDoc(
		storcliFailure,
		`{"Command Status":{"Controller":2,"Status":"Failure","Description":"None","Detailed Status":[{"ErrMsg":"Cachevault is absent!"}]}}`,
		`{"Command Status":{"Controller":3,"Status":"Failure"}}`,
	))
	if err != nil || len(entries) != 3 {
		t.Fatalf("entries=%d err=%v", len(entries), err)
	}
	for i, want := range []string{"Controller 1 not found", "Cachevault is absent!", "storcli reported a failure"} {
		if entries[i].ok || entries[i].errText != want {
			t.Errorf("entry %d errText = %q, want %q", i, entries[i].errText, want)
		}
	}
	if _, err := parseStorcliShow("nope"); err == nil {
		t.Error("parseStorcliShow should fail on invalid JSON")
	}
	for name, fn := range map[string]func(string) error{
		"controllers": func(s string) error { _, _, err := parseStorcliControllers(s); return err },
		"phys":        func(s string) error { _, _, _, err := parseStorcliPhys(s); return err },
		"enclosures":  func(s string) error { _, err := parseStorcliEnclosures(s); return err },
		"status":      func(s string) error { return applyStorcliEnclosureStatus(s, nil) },
		"drives":      func(s string) error { _, err := parseStorcliDrives(s); return err },
	} {
		if fn("{") == nil {
			t.Errorf("%s parser accepted invalid JSON", name)
		}
	}
}

func TestStorcliControllerFallbacks(t *testing.T) {
	out := storcliDoc(storcliFailure, `{"Command Status":{"Controller":0,"Status":"Success"},"Response Data":{
		"Basics":{"Model":"HBA 9500-16i","PCI Address":"00:41:00:00"},
		"HwCfg":{"ROC temperature(Degree Celsius)":"55C"},
		"PD LIST":[{"EID:Slt":"2:0"},{"EID:Slt":"2:1"}]}}`)
	ctrls, errs, err := parseStorcliControllers(out)
	if err != nil || len(ctrls) != 1 || len(errs) != 1 || !strings.Contains(errs[0], "/c1 show all") {
		t.Fatalf("ctrls=%d errs=%v err=%v", len(ctrls), errs, err)
	}
	c := ctrls[0]
	if c.ID != "c0" || c.PCIAddress != "0000:41:00.0" || *c.TemperatureCelsius != 55 || c.PhysicalDrives != 2 {
		t.Errorf("unexpected controller: %+v", c)
	}

	for addr, want := range map[string]string{"00:41:00": "", "00:zz:00:00": ""} {
		if got := pciAddressFromStorcli(map[string]any{"PCI Address": addr}, nil); got != want {
			t.Errorf("pciAddressFromStorcli(%q) = %q", addr, got)
		}
	}
	if controllerTemperature(map[string]any{"ROC temperature(Degree Celsius)": "n/a"}) != nil ||
		controllerTemperature(map[string]any{}) != nil {
		t.Error("controllerTemperature should be nil without a reading")
	}
}

func TestStorcliPhysEnclosuresDrivesFallbacks(t *testing.T) {
	phys, ports, errs, err := parseStorcliPhys(storcliDoc(storcliFailure, `{"Command Status":{"Controller":0,"Status":"Success"},
		"Response Data":{"PhyInfo":[{"SAS_Addr":"0x1"},{"PhyNo":1,"SAS_Addr":"0x0000000000000000","Link_Speed":"No limit","Enbl":"N"}]}}`))
	if err != nil || len(errs) != 1 || len(phys[0]) != 1 || phys[0][0].Enabled || phys[0][0].Connected || len(ports[0]) != 0 {
		t.Errorf("phys=%+v ports=%+v errs=%v err=%v", phys, ports, errs, err)
	}

	encOut := storcliDoc(storcliFailure, `{"Command Status":{"Controller":0,"Status":"Success"},"Response Data":{
		"Enclosure /c0/e8 ":{"Information":{"Enclosure Serial Number":"N/A","Status":"OK"},
			"Properties":[{"EID":8,"State":"Degraded","Slots":12,"PD":3,"ProdID":"BP12G+"}]},
		"Enclosure /c0/e9 ":{"Information":{"EnclLogicalID":"0x5000000000000009"}},
		"Enclosure /c0/e10 ":"not an object",
		"Unrelated":{}}}`)
	encls, err := parseStorcliEnclosures(encOut)
	if err != nil || len(encls) != 2 {
		t.Fatalf("enclosures=%d err=%v", len(encls), err)
	}
	e8 := encls[[2]int{0, 8}]
	if e8.serial != "" || e8.product != "BP12G+" || e8.state != "Degraded" || *e8.populated != 3 || e8.partner != nil {
		t.Errorf("unexpected e8: %+v", e8)
	}
	statusOut := storcliDoc(storcliFailure, `{"Command Status":{"Controller":0,"Status":"Success"},"Response Data":{
		"Enclosure /c0/e7 ":{"Power Supply Info":[{"Power Supply":0,"Status":"Failed"}]}}}`)
	if err := applyStorcliEnclosureStatus(statusOut, encls); err != nil || !encls[[2]int{0, 7}].psus[0].Problem {
		t.Errorf("status for a new enclosure not recorded: %v", err)
	}

	drivesOut := storcliDoc(storcliFailure,
		`{"Command Status":{"Controller":1,"Status":"Success"},"Response Data":{
			"Drive /c1/s3":[{"EID:Slt":" :3","DID":5,"State":"JBOD","Model":"X"}],
			"Drive /c1/e4/s1":[{"DID":6}],
			"Drive /c1/e4/s0":[{"DID":7}],
			"Drive /c1/e2/s9":[],
			"Not a drive":[{"DID":1}]}}`,
		`{"Command Status":{"Controller":0,"Status":"Success"},"Response Data":{"Drive /c0/e2/s0":[{"DID":8}]}}`)
	drives, err := parseStorcliDrives(drivesOut)
	if err != nil || len(drives) != 4 {
		t.Fatalf("drives=%d err=%v", len(drives), err)
	}
	order := []string{}
	for _, d := range drives {
		order = append(order, strings.TrimSpace(strings.Join([]string{
			string(rune('0' + d.ControllerIndex)), string(rune('0' + derefInt(d.EnclosureDeviceID, 0))), string(rune('0' + d.Slot)),
		}, "")))
	}
	if strings.Join(order, ",") != "020,103,140,141" {
		t.Errorf("drive order = %v", order)
	}
	if drives[1].EnclosureDeviceID != nil || drives[1].State != "JBOD" {
		t.Errorf("direct-attached drive parsed wrongly: %+v", drives[1])
	}
}

func TestSESDecoding(t *testing.T) {
	var u sesU64
	for in, want := range map[string]uint64{`null`: 0, `""`: 0, `"0x10"`: 16, `"12"`: 12, `5764824130926792230`: 5764824130926792230} {
		if err := json.Unmarshal([]byte(in), &u); err != nil || uint64(u) != want {
			t.Errorf("sesU64(%s) = %d, %v", in, u, err)
		}
	}
	if err := json.Unmarshal([]byte(`"0xzz"`), &u); err == nil {
		t.Error("sesU64 accepted an invalid value")
	}
	var f sesFlag
	for in, want := range map[string]bool{`1`: true, `true`: true, `0`: false, `false`: false, `null`: false, `"1"`: true} {
		if err := json.Unmarshal([]byte(in), &f); err != nil || bool(f) != want {
			t.Errorf("sesFlag(%s) = %v", in, f)
		}
	}

	var p sesPath
	if parseSESConfig("{", &p) == nil || parseSESConfig(`{}`, &p) == nil || parseSESJoin("{", &p) == nil || parseSESJoin(`{}`, &p) == nil {
		t.Error("SES parsers accepted invalid documents")
	}
	p = sesPath{vendor: "SYSFS", product: "FROM-SYSFS", revision: "01"}
	cfg := `{"configuration_diagnostic_page":{"enclosure_descriptor_list":[
		{"subenclosure_identifier":{"i":1},"enclosure_logical_identifier":1,"product_identification":"SECONDARY"},
		{"subenclosure_identifier":{"i":0},"enclosure_logical_identifier":"0x500000000000000a","enclosure_vendor_identification":"  ",
		 "product_identification":"","product_revision_level":""}]}}`
	if err := parseSESConfig(cfg, &p); err != nil || p.logicalID != "0x500000000000000a" || p.vendor != "SYSFS" ||
		p.product != "FROM-SYSFS" || p.revision != "01" {
		t.Errorf("unexpected config: %+v err=%v", p, err)
	}

	for status, want := range map[string]int{"Unrecoverable": 3, "Critical": 2, "Noncritical": 1, "OK": 0, "Not installed": 0} {
		if sesSeverity(status) != want {
			t.Errorf("sesSeverity(%q) != %d", status, want)
		}
	}
	if !sesProblem(sesStatusDescriptor{PredictedFailure: true}) || !sesProblem(sesStatusDescriptor{}, false, true) ||
		sesProblem(sesStatusDescriptor{}, false) {
		t.Error("sesProblem is wrong")
	}
	if descriptorText(" <empty> ") != "" || descriptorText("SN=1;FW=2") != "" || descriptorText("Fan 1") != "Fan 1" {
		t.Error("descriptorText is wrong")
	}
	raw := 0xffff
	if sesReadingValue(&sesReading{RawValue: &raw}) != nil || sesReadingValue(nil) != nil || sesReadingValue(&sesReading{}) != nil {
		t.Error("invalid readings must have no value")
	}
}

// sesEl builds a joined-page element for synthetic SES paths.
func sesEl(typ, num int, status, desc string, st sesStatusDescriptor) sesElement {
	st.Status = sesCode{Meaning: status}
	return sesElement{Type: sesCode{I: typ}, Number: num, Descriptor: desc, Status: st}
}

func TestBuildSESEnclosureEdgeCases(t *testing.T) {
	fanSpeed, amps, invalid := 120, 150, sesInvalidReading
	a := sesPath{device: "sg9", revision: "0300", elements: []sesElement{
		sesEl(sesTypePowerSupply, 0, "Critical", "PSU A", sesStatusDescriptor{ACFail: true, Fail: true}),
		sesEl(sesTypeCooling, 0, "OK", "", sesStatusDescriptor{ActualFanSpeed: &fanSpeed}),
		sesEl(sesTypeCooling, 1, "OK", "", sesStatusDescriptor{Fail: true}),
		sesEl(sesTypeESCElectronics, 0, "OK", "", sesStatusDescriptor{Report: true}),
		sesEl(sesTypeESCElectronics, 1, "Noncritical", "", sesStatusDescriptor{}),
		sesEl(sesTypeSASConnector, 0, "Critical", "AA=500000000000000b;AP=zz", sesStatusDescriptor{Fail: true}),
		sesEl(sesTypeSASConnector, 1, "Not installed", "", sesStatusDescriptor{Fail: true}),
		sesEl(sesTypeDeviceSlot, 0, "OK", "", sesStatusDescriptor{FaultSensed: true}),
		sesEl(sesTypeDeviceSlot, 1, "Not installed", "", sesStatusDescriptor{}),
		sesEl(sesTypeTemperature, 0, "OK", "", sesStatusDescriptor{Temperature: &sesCode{I: 0}}),
		sesEl(sesTypeCurrent, 0, "Unrecoverable", "", sesStatusDescriptor{CritOver: true, Current: &sesReading{RawValue: &amps}}),
		sesEl(sesTypeVoltage, 0, "Critical", "", sesStatusDescriptor{CritOver: true, Voltage: &sesReading{RawValue: &invalid}}),
	}}
	// A second path reports the fan as worse; worst status wins.
	b := sesPath{device: "sg10", elements: []sesElement{
		sesEl(sesTypeCooling, 0, "Noncritical", "", sesStatusDescriptor{}),
		sesEl(sesTypeDeviceSlot, 0, "OK", "", sesStatusDescriptor{}),
	}}
	b.elements[1].Additional = &sesAdditionalStatus{Phys: []sesPhyDescriptor{{AttachedSASAddress: 0}}}
	groups := groupSESPaths([]sesPath{a, b})
	if len(groups) != 2 {
		t.Fatalf("paths without a logical ID must stand alone, got %d groups", len(groups))
	}
	merged := mergeSESElements([]sesPath{a, b})
	if merged[sesTypeCooling][0].el.Status.Status.Meaning != "Noncritical" || merged[sesTypeCooling][0].path != 1 {
		t.Errorf("worst status should win: %+v", merged[sesTypeCooling][0])
	}

	e := buildSESEnclosure([]sesPath{a})
	if e.ID != "sg9" || e.Status != "Unrecoverable" {
		t.Errorf("id=%q status=%q", e.ID, e.Status)
	}
	if e.VoltageSensors[0].Problem || e.VoltageSensors[0].Status != "Critical" || *e.CurrentSensors[0].Value != 1.5 {
		t.Errorf("unexpected sensors: %+v %+v", e.VoltageSensors[0], e.CurrentSensors[0])
	}
	if *e.Fans[0].RPM != 1200 || !e.Fans[1].Problem || e.PowerSupplies[0].Description != "PSU A" ||
		e.IOMs[0].Firmware != "0300" || !e.IOMs[0].HostVisible || e.IOMs[1].HostVisible || e.TemperatureSensors[0].Value != nil ||
		e.Connectors[0].AttachedPhy != nil || e.Connectors[1].Installed || !e.SlotDetails[0].Problem || *e.SlotsPopulated != 1 {
		t.Errorf("unexpected enclosure: %+v", e)
	}

	topo := &dto.StorageTopology{Controllers: []dto.StorageController{}, Enclosures: []dto.StorageEnclosure{}, Drives: []dto.StorageDrive{}}
	assembleTopology(topo, nil, []sesPath{a}, nil)
	got := topo.Enclosures[0]
	want := []string{
		"power supply 0: Critical (AC fail, fail)",
		"fan 1: OK (fail)",
		"current sensor 0: Unrecoverable (1.5 A, over critical)",
		"I/O module 1: Noncritical",
		"connector 0: Critical (fail)",
		"slot 0: OK (fault)",
		"path redundancy degraded: only 1 of 2 host paths active",
		"path redundancy degraded: I/O module 1 is Noncritical",
	}
	if strings.Join(got.Problems, "|") != strings.Join(want, "|") {
		t.Errorf("problems =\n%v\nwant\n%v", got.Problems, want)
	}
	if topo.Summary.EnclosuresWithProblems != 1 {
		t.Errorf("summary = %+v", topo.Summary)
	}
}

func TestAssembleTopologyMergeEdgeCases(t *testing.T) {
	two, five := 2, 5
	sc := &storcliResult{
		controllers: []dto.StorageController{{ID: "c1", Index: 1}, {ID: "c0", Index: 0, SASAddress: "0x5000000000000010"}},
		enclosures: map[[2]int]*storcliEnclosure{
			{1, 3}: {controller: 1, eid: 3, state: "Critical"},
			{0, 5}: {controller: 0, eid: 5, logicalID: "0x500000000000000a", serial: "SER", vendor: "V", product: "P",
				state: "Degraded", populated: &two},
			{0, 4}: {controller: 0, eid: 4, sims: []dto.EnclosureIOM{{Index: 0, Status: "OK"}, {Index: 1, Status: "OK"}}},
		},
		drives: []dto.StorageDrive{
			{ControllerIndex: 0, EnclosureDeviceID: &five, MediaErrors: &two, PredictiveFailures: &two, SerialNumber: "S1",
				ControllerPorts: []int{0}, ActivePaths: 1, Ports: []dto.StorageDrivePort{{Port: 0}, {Port: 1}}},
			{ControllerIndex: 0, SMARTAlert: true},
		},
	}
	// SES path for e5 without vendor/product/serial; its IOMs disagree on firmware.
	ses := sesPath{device: "sg3", logicalID: "0x500000000000000a", elements: []sesElement{
		sesEl(sesTypeESCElectronics, 0, "OK", "FW=0100", sesStatusDescriptor{}),
		sesEl(sesTypeESCElectronics, 1, "OK", "FW=0200", sesStatusDescriptor{}),
		sesEl(sesTypeSASConnector, 0, "OK", "AA=5000000000000099", sesStatusDescriptor{}),
		sesEl(sesTypeSASConnector, 1, "OK", "AA=5000000000000011", sesStatusDescriptor{}),
	}}
	topo := &dto.StorageTopology{Controllers: []dto.StorageController{}, Enclosures: []dto.StorageEnclosure{}, Drives: []dto.StorageDrive{}}
	assembleTopology(topo, sc, []sesPath{ses}, map[string]string{"S1": "sdq"})

	ids := []string{}
	for _, e := range topo.Enclosures {
		ids = append(ids, e.ID)
	}
	if strings.Join(ids, ",") != "c0-e4,500000000000000a,c1-e3" {
		t.Fatalf("enclosure order = %v", ids)
	}
	e5 := topo.Enclosures[1]
	if e5.SerialNumber != "SER" || e5.Vendor != "V" || e5.Product != "P" || e5.Status != "Degraded" || *e5.SlotsPopulated != 2 ||
		!e5.IOMFirmwareMismatch || e5.IOMFirmwareDiffersFromPeers {
		t.Errorf("unexpected e5: %+v", e5)
	}
	wantProblems := []string{
		"I/O module firmware mismatch: 0100, 0200",
		"path redundancy degraded: only 1 of 2 host paths active",
		"path redundancy degraded: 1 dual-ported drives have a single active path",
		"enclosure state: Degraded",
	}
	if strings.Join(e5.Problems, "|") != strings.Join(wantProblems, "|") {
		t.Errorf("problems = %v", e5.Problems)
	}
	if c1 := e5.Connectors[1]; c1.AttachedKind != "controller" || c1.AttachedID != "c0" || c1.AttachedPort != nil {
		t.Errorf("connector to a controller without a phy should resolve without a port: %+v", c1)
	}
	if e5.Connectors[0].AttachedKind != "" {
		t.Errorf("unknown far end must stay unresolved: %+v", e5.Connectors[0])
	}
	if e3 := topo.Enclosures[2]; e3.Status != "Critical" || len(e3.Problems) != 0 {
		t.Errorf("SES-severity storcli state must not be repeated as a problem: %+v", e3)
	}
	if e4 := topo.Enclosures[0]; e4.Status != "Unknown" || e4.Redundancy.ActivePaths != 0 || e4.Redundancy.Degraded || len(e4.Problems) != 0 {
		t.Errorf("unexpected e4: %+v", e4)
	}
	if topo.Drives[0].Device != "sdq" || topo.Drives[0].EnclosureID != e5.ID {
		t.Errorf("unexpected drive: %+v", topo.Drives[0])
	}
	want := dto.StorageTopologySummary{Controllers: 2, Enclosures: 3, Drives: 2, DrivesWithMediaErrors: 1,
		DrivesWithPredictiveFailure: 2, SinglePathDrives: 1, EnclosuresWithProblems: 1}
	if topo.Summary != want {
		t.Errorf("summary = %+v, want %+v", topo.Summary, want)
	}
}
