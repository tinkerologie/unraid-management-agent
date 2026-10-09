package mcp

import (
	"strings"
	"testing"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

// TestFindRootCauseArrayState verifies the array_not_started signal follows
// Unraid's upper-case mdState ("STARTED").
func TestFindRootCauseArrayState(t *testing.T) {
	tests := []struct {
		state      string
		wantSignal bool
	}{
		{"STARTED", false},
		{"STOPPED", true},
	}
	for _, tt := range tests {
		t.Run(tt.state, func(t *testing.T) {
			server, mock := setupInitializedServer(t)
			mock.arrayStatus = &dto.ArrayStatus{State: tt.state}
			cs, cleanup := connectClientToServer(t, server)
			defer cleanup()

			_, text := callToolJSON(t, cs, "find_root_cause", map[string]any{})

			if got := strings.Contains(text, "array_not_started"); got != tt.wantSignal {
				t.Errorf("state %q: array_not_started present = %v, want %v; response: %s",
					tt.state, got, tt.wantSignal, text)
			}
		})
	}
}
