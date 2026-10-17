package collectors

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/digitalocean/go-libvirt"
	"github.com/digitalocean/go-libvirt/libvirttest"
	"github.com/digitalocean/go-libvirt/socket"

	"github.com/tinkerologie/unraid-management-agent/daemon/domain"
	"github.com/tinkerologie/unraid-management-agent/daemon/dto"
)

// fakeGuestProgram is the result of one program run through guest-exec.
type fakeGuestProgram struct {
	out, stderr string
	exitCode    int
	running     bool // never exits
}

// fakeGuest is a libvirt connection to VMs with a guest agent.
type fakeGuest struct {
	mu        sync.Mutex
	states    map[string]libvirt.DomainState // missing: VM not found
	stateErr  error
	agentDown bool
	replies   map[string]string // command -> JSON "return" value
	programs  map[string]fakeGuestProgram
	badReply  string // command whose reply is not JSON
	empty     string // command with an empty reply
	pids      map[int]string
	statusErr error // returned by guest-exec-status
}

func (f *fakeGuest) DomainLookupByName(name string) (libvirt.Domain, error) {
	if _, ok := f.states[name]; !ok {
		return libvirt.Domain{}, errors.New("no domain")
	}
	return libvirt.Domain{Name: name}, nil
}

func (f *fakeGuest) DomainGetState(dom libvirt.Domain, _ uint32) (int32, int32, error) {
	if f.stateErr != nil {
		return 0, 0, f.stateErr
	}
	return int32(f.states[dom.Name]), 0, nil
}

func (f *fakeGuest) QEMUDomainAgentCommand(_ libvirt.Domain, cmd string, _ int32, _ uint32) (libvirt.OptString, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.agentDown {
		return nil, errors.New("guest agent is not connected")
	}
	var req struct {
		Execute   string          `json:"execute"`
		Arguments json.RawMessage `json:"arguments"`
	}
	_ = json.Unmarshal([]byte(cmd), &req)
	switch req.Execute {
	case f.badReply:
		return libvirt.OptString{"not json"}, nil
	case f.empty:
		return libvirt.OptString{}, nil
	case "guest-exec":
		var args struct {
			Path string   `json:"path"`
			Arg  []string `json:"arg"`
		}
		_ = json.Unmarshal(req.Arguments, &args)
		if _, ok := f.programs[args.Path]; !ok {
			return nil, errors.New("Failed to execute child process (No such file or directory)")
		}
		if f.pids == nil {
			f.pids = map[int]string{}
		}
		pid := len(f.pids) + 100
		f.pids[pid] = args.Path
		return libvirt.OptString{`{"return":{"pid":` + itoa(pid) + `}}`}, nil
	case "guest-exec-status":
		if f.statusErr != nil {
			return nil, f.statusErr
		}
		var args struct {
			PID int `json:"pid"`
		}
		_ = json.Unmarshal(req.Arguments, &args)
		p := f.programs[f.pids[args.PID]]
		if p.running {
			return libvirt.OptString{`{"return":{"exited":false}}`}, nil
		}
		status, _ := json.Marshal(map[string]any{"return": map[string]any{
			"exited":   true,
			"exitcode": p.exitCode,
			"out-data": base64.StdEncoding.EncodeToString([]byte(p.out)),
			"err-data": base64.StdEncoding.EncodeToString([]byte(p.stderr)),
		}})
		return libvirt.OptString{string(status)}, nil
	}
	if r, ok := f.replies[req.Execute]; ok {
		return libvirt.OptString{`{"return":` + r + `}`}, nil
	}
	return libvirt.OptString{`{"return":{}}`}, nil
}

func (f *fakeGuest) Disconnect() error { return nil }

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}

const (
	testSMIOut  = "0, NVIDIA GeForce RTX 4090, 34, 12, 13, 24564, 63.29, 0, 2535\r\n"
	testDiskOut = `{"Model":"WD_BLACK SN850X 4000GB","Serial":"E823_8FA6. ","Bus":"NVMe","Temp":35,"TempMax":90,"Wear":1}`
	testFSInfo  = `[{"mountpoint":"C:\\","type":"NTFS","total-bytes":1000,"used-bytes":400},{"mountpoint":"","type":"FAT32"},{"mountpoint":"System Reserved","type":"NTFS","total-bytes":5},{"mountpoint":"/data","type":"xfs"}]`
)

func windowsGuest() *fakeGuest {
	return &fakeGuest{
		states: map[string]libvirt.DomainState{"Windows 11": libvirt.DomainRunning},
		replies: map[string]string{
			"guest-get-host-name": `{"host-name":"WIN11"}`,
			"guest-get-osinfo":    `{"id":"mswindows","pretty-name":"Microsoft Windows 11"}`,
			"guest-get-fsinfo":    testFSInfo,
		},
		programs: map[string]fakeGuestProgram{
			guestNvidiaSMIWindows: {out: testSMIOut},
			guestPowerShell:       {out: testDiskOut},
			"fps.exe":             {out: " 144\r\n"},
		},
	}
}

func fastGuestExec(t *testing.T) {
	t.Helper()
	oldPoll, oldTimeout := guestExecPoll, guestExecTimeout
	guestExecPoll, guestExecTimeout = time.Millisecond, 50*time.Millisecond
	t.Cleanup(func() { guestExecPoll, guestExecTimeout = oldPoll, oldTimeout })
}

