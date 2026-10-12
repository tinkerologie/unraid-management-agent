package collectors

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/tinkerologie/unraid-management-agent/daemon/domain"
	"github.com/tinkerologie/unraid-management-agent/daemon/dto"
)

const exclusiveDisksIni = `["parity"]
idx="0"
name="parity"
type="Parity"
status="DISK_NP"
["disk1"]
idx="1"
name="disk1"
type="Data"
spundown="0"
fsStatus="Mounted"
["disk2"]
idx="2"
name="disk2"
type="Data"
spundown="0"
fsStatus="Mounted"
["disk3"]
idx="3"
name="disk3"
type="Data"
fsStatus="Unmountable: unsupported or no file system"
["disk4"]
idx="4"
name="disk4"
type="Data"
spundown="1"
fsStatus="Mounted"
["disk5"]
idx="5"
name="disk5"
type="Data"
spundown="0"
fsStatus="Mounted"
["disk6"]
idx="6"
name="disk6"
type="Data"
fsStatus="Mounted"
["cache"]
idx="30"
name="cache"
type="Cache"
slots="2"
spundown="0"
fsStatus="Mounted"
shareEnabled="yes"
["cache2"]
idx="31"
name="cache2"
type="Cache"
spundown="0"
["slow"]
idx="32"
name="slow"
type="Cache"
slots="2"
spundown="0"
fsStatus="Mounted"
["slow2"]
idx="33"
name="slow2"
type="Cache"
spundown="1"
["scratch"]
idx="34"
name="scratch"
type="Cache"
slots="0"
fsStatus="Mounted"
shareEnabled="no"
["nvme"]
idx="40"
slots="1"
type="Cache"
spundown="0"
fsStatus="Mounted"
["flash"]
name="flash"
type="Boot"
fsStatus="Mounted"
`

const startedVarIni = "shareUser=\"e\"\nfsState=\"Started\"\nshareUserExclusive=\"yes\"\n"

