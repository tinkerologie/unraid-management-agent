package collectors

import (
	"math"
	"testing"
	"time"

	"github.com/tinkerologie/unraid-management-agent/daemon/dto"
)

// zfsTime builds a local-time timestamp, as zpool prints them.
func zfsTime(year int, month time.Month, day, h, m, s int) time.Time {
	return time.Date(year, month, day, h, m, s, 0, time.Local)
}

// TestParseScanInfoFormats covers every scan line format zpool prints (#185).
func TestParseScanInfoFormats(t *testing.T) {
	end := zfsTime(2026, time.September, 17, 22, 6, 13)
	tests := []struct {
		name     string
		line     string
		status   string
		state    string
		repaired uint64
		errors   int
		start    time.Time
		end      time.Time
		pct      float64
	}{
		{
			name: "completed scrub with days", status: "scrub completed", state: "finished", pct: 100,
			line:  "scan: scrub repaired 0B in 2 days 00:06:12 with 0 errors on Thu Sep 17 22:06:13 2026",
			start: end.Add(-(48*time.Hour + 6*time.Minute + 12*time.Second)), end: end,
		},
		{
			name: "completed scrub with repairs and errors", status: "scrub completed", state: "finished", pct: 100,
			line:     "scan: scrub repaired 512K in 01:23:45 with 5 errors on Thu Sep 17 22:06:13 2026",
			repaired: 512 * 1024, errors: 5,
			start: end.Add(-(time.Hour + 23*time.Minute + 45*time.Second)), end: end,
		},
		{
			name: "completed resilver", status: "resilver completed", state: "finished", pct: 100,
			line:     "scan: resilvered 1.21T in 10:11:12 with 0 errors on Thu Sep 17 22:06:13 2026",
			repaired: uint64(math.Round(1.21 * math.Pow(1024, 4))),
			start:    end.Add(-(10*time.Hour + 11*time.Minute + 12*time.Second)), end: end,
		},
		{
			name: "canceled scrub", status: "scrub canceled", state: "canceled",
			line: "scan: scrub canceled on Thu Sep 17 22:06:13 2026", end: end,
		},
		{
			name: "canceled resilver", status: "resilver canceled", state: "canceled",
			line: "scan: resilver canceled on Thu Sep 17 22:06:13 2026", end: end,
		},
		{
			name: "scrub in progress, ctime single-digit day", status: "scrub in progress", state: "scanning",
			line:  "scan: scrub in progress since Sun Nov  9 02:39:43 2025",
			start: zfsTime(2025, time.November, 9, 2, 39, 43),
		},
		{
			name: "scrub paused", status: "scrub paused", state: "paused",
			line: "scan: scrub paused since Thu Sep 17 22:06:13 2026",
		},
		{
			name: "sequential rebuild completed", status: "resilver completed", state: "finished", pct: 100,
			line:     "scan: resilvered (draid1-0) 2G in 00:10:00 with 0 errors on Thu Sep 17 22:06:13 2026",
			repaired: 2 << 30, start: end.Add(-10 * time.Minute), end: end,
		},
		{
			name: "sequential rebuild canceled", status: "resilver canceled", state: "canceled",
			line: "scan: resilver (draid1-0) canceled on Thu Sep 17 22:06:13 2026", end: end,
		},
		{
			name: "sequential rebuild in progress", status: "resilver in progress", state: "scanning",
			line: "scan: resilver (mirror-1) in progress since Thu Sep 17 22:06:13 2026", start: end,
		},
		{name: "none requested", line: "scan: none requested"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool := &dto.ZFSPool{}
			parseScanInfo(pool, tt.line)
			if pool.ScanStatus != tt.status || pool.ScanState != tt.state {
				t.Errorf("status/state = %q/%q, want %q/%q", pool.ScanStatus, pool.ScanState, tt.status, tt.state)
			}
			if pool.ScanRepairedBytes != tt.repaired || pool.ScanErrors != tt.errors || pool.ScanProgressPct != tt.pct {
				t.Errorf("repaired/errors/pct = %d/%d/%v, want %d/%d/%v",
					pool.ScanRepairedBytes, pool.ScanErrors, pool.ScanProgressPct, tt.repaired, tt.errors, tt.pct)
			}
			if !pool.ScanStartTime.Equal(tt.start) || !pool.ScanEndTime.Equal(tt.end) {
				t.Errorf("start/end = %v/%v, want %v/%v", pool.ScanStartTime, pool.ScanEndTime, tt.start, tt.end)
			}
		})
	}
}

