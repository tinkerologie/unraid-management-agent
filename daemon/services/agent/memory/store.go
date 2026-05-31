// Package memory provides the agent's episodic (incident) and semantic
// (preference) memory with a simple keyword/tag recall.
package memory

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
)

// DefaultConfigDir matches the other agent/watchdog stores.
const DefaultConfigDir = "/boot/config/plugins/unraid-management-agent"

// MemoryFile is the on-disk filename.
const MemoryFile = "agent_memory.json"

type persisted struct {
	Incidents   []dto.AgentIncident   `json:"incidents"`
	Preferences []dto.AgentPreference `json:"preferences"`
}

// Store holds episodic incidents and semantic preferences, persisted as JSON.
type Store struct {
	mu           sync.RWMutex
	incidents    []dto.AgentIncident
	preferences  []dto.AgentPreference
	maxIncidents int
	filePath     string
}

// NewStore creates a memory store. Empty dir uses DefaultConfigDir.
func NewStore(configDir string, maxIncidents int) *Store {
	if configDir == "" {
		configDir = DefaultConfigDir
	}
	if maxIncidents <= 0 {
		maxIncidents = 200
	}
	return &Store{maxIncidents: maxIncidents, filePath: filepath.Join(configDir, MemoryFile)}
}

// AddIncident records an incident, keeping only the newest maxIncidents.
func (s *Store) AddIncident(inc dto.AgentIncident) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.incidents = append(s.incidents, inc)
	sort.Slice(s.incidents, func(i, j int) bool { return s.incidents[i].At.After(s.incidents[j].At) })
	if len(s.incidents) > s.maxIncidents {
		s.incidents = s.incidents[:s.maxIncidents]
	}
}

// ListIncidents returns incidents newest-first.
func (s *Store) ListIncidents() []dto.AgentIncident {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]dto.AgentIncident, len(s.incidents))
	copy(out, s.incidents)
	return out
}

// tokenize lowercases and splits on non-alphanumeric runs.
func tokenize(s string) map[string]bool {
	set := map[string]bool{}
	for _, f := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < '0' || r > '9')
	}) {
		if len(f) > 1 {
			set[f] = true
		}
	}
	return set
}

// Recall returns up to k incidents whose signature shares tokens with query,
// scored by token overlap (positive scores only), highest first.
func (s *Store) Recall(query string, k int) []dto.AgentIncident {
	s.mu.RLock()
	defer s.mu.RUnlock()
	q := tokenize(query)
	type scored struct {
		inc   dto.AgentIncident
		score int
	}
	var hits []scored
	for _, inc := range s.incidents {
		score := 0
		for t := range tokenize(inc.Signature) {
			if q[t] {
				score++
			}
		}
		if score > 0 {
			hits = append(hits, scored{inc, score})
		}
	}
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].score > hits[j].score })
	out := make([]dto.AgentIncident, 0, k)
	for i := 0; i < len(hits) && i < k; i++ {
		out = append(out, hits[i].inc)
	}
	return out
}

// AddPreference appends a preference.
func (s *Store) AddPreference(p dto.AgentPreference) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.preferences = append(s.preferences, p)
}

// ListPreferences returns all preferences.
func (s *Store) ListPreferences() []dto.AgentPreference {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]dto.AgentPreference, len(s.preferences))
	copy(out, s.preferences)
	return out
}

// ConfirmPreference flips a pending preference to active.
func (s *Store) ConfirmPreference(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.preferences {
		if s.preferences[i].ID == id {
			s.preferences[i].Status = dto.PreferenceActive
			return nil
		}
	}
	return fmt.Errorf("preference %q not found", id)
}

// ActivePreferences returns only active preferences.
func (s *Store) ActivePreferences() []dto.AgentPreference {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []dto.AgentPreference
	for _, p := range s.preferences {
		if p.Status == dto.PreferenceActive {
			out = append(out, p)
		}
	}
	return out
}

// Save writes the store to disk.
func (s *Store) Save() error {
	s.mu.RLock()
	data := persisted{Incidents: s.incidents, Preferences: s.preferences}
	s.mu.RUnlock()
	if err := os.MkdirAll(filepath.Dir(s.filePath), 0o755); err != nil { // #nosec G301 -- plugin config dir
		return fmt.Errorf("creating agent config dir: %w", err)
	}
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal memory: %w", err)
	}
	if err := os.WriteFile(s.filePath, b, 0o600); err != nil { // #nosec G306 -- 0600 is restrictive
		return fmt.Errorf("writing memory: %w", err)
	}
	return nil
}

// Load reads the store from disk; a missing file is not an error.
func (s *Store) Load() error {
	b, err := os.ReadFile(s.filePath) // #nosec G304 -- path is operator-controlled, under the plugin config dir
	if err != nil {
		if os.IsNotExist(err) {
			logger.Info("Agent: no memory file yet, starting empty")
			return nil
		}
		return fmt.Errorf("reading memory: %w", err)
	}
	var data persisted
	if err := json.Unmarshal(b, &data); err != nil {
		return fmt.Errorf("unmarshal memory: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.incidents = data.Incidents
	s.preferences = data.Preferences
	return nil
}
