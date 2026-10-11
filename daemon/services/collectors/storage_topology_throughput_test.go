package collectors

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

func TestParseDiskstats(t *testing.T) {
	tests := []struct {
		name string
		data string
		want map[string][2]uint64
	}{
		{
			name: "whole disks and partitions",
			data: "   8       0 sda 6521 10 497592 302 1585 3 44272 760 0 212 972 44 0 52312 11 134 127\n" +
				"   8       1 sda1 100 0 2000 1 50 0 4000 2 0 3 3 0 0 0 0 0 0\n",
			want: map[string][2]uint64{"sda": {497592, 44272}, "sda1": {2000, 4000}},
		},
		{
			name: "short and malformed lines are skipped",
			data: "8 0 sdb 1 2 3\n8 16 sdc 1 0 x 0 0 0 5 0 0 0 0\n\n8 32 sdd 1 0 7 0 0 0 9 0 0 0 0\n",
			want: map[string][2]uint64{"sdd": {7, 9}},
		},
		{name: "empty", data: "", want: map[string][2]uint64{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseDiskstats(tt.data)
			if fmt.Sprint(got) != fmt.Sprint(tt.want) {
				t.Errorf("parseDiskstats() = %v, want %v", got, tt.want)
			}
		})
	}
}

// throughputTopology is one controller with two x4 12G ports: port 0 cabled to
// enclosure "a" and port 1 to enclosure "b" (multipath tests add a second port
// to "a"); enclosure "c" is daisy-chained behind "a" with no direct link.
func throughputTopology() *dto.StorageTopology {
	rate := 12.0
	ctrl := dto.StorageController{
		Index: 0,
		Ports: []dto.StorageControllerPort{
			{Port: 0, Width: 4, LinkRateGbps: &rate, AttachedEnclosureID: "a"},
			{Port: 1, Width: 4, LinkRateGbps: &rate, AttachedEnclosureID: "b"},
		},
	}
	for i := 0; i < 8; i++ {
		ctrl.Phys = append(ctrl.Phys, dto.StorageControllerPhy{Phy: i, Connected: true, LinkRateGbps: &rate})
	}
	ctrl.Phys = append(ctrl.Phys, dto.StorageControllerPhy{Phy: 8}) // unconnected
	return &dto.StorageTopology{
		Controllers: []dto.StorageController{ctrl},
		Enclosures:  []dto.StorageEnclosure{{ID: "a"}, {ID: "b"}, {ID: "c"}},
		Drives: []dto.StorageDrive{
			{ControllerIndex: 0, EnclosureID: "a", Device: "sda"},
			{ControllerIndex: 0, EnclosureID: "a", Device: "sdb"},
			{ControllerIndex: 0, EnclosureID: "b", Device: "sdc"},
			{ControllerIndex: 0, EnclosureID: "c", Device: "sdd"},
			{ControllerIndex: 0, Device: "sde"},                   // direct-attached
			{ControllerIndex: 0, EnclosureID: "b"},                // no block device
			{ControllerIndex: 0, EnclosureID: "b", Device: "sdz"}, // not in diskstats
		},
	}
}

func sample(at time.Time, sectors map[string][2]uint64) *diskSample {
	return &diskSample{at: at, sectors: sectors}
}

