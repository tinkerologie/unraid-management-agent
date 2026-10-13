package main

import (
	"strings"
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

func TestApplyFileConfigAllowUnauthenticatedWrites(t *testing.T) {
	orig := cli.AllowUnauthenticatedWrites
	t.Cleanup(func() { cli.AllowUnauthenticatedWrites = orig })

	cli.AllowUnauthenticatedWrites = false
	applyFileConfig(&domain.FileConfig{})
	if cli.AllowUnauthenticatedWrites {
		t.Error("AllowUnauthenticatedWrites must stay false when the file does not set it")
	}

	allow := true
	applyFileConfig(&domain.FileConfig{AllowUnauthenticatedWrites: &allow})
	if !cli.AllowUnauthenticatedWrites {
		t.Error("AllowUnauthenticatedWrites = false, want true from the config file")
	}
}

func TestWriteAccessWarning(t *testing.T) {
	tests := []struct {
		name    string
		token   string
		allow   bool
		wantSub string
	}{
		{"token set", "secret-token", false, ""},
		{"token set with opt-out", "secret-token", true, ""},
		{"no token locks writes", "", false, "network write access is disabled"},
		{"whitespace token locks writes", "  ", false, "network write access is disabled"},
		{"no token with opt-out warns", "", true, "anyone on the network can control"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := writeAccessWarning(tt.token, tt.allow)
			if tt.wantSub == "" {
				if got != "" {
					t.Errorf("writeAccessWarning() = %q, want empty", got)
				}
				return
			}
			if !strings.Contains(got, tt.wantSub) {
				t.Errorf("writeAccessWarning() = %q, want it to contain %q", got, tt.wantSub)
			}
		})
	}
}

func TestApplyWriteAccess(t *testing.T) {
	for _, allow := range []bool{false, true} {
		appCtx := &domain.Context{}
		applyWriteAccess(appCtx, "", allow)
		if appCtx.AllowUnauthenticatedWrites != allow {
			t.Errorf("AllowUnauthenticatedWrites = %v, want %v", appCtx.AllowUnauthenticatedWrites, allow)
		}
		if appCtx.WritesLocked() == allow {
			t.Errorf("WritesLocked() = %v with no token and opt-out %v", appCtx.WritesLocked(), allow)
		}
	}
}
