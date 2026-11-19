package communityapps

import (
	"crypto/rand"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func installFixtureReceipt(phase string, state AttemptState) installReceipt {
	return installReceipt{
		Version: installReceiptVersion,
		PlanID:  rand.Text(),
		Owner:   rand.Text(),
		Attempt: Attempt{
			ID:            rand.Text(),
			Phase:         phase,
			State:         state,
			Configuration: Configuration{Name: "recovery-fixture", Image: "example/recovery:latest"},
		},
	}
}

func TestReceiptVisibilityWaitsForDirectorySyncAfterExclusiveClaim(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can sync a directory without read permission")
	}
	service := NewService(nil, ServiceOptions{HostRoot: t.TempDir()})
	t.Cleanup(service.Close)
	dir := filepath.Join(service.hostRoot, attemptDirectory)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o300); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	receipt := installFixtureReceipt("queued", AttemptPending)
	if err := service.saveReceipt(receipt, true); !errors.Is(err, ErrAttemptStorage) {
		t.Fatalf("a rename with unavailable directory sync was acknowledged: %v", err)
	}
	filename := filepath.Join(dir, receipt.PlanID+".json")
	if _, err := os.Stat(filename); err != nil {
		t.Fatalf("exclusive rename did not take effect before the failed sync: %v", err)
	}

	// Flash can briefly disappear after the rename. A missing directory
	// must not yield a clean empty index while the claim remains uncertain.
	held := dir + "-held"
	if err := os.Rename(dir, held); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := os.Stat(held); err == nil {
			_ = os.Rename(held, dir)
		}
	})
	if _, err := service.GetAttempt(receipt.Attempt.ID); !errors.Is(err, ErrAttemptStorage) {
		t.Fatalf("missing flash appeared to clear the pending directory sync: %v", err)
	}

	if err := os.Rename(held, dir); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if found, err := service.GetAttempt(receipt.Attempt.ID); err != nil || found.State != AttemptPending {
		t.Fatalf("the original claim was not loaded after the directory became syncable: %+v, %v", found, err)
	}
	if err := service.saveReceipt(receipt, true); !errors.Is(err, ErrAttemptStorage) {
		t.Fatalf("an exclusive claim replaced the existing durable receipt: %v", err)
	}
	if found, err := service.GetAttempt(receipt.Attempt.ID); err != nil || found.State != AttemptPending {
		t.Fatalf("the original claim changed after duplicate publication: %+v, %v", found, err)
	}
}

func TestReceiptSetupRefusesInaccessibleFlashPaths(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory permission restrictions")
	}
	for _, tc := range []struct {
		name     string
		location string
		mode     fs.FileMode
	}{
		{name: "missing claim directory cannot be created", location: filepath.Dir(attemptDirectory), mode: 0o500},
		{name: "claim directory cannot be inspected", location: filepath.Dir(attemptDirectory), mode: 0o400},
		{name: "claim directory cannot accept a temporary file", location: attemptDirectory, mode: 0o500},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service := NewService(nil, ServiceOptions{HostRoot: t.TempDir()})
			t.Cleanup(service.Close)
			blocked := filepath.Join(service.hostRoot, tc.location)
			if err := os.MkdirAll(blocked, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(blocked, tc.mode); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.Chmod(blocked, 0o700) })
			receipt := installFixtureReceipt("queued", AttemptPending)
			if err := service.saveReceipt(receipt, true); !errors.Is(err, ErrAttemptStorage) {
				t.Fatalf("a claim was accepted on inaccessible flash: %v", err)
			}
			if err := os.Chmod(blocked, 0o700); err != nil {
				t.Fatal(err)
			}
			if _, err := service.GetAttempt(receipt.Attempt.ID); !errors.Is(err, ErrAttemptNotFound) {
				t.Fatalf("failed claim left a durable attempt: %v", err)
			}
			if err := service.saveReceipt(receipt, true); err != nil {
				t.Fatalf("claim did not succeed after permissions were restored: %v", err)
			}
		})
	}
}

func TestReceiptRejectsStaleWorkerProgressAfterTerminalOutcome(t *testing.T) {
	service := NewService(nil, ServiceOptions{HostRoot: t.TempDir()})
	t.Cleanup(service.Close)
	if _, err := service.GetAttempt("invalid-id"); !errors.Is(err, ErrAttemptNotFound) {
		t.Fatalf("invalid attempt ID returned %v", err)
	}
	receipt := installFixtureReceipt("complete", AttemptSucceeded)
	if err := service.loadReceipts(); err != nil {
		t.Fatal(err)
	}
	if err := service.persistProgress(receipt); err != nil {
		t.Fatal(err)
	}
	stale := receipt
	stale.Attempt.Phase, stale.Attempt.State = "pulling_image", AttemptRunning
	service.pendingCommit[receipt.PlanID] = stale
	if err := service.persistProgress(stale); err != nil {
		t.Fatal(err)
	}
	if _, exists := service.pendingCommit[receipt.PlanID]; exists {
		t.Fatal("terminal attempt retained stale recovery work")
	}
	fresh := NewService(nil, ServiceOptions{HostRoot: service.hostRoot})
	t.Cleanup(fresh.Close)
	got, err := fresh.GetAttempt(receipt.Attempt.ID)
	if err != nil || got.State != AttemptSucceeded || got.Phase != "complete" {
		t.Fatalf("stale progress changed terminal attempt: %+v, %v", got, err)
	}
}
