package collectors

import (
	"strings"
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

func TestZpoolListOutputParsing(t *testing.T) {
	// Test parsing of zpool list output
	output := `NAME    SIZE  ALLOC   FREE  CKPOINT  EXPANDSZ   FRAG    CAP  DEDUP    HEALTH  ALTROOT
pool1  3.62T  1.21T  2.41T        -         -     5%    33%  1.00x    ONLINE  -
pool2  7.27T  3.50T  3.77T        -         -    10%    48%  1.00x    ONLINE  -
`
	lines := strings.Split(output, "\n")

	var pools []struct {
		Name   string
		Size   string
		Alloc  string
		Free   string
		Health string
	}

	for i, line := range lines {
		// Skip header line
		if i == 0 || strings.TrimSpace(line) == "" {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) >= 10 {
			pool := struct {
				Name   string
				Size   string
				Alloc  string
				Free   string
				Health string
			}{
				Name:   fields[0],
				Size:   fields[1],
				Alloc:  fields[2],
				Free:   fields[3],
				Health: fields[9],
			}
			pools = append(pools, pool)
		}
	}

	if len(pools) != 2 {
		t.Errorf("Expected 2 pools, got %d", len(pools))
	}

	if len(pools) > 0 && pools[0].Name != "pool1" {
		t.Errorf("First pool name = %q, want %q", pools[0].Name, "pool1")
	}

	if len(pools) > 0 && pools[0].Health != "ONLINE" {
		t.Errorf("First pool health = %q, want %q", pools[0].Health, "ONLINE")
	}
}

func TestZFSDatasetOutputParsing(t *testing.T) {
	// Test parsing of zfs list output
	output := `NAME                   USED  AVAIL     REFER  MOUNTPOINT
pool1                 1.21T  2.30T       96K  /mnt/pool1
pool1/data            500G  2.30T      500G  /mnt/pool1/data
pool1/backup          720G  2.30T      720G  /mnt/pool1/backup
`
	lines := strings.Split(output, "\n")

	var datasets []struct {
		Name       string
		Used       string
		Avail      string
		Refer      string
		Mountpoint string
	}

	for i, line := range lines {
		// Skip header line
		if i == 0 || strings.TrimSpace(line) == "" {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) >= 5 {
			dataset := struct {
				Name       string
				Used       string
				Avail      string
				Refer      string
				Mountpoint string
			}{
				Name:       fields[0],
				Used:       fields[1],
				Avail:      fields[2],
				Refer:      fields[3],
				Mountpoint: fields[4],
			}
			datasets = append(datasets, dataset)
		}
	}

	if len(datasets) != 3 {
		t.Errorf("Expected 3 datasets, got %d", len(datasets))
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

	hub := domain.NewEventBus(10)
	ctx := &domain.Context{Hub: hub}
	collector := NewZFSCollector(ctx)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool := &dto.ZFSPool{}
			collector.parseScanInfo(pool, tt.line)

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
	hub := domain.NewEventBus(10)
	ctx := &domain.Context{Hub: hub}
	collector := NewZFSCollector(ctx)

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
			result := collector.parseVdevLine(tt.line)
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
	hub := domain.NewEventBus(10)
	ctx := &domain.Context{Hub: hub}
	collector := NewZFSCollector(ctx)

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
			result := collector.parseVdevLine(tt.line)
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
