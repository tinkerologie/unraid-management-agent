package collectors

import (
	"testing"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

func TestNewZFSCollector(t *testing.T) {
	hub := domain.NewEventBus(10)
	ctx := &domain.Context{Hub: hub}

	collector := NewZFSCollector(ctx)

	if collector == nil {
		t.Fatal("NewZFSCollector() returned nil")
	}

	if collector.ctx != ctx {
		t.Error("ZFSCollector context not set correctly")
	}
}

func TestParseZPoolListOutput(t *testing.T) {
	tests := []struct {
		name      string
		output    string
		wantCount int
		wantErr   bool
		check     func(t *testing.T, pools []dto.ZFSPool)
	}{
		{
			name:      "empty output",
			output:    "",
			wantCount: 0,
			wantErr:   false,
		},
		{
			name: "valid multi-pool output",
			output: "pool1\t3977237090304\t1330360827904\t2646876262400\t5.5\t33.2\t1.00x\tONLINE\t-\n" +
				"pool2\t7993072746496\t3848290697216\t4144782049280\t-\t-\t1.25\tDEGRADED\t/mnt/alt",
			wantCount: 2,
			wantErr:   false,
			check: func(t *testing.T, pools []dto.ZFSPool) {
				if pools[0].Name != "pool1" {
					t.Errorf("pool[0].Name = %q, want pool1", pools[0].Name)
				}
				if pools[0].SizeBytes != 3977237090304 {
					t.Errorf("pool[0].SizeBytes = %d, want 3977237090304", pools[0].SizeBytes)
				}
				if pools[0].AllocatedBytes != 1330360827904 {
					t.Errorf("pool[0].AllocatedBytes = %d, want 1330360827904", pools[0].AllocatedBytes)
				}
				if pools[0].FreeBytes != 2646876262400 {
					t.Errorf("pool[0].FreeBytes = %d, want 2646876262400", pools[0].FreeBytes)
				}
				if pools[0].FragmentationPct != 5.5 {
					t.Errorf("pool[0].FragmentationPct = %f, want 5.5", pools[0].FragmentationPct)
				}
				if pools[0].CapacityPct != 33.2 {
					t.Errorf("pool[0].CapacityPct = %f, want 33.2", pools[0].CapacityPct)
				}
				if pools[0].DedupRatio != 1.0 {
					t.Errorf("pool[0].DedupRatio = %f, want 1.0", pools[0].DedupRatio)
				}
				if pools[0].Health != "ONLINE" {
					t.Errorf("pool[0].Health = %q, want ONLINE", pools[0].Health)
				}
				if pools[0].Altroot != "" {
					t.Errorf("pool[0].Altroot = %q, want empty", pools[0].Altroot)
				}

				if pools[1].Name != "pool2" {
					t.Errorf("pool[1].Name = %q, want pool2", pools[1].Name)
				}
				if pools[1].Health != "DEGRADED" {
					t.Errorf("pool[1].Health = %q, want DEGRADED", pools[1].Health)
				}
				if pools[1].Altroot != "/mnt/alt" {
					t.Errorf("pool[1].Altroot = %q, want /mnt/alt", pools[1].Altroot)
				}
				if pools[1].DedupRatio != 1.25 {
					t.Errorf("pool[1].DedupRatio = %f, want 1.25", pools[1].DedupRatio)
				}
			},
		},
		{
			name:    "invalid too few fields",
			output:  "pool1\t1000\t2000",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pools, err := parseZPoolListOutput(tt.output)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseZPoolListOutput() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr {
				if len(pools) != tt.wantCount {
					t.Errorf("got %d pools, want %d", len(pools), tt.wantCount)
				}
				if tt.check != nil {
					tt.check(t, pools)
				}
			}
		})
	}
}

