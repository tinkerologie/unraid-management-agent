package controllers

import (
	"fmt"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/platform"
)

// requireBinary returns a typed, human-readable error when the binary needed for
// a control operation is unavailable, so callers get a clear "<subsystem>
// control unavailable" message instead of a cryptic shell/exec failure. This is
// the OS-resilience capability gate for control paths that shell out to a binary
// (e.g. virsh, mdcmd). Native-API control paths (Docker SDK, libvirt API)
// already surface clear connection errors and do not need this gate.
func requireBinary(subsystem, binaryPath string) error {
	if !platform.BinaryExists(binaryPath) {
		return fmt.Errorf("%s control unavailable: required binary %s not found", subsystem, binaryPath)
	}
	return nil
}
