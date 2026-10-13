package collectors

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/tinkerologie/unraid-management-agent/daemon/constants"
	"github.com/tinkerologie/unraid-management-agent/daemon/dto"
	"github.com/tinkerologie/unraid-management-agent/daemon/lib"
	"github.com/tinkerologie/unraid-management-agent/daemon/logger"
	"gopkg.in/ini.v1"
)

const (
	// driveHealthTTL is how long a disk's SMART reading is reused before
	// smartctl is run again. SMART values change slowly; this keeps the cost
	// at one smartctl run per pool member every five minutes however short the
	// ZFS interval is.
	driveHealthTTL = 5 * time.Minute
	// driveHealthTimeout bounds one smartctl run.
	driveHealthTimeout = 15 * time.Second
	// driveHealthWorkers bounds how many smartctl processes exist at once,
	// including ones that timed out but have not exited yet.
	driveHealthWorkers = 4
)

// blockDeviceName matches a kernel block device name such as sda or nvme0n1.
var blockDeviceName = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// diskRef is a disk to read SMART data from.
type diskRef struct {
	name string // kernel name, e.g. sdb or nvme0n1
	// generation is the kernel's diskseq for the disk. It changes when the
	// disk is replaced or re-probed, even under the same name, so a cached
	// reading is never attributed to a different drive. Empty when unknown.
	generation string
	// smartType is the smartctl -d type (nvme, sat or scsi), chosen from
	// sysfs so smartctl does not probe the drive to detect it. Empty means
	// the disk is never read: USB bridges may ignore the standby check, and
	// smartctl's SCSI power check cannot fail closed (a stopped drive can
	// look active), so a rotational SCSI disk is read only when Unraid
	// manages it and reports its spin state.
	smartType string
	// asleep is true when Unraid reports the disk spun down. It is not read.
	asleep bool
	// unraidOnly is true for a rotational SCSI disk: it may only be read
	// while Unraid manages it and reports it spinning.
	unraidOnly bool
}

// unraidSpinStates returns which disks Unraid reports as spun down, keyed
// by kernel device name, from disks.ini. Unraid tracks spin state for every
// array and pool device, including spin-downs from its UI, its timers and
// this agent's API. Disks Unraid does not manage, and disks whose spundown
// value is not "0" or "1", are absent.
func unraidSpinStates(path string) map[string]bool {
	cfg, err := ini.Load(path)
	if err != nil {
		return nil
	}
	states := make(map[string]bool)
	for _, section := range cfg.Sections() {
		device := strings.Trim(section.Key("device").String(), `"`)
		if device == "" {
			continue
		}
		switch strings.Trim(section.Key("spundown").String(), `"`) {
		case "0":
			states[device] = false
		case "1":
			states[device] = true
		}
		// Any other value (or none) leaves the disk out: its state is unknown.
	}
	return states
}

// describeDisk works out how to read a disk's SMART data from sysfs and
// Unraid's spin states (see unraidSpinStates), without touching the disk.
func describeDisk(sysRoot, disk string, spinStates map[string]bool) diskRef {
	ref := diskRef{name: disk}
	block := filepath.Join(sysRoot, "class", "block", disk)
	if seq, err := os.ReadFile(filepath.Join(block, "diskseq")); err == nil { // #nosec G304 -- sysfs path built from a validated kernel device name
		ref.generation = strings.TrimSpace(string(seq))
	}
	spunDown, managed := spinStates[disk]
	ref.asleep = spunDown
	real, err := filepath.EvalSymlinks(block)
	switch {
	case err != nil || strings.Contains(real, "/usb"):
	case strings.HasPrefix(disk, "nvme"):
		ref.smartType = "nvme"
	case strings.HasPrefix(disk, "sd"):
		// libata disks (also SATA disks behind a SAS HBA's SATL) report "ATA".
		vendor, _ := os.ReadFile(filepath.Join(block, "device", "vendor"))        // #nosec G304 -- sysfs path built from a validated kernel device name
		rotational, _ := os.ReadFile(filepath.Join(block, "queue", "rotational")) // #nosec G304 -- sysfs path built from a validated kernel device name
		switch {
		case strings.TrimSpace(string(vendor)) == "ATA":
			ref.smartType = "sat"
		case strings.TrimSpace(string(rotational)) == "0":
			ref.smartType = "scsi"
		default:
			// Rotational, or unknown: treat as rotational.
			ref.unraidOnly = true
			if managed {
				ref.smartType = "scsi"
			}
		}
	}
	return ref
}

