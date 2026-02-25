package collectors

import (
	"testing"
	"time"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
)

func TestNewDockerCollector(t *testing.T) {
	hub := domain.NewEventBus(10)
	ctx := &domain.Context{
		Hub: hub,
	}

	collector := NewDockerCollector(ctx)

	if collector == nil {
		t.Fatal("Expected collector to be created")
	}

	if collector.appCtx != ctx {
		t.Error("Expected appCtx to be set")
	}

	if collector.initialized {
		t.Error("Expected initialized to be false initially")
	}

	if collector.dockerClient != nil {
		t.Error("Expected dockerClient to be nil initially")
	}
}

func TestDockerCollector_InitClient_NoDocker(t *testing.T) {
	hub := domain.NewEventBus(10)
	ctx := &domain.Context{
		Hub: hub,
	}

	collector := NewDockerCollector(ctx)

	// In test environment without Docker, this should fail gracefully
	// but not panic
	err := collector.initClient()
	// Error is expected in test environment without Docker socket
	if err == nil {
		// Docker is available, client should be initialized
		if collector.dockerClient == nil {
			t.Error("Expected dockerClient to be set when Docker is available")
		}
		if !collector.initialized {
			t.Error("Expected initialized to be true when Docker is available")
		}
	}
}

func TestDockerCollector_Collect_NoDocker(t *testing.T) {
	hub := domain.NewEventBus(10)
	ctx := &domain.Context{
		Hub: hub,
	}

	collector := NewDockerCollector(ctx)

	// Subscribe to events
	sub := hub.Sub("container_list_update")

	// Collect should not panic even without Docker
	collector.Collect()

	// Should receive an event (empty list or actual containers)
	select {
	case msg := <-sub:
		if msg == nil {
			t.Error("Expected non-nil message")
		}
	case <-time.After(1 * time.Second):
		t.Error("Expected to receive container_list_update event")
	}
}

func TestDockerFormatUptime(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		expected string
	}{
		{
			name:     "minutes only",
			duration: 45 * time.Minute,
			expected: "45m",
		},
		{
			name:     "hours and minutes",
			duration: 2*time.Hour + 30*time.Minute,
			expected: "2h 30m",
		},
		{
			name:     "days hours minutes",
			duration: 3*24*time.Hour + 5*time.Hour + 15*time.Minute,
			expected: "3d 5h 15m",
		},
		{
			name:     "zero minutes",
			duration: 0,
			expected: "0m",
		},
		{
			name:     "one day",
			duration: 24 * time.Hour,
			expected: "1d 0h 0m",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := dockerFormatUptime(tt.duration)
			if result != tt.expected {
				t.Errorf("dockerFormatUptime(%v) = %s, expected %s", tt.duration, result, tt.expected)
			}
		})
	}
}

func TestDockerFormatMemoryDisplay(t *testing.T) {
	tests := []struct {
		name     string
		used     uint64
		limit    uint64
		expected string
	}{
		{
			name:     "zero limit",
			used:     0,
			limit:    0,
			expected: "0 / 0",
		},
		{
			name:     "megabytes",
			used:     256 * 1024 * 1024,
			limit:    512 * 1024 * 1024,
			expected: "256.00 MB / 512.00 MB",
		},
		{
			name:     "gigabytes",
			used:     2 * 1024 * 1024 * 1024,
			limit:    4 * 1024 * 1024 * 1024,
			expected: "2.00 GB / 4.00 GB",
		},
		{
			name:     "mixed - limit over 1GB",
			used:     512 * 1024 * 1024,
			limit:    2 * 1024 * 1024 * 1024,
			expected: "0.50 GB / 2.00 GB",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := dockerFormatMemoryDisplay(tt.used, tt.limit)
			if result != tt.expected {
				t.Errorf("dockerFormatMemoryDisplay(%d, %d) = %s, expected %s", tt.used, tt.limit, result, tt.expected)
			}
		})
	}
}
