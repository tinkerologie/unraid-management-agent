package collectors

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/tinkerologie/unraid-management-agent/daemon/domain"
)

func usageCollector(t *testing.T, content string) *ShareCollector {
	t.Helper()
	dir := t.TempDir()
	p := sharePaths{sharesIni: filepath.Join(dir, "shares.ini"), disksIni: filepath.Join(dir, "disks.ini"), varIni: filepath.Join(dir, "var.ini"), bootConfigDir: dir}
	for path, data := range map[string]string{p.sharesIni: content, p.disksIni: strings.Replace(exclusiveDisksIni, "[\"cache\"]", "[\"cache\"]\nfsType=\"zfs\"", 1)} {
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return &ShareCollector{ctx: &domain.Context{Intervals: domain.Intervals{ZFS: 60}}, paths: p,
		runCommand: func(context.Context, string, ...string) (string, error) {
			t.Fatal("unexpected command")
			return "", nil
		}}
}

func TestShareStorageCounters(t *testing.T) {
	for _, tt := range []struct {
		name, counters    string
		used, free, total uint64
		percent           float64
	}{
		{"derived total", "used=2\nfree=6\nsize=0", 2048, 6144, 8192, 25},
		{"preserve nonzero size", "used=2\nfree=6\nsize=16", 2048, 6144, 16384, 12.5},
		{"missing", "", 0, 0, 0, 0},
		{"invalid", "used=-1\nfree=bad\nsize=bad", 0, 0, 0, 0},
		{"partial", "used=bad\nfree=4", 0, 4096, 4096, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := usageCollector(t, "[shareName=\"media\"]\nuseCache=no\n"+tt.counters)
			shares, err := c.collectShares(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if len(shares) != 1 {
				t.Fatalf("shares=%v", shares)
			}
			s := shares[0]
			if s.Used != nil || s.StorageUsed != tt.used || s.StorageFree != tt.free || s.StorageTotal != tt.total || s.StorageUsagePercent != tt.percent {
				t.Errorf("capacity=%+v", s)
			}
			if s.Free != tt.free || s.Total != tt.total || s.UsagePercent != tt.percent {
				t.Errorf("aliases=%+v", s)
			}
		})
	}
}

func TestShareMeasuredUsage(t *testing.T) {
	for _, tt := range []struct {
		name, out string
		err       error
		want      *uint64
	}{
		{"descendants", "cache/media\t10\ncache/media/child\t20\ncache/media/child/grandchild\t3\ncache/mediabox\t999\nother/media\t888\ncache/media@snap\t999", nil, new(uint64(33))},
		{"known zero", "cache/media\t0", nil, new(uint64(0))},
		{"wrong pool", "other/media\t123", nil, nil},
		{"missing direct", "cache/media/child\t123", nil, nil},
		{"bad direct", "cache/media\tinvalid", nil, nil},
		{"bad child", "cache/media\t1\ncache/media/child\tinvalid", nil, nil},
		{"missing value", "cache/media", nil, nil},
		{"extra column", "cache/media\t1\t2", nil, nil},
		{"overflow", "cache/media\t18446744073709551615\ncache/media/child\t1", nil, nil},
		{"failed", "cache/media\t1", errors.New("failed"), nil},
		{"timeout", "", context.DeadlineExceeded, nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := usageCollector(t, "[shareName=\"media\"]\nuseCache=only\ncachePool=cache\nused=4\nfree=4")
			calls := 0
			c.runCommand = func(ctx context.Context, command string, args ...string) (string, error) {
				calls++
				want := []string{"list", "-Hp", "-t", "filesystem", "-o", "name,refer", "-r", "cache"}
				if command != "zfs" || !reflect.DeepEqual(args, want) {
					t.Errorf("command=%s %v", command, args)
				}
				deadline, ok := ctx.Deadline()
				if !ok || time.Until(deadline) > 60*time.Second {
					t.Error("missing bounded timeout")
				}
				return tt.out, tt.err
			}
			shares, err := c.collectShares(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if calls != 1 || !reflect.DeepEqual(shares[0].Used, tt.want) {
				t.Errorf("calls=%d Used=%v want=%v", calls, shares[0].Used, tt.want)
			}
			if shares[0].StorageUsed != 4096 || shares[0].UsagePercent != 50 {
				t.Errorf("measurement altered storage: %+v", shares[0])
			}
		})
	}
}

func TestShareUsageGates(t *testing.T) {
	for _, tt := range []struct {
		name, config, old, new string
		disabled, missing      bool
	}{
		{name: "ZFS disabled", disabled: true},
		{name: "array", config: "useCache=no\ncachePool=cache"},
		{name: "mixed yes", config: "useCache=yes\ncachePool=cache"},
		{name: "mixed prefer", config: "useCache=prefer\ncachePool=cache"},
		{name: "no pool", config: "useCache=only"},
		{name: "secondary pool", config: "useCache=only\ncachePool=cache\ncachePool2=slow"},
		{name: "invalid pool", config: "useCache=only\ncachePool=../cache"},
		{name: "command injection", config: "useCache=only\ncachePool=$(reboot)"},
		{name: "not a pool", config: "useCache=only\ncachePool=disk1"},
		{name: "non ZFS", old: "fsType=\"zfs\"", new: "fsType=\"btrfs\""},
		{name: "unknown filesystem", old: "fsType=\"zfs\"", new: ""},
		{name: "sleeping member", old: "idx=\"31\"\nname=\"cache2\"\ntype=\"Cache\"\nspundown=\"0\"", new: "idx=\"31\"\ntype=\"Cache\"\nspundown=\"1\""},
		{name: "unknown member", old: "idx=\"31\"\nname=\"cache2\"\ntype=\"Cache\"\nspundown=\"0\"", new: "idx=\"31\"\ntype=\"Cache\""},
		{name: "missing member", old: "idx=\"31\"", new: "idx=\"99\""},
		{name: "invalid member", old: "idx=\"31\"", new: "idx=\"bad\""},
		{name: "unknown slots", old: "slots=\"2\"", new: "slots=\"bad\""},
		{name: "overflow slots", old: "slots=\"2\"", new: "slots=\"9223372036854775807\""},
		{name: "duplicate slot", old: "idx=\"33\"", new: "idx=\"31\""},
		{name: "zero slots", old: "slots=\"2\"", new: "slots=\"0\""},
		{name: "unmounted", old: "slots=\"2\"\nspundown=\"0\"\nfsStatus=\"Mounted\"", new: "slots=\"2\"\nspundown=\"0\"\nfsStatus=\"Unmounted\""},
		{name: "unreadable disks", missing: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			config := tt.config
			if config == "" {
				config = "useCache=only\ncachePool=cache"
			}
			c := usageCollector(t, "[shareName=\"media\"]\n"+config)
			if tt.disabled {
				c.ctx.Intervals.ZFS = 0
			}
			if tt.old != "" {
				b, err := os.ReadFile(c.paths.disks())
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(c.paths.disks(), []byte(strings.Replace(string(b), tt.old, tt.new, 1)), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if tt.missing {
				if err := os.Remove(c.paths.disks()); err != nil {
					t.Fatal(err)
				}
			}
			shares, err := c.collectShares(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if shares[0].Used != nil {
				t.Errorf("unexpected measured usage: %v", *shares[0].Used)
			}
		})
	}
}

func TestShareUsageConfigAndPoolBatching(t *testing.T) {
	c := usageCollector(t, "[shareName=\"media\"]\ncachePool=cache\n[shareName=\"empty\"]\nuseCache=only\ncachePool=cache")
	if err := os.Mkdir(filepath.Join(c.paths.bootConfigDir, "shares"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(c.paths.bootConfigDir, "shares", "media.cfg"), []byte("shareUseCache=only"), 0600); err != nil {
		t.Fatal(err)
	}
	calls := 0
	c.runCommand = func(context.Context, string, ...string) (string, error) {
		calls++
		return "cache/media\t42\ncache/empty\t0", nil
	}
	shares, err := c.collectShares(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || shares[0].Used == nil || *shares[0].Used != 42 || shares[1].Used == nil || *shares[1].Used != 0 {
		t.Errorf("calls=%d shares=%+v", calls, shares)
	}
}

// A single snapshot must serve both usage enrichment and exclusive eligibility.
func TestShareUsageUsesOneDiskSnapshot(t *testing.T) {
	c := usageCollector(t, "[shareName=\"media\"]\nuseCache=only\ncachePool=cache\nexclusive=no")
	if err := os.WriteFile(c.paths.vars(), []byte(startedVarIni), 0600); err != nil {
		t.Fatal(err)
	}
	c.runCommand = func(context.Context, string, ...string) (string, error) {
		if err := os.Remove(c.paths.disks()); err != nil {
			t.Fatal(err)
		}
		return "cache/media\t7", nil
	}
	shares, err := c.collectShares(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if shares[0].Used == nil || shares[0].ExclusiveEligible == nil || !*shares[0].ExclusiveEligible {
		t.Errorf("snapshot not reused: %+v", shares[0])
	}
}
