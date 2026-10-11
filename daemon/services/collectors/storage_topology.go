package collectors

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/constants"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/lib"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
)

const (
	// storageTopologyStartupStagger delays the first collection so it does not pile onto boot.
	storageTopologyStartupStagger = 20 * time.Second
	// storcliTimeout bounds one storcli call. storcli normally answers in well
	// under a second, but MegaRAID firmware has been observed to take ~40 s
	// while busy, so the limit is generous.
	storcliTimeout = 90 * time.Second
	// sgSesTimeout bounds one sg_ses call (normally a few milliseconds).
	sgSesTimeout = 15 * time.Second
	// storageTopologyCycleBudget bounds a whole collection cycle.
	storageTopologyCycleBudget = 4 * time.Minute
	// sesPeripheralType is the SCSI peripheral device type of enclosure services devices.
	sesPeripheralType = "13"
)

// CommandRunner runs a command and returns its stdout. The returned channel is
// closed once the process has exited (see lib.ExecCommandStdoutGuarded).
type CommandRunner func(ctx context.Context, timeout time.Duration, name string, args ...string) (string, <-chan struct{}, error)

// StorageTopologyCollector periodically collects the SAS storage topology from
// storcli (Broadcom/LSI controllers) and sg_ses (SCSI Enclosure Services). Both
// sources are optional: without storcli only SES enclosures are reported, and on
// systems with neither the collector publishes an "unsupported" state.
//
// Every command is read-only, runs sequentially (storcli must never run twice at
// once), and is bounded by a timeout. A storcli process that does not exit after
// being killed (firmware stall) blocks further storcli calls until it is reaped.
type StorageTopologyCollector struct {
	appCtx *domain.Context
	// RunFn executes commands; replaceable for tests.
	RunFn CommandRunner
	// SysfsRoot is the sysfs mount point ("/sys"); replaceable for tests.
	SysfsRoot string
	// StorcliCandidates and SgSesCandidates are the binaries tried, in order.
	StorcliCandidates []string
	SgSesCandidates   []string
	// IsExecutable reports whether a candidate binary can be run; replaceable for tests.
	IsExecutable func(path string) bool
	// DiskstatsPath is the block device I/O statistics file ("/proc/diskstats"); replaceable for tests.
	DiskstatsPath string
	// Now returns the current time; replaceable for tests.
	Now func() time.Time
	// Stagger delays the first collection after Start.
	Stagger time.Duration

	inflight map[string]<-chan struct{}
	lastErrs string
	// prevDisk is the previous cycle's diskstats sample, used to compute throughput.
	prevDisk *diskSample
}

// NewStorageTopologyCollector creates a collector with production defaults.
func NewStorageTopologyCollector(ctx *domain.Context) *StorageTopologyCollector {
	return &StorageTopologyCollector{
		appCtx:    ctx,
		RunFn:     lib.ExecCommandStdoutGuarded,
		SysfsRoot: "/sys",
		StorcliCandidates: []string{
			"/sbin/storcli", "/usr/sbin/storcli", "/usr/local/sbin/storcli", "/usr/bin/storcli",
			"/opt/MegaRAID/storcli/storcli64", "/usr/local/sbin/storcli64", "/usr/sbin/storcli64",
		},
		SgSesCandidates: []string{"/usr/bin/sg_ses", "/usr/sbin/sg_ses", "/bin/sg_ses", "/sbin/sg_ses"},
		IsExecutable:    isExecutableFile,
		DiskstatsPath:   "/proc/diskstats",
		Now:             time.Now,
		Stagger:         storageTopologyStartupStagger,
		inflight:        map[string]<-chan struct{}{},
	}
}

// isExecutableFile reports whether path is a regular file with an execute bit.
func isExecutableFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0
}

// Start runs the collector until ctx is cancelled.
func (c *StorageTopologyCollector) Start(ctx context.Context, interval time.Duration) {
	logger.Info("Starting storage topology collector (interval: %v)", interval)

	select {
	case <-ctx.Done():
		return
	case <-time.After(c.Stagger):
	}

	run := func() {
		defer func() {
			if r := recover(); r != nil {
				logger.LogPanicWithStack("Storage topology collector", r)
			}
		}()
		collectWithWatchdog(ctx, "StorageTopology", interval, func() { c.Collect(ctx) })
	}
	run()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			logger.Info("Storage topology collector stopping due to context cancellation")
			return
		case <-ticker.C:
			run()
		}
	}
}

