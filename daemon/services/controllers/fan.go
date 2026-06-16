package controllers

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/lib"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
)

// FanController orchestrates fan monitoring and control across providers.
type FanController struct {
	mu          sync.RWMutex
	hwmon       *HwmonProvider
	ipmi        *IPMIProvider
	safety      *FanSafetyGuard
	curves      *FanCurveEngine
	configStore *FanConfigStore
	config      dto.FanControlConfig
	initialized bool

	// detectExternal reports whether a third-party fan-control plugin is active.
	// It is evaluated live (not cached) so a plugin enabled after startup is
	// honoured without a daemon restart. Injectable for tests.
	detectExternal func() dto.ExternalFanControl
}

// NewFanController creates a new fan controller. Call Initialize() to discover hardware.
func NewFanController() *FanController {
	return &FanController{
		hwmon:          NewHwmonProvider(),
		ipmi:           NewIPMIProvider(),
		configStore:    NewFanConfigStore(""),
		detectExternal: lib.DetectExternalFanControl,
	}
}

// externalStatus returns the current third-party fan-control status, evaluated
// live. Safe to call on a controller without an injected detector (returns the
// inactive zero value).
func (c *FanController) externalStatus() dto.ExternalFanControl {
	if c.detectExternal == nil {
		return dto.ExternalFanControl{}
	}
	return c.detectExternal()
}

// Initialize discovers hardware, loads config, and sets up safety guards.
func (c *FanController) Initialize() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Load persistent config
	cfgData, err := c.configStore.Load()
	if err != nil {
		logger.Warning("Fan control: Failed to load config, using defaults: %v", err)
		c.config = defaultFanControlConfig()
	} else {
		c.config = cfgData.Config
	}

	// Discover hardware
	c.hwmon.Discover()

	// Detect third-party fan-control plugins. When one is active the agent
	// stays monitor-only and refuses to modify fan speeds so it does not fight
	// the other controller. Detection is re-evaluated live on each write/status,
	// this is just a startup log of the current state.
	if ext := c.externalStatus(); ext.Active {
		logger.Info("Fan control: Detected active third-party fan control (%s); staying monitor-only and will not modify fan speeds",
			strings.Join(ext.Controllers, ", "))
	}

	// Check IPMI availability
	if c.config.ControlMethod == dto.FanMethodIPMI {
		if !c.ipmi.IsAvailable() {
			logger.Warning("Fan control: IPMI requested but not available, falling back to hwmon")
			c.config.ControlMethod = dto.FanMethodHwmon
		}
	}

	// Set up safety guard
	c.safety = NewFanSafetyGuard(c.hwmon, c.config.Safety)

	// Capture initial fan state for restoration on shutdown
	fans := c.hwmon.ReadAll()
	c.safety.CaptureState(fans)

	// Set up curve engine
	c.curves = NewFanCurveEngine(c.hwmon, c.safety)

	// Restore saved custom profiles
	for _, profile := range cfgData.Profiles {
		if !profile.BuiltIn {
			if addErr := c.curves.AddProfile(profile); addErr != nil {
				logger.Warning("Fan control: Failed to restore profile %q: %v", profile.Name, addErr)
			}
		}
	}

	// Restore saved assignments
	for fanID, assignment := range cfgData.Assignments {
		if assignErr := c.curves.AssignProfile(fanID, assignment.ProfileName, assignment.Source); assignErr != nil {
			logger.Warning("Fan control: Failed to restore assignment for %s: %v", fanID, assignErr)
		}
	}

	c.initialized = true
	logger.Info("Fan control: Initialized with %d fans, control_enabled=%v, method=%s",
		len(fans), c.config.ControlEnabled, c.config.ControlMethod)
	return nil
}

