package communityapps

import (
	"bytes"
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReceiptIndexDoesNotSelectAnArbitraryDuplicate(t *testing.T) {
	root := t.TempDir()
	service := NewService(nil, ServiceOptions{HostRoot: root})
	intact := installReceipt{
		Version: installReceiptVersion, PlanID: rand.Text(), Owner: rand.Text(),
		Attempt: Attempt{ID: rand.Text(), State: AttemptSucceeded, Phase: "complete", Configuration: Configuration{Name: "intact"}},
	}
	if err := service.saveReceipt(intact, true); err != nil {
		t.Fatal(err)
	}
	duplicate := installReceipt{
		Version: installReceiptVersion, Owner: rand.Text(),
		Attempt: Attempt{ID: rand.Text(), State: AttemptPending, Phase: "queued", Configuration: Configuration{Name: "duplicate"}},
	}
	files := make(map[string][]byte)
	for _, letter := range []string{"A", "B", "C"} {
		duplicate.PlanID = strings.Repeat(letter, 26)
		if err := service.saveReceipt(duplicate, true); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(root, attemptDirectory, duplicate.PlanID+".json")
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		files[duplicate.PlanID] = data
	}

	if got, err := service.GetAttempt(intact.Attempt.ID); err != nil || got.State != AttemptSucceeded {
		t.Fatalf("duplicate attempt IDs hid a separate intact receipt: %+v, %v", got, err)
	}
	if _, err := service.GetAttempt(duplicate.Attempt.ID); !errors.Is(err, ErrAttemptStorage) {
		t.Fatalf("ambiguous attempt status returned %v", err)
	}
	if pending, err := service.interruptedInstalls(); !errors.Is(err, ErrAttemptStorage) || len(pending) != 0 {
		t.Fatalf("recovery selected an ambiguous attempt: %+v, %v", pending, err)
	}
	duplicate.Attempt.State, duplicate.Attempt.Phase = AttemptFailed, "complete"
	for id, original := range files {
		duplicate.PlanID = id
		if err := service.persistProgress(duplicate); !errors.Is(err, ErrAttemptStorage) {
			t.Fatalf("stale progress overwrote an ambiguous receipt: %v", err)
		}
		path := filepath.Join(root, attemptDirectory, id+".json")
		if got, err := os.ReadFile(path); err != nil || !bytes.Equal(got, original) {
			t.Fatalf("ambiguous receipt %s was changed or removed: %v", id, err)
		}
	}
}
