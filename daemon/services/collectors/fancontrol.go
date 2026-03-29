// Package collectors provides data collection services for system metrics.
package collectors

import (
	"context"
	"time"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/constants"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/lib"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
)

// FanControlCollector periodically reads fan status and publishes it to the event bus.
type FanControlCollector struct {
	ctx *domain.Context
}

// NewFanControlCollector creates a new fan control collector.
func NewFanControlCollector(ctx *domain.Context) *FanControlCollector {
	return &FanControlCollector{ctx: ctx}
}

// Start begins the periodic fan status collection loop with panic recovery.
func (c *FanControlCollector) Start(ctx context.Context, interval time.Duration) {
	func() {
		defer func() {
			if r := recover(); r != nil {
				logger.LogPanicWithStack("Fan control collector", r)
			}
		}()
		c.Collect()
	}()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			func() {
				defer func() {
					if r := recover(); r != nil {
						logger.LogPanicWithStack("Fan control collector", r)
					}
				}()
				c.Collect()
			}()
		}
	}
}

// Collect reads the current fan status and publishes it to the event bus.
func (c *FanControlCollector) Collect() {
	logger.Debug("Collecting fan control data...")

	hwmonFans := lib.DiscoverHwmonFans()

	fans := make([]dto.FanDevice, 0, len(hwmonFans))
	controllable := 0
	for _, hf := range hwmonFans {
		fan := dto.FanDevice{
			ID:         hf.ID,
			Name:       hf.Name,
			RPM:        hf.RPM,
			HwmonPath:  hf.HwmonDir,
			HwmonIndex: hf.FanIndex,
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

	status := &dto.FanControlStatus{
		Fans: fans,
		Config: dto.FanControlConfig{
			Enabled:       true,
			ControlMethod: dto.FanMethodHwmon,
		},
		Summary: dto.FanControlSummary{
			TotalFans:        len(fans),
			ControllableFans: controllable,
		},
		Timestamp: time.Now(),
	}

	domain.Publish(c.ctx.Hub, constants.TopicFanControlUpdate, status)
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