// Collect gathers the topology and publishes it.
func (c *StorageTopologyCollector) Collect(ctx context.Context) {
	topo := c.Gather(ctx)
	if ctx.Err() != nil {
		return // shutting down; do not publish a partial result
	}
	domain.Publish(c.appCtx.Hub, constants.TopicStorageTopologyUpdate, topo)
	logger.Debug("StorageTopology: published (controllers=%d, enclosures=%d, drives=%d, errors=%d, %dms)",
		topo.Summary.Controllers, topo.Summary.Enclosures, topo.Summary.Drives, len(topo.Errors),
		topo.CollectionDurationMs)
}

// Gather collects one topology snapshot.
func (c *StorageTopologyCollector) Gather(ctx context.Context) *dto.StorageTopology {
	start := time.Now()
	cycleCtx, cancel := context.WithTimeout(ctx, storageTopologyCycleBudget)
	defer cancel()

	topo := &dto.StorageTopology{
		State:       dto.StorageTopologyStateOK,
		Controllers: []dto.StorageController{},
		Enclosures:  []dto.StorageEnclosure{},
		Drives:      []dto.StorageDrive{},
	}
	var errs []string
	disk := c.sampleDiskstats()

	// storcli: no binary, or a binary that reports no controllers, means "no storcli data".
	var sc *storcliResult
	storcliFound := false
	if storcli := c.findBinary(c.StorcliCandidates); storcli != "" {
		var scErrs []string
		sc, scErrs = c.collectStorcli(cycleCtx, storcli, topo)
		errs = append(errs, scErrs...)
		storcliFound = sc != nil || len(scErrs) > 0
	}

	sesDevices := c.discoverSESDevices()
	var paths []sesPath
	sgSes := c.findBinary(c.SgSesCandidates)
	switch {
	case len(sesDevices) == 0:
	case sgSes == "":
		errs = append(errs, fmt.Sprintf("found %d SES device(s) but sg_ses (sg3_utils) is not installed", len(sesDevices)))
	default:
		var sesErrs []string
		paths, sesErrs = c.collectSES(cycleCtx, sgSes, sesDevices)
		errs = append(errs, sesErrs...)
		topo.Sources.SES = len(paths) > 0
		topo.Sources.SESDevices = len(paths)
	}

	if !storcliFound && (sgSes == "" || len(sesDevices) == 0) {
		topo.State = dto.StorageTopologyStateUnsupported
	}

	var drives []dto.StorageDrive
	if sc != nil {
		drives = sc.drives
	}
	assembleTopology(topo, sc, paths, c.driveDevices(drives))
	for i := range topo.Controllers {
		c.applyPCIeLink(&topo.Controllers[i])
	}
	applyThroughput(topo, c.prevDisk, disk)
	c.prevDisk = disk

	topo.Errors = errs
	topo.CollectionDurationMs = time.Since(start).Milliseconds()
	topo.Timestamp = time.Now()
	c.logErrors(errs)
	return topo
}

// logErrors logs collection errors once per distinct set, to avoid repeating
// the same warning every cycle.
func (c *StorageTopologyCollector) logErrors(errs []string) {
	sig := strings.Join(errs, "\n")
	if sig == c.lastErrs {
		return
	}
	c.lastErrs = sig
	if len(errs) == 0 {
		logger.Info("StorageTopology: collection is healthy again")
		return
	}
	logger.Warning("StorageTopology: %d collection problem(s): %s", len(errs), strings.Join(errs, "; "))
}

// findBinary returns the first executable candidate, or "".
func (c *StorageTopologyCollector) findBinary(candidates []string) string {
	for _, p := range candidates {
		if c.IsExecutable(p) {
			return p
		}
	}
	return ""
}

