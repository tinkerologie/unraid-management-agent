package controllers

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/lib"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
)

// builtInProfiles returns the default fan profiles.
func builtInProfiles() []dto.FanProfile {
	return []dto.FanProfile{
		{
			Name:        "quiet",
			Description: "Prioritizes low noise; fans ramp slowly",
			BuiltIn:     true,
			CurvePoints: []dto.FanCurvePoint{
				{TempCelsius: 30, SpeedPercent: 20},
				{TempCelsius: 50, SpeedPercent: 30},
				{TempCelsius: 65, SpeedPercent: 50},
				{TempCelsius: 80, SpeedPercent: 80},
				{TempCelsius: 90, SpeedPercent: 100},
			},
		},
		{
			Name:        "balanced",
			Description: "Balanced cooling and noise",
			BuiltIn:     true,
			CurvePoints: []dto.FanCurvePoint{
				{TempCelsius: 30, SpeedPercent: 25},
				{TempCelsius: 45, SpeedPercent: 40},
				{TempCelsius: 60, SpeedPercent: 60},
				{TempCelsius: 75, SpeedPercent: 85},
				{TempCelsius: 85, SpeedPercent: 100},
			},
		},
		{
			Name:        "performance",
			Description: "Prioritizes cooling over noise",
			BuiltIn:     true,
			CurvePoints: []dto.FanCurvePoint{
				{TempCelsius: 30, SpeedPercent: 40},
				{TempCelsius: 40, SpeedPercent: 55},
				{TempCelsius: 55, SpeedPercent: 75},
				{TempCelsius: 70, SpeedPercent: 90},
				{TempCelsius: 80, SpeedPercent: 100},
			},
		},
	}
}

// fanCurveAssignment links a fan to a profile and temperature sensor.
type fanCurveAssignment struct {
	ProfileName    string
	TempSensorPath string
}

// FanCurveEngine evaluates temperature→speed curves and applies PWM changes.
type FanCurveEngine struct {
	mu          sync.RWMutex
	profiles    map[string]dto.FanProfile
	assignments map[string]fanCurveAssignment // keyed by fan ID
	hwmon       *HwmonProvider
	safety      *FanSafetyGuard
	cancel      context.CancelFunc
	running     bool
}

// NewFanCurveEngine creates a curve engine with built-in profiles.
func NewFanCurveEngine(hwmon *HwmonProvider, safety *FanSafetyGuard) *FanCurveEngine {
	profileMap := make(map[string]dto.FanProfile)
	for _, p := range builtInProfiles() {
		profileMap[p.Name] = p
	}

	return &FanCurveEngine{
		profiles:    profileMap,
		assignments: make(map[string]fanCurveAssignment),
		hwmon:       hwmon,
		safety:      safety,
	}
}

// AddProfile registers a custom profile. Built-in profiles cannot be overwritten.
func (e *FanCurveEngine) AddProfile(profile dto.FanProfile) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if existing, ok := e.profiles[profile.Name]; ok && existing.BuiltIn {
		return &fanError{msg: "cannot overwrite built-in profile: " + profile.Name}
	}

	// Sort curve points by temperature
	sort.Slice(profile.CurvePoints, func(i, j int) bool {
		return profile.CurvePoints[i].TempCelsius < profile.CurvePoints[j].TempCelsius
	})

	e.profiles[profile.Name] = profile
	return nil
}

// AssignProfile links a fan to a named profile and optional temperature sensor.
func (e *FanCurveEngine) AssignProfile(fanID, profileName, tempSensorPath string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if _, ok := e.profiles[profileName]; !ok {
		return &fanError{msg: "profile not found: " + profileName}
	}

	e.assignments[fanID] = fanCurveAssignment{
		ProfileName:    profileName,
		TempSensorPath: tempSensorPath,
	}
	return nil
}

// RemoveAssignment removes a fan's profile assignment.
func (e *FanCurveEngine) RemoveAssignment(fanID string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.assignments, fanID)
}

// GetAssignment returns the profile assignment for a fan, if any.
func (e *FanCurveEngine) GetAssignment(fanID string) (string, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	a, ok := e.assignments[fanID]
	if !ok {
		return "", false
	}
	return a.ProfileName, true
}

// Profiles returns a copy of all registered profiles.
func (e *FanCurveEngine) Profiles() []dto.FanProfile {
	e.mu.RLock()
	defer e.mu.RUnlock()

	profiles := make([]dto.FanProfile, 0, len(e.profiles))
	for _, p := range e.profiles {
		profiles = append(profiles, p)
	}
	return profiles
}

// Start begins the periodic curve evaluation loop.
func (e *FanCurveEngine) Start(interval time.Duration) {
	e.mu.Lock()
	if e.running {
		e.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	e.cancel = cancel
	e.running = true
	e.mu.Unlock()

	go e.loop(ctx, interval)
}

// Stop halts the curve evaluation loop.
func (e *FanCurveEngine) Stop() {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.cancel != nil {
		e.cancel()
		e.cancel = nil
	}
	e.running = false
}

func (e *FanCurveEngine) loop(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.applyCurves()
		}
	}
}

func (e *FanCurveEngine) applyCurves() {
	e.mu.RLock()
	assignments := make(map[string]fanCurveAssignment, len(e.assignments))
	for k, v := range e.assignments {
		assignments[k] = v
	}
	e.mu.RUnlock()

	for fanID, assignment := range assignments {
		e.mu.RLock()
		profile, ok := e.profiles[assignment.ProfileName]
		e.mu.RUnlock()
		if !ok {
			continue
		}

		// Read the linked temperature sensor
		tempC := 0.0
		if assignment.TempSensorPath != "" {
			raw := lib.ReadSysfsInt(assignment.TempSensorPath)
			if raw > 0 {
				tempC = float64(raw) / 1000.0
			}
		}
		if tempC == 0 || !lib.IsPlausibleTempC(tempC) {
			continue // no valid or plausible temp reading, skip
		}

		targetPct := interpolateSpeed(profile.CurvePoints, tempC)
		targetPct = e.safety.ValidatePWM(targetPct)
		targetPWM := lib.PctToPWM(targetPct)

		if err := e.hwmon.SetPWM(fanID, targetPWM); err != nil {
			logger.Debug("Fan curve: Failed to set PWM for %s: %v", fanID, err)
		}
	}
}

// interpolateSpeed determines the fan speed percentage for a given temperature
// by linearly interpolating between the nearest curve points.
func interpolateSpeed(points []dto.FanCurvePoint, tempC float64) int {
	if len(points) == 0 {
		return 100
	}

	// Below lowest point
	if tempC <= points[0].TempCelsius {
		return points[0].SpeedPercent
	}

	// Above highest point
	if tempC >= points[len(points)-1].TempCelsius {
		return points[len(points)-1].SpeedPercent
	}

	// Find surrounding points and interpolate
	for i := 1; i < len(points); i++ {
		if tempC <= points[i].TempCelsius {
			lower := points[i-1]
			upper := points[i]

			tempRange := upper.TempCelsius - lower.TempCelsius
			if tempRange == 0 {
				return upper.SpeedPercent
			}

			fraction := (tempC - lower.TempCelsius) / tempRange
			speedRange := float64(upper.SpeedPercent - lower.SpeedPercent)
			return lower.SpeedPercent + int(fraction*speedRange)
		}
	}

	return 100
}

// fanError is a simple error type for fan-related errors.
type fanError struct {
	msg string
}

func (e *fanError) Error() string {
	return e.msg
}
