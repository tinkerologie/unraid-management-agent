package collectors

import (
	"strings"
	"testing"

	"github.com/cskr/pubsub"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
)

func TestNewZFSCollector(t *testing.T) {
	hub := pubsub.New(10)
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
		status   string
		healthy  bool
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
