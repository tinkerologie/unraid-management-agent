package collectors

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tinkerologie/unraid-management-agent/daemon/constants"
	"github.com/tinkerologie/unraid-management-agent/daemon/domain"
	"github.com/tinkerologie/unraid-management-agent/daemon/dto"
	"github.com/tinkerologie/unraid-management-agent/daemon/lib"
)

func psuWrite(t *testing.T, path, value string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(value), 0600); err != nil {
		t.Fatal(err)
	}
}

func psuUSB(t *testing.T, root, port, pid, serial, hwmon string, files map[string]string) string {
	t.Helper()
	usb := filepath.Join(root, "bus/usb/devices", port)
	psuWrite(t, filepath.Join(usb, "idVendor"), "1b1c")
	psuWrite(t, filepath.Join(usb, "idProduct"), pid)
	if serial != "" {
		psuWrite(t, filepath.Join(usb, "serial"), serial)
	}
	if hwmon != "" {
		path := filepath.Join(root, "class/hwmon", hwmon)
		psuWrite(t, filepath.Join(path, "name"), "corsairpsu")
		hid := filepath.Join(usb, port+":1.0", "0003:1B1C:"+strings.ToUpper(pid)+".0001")
		if err := os.MkdirAll(hid, 0755); err != nil {
			t.Fatal(err)
		}
		driver := filepath.Join(root, "bus/hid/drivers/corsair-psu")
		if err := os.MkdirAll(driver, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(driver, filepath.Join(hid, "driver")); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(hid, filepath.Join(path, "device")); err != nil {
			t.Fatal(err)
		}
		for name, value := range files {
			psuWrite(t, filepath.Join(path, name), value)
		}
	}
	return usb
}

func psuSensor(t *testing.T, d dto.PSUDevice, name string) dto.PSUSensor {
	t.Helper()
	for _, sensor := range d.Sensors {
		if sensor.Name == name {
			return sensor
		}
	}
	t.Fatalf("missing sensor %s", name)
	return dto.PSUSensor{}
}

func TestPSUCorsairTelemetry(t *testing.T) {
	root := t.TempDir()
	psuUSB(t, root, "1-1", "1c0d", "private-device-serial", "hwmon12", map[string]string{
		"in0_input": "230000", "in1_input": "12100", "curr1_input": "1000", "curr2_input": "10000",
		"power1_input": "500000000", "power2_input": "120000000", "fan1_input": "0", "temp1_input": "45000",
		"temp2_input": "-10000", "temp1_label": "VRM sensor",
	})
	c := &PSUCollector{SysfsRoot: root}
	s := c.Gather(context.Background())
	if s.State != dto.PSUStateOK || len(s.Devices) != 1 {
		t.Fatalf("snapshot: %+v", s)
	}
	d := s.Devices[0]
	if d.Model != "RM1000i" || d.Family != "RMi" || *d.CapacityWatts != 1000 {
		t.Fatalf("identity: %+v", d)
	}
	for name, want := range map[string]float64{"input_voltage": 230, "12v_voltage": 12.1, "input_current": 1, "12v_current": 10, "output_power": 500, "12v_power": 120, "fan_speed": 0, "vrm_temperature": 45, "case_temperature": -10, "load": 50} {
		v := psuSensor(t, d, name).Value
		if v == nil || *v != want {
			t.Errorf("%s: %v want %v", name, v, want)
		}
	}
	for _, name := range []string{"input_power", "efficiency", "5v_current"} {
		if psuSensor(t, d, name).Value != nil {
			t.Errorf("%s should be unknown", name)
		}
	}
	if !psuSensor(t, d, "load").Derived {
		t.Error("load must be marked derived")
	}
	if psuSensor(t, d, "vrm_temperature").Label != "VRM sensor" {
		t.Error("driver label ignored")
	}
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "private-device-serial") {
		t.Error("raw serial leaked")
	}
	// Re-enumerating a HID device does not change the physical PSU identity.
	old := d.ID
	if got := c.Gather(context.Background()).Devices[0].ID; got != old {
		t.Errorf("unstable ID: %s / %s", old, got)
	}
}