// smartctlRunner runs smartctl for one disk and returns its stdout. The
// channel is closed once the process has been reaped.
type smartctlRunner func(timeout time.Duration, disk, smartType string) (string, <-chan struct{}, error)

// runSmartctl reads a disk's SMART data as JSON without waking it from
// standby. The device type is given explicitly so smartctl does not send
// detection commands before its power-mode check, and for ATA the check fails
// closed: when CHECK POWER MODE is unsupported or returns an unknown value,
// smartctl exits (status 3) instead of reading the drive.
func runSmartctl(timeout time.Duration, disk, smartType string) (string, <-chan struct{}, error) {
	return lib.ExecCommandStdoutGuarded(context.Background(), timeout,
		constants.SmartctlBin, "-j", "-a", "-n", "standby,2,3", "-d", smartType, "/dev/"+disk)
}

// driveHealthEntry is the cached state of one disk.
type driveHealthEntry struct {
	generation string           // diskseq the entry belongs to
	health     *dto.DriveHealth // last successful read, nil if none
	standby    bool             // asleep at the last attempt
	attempted  time.Time        // last time smartctl was run
	inflight   <-chan struct{}  // closed once the last smartctl process exited
}

// driveHealthCache reads SMART data for ZFS pool members and caches it per disk.
type driveHealthCache struct {
	mu      sync.Mutex
	binary  string
	ttl     time.Duration
	timeout time.Duration
	// slots bounds the smartctl processes alive at once. A slot is held until
	// the process has been reaped, not just until it timed out.
	slots chan struct{}
	// spinStates returns Unraid's current spin states (see unraidSpinStates).
	// It is checked again right before each smartctl run, so a disk spun
	// down while its read waited for a slot is not read. nil skips the check.
	spinStates func() map[string]bool
	now        func() time.Time
	run        smartctlRunner
	entries    map[string]*driveHealthEntry
}

func newDriveHealthCache() *driveHealthCache {
	return &driveHealthCache{
		binary:  constants.SmartctlBin,
		ttl:     driveHealthTTL,
		timeout: driveHealthTimeout,
		slots:   make(chan struct{}, driveHealthWorkers),
		now:     time.Now,
		run:     runSmartctl,
		entries: make(map[string]*driveHealthEntry),
	}
}

