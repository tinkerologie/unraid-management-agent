package controllers

import (
	"fmt"
	"strings"
	"time"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/constants"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/lib"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
)

// RemoteShareController provides mount/unmount operations for Unassigned Devices
// SMB/NFS remote shares. Operations are delegated to the plugin's rc.unassigned
// control script so that credentials, protocol versions, and mount options are
// handled exactly as they are from the Unraid web UI.
type RemoteShareController struct{}

// NewRemoteShareController creates a new remote share controller.
func NewRemoteShareController() *RemoteShareController {
	return &RemoteShareController{}
}

// Mount mounts the configured remote share identified by source
// ("//server/share" for SMB or "server:/export" for NFS).
func (rc *RemoteShareController) Mount(source string) error {
	return rc.run("mount", source)
}

// Unmount unmounts the remote share identified by source.
func (rc *RemoteShareController) Unmount(source string) error {
	return rc.run("umount", source)
}

// run validates the source and invokes the rc.unassigned control script. The
// script only acts on shares present in the Unassigned Devices configuration,
// so an unknown source is a safe no-op.
func (rc *RemoteShareController) run(action, source string) error {
	if err := lib.ValidateRemoteShareSource(source); err != nil {
		return fmt.Errorf("validate remote share source: %w", err)
	}

	logger.Info("RemoteShare: %s remote share %q", action, source)

	output, err := lib.ExecCommandWithTimeout(60*time.Second, constants.RcUnassignedBin, action, source)
	if err != nil {
		return fmt.Errorf("failed to %s remote share %q: %w (output: %s)",
			action, source, err, strings.TrimSpace(strings.Join(output, "\n")))
	}

	logger.Info("RemoteShare: %s of %q completed", action, source)
	return nil
}
