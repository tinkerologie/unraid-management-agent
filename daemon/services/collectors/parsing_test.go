package collectors

import (
	"testing"

	"github.com/cskr/pubsub"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

// --- System Collector: parseSensorsOutput ---

func TestParseSensorsOutput(t *testing.T) {
	hub := pubsub.New(10)
	ctx := &domain.Context{Hub: hub}
	c := NewSystemCollector(ctx)

	tests := []struct {
		name     string
		output   string
		minCount int
	}{
		{
			name:     "typical sensors output",
			output:   "coretemp-isa-0000\nCore 0:\n  temp2_input: 45.000\nCore 1:\n  temp3_input: 47.000\n\nnct6775-isa-0a20\nMB Temp:\n  temp1_input: 32.000\nSYSTIN:\n  temp2_input: 33.000\n",
			minCount: 4,
		},
		{
			name:     "empty output",
			output:   "",
			minCount: 0,
		},
		{
			name:     "only chip headers no values",
			output:   "coretemp-isa-0000\nAdapter: ISA adapter\n",
			minCount: 0,
		},
		{
			name:     "single sensor",
			output:   "coretemp-isa-0000\nCore 0:\n  temp1_input: 55.000\n",
			minCount: 1,
		},
		{
			name:     "sensor with no label",
			output:   "hwmon-chip\n  temp1_input: 42.000\n",
			minCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := c.parseSensorsOutput(tt.output)
			if len(result) < tt.minCount {
				t.Errorf("Expected at least %d temperatures, got %d: %v", tt.minCount, len(result), result)
			}
		})
	}
}

// --- System Collector: parseFanSpeeds ---

func TestParseFanSpeeds(t *testing.T) {
	hub := pubsub.New(10)
	ctx := &domain.Context{Hub: hub}
	c := NewSystemCollector(ctx)

	tests := []struct {
		name     string
		output   string
		minCount int
	}{
		{
			name:     "typical fan output",
			output:   "nct6775-isa-0a20\n  fan1_input: 1200\n  fan2_input: 850\n",
			minCount: 2,
		},
		{
			name:     "empty output",
			output:   "",
			minCount: 0,
		},
		{
			name:     "no fan entries",
			output:   "coretemp-isa-0000\n  temp1_input: 55.000\n",
			minCount: 0,
		},
		{
			name:     "single fan",
			output:   "nct6775-isa-0a20\n  fan1_input: 900\n",
			minCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := c.parseFanSpeeds(tt.output)
			if len(result) < tt.minCount {
				t.Errorf("Expected at least %d fan speeds, got %d: %v", tt.minCount, len(result), result)
			}
		})
	}
}

// --- ZFS Collector: parseVdevLine ---

func TestParseVdevLine(t *testing.T) {
	hub := pubsub.New(10)
	ctx := &domain.Context{Hub: hub}
	c := NewZFSCollector(ctx)

	tests := []struct {
		name      string
		line      string
		wantNil   bool
		wantName  string
		wantType  string
		wantState string
		wantRead  uint64
		wantWrite uint64
		wantCksum uint64
	}{
		{"disk vdev", "sdg1      ONLINE       0     0     0", false, "sdg1", "disk", "ONLINE", 0, 0, 0},
		{"mirror vdev", "mirror-0  ONLINE       0     0     0", false, "mirror-0", "mirror", "ONLINE", 0, 0, 0},
		{"raidz1 vdev", "raidz1-0  ONLINE       0     0     0", false, "raidz1-0", "raidz1", "ONLINE", 0, 0, 0},
		{"raidz2 vdev", "raidz2-0  DEGRADED     0     0     0", false, "raidz2-0", "raidz2", "DEGRADED", 0, 0, 0},
		{"raidz3 vdev", "raidz3-0  ONLINE       0     0     0", false, "raidz3-0", "raidz3", "ONLINE", 0, 0, 0},
		{"cache vdev", "cache-0   ONLINE       0     0     0", false, "cache-0", "cache", "ONLINE", 0, 0, 0},
		{"log vdev", "log-0     ONLINE       0     0     0", false, "log-0", "log", "ONLINE", 0, 0, 0},
		{"spare vdev", "spare-0   AVAIL        0     0     0", false, "spare-0", "spare", "AVAIL", 0, 0, 0},
		{"with errors", "sda1      ONLINE       5     3     1", false, "sda1", "disk", "ONLINE", 5, 3, 1},
		{"too few fields", "sda1 ONLINE", true, "", "", "", 0, 0, 0},
		{"empty line", "", true, "", "", "", 0, 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := c.parseVdevLine(tt.line)
			if tt.wantNil {
				if result != nil {
					t.Errorf("Expected nil, got %+v", result)
				}
				return
			}
			if result == nil {
				t.Fatal("Expected non-nil result")
			}
			if result.Name != tt.wantName {
				t.Errorf("Name: got %q, want %q", result.Name, tt.wantName)
			}
			if result.Type != tt.wantType {
				t.Errorf("Type: got %q, want %q", result.Type, tt.wantType)
			}
			if result.State != tt.wantState {
				t.Errorf("State: got %q, want %q", result.State, tt.wantState)
			}
			if result.ReadErrors != tt.wantRead {
				t.Errorf("ReadErrors: got %d, want %d", result.ReadErrors, tt.wantRead)
			}
			if result.WriteErrors != tt.wantWrite {
				t.Errorf("WriteErrors: got %d, want %d", result.WriteErrors, tt.wantWrite)
			}
			if result.ChecksumErrors != tt.wantCksum {
				t.Errorf("ChecksumErrors: got %d, want %d", result.ChecksumErrors, tt.wantCksum)
			}
		})
	}
}

