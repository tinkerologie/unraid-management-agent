package controllers

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/digitalocean/go-libvirt"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/constants"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/lib"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
)

// VMController provides control operations for virtual machines using the libvirt Go API.
// It handles VM lifecycle operations including start, stop, restart, pause, resume, hibernate, and force stop.
type VMController struct{}

// NewVMController creates a new VM controller.
func NewVMController() *VMController {
	return &VMController{}
}

// connect establishes a connection to libvirt and returns the connection and domain.
func (vc *VMController) connect(vmName string) (*libvirt.Libvirt, libvirt.Domain, error) {
	uri, _ := url.Parse(string(libvirt.QEMUSystem))
	l, err := libvirt.ConnectToURI(uri)
	if err != nil {
		return nil, libvirt.Domain{}, fmt.Errorf("failed to connect to libvirt: %w", err)
	}

	domain, err := l.DomainLookupByName(vmName)
	if err != nil {
		if disconnectErr := l.Disconnect(); disconnectErr != nil {
			logger.Debug("VM: Error disconnecting from libvirt: %v", disconnectErr)
		}
		return nil, libvirt.Domain{}, fmt.Errorf("VM '%s' not found: %w", vmName, err)
	}

	return l, domain, nil
}

// pmWakeup wakes a VM from pmsuspended state (e.g. Windows sleep) using virsh dompmwakeup.
// libvirt DomainResume and DomainCreate do not work for pmsuspended domains.
func (vc *VMController) pmWakeup(vmName string) error {
	_, err := lib.ExecCommand(constants.VirshBin, "dompmwakeup", vmName)
	if err != nil {
		return fmt.Errorf("failed to wake VM from pmsuspended: %w", err)
	}
	return nil
}

// Start starts a virtual machine by name using the libvirt API.
// If the VM is pmsuspended (e.g. Windows sleep), uses virsh dompmwakeup instead.
func (vc *VMController) Start(vmName string) error {
	logger.Info("Starting VM: %s", vmName)

	l, domain, err := vc.connect(vmName)
	if err != nil {
		return err
	}
	defer l.Disconnect() //nolint:errcheck

	state, _, err := l.DomainGetState(domain, 0)
	if err == nil && libvirt.DomainState(state) == libvirt.DomainPmsuspended {
		if err := vc.pmWakeup(vmName); err != nil {
			return err
		}
		logger.Info("Successfully woke VM from pmsuspended: %s", vmName)
		return nil
	}

	if err := l.DomainCreate(domain); err != nil {
		return fmt.Errorf("failed to start VM %s: %w", vmName, err)
	}

	logger.Info("Successfully started VM: %s", vmName)
	return nil
}

// Stop gracefully shuts down a virtual machine by name using the libvirt API.
func (vc *VMController) Stop(vmName string) error {
	logger.Info("Stopping VM: %s", vmName)

	l, domain, err := vc.connect(vmName)
	if err != nil {
		return err
	}
	defer l.Disconnect() //nolint:errcheck

	if err := l.DomainShutdown(domain); err != nil {
		return fmt.Errorf("failed to shutdown VM %s: %w", vmName, err)
	}

	logger.Info("Successfully initiated shutdown for VM: %s", vmName)
	return nil
}

// Restart reboots a virtual machine by name using the libvirt API.
func (vc *VMController) Restart(vmName string) error {
	logger.Info("Restarting VM: %s", vmName)

	l, domain, err := vc.connect(vmName)
	if err != nil {
		return err
	}
	defer l.Disconnect() //nolint:errcheck

	// Reboot with default flags
	if err := l.DomainReboot(domain, 0); err != nil {
		return fmt.Errorf("failed to reboot VM %s: %w", vmName, err)
	}

	logger.Info("Successfully initiated reboot for VM: %s", vmName)
	return nil
}

// Pause suspends a running virtual machine by name using the libvirt API.
func (vc *VMController) Pause(vmName string) error {
	logger.Info("Pausing VM: %s", vmName)

	l, domain, err := vc.connect(vmName)
	if err != nil {
		return err
	}
	defer l.Disconnect() //nolint:errcheck

	if err := l.DomainSuspend(domain); err != nil {
		return fmt.Errorf("failed to suspend VM %s: %w", vmName, err)
	}

	logger.Info("Successfully paused VM: %s", vmName)
	return nil
}

