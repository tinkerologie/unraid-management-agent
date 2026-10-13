package collectors

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/tinkerologie/unraid-management-agent/daemon/domain"
	"github.com/tinkerologie/unraid-management-agent/daemon/dto"
)

func readSmartctlFixture(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "smartctl", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return string(data)
}

// TestParseSmartctlJSON checks the fields taken from real `smartctl -j -a`
// output of an NVMe, a SAS and a SATA drive (identifiers removed), and the
// standby, failure and malformed cases.
func TestParseSmartctlJSON(t *testing.T) {
	now := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name        string
		output      string
		want        *dto.DriveHealth
		wantStandby bool
		wantErr     bool
	}{
		{
			name:   "nvme",
			output: readSmartctlFixture(t, "nvme.json"),
			want: &dto.DriveHealth{
				Model:              "MKNSSDPE2TB-D8",
				Protocol:           "NVMe",
				MediaType:          "nvme",
				SmartPassed:        new(true),
				TemperatureCelsius: new(35),
				PercentageUsed:     new(89),
				AvailableSpare:     new(100),
				CriticalWarning:    new(0),
				MediaErrors:        new(uint64(0)),
				PowerOnHours:       new(uint64(31520)),
				CollectedAt:        now,
			},
		},
		{
			name:   "sas hdd",
			output: readSmartctlFixture(t, "scsi.json"),
			want: &dto.DriveHealth{
				Model:              "SEAGATE ST24000NM007H",
				Protocol:           "SCSI",
				MediaType:          "hdd",
				SmartPassed:        new(true),
				TemperatureCelsius: new(37),
				MediaErrors:        new(uint64(0)),
				PowerOnHours:       new(uint64(14219)),
				CollectedAt:        now,
			},
		},
		{
			name:   "sata ssd",
			output: readSmartctlFixture(t, "ata.json"),
			want: &dto.DriveHealth{
				Model:              "Micron_5400_MTFDDAK7T6TGA",
				Protocol:           "ATA",
				MediaType:          "ssd",
				SmartPassed:        new(true),
				TemperatureCelsius: new(32),
				MediaErrors:        new(uint64(0)),
				PowerOnHours:       new(uint64(14791)),
				CollectedAt:        now,
			},
		},
		{
			name:        "standby",
			output:      readSmartctlFixture(t, "standby.json"),
			wantStandby: true,
		},
		{
			name:        "scsi low power",
			output:      `{"smartctl":{"messages":[{"string":"Device is in LOW POWER mode, exit(2)","severity":"information"}],"exit_status":2},"device":{"protocol":"SCSI"}}`,
			wantStandby: true,
		},
		{
			name:    "ata power check unsupported (fails closed)",
			output:  `{"smartctl":{"messages":[{"string":"CHECK POWER MODE not implemented, exit(3)","severity":"information"}],"exit_status":3},"device":{"protocol":"ATA"}}`,
			wantErr: true,
		},
		{
			name:    "ata power check unknown value (fails closed)",
			output:  `{"smartctl":{"messages":[{"string":"CHECK POWER MODE returned unknown value 0x42, exit(3)","severity":"information"}],"exit_status":3},"device":{"protocol":"ATA"}}`,
			wantErr: true,
		},
		{
			name:    "open failed",
			output:  readSmartctlFixture(t, "open-failed.json"),
			wantErr: true,
		},
		{
			name:    "empty",
			output:  "  \n",
			wantErr: true,
		},
		{
			name:    "not json",
			output:  "smartctl: command not found",
			wantErr: true,
		},
		{
			name: "failing drive, scsi counters partly present, oversized nvme counter",
			output: `{"smartctl":{"exit_status":8},"device":{"protocol":"SCSI"},"model_name":" X ",
				"rotation_rate":7200,"smart_status":{"passed":false},
				"scsi_error_counter_log":{"read":{"total_uncorrected_errors":2},"verify":{"total_uncorrected_errors":3}},
				"ata_smart_attributes":{"table":[{"id":187,"raw":{"value":9}}]},
				"nvme_smart_health_information_log":{"media_errors":340282366920938463463374607431768211455}}`,
			want: &dto.DriveHealth{
				Model:       "X",
				Protocol:    "SCSI",
				MediaType:   "hdd",
				SmartPassed: new(false),
				MediaErrors: new(uint64(5)),
				CollectedAt: now,
			},
		},
		{
			name: "scsi log without uncorrected counters, ata without attribute 187",
			output: `{"device":{"protocol":"ATA"},
				"scsi_error_counter_log":{"read":{"total_errors_corrected":1}},
				"ata_smart_attributes":{"table":[{"id":5,"raw":{"value":1}}]}}`,
			want: &dto.DriveHealth{Protocol: "ATA", CollectedAt: now},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, standby, err := parseSmartctlJSON(tt.output, now)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if standby != tt.wantStandby {
				t.Errorf("standby = %v, want %v", standby, tt.wantStandby)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("health = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// fakeSysfs builds a sysfs/dev tree: an NVMe disk with a partition, a whole
// SATA disk with a partition, a LUKS mapping over a partition, a by-id link,
// and broken device-mapper chains.
func fakeSysfs(t *testing.T) (sysRoot, devRoot string) {
	t.Helper()
	root := t.TempDir()
	sysRoot = filepath.Join(root, "sys")
	devRoot = filepath.Join(root, "dev")
	block := filepath.Join(sysRoot, "class", "block")

	mkdir := func(p string) {
		t.Helper()
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	touch := func(p string) {
		t.Helper()
		mkdir(filepath.Dir(p))
		if err := os.WriteFile(p, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	link := func(target, name string) {
		t.Helper()
		mkdir(filepath.Dir(name))
		if err := os.Symlink(target, name); err != nil {
			t.Fatal(err)
		}
	}
	addDisk := func(disk string, parts ...string) {
		dir := filepath.Join(sysRoot, "devices", "pci", disk)
		mkdir(dir)
		link(dir, filepath.Join(block, disk))
		if err := os.WriteFile(filepath.Join(dir, "diskseq"), []byte("7\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		for _, part := range parts {
			touch(filepath.Join(dir, part, "partition"))
			link(filepath.Join(dir, part), filepath.Join(block, part))
		}
	}

	addDisk("nvme0n1", "nvme0n1p1")
	addDisk("sdb", "sdb1")
	addDisk("sdc", "sdc1")
	// sdb is a SAS SSD, sdd a SAS hard drive; sde has no rotational flag.
	touch(filepath.Join(sysRoot, "devices", "pci", "sdb", "queue", "rotational"))
	if err := os.WriteFile(filepath.Join(sysRoot, "devices", "pci", "sdb", "queue", "rotational"), []byte("0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	addDisk("sde")
	addDisk("sdd")
	touch(filepath.Join(sysRoot, "devices", "pci", "sdd", "queue", "rotational"))
	if err := os.WriteFile(filepath.Join(sysRoot, "devices", "pci", "sdd", "queue", "rotational"), []byte("1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	touch(filepath.Join(sysRoot, "devices", "pci", "sdc", "device", "vendor"))
	if err := os.WriteFile(filepath.Join(sysRoot, "devices", "pci", "sdc", "device", "vendor"), []byte("ATA     \n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// A USB-attached disk.
	usb := filepath.Join(sysRoot, "devices", "pci", "usb2", "2-1", "sdu")
	mkdir(usb)
	link(usb, filepath.Join(block, "sdu"))
	// A virtio disk smartctl can't read.
	addDisk("vda")

	// dm-0 is a LUKS mapping on sdc1, reachable as /dev/mapper/crypt1.
	touch(filepath.Join(block, "dm-0", "slaves", "sdc1"))
	touch(filepath.Join(devRoot, "dm-0"))
	link(filepath.Join(devRoot, "dm-0"), filepath.Join(devRoot, "mapper", "crypt1"))

	// By-id links to sdb1 and nvme0n1p1 (EUI aliases contain a dot).
	touch(filepath.Join(devRoot, "sdb1"))
	link(filepath.Join(devRoot, "sdb1"), filepath.Join(devRoot, "disk", "by-id", "ata-MODEL-part1"))
	touch(filepath.Join(devRoot, "nvme0n1p1"))
	link(filepath.Join(devRoot, "nvme0n1p1"), filepath.Join(devRoot, "disk", "by-id", "nvme-eui.0025385a01234567-part1"))
	// A by-id link pointing at something that is not a block device name.
	touch(filepath.Join(devRoot, "odd name"))
	link(filepath.Join(devRoot, "odd name"), filepath.Join(devRoot, "disk", "by-id", "odd-alias"))

	// dm-1 -> dm-2 -> dm-3 -> dm-4: deeper than the resolver follows.
	for i, next := range []string{"dm-2", "dm-3", "dm-4", "dm-5"} {
		touch(filepath.Join(block, "dm-"+string(rune('1'+i)), "slaves", next))
	}
	// dm-9 names a slave that does not exist.
	touch(filepath.Join(block, "dm-9", "slaves", "ghost"))

	return sysRoot, devRoot
}

func TestResolveWholeDisk(t *testing.T) {
	sysRoot, devRoot := fakeSysfs(t)

	tests := []struct {
		member string
		want   string
	}{
		{"nvme0n1p1", "nvme0n1"},
		{"sdb", "sdb"},
		{"sdb1", "sdb"},
		{"/dev/sdb1", "sdb"},
		{"crypt1", "sdc"},
		{"dm-0", "sdc"},
		{"ata-MODEL-part1", "sdb"},
		{"nvme-eui.0025385a01234567-part1", "nvme0n1"},
		{"odd-alias", ""},
		{"..", ""},
		{"/", ""},
		{"12345678901234567890", ""}, // missing device shown by GUID
		{"bad name!", ""},
		{"dm-1", ""}, // too many layers
		{"dm-9", ""}, // dangling slave
	}
	for _, tt := range tests {
		if got := resolveWholeDisk(sysRoot, devRoot, tt.member); got != tt.want {
			t.Errorf("resolveWholeDisk(%q) = %q, want %q", tt.member, got, tt.want)
		}
	}
}

func TestPoolDrives(t *testing.T) {
	pool := &dto.ZFSPool{VDEVs: []dto.ZFSVdev{
		{Name: "raidz2-0", Type: "raidz2", Devices: []dto.ZFSDevice{
			{Name: "sdb1", State: "ONLINE"},
			{Name: "nvme0n1p1", State: "DEGRADED"},
		}},
		{Name: "sdc1", Type: "disk", State: "ONLINE"},
		{Name: "mirror-1", Type: "mirror", State: "ONLINE"}, // no devices parsed
	}}
	want := []dto.ZFSDrive{
		{Name: "sdb1", Vdev: "raidz2-0", State: "ONLINE"},
		{Name: "nvme0n1p1", Vdev: "raidz2-0", State: "DEGRADED"},
		{Name: "sdc1", State: "ONLINE"},
	}
	if got := poolDrives(pool); !reflect.DeepEqual(got, want) {
		t.Errorf("poolDrives() = %+v, want %+v", got, want)
	}
	if got := poolDrives(&dto.ZFSPool{}); got != nil {
		t.Errorf("poolDrives(no vdevs) = %+v, want nil", got)
	}
}

// fakeSmartctl serves smartctl output per disk and records the calls.
type fakeSmartctl struct {
	mu       sync.Mutex
	outputs  map[string]string
	calls    []string
	types    map[string]string
	inflight map[string]chan struct{}
}

func (f *fakeSmartctl) run(_ time.Duration, disk, smartType string) (string, <-chan struct{}, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, disk)
	if f.types != nil {
		f.types[disk] = smartType
	}
	done := f.inflight[disk]
	if done == nil {
		done = make(chan struct{})
		close(done)
	}
	out, ok := f.outputs[disk]
	if !ok {
		return "", done, errors.New("exit status 2")
	}
	return out, done, nil
}

func (f *fakeSmartctl) takeCalls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	calls := f.calls
	f.calls = nil
	return calls
}

func newTestCache(t *testing.T, runner *fakeSmartctl, clock *time.Time) *driveHealthCache {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "smartctl")
	if err := os.WriteFile(binary, nil, 0o755); err != nil {
		t.Fatal(err)
	}
	cache := newDriveHealthCache()
	cache.binary = binary
	cache.slots = make(chan struct{}, 2)
	cache.now = func() time.Time { return *clock }
	cache.run = runner.run
	return cache
}

// refs turns disk names into diskRefs read as SCSI with no known generation.
func refs(names ...string) []diskRef {
	out := make([]diskRef, len(names))
	for i, name := range names {
		out[i] = diskRef{name: name, smartType: "scsi"}
	}
	return out
}

func sameSet(a, b []string) bool {
	seen := map[string]int{}
	for _, s := range a {
		seen[s]++
	}
	for _, s := range b {
		seen[s]--
	}
	for _, n := range seen {
		if n != 0 {
			return false
		}
	}
	return len(a) == len(b)
}

func TestDriveHealthCacheLookup(t *testing.T) {
	clock := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	start := clock
	runner := &fakeSmartctl{outputs: map[string]string{
		"nvme0n1": readSmartctlFixture(t, "nvme.json"),
		"sdb":     readSmartctlFixture(t, "scsi.json"),
		"sdx":     readSmartctlFixture(t, "standby.json"),
	}}
	cache := newTestCache(t, runner, &clock)

	// First lookup reads every distinct, valid disk once.
	got := cache.lookup(refs("nvme0n1", "sdb", "nvme0n1", "sdx", "sdfail", "bad/name"))
	if calls := runner.takeCalls(); !sameSet(calls, []string{"nvme0n1", "sdb", "sdx", "sdfail"}) {
		t.Fatalf("calls = %v", calls)
	}
	if got["nvme0n1"] == nil || *got["nvme0n1"].PercentageUsed != 89 || got["nvme0n1"].Standby {
		t.Errorf("nvme0n1 = %+v", got["nvme0n1"])
	}
	if got["sdb"] == nil || got["sdb"].MediaType != "hdd" {
		t.Errorf("sdb = %+v", got["sdb"])
	}
	if want := (&dto.DriveHealth{Standby: true}); !reflect.DeepEqual(got["sdx"], want) {
		t.Errorf("sdx (asleep, never read) = %+v, want %+v", got["sdx"], want)
	}
	if _, ok := got["sdfail"]; ok {
		t.Errorf("sdfail should be absent, got %+v", got["sdfail"])
	}

	// Within the TTL nothing is re-read.
	clock = clock.Add(time.Minute)
	cache.lookup(refs("nvme0n1", "sdb", "sdx", "sdfail"))
	if calls := runner.takeCalls(); len(calls) != 0 {
		t.Fatalf("calls within TTL = %v", calls)
	}

	// After the TTL: sdb is now asleep and keeps its last reading; nvme0n1
	// fails and keeps its last reading; sdfail now reads; sdx is dropped.
	runner.mu.Lock()
	runner.outputs["sdb"] = readSmartctlFixture(t, "standby.json")
	delete(runner.outputs, "nvme0n1")
	runner.outputs["sdfail"] = readSmartctlFixture(t, "ata.json")
	runner.mu.Unlock()
	clock = clock.Add(driveHealthTTL)
	got = cache.lookup(refs("nvme0n1", "sdb", "sdfail"))
	if calls := runner.takeCalls(); !sameSet(calls, []string{"nvme0n1", "sdb", "sdfail"}) {
		t.Fatalf("calls after TTL = %v", calls)
	}
	if h := got["sdb"]; h == nil || !h.Standby || h.MediaType != "hdd" || !h.CollectedAt.Equal(start) {
		t.Errorf("sdb (asleep after a read) = %+v", h)
	}
	if h := got["nvme0n1"]; h == nil || h.Standby || !h.CollectedAt.Equal(start) {
		t.Errorf("nvme0n1 (failed re-read) = %+v", h)
	}
	if h := got["sdfail"]; h == nil || h.Protocol != "ATA" {
		t.Errorf("sdfail = %+v", h)
	}
	if _, ok := cache.entries["sdx"]; ok {
		t.Error("sdx entry should have been pruned")
	}

	// sdb wakes up: standby clears.
	runner.mu.Lock()
	runner.outputs["sdb"] = readSmartctlFixture(t, "scsi.json")
	runner.mu.Unlock()
	clock = clock.Add(driveHealthTTL)
	got = cache.lookup(refs("sdb"))
	if h := got["sdb"]; h == nil || h.Standby || !h.CollectedAt.Equal(clock) {
		t.Errorf("sdb (awake again) = %+v", h)
	}
	runner.takeCalls()
}

func TestDriveHealthCacheSkipsStuckSmartctl(t *testing.T) {
	clock := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	stuck := make(chan struct{})
	runner := &fakeSmartctl{
		outputs:  map[string]string{"sdb": readSmartctlFixture(t, "scsi.json")},
		inflight: map[string]chan struct{}{"sdb": stuck},
	}
	cache := newTestCache(t, runner, &clock)

	cache.lookup(refs("sdb"))
	runner.takeCalls()

	clock = clock.Add(driveHealthTTL)
	got := cache.lookup(refs("sdb"))
	if calls := runner.takeCalls(); len(calls) != 0 {
		t.Fatalf("smartctl re-run while the previous one is stuck: %v", calls)
	}
	if got["sdb"] == nil {
		t.Error("cached reading should still be returned")
	}

	close(stuck)
	cache.lookup(refs("sdb"))
	if calls := runner.takeCalls(); len(calls) != 1 {
		t.Fatalf("calls after the stuck run exited = %v", calls)
	}
}

func TestDriveHealthCacheWithoutSmartctl(t *testing.T) {
	cache := newDriveHealthCache()
	cache.binary = filepath.Join(t.TempDir(), "missing")
	cache.run = func(time.Duration, string, string) (string, <-chan struct{}, error) {
		t.Fatal("smartctl must not run when the binary is missing")
		return "", nil, nil
	}
	if got := cache.lookup(refs("sdb")); got != nil {
		t.Errorf("lookup() = %v, want nil", got)
	}
}

func TestRunSmartctlReportsStartFailure(t *testing.T) {
	// smartctl is not installed on the test host (or /dev/zz-none does not
	// exist), so either the start or the run fails.
	if _, _, err := runSmartctl(time.Second, "zz-none", "scsi"); err == nil {
		t.Error("runSmartctl() on a missing device returned no error")
	}
}

func TestAttachDrives(t *testing.T) {
	sysRoot, devRoot := fakeSysfs(t)
	clock := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	runner := &fakeSmartctl{outputs: map[string]string{
		"nvme0n1": readSmartctlFixture(t, "nvme.json"),
		"sdb":     readSmartctlFixture(t, "scsi.json"),
	}, types: map[string]string{}}
	collector := &ZFSCollector{sysRoot: sysRoot, devRoot: devRoot, driveHealth: newTestCache(t, runner, &clock)}

	pools := []dto.ZFSPool{
		{Name: "fast", VDEVs: []dto.ZFSVdev{{Name: "mirror-0", Type: "mirror", Devices: []dto.ZFSDevice{
			{Name: "nvme0n1p1", State: "ONLINE"},
			{Name: "9876543210", State: "UNAVAIL"},
		}}}},
		{Name: "slow", VDEVs: []dto.ZFSVdev{{Name: "sdb1", Type: "disk", State: "ONLINE"}}},
		{Name: "unknown"},
	}
	collector.attachDrives(pools)

	fast := pools[0].Drives
	if len(fast) != 2 || fast[0].Disk != "nvme0n1" || fast[0].Health == nil || *fast[0].Health.PercentageUsed != 89 {
		t.Errorf("fast drives = %+v", fast)
	}
	if fast[1].Disk != "" || fast[1].Health != nil {
		t.Errorf("missing member = %+v", fast[1])
	}
	slow := pools[1].Drives
	if len(slow) != 1 || slow[0].Disk != "sdb" || slow[0].Vdev != "" || slow[0].Health == nil || slow[0].Health.Protocol != "SCSI" {
		t.Errorf("slow drives = %+v", slow)
	}
	if want := map[string]string{"nvme0n1": "nvme", "sdb": "scsi"}; !reflect.DeepEqual(runner.types, want) {
		t.Errorf("smartctl device types = %v, want %v", runner.types, want)
	}
	if pools[2].Drives != nil {
		t.Errorf("pool without status should have no drives, got %+v", pools[2].Drives)
	}

	// Without a health cache the members are still listed.
	pools = []dto.ZFSPool{{Name: "slow", VDEVs: []dto.ZFSVdev{{Name: "sdb1", Type: "disk", State: "ONLINE"}}}}
	(&ZFSCollector{sysRoot: sysRoot, devRoot: devRoot}).attachDrives(pools)
	if d := pools[0].Drives; len(d) != 1 || d[0].Disk != "sdb" || d[0].Health != nil {
		t.Errorf("drives without health cache = %+v", d)
	}
}

// TestCollectPoolsAttachesDrives runs collectPools against canned zpool
// output and checks each pool comes back with its members and their health.
func TestCollectPoolsAttachesDrives(t *testing.T) {
	sysRoot, devRoot := fakeSysfs(t)
	clock := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	runner := &fakeSmartctl{outputs: map[string]string{"nvme0n1": readSmartctlFixture(t, "nvme.json")}}

	status := "  pool: fast\n state: ONLINE\nconfig:\n\n\tNAME           STATE     READ WRITE CKSUM\n" +
		"\tfast           ONLINE       0     0     0\n" +
		"\t  mirror-0     ONLINE       0     0     0\n" +
		"\t    nvme0n1p1  ONLINE       0     0     0\n" +
		"\t    sdb1       ONLINE       0     0     0\n\nerrors: No known data errors\n"
	exec := func(_ string, args ...string) (string, error) {
		switch args[0] {
		case "list":
			if len(args) == 4 { // list -H -o name
				return "fast\n", nil
			}
			return "fast\t1000\t400\t600\t5\t40\t1.00\tONLINE\t-\n", nil
		case "get":
			return "guid\t42\nautotrim\ton\n", nil
		case "status":
			return status, nil
		}
		return "", errors.New("unexpected zpool call")
	}
	collector := &ZFSCollector{execOutput: exec, sysRoot: sysRoot, devRoot: devRoot,
		driveHealth: newTestCache(t, runner, &clock)}

	pools, err := collector.collectPools()
	if err != nil {
		t.Fatalf("collectPools() error = %v", err)
	}
	if len(pools) != 1 || pools[0].GUID != "42" {
		t.Fatalf("pools = %+v", pools)
	}
	drives := pools[0].Drives
	if len(drives) != 2 || drives[0].Disk != "nvme0n1" || drives[0].Vdev != "mirror-0" ||
		drives[0].Health == nil || drives[1].Disk != "sdb" || drives[1].Health != nil {
		t.Errorf("drives = %+v", drives)
	}
}

func TestDescribeDisk(t *testing.T) {
	sysRoot, _ := fakeSysfs(t)
	spin := map[string]bool{"nvme0n1": false, "sdc": true}
	tests := []struct {
		disk string
		spin map[string]bool
		want diskRef
	}{
		{"nvme0n1", spin, diskRef{name: "nvme0n1", generation: "7", smartType: "nvme"}},
		{"sdb", spin, diskRef{name: "sdb", generation: "7", smartType: "scsi"}}, // SAS SSD
		{"sdc", spin, diskRef{name: "sdc", generation: "7", smartType: "sat", asleep: true}},
		// SAS hard drive: read only when Unraid manages it.
		{"sdd", nil, diskRef{name: "sdd", generation: "7", unraidOnly: true}},
		{"sdd", map[string]bool{"sdd": false}, diskRef{name: "sdd", generation: "7", smartType: "scsi", unraidOnly: true}},
		{"sdd", map[string]bool{"sdd": true}, diskRef{name: "sdd", generation: "7", smartType: "scsi", asleep: true, unraidOnly: true}},
		// Unknown rotational flag: treated as a hard drive.
		{"sde", nil, diskRef{name: "sde", generation: "7", unraidOnly: true}},
		{"sdu", spin, diskRef{name: "sdu"}},                  // USB bridge: not read
		{"vda", spin, diskRef{name: "vda", generation: "7"}}, // no SMART transport
		{"sdz", spin, diskRef{name: "sdz"}},                  // gone
	}
	for _, tt := range tests {
		if got := describeDisk(sysRoot, tt.disk, tt.spin); got != tt.want {
			t.Errorf("describeDisk(%q, %v) = %+v, want %+v", tt.disk, tt.spin, got, tt.want)
		}
	}
}

func TestUnraidSpinStates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "disks.ini")
	ini := `["parity"]
name="parity"
device=""
spundown="0"
["exos24tb"]
name="exos24tb"
device="sdy"
spundown="0"
["exos24tb2"]
name="exos24tb2"
device="sdz"
spundown="1"
["flash"]
name="flash"
device="sda"
["odd"]
device="sdb"
spundown=""
["odder"]
device="sdc"
spundown="2"
["slotless"]
device=""
spundown="0"
`
	if err := os.WriteFile(path, []byte(ini), 0o644); err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"sdy": false, "sdz": true}
	if got := unraidSpinStates(path); !reflect.DeepEqual(got, want) {
		t.Errorf("unraidSpinStates() = %v, want %v", got, want)
	}
	if got := unraidSpinStates(filepath.Join(t.TempDir(), "missing.ini")); got != nil {
		t.Errorf("missing disks.ini = %v, want nil", got)
	}
}

// TestDriveHealthCacheDiskReplaced checks that a reading is dropped when a
// different disk (new diskseq) appears under the same name.
func TestDriveHealthCacheDiskReplaced(t *testing.T) {
	clock := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	runner := &fakeSmartctl{outputs: map[string]string{"sdb": readSmartctlFixture(t, "scsi.json")}}
	cache := newTestCache(t, runner, &clock)

	if got := cache.lookup([]diskRef{{name: "sdb", generation: "1", smartType: "scsi"}}); got["sdb"] == nil {
		t.Fatal("first disk not read")
	}
	runner.takeCalls()

	// Replacement disk under the same name, asleep: re-read at once, and the
	// old disk's reading must not be reported for it.
	runner.mu.Lock()
	runner.outputs["sdb"] = readSmartctlFixture(t, "standby.json")
	runner.mu.Unlock()
	got := cache.lookup([]diskRef{{name: "sdb", generation: "2", smartType: "scsi"}})
	if calls := runner.takeCalls(); len(calls) != 1 {
		t.Fatalf("replacement disk not re-read within the TTL: %v", calls)
	}
	if want := (&dto.DriveHealth{Standby: true}); !reflect.DeepEqual(got["sdb"], want) {
		t.Errorf("replacement disk = %+v, want %+v", got["sdb"], want)
	}
}

func TestDriveHealthCacheSkipsUnreadableDisks(t *testing.T) {
	clock := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	runner := &fakeSmartctl{outputs: map[string]string{}}
	cache := newTestCache(t, runner, &clock)
	got := cache.lookup([]diskRef{{name: "sdu"}})
	if calls := runner.takeCalls(); len(calls) != 0 || len(got) != 0 {
		t.Errorf("calls = %v, result = %v; a disk without a SMART type must not be read", calls, got)
	}
}

// TestDriveHealthCacheHoldsSlotsForStuckProcesses checks that a smartctl
// that timed out but has not exited keeps its process slot, and that disks
// waiting for a slot give up instead of blocking the collection.
func TestDriveHealthCacheHoldsSlotsForStuckProcesses(t *testing.T) {
	clock := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	stuck := make(chan struct{})
	runner := &fakeSmartctl{
		outputs:  map[string]string{"sdb": readSmartctlFixture(t, "scsi.json")},
		inflight: map[string]chan struct{}{"sda": stuck},
	}
	cache := newTestCache(t, runner, &clock)
	cache.slots = make(chan struct{}, 1)
	cache.timeout = 20 * time.Millisecond

	cache.lookup(refs("sda"))
	if calls := runner.takeCalls(); len(calls) != 1 {
		t.Fatalf("calls = %v", calls)
	}

	start := time.Now()
	got := cache.lookup(refs("sda", "sdb"))
	if calls := runner.takeCalls(); len(calls) != 0 {
		t.Fatalf("smartctl started while the only slot is held by a stuck process: %v", calls)
	}
	if time.Since(start) > 5*time.Second {
		t.Error("lookup waited far longer than the queue timeout")
	}
	if _, ok := got["sdb"]; ok {
		t.Error("sdb should not have been read")
	}

	// Once the stuck process exits, its slot frees and sdb is read.
	close(stuck)
	deadline := time.Now().Add(5 * time.Second)
	for len(cache.slots) > 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	got = cache.lookup(refs("sda", "sdb"))
	if calls := runner.takeCalls(); !sameSet(calls, []string{"sdb"}) {
		t.Fatalf("calls after the stuck process exited = %v", calls)
	}
	if got["sdb"] == nil {
		t.Error("sdb not read after the slot freed")
	}
}

// TestDriveHealthCacheSkipsSpunDownDisks checks that a disk Unraid reports
// spun down is never read, keeps its last reading flagged as standby, and is
// read again as soon as it spins up.
func TestDriveHealthCacheSkipsSpunDownDisks(t *testing.T) {
	clock := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	runner := &fakeSmartctl{outputs: map[string]string{"sdd": readSmartctlFixture(t, "scsi.json")}}
	cache := newTestCache(t, runner, &clock)
	awake := diskRef{name: "sdd", smartType: "scsi"}
	asleep := diskRef{name: "sdd", smartType: "scsi", asleep: true}

	// Asleep before it was ever read: not read, reported as standby.
	got := cache.lookup([]diskRef{asleep})
	if calls := runner.takeCalls(); len(calls) != 0 {
		t.Fatalf("spun-down disk was read: %v", calls)
	}
	if want := (&dto.DriveHealth{Standby: true}); !reflect.DeepEqual(got["sdd"], want) {
		t.Errorf("asleep, never read = %+v, want %+v", got["sdd"], want)
	}

	// Spins up: read at once.
	clock = clock.Add(time.Minute)
	got = cache.lookup([]diskRef{awake})
	if calls := runner.takeCalls(); len(calls) != 1 {
		t.Fatalf("calls after spin-up = %v", calls)
	}
	if h := got["sdd"]; h == nil || h.Standby {
		t.Errorf("awake reading = %+v", h)
	}

	// Spun down again within the TTL: last reading kept, flagged standby.
	clock = clock.Add(time.Minute)
	got = cache.lookup([]diskRef{asleep})
	if h := got["sdd"]; h == nil || !h.Standby || h.Protocol != "SCSI" {
		t.Errorf("asleep after a read = %+v", h)
	}
	// Spins up again within the TTL: re-read straight away.
	clock = clock.Add(time.Minute)
	got = cache.lookup([]diskRef{awake})
	if calls := runner.takeCalls(); len(calls) != 1 {
		t.Fatalf("calls after second spin-up = %v", calls)
	}
	if h := got["sdd"]; h == nil || h.Standby || !h.CollectedAt.Equal(clock) {
		t.Errorf("re-read after spin-up = %+v", h)
	}
}

// TestDriveHealthCacheRechecksSpinStateBeforeReading checks that Unraid's
// spin state is read again right before smartctl runs, so a disk spun down
// (or no longer managed) after the snapshot is not read.
func TestDriveHealthCacheRechecksSpinStateBeforeReading(t *testing.T) {
	clock := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	runner := &fakeSmartctl{outputs: map[string]string{
		"sdd": readSmartctlFixture(t, "scsi.json"),
		"sde": readSmartctlFixture(t, "scsi.json"),
		"sdf": readSmartctlFixture(t, "scsi.json"),
	}}
	cache := newTestCache(t, runner, &clock)
	cache.spinStates = func() map[string]bool {
		// sdd spun down meanwhile, sde left Unraid, sdf still spinning.
		return map[string]bool{"sdd": true, "sdf": false}
	}
	got := cache.lookup([]diskRef{
		{name: "sdd", smartType: "scsi", unraidOnly: true},
		{name: "sde", smartType: "scsi", unraidOnly: true},
		{name: "sdf", smartType: "scsi", unraidOnly: true},
	})
	if calls := runner.takeCalls(); !sameSet(calls, []string{"sdf"}) {
		t.Fatalf("calls = %v, want only sdf", calls)
	}
	if got["sdf"] == nil || got["sdd"] != nil || got["sde"] != nil {
		t.Errorf("result = %v", got)
	}
	// The read that ran frees its slot once its process is reaped (in the
	// background); the skipped ones must not hold any.
	deadline := time.Now().Add(5 * time.Second)
	for len(cache.slots) > 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if len(cache.slots) != 0 {
		t.Errorf("skipped reads left %d process slots taken", len(cache.slots))
	}
}

func TestNewZFSCollectorChecksDisksIni(t *testing.T) {
	c := NewZFSCollector(&domain.Context{Hub: domain.NewEventBus(10)})
	path := filepath.Join(t.TempDir(), "disks.ini")
	if err := os.WriteFile(path, []byte("[\"d\"]\ndevice=\"sdy\"\nspundown=\"1\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c.disksIni = path
	if got := c.driveHealth.spinStates(); !got["sdy"] {
		t.Errorf("spinStates() = %v, want sdy spun down", got)
	}
}
