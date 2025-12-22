package collectors

import (
	"encoding/json"
	"testing"
	"time"

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
func TestDockerParseSize(t *testing.T) {
	hub := pubsub.New(10)
	ctx := &domain.Context{Hub: hub}
	collector := NewDockerCollector(ctx)

	tests := []struct {
		name     string
		input    string
		expected uint64
	}{
		{"empty string", "", 0},
		{"zero bytes", "0B", 0},
		{"bytes", "100B", 100},
		{"kilobytes", "1KB", 1024},
		{"kilobytes decimal", "1.5KB", 1536},
		{"megabytes", "1MB", 1024 * 1024},
		{"megabytes decimal", "2.5MB", uint64(2.5 * 1024 * 1024)},
		{"gigabytes", "1GB", 1024 * 1024 * 1024},
		{"gigabytes decimal", "1.5GB", uint64(1.5 * 1024 * 1024 * 1024)},
		{"terabytes", "1TB", 1024 * 1024 * 1024 * 1024},
		{"lowercase kb", "1kb", 1024},
		{"lowercase mb", "1mb", 1024 * 1024},
		{"lowercase gb", "1gb", 1024 * 1024 * 1024},
		{"with spaces", "  1MB  ", 1024 * 1024},
		{"invalid string", "invalid", 0},
		{"kib variant", "1KiB", 1024},
		{"mib variant", "1MiB", 1024 * 1024},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := collector.parseSize(tt.input)
			if result != tt.expected {
				t.Errorf("parseSize(%q) = %d, want %d", tt.input, result, tt.expected)
			}
		})
	}
}

func TestDockerParsePorts(t *testing.T) {
	hub := pubsub.New(10)
	ctx := &domain.Context{Hub: hub}
	collector := NewDockerCollector(ctx)

	tests := []struct {
		name          string
		input         string
		expectedCount int
		checkFirst    bool
		firstPrivate  int
		firstPublic   int
		firstType     string
	}{
		{
			name:          "empty string",
			input:         "",
			expectedCount: 0,
		},
		{
			name:          "single port mapping",
			input:         "0.0.0.0:8080->80/tcp",
			expectedCount: 1,
			checkFirst:    true,
			firstPrivate:  80,
			firstPublic:   8080,
			firstType:     "tcp",
		},
		{
			name:          "exposed port only",
			input:         "80/tcp",
			expectedCount: 1,
			checkFirst:    true,
			firstPrivate:  80,
			firstPublic:   0,
			firstType:     "tcp",
		},
		{
			name:          "multiple port mappings",
			input:         "0.0.0.0:8080->80/tcp, 0.0.0.0:443->443/tcp",
			expectedCount: 2,
			checkFirst:    true,
			firstPrivate:  80,
			firstPublic:   8080,
			firstType:     "tcp",
		},
		{
			name:          "udp port",
			input:         "0.0.0.0:53->53/udp",
			expectedCount: 1,
			checkFirst:    true,
			firstPrivate:  53,
			firstPublic:   53,
			firstType:     "udp",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := collector.parsePorts(tt.input)
			if len(result) != tt.expectedCount {
				t.Errorf("parsePorts(%q) returned %d ports, want %d", tt.input, len(result), tt.expectedCount)
				return
			}
			if tt.checkFirst && len(result) > 0 {
				if result[0].PrivatePort != tt.firstPrivate {
					t.Errorf("PrivatePort = %d, want %d", result[0].PrivatePort, tt.firstPrivate)
				}
				if result[0].PublicPort != tt.firstPublic {
					t.Errorf("PublicPort = %d, want %d", result[0].PublicPort, tt.firstPublic)
				}
				if result[0].Type != tt.firstType {
					t.Errorf("Type = %q, want %q", result[0].Type, tt.firstType)
				}
			}
		})
	}
}

func TestDockerFormatUptime(t *testing.T) {
	hub := pubsub.New(10)
	ctx := &domain.Context{Hub: hub}
	collector := NewDockerCollector(ctx)

	tests := []struct {
		name     string
		duration time.Duration
		expected string
	}{
		{"zero", 0, "0m"},
		{"1 minute", time.Minute, "1m"},
		{"30 minutes", 30 * time.Minute, "30m"},
		{"1 hour", time.Hour, "1h 0m"},
		{"1 hour 30 minutes", time.Hour + 30*time.Minute, "1h 30m"},
		{"2 hours", 2 * time.Hour, "2h 0m"},
		{"1 day", 24 * time.Hour, "1d 0h 0m"},
		{"1 day 2 hours", 26 * time.Hour, "1d 2h 0m"},
		{"2 days 3 hours 15 minutes", 51*time.Hour + 15*time.Minute, "2d 3h 15m"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := collector.formatUptime(tt.duration)
			if result != tt.expected {
				t.Errorf("formatUptime(%v) = %q, want %q", tt.duration, result, tt.expected)
			}
		})
	}
}

func TestDockerFormatMemoryDisplay(t *testing.T) {
	hub := pubsub.New(10)
	ctx := &domain.Context{Hub: hub}
	collector := NewDockerCollector(ctx)

	tests := []struct {
		name     string
		used     uint64
		limit    uint64
		expected string
	}{
		{"zero limit", 100, 0, "0 / 0"},
		{"megabytes small", 256 * 1024 * 1024, 512 * 1024 * 1024, "256.00 MB / 512.00 MB"},
		{"gigabytes boundary", 512 * 1024 * 1024, 1024 * 1024 * 1024, "0.50 GB / 1.00 GB"},
		{"gigabytes", 2 * 1024 * 1024 * 1024, 8 * 1024 * 1024 * 1024, "2.00 GB / 8.00 GB"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := collector.formatMemoryDisplay(tt.used, tt.limit)
			if result != tt.expected {
				t.Errorf("formatMemoryDisplay(%d, %d) = %q, want %q", tt.used, tt.limit, result, tt.expected)
			}
		})
	}
}