// lookup returns the health of each disk, refreshing entries older than the
// TTL. Disks that have never been read successfully and are not asleep are
// left out. Entries for disks no longer asked for are dropped.
func (c *driveHealthCache) lookup(disks []diskRef) map[string]*dto.DriveHealth {
	if _, err := os.Stat(c.binary); err != nil {
		return nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	now := c.now()
	wanted := make(map[string]bool, len(disks))
	var stale []diskRef
	for _, ref := range disks {
		if wanted[ref.name] || !blockDeviceName.MatchString(ref.name) {
			continue
		}
		wanted[ref.name] = true
		entry := c.entries[ref.name]
		if entry == nil || entry.generation != ref.generation {
			// New disk, or a different disk now has this name.
			entry = &driveHealthEntry{generation: ref.generation}
			c.entries[ref.name] = entry
		}
		if ref.asleep {
			// Keep the last reading; read again as soon as the disk spins up.
			entry.standby = true
			entry.attempted = time.Time{}
			continue
		}
		if ref.smartType == "" {
			continue
		}
		if !entry.attempted.IsZero() && now.Sub(entry.attempted) < c.ttl {
			continue
		}
		if entry.inflight != nil && !closed(entry.inflight) {
			// A previous smartctl for this disk is still stuck; don't pile up.
			continue
		}
		stale = append(stale, ref)
	}
	for disk := range c.entries {
		if !wanted[disk] {
			delete(c.entries, disk)
		}
	}

	c.refresh(stale, now)

	result := make(map[string]*dto.DriveHealth, len(wanted))
	for disk := range wanted {
		entry := c.entries[disk]
		switch {
		case entry.health != nil:
			health := *entry.health
			health.Standby = entry.standby
			result[disk] = &health
		case entry.standby:
			result[disk] = &dto.DriveHealth{Standby: true}
		}
	}
	return result
}

// refresh runs smartctl for the given disks while process slots are free.
// Disks still waiting for a slot after c.timeout are left for the next
// cycle, so stuck processes cannot hold up the ZFS collection for long.
func (c *driveHealthCache) refresh(disks []diskRef, now time.Time) {
	type outcome struct {
		disk     string
		health   *dto.DriveHealth
		standby  bool
		inflight <-chan struct{}
	}
	results := make(chan outcome, len(disks))
	queue, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()
	var wg sync.WaitGroup
	for _, ref := range disks {
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case c.slots <- struct{}{}:
			case <-queue.Done():
				return
			}
			if c.spinStates != nil {
				spunDown, managed := c.spinStates()[ref.name]
				if spunDown || (ref.unraidOnly && !managed) {
					<-c.slots
					return
				}
			}
			out, done, runErr := c.run(c.timeout, ref.name, ref.smartType)
			go func() {
				if done != nil {
					<-done
				}
				<-c.slots
			}()
			health, standby, err := parseSmartctlJSON(out, now)
			if err != nil {
				logger.Debug("ZFS: SMART read of %s failed: %v (smartctl: %v)", ref.name, err, runErr)
			}
			results <- outcome{disk: ref.name, health: health, standby: standby, inflight: done}
		}()
	}
	wg.Wait()
	close(results)

	for r := range results {
		entry := c.entries[r.disk]
		entry.attempted = now
		entry.inflight = r.inflight
		switch {
		case r.standby:
			entry.standby = true
		case r.health != nil:
			entry.health = r.health
			entry.standby = false
		}
	}
}

// closed reports whether ch has been closed.
func closed(ch <-chan struct{}) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}

// smartctlJSON is the subset of `smartctl -j -a` output the agent uses.
type smartctlJSON struct {
	Smartctl struct {
		ExitStatus int `json:"exit_status"`
		Messages   []struct {
			String string `json:"string"`
		} `json:"messages"`
	} `json:"smartctl"`
	Device struct {
		Protocol string `json:"protocol"`
	} `json:"device"`
	ModelName    string `json:"model_name"`
	RotationRate *int   `json:"rotation_rate"`
	SmartStatus  *struct {
		Passed *bool `json:"passed"`
	} `json:"smart_status"`
	Temperature *struct {
		Current *int `json:"current"`
	} `json:"temperature"`
	PowerOnTime *struct {
		Hours *uint64 `json:"hours"`
	} `json:"power_on_time"`
	NVMeLog *struct {
		CriticalWarning *int    `json:"critical_warning"`
		AvailableSpare  *int    `json:"available_spare"`
		PercentageUsed  *int    `json:"percentage_used"`
		MediaErrors     *uint64 `json:"media_errors"`
	} `json:"nvme_smart_health_information_log"`
	SCSIErrors *struct {
		Read   *scsiErrorCounters `json:"read"`
		Write  *scsiErrorCounters `json:"write"`
		Verify *scsiErrorCounters `json:"verify"`
	} `json:"scsi_error_counter_log"`
	ATAAttributes *struct {
		Table []struct {
			ID  int `json:"id"`
			Raw struct {
				Value *uint64 `json:"value"`
			} `json:"raw"`
		} `json:"table"`
	} `json:"ata_smart_attributes"`
}