func testPoller(f *fakeGuest, sensors ...domain.VMGuestSensorConfig) (*vmGuestPoller, chan any) {
	hub := domain.NewEventBus(100)
	sub := hub.Sub("vm_guest_update")
	p := newVMGuestPoller(domain.VMGuestConfig{VMs: []string{"Windows 11"}, Interval: 30, Sensors: sensors}, hub)
	p.dial = func(context.Context) (guestConn, error) { return f, nil }
	return p, sub
}

func nextGuestUpdate(t *testing.T, sub chan any) *dto.VMGuestInfo {
	t.Helper()
	select {
	case msg := <-sub:
		infos := msg.([]*dto.VMGuestInfo)
		if len(infos) != 1 {
			t.Fatalf("got %d VMs, want 1", len(infos))
		}
		return infos[0]
	case <-time.After(2 * time.Second):
		t.Fatal("no vm_guest_update published")
	}
	return nil
}

var fpsSensor = domain.VMGuestSensorConfig{VM: "Windows 11", Name: "fps", Command: []string{"fps.exe", "fps"}, Interval: 5}

func TestVMGuestPoller_RunningWindows(t *testing.T) {
	fastGuestExec(t)
	p, sub := testPoller(windowsGuest(), fpsSensor)

	p.updateAll(context.Background())
	info := nextGuestUpdate(t, sub)
	if !info.Running || !info.AgentConnected || info.Hostname != "WIN11" || info.OSName != "Microsoft Windows 11" {
		t.Errorf("info = %+v", info)
	}
	if len(info.Filesystems) != 1 || *info.Filesystems[0].UsedBytes != 400 {
		t.Errorf("filesystems = %+v", info.Filesystems)
	}
	if len(info.GPUs) != 1 || *info.GPUs[0].TemperatureC != 34 || *info.GPUs[0].MemoryTotalBytes != 24564*1024*1024 {
		t.Errorf("gpus = %+v", info.GPUs)
	}
	if len(info.Disks) != 1 || info.Disks[0].Serial != "E823_8FA6." || *info.Disks[0].TemperatureC != 35 {
		t.Errorf("disks = %+v", info.Disks)
	}
	if len(info.Sensors) != 1 || info.Sensors[0].Value != nil {
		t.Errorf("sensor before first run = %+v", info.Sensors)
	}

	p.runSensor(context.Background(), fpsSensor)
	info = nextGuestUpdate(t, sub)
	if s := info.Sensors[0]; s.Value == nil || *s.Value != "144" || s.UpdatedAt == nil || s.Error != "" {
		t.Errorf("sensor = %+v", s)
	}
}

