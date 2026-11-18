package communityapps

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const attemptDirectory = "boot/config/plugins/unraid-management-agent/community-app-attempts"

type installReceipt struct {
	Version          int               `json:"version"`
	PlanID           string            `json:"plan_id"`
	ApprovalHash     [sha256.Size]byte `json:"approval_hash"`
	Owner            string            `json:"owner"`
	OwnedContainerID string            `json:"owned_container_id,omitempty"`
	ImageID          string            `json:"image_id,omitempty"`
	TemplateKeyHash  [sha256.Size]byte `json:"template_key_hash"`
	TemplateMAC      [sha256.Size]byte `json:"template_mac"`
	Attempt          Attempt           `json:"attempt"`
}

// loadReceipts and indexReceipt run under the service mutex. Clear the loaded
// flag on an uncertain write so a retry reloads the durable plan claim.
func (s *Service) loadReceipts() error {
	if s.receiptsLoaded {
		return nil
	}
	dir := filepath.Join(s.hostRoot, attemptDirectory)
	entries, err := os.ReadDir(dir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return ErrAttemptStorage
	}
	s.receipts = make(map[string]installReceipt, len(entries))
	s.attemptIDs = make(map[string]string, len(entries))
	for _, entry := range entries {
		planID, valid := strings.CutSuffix(entry.Name(), ".json")
		if !valid || !opaqueID.MatchString(planID) {
			continue
		}
		var receipt installReceipt
		err := readInstallFile(filepath.Join(dir, entry.Name()), 256<<10, func(reader io.Reader) error {
			return json.NewDecoder(reader).Decode(&receipt)
		})
		if err != nil || receipt.Version != 1 || receipt.PlanID != planID ||
			!opaqueID.MatchString(receipt.Attempt.ID) || !opaqueID.MatchString(receipt.Owner) ||
			!nativeName.MatchString(receipt.Attempt.Configuration.Name) ||
			s.attemptIDs[receipt.Attempt.ID] != "" {
			return ErrAttemptStorage
		}
		s.indexReceipt(receipt)
	}
	s.receiptsLoaded = true
	return nil
}

func (s *Service) indexReceipt(receipt installReceipt) {
	s.receipts[receipt.PlanID] = receipt
	s.attemptIDs[receipt.Attempt.ID] = receipt.PlanID
}

func (s *Service) saveReceipt(receipt installReceipt, exclusive bool) error {
	dir := filepath.Join(s.hostRoot, attemptDirectory)
	err := s.writeReceipt(dir, receipt, exclusive)
	if err != nil {
		s.receiptsLoaded = false
		return ErrAttemptStorage
	}
	return nil
}

func (s *Service) writeReceipt(dir string, receipt installReceipt, exclusive bool) error {
	if err := ensureInstallDirectory(s.hostRoot, attemptDirectory, 0o700); err != nil {
		return err
	}
	file, err := os.CreateTemp(dir, ".receipt-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(file.Name()) }()
	err = json.NewEncoder(file).Encode(receipt)
	if err == nil {
		err = file.Sync()
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	target := filepath.Join(dir, receipt.PlanID+".json")
	if exclusive {
		err = renameExclusive(file.Name(), target)
	} else {
		err = os.Rename(file.Name(), target)
	}
	if err != nil {
		return err
	}
	return syncDirectory(dir)
}

// ensureInstallDirectory syncs newly-created directory entries in their
// parents. This also refuses symlink components in native flash paths.
func ensureInstallDirectory(root, relative string, perm fs.FileMode) error {
	parent := root
	for component := range strings.SplitSeq(relative, string(filepath.Separator)) {
		child := filepath.Join(parent, component)
		info, err := os.Lstat(child)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			if err := os.Mkdir(child, perm); err != nil {
				return err
			}
			if err := syncDirectory(parent); err != nil {
				return err
			}
		case err != nil:
			return err
		case !info.IsDir():
			return ErrStorageState
		}
		parent = child
	}
	return nil
}

func syncDirectory(path string) error {
	// #nosec G304 -- Called on fixed host state directories.
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = dir.Close() }()
	return dir.Sync()
}

func (s *Service) persistProgress(receipt installReceipt) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	err := s.saveReceipt(receipt, false)
	if err == nil {
		s.indexReceipt(receipt)
	}
	return err
}
