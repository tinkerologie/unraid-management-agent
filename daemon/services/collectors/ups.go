package collectors

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/tinkerologie/unraid-management-agent/daemon/constants"
	"github.com/tinkerologie/unraid-management-agent/daemon/domain"
	"github.com/tinkerologie/unraid-management-agent/daemon/dto"
	"github.com/tinkerologie/unraid-management-agent/daemon/lib"
	"github.com/tinkerologie/unraid-management-agent/daemon/logger"
)

// UPSCollector collects UPS (Uninterruptible Power Supply) status information.
// It supports both apcupsd and NUT (Network UPS Tools) monitoring systems.
type UPSCollector struct {
	ctx *domain.Context
	// execOutput runs apcaccess and upsc. Nil means lib.ExecCommandOutput;
	// tests inject a fake.
	execOutput func(command string, args ...string) (string, error)
}

// NewUPSCollector creates a new UPS status collector with the given context.
func NewUPSCollector(ctx *domain.Context) *UPSCollector {
	return &UPSCollector{ctx: ctx}
}

// Start begins the UPS collector's periodic data collection.
// It runs in a goroutine and publishes UPS status updates at the specified interval until the context is cancelled.
func (c *UPSCollector) Start(ctx context.Context, interval time.Duration) {
	logger.Info("Starting ups collector (interval: %v)", interval)

	runCollectSafely := func(phase string) {
		defer func() {
			if r := recover(); r != nil {
				logger.LogPanicWithStack("UPS collector ("+phase+")", r)
			}
		}()
		collectWithWatchdog(ctx, "UPS", interval, c.Collect)
	}

	runCollectSafely("startup")

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info("UPS collector stopping due to context cancellation")
			return
		case <-ticker.C:
			runCollectSafely("periodic collection")
		}
	}
}

// Collect gathers UPS status information and publishes it to the event bus.
// It attempts to collect data from apcupsd first, then falls back to NUT if apcupsd is not available.
func (c *UPSCollector) Collect() {

	logger.Debug("Collecting ups data...")

	// Try apcaccess first (APC UPS)
	var upsData *dto.UPSStatus
	var err error

	if lib.CommandExists("apcaccess") {
		upsData, err = c.collectAPC()
		if err == nil {
			domain.Publish(c.ctx.Hub, constants.TopicUPSStatusUpdate, upsData)
			logger.Debug("Published %s event (APC)", constants.TopicUPSStatusUpdate.Name)
			return
		}
		logger.Debug("apcaccess failed, falling back to NUT: %v", err)
	}

	// Fallback to upsc (NUT - Network UPS Tools)
	if lib.CommandExists("upsc") {
		upsData, err = c.collectNUT()
		if err == nil {
			domain.Publish(c.ctx.Hub, constants.TopicUPSStatusUpdate, upsData)
			logger.Debug("Published %s event (NUT)", constants.TopicUPSStatusUpdate.Name)
			return
		}
		logger.Debug("Failed to collect NUT UPS data: %v", err)
	}

	// No UPS available
	logger.Debug("No UPS detected or configured")
}

// run executes a command through execOutput, defaulting to lib.ExecCommandOutput.
func (c *UPSCollector) run(command string, args ...string) (string, error) {
	if c.execOutput != nil {
		return c.execOutput(command, args...)
	}
	return lib.ExecCommandOutput(command, args...)
}

func (c *UPSCollector) collectAPC() (*dto.UPSStatus, error) {
	output, err := c.run("apcaccess")
	if err != nil {
		return nil, err
	}
	return parseAPCOutput(output)
}

// parseAPCOutput parses the stdout of apcaccess into a UPSStatus DTO.
func parseAPCOutput(output string) (*dto.UPSStatus, error) {
	status := &dto.UPSStatus{
		Connected: true,
		Timestamp: time.Now(),
	}

	lines := strings.SplitSeq(output, "\n")
	for line := range lines {
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
			status.LoadPercent = parseOptionalFloat(value)
		case "BCHARGE":
			if strings.HasSuffix(value, "Percent") {
				value = strings.TrimSuffix(value, " Percent")
			}
			status.BatteryCharge = parseOptionalFloat(value)
		case "TIMELEFT":
			if strings.HasSuffix(value, "Minutes") {
				value = strings.TrimSuffix(value, " Minutes")
			}
			if minutes := parseOptionalFloat(value); minutes != nil {
				status.RuntimeLeft = optionalInt(*minutes * 60) // Convert minutes to seconds
			}
		case "NOMPOWER":
			// Parse nominal power (e.g., "800 Watts")
			if strings.HasSuffix(value, "Watts") {
				value = strings.TrimSuffix(value, " Watts")
			}
			status.NominalPower = parseOptionalFloat(value)
		case "LINEV":
			status.InputVoltage = parseOptionalFloat(strings.TrimSuffix(value, " Volts"))
		case "OUTPUTV":
			status.OutputVoltage = parseOptionalFloat(strings.TrimSuffix(value, " Volts"))
		case "BATTV":
			status.BatteryVoltage = parseOptionalFloat(strings.TrimSuffix(value, " Volts"))
		case "LINEFREQ":
			status.LineFrequency = parseOptionalFloat(strings.TrimSuffix(value, " Hz"))
		case "ITEMP":
			// Celsius only: another unit leaves the suffix, so the reading stays null.
			status.Temperature = parseOptionalFloat(strings.TrimSuffix(value, " C"))
		case "TONBATT":
			status.TimeOnBattery = parseOptionalInt(strings.TrimSuffix(value, " Seconds"))
		case "CUMONBATT":
			status.CumulativeTimeOnBattery = parseOptionalInt(strings.TrimSuffix(value, " Seconds"))
		case "NUMXFERS":
			status.TransferCount = parseOptionalInt(value)
		case "SELFTEST":
			status.SelfTest = value
		case "SERIALNO":
			status.SerialNumber = value
		case "MODEL":
			status.Model = value
		}
	}

	// apcupsd has no real power reading; estimate it from nominal power and
	// load, but only when the UPS reports both.
	status.PowerWatts = derivePower(status.NominalPower, status.LoadPercent)

	return status, nil
}