func TestParseZPoolStatusOutput(t *testing.T) {
	tests := []struct {
		name    string
		output  string
		check   func(t *testing.T, pool *dto.ZFSPool)
		wantErr bool
	}{
		{
			name: "healthy mirror pool with scrub complete",
			output: `  pool: tank
 state: ONLINE
  scan: scrub repaired 0B in 00:00:01 with 0 errors on Sun Nov 10 02:39:43 2025
config:

	NAME        STATE     READ WRITE CKSUM
	tank        ONLINE       0     0     0
	  mirror-0  ONLINE       0     0     0
	    sda     ONLINE       0     0     0
	    sdb     ONLINE       0     0     0

errors: No known data errors
`,
			check: func(t *testing.T, pool *dto.ZFSPool) {
				if pool.State != "ONLINE" {
					t.Errorf("State = %q, want ONLINE", pool.State)
				}
				if pool.ScanStatus != "scrub completed" {
					t.Errorf("ScanStatus = %q, want scrub completed", pool.ScanStatus)
				}
				if pool.ScanState != "finished" {
					t.Errorf("ScanState = %q, want finished", pool.ScanState)
				}
				if pool.ScanErrors != 0 {
					t.Errorf("ScanErrors = %d, want 0", pool.ScanErrors)
				}
				if len(pool.VDEVs) != 1 {
					t.Fatalf("len(VDEVs) = %d, want 1", len(pool.VDEVs))
				}
				vdev := pool.VDEVs[0]
				if vdev.Name != "mirror-0" || vdev.Type != "mirror" {
					t.Errorf("VDEV = %s (%s), want mirror-0 (mirror)", vdev.Name, vdev.Type)
				}
				if len(vdev.Devices) != 2 {
					t.Fatalf("len(vdev.Devices) = %d, want 2", len(vdev.Devices))
				}
				if vdev.Devices[0].Name != "sda" || vdev.Devices[1].Name != "sdb" {
					t.Errorf("devices = %v, %v", vdev.Devices[0].Name, vdev.Devices[1].Name)
				}
			},
		},
		{
			name: "degraded raidz pool with errors and corrupted files",
			output: `  pool: tank
 state: DEGRADED
status: One or more devices has experienced an unrecoverable error.
  scan: scrub in progress since Sun Nov 10 02:39:43 2025
config:

	NAME        STATE     READ WRITE CKSUM
	tank        DEGRADED     1     2     3
	  raidz1-0  DEGRADED     1     2     3
	    sda     ONLINE       0     0     0
	    sdb     FAULTED      5    10    15
	    sdc     ONLINE       0     0     0

errors: Permanent errors have been detected in the following files:

        /mnt/tank/file1.bin
        /mnt/tank/file2.bin
`,
			check: func(t *testing.T, pool *dto.ZFSPool) {
				if pool.State != "DEGRADED" {
					t.Errorf("State = %q, want DEGRADED", pool.State)
				}
				if pool.ScanStatus != "in progress" {
					t.Errorf("ScanStatus = %q, want in progress", pool.ScanStatus)
				}
				if pool.ScanState != "scanning" {
					t.Errorf("ScanState = %q, want scanning", pool.ScanState)
				}
				if pool.ReadErrors != 1 || pool.WriteErrors != 2 || pool.ChecksumErrors != 3 {
					t.Errorf("Pool errors = %d/%d/%d, want 1/2/3", pool.ReadErrors, pool.WriteErrors, pool.ChecksumErrors)
				}
				if len(pool.VDEVs) != 1 {
					t.Fatalf("len(VDEVs) = %d, want 1", len(pool.VDEVs))
				}
				if len(pool.VDEVs[0].Devices) != 3 {
					t.Fatalf("len(Devices) = %d, want 3", len(pool.VDEVs[0].Devices))
				}
				sdb := pool.VDEVs[0].Devices[1]
				if sdb.State != "FAULTED" || sdb.ReadErrors != 5 || sdb.WriteErrors != 10 || sdb.ChecksumErrors != 15 {
					t.Errorf("sdb faulty stats = %+v", sdb)
				}
				if len(pool.CorruptedFiles) != 2 {
					t.Fatalf("len(CorruptedFiles) = %d, want 2", len(pool.CorruptedFiles))
				}
				if pool.CorruptedFiles[0] != "/mnt/tank/file1.bin" || pool.CorruptedFiles[1] != "/mnt/tank/file2.bin" {
					t.Errorf("CorruptedFiles = %v", pool.CorruptedFiles)
				}
			},
		},
		{
			name: "resilver in progress",
			output: `  pool: vault
 state: ONLINE
  scan: resilver in progress since Sun Nov 10 03:00:00 2025
config:

	NAME        STATE     READ WRITE CKSUM
	vault       ONLINE       0     0     0
	  sda       ONLINE       0     0     0

errors: No known data errors
`,
			check: func(t *testing.T, pool *dto.ZFSPool) {
				if pool.ScanStatus != "in progress" {
					t.Errorf("ScanStatus = %q, want in progress", pool.ScanStatus)
				}
				if pool.ScanState != "scanning" {
					t.Errorf("ScanState = %q, want scanning", pool.ScanState)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool := &dto.ZFSPool{Name: "test"}
			err := parseZPoolStatusOutput(pool, tt.output)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseZPoolStatusOutput() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && tt.check != nil {
				tt.check(t, pool)
			}
		})
	}
}

func TestParseZFSDatasetListOutput(t *testing.T) {
	tests := []struct {
		name      string
		output    string
		wantCount int
		check     func(t *testing.T, datasets []dto.ZFSDataset)
	}{
		{
			name:      "empty output",
			output:    "",
			wantCount: 0,
		},
		{
			name: "valid dataset output",
			output: "tank\tfilesystem\t1000000\t2000000\t500000\t1.50x\t/mnt/tank\t0\t0\tlz4\toff\n" +
				"tank/data\tfilesystem\t500000\t2000000\t500000\t1.00\t/mnt/tank/data\t10000000\t5000\tzstd\ton",
			wantCount: 2,
			check: func(t *testing.T, datasets []dto.ZFSDataset) {
				ds0 := datasets[0]
				if ds0.Name != "tank" || ds0.Type != "filesystem" {
					t.Errorf("ds0 = %s (%s)", ds0.Name, ds0.Type)
				}
				if ds0.UsedBytes != 1000000 || ds0.AvailableBytes != 2000000 || ds0.ReferencedBytes != 500000 {
					t.Errorf("ds0 bytes error = %+v", ds0)
				}
				if ds0.CompressRatio != 1.5 || ds0.Mountpoint != "/mnt/tank" || ds0.Compression != "lz4" || ds0.Readonly {
					t.Errorf("ds0 properties error = %+v", ds0)
				}

				ds1 := datasets[1]
				if ds1.Name != "tank/data" || ds1.QuotaBytes != 10000000 || ds1.ReservationBytes != 5000 || !ds1.Readonly {
					t.Errorf("ds1 properties error = %+v", ds1)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			datasets, err := parseZFSDatasetListOutput(tt.output)
			if err != nil {
				t.Fatalf("parseZFSDatasetListOutput() unexpected error: %v", err)
			}
			if len(datasets) != tt.wantCount {
				t.Errorf("got %d datasets, want %d", len(datasets), tt.wantCount)
			}
			if tt.check != nil {
				tt.check(t, datasets)
			}
		})
	}
}

func TestParseZFSSnapshotListOutput(t *testing.T) {
	output := "tank@snap1\t1000\t500\t1700000000\n" +
		"tank/data@snap2\t2000\t1000\t1700000500\n"

	snapshots, err := parseZFSSnapshotListOutput(output)
	if err != nil {
		t.Fatalf("parseZFSSnapshotListOutput() error = %v", err)
	}
	if len(snapshots) != 2 {
		t.Fatalf("got %d snapshots, want 2", len(snapshots))
	}
	if snapshots[0].Name != "tank@snap1" || snapshots[0].Dataset != "tank" || snapshots[0].UsedBytes != 1000 {
		t.Errorf("snapshot[0] error = %+v", snapshots[0])
	}
	if snapshots[1].Name != "tank/data@snap2" || snapshots[1].Dataset != "tank/data" || snapshots[1].ReferencedBytes != 1000 {
		t.Errorf("snapshot[1] error = %+v", snapshots[1])
	}

	empty, err := parseZFSSnapshotListOutput("")
	if err != nil || len(empty) != 0 {
		t.Errorf("empty snapshots error = %v, len = %d", err, len(empty))
	}
}

func TestZFSHealthStatus(t *testing.T) {
	// Test ZFS health status values
	tests := []struct {
		status  string
		healthy bool
	}{
		{"ONLINE", true},
		{"DEGRADED", false},
		{"FAULTED", false},
		{"OFFLINE", false},
		{"UNAVAIL", false},
		{"REMOVED", false},
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			isHealthy := tt.status == "ONLINE"
			if isHealthy != tt.healthy {
				t.Errorf("Health status %q: isHealthy = %v, want %v", tt.status, isHealthy, tt.healthy)
			}
		})
	}
}
func TestZFSScanStatusParsing(t *testing.T) {
	tests := []struct {
		name           string
		line           string
		expectedStatus string
		expectedState  string
	}{
		{
			name:           "scrub in progress",
			line:           "scan: scrub in progress since Sun Nov 10 02:39:43 2025",
			expectedStatus: "in progress",
			expectedState:  "scanning",
		},
		{
			name:           "scrub completed",
			line:           "scan: scrub repaired 0B in 00:00:01 with 0 errors on Sun Nov 10 02:39:43 2025",
			expectedStatus: "scrub completed",
			expectedState:  "finished",
		},
		{
			name:           "resilver in progress",
			line:           "scan: resilver in progress since Sun Nov 10 02:39:43 2025",
			expectedStatus: "in progress", // "in progress" matches first
			expectedState:  "scanning",
		},
		{
			name:           "resilver alone",
			line:           "scan: resilver started since Sun Nov 10 02:39:43 2025",
			expectedStatus: "resilver in progress",
			expectedState:  "scanning",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool := &dto.ZFSPool{}
			parseScanInfo(pool, tt.line)

			if pool.ScanStatus != tt.expectedStatus {
				t.Errorf("ScanStatus = %q, want %q", pool.ScanStatus, tt.expectedStatus)
			}
			if pool.ScanState != tt.expectedState {
				t.Errorf("ScanState = %q, want %q", pool.ScanState, tt.expectedState)
			}
		})
	}
}

