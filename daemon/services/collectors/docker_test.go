package collectors

import (
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	"github.com/tinkerologie/unraid-management-agent/daemon/domain"
	"github.com/tinkerologie/unraid-management-agent/daemon/dto"
	"github.com/tinkerologie/unraid-management-agent/daemon/lib/testutil"
)

func TestParseCgroupInactiveFile(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  uint64
		ok    bool
	}{
		{"valid", "anon 100\nactive_file 200\ninactive_file 436\nfile 636\n", 436, true},
		{"whitespace", "\n\tinactive_file\t42 \n", 42, true},
		{"zero", "inactive_file 0\n", 0, true},
		{"maximum", "inactive_file 18446744073709551615\n", math.MaxUint64, true},
		{"absent", "active_file 436\n", 0, false},
		{"empty", "", 0, false},
		{"non-numeric", "inactive_file invalid\n", 0, false},
		{"negative", "inactive_file -1\n", 0, false},
		{"overflow", "inactive_file 18446744073709551616\n", 0, false},
		{"missing value", "inactive_file\n", 0, false},
		{"extra fields", "inactive_file 436 extra\n", 0, false},
		{"prefixed key", "inactive_file_x 436\n", 0, false},
		{"scanner error", strings.Repeat("x", 64*1024), 0, false},
		{"scanner error after counter", "inactive_file 436\n" + strings.Repeat("x", 64*1024), 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := parseCgroupInactiveFile(strings.NewReader(tt.input))
			if got != tt.want || ok != tt.ok {
				t.Errorf("parseCgroupInactiveFile() = (%d, %t), want (%d, %t)", got, ok, tt.want, tt.ok)
			}
		})
	}
	t.Run("read error after counter", func(t *testing.T) {
		reader := io.MultiReader(strings.NewReader("inactive_file 436\n"), iotest.ErrReader(io.ErrUnexpectedEOF))
		if got, ok := parseCgroupInactiveFile(reader); got != 0 || ok {
			t.Errorf("parseCgroupInactiveFile() = (%d, %t), want (0, false)", got, ok)
		}
	})
}

func TestDockerWorkingSetMemory(t *testing.T) {
	tests := []struct {
		name                  string
		current, inactiveFile uint64
		want                  uint64
	}{
		{"normal", 636, 436, 200},
		{"zero inactive", 636, 0, 636},
		{"equal", 636, 636, 0},
		{"greater", 636, 637, 0},
		{"both zero", 0, 0, 0},
		{"maximum inactive", 1, math.MaxUint64, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := dockerWorkingSetMemory(tt.current, tt.inactiveFile); got != tt.want {
				t.Errorf("dockerWorkingSetMemory(%d, %d) = %d, want %d", tt.current, tt.inactiveFile, got, tt.want)
			}
		})
	}
}

func TestReadCgroupMemory(t *testing.T) {
	const mib = 1024 * 1024
	current := fmt.Sprintf("%d\n", 636*mib)
	inactive := fmt.Sprintf("active_file 100\ninactive_file %d\n", 436*mib)
	tests := []struct {
		name          string
		current       string
		stat          string
		statDirectory bool
		wantMB        float64
		wantPercent   float64
		wantDisplay   string
	}{
		{"adjusted", current, inactive, false, 200, 25, "200.00 MB / 800.00 MB"},
		{"missing stat", current, "", false, 636, 79.5, "636.00 MB / 800.00 MB"},
		{"unreadable stat", current, "", true, 636, 79.5, "636.00 MB / 800.00 MB"},
		{"malformed stat", current, "inactive_file invalid\n", false, 636, 79.5, "636.00 MB / 800.00 MB"},
		{"absent counter", current, "active_file 436\n", false, 636, 79.5, "636.00 MB / 800.00 MB"},
		{"zero inactive", current, "inactive_file 0\n", false, 636, 79.5, "636.00 MB / 800.00 MB"},
		{"equal inactive", current, fmt.Sprintf("inactive_file %d\n", 636*mib), false, 0, 0, "0.00 MB / 800.00 MB"},
		{"greater inactive", current, fmt.Sprintf("inactive_file %d\n", 637*mib), false, 0, 0, "0.00 MB / 800.00 MB"},
		{"missing current", "", inactive, false, 0, 0, "0.00 MB / 800.00 MB"},
		{"malformed current", "invalid\n", inactive, false, 0, 0, "0.00 MB / 800.00 MB"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			testutil.WriteFile(t, dir, "memory.max", fmt.Sprintf("%d\n", 800*mib))
			if tt.current != "" {
				testutil.WriteFile(t, dir, "memory.current", tt.current)
			}
			if tt.stat != "" {
				testutil.WriteFile(t, dir, "memory.stat", tt.stat)
			}
			if tt.statDirectory {
				if err := os.Mkdir(filepath.Join(dir, "memory.stat"), 0o750); err != nil {
					t.Fatal(err)
				}
			}
			var cont dto.ContainerInfo
			readCgroupMemory(dir, &cont)
			if cont.MemoryUsage != uint64(tt.wantMB*mib) || cont.MemoryUsageMB != tt.wantMB ||
				cont.MemoryPercent != tt.wantPercent || cont.MemoryDisplay != tt.wantDisplay || cont.MemoryLimit != 800*mib {
				t.Errorf("memory = (%d bytes, %g MB, %g%%, %q, limit %d), want (%g MB, %g%%, %q, limit %d)",
					cont.MemoryUsage, cont.MemoryUsageMB, cont.MemoryPercent, cont.MemoryDisplay, cont.MemoryLimit,
					tt.wantMB, tt.wantPercent, tt.wantDisplay, 800*mib)
			}
		})
	}
}

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