// exclusiveFixture builds a fake /mnt tree, var.ini and disks.ini in a temp dir.
func exclusiveFixture(t *testing.T, varIni string) sharePaths {
	t.Helper()
	root := t.TempDir()
	mnt := filepath.Join(root, "mnt")
	for _, dir := range []string{
		"cache/appdata",   // primary pool copy
		"disk1/appdata",   // stray copy
		"disk2",           // holds a file named appdata
		"disk4/appdata",   // spun down: never probed
		"disk5/appdata",   // excluded from user shares by var.ini
		"disk6/appdata",   // spin state unknown: never probed
		"slow/appdata",    // pool with a spun-down device: never probed
		"scratch/appdata", // pool excluded from user shares
		"cache2/appdata",  // pool member device, never a branch
		"flash/appdata",   // boot device, never a branch
		"disk3/appdata",   // unmountable disk, never a branch
		"cache/downloads", // FUSE fallback with no stray folder found
	} {
		if err := os.MkdirAll(filepath.Join(mnt, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// A plain file named after the share is not a folder and does not count.
	if err := os.WriteFile(filepath.Join(mnt, "disk2", "appdata"), nil, 0o600); err != nil {
		t.Fatal(err)
	}

	p := sharePaths{
		mntRoot:   mnt,
		sharesIni: filepath.Join(root, "shares.ini"),
		varIni:    filepath.Join(root, "var.ini"),
		disksIni:  filepath.Join(root, "disks.ini"),
	}
	if err := os.WriteFile(p.disksIni, []byte(exclusiveDisksIni), 0o600); err != nil {
		t.Fatal(err)
	}
	if varIni != "" {
		if err := os.WriteFile(p.varIni, []byte(varIni), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return p
}

func boolPtr(b bool) *bool { return &b }

// poolOnly returns a pool-only share with the shares.ini exclusive flag.
func poolOnly(name, pool string, exclusive bool) dto.ShareInfo {
	return dto.ShareInfo{Name: name, UseCache: "only", CachePool: pool, Exclusive: boolPtr(exclusive)}
}

func locs(l *[]string) []string {
	if l == nil {
		return nil
	}
	return *l
}

func deref(b *bool) any {
	if b == nil {
		return nil
	}
	return *b
}

func TestApplyExclusiveState(t *testing.T) {
	p := exclusiveFixture(t, startedVarIni+"shareUserExclude=\"disk5, disk9\"\n")
	shares := []dto.ShareInfo{
		poolOnly("system", "cache", true),
		poolOnly("appdata", "cache", false),
		poolOnly("downloads", "cache", false),
		{Name: "media", UseCache: "yes", CachePool: "cache", Exclusive: boolPtr(false)},
		{Name: "backup", UseCache: "only", CachePool: "cache", CachePool2: "disk1", Exclusive: boolPtr(false)},
		poolOnly("missing", "cache", false),
		{Name: "old", UseCache: "only", CachePool: "cache"}, // no exclusive key in shares.ini
		poolOnly("../escape", "cache", false),
	}
	p.applyExclusiveState(shares, nil)

	unchecked := []string{"disk4", "disk6", "slow"}
	want := []struct {
		exclusive, eligible *bool
		pool                string
		locations           []string
		unchecked           []string
	}{
		{boolPtr(true), boolPtr(true), "cache", nil, nil},
		{boolPtr(false), boolPtr(true), "cache", []string{"cache", "disk1"}, unchecked},
		{boolPtr(false), boolPtr(true), "cache", []string{"cache"}, unchecked},
		{boolPtr(false), boolPtr(false), "", nil, nil},
		{boolPtr(false), boolPtr(false), "", nil, nil},
		{boolPtr(false), boolPtr(true), "cache", []string{}, unchecked},
		{nil, nil, "", nil, nil},
		{nil, nil, "", nil, nil},
	}
	for i, w := range want {
		got := shares[i]
		if !reflect.DeepEqual(got.Exclusive, w.exclusive) ||
			!reflect.DeepEqual(got.ExclusiveEligible, w.eligible) ||
			got.ExclusivePool != w.pool ||
			!reflect.DeepEqual(locs(got.Locations), w.locations) ||
			!reflect.DeepEqual(got.LocationsUnchecked, w.unchecked) {
			t.Errorf("%s: got exclusive=%v eligible=%v pool=%q locations=%#v unchecked=%#v, want %v %v %q %#v %#v",
				got.Name, deref(got.Exclusive), deref(got.ExclusiveEligible), got.ExclusivePool,
				locs(got.Locations), got.LocationsUnchecked,
				deref(w.exclusive), deref(w.eligible), w.pool, w.locations, w.unchecked)
		}
	}
}

func TestApplyExclusiveStateGloballyDisabled(t *testing.T) {
	p := exclusiveFixture(t, "shareUser=\"e\"\nfsState=\"Started\"\nshareUserExclusive=\"no\"\n")
	shares := []dto.ShareInfo{poolOnly("appdata", "cache", false)}
	p.applyExclusiveState(shares, nil)
	if shares[0].Exclusive == nil || *shares[0].Exclusive || shares[0].ExclusiveEligible == nil || *shares[0].ExclusiveEligible {
		t.Fatalf("want exclusive=false eligible=false, got %+v", shares[0])
	}
	if shares[0].Locations != nil {
		t.Errorf("locations reported for an ineligible share: %v", *shares[0].Locations)
	}
}

func TestApplyExclusiveStateUnavailable(t *testing.T) {
	for name, varIni := range map[string]string{
		"no var.ini":           "",
		"pre-6.12 var.ini":     "shareUser=\"e\"\nfsState=\"Started\"\n",
		"array stopped":        "shareUser=\"e\"\nfsState=\"Stopped\"\nshareUserExclusive=\"yes\"\n",
		"user shares disabled": "shareUser=\"-\"\nfsState=\"Started\"\nshareUserExclusive=\"yes\"\n",
	} {
		t.Run(name, func(t *testing.T) {
			shares := []dto.ShareInfo{poolOnly("system", "cache", true)}
			exclusiveFixture(t, varIni).applyExclusiveState(shares, nil)
			if shares[0].Exclusive != nil || shares[0].ExclusiveEligible != nil || shares[0].ExclusivePool != "" {
				t.Errorf("fields set although state is unknown: %+v", shares[0])
			}
		})
	}
}

func TestApplyExclusiveStateNoDisksIni(t *testing.T) {
	p := exclusiveFixture(t, startedVarIni)
	if err := os.Remove(p.disksIni); err != nil {
		t.Fatal(err)
	}
	shares := []dto.ShareInfo{poolOnly("appdata", "cache", false), poolOnly("downloads", "cache", false)}
	p.applyExclusiveState(shares, nil)
	for _, s := range shares {
		if s.Exclusive == nil || *s.Exclusive || s.ExclusiveEligible != nil || s.Locations != nil || s.LocationsUnchecked != nil {
			t.Errorf("%s: want FUSE with eligibility unknown, got %+v", s.Name, s)
		}
	}
}

func TestShareLocationsProbeError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission checks do not apply to root")
	}
	p := exclusiveFixture(t, startedVarIni)
	disk1 := filepath.Join(p.mntRoot, "disk1")
	if err := os.Chmod(disk1, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(disk1, 0o755) }) // #nosec G302 -- restore test dir for cleanup

	found, unchecked := p.shareLocations([]storageBranch{
		{name: "cache", awake: true},
		{name: "disk1", awake: true},
		{name: "disk2", awake: true},
	}, "appdata")
	if !reflect.DeepEqual(found, []string{"cache"}) || !reflect.DeepEqual(unchecked, []string{"disk1"}) {
		t.Errorf("found=%v unchecked=%v, want [cache] [disk1]", found, unchecked)
	}
}

func TestApplyExclusiveStateIncludedDisks(t *testing.T) {
	p := exclusiveFixture(t, startedVarIni+"shareUserInclude=\"disk5\"\n")
	shares := []dto.ShareInfo{poolOnly("appdata", "cache", false)}
	p.applyExclusiveState(shares, nil)
	// disk1 is not in the included disks; pools are unaffected.
	if want := []string{"cache", "disk5"}; !reflect.DeepEqual(locs(shares[0].Locations), want) {
		t.Errorf("locations = %v, want %v", locs(shares[0].Locations), want)
	}
}

func TestApplyExclusiveStatePoolsOnlyServer(t *testing.T) {
	prefer := dto.ShareInfo{Name: "appdata", UseCache: "prefer", CachePool: "cache", Exclusive: boolPtr(false)}
	yes := dto.ShareInfo{Name: "system", UseCache: "yes", CachePool: "cache", Exclusive: boolPtr(true)}
	twoPools := dto.ShareInfo{Name: "media", UseCache: "yes", CachePool: "cache", CachePool2: "slow", Exclusive: boolPtr(false)}
	for _, tt := range []struct {
		slots    string
		eligible bool
	}{
		{"SYS_ARRAY_SLOTS=\"0\"\n", true},
		{"SYS_ARRAY_SLOTS=\"2\"\n", true},
		{"SYS_ARRAY_SLOTS=\"6\"\n", false},
		{"", false},
	} {
		p := exclusiveFixture(t, startedVarIni+tt.slots)
		shares := []dto.ShareInfo{prefer, yes, twoPools}
		p.applyExclusiveState(shares, nil)
		if deref(shares[0].ExclusiveEligible) != tt.eligible || deref(shares[1].ExclusiveEligible) != tt.eligible {
			t.Errorf("%q: eligible=%v/%v, want %v", tt.slots, deref(shares[0].ExclusiveEligible), deref(shares[1].ExclusiveEligible), tt.eligible)
		}
		if tt.eligible && !reflect.DeepEqual(locs(shares[0].Locations), []string{"cache", "disk1", "disk5"}) {
			t.Errorf("%q: locations=%v", tt.slots, locs(shares[0].Locations))
		}
		if tt.eligible && shares[1].ExclusivePool != "cache" {
			t.Errorf("%q: exclusive_pool=%q", tt.slots, shares[1].ExclusivePool)
		}
		if deref(shares[2].ExclusiveEligible) != false {
			t.Errorf("%q: share with a secondary pool reported eligible", tt.slots)
		}
	}
}

func TestApplyExclusiveStatePoolsOnlyImplicitPool(t *testing.T) {
	p := exclusiveFixture(t, startedVarIni+"SYS_ARRAY_SLOTS=\"0\"\n")
	shares := []dto.ShareInfo{
		{Name: "appdata", UseCache: "yes", Exclusive: boolPtr(false)},
		{Name: "system", UseCache: "no", Exclusive: boolPtr(true)},
	}
	p.applyExclusiveState(shares, nil)
	for _, s := range shares {
		if s.ExclusivePool != "cache" || deref(s.ExclusiveEligible) != true {
			t.Errorf("%s: pool=%q eligible=%v, want cache true", s.Name, s.ExclusivePool, deref(s.ExclusiveEligible))
		}
	}
	if !reflect.DeepEqual(locs(shares[0].Locations), []string{"cache", "disk1", "disk5"}) {
		t.Errorf("appdata locations=%v", locs(shares[0].Locations))
	}

	// No pools at all: not pool-only, so not eligible.
	if err := os.WriteFile(p.disksIni, []byte(`["disk1"]`+"\nidx=\"1\"\ntype=\"Data\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	shares = []dto.ShareInfo{{Name: "appdata", UseCache: "yes", Exclusive: boolPtr(false)}}
	p.applyExclusiveState(shares, nil)
	if deref(shares[0].ExclusiveEligible) != false || shares[0].ExclusivePool != "" {
		t.Errorf("no pools: %+v", shares[0])
	}
}

func TestApplyExclusiveStateRemovedPool(t *testing.T) {
	p := exclusiveFixture(t, startedVarIni)
	shares := []dto.ShareInfo{
		poolOnly("appdata", "gone", false),
		poolOnly("system", "gone", true), // exclusive: trusted as is
	}
	p.applyExclusiveState(shares, nil)
	if s := shares[0]; deref(s.ExclusiveEligible) != false || s.ExclusivePool != "" || s.Locations != nil {
		t.Errorf("removed pool: %+v", s)
	}
	if s := shares[1]; deref(s.ExclusiveEligible) != true || s.ExclusivePool != "gone" {
		t.Errorf("exclusive share: %+v", s)
	}
}

func TestPoolNames(t *testing.T) {
	got := poolNames(parseINISections(exclusiveDisksIni))
	want := map[string]bool{"cache": true, "slow": true, "scratch": true, "nvme": true}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("poolNames = %v, want %v", got, want)
	}
}

func TestFirstPool(t *testing.T) {
	disks := parseINISections(exclusiveDisksIni + `["early9"]
idx="20"
type="Cache"
["noidx"]
type="Cache"
`)
	if got := firstPool(disks); got != "early" {
		t.Errorf("firstPool = %q, want early", got)
	}
	if got := firstPool(nil); got != "" {
		t.Errorf("firstPool(nil) = %q", got)
	}
}

func TestApplyExclusiveStateExportUnknown(t *testing.T) {
	for version, eligible := range map[string]*bool{"7.1.4": nil, "7.4.0": boolPtr(true)} {
		p := exclusiveFixture(t, startedVarIni+"version=\""+version+"\"\n")
		shares := []dto.ShareInfo{poolOnly("appdata", "cache", false)}
		p.applyExclusiveState(shares, map[string]bool{"appdata": true})
		got := shares[0]
		if deref(got.Exclusive) != false || !reflect.DeepEqual(got.ExclusiveEligible, eligible) {
			t.Errorf("%s: exclusive=%v eligible=%v, want false %v", version, deref(got.Exclusive), deref(got.ExclusiveEligible), deref(eligible))
		}
		if (eligible == nil) != (got.Locations == nil) {
			t.Errorf("%s: locations=%v with eligible=%v", version, locs(got.Locations), deref(eligible))
		}
	}
}

func TestEnrichShareWithConfigExportKnown(t *testing.T) {
	c := &ShareCollector{}
	cc := NewConfigCollector()
	// No .cfg: Unraid defaults apply, so the export state is known.
	if !c.enrichShareWithConfig(&dto.ShareInfo{Name: "no-such-share-cfg"}, cc) {
		t.Error("missing share config should leave the export state known")
	}
	if c.enrichShareWithConfig(&dto.ShareInfo{Name: "../escape"}, cc) {
		t.Error("unreadable share config should leave the export state unknown")
	}
}

func TestShareLocationsJSON(t *testing.T) {
	for _, tt := range []struct {
		locations *[]string
		want      string
	}{
		{nil, ""},
		{&[]string{}, `"locations":[]`},
		{&[]string{"cache", "disk1"}, `"locations":["cache","disk1"]`},
	} {
		out, err := json.Marshal(dto.ShareInfo{Name: "appdata", Locations: tt.locations})
		if err != nil {
			t.Fatal(err)
		}
		has := strings.Contains(string(out), `"locations"`)
		if (tt.want == "") == has || (tt.want != "" && !strings.Contains(string(out), tt.want)) {
			t.Errorf("locations=%v serialized as %s, want %q", locs(tt.locations), out, tt.want)
		}
	}
}

func TestApplyExclusiveStateNFSBeforeRelease72(t *testing.T) {
	nfsShare := poolOnly("appdata", "cache", false)
	nfsShare.NFSExport = true
	for _, tt := range []struct {
		version  string
		eligible bool
	}{
		{"6.12.10", false},
		{"7.1.4", false},
		{"7.2.0-rc.1", true},
		{"7.4.0", true},
	} {
		p := exclusiveFixture(t, startedVarIni+"version=\""+tt.version+"\"\n")
		shares := []dto.ShareInfo{nfsShare, poolOnly("downloads", "cache", false)}
		p.applyExclusiveState(shares, nil)
		if got := *shares[0].ExclusiveEligible; got != tt.eligible {
			t.Errorf("%s: NFS share eligible=%v, want %v", tt.version, got, tt.eligible)
		}
		if !*shares[1].ExclusiveEligible {
			t.Errorf("%s: non-NFS share not eligible", tt.version)
		}
	}
}

// TestCollectSharesExclusiveFlag runs the shares.ini parser end to end on
// the format emhttp writes on Unraid 7.4.
func TestCollectSharesExclusiveFlag(t *testing.T) {
	p := exclusiveFixture(t, startedVarIni)
	sharesIni := `["system"]
name="system"
useCache="only"
cachePool="cache"
cachePool2=""
exclusive="yes"
["appdata"]
name="appdata"
useCache="only"
cachePool="cache"
cachePool2=""
exclusive="no"
["old"]
name="old"
useCache="only"
cachePool="cache"
`
	if err := os.WriteFile(p.sharesIni, []byte(sharesIni), 0o600); err != nil {
		t.Fatal(err)
	}
	c := &ShareCollector{ctx: &domain.Context{}, paths: p}
	shares, err := c.collectShares(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(shares) != 3 {
		t.Fatalf("got %d shares, want 3", len(shares))
	}
	if s := shares[0]; deref(s.Exclusive) != true || s.ExclusivePool != "cache" || deref(s.ExclusiveEligible) != true {
		t.Errorf("system: %+v", s)
	}
	if s := shares[1]; deref(s.Exclusive) != false || !reflect.DeepEqual(locs(s.Locations), []string{"cache", "disk1", "disk5"}) {
		t.Errorf("appdata: %+v", s)
	}
	if s := shares[2]; s.Exclusive != nil || s.ExclusiveEligible != nil {
		t.Errorf("old: %+v", s)
	}
}

func TestBranchAwake(t *testing.T) {
	spinning := map[int]bool{1: true, 2: false, 3: true, 4: true}
	for _, tt := range []struct {
		kv   map[string]string
		pool bool
		want bool
	}{
		{map[string]string{"idx": "1"}, false, true},
		{map[string]string{"idx": "1", "slots": "x"}, false, true}, // disks ignore slots
		{map[string]string{"idx": "2"}, false, false},              // spun down
		{map[string]string{"idx": "9"}, false, false},              // no spin state
		{map[string]string{"spundown": "0"}, false, false},         // no idx
		{map[string]string{"idx": "3", "slots": "2"}, true, true},  // pool 3-4
		{map[string]string{"idx": "4", "slots": "2"}, true, false}, // member 5 unknown
		{map[string]string{"idx": "3"}, true, false},               // pool size unknown
		{map[string]string{"idx": "3", "slots": "0"}, true, false},
		{map[string]string{"idx": "3", "slots": "x"}, true, false},
	} {
		if got := branchAwake(tt.kv, spinning, tt.pool); got != tt.want {
			t.Errorf("branchAwake(%v, pool=%v) = %v, want %v", tt.kv, tt.pool, got, tt.want)
		}
	}
}

func TestReleaseBefore(t *testing.T) {
	for version, want := range map[string]bool{
		"6.12.10":    true,
		"7.1.4":      true,
		"7.2":        false,
		"7.2.0-rc.1": false,
		"7.10.0":     false,
		"8.0.0":      false,
		"7.1-beta":   true,
		"":           false,
		"7":          false,
		"x.y.z":      false,
	} {
		if got := releaseBefore(version, 7, 2); got != want {
			t.Errorf("releaseBefore(%q, 7, 2) = %v, want %v", version, got, want)
		}
	}
}

func TestSharePathsDefaults(t *testing.T) {
	var p sharePaths
	if p.mnt() != "/mnt" || p.shares() != "/var/local/emhttp/shares.ini" ||
		p.vars() != "/var/local/emhttp/var.ini" || p.disks() != "/var/local/emhttp/disks.ini" {
		t.Errorf("unexpected defaults: %s %s %s %s", p.mnt(), p.shares(), p.vars(), p.disks())
	}
}