func TestZFSVdevLineParsing(t *testing.T) {
	tests := []struct {
		name         string
		line         string
		wantNil      bool
		wantName     string
		wantState    string
		wantReadErr  uint64
		wantWriteErr uint64
		wantCksumErr uint64
	}{
		{
			name:         "valid vdev line",
			line:         "  sdg1      ONLINE       0     0     0",
			wantNil:      false,
			wantName:     "sdg1",
			wantState:    "ONLINE",
			wantReadErr:  0,
			wantWriteErr: 0,
			wantCksumErr: 0,
		},
		{
			name:         "vdev with errors",
			line:         "  sda1      ONLINE       5     10    15",
			wantNil:      false,
			wantName:     "sda1",
			wantState:    "ONLINE",
			wantReadErr:  5,
			wantWriteErr: 10,
			wantCksumErr: 15,
		},
		{
			name:         "degraded vdev",
			line:         "  sdb2      DEGRADED     0     1     0",
			wantNil:      false,
			wantName:     "sdb2",
			wantState:    "DEGRADED",
			wantReadErr:  0,
			wantWriteErr: 1,
			wantCksumErr: 0,
		},
		{
			name:         "faulted vdev",
			line:         "  sdc       FAULTED      100   200   50",
			wantNil:      false,
			wantName:     "sdc",
			wantState:    "FAULTED",
			wantReadErr:  100,
			wantWriteErr: 200,
			wantCksumErr: 50,
		},
		{
			name:    "too few fields",
			line:    "  sdg1    ONLINE",
			wantNil: true,
		},
		{
			name:    "empty line",
			line:    "",
			wantNil: true,
		},
		{
			name:    "whitespace only",
			line:    "     ",
			wantNil: true,
		},
		{
			name:      "nvme device",
			line:      "  nvme0n1p1 ONLINE       0     0     0",
			wantNil:   false,
			wantName:  "nvme0n1p1",
			wantState: "ONLINE",
		},
		{
			name:      "full path device",
			line:      "  /dev/disk/by-id/wwn-0x5000 ONLINE 0 0 0",
			wantNil:   false,
			wantName:  "/dev/disk/by-id/wwn-0x5000",
			wantState: "ONLINE",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseVdevLine(tt.line)
			if tt.wantNil {
				if result != nil {
					t.Errorf("parseVdevLine(%q) = %v, want nil", tt.line, result)
				}
			} else {
				if result == nil {
					t.Errorf("parseVdevLine(%q) = nil, want non-nil", tt.line)
					return
				}
				if result.Name != tt.wantName {
					t.Errorf("parseVdevLine(%q).Name = %q, want %q", tt.line, result.Name, tt.wantName)
				}
				if result.State != tt.wantState {
					t.Errorf("parseVdevLine(%q).State = %q, want %q", tt.line, result.State, tt.wantState)
				}
				if result.ReadErrors != tt.wantReadErr {
					t.Errorf("parseVdevLine(%q).ReadErrors = %d, want %d", tt.line, result.ReadErrors, tt.wantReadErr)
				}
				if result.WriteErrors != tt.wantWriteErr {
					t.Errorf("parseVdevLine(%q).WriteErrors = %d, want %d", tt.line, result.WriteErrors, tt.wantWriteErr)
				}
				if result.ChecksumErrors != tt.wantCksumErr {
					t.Errorf("parseVdevLine(%q).ChecksumErrors = %d, want %d", tt.line, result.ChecksumErrors, tt.wantCksumErr)
				}
			}
		})
	}
}

