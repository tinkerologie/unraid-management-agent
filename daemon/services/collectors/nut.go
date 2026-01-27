package collectors

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/constants"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/lib"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
)

// NUTCollector collects NUT (Network UPS Tools) status information.
// This collector provides detailed UPS data when the NUT plugin is installed.
type NUTCollector struct {
	ctx *domain.Context
}

// NewNUTCollector creates a new NUT status collector with the given context.
func NewNUTCollector(ctx *domain.Context) *NUTCollector {
	return &NUTCollector{ctx: ctx}
}

// Start begins the NUT collector's periodic data collection.
func (c *NUTCollector) Start(ctx context.Context, interval time.Duration) {
	logger.Info("Starting NUT collector (interval: %v)", interval)

	// Run once immediately
	func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("NUT collector PANIC on startup: %v", r)
			}
		}()
		c.Collect()
	}()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info("NUT collector stopping due to context cancellation")
			return
		case <-ticker.C:
			func() {
				defer func() {
					if r := recover(); r != nil {
						logger.Error("NUT collector PANIC in loop: %v", r)
					}
				}()
				c.Collect()
			}()
		}
	}
}

// Collect gathers NUT status information and publishes it to the event bus.
func (c *NUTCollector) Collect() {
	logger.Debug("Collecting NUT data...")

	response := &dto.NUTResponse{
		Timestamp: time.Now(),
	}

	// Check if NUT plugin is installed
	if _, err := os.Stat(constants.NutPluginDir); os.IsNotExist(err) {
		response.Installed = false
		c.ctx.Hub.Pub(response, "nut_status_update")
		logger.Debug("NUT plugin not installed")
		return
	}
	response.Installed = true

	// Load NUT configuration
	config, err := c.loadNUTConfig()
	if err != nil {
		logger.Warning("Failed to load NUT config: %v", err)
	} else {
		response.Config = config
	}

	// Check if NUT service is running
	response.Running = c.isNUTRunning()

	if !response.Running {
		c.ctx.Hub.Pub(response, "nut_status_update")
		logger.Debug("NUT service not running")
		return
	}

	// Get list of UPS devices
	devices, err := c.listDevices()
	if err != nil {
		logger.Warning("Failed to list NUT devices: %v", err)
	} else {
		response.Devices = devices
	}

	// Get detailed status for the first available device
	if len(devices) > 0 {
		status, err := c.collectStatus(devices[0].Name, c.getHostFromConfig(config))
		if err != nil {
			logger.Warning("Failed to collect NUT status: %v", err)
		} else {
			response.Status = status
		}
	}

	c.ctx.Hub.Pub(response, "nut_status_update")
	logger.Debug("Published nut_status_update event")
}

// loadNUTConfig reads the NUT plugin configuration file
func (c *NUTCollector) loadNUTConfig() (*dto.NUTConfig, error) {
	file, err := os.Open(constants.NutPluginCfg)
	if err != nil {
		return nil, err
	}
	defer file.Close() //nolint:errcheck // Error checking not needed for defer Close

	config := &dto.NUTConfig{}
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.Trim(strings.TrimSpace(parts[1]), "\"")

		switch key {
		case "SERVICE":
			config.ServiceEnabled = value == "enable"
		case "MODE":
			config.Mode = value
		case "NAME":
			config.UPSName = value
		case "DRIVER":
			config.Driver = value
		case "PORT":
			config.Port = value
		case "IPADDR":
			config.IPAddress = value
		case "POLL":
			if poll, err := strconv.Atoi(value); err == nil {
				config.PollInterval = poll
			}
		case "SHUTDOWN":
			config.ShutdownMode = value
		case "BATTERYLEVEL":
			if level, err := strconv.Atoi(value); err == nil {
				config.BatteryLevel = level
			}
		case "RTVALUE":
			if rt, err := strconv.Atoi(value); err == nil {
				config.RuntimeValue = rt
			}
		case "TIMEOUT":
			if timeout, err := strconv.Atoi(value); err == nil {
				config.Timeout = timeout
			}
		}
	}

	return config, scanner.Err()
}

// isNUTRunning checks if the NUT service is running
func (c *NUTCollector) isNUTRunning() bool {
	// Check for PID file
	if _, err := os.Stat(constants.NutPidFile); err == nil {
		return true
	}

	// Also check if upsd process is running
	output, err := lib.ExecCommandOutput("pgrep", "-x", "upsd")
	if err == nil && strings.TrimSpace(output) != "" {
		return true
	}

	return false
}

// listDevices returns a list of available NUT UPS devices
func (c *NUTCollector) listDevices() ([]dto.NUTDevice, error) {
	if !lib.CommandExists("upsc") {
		return nil, fmt.Errorf("upsc command not found")
	}

	output, err := lib.ExecCommandOutput("upsc", "-l", "localhost")
	if err != nil {
		// Try without host
		output, err = lib.ExecCommandOutput("upsc", "-l")
		if err != nil {
			return nil, err
		}
	}

	var devices []dto.NUTDevice
	lines := strings.Split(strings.TrimSpace(output), "\n")
	for _, line := range lines {
		name := strings.TrimSpace(line)
		if name == "" {
			continue
		}
		devices = append(devices, dto.NUTDevice{
			Name:        name,
			Description: fmt.Sprintf("UPS device: %s", name),
			Available:   true,
		})
	}

	return devices, nil
}

// getHostFromConfig returns the host from NUT config, defaulting to localhost
func (c *NUTCollector) getHostFromConfig(config *dto.NUTConfig) string {
	if config != nil && config.IPAddress != "" && config.IPAddress != "127.0.0.1" {
		return config.IPAddress
	}
	return "localhost"
}

