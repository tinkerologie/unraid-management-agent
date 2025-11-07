package collectors

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/lib"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
)

type UPSCollector struct {
	ctx *domain.Context
}

func NewUPSCollector(ctx *domain.Context) *UPSCollector {
	return &UPSCollector{ctx: ctx}
}

func (c *UPSCollector) Start(ctx context.Context, interval time.Duration) {
	logger.Info("Starting ups collector (interval: %v)", interval)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info("UPS collector stopping due to context cancellation")
			return
		case <-ticker.C:
			c.Collect()
		}
	}
}

func (c *UPSCollector) Collect() {

	logger.Debug("Collecting ups data...")

	// Try apcaccess first (APC UPS)
	var upsData *dto.UPSStatus
	var err error

	if lib.CommandExists("apcaccess") {
		upsData, err = c.collectAPC()
		if err == nil {
			c.ctx.Hub.Pub(upsData, "ups_status_update")
			logger.Debug("Published ups_status_update event (APC)")
			return
		}
		logger.Warning("Failed to collect APC UPS data", "error", err)
	}

	// Fallback to upsc (NUT - Network UPS Tools)
	if lib.CommandExists("upsc") {
		upsData, err = c.collectNUT()
		if err == nil {
			c.ctx.Hub.Pub(upsData, "ups_status_update")
			logger.Debug("Published ups_status_update event (NUT)")
			return
		}
		logger.Warning("Failed to collect NUT UPS data", "error", err)
	}

	// No UPS available
	logger.Debug("No UPS detected or configured")
}

func (c *UPSCollector) collectAPC() (*dto.UPSStatus, error) {
	output, err := lib.ExecCommandOutput("apcaccess")
	if err != nil {
		return nil, err
	}

	status := &dto.UPSStatus{
		Connected: true,
		Timestamp: time.Now(),
	}

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		switch key {
		case "STATUS":
			status.Status = value
		case "LOADPCT":
			if strings.HasSuffix(value, "Percent") {
				value = strings.TrimSuffix(value, " Percent")
			}
			if load, err := strconv.ParseFloat(value, 64); err == nil {
				status.LoadPercent = load
			}
		case "BCHARGE":
			if strings.HasSuffix(value, "Percent") {
				value = strings.TrimSuffix(value, " Percent")
			}
			if charge, err := strconv.ParseFloat(value, 64); err == nil {
				status.BatteryCharge = charge
			}
		case "TIMELEFT":
			if strings.HasSuffix(value, "Minutes") {
				value = strings.TrimSuffix(value, " Minutes")
			}
			if minutes, err := strconv.ParseFloat(value, 64); err == nil {
				status.RuntimeLeft = int(minutes * 60) // Convert minutes to seconds
			}
		case "NOMPOWER":
			// Parse nominal power (e.g., "800 Watts")
			if strings.HasSuffix(value, "Watts") {
				value = strings.TrimSuffix(value, " Watts")
			}
			if power, err := strconv.ParseFloat(value, 64); err == nil {
				status.NominalPower = power
			}
		case "LINEV":
			if strings.HasSuffix(value, "Volts") {
				value = strings.TrimSuffix(value, " Volts")
			}
			if _, err := strconv.ParseFloat(value, 64); err == nil {
				// status.InputVoltage (not in DTO) = volts
			}
		case "BATTV":
			if strings.HasSuffix(value, "Volts") {
				value = strings.TrimSuffix(value, " Volts")
			}
			if _, err := strconv.ParseFloat(value, 64); err == nil {
				// status.BatteryVoltage (not in DTO) = volts
			}
		case "MODEL":
			status.Model = value
		}
	}

	// Calculate actual power consumption from load percentage and nominal power
	if status.NominalPower > 0 && status.LoadPercent > 0 {
		status.PowerWatts = status.NominalPower * status.LoadPercent / 100.0
	}

	return status, nil
}

func (c *UPSCollector) collectNUT() (*dto.UPSStatus, error) {
	// First, get list of UPS devices
	output, err := lib.ExecCommandOutput("upsc", "-l")
	if err != nil {
		return nil, err
	}

	devices := strings.Split(strings.TrimSpace(output), "\n")
	if len(devices) == 0 || devices[0] == "" {
		return nil, fmt.Errorf("no UPS devices found")
	}

	// Use first device
	device := devices[0]

	// Get device status
	output, err = lib.ExecCommandOutput("upsc", device)
	if err != nil {
		return nil, err
	}

	status := &dto.UPSStatus{
		Connected: true,
		Timestamp: time.Now(),
	}

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		switch key {
		case "ups.status":
			status.Status = value
		case "ups.load":
			if load, err := strconv.ParseFloat(value, 64); err == nil {
				status.LoadPercent = load
			}
		case "battery.charge":
			if charge, err := strconv.ParseFloat(value, 64); err == nil {
				status.BatteryCharge = charge
			}
		case "battery.runtime":
			if seconds, err := strconv.ParseFloat(value, 64); err == nil {
				status.RuntimeLeft = int(seconds) // Already in seconds
			}
		case "ups.power.nominal", "ups.realpower.nominal":
			// Parse nominal power (usually in Watts)
			if power, err := strconv.ParseFloat(value, 64); err == nil {
				status.NominalPower = power
			}
		case "input.voltage":
			if _, err := strconv.ParseFloat(value, 64); err == nil {
				// status.InputVoltage (not in DTO) = volts
			}
		case "battery.voltage":
			if _, err := strconv.ParseFloat(value, 64); err == nil {
				// status.BatteryVoltage (not in DTO) = volts
			}
		case "device.model", "ups.model":
			status.Model = value
		}
	}

	// Calculate actual power consumption from load percentage and nominal power
	if status.NominalPower > 0 && status.LoadPercent > 0 {
		status.PowerWatts = status.NominalPower * status.LoadPercent / 100.0
	}

	return status, nil
}
