package collectors

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

const topoFixtures = "testdata/storage_topology"

// fakeRunner serves canned command output keyed by "<binary basename> <args...>".
type fakeRunner struct {
	mu      sync.Mutex
	outputs map[string]string
	errs    map[string]error
	pending map[string]chan struct{} // keys whose process "has not exited"
	calls   []string
}

func newFakeRunner(t *testing.T) *fakeRunner {
	t.Helper()
	f := &fakeRunner{outputs: map[string]string{}, errs: map[string]error{}, pending: map[string]chan struct{}{}}
	storcli := map[string]string{
		"show":                     "storcli_show.json",
		"/call show all":           "storcli_call_show_all.json",
		"/call/pall show":          "storcli_call_pall_show.json",
		"/call/eall show all":      "storcli_call_eall_show_all.json",
		"/call/eall show status":   "storcli_call_eall_show_status.json",
		"/call/eall/sall show all": "storcli_call_eall_sall_show_all.json",
	}
	for args, file := range storcli {
		f.outputs["storcli "+args+" J nolog"] = readFixture(t, file)
	}
	for _, n := range []int{0, 4, 5, 6, 7} {
		f.outputs[fmt.Sprintf("sg_ses --readonly --json --page=1 /dev/sg%d", n)] = readFixture(t, fmt.Sprintf("sg_ses_sg%d_page1.json", n))
		f.outputs[fmt.Sprintf("sg_ses --readonly --json --join /dev/sg%d", n)] = readFixture(t, fmt.Sprintf("sg_ses_sg%d_join.json", n))
	}
	return f
}

func readFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(topoFixtures, name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return string(b)
}

func (f *fakeRunner) run(_ context.Context, _ time.Duration, name string, args ...string) (string, <-chan struct{}, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := filepath.Base(name) + " " + strings.Join(args, " ")
	f.calls = append(f.calls, key)
	done := make(chan struct{})
	if ch, ok := f.pending[key]; ok {
		done = ch
	} else {
		close(done)
	}
	if err, ok := f.errs[key]; ok {
		return f.outputs[key], done, err
	}
	out, ok := f.outputs[key]
	if !ok {
		return "", done, fmt.Errorf("unexpected command %q", key)
	}
	return out, done, nil
}

func (f *fakeRunner) called(prefix string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.calls {
		if strings.HasPrefix(c, prefix) {
			n++
		}
	}
	return n
}

// newFixtureCollector returns a collector wired to the fixtures; binaries lists
// which candidate binaries "exist".
func newFixtureCollector(t *testing.T, runner *fakeRunner, binaries ...string) *StorageTopologyCollector {
	t.Helper()
	c := NewStorageTopologyCollector(&domain.Context{})
	c.RunFn = runner.run
	c.SysfsRoot = filepath.Join(topoFixtures, "sysfs")
	c.StorcliCandidates = []string{"/sbin/storcli"}
	c.SgSesCandidates = []string{"/usr/bin/sg_ses"}
	exists := map[string]bool{}
	for _, b := range binaries {
		exists[b] = true
	}
	c.IsExecutable = func(p string) bool { return exists[p] }
	c.DiskstatsPath = filepath.Join(t.TempDir(), "diskstats") // absent unless a test writes it
	return c
}

func findEnclosure(t *testing.T, topo *dto.StorageTopology, eid int) dto.StorageEnclosure {
	t.Helper()
	for _, e := range topo.Enclosures {
		if e.EnclosureDeviceID != nil && *e.EnclosureDeviceID == eid {
			return e
		}
	}
	t.Fatalf("enclosure e%d not found", eid)
	return dto.StorageEnclosure{}
}