// collectStatus collects detailed status for a specific UPS device
func (c *NUTCollector) collectStatus(deviceName, host string) (*dto.NUTStatus, error) {
	target := fmt.Sprintf("%s@%s", deviceName, host)
	output, err := lib.ExecCommandOutput("upsc", target)
	if err != nil {
		return nil, fmt.Errorf("failed to query UPS %s: %w", target, err)
	}

	status := &dto.NUTStatus{
		Connected:    true,
		DeviceName:   deviceName,
		Host:         host,
		RawVariables: make(map[string]string),
		Timestamp:    time.Now(),
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

		// Store in raw variables
		status.RawVariables[key] = value

		// Parse specific fields
		switch key {
		// Driver info
		case "driver.name":
			status.Driver = value
		case "driver.state":
			status.DriverState = value
		case "driver.version":
			status.DriverVersion = value
		case "driver.version.data":
			status.DriverVersionData = value
		case "driver.version.usb":
			status.DriverVersionUSB = value

		// Device identification
		case "device.mfr", "ups.mfr":
			status.Manufacturer = value
		case "device.model", "ups.model":
			status.Model = value
		case "device.serial", "ups.serial":
			status.Serial = value
		case "device.type":
			status.Type = value
		case "ups.productid":
			status.ProductID = value
		case "ups.vendorid":
			status.VendorID = value

		// UPS status
		case "ups.status":
			status.Status = value
			status.StatusText = dto.NUTStatusText(value)
		case "ups.beeper.status":
			status.BeeperStatus = value
		case "ups.test.result":
			status.TestResult = value

		// Battery info
		case "battery.charge":
			if v, err := strconv.ParseFloat(value, 64); err == nil {
				status.BatteryCharge = v
			}
		case "battery.charge.low":
			if v, err := strconv.ParseFloat(value, 64); err == nil {
				status.BatteryChargeLow = v
			}
		case "battery.charge.warning":
			if v, err := strconv.ParseFloat(value, 64); err == nil {
				status.BatteryChargeWarning = v
			}
		case "battery.runtime":
			if v, err := strconv.ParseFloat(value, 64); err == nil {
				status.BatteryRuntime = int(v)
			}
		case "battery.runtime.low":
			if v, err := strconv.ParseFloat(value, 64); err == nil {
				status.BatteryRuntimeLow = int(v)
			}
		case "battery.voltage":
			if v, err := strconv.ParseFloat(value, 64); err == nil {
				status.BatteryVoltage = v
			}
		case "battery.voltage.nominal":
			if v, err := strconv.ParseFloat(value, 64); err == nil {
				status.BatteryVoltageNominal = v
			}
		case "battery.type":
			status.BatteryType = value
		case "battery.status":
			status.BatteryStatus = value
		case "battery.mfr.date":
			status.BatteryMfrDate = value

		// Input power
		case "input.voltage":
			if v, err := strconv.ParseFloat(value, 64); err == nil {
				status.InputVoltage = v
			}
		case "input.voltage.nominal":
			if v, err := strconv.ParseFloat(value, 64); err == nil {
				status.InputVoltageNominal = v
			}
		case "input.frequency":
			if v, err := strconv.ParseFloat(value, 64); err == nil {
				status.InputFrequency = v
			}
		case "input.transfer.high":
			if v, err := strconv.ParseFloat(value, 64); err == nil {
				status.InputTransferHigh = v
			}
		case "input.transfer.low":
			if v, err := strconv.ParseFloat(value, 64); err == nil {
				status.InputTransferLow = v
			}
		case "input.current":
			if v, err := strconv.ParseFloat(value, 64); err == nil {
				status.InputCurrent = v
			}

		// Output power
		case "output.voltage":
			if v, err := strconv.ParseFloat(value, 64); err == nil {
				status.OutputVoltage = v
			}
		case "output.frequency":
			if v, err := strconv.ParseFloat(value, 64); err == nil {
				status.OutputFrequency = v
			}
		case "output.current":
			if v, err := strconv.ParseFloat(value, 64); err == nil {
				status.OutputCurrent = v
			}

		// Load and power
		case "ups.load":
			if v, err := strconv.ParseFloat(value, 64); err == nil {
				status.LoadPercent = v
			}
		case "ups.realpower":
			if v, err := strconv.ParseFloat(value, 64); err == nil {
				status.RealPower = v
			}
		case "ups.realpower.nominal":
			if v, err := strconv.ParseFloat(value, 64); err == nil {
				status.RealPowerNominal = v
			}
		case "ups.power":
			if v, err := strconv.ParseFloat(value, 64); err == nil {
				status.ApparentPower = v
			}
		case "ups.power.nominal":
			if v, err := strconv.ParseFloat(value, 64); err == nil {
				status.ApparentPowerNominal = v
			}

		// Timing
		case "ups.delay.shutdown":
			if v, err := strconv.Atoi(value); err == nil {
				status.DelayShutdown = v
			}
		case "ups.delay.start":
			if v, err := strconv.Atoi(value); err == nil {
				status.DelayStart = v
			}
		case "ups.timer.shutdown":
			if v, err := strconv.Atoi(value); err == nil {
				status.TimerShutdown = v
			}
		case "ups.timer.start":
			if v, err := strconv.Atoi(value); err == nil {
				status.TimerStart = v
			}
		}
	}

	// Calculate real power if not directly available
	if status.RealPower == 0 && status.RealPowerNominal > 0 && status.LoadPercent > 0 {
		status.RealPower = status.RealPowerNominal * status.LoadPercent / 100.0
	}

	// Calculate apparent power if not directly available
	if status.ApparentPower == 0 && status.ApparentPowerNominal > 0 && status.LoadPercent > 0 {
		status.ApparentPower = status.ApparentPowerNominal * status.LoadPercent / 100.0
	}

	return status, nil
}
