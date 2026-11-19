package communityapps

import (
	"io"
	"path/filepath"
	"regexp"
	"strings"
)

const engineBootIDPath = "proc/sys/kernel/random/boot_id"

var engineBootIDPattern = regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`)

func (s *Service) readEngineBootID() string {
	var id string
	err := readInstallFile(filepath.Join(s.hostRoot, engineBootIDPath), 128, func(r io.Reader) error {
		data, err := io.ReadAll(r)
		id = strings.TrimSpace(string(data))
		return err
	})
	if err != nil || !engineBootIDPattern.MatchString(id) {
		return ""
	}
	return id
}

// An old Docker request can still take effect after a client or daemon restart.
// Only a new host boot proves that all processes from that request have ended.
// Legacy or unavailable boot proof is first observed and saved for a future boot.
func (s *Service) oldEngineCallsDrained(receipt *installReceipt) bool {
	current := s.readEngineBootID()
	if current == "" || current == receipt.EngineBootID {
		return false
	}
	if engineBootIDPattern.MatchString(receipt.EngineBootID) {
		return true
	}
	next := *receipt
	next.EngineBootID = current
	if s.persistProgress(next) == nil {
		*receipt = next
	}
	return false
}