// GetStatus returns the complete fan control status for the collector.
func (c *FanController) GetStatus() *dto.FanControlStatus {
	c.mu.RLock()
	defer c.mu.RUnlock()

	ext := c.externalStatus()

	if !c.initialized {
		return &dto.FanControlStatus{
			Fans:            []dto.FanDevice{},
			Profiles:        builtInProfiles(),
			Config:          c.config,
			ExternalControl: &ext,
			Timestamp:       time.Now(),
		}
	}

	// Read current fan state
	fans := c.hwmon.ReadAll()

	// If IPMI is active, merge those fans
	if c.config.ControlMethod == dto.FanMethodIPMI && c.ipmi.available {
		ipmiFans := c.ipmi.ReadAll()
		fans = mergeFanDevices(fans, ipmiFans)
	}

	// Annotate fans with profile assignments
	for i := range fans {
		src, ok := c.curves.GetAssignmentSource(fans[i].ID)
		if !ok {
			continue
		}
		if profileName, pok := c.curves.GetAssignment(fans[i].ID); pok {
			fans[i].ActiveProfile = profileName
		}
		s := src
		fans[i].TempSource = &s
		if src.Type == dto.FanTempSourceHwmon {
			fans[i].TempSensorPath = src.SensorPath
		}
	}

	// Check temperature safety. When deferring to a third-party controller the
	// agent never forces full speed itself (that controller owns thermal
	// response); CheckTemperatureSafety still logs the critical-temp warning.
	if c.config.ControlEnabled && c.safety.CheckTemperatureSafety() && !ext.Active {
		c.safety.EmergencyFullSpeed()
	}

	// Detect fan failures
	failedFans := c.safety.DetectFailures(fans)

	// Build summary
	controllable := 0
	for _, f := range fans {
		if f.Controllable {
			controllable++
		}
	}

	return &dto.FanControlStatus{
		Fans:     fans,
		Profiles: c.curves.Profiles(),
		Config:   c.config,
		Summary: dto.FanControlSummary{
			TotalFans:        len(fans),
			ControllableFans: controllable,
			FailedFans:       failedFans,
		},
		ExternalControl: &ext,
		Timestamp:       time.Now(),
	}
}

// deferralError returns a non-nil error when a third-party fan controller is
// active, signalling that the agent must not modify fan speeds. Returns nil
// when no external controller is active.
func (c *FanController) deferralError() error {
	if ext := c.externalStatus(); ext.Active {
		return fmt.Errorf("fan control deferred to active plugin: %s", strings.Join(ext.Controllers, ", "))
	}
	return nil
}

