package discovery

import (
	"context"
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
			s := NewService(tt.config, tt.hostname, 8043, "2026.06.01", "")
			if got := s.instanceName(); got != tt.want {
				t.Errorf("instanceName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTxtRecords(t *testing.T) {
	s := NewService(domain.DiscoveryConfig{Enabled: true}, "tower", 8043, "2026.06.01", "")
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
	s := NewService(domain.DiscoveryConfig{Enabled: true}, "tower", 8043, "2026.06.01", "")
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

func TestAdvertiseIP(t *testing.T) {
	tests := []struct {
		name        string
		bindAddress string
		wantFixed   string // non-empty: exact IP expected; empty: heuristic fallback
	}{
		{name: "specific IPv4 bind address is advertised", bindAddress: "192.168.40.10", wantFixed: "192.168.40.10"},
		{name: "specific IPv6 bind address is advertised", bindAddress: "2001:db8::10", wantFixed: "2001:db8::10"},
		{name: "IPv4 unspecified falls back to heuristic", bindAddress: "0.0.0.0"},
		{name: "IPv6 unspecified falls back to heuristic", bindAddress: "::"},
		{name: "empty falls back to heuristic", bindAddress: ""},
		{name: "invalid value falls back to heuristic", bindAddress: "not-an-ip"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewService(domain.DiscoveryConfig{Enabled: true}, "tower", 8043, "2026.06.01", tt.bindAddress)
			got := s.advertiseIP()
			if tt.wantFixed != "" {
				if got == nil || got.String() != tt.wantFixed {
					t.Errorf("advertiseIP() = %v, want %s", got, tt.wantFixed)
				}
				return
			}
			// Heuristic fallback must match primaryIPv4 (may be nil in CI).
			want := primaryIPv4()
			if (got == nil) != (want == nil) || (got != nil && !got.Equal(want)) {
				t.Errorf("advertiseIP() = %v, want primaryIPv4() result %v", got, want)
			}
		})
	}
}

func TestStartSkipsAdvertisementOnLoopbackBind(t *testing.T) {
	for _, bindAddr := range []string{"127.0.0.1", "127.0.0.53", "::1"} {
		t.Run(bindAddr, func(t *testing.T) {
			s := NewService(domain.DiscoveryConfig{Enabled: true}, "tower", 8043, "2026.06.01", bindAddr)
			if err := s.Start(context.Background()); err != nil {
				t.Fatalf("Start() with loopback bind %q returned error: %v", bindAddr, err)
			}
			if s.server != nil {
				t.Errorf("Start() with loopback bind %q must not register an mDNS server", bindAddr)
			}
			s.Shutdown()
		})
	}
}