// TestParseZPoolStatusScanProgress checks the indented progress lines of an
// in-progress and a paused scrub.
func TestParseZPoolStatusScanProgress(t *testing.T) {
	inProgress := `  pool: tank
 state: ONLINE
  scan: scrub in progress since Thu Sep 17 22:06:13 2026
	1.23T / 4.56T scanned at 1.2G/s, 1.00T / 4.56T issued at 1.0G/s
	64K repaired, 21.93% done, 01:02:03 to go
config:

	NAME        STATE     READ WRITE CKSUM
	tank        ONLINE       0     0     0
	  sdb       ONLINE       0     0     0

errors: No known data errors
`
	pool := &dto.ZFSPool{}
	if err := parseZPoolStatusOutput(pool, inProgress); err != nil {
		t.Fatal(err)
	}
	if pool.ScanStatus != "scrub in progress" || pool.ScanProgressPct != 21.93 || pool.ScanRepairedBytes != 64*1024 {
		t.Errorf("in progress: status=%q pct=%v repaired=%d", pool.ScanStatus, pool.ScanProgressPct, pool.ScanRepairedBytes)
	}
	if !pool.ScanStartTime.Equal(zfsTime(2026, time.September, 17, 22, 6, 13)) {
		t.Errorf("in progress start = %v", pool.ScanStartTime)
	}
	if len(pool.VDEVs) == 0 {
		t.Error("vdevs after the scan block were not parsed")
	}

	paused := `  pool: tank
 state: ONLINE
  scan: scrub paused since Thu Sep 17 22:06:13 2026
	scrub started on Wed Sep 16 08:00:00 2026
	2.00T / 4.56T scanned, 1.50T / 4.56T issued
	0B repaired, 32.89% done
config:
`
	pool = &dto.ZFSPool{}
	if err := parseZPoolStatusOutput(pool, paused); err != nil {
		t.Fatal(err)
	}
	if pool.ScanStatus != "scrub paused" || pool.ScanState != "paused" || pool.ScanProgressPct != 32.89 {
		t.Errorf("paused: status=%q state=%q pct=%v", pool.ScanStatus, pool.ScanState, pool.ScanProgressPct)
	}
	if !pool.ScanStartTime.Equal(zfsTime(2026, time.September, 16, 8, 0, 0)) {
		t.Errorf("paused start = %v", pool.ScanStartTime)
	}
}

// TestParseZFSSize covers zfs_nicenum sizes and invalid input.
func TestParseZFSSize(t *testing.T) {
	tests := map[string]uint64{"0B": 0, "512B": 512, "4K": 4096, "1.5M": 1572864, "2G": 2 << 30, "1T": 1 << 40}
	for in, want := range tests {
		if got, ok := parseZFSSize(in); !ok || got != want {
			t.Errorf("parseZFSSize(%q) = %d, %v; want %d", in, got, ok, want)
		}
	}
	for _, bad := range []string{"", "abc", "-1K"} {
		if _, ok := parseZFSSize(bad); ok {
			t.Errorf("parseZFSSize(%q) accepted invalid input", bad)
		}
	}
}

// TestParseScanDuration covers HH:MM:SS and "N days HH:MM:SS" durations.
func TestParseScanDuration(t *testing.T) {
	tests := map[string]time.Duration{
		"00:00:01":        time.Second,
		"01:23:45":        time.Hour + 23*time.Minute + 45*time.Second,
		"1 days 00:00:00": 24 * time.Hour,
		"2 days 00:06:12": 48*time.Hour + 6*time.Minute + 12*time.Second,
	}
	for in, want := range tests {
		if got, ok := parseScanDuration(in); !ok || got != want {
			t.Errorf("parseScanDuration(%q) = %v, %v; want %v", in, got, ok, want)
		}
	}
	if _, ok := parseScanDuration("soon"); ok {
		t.Error("parseScanDuration accepted invalid input")
	}
}