func TestVMGuestPoller_VMOffKeepsEntriesWithNullReadings(t *testing.T) {
	fastGuestExec(t)
	f := windowsGuest()
	p, sub := testPoller(f, fpsSensor)
	p.updateAll(context.Background())
	nextGuestUpdate(t, sub)
	p.runSensor(context.Background(), fpsSensor)
	nextGuestUpdate(t, sub)

	f.states["Windows 11"] = libvirt.DomainShutoff
	p.updateAll(context.Background())
	info := nextGuestUpdate(t, sub)
	if info.Running || info.AgentConnected || info.Error != "" {
		t.Errorf("info = %+v", info)
	}
	if g := info.GPUs; len(g) != 1 || g[0].Name != "NVIDIA GeForce RTX 4090" || g[0].TemperatureC != nil {
		t.Errorf("gpus = %+v", g)
	}
	if d := info.Disks; len(d) != 1 || d[0].Model == "" || d[0].TemperatureC != nil || d[0].WearPercent != nil {
		t.Errorf("disks = %+v", d)
	}
	if fs := info.Filesystems; len(fs) != 1 || fs[0].Mountpoint != `C:\` || fs[0].TotalBytes != nil {
		t.Errorf("filesystems = %+v", fs)
	}
	if s := info.Sensors[0]; s.Value != nil || s.Error != "" {
		t.Errorf("sensor = %+v", s)
	}

	p.runSensor(context.Background(), fpsSensor)
	if s := nextGuestUpdate(t, sub).Sensors[0]; s.Value != nil || s.Error != "" {
		t.Errorf("sensor while off = %+v", s)
	}
}

func TestVMGuestPoller_AgentDown(t *testing.T) {
	f := windowsGuest()
	f.agentDown = true
	p, sub := testPoller(f, fpsSensor)
	p.updateAll(context.Background())
	info := nextGuestUpdate(t, sub)
	if !info.Running || info.AgentConnected || info.Error != "guest agent not responding" {
		t.Errorf("info = %+v", info)
	}
	p.runSensor(context.Background(), fpsSensor)
	if s := nextGuestUpdate(t, sub).Sensors[0]; s.Value != nil || !strings.Contains(s.Error, "not connected") {
		t.Errorf("sensor = %+v", s)
	}
}

func TestVMGuestPoller_LookupErrors(t *testing.T) {
	f := windowsGuest()
	delete(f.states, "Windows 11")
	p, sub := testPoller(f, fpsSensor)
	p.updateAll(context.Background())
	if info := nextGuestUpdate(t, sub); info.Running || !strings.HasPrefix(info.Error, "VM not found") {
		t.Errorf("info = %+v", info)
	}
	p.runSensor(context.Background(), fpsSensor)
	if s := nextGuestUpdate(t, sub).Sensors[0]; !strings.HasPrefix(s.Error, "VM not found") {
		t.Errorf("sensor = %+v", s)
	}

	f = windowsGuest()
	f.stateErr = errors.New("state failed")
	p, sub = testPoller(f)
	p.updateAll(context.Background())
	if info := nextGuestUpdate(t, sub); info.Error != "state failed" {
		t.Errorf("info = %+v", info)
	}
}

func TestVMGuestPoller_DialFailure(t *testing.T) {
	p, sub := testPoller(windowsGuest(), fpsSensor)
	p.dial = func(context.Context) (guestConn, error) { return nil, errors.New("no socket") }
	p.updateAll(context.Background())
	if info := nextGuestUpdate(t, sub); info.Error != "libvirt connection failed" {
		t.Errorf("info = %+v", info)
	}
	p.runSensor(context.Background(), fpsSensor)
	if s := nextGuestUpdate(t, sub).Sensors[0]; s.Error != "libvirt connection failed" {
		t.Errorf("sensor = %+v", s)
	}
}

func TestVMGuestPoller_ConnectionPerVM(t *testing.T) {
	f := windowsGuest()
	f.states["Other"] = libvirt.DomainShutoff
	hub := domain.NewEventBus(10)
	sub := hub.Sub("vm_guest_update")
	p := newVMGuestPoller(domain.VMGuestConfig{VMs: []string{"Other", "Windows 11"}, Interval: 30}, hub)
	dials := 0
	p.dial = func(context.Context) (guestConn, error) {
		dials++
		if dials == 1 {
			return nil, errors.New("no socket")
		}
		return f, nil
	}
	fastGuestExec(t)
	p.updateAll(context.Background())
	infos := (<-sub).([]*dto.VMGuestInfo)
	if dials != 2 || infos[0].Error != "libvirt connection failed" || !infos[1].AgentConnected {
		t.Errorf("dials = %d, infos = %+v, %+v", dials, infos[0], infos[1])
	}
}

func TestVMGuestPoller_CommandFailuresClearReadings(t *testing.T) {
	fastGuestExec(t)
	f := windowsGuest()
	p, sub := testPoller(f)
	p.updateAll(context.Background())
	nextGuestUpdate(t, sub)

	// fsinfo fails, nvidia-smi exits non-zero, the disk query is not JSON.
	f.badReply = "guest-get-fsinfo"
	f.programs[guestNvidiaSMIWindows] = fakeGuestProgram{exitCode: 9, stderr: "NVIDIA-SMI has failed\nmore"}
	f.programs[guestPowerShell] = fakeGuestProgram{out: "oops"}
	p.updateAll(context.Background())
	info := nextGuestUpdate(t, sub)
	if !info.AgentConnected || len(info.GPUs) != 1 || info.GPUs[0].TemperatureC != nil {
		t.Errorf("gpus = %+v", info.GPUs)
	}
	if len(info.Disks) != 1 || info.Disks[0].TemperatureC != nil {
		t.Errorf("disks = %+v", info.Disks)
	}
	if len(info.Filesystems) != 1 || info.Filesystems[0].TotalBytes != nil {
		t.Errorf("filesystems = %+v", info.Filesystems)
	}

	// The disk query itself fails to start.
	delete(f.programs, guestPowerShell)
	p.updateAll(context.Background())
	if info := nextGuestUpdate(t, sub); len(info.Disks) != 1 || info.Disks[0].TemperatureC != nil {
		t.Errorf("disks = %+v", info.Disks)
	}
}

func TestVMGuestPoller_LinuxGuest(t *testing.T) {
	fastGuestExec(t)
	f := windowsGuest()
	f.replies["guest-get-osinfo"] = `{"id":"ubuntu","pretty-name":"Ubuntu 24.04"}`
	f.programs[guestNvidiaSMILinux] = fakeGuestProgram{out: testSMIOut}
	p, sub := testPoller(f)
	p.updateAll(context.Background())
	info := nextGuestUpdate(t, sub)
	if len(info.GPUs) != 1 || len(info.Disks) != 0 || info.OSName != "Ubuntu 24.04" {
		t.Errorf("info = %+v", info)
	}
}

func TestVMGuestExec(t *testing.T) {
	fastGuestExec(t)
	f := windowsGuest()
	p, _ := testPoller(f)
	dom := libvirt.Domain{Name: "Windows 11"}

	// A run that times out is remembered and not started again while it runs.
	f.programs["slow.exe"] = fakeGuestProgram{running: true}
	if _, err := p.exec(context.Background(), f, dom, "k", "slow.exe", nil); err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Errorf("slow: err = %v", err)
	}
	if _, err := p.exec(context.Background(), f, dom, "k", "slow.exe", nil); err == nil || err.Error() != "previous run is still running" {
		t.Errorf("still running: err = %v", err)
	}
	starts := len(f.pids)

	// Once it exits it is reaped and the next run starts.
	f.programs["slow.exe"] = fakeGuestProgram{out: "done"}
	if out, err := p.exec(context.Background(), f, dom, "k", "slow.exe", nil); err != nil || out != "done" || len(f.pids) != starts+1 {
		t.Errorf("after exit: out = %q, err = %v", out, err)
	}
	if _, ok := p.pending["k"]; ok {
		t.Error("pending PID not cleared")
	}

	// A failed status check of a pending run keeps it pending; a PID the
	// guest no longer knows (it restarted) is dropped.
	f.programs["slow.exe"] = fakeGuestProgram{running: true}
	if _, err := p.exec(context.Background(), f, dom, "k", "slow.exe", nil); err == nil {
		t.Fatal("slow again: want timeout")
	}
	f.statusErr = errors.New("guest agent is not connected")
	if _, err := p.exec(context.Background(), f, dom, "k", "slow.exe", nil); err == nil || !strings.HasPrefix(err.Error(), "previous run: ") {
		t.Errorf("status failure: err = %v", err)
	}
	if _, ok := p.pending["k"]; !ok {
		t.Error("pending PID dropped after a status failure")
	}
	f.statusErr = errors.New("guest agent command failed: unable to execute QEMU agent command 'guest-exec-status': PID lld does not exist")
	f.programs["slow.exe"] = fakeGuestProgram{out: "again"}
	if _, err := p.exec(context.Background(), f, dom, "k", "slow.exe", nil); err == nil {
		t.Error("unknown PID then status failure on the new run: want error")
	}
	f.statusErr = nil
	delete(p.pending, "k")

	// A status error mid-run keeps the PID pending.
	f.empty = "guest-exec-status"
	if _, err := p.exec(context.Background(), f, dom, "k2", "fps.exe", nil); err == nil {
		t.Error("empty status reply: want error")
	}
	if _, ok := p.pending["k2"]; !ok {
		t.Error("PID not kept after status error")
	}

	f.programs["fail.exe"] = fakeGuestProgram{exitCode: 1, stderr: strings.Repeat("x", 300)}
	f.empty = ""
	if _, err := p.exec(context.Background(), f, dom, "k3", "fail.exe", nil); err == nil || len(err.Error()) != len("exit code 1: ")+200 {
		t.Errorf("fail: err = %v", err)
	}
	if _, err := p.exec(context.Background(), f, dom, "k4", "missing.exe", nil); err == nil {
		t.Error("missing program: want error")
	}
}

func TestVMGuestExec_ContextCancelled(t *testing.T) {
	fastGuestExec(t)
	f := windowsGuest()
	p, _ := testPoller(f)
	dom := libvirt.Domain{Name: "Windows 11"}
	f.programs["slow.exe"] = fakeGuestProgram{running: true}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := p.exec(ctx, f, dom, "k", "slow.exe", nil); !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
	if _, ok := p.pending["k"]; ok {
		t.Error("PID recorded as pending after cancellation")
	}
	if len(f.pids) != 0 {
		t.Errorf("started %d guest commands after cancellation", len(f.pids))
	}
}

func TestUnknownGuestPID(t *testing.T) {
	for msg, want := range map[string]bool{
		"unable to execute QEMU agent command 'guest-exec-status': PID lld does not exist":  true,
		"unable to execute QEMU agent command 'guest-exec-status': Invalid parameter 'pid'": true,
		"Guest agent is not responding": false,
	} {
		if got := unknownGuestPID(errors.New(msg)); got != want {
			t.Errorf("unknownGuestPID(%q) = %v, want %v", msg, got, want)
		}
	}
}

func TestGuestExecStatusResult(t *testing.T) {
	zero, one := 0, 1
	ok := base64.StdEncoding.EncodeToString([]byte("out"))
	tests := []struct {
		name    string
		st      guestExecStatus
		want    string
		wantErr string
	}{
		{"success", guestExecStatus{ExitCode: &zero, OutData: ok}, "out", ""},
		{"signal", guestExecStatus{Signal: &one}, "", "terminated by signal 1"},
		{"no exit code", guestExecStatus{}, "", "no exit code"},
		{"truncated", guestExecStatus{ExitCode: &zero, OutTruncated: true}, "", "output truncated"},
		{"bad base64", guestExecStatus{ExitCode: &zero, OutData: "!!"}, "", "decode output"},
	}
	for _, tt := range tests {
		got, err := tt.st.result()
		if got != tt.want || (tt.wantErr == "") != (err == nil) || (err != nil && !strings.Contains(err.Error(), tt.wantErr)) {
			t.Errorf("%s: got %q, %v", tt.name, got, err)
		}
	}
}

func TestAgentCall(t *testing.T) {
	f := windowsGuest()
	dom := libvirt.Domain{Name: "Windows 11"}
	if err := agentCall(f, dom, "x", map[string]any{"bad": func() {}}, nil); err == nil {
		t.Error("unmarshalable args: want error")
	}
	f.empty = "guest-ping"
	if err := agentCall(f, dom, "guest-ping", nil, nil); err == nil {
		t.Error("empty reply: want error")
	}
	f.badReply = "guest-info"
	if err := agentCall(f, dom, "guest-info", nil, nil); err == nil {
		t.Error("bad reply: want error")
	}
}

func TestParseNvidiaSMI(t *testing.T) {
	out := "0, NVIDIA GeForce RTX 4090, 34, 12, 13, 24564, 63.29, [N/A], 2535\n" +
		"1, Odd, Name, [Not Supported], 0, -1, 100, 10, 20, 30\n" +
		"short, line\n" +
		"x, a, 1, 2, 3, 4, 5, 6, 7\n"
	gpus := parseNvidiaSMI(out)
	if len(gpus) != 2 {
		t.Fatalf("got %d GPUs, want 2: %+v", len(gpus), gpus)
	}
	if gpus[0].FanPercent != nil || *gpus[0].PowerWatts != 63.29 {
		t.Errorf("gpu 0 = %+v", gpus[0])
	}
	if g := gpus[1]; g.Name != "Odd,Name" || g.TemperatureC != nil || g.MemoryUsedBytes != nil || *g.ClockMHz != 30 {
		t.Errorf("gpu 1 = %+v", g)
	}
}

func TestParseGuestDisks(t *testing.T) {
	disks, err := parseGuestDisks("")
	if err != nil || disks == nil || len(disks) != 0 {
		t.Errorf("empty: %v %v", disks, err)
	}
	disks, err = parseGuestDisks(`[{"Model":"A","Temp":0,"TempMax":null,"Wear":0},{"Model":"B","Temp":40}]`)
	if err != nil || len(disks) != 2 {
		t.Fatalf("array: %v %v", disks, err)
	}
	if d := disks[0]; d.TemperatureC != nil || d.TemperatureMaxC != nil || d.WearPercent == nil || *d.WearPercent != 0 {
		t.Errorf("disk A = %+v", d)
	}
	if _, err := parseGuestDisks("{"); err == nil {
		t.Error("bad JSON: want error")
	}
}

func TestVMGuestPoller_StartStops(t *testing.T) {
	f := windowsGuest()
	f.agentDown = true
	p, sub := testPoller(f, fpsSensor)
	p.cfg.Interval = 1
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { p.Start(ctx); close(done) }()
	nextGuestUpdate(t, sub)
	nextGuestUpdate(t, sub)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return after cancel")
	}
}

func TestVMGuestLoopRecoversAndTicks(t *testing.T) {
	p, _ := testPoller(windowsGuest())
	ctx, cancel := context.WithCancel(context.Background())
	var mu sync.Mutex
	calls := 0
	done := make(chan struct{})
	go func() {
		p.loop(ctx, time.Millisecond, func() {
			mu.Lock()
			calls++
			n := calls
			mu.Unlock()
			if n == 3 {
				cancel()
			}
			panic("boom")
		})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("loop did not stop")
	}
}

type pipeDialer struct{ err error }

// Dial returns a connection on which libvirt never replies.
func (d pipeDialer) Dial() (net.Conn, error) {
	if d.err != nil {
		return nil, d.err
	}
	client, _ := net.Pipe()
	return client, nil
}

func useGuestDialer(t *testing.T, d socket.Dialer, limit time.Duration) {
	t.Helper()
	oldDialer, oldLimit := newGuestDialer, guestConnLimit
	newGuestDialer, guestConnLimit = func() socket.Dialer { return d }, limit
	t.Cleanup(func() { newGuestDialer, guestConnLimit = oldDialer, oldLimit })
}

func TestDialLibvirt(t *testing.T) {
	t.Run("connects", func(t *testing.T) {
		useGuestDialer(t, libvirttest.New(), time.Minute)
		conn, err := dialLibvirt(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if err := conn.Disconnect(); err != nil {
			t.Error(err)
		}
	})
	t.Run("stalled libvirt hits the limit", func(t *testing.T) {
		useGuestDialer(t, pipeDialer{}, 50*time.Millisecond)
		if _, err := dialLibvirt(context.Background()); err == nil {
			t.Error("want error")
		}
	})
	t.Run("cancelled", func(t *testing.T) {
		useGuestDialer(t, pipeDialer{}, time.Minute)
		ctx, cancel := context.WithCancel(context.Background())
		time.AfterFunc(50*time.Millisecond, cancel)
		if _, err := dialLibvirt(ctx); err == nil {
			t.Error("want error")
		}
	})
	t.Run("dial error", func(t *testing.T) {
		useGuestDialer(t, pipeDialer{err: errors.New("no socket")}, time.Minute)
		if _, err := dialLibvirt(context.Background()); err == nil {
			t.Error("want error")
		}
	})
}

func TestClosingDialerClosedBeforeDial(t *testing.T) {
	d := &closingDialer{Dialer: pipeDialer{}}
	d.close()
	if _, err := d.Dial(); err == nil {
		t.Error("want error after close")
	}
}

func strp(s string) *string { return &s }

func TestExpandJSONSensor(t *testing.T) {
	tests := []struct {
		name     string
		out      string
		reserved map[string]bool
		attrs    map[string]string
		want     []jsonSensorValue
		wantErr  bool
	}{
		{
			name: "frames example, keys sorted",
			out:  `{"avg":98.4,"low5":91,"low1":82,"worst":66,"graph":"82|10.4,10.0"}`,
			want: []jsonSensorValue{
				{"frames_avg", strp("98.4"), nil}, {"frames_graph", strp("82|10.4,10.0"), nil},
				{"frames_low1", strp("82"), nil}, {"frames_low5", strp("91"), nil}, {"frames_worst", strp("66"), nil},
			},
		},
		{
			name: "numbers without exponent noise",
			out:  `{"a":1e3,"b":0.5,"c":-2,"d":1.50,"e":12345678901234567890,"f":9007199254740993,"g":1.5E-3,"h":-1.25e1,"i":2e+2,"j":-0,"k":0.000,"l":123.456e-2,"m":1e-3}`,
			want: []jsonSensorValue{
				{"frames_a", strp("1000"), nil}, {"frames_b", strp("0.5"), nil}, {"frames_c", strp("-2"), nil},
				{"frames_d", strp("1.5"), nil}, {"frames_e", strp("12345678901234567890"), nil},
				{"frames_f", strp("9007199254740993"), nil}, {"frames_g", strp("0.0015"), nil}, {"frames_h", strp("-12.5"), nil},
				{"frames_i", strp("200"), nil}, {"frames_j", strp("0"), nil}, {"frames_k", strp("0"), nil},
				{"frames_l", strp("1.23456"), nil}, {"frames_m", strp("0.001"), nil},
			},
		},
		{
			name: "string, bool and null",
			out:  `{"s":" hi ","t":true,"f":false,"n":null}`,
			want: []jsonSensorValue{
				{"frames_f", strp("false"), nil}, {"frames_n", nil, nil}, {"frames_s", strp(" hi "), nil}, {"frames_t", strp("true"), nil},
			},
		},
		{
			name: "nested values skipped",
			out:  `{"obj":{"a":1},"arr":[1,2],"ok":1}`,
			want: []jsonSensorValue{{"frames_ok", strp("1"), nil}},
		},
		{
			name: "number out of range skipped",
			out:  `{"big":1e999,"small":1e-999,"huge":1e99999999999999999999,"ok":1}`,
			want: []jsonSensorValue{{"frames_ok", strp("1"), nil}},
		},
		{
			name:     "keys sanitised, empty key and name clashes skipped",
			out:      `{"Low 5%":1,"":2,"low_5_":3,"LOW_5_":4,"taken":5}`,
			reserved: map[string]bool{"frames_taken": true},
			want:     []jsonSensorValue{{"frames_low_5_", strp("4"), nil}},
		},
		{
			name: "key too long skipped",
			out:  `{"` + strings.Repeat("k", 70) + `":1,"ok":2}`,
			want: []jsonSensorValue{{"frames_ok", strp("2"), nil}},
		},
		{
			name:  "attributes attach to target and are not sensors",
			out:   `{"avg":98.4,"graph":"82|10","unit":"fps","gone":1}`,
			attrs: map[string]string{"graph": "avg", "unit": "avg", "miss": "avg", "gone": "nowhere"},
			want: []jsonSensorValue{
				{"frames_avg", strp("98.4"), map[string]*string{"graph": strp("82|10"), "unit": strp("fps"), "miss": nil}},
			},
		},
		{
			name:  "attribute values nested or null are nil",
			out:   `{"avg":1,"a":null,"b":{"x":1}}`,
			attrs: map[string]string{"a": "avg", "b": "avg"},
			want:  []jsonSensorValue{{"frames_avg", strp("1"), map[string]*string{"a": nil, "b": nil}}},
		},
		{name: "empty object", out: `{}`},
		{name: "invalid JSON", out: `{"a":`, wantErr: true},
		{name: "empty output", out: ``, wantErr: true},
		{name: "array", out: `[1,2]`, wantErr: true},
		{name: "scalar", out: `42`, wantErr: true},
		{name: "null", out: `null`, wantErr: true},
		{name: "trailing data", out: `{"a":1} {"b":2}`, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := expandJSONSensor("frames", tt.out, tt.reserved, tt.attrs)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
			for i := range got {
				w := tt.want[i]
				if got[i].name != w.name || (got[i].value == nil) != (w.value == nil) || (w.value != nil && *got[i].value != *w.value) {
					t.Errorf("[%d] got %s=%v, want %s=%v", i, got[i].name, got[i].value, w.name, w.value)
				}
				if !reflect.DeepEqual(got[i].attributes, w.attributes) {
					t.Errorf("[%d] attributes = %v, want %v", i, got[i].attributes, w.attributes)
				}
			}
		})
	}
}

var framesSensor = domain.VMGuestSensorConfig{VM: "Windows 11", Name: "frames", Command: []string{"frames.ps1"}, Interval: 5, JSON: true}

func sensorValues(infoSensors []dto.VMGuestSensor) map[string]string {
	m := make(map[string]string)
	for _, s := range infoSensors {
		if s.Value == nil {
			m[s.Name] = "<nil>"
		} else {
			m[s.Name] = *s.Value
		}
	}
	return m
}

func sensorNames(infoSensors []dto.VMGuestSensor) string {
	names := make([]string, len(infoSensors))
	for i, s := range infoSensors {
		names[i] = s.Name
	}
	return strings.Join(names, ",")
}

func TestVMGuestPoller_JSONSensor(t *testing.T) {
	fastGuestExec(t)
	f := windowsGuest()
	f.programs["frames.ps1"] = fakeGuestProgram{out: `{"worst":66,"avg":98.4,"n":null,"nested":{"a":1}}`}
	// A plain sensor before and a plain sensor named like a key after.
	after := domain.VMGuestSensorConfig{VM: "Windows 11", Name: "frames_worst", Command: []string{"fps.exe"}, Interval: 5}
	p, sub := testPoller(f, fpsSensor, framesSensor, after)

	if got := sensorNames(p.infos["Windows 11"].Sensors); got != "fps,frames_worst" {
		t.Errorf("before first run = %s", got)
	}
	p.runSensor(context.Background(), framesSensor)
	info := nextGuestUpdate(t, sub)
	if got := sensorNames(info.Sensors); got != "fps,frames_avg,frames_n,frames_worst" {
		t.Errorf("names = %s", got)
	}
	// frames_worst is a configured sensor, so the key does not shadow it.
	if v := sensorValues(info.Sensors); v["frames_avg"] != "98.4" || v["frames_n"] != "<nil>" || v["frames_worst"] != "<nil>" {
		t.Errorf("values = %v", v)
	}
	for _, s := range info.Sensors {
		if s.Name == "frames_avg" && (s.UpdatedAt == nil || s.Error != "") {
			t.Errorf("sensor = %+v", s)
		}
	}

	// A key that disappears from the output drops out; the others update.
	f.programs["frames.ps1"] = fakeGuestProgram{out: `{"avg":50,"extra":"x"}`}
	p.runSensor(context.Background(), framesSensor)
	info = nextGuestUpdate(t, sub)
	if got := sensorNames(info.Sensors); got != "fps,frames_avg,frames_extra,frames_worst" {
		t.Errorf("names = %s", got)
	}
	if v := sensorValues(info.Sensors); v["frames_avg"] != "50" || v["frames_extra"] != "x" {
		t.Errorf("values = %v", v)
	}
}

func TestVMGuestPoller_JSONSensorAttributes(t *testing.T) {
	fastGuestExec(t)
	f := windowsGuest()
	f.programs["frames.ps1"] = fakeGuestProgram{out: `{"avg":98.4,"graph":"82|10"}`}
	s := framesSensor
	s.Attributes = map[string]string{"graph": "avg"}
	p, sub := testPoller(f, s)

	p.runSensor(context.Background(), s)
	info := nextGuestUpdate(t, sub)
	if got := sensorNames(info.Sensors); got != "frames_avg" {
		t.Fatalf("names = %s", got)
	}
	if a := info.Sensors[0].Attributes; len(a) != 1 || a["graph"] == nil || *a["graph"] != "82|10" {
		t.Errorf("attributes = %v", a)
	}

	// A failed run keeps the attribute with a nil value.
	f.programs["frames.ps1"] = fakeGuestProgram{exitCode: 1}
	p.runSensor(context.Background(), s)
	info = nextGuestUpdate(t, sub)
	if a, ok := info.Sensors[0].Attributes["graph"]; !ok || a != nil || info.Sensors[0].Value != nil {
		t.Errorf("sensor = %+v", info.Sensors[0])
	}
}

func TestVMGuestPoller_JSONSensorAttributesVMOff(t *testing.T) {
	fastGuestExec(t)
	f := windowsGuest()
	f.programs["frames.ps1"] = fakeGuestProgram{out: `{"avg":98.4,"graph":"82|10"}`}
	s := framesSensor
	s.Attributes = map[string]string{"graph": "avg"}
	p, sub := testPoller(f, s)
	p.runSensor(context.Background(), s)
	before := nextGuestUpdate(t, sub)

	f.states["Windows 11"] = libvirt.DomainShutoff
	p.updateAll(context.Background())
	after := nextGuestUpdate(t, sub)
	if a, ok := after.Sensors[0].Attributes["graph"]; !ok || a != nil {
		t.Errorf("attributes after shutdown = %v", after.Sensors[0].Attributes)
	}
	if a := before.Sensors[0].Attributes["graph"]; a == nil || *a != "82|10" {
		t.Errorf("earlier snapshot changed: %v", before.Sensors[0].Attributes)
	}
}

func TestVMGuestPoller_JSONSensorFailureKeepsKeys(t *testing.T) {
	fastGuestExec(t)
	for name, prog := range map[string]fakeGuestProgram{
		"invalid JSON": {out: `not json`},
		"array":        {out: `[1]`},
		"exit code":    {exitCode: 1},
	} {
		t.Run(name, func(t *testing.T) {
			f := windowsGuest()
			f.programs["frames.ps1"] = fakeGuestProgram{out: `{"avg":98.4,"low1":82}`}
			p, sub := testPoller(f, framesSensor)
			p.runSensor(context.Background(), framesSensor)
			nextGuestUpdate(t, sub)

			f.programs["frames.ps1"] = prog
			p.runSensor(context.Background(), framesSensor)
			info := nextGuestUpdate(t, sub)
			if got := sensorNames(info.Sensors); got != "frames_avg,frames_low1" {
				t.Fatalf("names = %s", got)
			}
			for _, s := range info.Sensors {
				if s.Value != nil || s.Error == "" || s.UpdatedAt == nil {
					t.Errorf("sensor = %+v", s)
				}
			}

			// A good run brings the values back and clears the error.
			f.programs["frames.ps1"] = fakeGuestProgram{out: `{"avg":1,"low1":2}`}
			p.runSensor(context.Background(), framesSensor)
			for _, s := range nextGuestUpdate(t, sub).Sensors {
				if s.Value == nil || s.Error != "" {
					t.Errorf("sensor = %+v", s)
				}
			}
		})
	}
}

func TestVMGuestPoller_JSONSensorVMOff(t *testing.T) {
	fastGuestExec(t)
	f := windowsGuest()
	f.programs["frames.ps1"] = fakeGuestProgram{out: `{"avg":98.4,"low1":82}`}
	p, sub := testPoller(f, framesSensor)
	p.runSensor(context.Background(), framesSensor)
	nextGuestUpdate(t, sub)

	f.states["Windows 11"] = libvirt.DomainShutoff
	p.updateAll(context.Background())
	info := nextGuestUpdate(t, sub)
	if got := sensorNames(info.Sensors); got != "frames_avg,frames_low1" {
		t.Fatalf("names after update = %s", got)
	}
	p.runSensor(context.Background(), framesSensor)
	info = nextGuestUpdate(t, sub)
	if got := sensorNames(info.Sensors); got != "frames_avg,frames_low1" {
		t.Fatalf("names = %s", got)
	}
	for _, s := range info.Sensors {
		if s.Value != nil || s.Error != "" {
			t.Errorf("sensor = %+v", s)
		}
	}
}

func TestVMGuestPoller_JSONSensorOtherVMUntouched(t *testing.T) {
	fastGuestExec(t)
	f := windowsGuest()
	f.programs["frames.ps1"] = fakeGuestProgram{out: `{"avg":98.4}`}
	f.states["Linux"] = libvirt.DomainRunning
	hub := domain.NewEventBus(100)
	other := domain.VMGuestSensorConfig{VM: "Linux", Name: "frames", Command: []string{"frames.ps1"}, Interval: 5, JSON: true}
	p := newVMGuestPoller(domain.VMGuestConfig{VMs: []string{"Windows 11", "Linux"}, Interval: 30, Sensors: []domain.VMGuestSensorConfig{framesSensor, other}}, hub)
	p.dial = func(context.Context) (guestConn, error) { return f, nil }
	p.runSensor(context.Background(), framesSensor)
	p.runSensor(context.Background(), other)
	for _, vm := range []string{"Windows 11", "Linux"} {
		if got := sensorNames(p.infos[vm].Sensors); got != "frames_avg" {
			t.Errorf("%s sensors = %s", vm, got)
		}
	}
}

func TestVMGuestPoller_JSONSensorNamesNotShared(t *testing.T) {
	fastGuestExec(t)
	gpu := domain.VMGuestSensorConfig{VM: "Windows 11", Name: "gpu", Command: []string{"gpu.ps1"}, Interval: 5, JSON: true}
	gpuCore := domain.VMGuestSensorConfig{VM: "Windows 11", Name: "gpu_core", Command: []string{"core.ps1"}, Interval: 5, JSON: true}
	for _, order := range [][]domain.VMGuestSensorConfig{{gpu, gpuCore}, {gpuCore, gpu}} {
		f := windowsGuest()
		f.programs["gpu.ps1"] = fakeGuestProgram{out: `{"core_temp":80,"load":5}`}
		f.programs["core.ps1"] = fakeGuestProgram{out: `{"temp":40}`}
		p, sub := testPoller(f, gpu, gpuCore)
		for _, s := range order {
			p.runSensor(context.Background(), s)
			nextGuestUpdate(t, sub)
		}
		// The sensor that ran first owns gpu_core_temp; the other cannot touch it.
		first := order[0]
		want := "40"
		if first.Name == "gpu" {
			want = "80"
		}
		info := p.infos["Windows 11"]
		count := 0
		for _, s := range info.Sensors {
			if s.Name == "gpu_core_temp" {
				count++
				if s.Value == nil || *s.Value != want {
					t.Errorf("first %s: gpu_core_temp = %v, want %s", first.Name, s.Value, want)
				}
			}
		}
		if count != 1 {
			t.Errorf("first %s: gpu_core_temp listed %d times", first.Name, count)
		}

		// A failing run of the other sensor leaves the owner's reading alone.
		other := order[1]
		f.programs[other.Command[0]] = fakeGuestProgram{out: `nope`}
		p.runSensor(context.Background(), other)
		for _, s := range nextGuestUpdate(t, sub).Sensors {
			if s.Name == "gpu_core_temp" && (s.Value == nil || *s.Value != want) {
				t.Errorf("first %s: after failure gpu_core_temp = %v, want %s", first.Name, s.Value, want)
			}
		}
	}
}

func TestVMGuestPoller_JSONSensorFailsBeforeFirstGoodRun(t *testing.T) {
	fastGuestExec(t)
	f := windowsGuest()
	f.programs["frames.ps1"] = fakeGuestProgram{out: `oops`}
	p, sub := testPoller(f, framesSensor)
	for range 2 {
		p.runSensor(context.Background(), framesSensor)
		if info := nextGuestUpdate(t, sub); len(info.Sensors) != 0 {
			t.Errorf("sensors = %+v, want none", info.Sensors)
		}
	}
	if !p.jsonFailing["Windows 11/frames"] {
		t.Error("failure not recorded")
	}
	f.programs["frames.ps1"] = fakeGuestProgram{out: `{"avg":1}`}
	p.runSensor(context.Background(), framesSensor)
	if got := sensorNames(nextGuestUpdate(t, sub).Sensors); got != "frames_avg" || p.jsonFailing["Windows 11/frames"] {
		t.Errorf("after recovery: sensors = %s, failing = %v", got, p.jsonFailing["Windows 11/frames"])
	}
}

// A sensor result that arrives after the VM was set offline is dropped, so it
// cannot restore readings the offline update cleared.
func TestVMGuestPoller_SensorResultAfterOfflineDropped(t *testing.T) {
	fastGuestExec(t)
	f := windowsGuest()
	f.programs["frames.ps1"] = fakeGuestProgram{out: `{"avg":98.4,"graph":"82|10"}`}
	s := framesSensor
	s.Attributes = map[string]string{"graph": "avg"}
	p, sub := testPoller(f, s, fpsSensor)
	p.runSensor(context.Background(), s)
	nextGuestUpdate(t, sub)

	// The VM goes offline while each run is under way.
	p.dial = func(context.Context) (guestConn, error) {
		p.setOffline("Windows 11", false, "")
		return f, nil
	}
	info := p.infos["Windows 11"]
	for _, run := range []domain.VMGuestSensorConfig{s, fpsSensor} {
		p.runSensor(context.Background(), run)
		for _, sensor := range info.Sensors {
			if sensor.Value != nil {
				t.Errorf("after %s ran: sensor %s = %q", run.Name, sensor.Name, *sensor.Value)
			}
			for k, v := range sensor.Attributes {
				if v != nil {
					t.Errorf("after %s ran: attribute %s = %q", run.Name, k, *v)
				}
			}
		}
	}
	if info.AgentConnected {
		t.Error("agent connected after the VM went offline")
	}
}
