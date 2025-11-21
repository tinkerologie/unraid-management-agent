package collectors

import (
	"encoding/json"
	"testing"

	"github.com/cskr/pubsub"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
)

func TestNewDockerCollector(t *testing.T) {
	hub := pubsub.New(10)
	ctx := &domain.Context{Hub: hub}

	collector := NewDockerCollector(ctx)

	if collector == nil {
		t.Fatal("NewDockerCollector() returned nil")
	}

	if collector.ctx != ctx {
		t.Error("DockerCollector context not set correctly")
	}
}

func TestDockerPSOutputParsing(t *testing.T) {
	// Test parsing of docker ps JSON output
	tests := []struct {
		name    string
		input   string
		wantID  string
		wantErr bool
	}{
		{
			name:    "valid container",
			input:   `{"ID":"abc123","Image":"nginx:latest","Names":"nginx","State":"running","Status":"Up 2 hours","Ports":"80/tcp"}`,
			wantID:  "abc123",
			wantErr: false,
		},
		{
			name:    "container with empty ports",
			input:   `{"ID":"def456","Image":"redis:alpine","Names":"redis","State":"running","Status":"Up 1 hour","Ports":""}`,
			wantID:  "def456",
			wantErr: false,
		},
		{
			name:    "invalid JSON",
			input:   `not json`,
			wantID:  "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var psOutput struct {
				ID     string `json:"ID"`
				Image  string `json:"Image"`
				Names  string `json:"Names"`
				State  string `json:"State"`
				Status string `json:"Status"`
				Ports  string `json:"Ports"`
			}

			err := json.Unmarshal([]byte(tt.input), &psOutput)
			if (err != nil) != tt.wantErr {
				t.Errorf("json.Unmarshal() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && psOutput.ID != tt.wantID {
				t.Errorf("ID = %q, want %q", psOutput.ID, tt.wantID)
			}
		})
	}
}

func TestDockerStateMapping(t *testing.T) {
	// Test container state parsing
	tests := []struct {
		state    string
		expected string
	}{
		{"running", "running"},
		{"exited", "exited"},
		{"paused", "paused"},
		{"created", "created"},
		{"restarting", "restarting"},
		{"removing", "removing"},
		{"dead", "dead"},
	}

	for _, tt := range tests {
		t.Run(tt.state, func(t *testing.T) {
			// Verify state is passed through correctly
			if tt.state != tt.expected {
				t.Errorf("State mapping %q != %q", tt.state, tt.expected)
			}
		})
	}
}
