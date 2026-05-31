package memory

import (
	"testing"
	"time"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

func TestMemoryRoundTripAndRecall(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir, 100)
	s.AddIncident(dto.AgentIncident{ID: "i1", Signature: "watchdog:Plex HTTP", Summary: "restarted plex", At: time.Now()})
	s.AddIncident(dto.AgentIncident{ID: "i2", Signature: "alert:High CPU", Summary: "killed runaway", At: time.Now().Add(time.Second)})
	if err := s.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}
	s2 := NewStore(dir, 100)
	if err := s2.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(s2.ListIncidents()) != 2 || s2.ListIncidents()[0].ID != "i2" {
		t.Fatalf("incidents reload/order wrong: %+v", s2.ListIncidents())
	}
	hits := s2.Recall("watchdog:Plex HTTP timeout", 3)
	if len(hits) == 0 || hits[0].ID != "i1" {
		t.Fatalf("recall should surface the Plex incident first: %+v", hits)
	}
	if got := s2.Recall("zfs:pool degraded", 3); len(got) != 0 {
		t.Fatalf("expected no recall for unrelated signature, got %+v", got)
	}
}

func TestMemoryMaxIncidents(t *testing.T) {
	s := NewStore(t.TempDir(), 3)
	for i := 0; i < 10; i++ {
		s.AddIncident(dto.AgentIncident{ID: string(rune('a' + i)), Signature: "x", At: time.Now().Add(time.Duration(i) * time.Second)})
	}
	if len(s.ListIncidents()) != 3 {
		t.Fatalf("expected bounded to 3, got %d", len(s.ListIncidents()))
	}
}

func TestPreferenceConfirm(t *testing.T) {
	s := NewStore(t.TempDir(), 100)
	s.AddPreference(dto.AgentPreference{ID: "p1", Kind: "auto_approve_tool", Subject: "restart_container", Status: dto.PreferencePending})
	if len(s.ActivePreferences()) != 0 {
		t.Fatal("pending preference must not be active")
	}
	if err := s.ConfirmPreference("p1"); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	if len(s.ActivePreferences()) != 1 {
		t.Fatal("confirmed preference should be active")
	}
	if err := s.ConfirmPreference("missing"); err == nil {
		t.Fatal("expected error confirming unknown preference")
	}
}
