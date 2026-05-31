package agent

import (
	"fmt"
	"testing"
	"time"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

func TestStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	s.Put(dto.AgentSession{ID: "a", Goal: "g1", Status: dto.SessionCompleted, StartedAt: time.Now()})
	s.Put(dto.AgentSession{ID: "b", Goal: "g2", Status: dto.SessionRunning, StartedAt: time.Now().Add(time.Second)})

	if err := s.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}
	s2 := NewStore(dir)
	if err := s2.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	got, ok := s2.Get("a")
	if !ok || got.Goal != "g1" {
		t.Fatalf("reload missing session a: %+v ok=%v", got, ok)
	}
	list := s2.List()
	if len(list) != 2 || list[0].ID != "b" {
		t.Fatalf("expected newest-first list, got %+v", list)
	}
}

func TestStoreSavePrunesInMemory(t *testing.T) {
	s := NewStore(t.TempDir())
	base := time.Now()
	total := MaxStoredSessions + 10
	for i := 0; i < total; i++ {
		s.Put(dto.AgentSession{
			ID:        fmt.Sprintf("sess-%d", i),
			Goal:      "g",
			Status:    dto.SessionCompleted,
			StartedAt: base.Add(time.Duration(i) * time.Second),
		})
	}
	if err := s.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}
	if got := len(s.List()); got != MaxStoredSessions {
		t.Fatalf("expected in-memory map pruned to %d, got %d", MaxStoredSessions, got)
	}
}
