package collectors

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"gopkg.in/ini.v1"

	"github.com/tinkerologie/unraid-management-agent/daemon/constants"
	"github.com/tinkerologie/unraid-management-agent/daemon/dto"
	"github.com/tinkerologie/unraid-management-agent/daemon/logger"
)

// defaultMntRoot is where Unraid mounts array disks and pools.
const defaultMntRoot = "/mnt"

// sharePaths holds the file locations the share collector reads. Empty
// fields fall back to the production defaults so a zero-value ShareCollector
// keeps working; tests point them at fixture files.
type sharePaths struct {
	mntRoot   string
	sharesIni string
	varIni    string
	disksIni  string
}

func (p sharePaths) mnt() string {
	if p.mntRoot == "" {
		return defaultMntRoot
	}
	return p.mntRoot
}

func (p sharePaths) shares() string {
	if p.sharesIni == "" {
		return constants.SharesIni
	}
	return p.sharesIni
}

func (p sharePaths) vars() string {
	if p.varIni == "" {
		return constants.VarIni
	}
	return p.varIni
}

func (p sharePaths) disks() string {
	if p.disksIni == "" {
		return constants.DisksIni
	}
	return p.disksIni
}

// applyExclusiveState completes the exclusive-share fields (Unraid 6.12+).
//
// With exclusive shares enabled globally, Unraid replaces /mnt/user/<share>
// with a symlink to /mnt/<pool>/<share> for every share that lives on a single
// pool, so I/O bypasses the FUSE user share layer (shfs). If a top-level folder
// for the share also exists on another pool or array disk, Unraid silently
// keeps the share on FUSE instead.
//
// Exclusive comes from the "exclusive" flag emhttp writes to shares.ini when
// it sets up the symlink, so nothing under /mnt/user (FUSE) is touched. The
// fields are cleared when the release has no exclusive-share setting or the
// user share file system is not mounted (array stopped, user shares disabled).
//
// exportUnknown names shares whose config could not be read; on releases
// where NFS export blocks exclusive access their eligibility stays unknown.
func (p sharePaths) applyExclusiveState(shares []dto.ShareInfo, exportUnknown map[string]bool) {
	settings, supported := p.exclusiveSettings()
	if !supported || !settings.userSharesMounted {
		for i := range shares {
			shares[i].Exclusive = nil
		}
		return
	}

	// disks.ini is read lazily: only needed for shares on FUSE and for the
	// implicit pool on pools-only servers.
	var disks map[string]map[string]string
	disksRead := false
	loadDisks := func() map[string]map[string]string {
		if !disksRead {
			disks, disksRead = p.readDisks(), true
		}
		return disks
	}
	var branches []storageBranch
	for i := range shares {
		share := &shares[i]
		if share.Exclusive == nil || validateShareName(share.Name) != nil {
			share.Exclusive = nil
			continue
		}
		exclusive := *share.Exclusive

		// Pool-only: primary storage is a pool and there is no secondary
		// storage. Without array slots Unraid treats any useCache value with
		// no secondary pool as "only", and a share without a primary pool as
		// being on the first pool (as the webGUI's share list does).
		pool := share.CachePool
		if pool == "" && settings.poolsOnly {
			pool = firstPool(loadDisks())
		}
		poolOnly := pool != "" && share.CachePool2 == "" &&
			(share.UseCache == "only" || settings.poolsOnly)
		if poolOnly && !exclusive {
			// A removed or renamed pool makes the assignment invalid rather
			// than eligible; without the pool list eligibility is unknown.
			disks := loadDisks()
			if disks == nil {
				continue
			}
			poolOnly = poolNames(disks)[pool]
		}
		if exclusive || poolOnly {
			share.ExclusivePool = pool
		}
		eligible := settings.enabled && poolOnly
		if eligible && settings.nfsBlocks {
			if exportUnknown[share.Name] {
				continue // NFS export, and so eligibility, unknown
			}
			eligible = !share.NFSExport
		}
		share.ExclusiveEligible = &eligible

		if eligible && !exclusive {
			if branches == nil && loadDisks() != nil {
				branches = storageBranches(loadDisks(), settings)
			}
			if branches != nil { // nil: disks.ini unreadable, locations unknown
				found, unchecked := p.shareLocations(branches, share.Name)
				share.Locations, share.LocationsUnchecked = &found, unchecked
			}
		}
	}
}

