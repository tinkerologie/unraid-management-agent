package collectors

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/tinkerologie/unraid-management-agent/daemon/constants"
	"github.com/tinkerologie/unraid-management-agent/daemon/domain"
	"github.com/tinkerologie/unraid-management-agent/daemon/dto"
)

// statOK makes the NUT plugin directory and PID file exist.
func statOK(name string) (os.FileInfo, error) {
	if name == constants.NutPluginDir || name == constants.NutPidFile {
		return nil, nil
	}
	return nil, os.ErrNotExist
}

// collectNUTResponse runs one Collect and returns the published response.
func collectNUTResponse(t *testing.T, c *NUTCollector) *dto.NUTResponse {
	t.Helper()
	ch := c.ctx.Hub.Sub(constants.TopicNUTStatusUpdate.Name)
	defer c.ctx.Hub.Unsub(ch, constants.TopicNUTStatusUpdate.Name)

	c.Collect()

	select {
	case msg := <-ch:
		resp, ok := msg.(*dto.NUTResponse)
		if !ok {
			t.Fatalf("published %T, want *dto.NUTResponse", msg)
		}
		return resp
	case <-time.After(2 * time.Second):
		t.Fatal("no nut_status_update published")
		return nil
	}
}

func newTestNUTCollector(outputs map[string]string) *NUTCollector {
	c := NewNUTCollector(&domain.Context{Hub: domain.NewEventBus(10)})
	c.execOutput = fakeExec(outputs)
	c.commandExists = func(string) bool { return true }
	c.stat = statOK
	return c
}

func TestNUTCollectorCollectAllDevices(t *testing.T) {
	c := newTestNUTCollector(map[string]string{
		"upsc -l localhost":      "ups\nups-gpu\nats\n",
		"upsc ups@localhost":     upscSmartUPSX3000,
		"upsc ups-gpu@localhost": strings.Replace(upscSmartUPSX3000, "battery.runtime: 623", "battery.runtime: 508", 1),
		"upsc ats@localhost":     upscAP7752ATS,
	})

	resp := collectNUTResponse(t, c)

	if !resp.Installed || !resp.Running {
		t.Fatalf("installed/running = %v/%v, want true/true", resp.Installed, resp.Running)
	}
	if len(resp.Devices) != 3 {
		t.Fatalf("devices = %v, want 3", resp.Devices)
	}
	if len(resp.Statuses) != 3 {
		t.Fatalf("statuses = %d, want 3", len(resp.Statuses))
	}
	for i, want := range []string{"ups", "ups-gpu", "ats"} {
		if resp.Statuses[i].DeviceName != want {
			t.Errorf("statuses[%d] = %q, want %q", i, resp.Statuses[i].DeviceName, want)
		}
	}
	// Status stays the first device, and is the same object as Statuses[0].
	if resp.Status == nil || resp.Status != resp.Statuses[0] {
		t.Errorf("status = %+v, want statuses[0]", resp.Status)
	}
	if rt := resp.Statuses[1].BatteryRuntime; rt == nil || *rt != 508 {
		t.Errorf("ups-gpu runtime = %v, want 508", rt)
	}
	ats := resp.Statuses[2]
	if ats.Type != "ats" || ats.BatteryCharge != nil || ats.OutputFrequency == nil {
		t.Errorf("ats = type %q battery %v frequency %v", ats.Type, ats.BatteryCharge, ats.OutputFrequency)
	}

	body, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if !strings.Contains(string(body), `"statuses":[{`) || !strings.Contains(string(body), `"status":{`) {
		t.Errorf("JSON lacks status/statuses: %s", body)
	}
}

func TestNUTCollectorCollectSkipsFailingDevice(t *testing.T) {
	// The first device fails: Status is nil as before, the others are listed.
	c := newTestNUTCollector(map[string]string{
		"upsc -l localhost":  "ups\nats\n",
		"upsc ats@localhost": upscAP7752ATS,
	})

	resp := collectNUTResponse(t, c)

	if resp.Status != nil {
		t.Errorf("status = %+v, want nil when the first device fails", resp.Status)
	}
	if len(resp.Statuses) != 1 || resp.Statuses[0].DeviceName != "ats" {
		t.Errorf("statuses = %+v, want only ats", resp.Statuses)
	}
}

func TestNUTCollectorCollectStatusesConcurrently(t *testing.T) {
	// The first device answers only once the last one has been queried, so a
	// sequential loop would time it out. Results keep device order.
	exec := fakeExec(map[string]string{
		"upsc ups@localhost": upscSmartUPSX3000,
		"upsc ats@localhost": upscAP7752ATS,
	})
	atsQueried := make(chan struct{})
	c := newTestNUTCollector(nil)
	c.execOutput = func(command string, args ...string) (string, error) {
		switch strings.Join(args, " ") {
		case "-l localhost":
			return "ups\nats\n", nil
		case "ups@localhost":
			select {
			case <-atsQueried:
			case <-time.After(time.Second):
				return "", errors.New("ups queried before ats: devices are queried sequentially")
			}
		case "ats@localhost":
			close(atsQueried)
		}
		return exec(command, args...)
	}

	resp := collectNUTResponse(t, c)

	if len(resp.Statuses) != 2 || resp.Statuses[0].DeviceName != "ups" || resp.Statuses[1].DeviceName != "ats" {
		t.Fatalf("statuses = %+v, want ups then ats", resp.Statuses)
	}
	if resp.Status != resp.Statuses[0] {
		t.Errorf("status = %+v, want statuses[0]", resp.Status)
	}
}

