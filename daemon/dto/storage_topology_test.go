package dto

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestNewPendingStorageTopology(t *testing.T) {
	now := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	topo := NewPendingStorageTopology(now)
	if topo.State != StorageTopologyStatePending || !topo.Timestamp.Equal(now) {
		t.Errorf("unexpected pending topology: %+v", topo)
	}
	b, err := json.Marshal(topo)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{`"controllers":[]`, `"enclosures":[]`, `"drives":[]`} {
		if !strings.Contains(string(b), field) {
			t.Errorf("JSON %s lacks %s", b, field)
		}
	}
}