// readDisks parses disks.ini; nil when it cannot be read.
func (p sharePaths) readDisks() map[string]map[string]string {
	// #nosec G304 -- path is the fixed emhttp disks.ini (overridable only in tests)
	data, err := os.ReadFile(p.disks())
	if err != nil {
		logger.Debug("Share: cannot read %s: %v", p.disks(), err)
		return nil
	}
	return parseINISections(string(data))
}

// poolNames returns the configured pools: Cache entries of disks.ini named
// without the member number suffix, as the webGUI lists them.
func poolNames(disks map[string]map[string]string) map[string]bool {
	pools := map[string]bool{}
	for section, kv := range disks {
		if kv["type"] == "Cache" {
			pools[strings.TrimRightFunc(section, unicode.IsDigit)] = true
		}
	}
	return pools
}

// firstPool returns the pool the webGUI assigns to a share without a primary
// pool on a pools-only server: the first pool in disks.ini order, named
// without the member number suffix.
func firstPool(disks map[string]map[string]string) string {
	first, firstIdx := "", -1
	for section, kv := range disks {
		idx, err := strconv.Atoi(kv["idx"])
		if kv["type"] != "Cache" || err != nil || (firstIdx >= 0 && idx >= firstIdx) {
			continue
		}
		first, firstIdx = section, idx
	}
	return strings.TrimRightFunc(first, unicode.IsDigit)
}

// exclusiveSettings holds the var.ini settings the exclusive share check uses.
type exclusiveSettings struct {
	enabled           bool // "Permit exclusive shares"
	nfsBlocks         bool // release refuses exclusive access for NFS exported shares
	userSharesMounted bool // user shares enabled and the array started
	poolsOnly         bool // server has no array slots (SYS_ARRAY_SLOTS <= 2)
	include           map[string]bool
	exclude           map[string]bool
}

// exclusiveSettings reads the global share settings from var.ini. supported
// is false when the exclusive share key is absent (Unraid before 6.12).
// Unraid lifted the NFS export restriction in 7.2.0.
func (p sharePaths) exclusiveSettings() (exclusiveSettings, bool) {
	cfg, err := ini.Load(p.vars())
	if err != nil {
		logger.Debug("Share: cannot read %s for exclusive share setting: %v", p.vars(), err)
		return exclusiveSettings{}, false
	}
	section := cfg.Section("")
	if !section.HasKey("shareUserExclusive") {
		return exclusiveSettings{}, false
	}
	return exclusiveSettings{
		enabled:   section.Key("shareUserExclusive").String() == "yes",
		nfsBlocks: releaseBefore(section.Key("version").String(), 7, 2),
		userSharesMounted: section.Key("shareUser").String() == "e" &&
			section.Key("fsState").String() == "Started",
		poolsOnly: poolsOnlyServer(section.Key("SYS_ARRAY_SLOTS").String()),
		include:   diskSet(section.Key("shareUserInclude").String()),
		exclude:   diskSet(section.Key("shareUserExclude").String()),
	}, true
}

// poolsOnlyServer mirrors the webGUI: a server with at most two array slots
// has no array, so shares can only live on pools. Unknown counts as false.
func poolsOnlyServer(slots string) bool {
	n, err := strconv.Atoi(slots)
	return err == nil && n <= 2
}

// diskSet parses a comma-separated disk list such as "disk1,disk3".
func diskSet(list string) map[string]bool {
	set := map[string]bool{}
	for disk := range strings.SplitSeq(list, ",") {
		if disk = strings.TrimSpace(disk); disk != "" {
			set[disk] = true
		}
	}
	return set
}

