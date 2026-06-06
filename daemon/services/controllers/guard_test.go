package controllers

import (
	"strings"
	"testing"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/platform"
)

func TestRequireBinary(t *testing.T) {
	if err := requireBinary("array", "/nonexistent/path/mdcmd-xyzzy"); err == nil {
		t.Fatal("expected error for missing binary")
	} else if !strings.Contains(err.Error(), "unavailable") {
		t.Errorf("error should mention 'unavailable', got: %v", err)
	}

	// /bin/sh exists on macOS and Linux (CI).
	if err := requireBinary("vm", "/bin/sh"); err != nil {
		t.Errorf("expected nil for present binary, got: %v", err)
	}
}

// TestArrayControlGatedWhenUnavailable verifies the capability gate: on a host
// without /proc/mdcmd and without the mdcmd binary (dev/CI), array control
// returns a clear "unavailable" error rather than a cryptic exec failure.
func TestArrayControlGatedWhenUnavailable(t *testing.T) {
	if platform.PathExists("/proc/mdcmd") || platform.BinaryExists("/usr/local/sbin/mdcmd") {
		t.Skip("mdcmd available on this host; capability gate not exercised")
	}
	ac := NewArrayController(&domain.Context{})
	err := ac.StartArray()
	if err == nil {
		t.Skip("mdcmd available on this host; gate not exercised")
	}
	if !strings.Contains(err.Error(), "unavailable") {
		t.Errorf("expected clear 'unavailable' error, got: %v", err)
	}
}
