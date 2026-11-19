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

const (
	attemptDirectory      = "boot/config/plugins/unraid-management-agent/community-app-attempts"
	installReceiptVersion = 2
	maxInstallReceipt     = 256 << 10
	// Leave space for the fixed-size ownership, failure and cleanup data
	// written after acceptance.
	maxReceiptProgress = 4 << 10
)

var errReceiptSync = errors.New("receipt rename was not synced")

type installReceipt struct {
	Version          int               `json:"version"`
	PlanID           string            `json:"plan_id"`
	ApprovalHash     [sha256.Size]byte `json:"approval_hash"`
	Owner            string            `json:"owner"`
	OwnedContainerID string            `json:"owned_container_id,omitempty"`
	ImageID          string            `json:"image_id,omitempty"`
	TemplateKeyHash  [sha256.Size]byte `json:"template_key_hash"`
	CleanupPhase     string            `json:"cleanup_phase,omitempty"`
	EngineRejected   bool              `json:"engine_rejected,omitempty"`
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
	if s.receiptSyncNeeded {
		if syncDirectory(dir) != nil {
			return ErrAttemptStorage
		}
		s.receiptSyncNeeded = false
	}
	s.receipts = make(map[string]installReceipt, len(entries))
	s.attemptIDs = make(map[string]string, len(entries))
	for _, entry := range entries {
		planID, valid := strings.CutSuffix(entry.Name(), ".json")
		if !valid || !opaqueID.MatchString(planID) {
			continue
		}
		var receipt installReceipt
		err := readInstallFile(filepath.Join(dir, entry.Name()), maxInstallReceipt, func(reader io.Reader) error {
			decoder := json.NewDecoder(reader)
			if err := decoder.Decode(&receipt); err != nil {
				return err
			}
			if err := decoder.Decode(new(json.RawMessage)); !errors.Is(err, io.EOF) {
				return ErrAttemptStorage
			}
			return nil
		})
		if err != nil || (receipt.Version != 1 && receipt.Version != installReceiptVersion) || receipt.PlanID != planID ||
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
		if err == ErrInstallPlanTooLarge {
			return err
		}
		if errors.Is(err, errReceiptSync) {
			s.receiptSyncNeeded = true
		}
		if exclusive {
			s.receiptsLoaded = false
		}
		return ErrAttemptStorage
	}
	return nil
}

func (s *Service) writeReceipt(dir string, receipt installReceipt, exclusive bool) error {
	data, err := json.Marshal(receipt)
	if err != nil {
		return err
	}
	if exclusive && len(data) > maxInstallReceipt-maxReceiptProgress {
		return ErrInstallPlanTooLarge
	}
	if len(data) > maxInstallReceipt {
		return ErrAttemptStorage
	}
	if err := ensureInstallDirectory(s.hostRoot, attemptDirectory, 0o700); err != nil {
		return err
	}
	file, err := os.CreateTemp(dir, ".receipt-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(file.Name()) }()
	_, err = file.Write(data)
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
	if syncDirectory(dir) != nil {
		return errReceiptSync
	}
	return nil
}

// ensureInstallDirectory syncs each parent before relying on its child.
// A retry must sync entries left by a prior mkdir with a failed sync.
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
		case err != nil:
			return err
		case !info.IsDir():
			return ErrStorageState
		}
		if err := syncDirectory(parent); err != nil {
			return err
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
	if current, exists := s.receipts[receipt.PlanID]; exists && isTerminal(current.Attempt.State) {
		delete(s.pendingCommit, receipt.PlanID)
		return nil
	}
	err := s.saveReceipt(receipt, false)
	if err == nil {
		s.indexReceipt(receipt)
		delete(s.pendingCommit, receipt.PlanID)
	}
	return err
}