// SetSpeed sets a fan's PWM duty cycle by percentage (0-100).
// Requires control_enabled to be true and the fan to be in manual mode.
func (c *FanController) SetSpeed(fanID string, pct int) error {
	if err := lib.ValidateFanID(fanID); err != nil {
		return err
	}
	if err := lib.ValidatePWMPercent(pct); err != nil {
		return err
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	if err := c.deferralError(); err != nil {
		return err
	}
	if !c.config.ControlEnabled {
		return fmt.Errorf("fan control is not enabled; enable it via the configuration endpoint first")
	}

	// Apply safety minimum
	pct = c.safety.ValidatePWM(pct)

	pwm := lib.PctToPWM(pct)
	if err := c.hwmon.SetPWM(fanID, pwm); err != nil {
		return fmt.Errorf("set PWM for %s: %w", fanID, err)
	}

	logger.Info("Fan control: Set %s speed to %d%% (PWM %d)", fanID, pct, pwm)
	return nil
}

// SetMode switches a fan between automatic and manual control.
func (c *FanController) SetMode(fanID string, mode string) error {
	if err := lib.ValidateFanID(fanID); err != nil {
		return err
	}
	if err := lib.ValidateFanControlMode(mode); err != nil {
		return err
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	if err := c.deferralError(); err != nil {
		return err
	}
	if !c.config.ControlEnabled {
		return fmt.Errorf("fan control is not enabled; enable it via the configuration endpoint first")
	}

	fanMode := dto.FanControlMode(mode)
	if err := c.hwmon.SetMode(fanID, fanMode); err != nil {
		return fmt.Errorf("set mode for %s: %w", fanID, err)
	}

	logger.Info("Fan control: Set %s mode to %s", fanID, mode)
	return nil
}

// SetProfile assigns a fan curve profile to a fan using the given temperature source.
func (c *FanController) SetProfile(fanID, profileName string, source dto.FanTempSource) error {
	if err := lib.ValidateFanID(fanID); err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.deferralError(); err != nil {
		return err
	}
	if !c.config.ControlEnabled {
		return fmt.Errorf("fan control is not enabled; enable it via the configuration endpoint first")
	}

	if err := c.hwmon.SetMode(fanID, dto.FanModeManual); err != nil {
		return fmt.Errorf("set manual mode for profile: %w", err)
	}
	if err := c.curves.AssignProfile(fanID, profileName, source); err != nil {
		return fmt.Errorf("assign profile: %w", err)
	}
	if !c.curves.running {
		c.curves.Start(time.Duration(c.config.PollInterval) * time.Second)
	}
	c.saveConfigLocked()
	logger.Info("Fan control: Assigned profile %q to %s (source=%s)", profileName, fanID, source.Type)
	return nil
}

// GetSensorCatalog returns the hwmon sensors and drives available as fan-curve
// temperature sources.
func (c *FanController) GetSensorCatalog() dto.FanSensorCatalog {
	cat := dto.FanSensorCatalog{
		Timestamp:    time.Now(),
		HwmonSensors: []dto.AvailableTempSensor{},
		Drives:       []dto.AvailableDriveSensor{},
	}
	for _, s := range lib.DiscoverHwmonTempSensors() {
		cat.HwmonSensors = append(cat.HwmonSensors, dto.AvailableTempSensor{
			Path: s.Path, Label: s.Label, TempC: s.TempC, Plausible: s.Plausible,
		})
	}
	if drives, err := lib.ReadDiskTemps(); err == nil {
		for _, d := range drives {
			cat.Drives = append(cat.Drives, dto.AvailableDriveSensor{
				ID: d.ID, Device: d.Device, TempC: d.TempC, SpunDown: d.SpunDown,
			})
		}
	} else {
		logger.Debug("Fan control: drive temperatures unavailable for sensor catalog: %v", err)
	}
	return cat
}

// CreateProfile registers a custom fan profile.
func (c *FanController) CreateProfile(profile dto.FanProfile) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	profile.BuiltIn = false
	if err := c.curves.AddProfile(profile); err != nil {
		return err
	}

	c.saveConfigLocked()
	logger.Info("Fan control: Created custom profile %q", profile.Name)
	return nil
}

// RestoreDefaults sets all fans back to automatic mode and removes curve assignments.
func (c *FanController) RestoreDefaults() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.deferralError(); err != nil {
		return err
	}

	c.curves.Stop()

	fans := c.hwmon.ReadAll()
	for _, f := range fans {
		if f.Controllable {
			c.curves.RemoveAssignment(f.ID)
			if err := c.hwmon.SetMode(f.ID, dto.FanModeAutomatic); err != nil {
				logger.Error("Fan control: Failed to restore automatic for %s: %v", f.ID, err)
			}
		}
	}

	c.saveConfigLocked()
	logger.Info("Fan control: Restored all fans to automatic mode")
	return nil
}

// UpdateConfig updates the fan control configuration. Changes are persisted.
func (c *FanController) UpdateConfig(config dto.FanControlConfig) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.config = config
	c.safety = NewFanSafetyGuard(c.hwmon, config.Safety)
	c.saveConfigLocked()

	logger.Info("Fan control: Configuration updated (control_enabled=%v, method=%s)", config.ControlEnabled, config.ControlMethod)
	return nil
}

// Shutdown gracefully stops curve evaluation and restores original fan state.
func (c *FanController) Shutdown() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.curves != nil {
		c.curves.Stop()
	}
	if c.safety != nil {
		c.safety.RestoreAll()
	}
	logger.Info("Fan control: Shutdown complete, original fan state restored")
}

// saveConfigLocked persists the current state. Must be called with mu held.
func (c *FanController) saveConfigLocked() {
	data := fanConfigData{
		Config:      c.config,
		Assignments: c.curves.assignments,
	}

	// Only save custom (non-built-in) profiles
	for _, p := range c.curves.Profiles() {
		if !p.BuiltIn {
			data.Profiles = append(data.Profiles, p)
		}
	}

	if err := c.configStore.Save(data); err != nil {
		logger.Error("Fan control: Failed to save config: %v", err)
	}
}

// mergeFanDevices combines hwmon and IPMI fan lists, deduplicating by name similarity.
func mergeFanDevices(hwmon, ipmi []dto.FanDevice) []dto.FanDevice {
	result := make([]dto.FanDevice, 0, len(hwmon)+len(ipmi))
	result = append(result, hwmon...)

	// IPMI fans that don't overlap with hwmon fans
	for _, iFan := range ipmi {
		found := false
		for _, hFan := range hwmon {
			if hFan.Name == iFan.Name {
				found = true
				break
			}
		}
		if !found {
			result = append(result, iFan)
		}
	}

	return result
}