func TestStorageTopologyGatherFromFixtures(t *testing.T) {
	runner := newFakeRunner(t)
	c := newFixtureCollector(t, runner, "/sbin/storcli", "/usr/bin/sg_ses")
	topo := c.Gather(context.Background())

	if topo.State != dto.StorageTopologyStateOK || len(topo.Errors) != 0 {
		t.Fatalf("state=%q errors=%v", topo.State, topo.Errors)
	}
	if !topo.Sources.Storcli || topo.Sources.StorcliPath != "/sbin/storcli" ||
		topo.Sources.StorcliVersion != "007.3305.0000.0000 Jan 23, 2025" || !topo.Sources.SES || topo.Sources.SESDevices != 5 {
		t.Errorf("unexpected sources: %+v", topo.Sources)
	}
	// Commands are read-only, and storcli always gets "J nolog".
	for _, call := range runner.calls {
		if strings.HasPrefix(call, "storcli ") && !strings.HasSuffix(call, " J nolog") {
			t.Errorf("storcli call without J nolog: %q", call)
		}
		if strings.HasPrefix(call, "sg_ses ") && !strings.Contains(call, "--readonly") {
			t.Errorf("sg_ses call without --readonly: %q", call)
		}
	}

	// Controller.
	if len(topo.Controllers) != 1 {
		t.Fatalf("controllers = %d, want 1", len(topo.Controllers))
	}
	ctrl := topo.Controllers[0]
	if ctrl.Model != "MegaRAID 9580-8i8e" || ctrl.FirmwareVersion != "5.310.02-4101" || ctrl.Status != "Optimal" ||
		ctrl.Personality != "JBOD-Mode" || ctrl.DriverName != "megaraid_sas" || ctrl.ID != ctrl.SerialNumber {
		t.Errorf("unexpected controller identity: %+v", ctrl)
	}
	if ctrl.TemperatureCelsius == nil || *ctrl.TemperatureCelsius != 63 {
		t.Errorf("temperature = %v, want 63", ctrl.TemperatureCelsius)
	}
	if ctrl.PCIAddress != "0000:02:00.0" || ctrl.PCIeLinkSpeed != "16.0 GT/s PCIe" || *ctrl.PCIeLinkWidth != 8 ||
		ctrl.PCIeMaxLinkSpeed != "16.0 GT/s PCIe" || *ctrl.PCIeMaxLinkWidth != 8 {
		t.Errorf("unexpected PCIe link: %+v", ctrl)
	}
	if ctrl.PhysicalDrives != 43 || len(ctrl.Phys) != 16 || len(ctrl.Ports) != 2 {
		t.Fatalf("drives=%d phys=%d ports=%d", ctrl.PhysicalDrives, len(ctrl.Phys), len(ctrl.Ports))
	}
	e242, e245 := findEnclosure(t, topo, 242), findEnclosure(t, topo, 245)
	for i, want := range []struct {
		encl string
		iom  int
	}{{e242.ID, 1}, {e245.ID, 0}} {
		p := ctrl.Ports[i]
		if p.Port != i || p.Width != 4 || *p.LinkRateGbps != 12 || p.AttachedDeviceType != "Edge Expander" ||
			p.AttachedEnclosureID != want.encl || *p.AttachedIOM != want.iom {
			t.Errorf("port %d = %+v, want enclosure %s IOM %d", i, p, want.encl, want.iom)
		}
	}
	if ctrl.Phys[8].Connected || ctrl.Phys[8].Port != nil || ctrl.Phys[8].LinkRateGbps != nil {
		t.Errorf("phy 8 should be unconnected: %+v", ctrl.Phys[8])
	}

	// Enclosures: the two shelves by EID, then the SES-only HighPoint device.
	if len(topo.Enclosures) != 3 || topo.Enclosures[2].Vendor != "HPT" {
		t.Fatalf("unexpected enclosure order: %d enclosures", len(topo.Enclosures))
	}
	if e242.Vendor != "NETAPP" || e242.Product != "DS424IOM12A" || *e242.PartnerDeviceID != 241 ||
		e242.ConnectorName != "C1 x4 & C2 x4" || e242.PortMode != "Multipath" || e242.Status != "OK" ||
		e242.Slots != 24 || *e242.SlotsPopulated != 22 || e242.SerialNumber == "" {
		t.Errorf("unexpected e242 identity: %+v", e242)
	}
	if len(e242.PowerSupplies) != 4 || len(e242.Fans) != 8 || len(e242.TemperatureSensors) != 12 ||
		len(e242.VoltageSensors) != 8 || len(e242.CurrentSensors) != 8 || len(e242.IOMs) != 2 ||
		len(e242.Connectors) != 8 || len(e242.SESDevices) != 2 {
		t.Fatalf("unexpected e242 element counts: %+v", e242)
	}
	psu := e242.PowerSupplies[0]
	if psu.Status != "OK" || psu.Problem || *psu.RatedWatts != 580 || psu.Firmware != "0311" || psu.SerialNumber == "" {
		t.Errorf("unexpected PSU: %+v", psu)
	}
	if fan := e242.Fans[0]; *fan.RPM != 3370 || fan.Speed != "at lowest speed" || fan.Problem {
		t.Errorf("unexpected fan: %+v", fan)
	}
	if ts := e242.TemperatureSensors[0]; *ts.Value != 25 || ts.Problem {
		t.Errorf("unexpected temperature sensor: %+v", ts)
	}
	if v := e242.VoltageSensors[1]; *v.Value != 12.22 {
		t.Errorf("unexpected voltage sensor: %+v", v)
	}
	for i, iom := range e242.IOMs {
		if iom.Firmware != "0281" || !iom.HostVisible || iom.ExpanderSASAddress == "" || iom.Problem {
			t.Errorf("unexpected IOM %d: %+v", i, iom)
		}
	}
	if *e242.SESDevices[0].ReportingIOM != 1 || *e242.SESDevices[1].ReportingIOM != 0 {
		t.Errorf("unexpected SES paths: %+v", e242.SESDevices)
	}
	// Cabling: shelf 1 connector 7 goes to controller port 0, connectors 0/1 and
	// 4/5 to shelf 2's I/O modules 0 and 1; shelf 2 connector 3 to port 1.
	if c7 := e242.Connectors[7]; c7.AttachedKind != "controller" || c7.AttachedID != ctrl.ID || *c7.AttachedPort != 0 ||
		c7.CableVendor != "THE MATE COMPANY" || c7.Type == "" {
		t.Errorf("unexpected e242 connector 7: %+v", c7)
	}
	if c0 := e242.Connectors[0]; c0.AttachedKind != "enclosure" || c0.AttachedID != e245.ID || *c0.AttachedIOM != 0 {
		t.Errorf("unexpected e242 connector 0: %+v", c0)
	}
	if c4 := e242.Connectors[4]; c4.AttachedID != e245.ID || *c4.AttachedIOM != 1 {
		t.Errorf("unexpected e242 connector 4: %+v", c4)
	}
	if c2 := e242.Connectors[2]; c2.Installed || c2.Type != "" || c2.AttachedKind != "" {
		t.Errorf("unexpected empty connector: %+v", c2)
	}
	if c3 := e245.Connectors[3]; c3.AttachedKind != "controller" || *c3.AttachedPort != 1 {
		t.Errorf("unexpected e245 connector 3: %+v", c3)
	}
	// Firmware: consistent within each shelf, different between the shelves.
	if e242.IOMFirmwareMismatch || e245.IOMFirmwareMismatch || !e242.IOMFirmwareDiffersFromPeers || !e245.IOMFirmwareDiffersFromPeers {
		t.Errorf("unexpected firmware flags: e242=%v/%v e245=%v/%v", e242.IOMFirmwareMismatch,
			e242.IOMFirmwareDiffersFromPeers, e245.IOMFirmwareMismatch, e245.IOMFirmwareDiffersFromPeers)
	}
	if r := e242.Redundancy; r.ExpectedPaths != 2 || r.ActivePaths != 2 || r.Degraded || r.SinglePathDrives != 0 {
		t.Errorf("unexpected redundancy: %+v", r)
	}
	if len(e242.Problems) != 0 || len(e245.Problems) != 0 {
		t.Errorf("healthy shelves reported problems: %v %v", e242.Problems, e245.Problems)
	}

	// The HighPoint NVMe enclosure reports a critical temperature sensor.
	hpt := topo.Enclosures[2]
	// Its voltage sensors are "Critical" only because of the invalid 0xFFFF reading:
	// the status is kept, but they are not counted as problems.
	if hpt.Status != "Critical" || hpt.EnclosureDeviceID != nil || len(hpt.Problems) != 1 ||
		hpt.VoltageSensors[0].Value != nil || hpt.VoltageSensors[0].Problem || hpt.VoltageSensors[0].Status != "Critical" ||
		hpt.VoltageSensors[0].Description != "VoltageSense01" || *hpt.TemperatureSensors[0].Value != 84 {
		t.Errorf("unexpected HPT enclosure: status=%s problems=%v volt=%+v", hpt.Status, hpt.Problems, hpt.VoltageSensors[0])
	}
	if !strings.Contains(hpt.Problems[0], "temperature sensor 0: Critical (84 °C") {
		t.Errorf("unexpected problem text: %q", hpt.Problems[0])
	}
	if hpt.Redundancy.Degraded || hpt.Redundancy.ExpectedPaths != 0 {
		t.Errorf("single-path NVMe enclosure must not be degraded: %+v", hpt.Redundancy)
	}

	// Drives.
	if len(topo.Drives) != 43 {
		t.Fatalf("drives = %d, want 43", len(topo.Drives))
	}
	d := topo.Drives[0]
	if *d.EnclosureDeviceID != 242 || d.Slot != 0 || d.EnclosureID != e242.ID || d.Device != "sdy" ||
		*d.LinkRateGbps != 6 || *d.MaxLinkRateGbps != 12 || !d.BelowMaxLinkRate || d.ActivePaths != 2 ||
		!d.Multipath || len(d.ControllerPorts) != 2 || d.ControllerPorts[0] != 1 || d.ControllerPorts[1] != 0 ||
		*d.TemperatureCelsius != 37 || d.WWN == "" || len(d.Ports) != 2 || d.Ports[0].SASAddress == "" {
		t.Errorf("unexpected first drive: %+v", d)
	}
	devices := 0
	for _, drive := range topo.Drives {
		if drive.Device != "" {
			devices++
		}
	}
	if devices != 43 {
		t.Errorf("%d drives mapped to block devices, want 43 (serials with leading spaces included)", devices)
	}
	want := dto.StorageTopologySummary{Controllers: 1, Enclosures: 3, Drives: 43, DrivesBelowMaxLinkRate: 26,
		DrivesWithOtherErrors: 24, EnclosuresWithProblems: 1}
	if topo.Summary != want {
		t.Errorf("summary = %+v, want %+v", topo.Summary, want)
	}
}