type scsiErrorCounters struct {
	TotalUncorrected *uint64 `json:"total_uncorrected_errors"`
}

// lowPowerMessage matches smartctl's "Device is in STANDBY mode, exit(2)"
// (also SLEEP, LOW POWER, IDLE_B... for SCSI), printed when -n skipped the drive.
var lowPowerMessage = regexp.MustCompile(`^Device is in .+ mode, exit\(\d+\)`)

// ataReportedUncorrect is ATA SMART attribute 187 (Reported_Uncorrect).
const ataReportedUncorrect = 187

// parseSmartctlJSON turns `smartctl -j -a -n standby` output into a
// DriveHealth. standby is true when smartctl skipped the disk because it was
// asleep. smartctl's exit status is a bitmask that is non-zero for a failing
// drive too, so the output is parsed whatever the exit status was.
func parseSmartctlJSON(out string, now time.Time) (*dto.DriveHealth, bool, error) {
	if strings.TrimSpace(out) == "" {
		return nil, false, errors.New("no output")
	}
	var raw smartctlJSON
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		// A value that does not fit its field (e.g. a 128-bit NVMe counter)
		// only loses that field; anything else is unusable output.
		var typeErr *json.UnmarshalTypeError
		if !errors.As(err, &typeErr) {
			return nil, false, fmt.Errorf("parse smartctl JSON: %w", err)
		}
	}
	for _, msg := range raw.Smartctl.Messages {
		switch {
		case lowPowerMessage.MatchString(msg.String):
			return nil, true, nil
		case strings.HasPrefix(msg.String, "CHECK POWER MODE"):
			return nil, false, fmt.Errorf("power mode unknown, drive not read: %s", strings.TrimSpace(msg.String))
		}
	}
	if raw.Device.Protocol == "" && raw.ModelName == "" {
		return nil, false, fmt.Errorf("no device data (exit status %d)", raw.Smartctl.ExitStatus)
	}

	health := &dto.DriveHealth{
		Model:       strings.TrimSpace(raw.ModelName),
		Protocol:    raw.Device.Protocol,
		CollectedAt: now,
	}
	switch {
	case raw.Device.Protocol == "NVMe":
		health.MediaType = "nvme"
	case raw.RotationRate != nil && *raw.RotationRate == 0:
		health.MediaType = "ssd"
	case raw.RotationRate != nil:
		health.MediaType = "hdd"
	}
	if raw.SmartStatus != nil {
		health.SmartPassed = raw.SmartStatus.Passed
	}
	if raw.Temperature != nil {
		health.TemperatureCelsius = raw.Temperature.Current
	}
	if raw.PowerOnTime != nil {
		health.PowerOnHours = raw.PowerOnTime.Hours
	}
	if log := raw.NVMeLog; log != nil {
		health.PercentageUsed = log.PercentageUsed
		health.AvailableSpare = log.AvailableSpare
		health.CriticalWarning = log.CriticalWarning
		health.MediaErrors = log.MediaErrors
	}
	if scsi := raw.SCSIErrors; scsi != nil {
		health.MediaErrors = sumUncorrected(scsi.Read, scsi.Write, scsi.Verify)
	}
	if ata := raw.ATAAttributes; ata != nil && health.MediaErrors == nil {
		for _, attr := range ata.Table {
			if attr.ID == ataReportedUncorrect {
				health.MediaErrors = attr.Raw.Value
				break
			}
		}
	}
	return health, false, nil
}

// sumUncorrected adds the SCSI uncorrected-error counters that are present,
// or returns nil when none are.
func sumUncorrected(logs ...*scsiErrorCounters) *uint64 {
	var total uint64
	found := false
	for _, l := range logs {
		if l != nil && l.TotalUncorrected != nil {
			total += *l.TotalUncorrected
			found = true
		}
	}
	if !found {
		return nil
	}
	return &total
}