// --- ZFS Collector: parseScanInfo ---

func TestParseScanInfo(t *testing.T) {
	hub := pubsub.New(10)
	ctx := &domain.Context{Hub: hub}
	c := NewZFSCollector(ctx)

	tests := []struct {
		name         string
		line         string
		expectStatus string
		expectState  string
		expectErrors int
	}{
		{
			name:         "scrub in progress",
			line:         "scan: scrub in progress since Sun Nov 10 02:39:43 2025",
			expectStatus: "in progress",
			expectState:  "scanning",
		},
		{
			name:         "scrub completed no errors",
			line:         "scan: scrub repaired 0B in 00:00:01 with 0 errors on Sun Nov 10 02:39:43 2025",
			expectStatus: "scrub completed",
			expectState:  "finished",
			expectErrors: 0,
		},
		{
			name:         "scrub completed with errors",
			line:         "scan: scrub repaired 512B in 01:23:45 with 5 errors on Mon Dec 01 12:00:00 2025",
			expectStatus: "scrub completed",
			expectState:  "finished",
			expectErrors: 5,
		},
		{
			name:         "resilver in progress",
			line:         "scan: resilver in progress since Thu Jan 01 00:00:00 2025",
			expectStatus: "in progress",
			expectState:  "scanning",
		},
		{
			name:         "unknown scan line",
			line:         "scan: none requested",
			expectStatus: "",
			expectState:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool := &dto.ZFSPool{}
			c.parseScanInfo(pool, tt.line)
			if pool.ScanStatus != tt.expectStatus {
				t.Errorf("ScanStatus: got %q, want %q", pool.ScanStatus, tt.expectStatus)
			}
			if pool.ScanState != tt.expectState {
				t.Errorf("ScanState: got %q, want %q", pool.ScanState, tt.expectState)
			}
			if pool.ScanErrors != tt.expectErrors {
				t.Errorf("ScanErrors: got %d, want %d", pool.ScanErrors, tt.expectErrors)
			}
		})
	}
}

// --- ZFS Collector: parseDatasetLine ---

