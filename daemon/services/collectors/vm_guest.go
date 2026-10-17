package collectors

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/digitalocean/go-libvirt"
	"github.com/digitalocean/go-libvirt/socket"
	"github.com/digitalocean/go-libvirt/socket/dialers"

	"github.com/tinkerologie/unraid-management-agent/daemon/constants"
	"github.com/tinkerologie/unraid-management-agent/daemon/domain"
	"github.com/tinkerologie/unraid-management-agent/daemon/dto"
	"github.com/tinkerologie/unraid-management-agent/daemon/logger"
)

// Commands run inside the guest for the full update. nvidia-smi reports memory
// in MiB. The PowerShell query lists each physical disk with its reliability
// counters.
const (
	guestNvidiaSMIWindows = `C:\Windows\System32\nvidia-smi.exe`
	guestNvidiaSMILinux   = "nvidia-smi"
	guestNvidiaSMIQuery   = "--query-gpu=index,name,temperature.gpu,utilization.gpu,memory.used,memory.total,power.draw,fan.speed,clocks.gr"
	guestPowerShell       = `C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe`
	guestDiskScript       = `Get-PhysicalDisk | ForEach-Object { $r = $_ | Get-StorageReliabilityCounter; ` +
		`[pscustomobject]@{Model=$_.FriendlyName; Serial=$_.SerialNumber; Bus=[string]$_.BusType; ` +
		`Temp=$r.Temperature; TempMax=$r.TemperatureMax; Wear=$r.Wear} } | ConvertTo-Json -Compress`
)

var (
	// guestAgentTimeout is how long libvirt waits for one guest agent reply.
	guestAgentTimeout int32 = 5
	// guestExecTimeout is how long a command run in the guest may take.
	guestExecTimeout = 30 * time.Second
	// guestExecPoll is the wait between guest-exec-status calls.
	guestExecPoll = 250 * time.Millisecond
	// guestConnLimit bounds one full update or sensor run. When it passes,
	// or the poller stops, the libvirt socket is closed so a stalled RPC
	// returns instead of blocking forever.
	guestConnLimit = 2 * time.Minute
	// newGuestDialer opens the libvirt socket.
	newGuestDialer = func() socket.Dialer { return dialers.NewLocal() }
)

// guestConn is the part of *libvirt.Libvirt the guest poller uses.
type guestConn interface {
	DomainLookupByName(name string) (libvirt.Domain, error)
	DomainGetState(dom libvirt.Domain, flags uint32) (int32, int32, error)
	QEMUDomainAgentCommand(dom libvirt.Domain, cmd string, timeout int32, flags uint32) (libvirt.OptString, error)
	Disconnect() error
}

// closingDialer remembers the socket it opens so it can be closed directly.
// Libvirt.Disconnect sends an RPC first, which would stall as well.
type closingDialer struct {
	socket.Dialer
	mu     sync.Mutex
	conn   net.Conn
	closed bool
}

func (d *closingDialer) Dial() (net.Conn, error) {
	conn, err := d.Dialer.Dial()
	d.mu.Lock()
	defer d.mu.Unlock()
	if err == nil && d.closed {
		_ = conn.Close()
		return nil, errors.New("libvirt connection cancelled")
	}
	d.conn = conn
	return conn, err
}

func (d *closingDialer) close() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.closed = true
	if d.conn != nil {
		_ = d.conn.Close()
	}
}

// watchedConn is a libvirt connection whose socket is closed when ctx is
// cancelled or guestConnLimit passes.
type watchedConn struct {
	*libvirt.Libvirt
	done chan struct{}
}

// Disconnect stops the watchdog only after the close RPC returns, since that
// RPC can stall too.
func (c *watchedConn) Disconnect() error {
	defer close(c.done)
	return c.Libvirt.Disconnect()
}

