package discovery

import (
	"slices"
	"strings"
	"testing"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
)

func TestInstanceName(t *testing.T) {
	tests := []struct {
		name     string
		config   domain.DiscoveryConfig
		hostname string
		want     string
	}{
		{
			name:     "falls back to hostname when no override",
			config:   domain.DiscoveryConfig{Enabled: true},
			hostname: "tower",
			want:     "tower",
		},
		{
			name:     "uses configured service name override",
			config:   domain.DiscoveryConfig{Enabled: true, ServiceName: "My Unraid"},
			hostname: "tower",
			want:     "My Unraid",
		},
		{
			name:     "empty hostname with no override returns empty",
			config:   domain.DiscoveryConfig{Enabled: true},
			hostname: "",
			want:     "",
		},
		{
			name:     "override wins even when hostname is empty",
			config:   domain.DiscoveryConfig{Enabled: true, ServiceName: "Main Unraid"},
			hostname: "",
			want:     "Main Unraid",
		},
		{
			name:     "special characters in hostname preserved",
			config:   domain.DiscoveryConfig{Enabled: true},
			hostname: "my-server_01",
			want:     "my-server_01",
		},
		{
			// instanceName returns values verbatim; mDNS-label encoding is the
			// zeroconf library's responsibility. These cases document that the
			// getter does not mangle or panic on unusual input.
			name:     "unicode hostname preserved",
			config:   domain.DiscoveryConfig{Enabled: true},
			hostname: "🏠-tower",
			want:     "🏠-tower",
		},
		{
			name:     "long hostname preserved",
			config:   domain.DiscoveryConfig{Enabled: true},
			hostname: strings.Repeat("a", 300),
			want:     strings.Repeat("a", 300),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewService(tt.config, tt.hostname, 8043, "2026.06.01")
			if got := s.instanceName(); got != tt.want {
				t.Errorf("instanceName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTxtRecords(t *testing.T) {
	s := NewService(domain.DiscoveryConfig{Enabled: true}, "tower", 8043, "2026.06.01")
	got := s.txtRecords()

	want := []string{"version=2026.06.01", "path=/api/v1", "name=tower"}
	if len(got) != len(want) {
		t.Errorf("txtRecords() returned %d records (%v), want %d (%v)", len(got), got, len(want), want)
	}
	for _, rec := range want {
		if !slices.Contains(got, rec) {
			t.Errorf("txtRecords() = %v, missing %q", got, rec)
		}
	}
}

func TestShutdownWithoutStartIsSafe(t *testing.T) {
	s := NewService(domain.DiscoveryConfig{Enabled: true}, "tower", 8043, "2026.06.01")
	// Shutdown before Start must be a no-op and must not panic.
	s.Shutdown()
}

func TestDefaultDiscoveryConfig(t *testing.T) {
	cfg := domain.DefaultDiscoveryConfig()
	if !cfg.Enabled {
		t.Error("DefaultDiscoveryConfig().Enabled = false, want true")
	}
	if cfg.ServiceName != "" {
		t.Errorf("DefaultDiscoveryConfig().ServiceName = %q, want empty", cfg.ServiceName)
	}
}
