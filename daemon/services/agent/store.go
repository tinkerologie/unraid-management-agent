package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
)

const (
	// DefaultConfigDir matches the watchdog/alert stores.
	DefaultConfigDir = "/boot/config/plugins/unraid-management-agent"
	// SessionsFile is the on-disk session log filename.
	SessionsFile = "agent_sessions.json"
	// MaxStoredSessions bounds the persisted session history.
	MaxStoredSessions = 200
)

// Store persists agent sessions to a JSON file and serves them from memory.
type Store struct {
	mu       sync.RWMutex
	sessions map[string]dto.AgentSession
	filePath string
}

// NewStore creates a session store. Empty dir uses DefaultConfigDir.
func NewStore(configDir string) *Store {
	if configDir == "" {
		configDir = DefaultConfigDir
	}
	return &Store{
		sessions: make(map[string]dto.AgentSession),
		filePath: filepath.Join(configDir, SessionsFile),
	}
}

// Put inserts or updates a session.
func (s *Store) Put(sess dto.AgentSession) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[sess.ID] = sess
}

// Get returns a session by ID.
func (s *Store) Get(id string) (dto.AgentSession, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.sessions[id]
	return v, ok
}

// List returns all sessions, newest StartedAt first.
func (s *Store) List() []dto.AgentSession {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]dto.AgentSession, 0, len(s.sessions))
	for _, v := range s.sessions {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartedAt.After(out[j].StartedAt) })
	return out
}

// Save writes the most recent MaxStoredSessions sessions to disk.
func (s *Store) Save() error {
	// Snapshot + prune atomically under a single write lock so a concurrent Put
	// cannot be lost between collecting the list and rebuilding the map. Inline
	// the collection+sort (rather than calling List(), which takes the RLock and
	// would deadlock under Lock).
	s.mu.Lock()
	list := make([]dto.AgentSession, 0, len(s.sessions))
	for _, v := range s.sessions {
		list = append(list, v)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].StartedAt.After(list[j].StartedAt) })
	if len(list) > MaxStoredSessions {
		list = list[:MaxStoredSessions]
	}
	// Rebuild the in-memory map to exactly the kept entries so it does not grow
	// unbounded.
	pruned := make(map[string]dto.AgentSession, len(list))
	for _, sess := range list {
		pruned[sess.ID] = sess
	}
	s.sessions = pruned
	s.mu.Unlock()

	// File I/O is performed outside the lock to avoid holding the mutex during disk access.
	if err := os.MkdirAll(filepath.Dir(s.filePath), 0o750); err != nil { //nolint:gosec // G301: Plugin config directory
		return fmt.Errorf("creating agent config dir: %w", err)
	}
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal sessions: %w", err)
	}
	if err := os.WriteFile(s.filePath, data, 0o600); err != nil { //nolint:gosec // G306: Plugin config file
		return fmt.Errorf("writing sessions: %w", err)
	}
	return nil
}

// Load reads sessions from disk. A missing file is not an error.
func (s *Store) Load() error {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			logger.Info("Agent: no session file yet, starting empty")
			return nil
		}
		return fmt.Errorf("reading sessions: %w", err)
	}
	var list []dto.AgentSession
	if err := json.Unmarshal(data, &list); err != nil {
		return fmt.Errorf("unmarshal sessions: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, sess := range list {
		s.sessions[sess.ID] = sess
	}
	return nil
}