// TestParseZPoolStatusMultipleScanLines covers zpool printing a scrub line and
// a resilver line, or one rebuild line per top-level vdev (#185).
func TestParseZPoolStatusMultipleScanLines(t *testing.T) {
	tests := []struct {
		name   string
		output string
		status string
		pct    float64
		end    time.Time
	}{
		{
			name: "active rebuild before a completed rebuild stays current",
			output: `  pool: tank
 state: DEGRADED
  scan: resilver (draid1-0) in progress since Thu Sep 17 22:06:13 2026
	1.00T / 4.00T scanned, 1.00T / 4.00T issued
	1.00T resilvered, 25.00% done, 03:00:00 to go
  scan: resilvered (draid1-1) 2G in 00:10:00 with 0 errors on Fri Sep 18 08:00:00 2026
config:
`,
			status: "resilver in progress", pct: 25,
		},
		{
			name: "completed scrub then active resilver: resilver is current",
			output: `  pool: tank
 state: DEGRADED
  scan: scrub repaired 0B in 01:00:00 with 0 errors on Thu Sep 17 22:06:13 2026
  scan: resilver in progress since Fri Sep 18 08:00:00 2026
	100G / 400G scanned, 100G / 400G issued
	100G resilvered, 25.00% done, 01:00:00 to go
config:
`,
			status: "resilver in progress", pct: 25,
		},
		{
			name: "two finished scans: the later end time wins",
			output: `  pool: tank
 state: ONLINE
  scan: scrub repaired 0B in 01:00:00 with 0 errors on Fri Sep 18 08:00:00 2026
  scan: resilvered 2G in 00:10:00 with 0 errors on Thu Sep 17 22:06:13 2026
config:
`,
			status: "scrub completed", pct: 100, end: zfsTime(2026, time.September, 18, 8, 0, 0),
		},
		{
			name: "older finished scan is replaced by a newer canceled one",
			output: `  pool: tank
 state: ONLINE
  scan: scrub repaired 0B in 01:00:00 with 0 errors on Thu Sep 17 22:06:13 2026
  scan: resilver canceled on Fri Sep 18 08:00:00 2026
config:
`,
			status: "resilver canceled", end: zfsTime(2026, time.September, 18, 8, 0, 0),
		},
		{
			name: "two active scans: the first stays current",
			output: `  pool: tank
 state: DEGRADED
  scan: resilver (draid1-0) in progress since Thu Sep 17 22:06:13 2026
	1.00T resilvered, 10.00% done, 05:00:00 to go
  scan: resilver (draid1-1) in progress since Fri Sep 18 08:00:00 2026
	2.00T resilvered, 90.00% done, 00:10:00 to go
config:
`,
			status: "resilver in progress", pct: 10,
		},
		{
			name: "unrecognised scan line is ignored",
			output: `  pool: tank
 state: ONLINE
  scan: something new from a future OpenZFS
  scan: scrub repaired 0B in 01:00:00 with 0 errors on Thu Sep 17 22:06:13 2026
config:
`,
			status: "scrub completed", pct: 100, end: zfsTime(2026, time.September, 17, 22, 6, 13),
		},
		{
			name: "scan block at end of output is kept",
			output: `  pool: tank
 state: ONLINE
  scan: scrub in progress since Thu Sep 17 22:06:13 2026
	1.00T / 4.00T scanned, 1.00T / 4.00T issued
	0B repaired, 50.00% done, 01:00:00 to go`,
			status: "scrub in progress", pct: 50,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool := &dto.ZFSPool{}
			if err := parseZPoolStatusOutput(pool, tt.output); err != nil {
				t.Fatal(err)
			}
			if pool.ScanStatus != tt.status || pool.ScanProgressPct != tt.pct {
				t.Errorf("status/pct = %q/%v, want %q/%v", pool.ScanStatus, pool.ScanProgressPct, tt.status, tt.pct)
			}
			if !tt.end.IsZero() && !pool.ScanEndTime.Equal(tt.end) {
				t.Errorf("end = %v, want %v", pool.ScanEndTime, tt.end)
			}
		})
	}
}

