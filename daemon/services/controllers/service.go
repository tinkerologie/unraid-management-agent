package controllers

import (
	"fmt"
	"strings"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/lib"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
)

// ServiceController provides control operations for Unraid system services.
// It handles starting, stopping, and restarting services like Docker, libvirt, SMB, NFS, etc.
type ServiceController struct{}

// NewServiceController creates a new service controller.
func NewServiceController() *ServiceController {
	return &ServiceController{}
}

// serviceMap maps service names to their rc script paths.
var serviceMap = map[string]string{
	"docker":    "/etc/rc.d/rc.docker",
	"libvirt":   "/etc/rc.d/rc.libvirt",
	"smb":       "/etc/rc.d/rc.samba",
	"samba":     "/etc/rc.d/rc.samba",
	"nfs":       "/etc/rc.d/rc.nfsd",
	"ftp":       "/etc/rc.d/rc.proftpd",
	"sshd":      "/etc/rc.d/rc.sshd",
	"ssh":       "/etc/rc.d/rc.sshd",
	"nginx":     "/etc/rc.d/rc.nginx",
	"syslog":    "/etc/rc.d/rc.rsyslogd",
	"ntpd":      "/etc/rc.d/rc.ntpd",
	"ntp":       "/etc/rc.d/rc.ntpd",
	"avahi":     "/etc/rc.d/rc.avahidaemon",
	"wireguard": "/etc/rc.d/rc.wireguard",
}

// validActions are the allowed service actions.
var validActions = map[string]bool{
	"start":   true,
	"stop":    true,
	"restart": true,
	"status":  true,
}

// ValidServiceNames returns the list of supported service names.
func ValidServiceNames() []string {
	// Return unique service names (not aliases)
	return []string{
		"docker", "libvirt", "smb", "nfs", "ftp",
		"sshd", "nginx", "syslog", "ntpd", "avahi", "wireguard",
	}
}

// StartService starts an Unraid system service.
func (sc *ServiceController) StartService(serviceName string) error {
	return sc.executeAction(serviceName, "start")
}

// StopService stops an Unraid system service.
func (sc *ServiceController) StopService(serviceName string) error {
	return sc.executeAction(serviceName, "stop")
}

// RestartService restarts an Unraid system service.
func (sc *ServiceController) RestartService(serviceName string) error {
	return sc.executeAction(serviceName, "restart")
}

// GetServiceStatus checks if a service is running.
func (sc *ServiceController) GetServiceStatus(serviceName string) (bool, error) {
	rcScript, ok := serviceMap[strings.ToLower(serviceName)]
	if !ok {
		return false, fmt.Errorf("unknown service: %s (valid: %s)", serviceName, strings.Join(ValidServiceNames(), ", "))
	}

	output, err := lib.ExecCommandOutput(rcScript, "status")
	if err != nil {
		// Most rc scripts return non-zero exit code when service is stopped
		return false, nil
	}

	// Check common status indicators
	outputLower := strings.ToLower(output)
	return strings.Contains(outputLower, "running") ||
		strings.Contains(outputLower, "is running") ||
		strings.Contains(outputLower, "started") ||
		strings.Contains(outputLower, "active"), nil
}

// executeAction executes a service action (start, stop, restart).
func (sc *ServiceController) executeAction(serviceName, action string) error {
	serviceName = strings.ToLower(serviceName)

	if !validActions[action] {
		return fmt.Errorf("invalid action: %s (valid: start, stop, restart)", action)
	}

	rcScript, ok := serviceMap[serviceName]
	if !ok {
		return fmt.Errorf("unknown service: %s (valid: %s)", serviceName, strings.Join(ValidServiceNames(), ", "))
	}

	logger.Info("Service: Executing %s on %s (%s)", action, serviceName, rcScript)

	_, err := lib.ExecCommand(rcScript, action)
	if err != nil {
		return fmt.Errorf("failed to %s service %s: %w", action, serviceName, err)
	}

	logger.Info("Service: Successfully executed %s on %s", action, serviceName)
	return nil
}
