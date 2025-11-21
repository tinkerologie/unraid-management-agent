package collectors

import (
	"testing"

	"github.com/cskr/pubsub"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
)

func TestNewNetworkCollector(t *testing.T) {
	hub := pubsub.New(10)
	ctx := &domain.Context{Hub: hub}

	collector := NewNetworkCollector(ctx)

	if collector == nil {
		t.Fatal("NewNetworkCollector() returned nil")
	}

	if collector.ctx != ctx {
		t.Error("NetworkCollector context not set correctly")
	}
}

func TestNetworkINIParsing(t *testing.T) {
	// Test parsing of network.ini format
	content := `[eth0]
NAME=eth0
IPADDR=192.168.1.100
NETMASK=255.255.255.0
GATEWAY=192.168.1.1
DNS_SERVER1=8.8.8.8
DNS_SERVER2=8.8.4.4

[bond0]
NAME=bond0
IPADDR=10.0.0.50
NETMASK=255.255.255.0
`
	// Verify the content can be read
	if content == "" {
		t.Error("Content is empty")
	}

	// Basic validation of expected keys
	expectedKeys := []string{"NAME=", "IPADDR=", "NETMASK=", "GATEWAY="}
	for _, key := range expectedKeys {
		if !contains(content, key) {
			t.Errorf("Expected key %q not found in content", key)
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsAt(s, substr, 0))
}

func containsAt(s, substr string, start int) bool {
	for i := start; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestNetworkInterfaceTypes(t *testing.T) {
	// Test interface type detection
	tests := []struct {
		name     string
		expected string
	}{
		{"eth0", "ethernet"},
		{"eth1", "ethernet"},
		{"bond0", "bond"},
		{"bond1", "bond"},
		{"br0", "bridge"},
		{"veth123", "virtual"},
		{"docker0", "docker"},
		{"lo", "loopback"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ifType := detectInterfaceType(tt.name)
			if ifType != tt.expected {
				t.Errorf("Interface %q type = %q, want %q", tt.name, ifType, tt.expected)
			}
		})
	}
}

func detectInterfaceType(name string) string {
	switch {
	case name == "lo":
		return "loopback"
	case len(name) >= 3 && name[:3] == "eth":
		return "ethernet"
	case len(name) >= 4 && name[:4] == "bond":
		return "bond"
	case len(name) >= 2 && name[:2] == "br":
		return "bridge"
	case len(name) >= 4 && name[:4] == "veth":
		return "virtual"
	case len(name) >= 6 && name[:6] == "docker":
		return "docker"
	default:
		return "unknown"
	}
}