// TestParseScanMalformedValues checks malformed sizes, durations, timestamps
// and percentages leave the related fields unset while parsing continues.
func TestParseScanMalformedValues(t *testing.T) {
	t.Run("scan lines", func(t *testing.T) {
		tests := []struct {
			name, line, status string
			wantErrors         int
		}{
			{"completed with bad size", "scan: scrub repaired 1.2X in 01:00:00 with 3 errors on Thu Sep 17 22:06:13 2026", "scrub completed", 3},
			{"completed with bad end time", "scan: scrub repaired 0B in 01:00:00 with 0 errors on someday", "scrub completed", 0},
			{"completed with bad duration", "scan: scrub repaired 0B in a while with 0 errors on Thu Sep 17 22:06:13 2026", "scrub completed", 0},
			{"canceled with bad time", "scan: scrub canceled on someday", "scrub canceled", 0},
			{"in progress with bad time", "scan: scrub in progress since someday", "scrub in progress", 0},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				pool := &dto.ZFSPool{}
				parseScanInfo(pool, tt.line)
				if pool.ScanStatus != tt.status || pool.ScanErrors != tt.wantErrors {
					t.Errorf("status/errors = %q/%d, want %q/%d", pool.ScanStatus, pool.ScanErrors, tt.status, tt.wantErrors)
				}
				if pool.ScanRepairedBytes != 0 {
					t.Errorf("repaired = %d, want 0 for a malformed size", pool.ScanRepairedBytes)
				}
				switch tt.name {
				case "completed with bad duration":
					if pool.ScanEndTime.IsZero() || !pool.ScanStartTime.IsZero() {
						t.Errorf("start/end = %v/%v, want end set and start unset", pool.ScanStartTime, pool.ScanEndTime)
					}
				case "completed with bad size":
					if pool.ScanEndTime.IsZero() {
						t.Error("end time must still be parsed when the size is malformed")
					}
				default:
					if !pool.ScanStartTime.IsZero() || !pool.ScanEndTime.IsZero() {
						t.Errorf("start/end = %v/%v, want both unset", pool.ScanStartTime, pool.ScanEndTime)
					}
				}
			})
		}
	})

	t.Run("detail lines", func(t *testing.T) {
		pool := &dto.ZFSPool{}
		parseScanDetail(pool, "1.2X repaired, 45.00% done, 01:00:00 to go")
		if pool.ScanRepairedBytes != 0 || pool.ScanProgressPct != 45 {
			t.Errorf("bad size: repaired/pct = %d/%v, want 0/45", pool.ScanRepairedBytes, pool.ScanProgressPct)
		}
		pool = &dto.ZFSPool{}
		parseScanDetail(pool, "64K repaired, 1.2.3% done")
		if pool.ScanRepairedBytes != 64*1024 || pool.ScanProgressPct != 0 {
			t.Errorf("bad percentage: repaired/pct = %d/%v, want 65536/0", pool.ScanRepairedBytes, pool.ScanProgressPct)
		}
		pool = &dto.ZFSPool{}
		parseScanDetail(pool, "scrub started on someday")
		if !pool.ScanStartTime.IsZero() {
			t.Errorf("bad started-on time: start = %v, want unset", pool.ScanStartTime)
		}
	})

	t.Run("blank line ends the scan block", func(t *testing.T) {
		output := `  pool: tank
 state: ONLINE
  scan: scrub in progress since Thu Sep 17 22:06:13 2026
	0B repaired, 10.00% done, 01:00:00 to go

	99G repaired, 99.00% done, 00:00:01 to go
config:
`
		pool := &dto.ZFSPool{}
		if err := parseZPoolStatusOutput(pool, output); err != nil {
			t.Fatal(err)
		}
		if pool.ScanProgressPct != 10 || pool.ScanRepairedBytes != 0 {
			t.Errorf("pct/repaired = %v/%d, want 10/0: lines after a blank line are not part of the scan", pool.ScanProgressPct, pool.ScanRepairedBytes)
		}
	})
}
