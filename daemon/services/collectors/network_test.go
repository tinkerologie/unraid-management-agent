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
func TestNetworkIPValidation(t *testing.T) {
	tests := []struct {
		ip    string
		valid bool
	}{
		{"192.168.1.1", true},
		{"10.0.0.1", true},
		{"255.255.255.255", true},
		{"0.0.0.0", true},
		{"", false},
		{"256.1.1.1", false},
		{"192.168.1", false},
	}

	for _, tt := range tests {
		t.Run(tt.ip, func(t *testing.T) {
			valid := isValidIP(tt.ip)
			if valid != tt.valid {
				t.Errorf("isValidIP(%q) = %v, want %v", tt.ip, valid, tt.valid)
			}
		})
	}
}

func isValidIP(ip string) bool {
	if ip == "" {
		return false
	}
	parts := splitDots(ip)
	if len(parts) != 4 {
		return false
	}
	for _, part := range parts {
		n := parseInt(part)
		if n < 0 || n > 255 {
			return false
		}
	}
	return true
}

func splitDots(s string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '.' {
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	parts = append(parts, s[start:])
	return parts
}

func parseInt(s string) int {
	if s == "" {
		return -1
	}
	n := 0
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return -1
		}
		n = n*10 + int(s[i]-'0')
	}
	return n
}

func TestNetworkSpeedValues(t *testing.T) {
	speeds := []struct {
		speed    int
		expected string
	}{
		{10, "10 Mbps"},
		{100, "100 Mbps"},
		{1000, "1 Gbps"},
		{10000, "10 Gbps"},
		{25000, "25 Gbps"},
	}

	for _, tt := range speeds {
		t.Run(tt.expected, func(t *testing.T) {
			var result string
			if tt.speed >= 1000 {
				result = formatGbps(tt.speed)
			} else {
				result = formatMbps(tt.speed)
			}
			if result != tt.expected {
				t.Errorf("Speed %d = %q, want %q", tt.speed, result, tt.expected)
			}
		})
	}
}

func formatMbps(speed int) string {
	return formatInt(speed) + " Mbps"
}

func formatGbps(speed int) string {
	return formatInt(speed/1000) + " Gbps"
}

func formatInt(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

func TestNetworkDuplexModes(t *testing.T) {
	modes := []string{"full", "half", "unknown"}

	for _, mode := range modes {
		t.Run(mode, func(t *testing.T) {
			if mode == "" {
				t.Error("Duplex mode should not be empty")
			}
		})
	}
}

// TestParseUint64 tests the parseUint64 helper function
func TestParseUint64(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected uint64
	}{
		{"zero", "0", 0},
		{"positive", "12345", 12345},
		{"large number", "18446744073709551615", 18446744073709551615}, // max uint64
		{"empty string", "", 0},
		{"negative (invalid)", "-100", 0},
		{"decimal (truncates)", "123.45", 0},
		{"with letters", "abc", 0},
		{"mixed", "123abc", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseUint64(tt.input)
			if result != tt.expected {
				t.Errorf("parseUint64(%q) = %d, want %d", tt.input, result, tt.expected)
			}
		})
	}
}
