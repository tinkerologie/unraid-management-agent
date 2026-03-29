package controllers

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/lib"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
)

// ipmitoolBin is the path to the ipmitool binary.
const ipmitoolBin = "/usr/bin/ipmitool"

// IPMIProvider reads and controls fans via IPMI (for server-grade boards with a BMC).
type IPMIProvider struct {
	available bool
}

// NewIPMIProvider creates a new IPMI fan provider.
func NewIPMIProvider() *IPMIProvider {
	return &IPMIProvider{}
}

// IsAvailable checks whether ipmitool is present and the BMC responds.
func (p *IPMIProvider) IsAvailable() bool {
	_, err := lib.ExecCommandOutput(ipmitoolBin, "sdr", "type", "Fan")
	p.available = err == nil
	if p.available {
		logger.Debug("IPMI: Fan control available via ipmitool")
	}
	return p.available
}

// ReadAll parses IPMI fan sensors and returns fan devices.
// Output example: "FAN1             | 3600 RPM          | ok"
func (p *IPMIProvider) ReadAll() []dto.FanDevice {
	if !p.available {
		return nil
	}

	output, err := lib.ExecCommandOutput(ipmitoolBin, "sdr", "type", "Fan")
	if err != nil {
		logger.Debug("IPMI: Failed to read fan sensors: %v", err)
		return nil
	}

	var fans []dto.FanDevice
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.Split(line, "|")
		if len(parts) < 3 {
			continue
		}

		name := strings.TrimSpace(parts[0])
		valueStr := strings.TrimSpace(parts[1])

		// Parse RPM from value field (e.g. "3600 RPM")
		rpm := 0
		if fields := strings.Fields(valueStr); len(fields) >= 1 {
			if v, err := strconv.Atoi(fields[0]); err == nil {
				rpm = v
			}
		}

		fanID := "ipmi_" + sanitizeFanName(name)
		fans = append(fans, dto.FanDevice{
			ID:           fanID,
			Name:         name,
			RPM:          rpm,
			Mode:         dto.FanModeAutomatic,
			Controllable: true, // IPMI fans are typically controllable
		})
	}

	return fans
}

// SetManualMode enables manual fan control via IPMI raw command.
// This sets the BMC to manual (non-automatic) fan duty cycle mode.
func (p *IPMIProvider) SetManualMode() error {
	if !p.available {
		return fmt.Errorf("IPMI not available")
	}
	// Standard IPMI raw command to set manual fan mode
	_, err := lib.ExecCommand(ipmitoolBin, "raw", "0x30", "0x30", "0x01", "0x00")
	return err
}

// SetAutomaticMode restores automatic (BMC-controlled) fan speed.
func (p *IPMIProvider) SetAutomaticMode() error {
	if !p.available {
		return fmt.Errorf("IPMI not available")
	}
	_, err := lib.ExecCommand(ipmitoolBin, "raw", "0x30", "0x30", "0x01", "0x01")
	return err
}

// SetDutyAll sets the fan duty cycle for all IPMI fans (0-100%).
func (p *IPMIProvider) SetDutyAll(percent int) error {
	if !p.available {
		return fmt.Errorf("IPMI not available")
	}
	if percent < 0 || percent > 100 {
		return fmt.Errorf("duty percent must be 0-100, got %d", percent)
	}
	hexPercent := fmt.Sprintf("0x%02x", percent)
	_, err := lib.ExecCommand(ipmitoolBin, "raw", "0x30", "0x30", "0x02", "0xff", hexPercent)
	return err
}

// sanitizeFanName converts an IPMI sensor name to a safe identifier component.
func sanitizeFanName(name string) string {
	name = strings.ToLower(name)
	name = strings.ReplaceAll(name, " ", "_")
	// only keep alphanumeric and underscores
	var b strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
		}
	}
	return b.String()
}