// releaseBefore reports whether an Unraid version string such as
// "6.12.10" or "7.2.0-rc.1" is older than major.minor. Unparsable versions
// count as current.
func releaseBefore(version string, major, minor int) bool {
	parts := strings.SplitN(version, ".", 3)
	if len(parts) < 2 {
		return false
	}
	gotMajor, errMajor := strconv.Atoi(parts[0])
	gotMinor, errMinor := strconv.Atoi(strings.SplitN(parts[1], "-", 2)[0])
	if errMajor != nil || errMinor != nil {
		return false
	}
	return gotMajor < major || (gotMajor == major && gotMinor < minor)
}

// storageBranch is an array disk or pool that is part of the user share
// file system.
type storageBranch struct {
	name string
	// awake is set only when disks.ini reports every device of the disk or
	// pool as spinning. Other branches are not probed, so the check never
	// wakes a disk.
	awake bool
}

// storageBranches returns the mounted array disks and pools that make up the
// user share file system, from disks.ini: mounted Data disks allowed by the
// global included/excluded disk settings and mounted pools that are not
// excluded from user shares. Pool member devices (cache2, ...) carry no
// fsStatus and flash/parity entries have other types, so they never match.
func storageBranches(sections map[string]map[string]string, settings exclusiveSettings) []storageBranch {
	// Pool devices follow the pool entry in slot order, so a pool spans the
	// idx range [idx, idx+slots).
	spinning := map[int]bool{}
	for _, kv := range sections {
		if idx, err := strconv.Atoi(kv["idx"]); err == nil {
			spinning[idx] = kv["spundown"] == "0"
		}
	}

	branches := []storageBranch{}
	for section, kv := range sections {
		name := kv["name"]
		if name == "" {
			name = section
		}
		if kv["fsStatus"] != "Mounted" || validateShareName(name) != nil {
			continue
		}
		switch kv["type"] {
		case "Data":
			if (len(settings.include) > 0 && !settings.include[name]) || settings.exclude[name] {
				continue
			}
		case "Cache":
			if kv["shareEnabled"] == "no" {
				continue
			}
		default:
			continue
		}
		branches = append(branches, storageBranch{name: name, awake: branchAwake(kv, spinning, kv["type"] == "Cache")})
	}
	sort.Slice(branches, func(i, j int) bool { return branches[i].name < branches[j].name })
	return branches
}

// branchAwake reports whether disks.ini shows the disk, or every device of
// the pool, as spinning. Missing or unknown spin state, or a pool whose
// device count is unknown, counts as not awake.
func branchAwake(kv map[string]string, spinning map[int]bool, pool bool) bool {
	idx, err := strconv.Atoi(kv["idx"])
	if err != nil {
		return false
	}
	slots := 1
	if pool {
		if slots, err = strconv.Atoi(kv["slots"]); err != nil || slots < 1 {
			return false
		}
	}
	for i := idx; i < idx+slots; i++ {
		if !spinning[i] {
			return false
		}
	}
	return true
}

// shareLocations lists the awake disks and pools holding a top-level folder
// for the share (one lstat of /mnt/<branch>/<name> each) and the ones that
// were left unchecked because they are spun down, their state is unknown or
// the check failed.
func (p sharePaths) shareLocations(branches []storageBranch, name string) (found, unchecked []string) {
	found = []string{}
	for _, branch := range branches {
		if !branch.awake {
			unchecked = append(unchecked, branch.name)
			continue
		}
		path := filepath.Join(p.mnt(), branch.name, name)
		info, err := os.Lstat(path)
		switch {
		case err == nil:
			if info.IsDir() {
				found = append(found, branch.name)
			}
		case errors.Is(err, fs.ErrNotExist):
		default:
			logger.Debug("Share: cannot check %s: %v", path, err)
			unchecked = append(unchecked, branch.name)
		}
	}
	return found, unchecked
}