func TestParseDatasetLine_Extended(t *testing.T) {
	hub := pubsub.New(10)
	ctx := &domain.Context{Hub: hub}
	c := NewZFSCollector(ctx)

	tests := []struct {
		name      string
		line      string
		wantNil   bool
		wantName  string
		wantType  string
		wantRO    bool
		wantUsed  uint64
		wantAvail uint64
		wantMount string
		wantCompr string
	}{
		{
			name:      "full dataset",
			line:      "pool/data\tfilesystem\t1073741824\t10737418240\t536870912\t1.50x\t/mnt/pool/data\t0\t0\tlz4\toff",
			wantNil:   false,
			wantName:  "pool/data",
			wantType:  "filesystem",
			wantRO:    false,
			wantUsed:  1073741824,
			wantAvail: 10737418240,
			wantMount: "/mnt/pool/data",
			wantCompr: "lz4",
		},
		{
			name:     "readonly dataset",
			line:     "pool/snap\tfilesystem\t1024\t2048\t512\t1.00\t-\t0\t0\toff\ton",
			wantNil:  false,
			wantName: "pool/snap",
			wantType: "filesystem",
			wantRO:   true,
		},
		{
			name:      "no mountpoint (dash)",
			line:      "pool/vol\tvolume\t2048\t4096\t1024\t1.00x\t-\t0\t0\toff\toff",
			wantNil:   false,
			wantName:  "pool/vol",
			wantType:  "volume",
			wantMount: "",
		},
		{
			name:    "too few fields",
			line:    "pool/data\tfilesystem\t1024",
			wantNil: true,
		},
		{
			name:    "empty line",
			line:    "",
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := c.parseDatasetLine(tt.line)
			if tt.wantNil {
				if result != nil {
					t.Errorf("Expected nil, got %+v", result)
				}
				return
			}
			if result == nil {
				t.Fatal("Expected non-nil result")
			}
			if result.Name != tt.wantName {
				t.Errorf("Name: got %q, want %q", result.Name, tt.wantName)
			}
			if result.Type != tt.wantType {
				t.Errorf("Type: got %q, want %q", result.Type, tt.wantType)
			}
			if result.Readonly != tt.wantRO {
				t.Errorf("Readonly: got %v, want %v", result.Readonly, tt.wantRO)
			}
			if tt.wantUsed > 0 && result.UsedBytes != tt.wantUsed {
				t.Errorf("UsedBytes: got %d, want %d", result.UsedBytes, tt.wantUsed)
			}
			if tt.wantAvail > 0 && result.AvailableBytes != tt.wantAvail {
				t.Errorf("AvailableBytes: got %d, want %d", result.AvailableBytes, tt.wantAvail)
			}
			if tt.wantMount != "" && result.Mountpoint != tt.wantMount {
				t.Errorf("Mountpoint: got %q, want %q", result.Mountpoint, tt.wantMount)
			}
			if tt.wantCompr != "" && result.Compression != tt.wantCompr {
				t.Errorf("Compression: got %q, want %q", result.Compression, tt.wantCompr)
			}
		})
	}
}

// --- ZFS Collector: parseSnapshotLine ---

func TestParseSnapshotLine_Extended(t *testing.T) {
	hub := pubsub.New(10)
	ctx := &domain.Context{Hub: hub}
	c := NewZFSCollector(ctx)

	tests := []struct {
		name        string
		line        string
		wantNil     bool
		wantName    string
		wantDataset string
		wantUsed    uint64
	}{
		{
			name:        "valid snapshot",
			line:        "pool/data@autosnap_2025-01-01\t1024\t2048\t1735689600",
			wantNil:     false,
			wantName:    "pool/data@autosnap_2025-01-01",
			wantDataset: "pool/data",
			wantUsed:    1024,
		},
		{
			name:    "no @ in name",
			line:    "pool/data\t1024\t2048\t1735689600",
			wantNil: true,
		},
		{
			name:    "too few fields",
			line:    "pool/data@snap\t1024",
			wantNil: true,
		},
		{
			name:    "empty line",
			line:    "",
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := c.parseSnapshotLine(tt.line)
			if tt.wantNil {
				if result != nil {
					t.Errorf("Expected nil, got %+v", result)
				}
				return
			}
			if result == nil {
				t.Fatal("Expected non-nil result")
			}
			if result.Name != tt.wantName {
				t.Errorf("Name: got %q, want %q", result.Name, tt.wantName)
			}
			if result.Dataset != tt.wantDataset {
				t.Errorf("Dataset: got %q, want %q", result.Dataset, tt.wantDataset)
			}
			if result.UsedBytes != tt.wantUsed {
				t.Errorf("UsedBytes: got %d, want %d", result.UsedBytes, tt.wantUsed)
			}
		})
	}
}

// --- Parity Collector: parseLine ---