func TestApplyThroughput(t *testing.T) {
	t0 := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	const mib = 1 << 20
	sectorsPerMiBs300 := uint64(300 * mib / 512) // 1 MiB/s over 300 s
	base := map[string][2]uint64{"sda": {1000, 1000}, "sdb": {0, 0}, "sdc": {0, 0}, "sdd": {0, 0}, "sde": {0, 0}}
	next := map[string][2]uint64{
		"sda": {1000 + 100*sectorsPerMiBs300, 1000 + 20*sectorsPerMiBs300}, // 100 MiB/s read, 20 MiB/s write
		"sdb": {50 * sectorsPerMiBs300, 0},
		"sdc": {10 * sectorsPerMiBs300, 10 * sectorsPerMiBs300},
		"sdd": {5 * sectorsPerMiBs300, 0},
		"sde": {0, 1 * sectorsPerMiBs300},
		"sdz": {999, 999},
	}
	pct := func(v float64) *float64 { return &v }

	tests := []struct {
		name       string
		prev, cur  *diskSample
		mutate     func(*dto.StorageTopology)
		controller *dto.StorageThroughput
		enclosures map[string]*dto.StorageThroughput
	}{
		{
			name:       "first cycle has no previous sample",
			prev:       nil,
			cur:        sample(t0, base),
			enclosures: map[string]*dto.StorageThroughput{},
		},
		{
			name:       "window shorter than one second",
			prev:       sample(t0, base),
			cur:        sample(t0.Add(999*time.Millisecond), next),
			enclosures: map[string]*dto.StorageThroughput{},
		},
		{
			name: "two cycles: daisy-chained enclosure has no capacity",
			prev: sample(t0, base),
			cur:  sample(t0.Add(300*time.Second), next),
			controller: &dto.StorageThroughput{
				ReadBytesPerSec: 165 * mib, WriteBytesPerSec: 31 * mib, TotalBytesPerSec: 196 * mib,
				CapacityBytesPerSec: 9.6e9, UtilizationPercent: pct(2.1), Drives: 5, IntervalSeconds: 300,
			},
			enclosures: map[string]*dto.StorageThroughput{
				"a": {ReadBytesPerSec: 150 * mib, WriteBytesPerSec: 20 * mib, TotalBytesPerSec: 170 * mib,
					CapacityBytesPerSec: 4.8e9, UtilizationPercent: pct(3.7), Drives: 2, IntervalSeconds: 300},
				"b": {ReadBytesPerSec: 10 * mib, WriteBytesPerSec: 10 * mib, TotalBytesPerSec: 20 * mib,
					CapacityBytesPerSec: 4.8e9, UtilizationPercent: pct(0.4), Drives: 1, IntervalSeconds: 300},
				"c": {ReadBytesPerSec: 5 * mib, TotalBytesPerSec: 5 * mib, Drives: 1, IntervalSeconds: 300},
			},
		},
		{
			name: "multipath enclosure on two ports sums both links",
			prev: sample(t0, base),
			cur:  sample(t0.Add(300*time.Second), next),
			mutate: func(topo *dto.StorageTopology) {
				topo.Controllers[0].Ports[1].AttachedEnclosureID = "a"
			},
			controller: &dto.StorageThroughput{
				ReadBytesPerSec: 165 * mib, WriteBytesPerSec: 31 * mib, TotalBytesPerSec: 196 * mib,
				CapacityBytesPerSec: 9.6e9, UtilizationPercent: pct(2.1), Drives: 5, IntervalSeconds: 300,
			},
			enclosures: map[string]*dto.StorageThroughput{
				"a": {ReadBytesPerSec: 150 * mib, WriteBytesPerSec: 20 * mib, TotalBytesPerSec: 170 * mib,
					CapacityBytesPerSec: 9.6e9, UtilizationPercent: pct(1.9), Drives: 2, IntervalSeconds: 300},
				"b": {ReadBytesPerSec: 10 * mib, WriteBytesPerSec: 10 * mib, TotalBytesPerSec: 20 * mib,
					Drives: 1, IntervalSeconds: 300},
				"c": {ReadBytesPerSec: 5 * mib, TotalBytesPerSec: 5 * mib, Drives: 1, IntervalSeconds: 300},
			},
		},
		{
			name: "counter reset counts as zero; capacity falls back to ports without phy rates",
			prev: sample(t0, next),
			cur: sample(t0.Add(150*time.Second), map[string][2]uint64{
				"sda": {10, 10}, // reset below the previous value
				"sdb": next["sdb"], "sdc": next["sdc"], "sdd": next["sdd"],
				"sde": {0, next["sde"][1] + 150*mib/512}, // +1 MiB/s write
			}),
			mutate: func(topo *dto.StorageTopology) {
				for i := range topo.Controllers[0].Phys {
					topo.Controllers[0].Phys[i].LinkRateGbps = nil
				}
			},
			controller: &dto.StorageThroughput{
				WriteBytesPerSec: 1 * mib, TotalBytesPerSec: 1 * mib,
				CapacityBytesPerSec: 9.6e9, UtilizationPercent: pct(0), Drives: 5, IntervalSeconds: 150,
			},
			enclosures: map[string]*dto.StorageThroughput{
				"a": {CapacityBytesPerSec: 4.8e9, UtilizationPercent: pct(0), Drives: 2, IntervalSeconds: 150},
				"b": {CapacityBytesPerSec: 4.8e9, UtilizationPercent: pct(0), Drives: 1, IntervalSeconds: 150},
				"c": {Drives: 1, IntervalSeconds: 150},
			},
		},
		{
			name:       "no drive in diskstats: throughput omitted",
			prev:       sample(t0, map[string][2]uint64{}),
			cur:        sample(t0.Add(300*time.Second), map[string][2]uint64{}),
			enclosures: map[string]*dto.StorageThroughput{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			topo := throughputTopology()
			if tt.mutate != nil {
				tt.mutate(topo)
			}
			applyThroughput(topo, tt.prev, tt.cur)
			assertThroughput(t, "controller 0", topo.Controllers[0].Throughput, tt.controller)
			for _, e := range topo.Enclosures {
				assertThroughput(t, "enclosure "+e.ID, e.Throughput, tt.enclosures[e.ID])
			}
		})
	}
}