// run executes one command, refusing to start a second instance of a binary
// whose previous invocation has not exited yet.
func (c *StorageTopologyCollector) run(ctx context.Context, timeout time.Duration, bin string, args ...string) (string, error) {
	if done, ok := c.inflight[bin]; ok {
		select {
		case <-done:
			delete(c.inflight, bin)
		default:
			return "", fmt.Errorf("%s: previous invocation has not exited yet; skipped", filepath.Base(bin))
		}
	}
	out, done, err := c.RunFn(ctx, timeout, bin, args...)
	c.inflight[bin] = done
	return out, err
}

// isTimeout reports whether err came from a deadline or cancellation.
func isTimeout(err error) bool {
	return errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled)
}

// collectStorcli runs the storcli queries sequentially. It stops at the first
// timeout: a stalled controller would only make every following call stall too.
func (c *StorageTopologyCollector) collectStorcli(ctx context.Context, bin string, topo *dto.StorageTopology) (*storcliResult, []string) {
	var errs []string
	call := func(args ...string) (string, bool) {
		full := append(append([]string{}, args...), "J", "nolog")
		out, err := c.run(ctx, storcliTimeout, bin, full...)
		if err != nil && (out == "" || isTimeout(err)) {
			errs = append(errs, fmt.Sprintf("storcli %s: %v", strings.Join(args, " "), err))
			return "", false
		}
		// storcli exits non-zero when a query has nothing to report (e.g. no
		// enclosures); the JSON on stdout still says what happened.
		return out, true
	}

	out, ok := call("show")
	if !ok {
		return nil, errs
	}
	info, err := parseStorcliShow(out)
	if err != nil {
		return nil, append(errs, "storcli show: "+err.Error())
	}
	if info.controllers == 0 {
		return nil, errs
	}
	topo.Sources.StorcliPath = bin
	topo.Sources.StorcliVersion = info.version

	sc := &storcliResult{}
	if out, ok = call("/call", "show", "all"); !ok {
		return sc, errs
	}
	controllers, cErrs, err := parseStorcliControllers(out)
	errs = append(errs, cErrs...)
	if err != nil {
		return sc, append(errs, "storcli /call show all: "+err.Error())
	}
	sc.controllers = controllers
	topo.Sources.Storcli = len(controllers) > 0

	if out, ok = call("/call/pall", "show"); !ok {
		return sc, errs
	}
	phys, ports, pErrs, err := parseStorcliPhys(out)
	errs = append(errs, pErrs...)
	if err != nil {
		errs = append(errs, "storcli /call/pall show: "+err.Error())
	}
	sc.phys, sc.ports = phys, ports

	if out, ok = call("/call/eall", "show", "all"); !ok {
		return sc, errs
	}
	if sc.enclosures, err = parseStorcliEnclosures(out); err != nil {
		errs = append(errs, "storcli /call/eall show all: "+err.Error())
		sc.enclosures = map[[2]int]*storcliEnclosure{}
	}

	if out, ok = call("/call/eall", "show", "status"); !ok {
		return sc, errs
	}
	if err := applyStorcliEnclosureStatus(out, sc.enclosures); err != nil {
		errs = append(errs, "storcli /call/eall show status: "+err.Error())
	}

	if out, ok = call("/call/eall/sall", "show", "all"); !ok {
		return sc, errs
	}
	if sc.drives, err = parseStorcliDrives(out); err != nil {
		errs = append(errs, "storcli /call/eall/sall show all: "+err.Error())
	}
	return sc, errs
}

// sesDevice is an enclosure services device found in sysfs.
type sesDevice struct {
	name     string
	vendor   string
	product  string
	revision string
}

// discoverSESDevices lists SCSI generic devices whose peripheral type is "enclosure services".
func (c *StorageTopologyCollector) discoverSESDevices() []sesDevice {
	base := filepath.Join(c.SysfsRoot, "class", "scsi_generic")
	entries, err := os.ReadDir(base)
	if err != nil {
		return nil
	}
	var devices []sesDevice
	for _, e := range entries {
		dir := filepath.Join(base, e.Name(), "device")
		if readTrimmed(filepath.Join(dir, "type")) != sesPeripheralType {
			continue
		}
		devices = append(devices, sesDevice{
			name:     e.Name(),
			vendor:   readTrimmed(filepath.Join(dir, "vendor")),
			product:  readTrimmed(filepath.Join(dir, "model")),
			revision: readTrimmed(filepath.Join(dir, "rev")),
		})
	}
	sort.Slice(devices, func(i, j int) bool { return sgNumber(devices[i].name) < sgNumber(devices[j].name) })
	return devices
}

