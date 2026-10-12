package dto

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestNewPendingNICList(t *testing.T) {
	now := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	list := NewPendingNICList(now)
	if list.State != NICStatePending || !list.Timestamp.Equal(now) {
		t.Errorf("unexpected pending list: %+v", list)
	}
	b, err := json.Marshal(list)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"cards":[]`) || strings.Contains(string(b), `"errors"`) {
		t.Errorf("JSON %s", b)
	}
}