func dialLibvirt(ctx context.Context) (guestConn, error) {
	d := &closingDialer{Dialer: newGuestDialer()}
	c := &watchedConn{Libvirt: libvirt.NewWithDialer(d), done: make(chan struct{})}
	timer := time.NewTimer(guestConnLimit)
	go func() {
		defer timer.Stop()
		select {
		case <-c.done:
		case <-ctx.Done():
			d.close()
		case <-timer.C:
			d.close()
		}
	}()
	if err := c.ConnectToURI(libvirt.QEMUSystem); err != nil {
		close(c.done)
		return nil, err
	}
	return c, nil
}

// vmGuestPoller reads data from inside VMs through the QEMU guest agent: a
// full update (host name, OS, filesystems, GPUs, disks) every interval, and
// each custom sensor on its own interval. Every change publishes a snapshot of
// all configured VMs.
type vmGuestPoller struct {
	cfg  domain.VMGuestConfig
	hub  *domain.EventBus
	dial func(context.Context) (guestConn, error)

	// pubMu keeps snapshots in the order they are published.
	pubMu sync.Mutex
	mu    sync.Mutex
	infos map[string]*dto.VMGuestInfo
	// pending holds the guest PID of a command that has not finished, by
	// VM and command, so it is not started again until it is reaped.
	pending map[string]int
	// jsonNames holds the sensor names expanded from each JSON sensor's
	// output, by VM and sensor, so they stay listed when a run fails.
	jsonNames map[string][]string
	// jsonFailing marks the JSON sensors whose last run failed, by VM and
	// sensor, so a failure is logged once rather than on every run.
	jsonFailing map[string]bool
	// offlineGen counts, by VM, the times the VM was set offline. A sensor
	// run drops its result if the count changed while it ran, so a reply
	// that arrives late cannot restore readings after the VM went offline.
	offlineGen map[string]uint64
}

func newVMGuestPoller(cfg domain.VMGuestConfig, hub *domain.EventBus) *vmGuestPoller {
	p := &vmGuestPoller{cfg: cfg, hub: hub, dial: dialLibvirt, infos: make(map[string]*dto.VMGuestInfo), pending: make(map[string]int), jsonNames: make(map[string][]string), jsonFailing: make(map[string]bool), offlineGen: make(map[string]uint64)}
	for _, vm := range cfg.VMs {
		p.infos[vm] = &dto.VMGuestInfo{
			VMName:      vm,
			Filesystems: []dto.VMGuestFilesystem{},
			GPUs:        []dto.VMGuestGPU{},
			Disks:       []dto.VMGuestDisk{},
			Sensors:     []dto.VMGuestSensor{},
		}
	}
	for _, s := range cfg.Sensors {
		if s.JSON {
			continue
		}
		info := p.infos[s.VM]
		info.Sensors = append(info.Sensors, dto.VMGuestSensor{Name: s.Name})
	}
	return p
}

// Start runs the full update and every sensor until ctx is cancelled.
func (p *vmGuestPoller) Start(ctx context.Context) {
	logger.Info("Starting VM guest agent reads for %d VM(s), %d sensor(s)", len(p.cfg.VMs), len(p.cfg.Sensors))
	var wg sync.WaitGroup
	for _, s := range p.cfg.Sensors {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p.loop(ctx, time.Duration(s.Interval)*time.Second, func() { p.runSensor(ctx, s) })
		}()
	}
	p.loop(ctx, time.Duration(p.cfg.Interval)*time.Second, func() { p.updateAll(ctx) })
	wg.Wait()
}

// loop calls fn now and then every interval, recovering from panics.
func (p *vmGuestPoller) loop(ctx context.Context, interval time.Duration, fn func()) {
	run := func() {
		defer func() {
			if r := recover(); r != nil {
				logger.LogPanicWithStack("VM guest agent", r)
			}
		}()
		fn()
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
		}
	}
}

