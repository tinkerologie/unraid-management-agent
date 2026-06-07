package controllers

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
)

const (
	// fanConfigDir is the persistent config directory on the Unraid flash drive.
	fanConfigDir = "/boot/config/plugins/unraid-management-agent"

	// fanConfigFile is the filename for fan control configuration.
	fanConfigFile = "fancontrol.json"
)

// fanConfigData is the on-disk JSON schema.
type fanConfigData struct {
	Config      dto.FanControlConfig          `json:"config"`
	Profiles    []dto.FanProfile              `json:"profiles,omitempty"`
	Assignments map[string]fanCurveAssignment `json:"assignments,omitempty"`
}

// FanConfigStore persists fan control settings to a JSON file.
type FanConfigStore struct {
	mu       sync.RWMutex
	filePath string
}

// NewFanConfigStore creates a config store. If configDir is empty, the default is used.
func NewFanConfigStore(configDir string) *FanConfigStore {
	if configDir == "" {
		configDir = fanConfigDir
	}
	return &FanConfigStore{
		filePath: filepath.Join(configDir, fanConfigFile),
	}
}

// Load reads the fan control config from disk. Returns a zero-value config if the file
// does not exist (first run).
func (s *FanConfigStore) Load() (fanConfigData, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var data fanConfigData

	raw, err := os.ReadFile(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			logger.Info("Fan config: No config file found at %s, using defaults", s.filePath)
			data.Config = defaultFanControlConfig()
			return data, nil
		}
		return data, fmt.Errorf("read fan config: %w", err)
	}

	if err := json.Unmarshal(raw, &data); err != nil {
		return data, fmt.Errorf("parse fan config: %w", err)
	}

	logger.Info("Fan config: Loaded from %s", s.filePath)
	return data, nil
}

// Save writes the fan control config to disk atomically.
func (s *FanConfigStore) Save(data fanConfigData) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal fan config: %w", err)
	}

	dir := filepath.Dir(s.filePath)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("create fan config dir: %w", err)
	}

	if err := os.WriteFile(s.filePath, raw, 0o600); err != nil {
		return fmt.Errorf("write fan config: %w", err)
	}

	logger.Info("Fan config: Saved to %s", s.filePath)
	return nil
}

// UnmarshalJSON accepts both the current shape ({profile_name, source}) and the
// legacy flat shape ({ProfileName, TempSensorPath}) so existing fancontrol.json
// files keep working. A legacy TempSensorPath maps to a hwmon source.
func (a *fanCurveAssignment) UnmarshalJSON(data []byte) error {
	// New shape first.
	type newShape struct {
		ProfileName string            `json:"profile_name"`
		Source      dto.FanTempSource `json:"source"`
	}
	var ns newShape
	if err := json.Unmarshal(data, &ns); err != nil {
		return fmt.Errorf("unmarshal fan curve assignment: %w", err)
	}

	// Legacy shape (capitalized keys from the old default Go encoding).
	type legacyShape struct {
		ProfileName    string `json:"ProfileName"`
		TempSensorPath string `json:"TempSensorPath"`
	}
	var ls legacyShape
	// Best-effort: legacyShape has only string fields, so this cannot fail on
	// already-valid JSON (the new-shape unmarshal above guarantees validity).
	_ = json.Unmarshal(data, &ls)

	a.ProfileName = ns.ProfileName
	if a.ProfileName == "" {
		a.ProfileName = ls.ProfileName
	}

	a.Source = ns.Source
	if a.Source.Type == "" && ls.TempSensorPath != "" {
		a.Source = dto.FanTempSource{Type: dto.FanTempSourceHwmon, SensorPath: ls.TempSensorPath}
	}
	return nil
}

// defaultFanControlConfig returns the initial configuration with monitoring
// enabled but active control disabled (safe default).
func defaultFanControlConfig() dto.FanControlConfig {
	return dto.FanControlConfig{
		Enabled:        true,
		ControlEnabled: false,
		ControlMethod:  dto.FanMethodHwmon,
		PollInterval:   5,
		Safety: dto.FanSafetyConfig{
			MinSpeedPercent:     DefaultMinSpeedPercent,
			CriticalTempC:       DefaultCriticalTempC,
			FailureRPMThreshold: DefaultFailureRPMThreshold,
		},
	}
}