func (c *UPSCollector) collectNUT() (*dto.UPSStatus, error) {
	// First, get list of UPS devices (try localhost first, then without host)
	output, err := c.run("upsc", "-l", "localhost")
	if err != nil {
		output, err = c.run("upsc", "-l")
		if err != nil {
			return nil, err
		}
	}

	devices := strings.Split(strings.TrimSpace(output), "\n")
	if len(devices) == 0 || devices[0] == "" {
		return nil, fmt.Errorf("no UPS devices found")
	}

	// Use first device with @localhost suffix for NUT protocol
	device := devices[0] + "@localhost"

	// Get device status
	output, err = c.run("upsc", device)
	if err != nil {
		return nil, err
	}

	status, err := parseNUTUpscOutput(output)
	if status != nil {
		// Say which NUT device this is; GET /nut lists every device.
		status.DeviceName = devices[0]
	}
	return status, err
}

// parseNUTUpscOutput parses the stdout of upsc into a UPSStatus DTO.
func parseNUTUpscOutput(output string) (*dto.UPSStatus, error) {
	status := &dto.UPSStatus{
		Connected: true,
		Timestamp: time.Now(),
	}

	lines := strings.SplitSeq(output, "\n")
	for line := range lines {
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
			status.LoadPercent = parseOptionalFloat(value)
		case "battery.charge":
			status.BatteryCharge = parseOptionalFloat(value)
		case "battery.runtime":
			status.RuntimeLeft = parseOptionalInt(value) // Already in seconds
		case "ups.realpower":
			status.PowerWatts = parseOptionalFloat(value)
		case "ups.realpower.nominal":
			// Nominal real power in watts. ups.power.nominal is in VA, so it
			// is not used: VA × load would overstate the power draw.
			status.NominalPower = parseOptionalFloat(value)
		case "input.voltage":
			status.InputVoltage = parseOptionalFloat(value)
		case "output.voltage":
			status.OutputVoltage = parseOptionalFloat(value)
		case "battery.voltage":
			status.BatteryVoltage = parseOptionalFloat(value)
		case "input.frequency":
			status.LineFrequency = parseOptionalFloat(value)
		case "ups.temperature":
			status.Temperature = parseOptionalFloat(value)
		case "battery.runtime.elapsed":
			status.TimeOnBattery = parseOptionalInt(value)
		case "input.transfers":
			// Not a standard NUT variable; only some drivers report it.
			status.TransferCount = parseOptionalInt(value)
		case "ups.test.result":
			status.SelfTest = value
		case "device.serial", "ups.serial":
			// Some drivers report the placeholder "unknown"; leave it out.
			if !strings.EqualFold(value, "unknown") {
				status.SerialNumber = value
			}
		case "device.model", "ups.model":
			status.Model = value
		}
	}

	// Prefer the UPS's own ups.realpower; otherwise estimate it from nominal
	// power and load, but only when the UPS reports both.
	if status.PowerWatts == nil {
		status.PowerWatts = derivePower(status.NominalPower, status.LoadPercent)
	}

	return status, nil
}

// parseOptionalFloat parses a UPS reading. It returns nil when the value is not
// a finite number, so a reading that is missing or garbled stays unknown
// instead of becoming 0.
func parseOptionalFloat(value string) *float64 {
	v, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return nil
	}
	return &v
}

// parseOptionalInt parses a whole-number UPS reading such as a runtime or delay
// in seconds (NUT may print "623" or "623.0"). It returns nil when the value is
// not a finite number.
func parseOptionalInt(value string) *int {
	v := parseOptionalFloat(value)
	if v == nil {
		return nil
	}
	return optionalInt(*v)
}

// optionalInt converts a finite reading to an int, or nil when it is outside
// the int range (Go's conversion of such a value is implementation-defined).
func optionalInt(v float64) *int {
	if v < math.MinInt64 || v >= math.MaxInt64 {
		return nil
	}
	return new(int(v))
}

// derivePower estimates power as nominal × load%. It returns nil unless both
// inputs are known and the nominal rating is positive.
func derivePower(nominal, loadPercent *float64) *float64 {
	if nominal == nil || loadPercent == nil || *nominal <= 0 {
		return nil
	}
	return new(*nominal * *loadPercent / 100.0)
}