func TestParityParseLine_Extended(t *testing.T) {
	c := NewParityCollector()

	tests := []struct {
		name         string
		line         string
		expectErr    bool
		expectAction string
		expectStatus string
	}{
		{
			name:         "5-field format old",
			line:         "2022 May 22 20:17:49|73068|54.8 MB/s|0|0",
			expectErr:    false,
			expectAction: "Parity-Check",
			expectStatus: "OK",
		},
		{
			name:         "7-field format",
			line:         "2024 Nov 30 00:30:26|100888|99128056|0|0|check P|9766436812",
			expectErr:    false,
			expectAction: "Parity-Check",
			expectStatus: "OK",
		},
		{
			name:         "7-field dual parity",
			line:         "2024 Nov 30 00:30:26|100888|99128056|0|0|check P Q|9766436812",
			expectErr:    false,
			expectAction: "Dual Parity-Check",
			expectStatus: "OK",
		},
		{
			name:         "with errors",
			line:         "2024 Nov 30 00:30:26|100888|99128056|0|5|check P|9766436812",
			expectErr:    false,
			expectStatus: "5 errors",
		},
		{
			name:         "canceled exit code",
			line:         "2024 Nov 30 00:30:26|100888|99128056|-4|0|check P|9766436812",
			expectErr:    false,
			expectStatus: "Canceled",
		},
		{
			name:         "recon parity sync",
			line:         "2024 Nov 30 00:30:26|100888|99128056|0|0|recon P|9766436812",
			expectErr:    false,
			expectAction: "Parity-Sync",
		},
		{
			name:         "recon dual parity sync",
			line:         "2024 Nov 30 00:30:26|100888|99128056|0|0|recon P Q|9766436812",
			expectErr:    false,
			expectAction: "Dual Parity-Sync",
		},
		{
			name:         "clear action",
			line:         "2024 Nov 30 00:30:26|100888|99128056|0|0|clear|9766436812",
			expectErr:    false,
			expectAction: "Parity-Clear",
		},
		{
			name:         "single-digit day double space",
			line:         "2025 Jan  2 06:25:17|100888|99128056|0|0|check P|9766436812",
			expectErr:    false,
			expectAction: "Parity-Check",
		},
		{
			name:         "unknown exit code",
			line:         "2024 Nov 30 00:30:26|100888|99128056|-99|0|check P|9766436812",
			expectErr:    false,
			expectStatus: "Exit code -99",
		},
		{
			name:      "too few fields",
			line:      "2024 Nov 30|100888",
			expectErr: true,
		},
		{
			name:      "invalid date",
			line:      "notadate|100888|99128056|0|0",
			expectErr: true,
		},
		{
			name:      "empty line",
			line:      "",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			record, err := c.parseLine(tt.line)
			if tt.expectErr {
				if err == nil {
					t.Error("Expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			if tt.expectAction != "" && record.Action != tt.expectAction {
				t.Errorf("Action: got %q, want %q", record.Action, tt.expectAction)
			}
			if tt.expectStatus != "" && record.Status != tt.expectStatus {
				t.Errorf("Status: got %q, want %q", record.Status, tt.expectStatus)
			}
		})
	}
}

// --- Parity Collector: parseSpeed ---

func TestParityParseSpeed_Extended(t *testing.T) {
	c := NewParityCollector()

	tests := []struct {
		name   string
		input  string
		minVal float64
		maxVal float64
	}{
		{"raw bytes/sec", "99128056", 94.0, 95.0},
		{"MB/s format", "54.8 MB/s", 54.0, 55.0},
		{"GB/s format", "1.0 GB/s", 1023.0, 1025.0},
		{"KB/s format", "512 KB/s", 0.4, 0.6},
		{"empty string", "", 0, 0},
		{"unavailable", "Unavailable", 0, 0},
		{"unavailable lowercase", "unavailable", 0, 0},
		{"B/s format", "1048576 B/s", 0.9, 1.1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := c.parseSpeed(tt.input)
			if result < tt.minVal || result > tt.maxVal {
				t.Errorf("parseSpeed(%q) = %f, want between %f and %f", tt.input, result, tt.minVal, tt.maxVal)
			}
		})
	}
}

// --- Parity Collector: parseAction ---

func TestParityParseAction_Extended(t *testing.T) {
	c := NewParityCollector()

	tests := []struct {
		input    string
		expected string
	}{
		{"check P", "Parity-Check"},
		{"check P Q", "Dual Parity-Check"},
		{"recon P", "Parity-Sync"},
		{"recon P Q", "Dual Parity-Sync"},
		{"clear", "Parity-Clear"},
		{"unknown", "unknown"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run("action_"+tt.input, func(t *testing.T) {
			result := c.parseAction(tt.input)
			if result != tt.expected {
				t.Errorf("parseAction(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}