func TestPSUAvailabilityAndIdentity(t *testing.T) {
	for _, tc := range []struct {
		name, pid, chip, want string
		files                 map[string]string
	}{
		{"missing driver", "1c07", "", dto.PSUStateDriverUnavailable, nil},
		{"empty hwmon", "1c07", "corsairpsu", dto.PSUStateError, nil},
		{"AXi excluded", "1c11", "corsairpsu", dto.PSUStateNotDetected, nil},
		{"unknown Corsair product", "9999", "corsairpsu", dto.PSUStateNotDetected, nil},
		{"motherboard excluded", "1c07", "nct6798", dto.PSUStateDriverUnavailable, map[string]string{"power1_input": "300000000"}},
		{"malformed partial", "1c07", "corsairpsu", dto.PSUStatePartial, map[string]string{"fan1_input": "0", "power1_input": "bad", "temp1_input": "127000", "curr1_input": "-1"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			hw := ""
			if tc.chip != "" {
				hw = "hwmon20"
			}
			psuUSB(t, root, "1-1", tc.pid, "", hw, tc.files)
			if hw != "" {
				psuWrite(t, filepath.Join(root, "class/hwmon", hw, "name"), tc.chip)
			}
			got := (&PSUCollector{SysfsRoot: root}).Gather(context.Background())
			if got.State != tc.want {
				t.Fatalf("state=%s want %s: %+v", got.State, tc.want, got)
			}
		})
	}
	root := t.TempDir()
	psuUSB(t, root, "1-1", "1c07", "", "", nil)
	psuUSB(t, root, "1-10", "1c0d", "", "hwmon10", map[string]string{"fan1_input": "600"})
	got := (&PSUCollector{SysfsRoot: root}).Gather(context.Background())
	if got.State != dto.PSUStatePartial || len(got.Devices) != 2 {
		t.Fatalf("multiple PSUs: %+v", got)
	}
	if got.Devices[0].ID == got.Devices[1].ID {
		t.Fatal("port fallback IDs collide")
	}
	for _, dev := range got.Devices {
		if dev.Model == "HX1000i" && dev.State != dto.PSUStateDriverUnavailable {
			t.Fatal("1-1 incorrectly matched 1-10 hwmon")
		}
	}
}

func TestPSUReadFailuresAndHotplug(t *testing.T) {
	root := t.TempDir()
	usb := psuUSB(t, root, "1-1", "1c07", "", "hwmon1", map[string]string{"fan1_input": "600"})
	if err := os.Mkdir(filepath.Join(root, "class/hwmon/hwmon1/power1_input"), 0755); err != nil {
		t.Fatal(err)
	}
	c := &PSUCollector{SysfsRoot: root}
	if got := c.Gather(context.Background()); got.State != dto.PSUStatePartial || len(got.Errors) == 0 {
		t.Fatalf("read error not preserved: %+v", got)
	}
	if err := os.RemoveAll(usb); err != nil {
		t.Fatal(err)
	}
	got := c.Gather(context.Background())
	if len(got.Devices) != 0 {
		t.Fatalf("stale devices after unplug: %+v", got)
	}
	// Remove the dangling hwmon node as the kernel does, then reconnect.
	if err := os.RemoveAll(filepath.Join(root, "class/hwmon/hwmon1")); err != nil {
		t.Fatal(err)
	}
	psuUSB(t, root, "1-1", "1c07", "", "hwmon14", map[string]string{"power1_input": "0"})
	if got = c.Gather(context.Background()); got.State != dto.PSUStateOK || *psuSensor(t, got.Devices[0], "output_power").Value != 0 {
		t.Fatalf("reconnect: %+v", got)
	}
	// Deterministic inventory access errors, even when tests run as root.
	psuWrite(t, filepath.Join(root, "bus/usb/devices/1-2/idVendor/child"), "x")
	if got = c.Gather(context.Background()); len(got.Errors) == 0 {
		t.Fatal("unreadable identity mistaken for absence")
	}
	badRoot := t.TempDir()
	psuWrite(t, filepath.Join(badRoot, "bus/usb/devices"), "not a directory")
	if got = (&PSUCollector{SysfsRoot: badRoot}).Gather(context.Background()); got.State != dto.PSUStateError {
		t.Fatal("inventory error mistaken for absence")
	}
}

func TestPSUCapabilityAndCancellation(t *testing.T) {
	root := t.TempDir()
	sys := filepath.Join(root, "sys")
	p := &HardwareProbe{Root: root}
	if state, _ := p.CheckPSU(context.Background()); state != dto.CapabilityAbsent {
		t.Fatal(state)
	}
	psuUSB(t, sys, "1-1", "1c07", "", "", nil)
	if state, _ := p.CheckPSU(context.Background()); state != dto.CapabilityPresent {
		t.Fatal(state)
	}
	if p.CheckFor(CapabilityPSU) == nil {
		t.Fatal("missing capability check")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if state, _ := p.CheckPSU(ctx); state != dto.CapabilityUnknown {
		t.Fatal(state)
	}
	hub := domain.NewEventBus(10)
	updates := hub.Sub(constants.TopicPSUStatusUpdate.Name)
	defer hub.Unsub(updates)
	c := NewPSUCollector(&domain.Context{Hub: hub})
	c.SysfsRoot = sys
	c.Collect(ctx)
	select {
	case <-updates:
		t.Fatal("cancelled run published")
	default:
	}
	active, stop := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { c.Start(active, time.Millisecond); close(done) }()
	select {
	case <-updates:
	case <-time.After(time.Second):
		t.Fatal("no startup snapshot")
	}
	select {
	case <-updates:
	case <-time.After(time.Second):
		t.Fatal("no periodic snapshot")
	}
	stop()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("collector did not stop")
	}
}