func TestZFSPoolStates(t *testing.T) {
	validStates := []string{
		"ONLINE",
		"DEGRADED",
		"FAULTED",
		"OFFLINE",
		"REMOVED",
		"UNAVAIL",
	}

	for _, state := range validStates {
		t.Run(state, func(t *testing.T) {
			if state == "" {
				t.Error("pool state should not be empty")
			}
		})
	}
}

// TestZFSVdevTypes tests parsing of different vdev types
func TestZFSVdevTypes(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		wantName string
		wantType string
	}{
		{
			name:     "disk type",
			line:     "  sda1      ONLINE       0     0     0",
			wantName: "sda1",
			wantType: "disk",
		},
		{
			name:     "raidz1 type",
			line:     "  raidz1-0  ONLINE       0     0     0",
			wantName: "raidz1-0",
			wantType: "raidz1",
		},
		{
			name:     "raidz2 type",
			line:     "  raidz2-0  ONLINE       0     0     0",
			wantName: "raidz2-0",
			wantType: "raidz2",
		},
		{
			name:     "raidz3 type",
			line:     "  raidz3-0  ONLINE       0     0     0",
			wantName: "raidz3-0",
			wantType: "raidz3",
		},
		{
			name:     "mirror type",
			line:     "  mirror-0  ONLINE       0     0     0",
			wantName: "mirror-0",
			wantType: "mirror",
		},
		{
			name:     "spare type",
			line:     "  spare-0   ONLINE       0     0     0",
			wantName: "spare-0",
			wantType: "spare",
		},
		{
			name:     "cache type",
			line:     "  cache     ONLINE       0     0     0",
			wantName: "cache",
			wantType: "cache",
		},
		{
			name:     "log type",
			line:     "  log       ONLINE       0     0     0",
			wantName: "log",
			wantType: "log",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseVdevLine(tt.line)
			if result == nil {
				t.Fatalf("parseVdevLine(%q) = nil, want non-nil", tt.line)
			}
			if result.Name != tt.wantName {
				t.Errorf("parseVdevLine(%q).Name = %q, want %q", tt.line, result.Name, tt.wantName)
			}
			if result.Type != tt.wantType {
				t.Errorf("parseVdevLine(%q).Type = %q, want %q", tt.line, result.Type, tt.wantType)
			}
		})
	}
}

func TestZFSParseOutput_EmptyLinesAndEdgeCases(t *testing.T) {
	pools, err := parseZPoolListOutput("   \n\n   ")
	if err != nil || len(pools) != 0 {
		t.Errorf("expected 0 pools, got %v, err %v", pools, err)
	}

	datasets, err := parseZFSDatasetListOutput("   \n\n   ")
	if err != nil || len(datasets) != 0 {
		t.Errorf("expected 0 datasets, got %v, err %v", datasets, err)
	}

	snaps, err := parseZFSSnapshotListOutput("   \n\n   ")
	if err != nil || len(snaps) != 0 {
		t.Errorf("expected 0 snaps, got %v, err %v", snaps, err)
	}
}

