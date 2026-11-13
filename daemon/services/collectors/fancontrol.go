// Package collectors provides data collection services for system metrics.
package collectors

import (
	"time"

	"github.com/tinkerologie/unraid-management-agent/daemon/constants"
	"github.com/tinkerologie/unraid-management-agent/daemon/domain"
	"github.com/tinkerologie/unraid-management-agent/daemon/dto"
	"github.com/tinkerologie/unraid-management-agent/daemon/lib"
	"github.com/tinkerologie/unraid-management-agent/daemon/logger"
)

// FanControlCollector periodically reads fan status and publishes it to the event bus.
type FanControlCollector struct {
	ctx *domain.Context
	// hwmonBasePath allows fan discovery against a fixture ("" = lib.HwmonBasePath).
	hwmonBasePath string
}

// NewFanControlCollector creates a new fan control collector.
func NewFanControlCollector(ctx *domain.Context) *FanControlCollector {
	return &FanControlCollector{ctx: ctx}
}

// Collect reads the current fan status and publishes it to the event bus.
func (c *FanControlCollector) Collect() {
	logger.Debug("Collecting fan control data...")
	domain.Publish(c.ctx.Hub, constants.TopicFanControlUpdate, c.buildStatus())
}

// buildStatus reads the current fan status, including whether a third-party fan
// controller is active so consumers can see when the agent is deferring.
func (c *FanControlCollector) buildStatus() *dto.FanControlStatus {
	basePath := c.hwmonBasePath
	if basePath == "" {
		basePath = lib.HwmonBasePath
	}
	hwmonFans := lib.DiscoverHwmonFansAt(basePath)

	fans := make([]dto.FanDevice, 0, len(hwmonFans))
	controllable := 0
	for _, hf := range hwmonFans {
		fan := dto.FanDevice{
			ID:         hf.ID,
			Name:       hf.Name,
			RPM:        hf.RPM,
			HwmonPath:  hf.HwmonDir,
			HwmonIndex: hf.FanIndex,
			Role:       hf.Role,
			PCIID:      hf.PCIID,
		}

		if hf.HasPWM {
			fan.Controllable = true
			fan.PWMValue = hf.PWMValue
			fan.PWMPercent = hf.PWMPercent
			fan.Mode = hwmonEnableToMode(hf.Mode)
			controllable++
		} else {
			fan.Mode = dto.FanModeAutomatic
		}

		fans = append(fans, fan)
	}

	ext := lib.DetectExternalFanControl()
	return &dto.FanControlStatus{
		Fans: fans,
		Config: dto.FanControlConfig{
			Enabled:       true,
			ControlMethod: dto.FanMethodHwmon,
		},
		Summary: dto.FanControlSummary{
			TotalFans:        len(fans),
			ControllableFans: controllable,
		},
		ExternalControl: &ext,
		Timestamp:       time.Now(),
	}
}

// hwmonEnableToMode converts a pwm_enable sysfs value to a FanControlMode.
func hwmonEnableToMode(val int) dto.FanControlMode {
	switch val {
	case 0:
		return dto.FanModeOff
	case 1:
		return dto.FanModeManual
	default:
		return dto.FanModeAutomatic
	}
}
