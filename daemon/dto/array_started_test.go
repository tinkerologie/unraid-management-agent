package dto

import "testing"

func TestArrayStatusIsStarted(t *testing.T) {
	tests := []struct {
		state string
		want  bool
	}{
		{"STARTED", true}, // Unraid's mdState
		{"Started", true},
		{"started", true},
		{"STOPPED", false},
		{"Stopped", false},
		{"unknown", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := (&ArrayStatus{State: tt.state}).IsStarted(); got != tt.want {
			t.Errorf("IsStarted(%q) = %v, want %v", tt.state, got, tt.want)
		}
	}
	var missing *ArrayStatus
	if missing.IsStarted() {
		t.Error("IsStarted on nil status must be false")
	}
}
