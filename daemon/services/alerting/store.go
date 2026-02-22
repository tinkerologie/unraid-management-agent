package alerting

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
	// DefaultConfigDir is the default directory for alert configuration.
	DefaultConfigDir = "/boot/config/plugins/unraid-management-agent"

	// AlertsConfigFile is the filename for alert rules configuration.
	AlertsConfigFile = "alerts.json"

	// MaxAlertRules is the maximum number of alert rules allowed.
	MaxAlertRules = 50
)

// Store manages persistent storage of alert rules in a JSON file.
type Store struct {
	mu       sync.RWMutex
	rules    []dto.AlertRule
	filePath string
}

// NewStore creates a new alert rule store. If configDir is empty, DefaultConfigDir is used.
func NewStore(configDir string) *Store {
	if configDir == "" {
		configDir = DefaultConfigDir
	}
	return &Store{
		filePath: filepath.Join(configDir, AlertsConfigFile),
		rules:    make([]dto.AlertRule, 0),
	}
}

// Load reads alert rules from the JSON config file.
// If the file doesn't exist, starts with an empty rule set.
func (s *Store) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			logger.Info("Alerting: No config file found at %s, starting with empty rules", s.filePath)
			s.rules = make([]dto.AlertRule, 0)
			return nil
		}
		return fmt.Errorf("failed to read alerts config: %w", err)
	}

	var config dto.AlertRulesConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("failed to parse alerts config: %w", err)
	}

	s.rules = config.Rules
	if s.rules == nil {
		s.rules = make([]dto.AlertRule, 0)
	}

	logger.Info("Alerting: Loaded %d rules from %s", len(s.rules), s.filePath)
	return nil
}

// save writes the current rules to the JSON config file (must be called with lock held).
func (s *Store) save() error {
	config := dto.AlertRulesConfig{Rules: s.rules}
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal alerts config: %w", err)
	}

	// Ensure directory exists
	dir := filepath.Dir(s.filePath)
	if err := os.MkdirAll(dir, 0o750); err != nil { //nolint:gosec // G301: Plugin config directory
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	if err := os.WriteFile(s.filePath, data, 0o600); err != nil { //nolint:gosec // G306: Plugin config file
		return fmt.Errorf("failed to write alerts config: %w", err)
	}

	return nil
}

// GetRules returns a copy of all alert rules.
func (s *Store) GetRules() []dto.AlertRule {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rules := make([]dto.AlertRule, len(s.rules))
	copy(rules, s.rules)
	return rules
}

// GetEnabledRules returns only enabled alert rules.
func (s *Store) GetEnabledRules() []dto.AlertRule {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var enabled []dto.AlertRule
	for _, r := range s.rules {
		if r.Enabled {
			enabled = append(enabled, r)
		}
	}
	return enabled
}

// GetRule returns a specific rule by ID.
func (s *Store) GetRule(id string) (*dto.AlertRule, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, r := range s.rules {
		if r.ID == id {
			rule := r
			return &rule, nil
		}
	}
	return nil, fmt.Errorf("alert rule not found: %s", id)
}

// CreateRule adds a new alert rule and persists to disk.
func (s *Store) CreateRule(rule dto.AlertRule) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.rules) >= MaxAlertRules {
		return fmt.Errorf("maximum number of alert rules (%d) reached", MaxAlertRules)
	}

	// Check for duplicate ID
	for _, r := range s.rules {
		if r.ID == rule.ID {
			return fmt.Errorf("alert rule with ID %s already exists", rule.ID)
		}
	}

	// Set defaults
	if rule.CooldownMinutes <= 0 {
		rule.CooldownMinutes = 5
	}

	s.rules = append(s.rules, rule)
	if err := s.save(); err != nil {
		// Rollback
		s.rules = s.rules[:len(s.rules)-1]
		return err
	}

	logger.Info("Alerting: Created rule %s (%s)", rule.ID, rule.Name)
	return nil
}

// UpdateRule updates an existing alert rule and persists to disk.
func (s *Store) UpdateRule(rule dto.AlertRule) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, r := range s.rules {
		if r.ID == rule.ID {
			if rule.CooldownMinutes <= 0 {
				rule.CooldownMinutes = 5
			}
			old := s.rules[i]
			s.rules[i] = rule
			if err := s.save(); err != nil {
				s.rules[i] = old
				return err
			}
			logger.Info("Alerting: Updated rule %s (%s)", rule.ID, rule.Name)
			return nil
		}
	}
	return fmt.Errorf("alert rule not found: %s", rule.ID)
}

// DeleteRule removes an alert rule by ID and persists to disk.
func (s *Store) DeleteRule(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, r := range s.rules {
		if r.ID == id {
			old := s.rules[i]
			s.rules = append(s.rules[:i], s.rules[i+1:]...)
			if err := s.save(); err != nil {
				// Rollback
				s.rules = append(s.rules[:i], append([]dto.AlertRule{old}, s.rules[i:]...)...)
				return err
			}
			logger.Info("Alerting: Deleted rule %s (%s)", id, r.Name)
			return nil
		}
	}
	return fmt.Errorf("alert rule not found: %s", id)
}
