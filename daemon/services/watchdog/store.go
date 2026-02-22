package watchdog

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
	// DefaultConfigDir is the default directory for health check configuration.
	DefaultConfigDir = "/boot/config/plugins/unraid-management-agent"

	// HealthChecksConfigFile is the filename for health check configuration.
	HealthChecksConfigFile = "healthchecks.json"

	// MaxHealthChecks is the maximum number of health checks allowed.
	MaxHealthChecks = 50

	// DefaultIntervalSeconds is the default check interval.
	DefaultIntervalSeconds = 30

	// MinIntervalSeconds is the minimum allowed check interval.
	MinIntervalSeconds = 10

	// DefaultTimeoutSeconds is the default probe timeout.
	DefaultTimeoutSeconds = 5

	// DefaultSuccessCode is the default expected HTTP status code.
	DefaultSuccessCode = 200
)

// Store manages persistent storage of health check configurations in a JSON file.
type Store struct {
	mu       sync.RWMutex
	checks   []dto.HealthCheck
	filePath string
}

// NewStore creates a new health check store. If configDir is empty, DefaultConfigDir is used.
func NewStore(configDir string) *Store {
	if configDir == "" {
		configDir = DefaultConfigDir
	}
	return &Store{
		filePath: filepath.Join(configDir, HealthChecksConfigFile),
		checks:   make([]dto.HealthCheck, 0),
	}
}

// Load reads health check configuration from the JSON config file.
// If the file doesn't exist, starts with an empty set.
func (s *Store) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			logger.Info("Health check config not found, starting with empty set")
			return nil
		}
		return fmt.Errorf("reading health check config: %w", err)
	}

	var config dto.HealthChecksConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("parsing health check config: %w", err)
	}

	s.checks = config.Checks
	if s.checks == nil {
		s.checks = make([]dto.HealthCheck, 0)
	}

	logger.Info("Loaded %d health checks from %s", len(s.checks), s.filePath)
	return nil
}

// save writes the current health checks to the JSON config file. Caller must hold the write lock.
func (s *Store) save() error {
	config := dto.HealthChecksConfig{Checks: s.checks}
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling health check config: %w", err)
	}

	dir := filepath.Dir(s.filePath)
	if err := os.MkdirAll(dir, 0o750); err != nil { //nolint:gosec // G301: Plugin config directory
		return fmt.Errorf("creating config directory: %w", err)
	}

	if err := os.WriteFile(s.filePath, data, 0o600); err != nil { //nolint:gosec // G306: Plugin config file
		return fmt.Errorf("writing health check config: %w", err)
	}

	return nil
}

// GetChecks returns a copy of all health checks.
func (s *Store) GetChecks() []dto.HealthCheck {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]dto.HealthCheck, len(s.checks))
	copy(result, s.checks)
	return result
}

// GetEnabledChecks returns only enabled health checks.
func (s *Store) GetEnabledChecks() []dto.HealthCheck {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]dto.HealthCheck, 0)
	for _, c := range s.checks {
		if c.Enabled {
			result = append(result, c)
		}
	}
	return result
}

// GetCheck returns a health check by ID.
func (s *Store) GetCheck(id string) (*dto.HealthCheck, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for i := range s.checks {
		if s.checks[i].ID == id {
			check := s.checks[i]
			return &check, nil
		}
	}
	return nil, fmt.Errorf("health check '%s' not found", id)
}

// CreateCheck adds a new health check and persists to disk.
func (s *Store) CreateCheck(check dto.HealthCheck) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.checks) >= MaxHealthChecks {
		return fmt.Errorf("maximum of %d health checks reached", MaxHealthChecks)
	}

	for _, existing := range s.checks {
		if existing.ID == check.ID {
			return fmt.Errorf("health check with ID '%s' already exists", check.ID)
		}
	}

	// Apply defaults
	if check.IntervalSeconds < MinIntervalSeconds {
		check.IntervalSeconds = DefaultIntervalSeconds
	}
	if check.TimeoutSeconds < 1 {
		check.TimeoutSeconds = DefaultTimeoutSeconds
	}
	if check.Type == dto.HealthCheckHTTP && check.SuccessCode == 0 {
		check.SuccessCode = DefaultSuccessCode
	}

	s.checks = append(s.checks, check)

	if err := s.save(); err != nil {
		// Rollback
		s.checks = s.checks[:len(s.checks)-1]
		return fmt.Errorf("saving after create: %w", err)
	}

	logger.Info("Created health check '%s' (%s)", check.ID, check.Type)
	return nil
}

// UpdateCheck updates an existing health check and persists to disk.
func (s *Store) UpdateCheck(check dto.HealthCheck) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.checks {
		if s.checks[i].ID == check.ID {
			old := s.checks[i]
			s.checks[i] = check

			if err := s.save(); err != nil {
				// Rollback
				s.checks[i] = old
				return fmt.Errorf("saving after update: %w", err)
			}

			logger.Info("Updated health check '%s'", check.ID)
			return nil
		}
	}

	return fmt.Errorf("health check '%s' not found", check.ID)
}

// DeleteCheck removes a health check by ID and persists to disk.
func (s *Store) DeleteCheck(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.checks {
		if s.checks[i].ID == id {
			old := s.checks[i]
			oldIdx := i
			s.checks = append(s.checks[:i], s.checks[i+1:]...)

			if err := s.save(); err != nil {
				// Rollback: re-insert at old position
				s.checks = append(s.checks[:oldIdx], append([]dto.HealthCheck{old}, s.checks[oldIdx:]...)...)
				return fmt.Errorf("saving after delete: %w", err)
			}

			logger.Info("Deleted health check '%s'", id)
			return nil
		}
	}

	return fmt.Errorf("health check '%s' not found", id)
}
