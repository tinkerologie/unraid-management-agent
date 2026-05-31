package remediation

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
)

const (
	// DefaultConfigDir matches the agent/watchdog/alert stores.
	DefaultConfigDir = "/boot/config/plugins/unraid-management-agent"
	// RunbooksFile is the on-disk proposed-runbook filename.
	RunbooksFile = "agent_runbooks.json"
)

// RunbookStore persists operator-proposed runbooks to a JSON file.
type RunbookStore struct {
	mu       sync.RWMutex
	runbooks []Runbook
	filePath string
}

// NewRunbookStore creates a runbook store. Empty dir uses DefaultConfigDir.
func NewRunbookStore(dir string) *RunbookStore {
	if dir == "" {
		dir = DefaultConfigDir
	}
	return &RunbookStore{filePath: filepath.Join(dir, RunbooksFile)}
}

// Add appends a proposed runbook.
func (s *RunbookStore) Add(rb Runbook) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.runbooks = append(s.runbooks, rb)
}

// List returns a copy of the stored runbooks.
func (s *RunbookStore) List() []Runbook {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Runbook, len(s.runbooks))
	copy(out, s.runbooks)
	return out
}

// Save writes the proposed runbooks to disk.
func (s *RunbookStore) Save() error {
	s.mu.RLock()
	list := make([]Runbook, len(s.runbooks))
	copy(list, s.runbooks)
	s.mu.RUnlock()

	if err := os.MkdirAll(filepath.Dir(s.filePath), 0o755); err != nil { //nolint:gosec // G301: Plugin config directory
		return fmt.Errorf("creating runbook config dir: %w", err)
	}
	data, err := json.MarshalIndent(map[string][]Runbook{"runbooks": list}, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal runbooks: %w", err)
	}
	if err := os.WriteFile(s.filePath, data, 0o600); err != nil { //nolint:gosec // G306: Plugin config file
		return fmt.Errorf("writing runbooks: %w", err)
	}
	return nil
}

// Load reads proposed runbooks from disk. A missing file is not an error.
func (s *RunbookStore) Load() error {
	data, err := os.ReadFile(s.filePath) //nolint:gosec // G304: Plugin config file
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			logger.Info("Runbooks: no proposal file yet, starting empty")
			return nil
		}
		return fmt.Errorf("reading runbooks: %w", err)
	}
	// Accept either {"runbooks": [...]} or a bare [...] array.
	var wrapper struct {
		Runbooks []Runbook `json:"runbooks"`
	}
	if err := json.Unmarshal(data, &wrapper); err == nil && wrapper.Runbooks != nil {
		s.mu.Lock()
		s.runbooks = wrapper.Runbooks
		s.mu.Unlock()
		return nil
	}
	var list []Runbook
	if err := json.Unmarshal(data, &list); err != nil {
		return fmt.Errorf("unmarshal runbooks: %w", err)
	}
	s.mu.Lock()
	s.runbooks = list
	s.mu.Unlock()
	return nil
}