// publish sends a copy of every VM's current data.
func (p *vmGuestPoller) publish() {
	p.pubMu.Lock()
	defer p.pubMu.Unlock()
	p.mu.Lock()
	out := make([]*dto.VMGuestInfo, 0, len(p.cfg.VMs))
	for _, vm := range p.cfg.VMs {
		info := *p.infos[vm]
		info.Filesystems = append([]dto.VMGuestFilesystem{}, info.Filesystems...)
		info.GPUs = append([]dto.VMGuestGPU{}, info.GPUs...)
		info.Disks = append([]dto.VMGuestDisk{}, info.Disks...)
		info.Sensors = append([]dto.VMGuestSensor{}, info.Sensors...)
		out = append(out, &info)
	}
	p.mu.Unlock()
	domain.Publish(p.hub, constants.TopicVMGuestUpdate, out)
}

// updateAll runs the full update for every VM and publishes once. Each VM
// gets its own connection, so a slow VM cannot use up another's time limit.
func (p *vmGuestPoller) updateAll(ctx context.Context) {
	for _, vm := range p.cfg.VMs {
		conn, err := p.dial(ctx)
		if err != nil {
			logger.Debug("VM guest agent: libvirt connection failed: %v", err)
			p.setOffline(vm, false, "libvirt connection failed")
			continue
		}
		p.updateVM(ctx, conn, vm)
		_ = conn.Disconnect()
	}
	p.publish()
}

// runningDomain looks up a VM and reports whether it is running.
func runningDomain(conn guestConn, vm string) (libvirt.Domain, bool, error) {
	dom, err := conn.DomainLookupByName(vm)
	if err != nil {
		return dom, false, fmt.Errorf("VM not found: %w", err)
	}
	state, _, err := conn.DomainGetState(dom, 0)
	if err != nil {
		return dom, false, err
	}
	return dom, libvirt.DomainState(state) == libvirt.DomainRunning, nil
}