// sgNumber extracts N from "sgN" for natural ordering (unparseable names sort last).
func sgNumber(name string) int {
	n, err := strconv.Atoi(strings.TrimPrefix(name, "sg"))
	if err != nil {
		return int(^uint(0) >> 1)
	}
	return n
}

// readTrimmed reads a small sysfs attribute, returning "" on error.
func readTrimmed(path string) string {
	b, err := os.ReadFile(path) // #nosec G304 -- fixed sysfs paths under the configured sysfs root
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// collectSES reads the configuration page and the joined status/descriptor/
// additional-status pages of every SES device.
func (c *StorageTopologyCollector) collectSES(ctx context.Context, bin string, devices []sesDevice) ([]sesPath, []string) {
	var errs []string
	var paths []sesPath
	for _, d := range devices {
		if ctx.Err() != nil {
			errs = append(errs, "sg_ses: collection cycle ran out of time")
			break
		}
		path := sesPath{device: d.name, vendor: d.vendor, product: d.product, revision: d.revision}
		devPath := "/dev/" + d.name
		out, err := c.run(ctx, sgSesTimeout, bin, "--readonly", "--json", "--page=1", devPath)
		if err == nil {
			err = parseSESConfig(out, &path)
		}
		if err != nil {
			errs = append(errs, fmt.Sprintf("sg_ses %s configuration page: %v", d.name, err))
			continue
		}
		out, err = c.run(ctx, sgSesTimeout, bin, "--readonly", "--json", "--join", devPath)
		if err == nil {
			err = parseSESJoin(out, &path)
		}
		if err != nil {
			errs = append(errs, fmt.Sprintf("sg_ses %s status pages: %v", d.name, err))
			continue
		}
		paths = append(paths, path)
	}
	return paths, errs
}

// driveDevices maps drive serial numbers to block device names using the unit
// serial number VPD page (0x80) exposed in sysfs. Only serials of the given
// drives are looked up.
func (c *StorageTopologyCollector) driveDevices(drives []dto.StorageDrive) map[string]string {
	result := map[string]string{}
	if len(drives) == 0 {
		return result
	}
	wanted := map[string]bool{}
	for _, d := range drives {
		if d.SerialNumber != "" {
			wanted[d.SerialNumber] = true
		}
	}
	matches, _ := filepath.Glob(filepath.Join(c.SysfsRoot, "block", "*", "device", "vpd_pg80"))
	for _, m := range matches {
		b, err := os.ReadFile(m) // #nosec G304 -- sysfs paths from a fixed glob under the sysfs root
		if err != nil || len(b) <= 4 {
			continue
		}
		serial := strings.Trim(string(b[4:]), " \x00\n")
		if wanted[serial] {
			result[serial] = filepath.Base(filepath.Dir(filepath.Dir(m)))
		}
	}
	return result
}

// applyPCIeLink fills the controller's PCIe link details from sysfs.
func (c *StorageTopologyCollector) applyPCIeLink(ctrl *dto.StorageController) {
	if ctrl.PCIAddress == "" {
		return
	}
	dir := filepath.Join(c.SysfsRoot, "bus", "pci", "devices", ctrl.PCIAddress)
	ctrl.PCIeLinkSpeed = readTrimmed(filepath.Join(dir, "current_link_speed"))
	ctrl.PCIeMaxLinkSpeed = readTrimmed(filepath.Join(dir, "max_link_speed"))
	if n, err := strconv.Atoi(readTrimmed(filepath.Join(dir, "current_link_width"))); err == nil {
		ctrl.PCIeLinkWidth = &n
	}
	if n, err := strconv.Atoi(readTrimmed(filepath.Join(dir, "max_link_width"))); err == nil {
		ctrl.PCIeMaxLinkWidth = &n
	}
}