func TestPSURepeatedSerialsAndGuard(t *testing.T) {
	root := t.TempDir()
	psuUSB(t, root, "1-1", "1c07", "000000", "hwmon1", map[string]string{"fan1_input": "0"})
	psuUSB(t, root, "1-2", "1c07", "000000", "hwmon2", map[string]string{"fan1_input": "0"})
	c := &PSUCollector{SysfsRoot: root}
	got := c.Gather(context.Background())
	if len(got.Devices) != 2 || got.Devices[0].ID == got.Devices[1].ID {
		t.Fatal("duplicate serials collapsed PSUs", got)
	}
	called := false
	c.PublishGuard = func(_ context.Context, status *dto.PSUStatus) {
		called = true
		if len(status.Devices) != 2 {
			t.Fatal(status)
		}
	}
	c.Collect(context.Background())
	if !called {
		t.Fatal("publication guard was bypassed")
	}
	// A collection exception must not terminate future manual polls.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	completed := make(chan struct{})
	calls := 0
	c.PublishGuard = func(context.Context, *dto.PSUStatus) {
		calls++
		if calls == 1 {
			panic("test publication fault")
		}
		cancel()
	}
	go func() { c.Start(ctx, time.Millisecond); close(completed) }()
	select {
	case <-completed:
	case <-time.After(time.Second):
		t.Fatal("panic recovery lost collector")
	}
	if calls < 2 {
		t.Fatal("poll after panic was not run")
	}
}

func TestPSUUevents(t *testing.T) {
	for _, tc := range []struct {
		subsystem, action string
		want              bool
	}{
		{"usb", "add", true}, {"usb", "remove", true}, {"hwmon", "add", true}, {"hwmon", "remove", true},
		{"pci", "add", false}, {"usb", "change", false},
	} {
		if got := PSUDeviceUevent(lib.Uevent{Subsystem: tc.subsystem, Action: tc.action}); got != tc.want {
			t.Errorf("%+v got %v", tc, got)
		}
	}
}

func TestPSUBindingAndUnknownCapacity(t *testing.T) {
	root := t.TempDir()
	usb := psuUSB(t, root, "1-1", "1c03", "", "hwmon1", map[string]string{"power1_input": "100000000"})
	c := &PSUCollector{SysfsRoot: root}
	got := c.Gather(context.Background())
	if got.Devices[0].CapacityWatts != nil || psuSensor(t, got.Devices[0], "load").Value != nil {
		t.Fatal("ambiguous model rating was guessed", got)
	}
	link := filepath.Join(usb, "1-1:1.0/0003:1B1C:1C03.0001/driver")
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if got = c.Gather(context.Background()); got.State != dto.PSUStateDriverUnavailable {
		t.Fatal("missing binding treated as ready", got)
	}
	other := filepath.Join(root, "bus/hid/drivers/hid-generic")
	if err := os.MkdirAll(other, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(other, link); err != nil {
		t.Fatal(err)
	}
	got = c.Gather(context.Background())
	if got.State != dto.PSUStateDriverUnavailable || got.Devices[0].Driver != "hid-generic" {
		t.Fatal("other binding treated as ready", got)
	}
}

func TestPSUInventoryAndBindingErrors(t *testing.T) {
	root := t.TempDir()
	usb := psuUSB(t, root, "1-1", "1c07", "", "hwmon1", map[string]string{"fan1_input": "0"})
	link := filepath.Join(usb, "1-1:1.0/0003:1B1C:1C07.0001/driver")
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(link, link); err != nil {
		t.Fatal(err)
	}
	hub := domain.NewEventBus(10)
	updates := hub.Sub(constants.TopicPSUStatusUpdate.Name)
	defer hub.Unsub(updates)
	c := NewPSUCollector(&domain.Context{Hub: hub})
	c.SysfsRoot = root
	c.Collect(context.Background())
	if got := (<-updates).(*dto.PSUStatus); got.State != dto.PSUStateError || len(got.Errors) == 0 {
		t.Fatal("driver access failure mistaken for absence", got)
	}
	// The same error may recur without changing availability or fabricating data.
	c.Collect(context.Background())
	<-updates
	if err := os.RemoveAll(filepath.Join(root, "class/hwmon")); err != nil {
		t.Fatal(err)
	}
	psuWrite(t, filepath.Join(root, "class/hwmon"), "not a directory")
	if got := c.Gather(context.Background()); got.State != dto.PSUStateError || len(got.Errors) == 0 {
		t.Fatal("hwmon inventory failure hidden", got)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	c.Start(cancelled, time.Second)
	c.Start(context.Background(), 0)
	// A cancelled device read skips acquisition and never invents a measurement.
	dev, _ := readPSUDevice(cancelled, psuSource{device: dto.PSUDevice{ID: "psu-cancelled", Sensors: []dto.PSUSensor{}}, hwmonPath: root})
	for _, sensor := range dev.Sensors {
		if sensor.Value != nil {
			t.Fatal("cancelled acquisition returned telemetry")
		}
	}
}