func (p *vmGuestPoller) updateVM(ctx context.Context, conn guestConn, vm string) {
	dom, running, err := runningDomain(conn, vm)
	if err != nil || !running {
		msg := ""
		if err != nil {
			msg = err.Error()
		}
		p.setOffline(vm, false, msg)
		return
	}
	if err := agentCall(conn, dom, "guest-ping", nil, nil); err != nil {
		p.setOffline(vm, true, "guest agent not responding")
		return
	}

	var host struct {
		HostName string `json:"host-name"`
	}
	var osInfo struct {
		ID         string `json:"id"`
		PrettyName string `json:"pretty-name"`
	}
	_ = agentCall(conn, dom, "guest-get-host-name", nil, &host)
	_ = agentCall(conn, dom, "guest-get-osinfo", nil, &osInfo)

	var fsList []guestFSInfo
	fsErr := agentCall(conn, dom, "guest-get-fsinfo", nil, &fsList)

	smi := guestNvidiaSMILinux
	if osInfo.ID == "mswindows" {
		smi = guestNvidiaSMIWindows
	}
	smiOut, gpuErr := p.exec(ctx, conn, dom, vm+"/nvidia-smi", smi, []string{guestNvidiaSMIQuery, "--format=csv,noheader,nounits"})
	var gpus []dto.VMGuestGPU
	if gpuErr == nil {
		gpus = parseNvidiaSMI(smiOut)
	} else {
		logger.Debug("VM guest agent: %s: nvidia-smi: %v", vm, gpuErr)
	}

	var disks []dto.VMGuestDisk
	var diskErr error
	if osInfo.ID == "mswindows" {
		var out string
		out, diskErr = p.exec(ctx, conn, dom, vm+"/disks", guestPowerShell, []string{"-NoProfile", "-NonInteractive", "-Command", guestDiskScript})
		if diskErr == nil {
			disks, diskErr = parseGuestDisks(out)
		}
		if diskErr != nil {
			logger.Debug("VM guest agent: %s: disk query: %v", vm, diskErr)
		}
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	info := p.infos[vm]
	info.Running = true
	info.AgentConnected = true
	info.Error = ""
	info.Hostname = host.HostName
	info.OSName = osInfo.PrettyName
	info.Timestamp = time.Now()
	if fsErr == nil {
		info.Filesystems = filesystemsFromFSInfo(fsList)
	} else {
		info.Filesystems = clearFilesystems(info.Filesystems)
	}
	if gpuErr == nil {
		info.GPUs = gpus
	} else {
		info.GPUs = clearGPUs(info.GPUs)
	}
	if diskErr == nil && disks != nil {
		info.Disks = disks
	} else {
		info.Disks = clearDisks(info.Disks)
	}
}

// nilAttributes returns a new map with the keys of attributes and nil values,
// so a map already published in a snapshot is not changed.
func nilAttributes(attributes map[string]*string) map[string]*string {
	if attributes == nil {
		return nil
	}
	attrs := make(map[string]*string, len(attributes))
	for k := range attributes {
		attrs[k] = nil
	}
	return attrs
}

// setOffline keeps the VM's GPUs, disks, filesystems and sensors with every
// reading set to null.
func (p *vmGuestPoller) setOffline(vm string, running bool, msg string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.offlineGen[vm]++
	info := p.infos[vm]
	info.Running = running
	info.AgentConnected = false
	info.Error = msg
	info.Timestamp = time.Now()
	info.Filesystems = clearFilesystems(info.Filesystems)
	info.GPUs = clearGPUs(info.GPUs)
	info.Disks = clearDisks(info.Disks)
	for i := range info.Sensors {
		info.Sensors[i].Value = nil
		info.Sensors[i].Error = ""
		info.Sensors[i].Attributes = nilAttributes(info.Sensors[i].Attributes)
	}
}

// runSensor runs one custom sensor command and publishes the result.
func (p *vmGuestPoller) runSensor(ctx context.Context, s domain.VMGuestSensorConfig) {
	p.mu.Lock()
	gen := p.offlineGen[s.VM]
	p.mu.Unlock()

	var out *string
	errMsg := ""
	conn, err := p.dial(ctx)
	if err != nil {
		errMsg = "libvirt connection failed"
	} else {
		defer conn.Disconnect() //nolint:errcheck
		dom, running, err := runningDomain(conn, s.VM)
		switch {
		case err != nil:
			errMsg = err.Error()
		case running:
			raw, err := p.exec(ctx, conn, dom, s.VM+"/sensor/"+s.Name, s.Command[0], s.Command[1:])
			if err != nil {
				errMsg = err.Error()
				logger.Debug("VM guest agent: %s: sensor %s: %v", s.VM, s.Name, err)
			} else {
				v := strings.TrimSpace(raw)
				out = &v
			}
		}
	}

	now := time.Now()
	p.mu.Lock()
	if p.offlineGen[s.VM] != gen {
		// The VM went offline while this ran; its readings stay cleared.
		p.mu.Unlock()
		return
	}
	if s.JSON {
		p.setJSONSensors(s, out, errMsg, now)
	} else {
		for i := range p.infos[s.VM].Sensors {
			sensor := &p.infos[s.VM].Sensors[i]
			if sensor.Name == s.Name {
				sensor.Value = out
				sensor.Error = errMsg
				if out != nil {
					sensor.UpdatedAt = &now
				}
			}
		}
	}
	p.mu.Unlock()
	p.publish()
}

// setJSONSensors stores the result of a JSON sensor run and rebuilds the VM's
// sensor list in config order, with each JSON sensor's keys sorted. A run that
// failed, or whose output is not a flat JSON object, sets every key seen so
// far to null. The caller holds p.mu.
func (p *vmGuestPoller) setJSONSensors(s domain.VMGuestSensorConfig, out *string, errMsg string, now time.Time) {
	key := s.VM + "/" + s.Name
	old := make(map[string]dto.VMGuestSensor)
	for _, sensor := range p.infos[s.VM].Sensors {
		old[sensor.Name] = sensor
	}

	var fresh []jsonSensorValue
	if out != nil {
		reserved := make(map[string]bool)
		for _, c := range p.cfg.Sensors {
			if c.VM == s.VM && c.Name != s.Name {
				reserved[c.Name] = true
				for _, n := range p.jsonNames[c.VM+"/"+c.Name] {
					reserved[n] = true
				}
			}
		}
		var err error
		if fresh, err = expandJSONSensor(s.Name, *out, reserved, s.Attributes); err != nil {
			errMsg = err.Error()
		} else {
			names := make([]string, len(fresh))
			for i, v := range fresh {
				names[i] = v.name
			}
			p.jsonNames[key] = names
		}
	}
	// A JSON sensor has no entry until its first good run, so a failure
	// before that is only visible in the log.
	if failing := errMsg != ""; out != nil || failing {
		if failing && !p.jsonFailing[key] {
			logger.Warning("VM guest agent: %s: sensor %s: %s", s.VM, s.Name, errMsg)
		} else if !failing && p.jsonFailing[key] {
			logger.Info("VM guest agent: %s: sensor %s: recovered", s.VM, s.Name)
		}
		p.jsonFailing[key] = failing
	}
	for _, v := range fresh {
		old[v.name] = dto.VMGuestSensor{Name: v.name, Value: v.value, Attributes: v.attributes, UpdatedAt: &now}
	}
	for _, name := range p.jsonNames[key] {
		if out == nil || errMsg != "" {
			sensor := old[name]
			sensor.Value, sensor.Error = nil, errMsg
			sensor.Attributes = nilAttributes(sensor.Attributes)
			old[name] = sensor
		}
	}

	list := []dto.VMGuestSensor{}
	for _, c := range p.cfg.Sensors {
		if c.VM != s.VM {
			continue
		}
		if !c.JSON {
			list = append(list, old[c.Name])
			continue
		}
		for _, name := range p.jsonNames[c.VM+"/"+c.Name] {
			list = append(list, old[name])
		}
	}
	p.infos[s.VM].Sensors = list
}

// plainDecimal writes a JSON number literal in plain decimal notation without
// rounding it: no exponent and no trailing zeros in the fraction. It reports
// false for an exponent beyond +-400.
func plainDecimal(lit string) (string, bool) {
	neg := strings.HasPrefix(lit, "-")
	mant, expStr, _ := strings.Cut(strings.ToLower(strings.TrimPrefix(lit, "-")), "e")
	exp := 0
	if expStr != "" {
		var err error
		if exp, err = strconv.Atoi(expStr); err != nil || exp > 400 || exp < -400 {
			return "", false
		}
	}
	intDigits, fracDigits, _ := strings.Cut(mant, ".")
	digits := intDigits + fracDigits
	point := len(intDigits) + exp
	if point < 0 {
		digits = strings.Repeat("0", -point) + digits
		point = 0
	} else if point > len(digits) {
		digits += strings.Repeat("0", point-len(digits))
	}
	res := strings.TrimLeft(digits[:point], "0")
	if res == "" {
		res = "0"
	}
	if frac := strings.TrimRight(digits[point:], "0"); frac != "" {
		res += "." + frac
	}
	if neg && res != "0" {
		res = "-" + res
	}
	return res, true
}

// jsonSensorValue is one key of a JSON sensor's output.
type jsonSensorValue struct {
	name       string
	value      *string
	attributes map[string]*string
}

var vmGuestSensorNamePattern = regexp.MustCompile(`^[a-z0-9_]{1,64}$`)

var jsonSensorKeyRegex = regexp.MustCompile(`[^a-z0-9_]`)

// expandJSONSensor turns a flat JSON object into one value per key, named
// <name>_<key> and sorted by key. Keys are lowercased and any other character
// than a letter, digit or underscore becomes an underscore. Keys whose name is
// empty, longer than 64 characters, already used or in reserved are skipped, as
// are nested objects and arrays. Numbers are written in plain decimal, strings
// as they are, booleans as true or false, and null as a nil value.
//
// attributes maps a key to the key of the value it becomes an attribute of. An
// attribute key is not a value of its own, and is nil on its target when it is
// missing or nested. When the target is missing the attribute is dropped.
func expandJSONSensor(name, out string, reserved map[string]bool, attributes map[string]string) ([]jsonSensorValue, error) {
	dec := json.NewDecoder(strings.NewReader(out))
	dec.UseNumber()
	var obj map[string]any
	if err := dec.Decode(&obj); err != nil || obj == nil {
		return nil, errors.New("output is not a JSON object")
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, errors.New("output is not a JSON object")
	}

	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	scalar := func(k string) (*string, bool) {
		switch v := obj[k].(type) {
		case nil:
			return nil, true
		case json.Number:
			plain, ok := plainDecimal(v.String())
			if !ok {
				logger.Debug("VM guest agent: sensor %s: skipping key %q: number %q out of range", name, k, v)
				return nil, false
			}
			return &plain, true
		case string:
			return &v, true
		case bool:
			s := strconv.FormatBool(v)
			return &s, true
		default:
			logger.Debug("VM guest agent: sensor %s: skipping key %q: nested value", name, k)
			return nil, false
		}
	}

	attrs := make(map[string]map[string]*string)
	missing := make(map[string]bool)
	for attr, target := range attributes {
		if attrs[target] == nil {
			attrs[target] = make(map[string]*string)
		}
		attrs[target][attr] = nil
		missing[target] = true
	}

	var values []jsonSensorValue
	used := make(map[string]bool)
	for _, k := range keys {
		suffix := jsonSensorKeyRegex.ReplaceAllString(strings.ToLower(k), "_")
		if target, ok := attributes[suffix]; ok {
			if v, ok := scalar(k); ok {
				attrs[target][suffix] = v
			}
			continue
		}
		full := name + "_" + suffix
		switch {
		case suffix == "" || !vmGuestSensorNamePattern.MatchString(full):
			logger.Warning("VM guest agent: sensor %s: skipping key %q: name %q is not 1-64 characters", name, k, full)
			continue
		case used[full] || reserved[full]:
			logger.Warning("VM guest agent: sensor %s: skipping key %q: name %q is already used", name, k, full)
			continue
		}
		value, ok := scalar(k)
		if !ok {
			continue
		}
		used[full] = true
		values = append(values, jsonSensorValue{name: full, value: value, attributes: attrs[suffix]})
		delete(missing, suffix)
	}
	for target := range missing {
		logger.Debug("VM guest agent: sensor %s: dropping attributes: target key %q is missing", name, target)
	}
	return values, nil
}

// agentCall runs a guest agent command and decodes its "return" value into
// out, which may be nil.
func agentCall(conn guestConn, dom libvirt.Domain, cmd string, args any, out any) error {
	req := map[string]any{"execute": cmd}
	if args != nil {
		req["arguments"] = args
	}
	body, err := json.Marshal(req)
	if err != nil {
		return err
	}
	res, err := conn.QEMUDomainAgentCommand(dom, string(body), guestAgentTimeout, 0)
	if err != nil {
		return err
	}
	if len(res) == 0 {
		return errors.New("empty guest agent reply")
	}
	var reply struct {
		Return json.RawMessage `json:"return"`
	}
	if err := json.Unmarshal([]byte(res[0]), &reply); err != nil {
		return fmt.Errorf("decode guest agent reply: %w", err)
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(reply.Return, out)
}

// exec runs a program inside the guest without a shell and returns its
// standard output. A command that did not finish last time is not started
// again until the guest reports it exited, which also releases its status.
// Polling stops as soon as ctx is cancelled.
func (p *vmGuestPoller) exec(ctx context.Context, conn guestConn, dom libvirt.Domain, key, path string, args []string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	p.mu.Lock()
	pid, busy := p.pending[key]
	p.mu.Unlock()
	if busy {
		st, err := execStatus(conn, dom, pid)
		switch {
		case err != nil && !unknownGuestPID(err):
			return "", fmt.Errorf("previous run: %w", err)
		case err == nil && !st.Exited:
			return "", errors.New("previous run is still running")
		}
		// Exited (and now reaped), or unknown to a guest that restarted.
		p.mu.Lock()
		delete(p.pending, key)
		p.mu.Unlock()
	}

	var started struct {
		PID int `json:"pid"`
	}
	execArgs := map[string]any{"path": path, "arg": args, "capture-output": true}
	if err := agentCall(conn, dom, "guest-exec", execArgs, &started); err != nil {
		return "", err
	}
	deadline := time.Now().Add(guestExecTimeout)
	for {
		st, err := execStatus(conn, dom, started.PID)
		if err == nil && st.Exited {
			return st.result()
		}
		if err == nil && time.Now().After(deadline) {
			err = fmt.Errorf("timed out after %v", guestExecTimeout)
		}
		if err != nil {
			p.mu.Lock()
			p.pending[key] = started.PID
			p.mu.Unlock()
			return "", err
		}
		// Cancellation leaves the PID unrecorded: the poller is shutting down.
		timer := time.NewTimer(guestExecPoll)
		select {
		case <-ctx.Done():
			timer.Stop()
			return "", ctx.Err()
		case <-timer.C:
		}
	}
}

// unknownGuestPID reports whether guest-exec-status failed because the guest
// agent does not know the PID, as after a guest restart. qemu-ga says "PID n
// does not exist" (the Windows build prints "PID lld does not exist"); older
// versions say "Invalid parameter 'pid'".
func unknownGuestPID(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "does not exist") || strings.Contains(msg, "Invalid parameter 'pid'")
}

// guestExecStatus is the reply to guest-exec-status. A process that ended
// abnormally has signal set and no exitcode.
type guestExecStatus struct {
	Exited       bool   `json:"exited"`
	ExitCode     *int   `json:"exitcode"`
	Signal       *int   `json:"signal"`
	OutData      string `json:"out-data"`
	ErrData      string `json:"err-data"`
	OutTruncated bool   `json:"out-truncated"`
}

func execStatus(conn guestConn, dom libvirt.Domain, pid int) (guestExecStatus, error) {
	var st guestExecStatus
	err := agentCall(conn, dom, "guest-exec-status", map[string]int{"pid": pid}, &st)
	return st, err
}

// result returns the output of an exited process, or an error unless it
// exited with code 0 and its output is complete.
func (st guestExecStatus) result() (string, error) {
	switch {
	case st.Signal != nil:
		return "", fmt.Errorf("terminated by signal %d", *st.Signal)
	case st.ExitCode == nil:
		return "", errors.New("no exit code")
	case *st.ExitCode != 0:
		stderr, _ := base64.StdEncoding.DecodeString(st.ErrData)
		line, _, _ := strings.Cut(strings.TrimSpace(string(stderr)), "\n")
		return "", fmt.Errorf("exit code %d: %s", *st.ExitCode, truncate(strings.TrimSpace(line), 200))
	case st.OutTruncated:
		return "", errors.New("output truncated")
	}
	out, err := base64.StdEncoding.DecodeString(st.OutData)
	if err != nil {
		return "", fmt.Errorf("decode output: %w", err)
	}
	return string(out), nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// guestFSInfo is one entry of guest-get-fsinfo.
type guestFSInfo struct {
	Mountpoint string  `json:"mountpoint"`
	Type       string  `json:"type"`
	TotalBytes *uint64 `json:"total-bytes"`
	UsedBytes  *uint64 `json:"used-bytes"`
}

// filesystemsFromFSInfo keeps mounted filesystems that report a size. Windows
// lists volumes without a drive letter by label ("System Reserved"), which is
// not a mount point and need not be unique, so only paths are kept.
func filesystemsFromFSInfo(list []guestFSInfo) []dto.VMGuestFilesystem {
	out := []dto.VMGuestFilesystem{}
	for _, f := range list {
		isPath := strings.HasPrefix(f.Mountpoint, "/") || strings.HasSuffix(f.Mountpoint, `:\`)
		if !isPath || f.TotalBytes == nil {
			continue
		}
		out = append(out, dto.VMGuestFilesystem{Mountpoint: f.Mountpoint, Type: f.Type, TotalBytes: f.TotalBytes, UsedBytes: f.UsedBytes})
	}
	return out
}

// parseNvidiaSMI parses nvidia-smi CSV output (noheader, nounits). The name is
// everything between the index and the last seven fields, so a comma in it
// does not shift the readings.
func parseNvidiaSMI(out string) []dto.VMGuestGPU {
	gpus := []dto.VMGuestGPU{}
	for line := range strings.SplitSeq(out, "\n") {
		fields := strings.Split(strings.TrimSpace(line), ",")
		if len(fields) < 9 {
			continue
		}
		for i := range fields {
			fields[i] = strings.TrimSpace(fields[i])
		}
		index, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}
		n := len(fields)
		r := fields[n-7:]
		gpus = append(gpus, dto.VMGuestGPU{
			Index:              index,
			Name:               strings.Join(fields[1:n-7], ","),
			TemperatureC:       smiFloat(r[0]),
			UtilizationPercent: smiFloat(r[1]),
			MemoryUsedBytes:    smiMiB(r[2]),
			MemoryTotalBytes:   smiMiB(r[3]),
			PowerWatts:         smiFloat(r[4]),
			FanPercent:         smiFloat(r[5]),
			ClockMHz:           smiFloat(r[6]),
		})
	}
	return gpus
}

// smiFloat parses a reading; "[N/A]", "[Not Supported]" and the like are nil.
func smiFloat(s string) *float64 {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil
	}
	return &v
}

func smiMiB(s string) *uint64 {
	v := smiFloat(s)
	if v == nil || *v < 0 {
		return nil
	}
	b := uint64(*v) * 1024 * 1024
	return &b
}

// parseGuestDisks parses the PowerShell disk query. ConvertTo-Json writes a
// single object, not an array, when there is one disk. A temperature of 0
// means the drive does not report it.
func parseGuestDisks(out string) ([]dto.VMGuestDisk, error) {
	out = strings.TrimSpace(out)
	if out == "" {
		return []dto.VMGuestDisk{}, nil
	}
	if strings.HasPrefix(out, "{") {
		out = "[" + out + "]"
	}
	var raw []struct {
		Model   string   `json:"Model"`
		Serial  string   `json:"Serial"`
		Bus     string   `json:"Bus"`
		Temp    *float64 `json:"Temp"`
		TempMax *float64 `json:"TempMax"`
		Wear    *float64 `json:"Wear"`
	}
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		return nil, fmt.Errorf("decode disk query: %w", err)
	}
	disks := make([]dto.VMGuestDisk, 0, len(raw))
	for _, d := range raw {
		disks = append(disks, dto.VMGuestDisk{
			Model:           strings.TrimSpace(d.Model),
			Serial:          strings.TrimSpace(d.Serial),
			Bus:             d.Bus,
			TemperatureC:    nonZero(d.Temp),
			TemperatureMaxC: nonZero(d.TempMax),
			WearPercent:     d.Wear,
		})
	}
	return disks, nil
}

func nonZero(v *float64) *float64 {
	if v == nil || *v == 0 {
		return nil
	}
	return v
}

func clearFilesystems(in []dto.VMGuestFilesystem) []dto.VMGuestFilesystem {
	out := make([]dto.VMGuestFilesystem, len(in))
	for i, f := range in {
		out[i] = dto.VMGuestFilesystem{Mountpoint: f.Mountpoint, Type: f.Type}
	}
	return out
}

func clearGPUs(in []dto.VMGuestGPU) []dto.VMGuestGPU {
	out := make([]dto.VMGuestGPU, len(in))
	for i, g := range in {
		out[i] = dto.VMGuestGPU{Index: g.Index, Name: g.Name}
	}
	return out
}

func clearDisks(in []dto.VMGuestDisk) []dto.VMGuestDisk {
	out := make([]dto.VMGuestDisk, len(in))
	for i, d := range in {
		out[i] = dto.VMGuestDisk{Model: d.Model, Serial: d.Serial, Bus: d.Bus}
	}
	return out
}
