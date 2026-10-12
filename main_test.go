package main

import (
	"testing"

	"github.com/tinkerologie/unraid-management-agent/daemon/domain"
)

func TestApplyFileConfigStorageTopologyInterval(t *testing.T) {
	orig := cli.IntervalStorageTopology
	t.Cleanup(func() { cli.IntervalStorageTopology = orig })

	interval := 900
	applyFileConfig(&domain.FileConfig{Intervals: &domain.FileConfigIntervals{StorageTopology: &interval}})
	if cli.IntervalStorageTopology != 900 {
		t.Errorf("IntervalStorageTopology = %d, want 900", cli.IntervalStorageTopology)
	}
	if !validCollectorNames["storage_topology"] {
		t.Error("storage_topology must be a valid collector name for --disable-collectors")
	}
}

func TestApplyFileConfigNICInterval(t *testing.T) {
	orig := cli.IntervalNIC
	t.Cleanup(func() { cli.IntervalNIC = orig })

	interval := 120
	applyFileConfig(&domain.FileConfig{Intervals: &domain.FileConfigIntervals{NIC: &interval}})
	if cli.IntervalNIC != 120 {
		t.Errorf("IntervalNIC = %d, want 120", cli.IntervalNIC)
	}
	if !validCollectorNames["nic"] {
		t.Error("nic must be a valid collector name for --disable-collectors")
	}
}