// Resume resumes a paused virtual machine by name using the libvirt API.
// If the VM is pmsuspended (e.g. Windows sleep), uses virsh dompmwakeup instead.
func (vc *VMController) Resume(vmName string) error {
	logger.Info("Resuming VM: %s", vmName)

	l, domain, err := vc.connect(vmName)
	if err != nil {
		return err
	}
	defer l.Disconnect() //nolint:errcheck

	state, _, err := l.DomainGetState(domain, 0)
	if err == nil && libvirt.DomainState(state) == libvirt.DomainPmsuspended {
		if err := vc.pmWakeup(vmName); err != nil {
			return err
		}
		logger.Info("Successfully woke VM from pmsuspended: %s", vmName)
		return nil
	}

	if err := l.DomainResume(domain); err != nil {
		return fmt.Errorf("failed to resume VM %s: %w", vmName, err)
	}

	logger.Info("Successfully resumed VM: %s", vmName)
	return nil
}

// Hibernate saves the VM state to disk and stops it using the libvirt API.
func (vc *VMController) Hibernate(vmName string) error {
	logger.Info("Hibernating VM: %s", vmName)

	l, domain, err := vc.connect(vmName)
	if err != nil {
		return err
	}
	defer l.Disconnect() //nolint:errcheck

	// ManagedSave saves the domain state to a file then stops it
	if err := l.DomainManagedSave(domain, 0); err != nil {
		return fmt.Errorf("failed to hibernate VM %s: %w", vmName, err)
	}

	logger.Info("Successfully hibernated VM: %s", vmName)
	return nil
}

// ForceStop immediately terminates a virtual machine by name without graceful shutdown using the libvirt API.
func (vc *VMController) ForceStop(vmName string) error {
	logger.Info("Force stopping VM: %s", vmName)

	l, domain, err := vc.connect(vmName)
	if err != nil {
		return err
	}
	defer l.Disconnect() //nolint:errcheck

	// Destroy immediately terminates the domain
	if err := l.DomainDestroy(domain); err != nil {
		return fmt.Errorf("failed to force stop VM %s: %w", vmName, err)
	}

	logger.Info("Successfully force stopped VM: %s", vmName)
	return nil
}

// CreateSnapshot creates a snapshot of a virtual machine using the libvirt API.
func (vc *VMController) CreateSnapshot(vmName, snapshotName, description string) error {
	logger.Info("Creating snapshot '%s' for VM: %s", snapshotName, vmName)

	l, domain, err := vc.connect(vmName)
	if err != nil {
		return err
	}
	defer l.Disconnect() //nolint:errcheck

	// Build snapshot XML
	descXML := ""
	if description != "" {
		descXML = fmt.Sprintf("<description>%s</description>", description)
	}

	xmlDesc := fmt.Sprintf(`<domainsnapshot><name>%s</name>%s</domainsnapshot>`, snapshotName, descXML)

	// Create the snapshot (flags=0 for default behavior)
	_, err = l.DomainSnapshotCreateXML(domain, xmlDesc, 0)
	if err != nil {
		return fmt.Errorf("failed to create snapshot '%s' for VM %s: %w", snapshotName, vmName, err)
	}

	logger.Info("Successfully created snapshot '%s' for VM: %s", snapshotName, vmName)
	return nil
}

