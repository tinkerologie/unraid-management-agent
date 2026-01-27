package controllers

import (
	"fmt"
	"net/url"

	"github.com/digitalocean/go-libvirt"

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

// Start starts a virtual machine by name using the libvirt API.
func (vc *VMController) Start(vmName string) error {
	logger.Info("Starting VM: %s", vmName)

	l, domain, err := vc.connect(vmName)
	if err != nil {
		return err
	}
	defer l.Disconnect() //nolint:errcheck

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
func (vc *VMController) Resume(vmName string) error {
	logger.Info("Resuming VM: %s", vmName)

	l, domain, err := vc.connect(vmName)
	if err != nil {
		return err
	}
	defer l.Disconnect() //nolint:errcheck

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
