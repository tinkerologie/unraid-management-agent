package controllers

import (
	"sync"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/lib"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
)

// DefaultMinSpeedPercent is the minimum PWM percentage allowed. Setting below
// this risks stalling the fan entirely, which can cause thermal damage.
const DefaultMinSpeedPercent = 20

// DefaultCriticalTempC is the temperature at which all fans are forced to 100%.
const DefaultCriticalTempC = 90.0

// DefaultFailureRPMThreshold is the RPM below which a fan is considered failed.
const DefaultFailureRPMThreshold = 100

// fanOriginalState stores the original mode and PWM for restoration.
type fanOriginalState struct {
	Mode     dto.FanControlMode
	PWMValue int
}

// FanSafetyGuard monitors and enforces hardware protection limits.
type FanSafetyGuard struct {
	mu            sync.Mutex
	originals     map[string]fanOriginalState
	config        dto.FanSafetyConfig
	hwmon         *HwmonProvider
	stateCaptured bool
}

// NewFanSafetyGuard creates a safety guard with the given configuration.
func NewFanSafetyGuard(hwmon *HwmonProvider, config dto.FanSafetyConfig) *FanSafetyGuard {
	if config.MinSpeedPercent <= 0 {
		config.MinSpeedPercent = DefaultMinSpeedPercent
	}
	if config.CriticalTempC <= 0 {
		config.CriticalTempC = DefaultCriticalTempC
	}
	if config.FailureRPMThreshold <= 0 {
		config.FailureRPMThreshold = DefaultFailureRPMThreshold
	}

	return &FanSafetyGuard{
		originals: make(map[string]fanOriginalState),
		config:    config,
		hwmon:     hwmon,
	}
}

// CaptureState saves the current fan modes and PWM values so they can be
// restored on shutdown. Must be called before any control changes.
func (g *FanSafetyGuard) CaptureState(fans []dto.FanDevice) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.stateCaptured {
		return
	}

	for _, f := range fans {
		if f.Controllable {
			g.originals[f.ID] = fanOriginalState{
				Mode:     f.Mode,
				PWMValue: f.PWMValue,
			}
		}
	}
	g.stateCaptured = true
	logger.Info("Fan safety: Captured original state for %d controllable fans", len(g.originals))
}

// RestoreAll sets every fan back to its original mode and PWM.
// This MUST be called on shutdown to prevent fans from staying at a manual speed.
func (g *FanSafetyGuard) RestoreAll() {
	g.mu.Lock()
	defer g.mu.Unlock()

	if !g.stateCaptured {
		return
	}

	for fanID, orig := range g.originals {
		if err := g.hwmon.SetMode(fanID, orig.Mode); err != nil {
			logger.Error("Fan safety: Failed to restore mode for %s: %v", fanID, err)
		}
		if orig.Mode == dto.FanModeManual {
			if err := g.hwmon.SetPWM(fanID, orig.PWMValue); err != nil {
				logger.Error("Fan safety: Failed to restore PWM for %s: %v", fanID, err)
			}
		}
	}
	logger.Info("Fan safety: Restored original state for %d fans", len(g.originals))
}

// ValidatePWM enforces the minimum speed threshold. If the requested
// percent is below the minimum, it returns the minimum instead.
func (g *FanSafetyGuard) ValidatePWM(pct int) int {
	if pct < g.config.MinSpeedPercent {
		logger.Warning("Fan safety: Requested PWM %d%% below minimum %d%%, clamping", pct, g.config.MinSpeedPercent)
		return g.config.MinSpeedPercent
	}
	return pct
}

// CheckTemperatureSafety reads the system's highest temperature and returns
// true if it exceeds the critical threshold. When true, callers must force
// all fans to full speed immediately.
func (g *FanSafetyGuard) CheckTemperatureSafety() bool {
	maxTemp := g.readMaxTemperature()
	if maxTemp >= g.config.CriticalTempC {
		logger.Error("Fan safety: Critical temperature %.1f°C >= %.1f°C — forcing full speed", maxTemp, g.config.CriticalTempC)
		return true
	}
	return false
}

// DetectFailures checks fans for stall conditions (low RPM while PWM > 0).
func (g *FanSafetyGuard) DetectFailures(fans []dto.FanDevice) []string {
	var failed []string
	for _, f := range fans {
		if f.Controllable && f.PWMPercent > 10 && f.RPM < g.config.FailureRPMThreshold {
			failed = append(failed, f.ID)
			logger.Warning("Fan safety: %s appears stalled (RPM=%d, PWM=%d%%)", f.ID, f.RPM, f.PWMPercent)
		}
	}
	return failed
}

// EmergencyFullSpeed forces all controllable fans to 100% via hwmon.
func (g *FanSafetyGuard) EmergencyFullSpeed() {
	fans := g.hwmon.ReadAll()
	for _, f := range fans {
		if f.Controllable {
			if err := g.hwmon.SetMode(f.ID, dto.FanModeManual); err != nil {
				logger.Error("Fan safety: Emergency mode set failed for %s: %v", f.ID, err)
			}
			if err := g.hwmon.SetPWM(f.ID, 255); err != nil {
				logger.Error("Fan safety: Emergency PWM set failed for %s: %v", f.ID, err)
			}
		}
	}
	logger.Warning("Fan safety: All fans forced to 100%%")
}

// readMaxTemperature scans hwmon temp*_input files for the highest reading.
func (g *FanSafetyGuard) readMaxTemperature() float64 {
	return lib.ReadMaxHwmonTemp()
}

// Config returns the current safety configuration.
func (g *FanSafetyGuard) Config() dto.FanSafetyConfig {
	return g.config
}