// ListSnapshots lists all snapshots for a virtual machine.
func (vc *VMController) ListSnapshots(vmName string) (*dto.VMSnapshotList, error) {
	logger.Debug("Listing snapshots for VM: %s", vmName)

	// Use virsh snapshot-list to get snapshot details
	lines, err := lib.ExecCommand(constants.VirshBin, "snapshot-list", vmName, "--name")
	if err != nil {
		return nil, fmt.Errorf("failed to list snapshots for VM %s: %w", vmName, err)
	}

	result := &dto.VMSnapshotList{
		VMName:    vmName,
		Snapshots: make([]dto.VMSnapshot, 0),
		Timestamp: time.Now(),
	}

	// Get current snapshot name
	currentLines, _ := lib.ExecCommand(constants.VirshBin, "snapshot-current", vmName, "--name")
	currentSnapshot := ""
	if len(currentLines) > 0 {
		currentSnapshot = strings.TrimSpace(currentLines[0])
	}

	for _, line := range lines {
		name := strings.TrimSpace(line)
		if name == "" {
			continue
		}

		snapshot := dto.VMSnapshot{
			Name:      name,
			VMName:    vmName,
			IsCurrent: name == currentSnapshot,
		}

		// Get snapshot details via virsh snapshot-info
		infoLines, err := lib.ExecCommand(constants.VirshBin, "snapshot-info", vmName, name)
		if err == nil {
			for _, infoLine := range infoLines {
				parts := strings.SplitN(infoLine, ":", 2)
				if len(parts) != 2 {
					continue
				}
				key := strings.TrimSpace(parts[0])
				value := strings.TrimSpace(parts[1])

				switch key {
				case "Description":
					snapshot.Description = value
				case "State":
					snapshot.State = value
				case "Creation Time":
					snapshot.CreatedAt = value
				case "Parent":
					snapshot.Parent = value
				}
			}
		}

		result.Snapshots = append(result.Snapshots, snapshot)
	}

	result.Count = len(result.Snapshots)
	logger.Debug("Found %d snapshots for VM: %s", result.Count, vmName)
	return result, nil
}

// DeleteSnapshot deletes a snapshot of a virtual machine.
func (vc *VMController) DeleteSnapshot(vmName, snapshotName string) error {
	logger.Info("Deleting snapshot '%s' for VM: %s", snapshotName, vmName)

	l, domain, err := vc.connect(vmName)
	if err != nil {
		return err
	}
	defer l.Disconnect() //nolint:errcheck

	// Look up the snapshot
	snapshot, err := l.DomainSnapshotLookupByName(domain, snapshotName, 0)
	if err != nil {
		return fmt.Errorf("snapshot '%s' not found for VM %s: %w", snapshotName, vmName, err)
	}

	// Delete the snapshot (flags=0 for default behavior)
	if err := l.DomainSnapshotDelete(snapshot, 0); err != nil {
		return fmt.Errorf("failed to delete snapshot '%s' for VM %s: %w", snapshotName, vmName, err)
	}

	logger.Info("Successfully deleted snapshot '%s' for VM: %s", snapshotName, vmName)
	return nil
}

// RestoreSnapshot restores a virtual machine to a previously created snapshot.
// WARNING: This is a destructive operation — the VM's current state is lost and replaced with the snapshot state.
func (vc *VMController) RestoreSnapshot(vmName, snapshotName string) error {
	logger.Info("Restoring snapshot '%s' for VM: %s", snapshotName, vmName)

	l, domain, err := vc.connect(vmName)
	if err != nil {
		return err
	}
	defer l.Disconnect() //nolint:errcheck

	// Look up the snapshot
	snapshot, err := l.DomainSnapshotLookupByName(domain, snapshotName, 0)
	if err != nil {
		return fmt.Errorf("snapshot '%s' not found for VM %s: %w", snapshotName, vmName, err)
	}

	// Revert to snapshot (flags=0 for default behavior)
	if err := l.DomainRevertToSnapshot(snapshot, 0); err != nil {
		return fmt.Errorf("failed to restore snapshot '%s' for VM %s: %w", snapshotName, vmName, err)
	}

	logger.Info("Successfully restored snapshot '%s' for VM: %s", snapshotName, vmName)
	return nil
}

// CloneVM clones a virtual machine using virt-clone.
// The source VM must be shut off before cloning.
func (vc *VMController) CloneVM(vmName, cloneName string) error {
	logger.Info("Cloning VM '%s' as '%s'", vmName, cloneName)

	// virt-clone handles copying disk images and generating new UUIDs/MACs
	output, err := lib.ExecCommandOutput(
		constants.VirtCloneBin,
		"--original", vmName,
		"--name", cloneName,
		"--auto-clone",
	)
	if err != nil {
		return fmt.Errorf("failed to clone VM '%s' as '%s': %w (output: %s)", vmName, cloneName, err, output)
	}

	logger.Info("Successfully cloned VM '%s' as '%s'", vmName, cloneName)
	return nil
}
