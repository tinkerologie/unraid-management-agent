package remediation

import (
	"testing"
)

func TestRunbookStorePersistsAcrossReload(t *testing.T) {
	dir := t.TempDir()
	s := NewRunbookStore(dir)
	s.Add(Runbook{Name: "drain_and_restart", Description: "Stop noisy containers then restart them."})
	if err := s.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}

	reloaded := NewRunbookStore(dir)
	if err := reloaded.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	got := reloaded.List()
	if len(got) != 1 {
		t.Fatalf("expected one runbook after reload, got %d", len(got))
	}
	if got[0].Name != "drain_and_restart" {
		t.Fatalf("unexpected runbook name %q", got[0].Name)
	}
}

func TestRunbookStoreLoadMissingFileIsNotError(t *testing.T) {
	s := NewRunbookStore(t.TempDir())
	if err := s.Load(); err != nil {
		t.Fatalf("missing file should not error: %v", err)
	}
	if len(s.List()) != 0 {
		t.Fatalf("expected empty store, got %d", len(s.List()))
	}
}

func TestRunbookStoreListReturnsCopy(t *testing.T) {
	s := NewRunbookStore(t.TempDir())
	s.Add(Runbook{Name: "a"})
	got := s.List()
	got[0].Name = "mutated"
	if s.List()[0].Name != "a" {
		t.Fatal("List must return a copy; internal state was mutated")
	}
}