func assertThroughput(t *testing.T, what string, got, want *dto.StorageThroughput) {
	t.Helper()
	g, _ := json.Marshal(got)
	w, _ := json.Marshal(want)
	if string(g) != string(w) {
		t.Errorf("%s throughput = %s, want %s", what, g, w)
	}
}

// TestStorageTopologyThroughputAcrossCycles runs two collections over the
// fixture topology with fake diskstats and a fake clock.
func TestStorageTopologyThroughputAcrossCycles(t *testing.T) {
	c := newFixtureCollector(t, newFakeRunner(t), "/sbin/storcli", "/usr/bin/sg_ses")
	now := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	c.Now = func() time.Time { return now }

	// Every fixture drive's block device, plus an unrelated disk and a partition.
	first := c.Gather(context.Background()) // no diskstats file yet
	writeDiskstats := func(perDrive func(i int) (uint64, uint64)) {
		var b strings.Builder
		b.WriteString("   8       0 sda 10 0 999999 0 0 0 999999 0 0 0 0 0 0 0 0 0 0\n")
		for i, d := range first.Drives {
			read, written := perDrive(i)
			fmt.Fprintf(&b, "  65 %7d %s 1 0 %d 0 1 0 %d 0 0 0 0 0 0 0 0 0 0\n", i, d.Device, read, written)
			fmt.Fprintf(&b, "  65 %7d %s1 1 0 %d 0 1 0 %d 0 0 0 0 0 0 0 0 0 0\n", i, d.Device, read, written)
		}
		if err := os.WriteFile(c.DiskstatsPath, []byte(b.String()), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, e := range first.Enclosures {
		if e.Throughput != nil {
			t.Fatalf("throughput without diskstats: %+v", e.Throughput)
		}
	}

	writeDiskstats(func(int) (uint64, uint64) { return 1000, 1000 })
	topo := c.Gather(context.Background())
	if topo.Controllers[0].Throughput != nil || topo.Enclosures[0].Throughput != nil {
		t.Fatal("first diskstats sample must not report throughput")
	}

	// 300 s later every drive has read 120000 and written 60000 sectors: 204800 + 102400 B/s each.
	now = now.Add(300 * time.Second)
	writeDiskstats(func(int) (uint64, uint64) { return 1000 + 120000, 1000 + 60000 })
	topo = c.Gather(context.Background())
	if len(topo.Errors) != 0 {
		t.Fatalf("errors: %v", topo.Errors)
	}

	ctrl := topo.Controllers[0].Throughput
	e242, e245 := findEnclosure(t, topo, 242), findEnclosure(t, topo, 245)
	if ctrl == nil || e242.Throughput == nil || e245.Throughput == nil {
		t.Fatalf("missing throughput: ctrl=%v e242=%v e245=%v", ctrl, e242.Throughput, e245.Throughput)
	}
	if ctrl.Drives != 43 || ctrl.ReadBytesPerSec != 43*204800 || ctrl.WriteBytesPerSec != 43*102400 ||
		ctrl.TotalBytesPerSec != 43*307200 || ctrl.CapacityBytesPerSec != 9.6e9 || ctrl.IntervalSeconds != 300 ||
		ctrl.UtilizationPercent == nil || *ctrl.UtilizationPercent != 0.1 {
		t.Errorf("unexpected controller throughput: %+v", *ctrl)
	}
	if e242.Throughput.Drives+e245.Throughput.Drives != 43 || e242.Throughput.CapacityBytesPerSec != 4.8e9 ||
		e245.Throughput.CapacityBytesPerSec != 4.8e9 ||
		e242.Throughput.TotalBytesPerSec != float64(e242.Throughput.Drives)*307200 {
		t.Errorf("unexpected enclosure throughput: e242=%+v e245=%+v", *e242.Throughput, *e245.Throughput)
	}
	// The SES-only HighPoint enclosure has no storcli drives.
	if topo.Enclosures[2].Throughput != nil {
		t.Errorf("SES-only enclosure got throughput: %+v", topo.Enclosures[2].Throughput)
	}
	ctrlJSON, _ := json.Marshal(ctrl)
	enclJSON, _ := json.Marshal(e242.Throughput)
	t.Logf("controller throughput: %s", ctrlJSON)
	t.Logf("enclosure %s throughput: %s", e242.ID, enclJSON)
}