// poolDrives lists the leaf members of a pool: the devices under each vdev,
// or the vdev itself when it is a single disk. It returns nil when the pool
// has no vdevs (status unavailable).
func poolDrives(pool *dto.ZFSPool) []dto.ZFSDrive {
	var drives []dto.ZFSDrive
	for _, vdev := range pool.VDEVs {
		if len(vdev.Devices) == 0 {
			if vdev.Type == "disk" {
				drives = append(drives, dto.ZFSDrive{Name: vdev.Name, State: vdev.State})
			}
			continue
		}
		for _, dev := range vdev.Devices {
			drives = append(drives, dto.ZFSDrive{Name: dev.Name, Vdev: vdev.Name, State: dev.State})
		}
	}
	return drives
}

// resolveWholeDisk maps a pool member as `zpool status` prints it (a
// partition such as sdb1 or nvme0n1p1, a whole disk, a device-mapper name or
// a /dev/disk/by-* link name) to the kernel name of the disk it lives on.
// It returns "" when the member cannot be found (e.g. a missing device that
// zpool shows by GUID).
func resolveWholeDisk(sysRoot, devRoot, member string) string {
	alias := filepath.Base(member)
	if alias == "." || alias == ".." || alias == string(filepath.Separator) {
		return ""
	}
	node := alias
	if _, err := os.Stat(filepath.Join(sysRoot, "class", "block", node)); err != nil {
		// Alias names (by-id links such as nvme-eui.0025...) may contain
		// characters kernel names don't; the resolved name is checked below.
		node = ""
		for _, dir := range []string{"mapper", filepath.Join("disk", "by-id"), filepath.Join("disk", "by-partuuid")} {
			target, err := filepath.EvalSymlinks(filepath.Join(devRoot, dir, alias))
			if err == nil {
				node = filepath.Base(target)
				break
			}
		}
		if node == "" {
			return ""
		}
	}

	// Follow device-mapper layers (LUKS) down to the partition, then the
	// partition up to its disk. Three hops cover dm -> partition -> disk.
	for range 3 {
		if !blockDeviceName.MatchString(node) {
			return ""
		}
		block := filepath.Join(sysRoot, "class", "block", node)
		if slaves, err := os.ReadDir(filepath.Join(block, "slaves")); err == nil && len(slaves) > 0 {
			node = slaves[0].Name()
			continue
		}
		// A partition's sysfs directory sits inside its disk's directory.
		if real, err := filepath.EvalSymlinks(filepath.Join(block, "partition")); err == nil {
			return filepath.Base(filepath.Dir(filepath.Dir(real)))
		}
		if _, err := os.Stat(block); err != nil {
			return ""
		}
		return node
	}
	return ""
}

// attachDrives fills each pool's Drives with its members, the disk each one
// is on, and that disk's cached SMART health.
func (c *ZFSCollector) attachDrives(pools []dto.ZFSPool) {
	spinStates := unraidSpinStates(c.disksIni)
	var disks []diskRef
	for i := range pools {
		pools[i].Drives = poolDrives(&pools[i])
		for j := range pools[i].Drives {
			drive := &pools[i].Drives[j]
			drive.Disk = resolveWholeDisk(c.sysRoot, c.devRoot, drive.Name)
			if drive.Disk != "" {
				disks = append(disks, describeDisk(c.sysRoot, drive.Disk, spinStates))
			}
		}
	}
	if c.driveHealth == nil {
		return
	}
	health := c.driveHealth.lookup(disks)
	for i := range pools {
		for j := range pools[i].Drives {
			drive := &pools[i].Drives[j]
			if h, ok := health[drive.Disk]; ok {
				drive.Health = h
			}
		}
	}
}