func TestNUTCollectorCollectStatusesRecoversPanic(t *testing.T) {
	// A panicking query loses only that device; the rest keep their order.
	exec := fakeExec(map[string]string{
		"upsc -l localhost":  "ups\nups-gpu\nats\n",
		"upsc ups@localhost": upscSmartUPSX3000,
		"upsc ats@localhost": upscAP7752ATS,
	})
	c := newTestNUTCollector(nil)
	c.execOutput = func(command string, args ...string) (string, error) {
		if strings.Join(args, " ") == "ups-gpu@localhost" {
			panic("boom")
		}
		return exec(command, args...)
	}

	resp := collectNUTResponse(t, c)

	if len(resp.Statuses) != 2 || resp.Statuses[0].DeviceName != "ups" || resp.Statuses[1].DeviceName != "ats" {
		t.Fatalf("statuses = %+v, want ups then ats", resp.Statuses)
	}
	if resp.Status != resp.Statuses[0] {
		t.Errorf("status = %+v, want statuses[0]", resp.Status)
	}
}

func TestNUTCollectorCollectNotInstalledOrNotRunning(t *testing.T) {
	c := newTestNUTCollector(nil)
	c.stat = func(string) (os.FileInfo, error) { return nil, os.ErrNotExist }
	if resp := collectNUTResponse(t, c); resp.Installed || resp.Statuses != nil {
		t.Errorf("not installed: %+v", resp)
	}

	// Installed, no PID file and pgrep finds nothing.
	c = newTestNUTCollector(nil)
	c.stat = func(name string) (os.FileInfo, error) {
		if name == constants.NutPluginDir {
			return nil, nil
		}
		return nil, os.ErrNotExist
	}
	if resp := collectNUTResponse(t, c); !resp.Installed || resp.Running || resp.Statuses != nil {
		t.Errorf("not running: %+v", resp)
	}

	// Installed, no PID file, but pgrep finds upsd.
	c = newTestNUTCollector(map[string]string{"pgrep -x upsd": "1234\n", "upsc -l localhost": "\n"})
	c.stat = func(name string) (os.FileInfo, error) {
		if name == constants.NutPluginDir {
			return nil, nil
		}
		return nil, os.ErrNotExist
	}
	if resp := collectNUTResponse(t, c); !resp.Running || resp.Status != nil || resp.Statuses != nil {
		t.Errorf("running without devices: %+v", resp)
	}
}

func TestNUTCollectorListDevices(t *testing.T) {
	c := newTestNUTCollector(map[string]string{"upsc -l": "ups\n\nats\n"})
	devices, err := c.listDevices()
	if err != nil || len(devices) != 2 || devices[1].Name != "ats" {
		t.Errorf("listDevices() = %v, %v; want ups and ats via the fallback", devices, err)
	}

	c = newTestNUTCollector(nil)
	if _, err := c.listDevices(); err == nil {
		t.Error("listDevices() error = nil when upsc -l fails")
	}

	c.commandExists = func(string) bool { return false }
	if _, err := c.listDevices(); err == nil {
		t.Error("listDevices() error = nil without upsc")
	}
}

func TestNUTCollectorDefaults(t *testing.T) {
	c := &NUTCollector{}
	if out, err := c.run("echo", "ok"); err != nil || strings.TrimSpace(out) != "ok" {
		t.Errorf("run(echo ok) = %q, %v", out, err)
	}
	if !c.hasCommand("echo") {
		t.Error("hasCommand(echo) = false")
	}
	if _, err := c.statPath(t.TempDir()); err != nil {
		t.Errorf("statPath(tempdir) error = %v", err)
	}
	if _, err := c.statPath("/nonexistent/uma-test"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("statPath(missing) error = %v, want not exist", err)
	}
}

func TestUPSCollectorNUTDeviceName(t *testing.T) {
	c := &UPSCollector{execOutput: fakeExec(map[string]string{
		"upsc -l localhost":  "ups\nups-gpu\nats\n",
		"upsc ups@localhost": upscSmartUPSX3000,
	})}
	status, err := c.collectNUT()
	if err != nil {
		t.Fatalf("collectNUT() error = %v", err)
	}
	if status.DeviceName != "ups" {
		t.Errorf("DeviceName = %q, want ups", status.DeviceName)
	}

	apc := &UPSCollector{execOutput: fakeExec(map[string]string{"apcaccess": "STATUS   : ONLINE\n"})}
	status, err = apc.collectAPC()
	if err != nil {
		t.Fatalf("collectAPC() error = %v", err)
	}
	body, _ := json.Marshal(status)
	if strings.Contains(string(body), "device_name") {
		t.Errorf("apcupsd status has a device_name: %s", body)
	}
}
