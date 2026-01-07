package collectors

import (
	"net"
	"strings"
	"testing"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

func TestCollectNetworkAccessURLs(t *testing.T) {
	// Test that the function returns a valid structure
	result := CollectNetworkAccessURLs()

	if result == nil {
		t.Fatal("CollectNetworkAccessURLs returned nil")
	}

	if result.Timestamp.IsZero() {
		t.Error("Timestamp should not be zero")
	}

	// Should have at least mDNS URL (hostname.local)
	hasMDNS := false
	for _, url := range result.URLs {
		if url.Type == dto.URLTypeMDNS {
			hasMDNS = true
			break
		}
	}

	if !hasMDNS {
		t.Log("Warning: No mDNS URL found (might be expected in some environments)")
	}

	// Log what was found for debugging
	t.Logf("Found %d access URLs", len(result.URLs))
	for _, url := range result.URLs {
		t.Logf("  Type: %s, Name: %s, IPv4: %s, IPv6: %s", url.Type, url.Name, url.IPv4, url.IPv6)
	}
}

func TestGetLANAccessURLs(t *testing.T) {
	urls := getLANAccessURLs()

	// Should find at least one LAN IP in most environments
	t.Logf("Found %d LAN URLs", len(urls))
	for _, url := range urls {
		if url.Type != dto.URLTypeLAN {
			t.Errorf("Expected type 'lan', got %q", url.Type)
		}
		if url.IPv4 == "" {
			t.Error("LAN URL should have IPv4 address")
		}
		t.Logf("  LAN: %s - %s", url.Name, url.IPv4)
	}
}

func TestGetMDNSAccessURL(t *testing.T) {
	url := getMDNSAccessURL()

	if url == nil {
		t.Skip("Could not get hostname, skipping mDNS test")
	}

	if url.Type != dto.URLTypeMDNS {
		t.Errorf("Expected type 'mdns', got %q", url.Type)
	}

	if url.Name != "mDNS" {
		t.Errorf("Expected name 'mDNS', got %q", url.Name)
	}

	if url.IPv4 == "" {
		t.Error("mDNS URL should have IPv4 address")
	}

	// Should end with .local
	if !strings.Contains(url.IPv4, ".local") {
		t.Errorf("mDNS URL should contain '.local', got %q", url.IPv4)
	}

	t.Logf("mDNS URL: %s", url.IPv4)
}

func TestGetWireGuardAccessURLs(t *testing.T) {
	urls := getWireGuardAccessURLs()

	// WireGuard may or may not be configured
	t.Logf("Found %d WireGuard URLs", len(urls))
	for _, url := range urls {
		if url.Type != dto.URLTypeWireGuard {
			t.Errorf("Expected type 'wireguard', got %q", url.Type)
		}
		t.Logf("  WireGuard: %s - IPv4: %s, IPv6: %s", url.Name, url.IPv4, url.IPv6)
	}
}

func TestGetIPv6AccessURLs(t *testing.T) {
	urls := getIPv6AccessURLs()

	t.Logf("Found %d IPv6 URLs", len(urls))
	for _, url := range urls {
		if url.Type != dto.URLTypeIPv6 {
			t.Errorf("Expected type 'ipv6', got %q", url.Type)
		}
		if url.IPv6 == "" {
			t.Error("IPv6 URL should have IPv6 address")
		}
		t.Logf("  IPv6: %s - %s", url.Name, url.IPv6)
	}
}

func TestIsPrivateIP(t *testing.T) {
	tests := []struct {
		name     string
		ip       string
		expected bool
	}{
		{"10.0.0.1", "10.0.0.1", true},
		{"10.255.255.255", "10.255.255.255", true},
		{"172.16.0.1", "172.16.0.1", true},
		{"172.31.255.255", "172.31.255.255", true},
		{"192.168.0.1", "192.168.0.1", true},
		{"192.168.255.255", "192.168.255.255", true},
		{"8.8.8.8", "8.8.8.8", false},
		{"1.1.1.1", "1.1.1.1", false},
		{"172.15.0.1", "172.15.0.1", false},
		{"172.32.0.1", "172.32.0.1", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip := net.ParseIP(tt.ip)
			if ip == nil {
				t.Fatalf("Failed to parse IP: %s", tt.ip)
			}

			result := isPrivateIP(ip)
			if result != tt.expected {
				t.Errorf("isPrivateIP(%s) = %v, expected %v", tt.ip, result, tt.expected)
			}
		})
	}
}

func TestGetPrimaryLANIP(t *testing.T) {
	ip := GetPrimaryLANIP()

	if ip == "" {
		t.Skip("Could not determine primary LAN IP (might be expected in isolated environment)")
	}

	t.Logf("Primary LAN IP: %s", ip)

	// Validate it looks like an IP
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		t.Errorf("Primary LAN IP %q is not a valid IP address", ip)
	}
}

func TestAccessURLTypes(t *testing.T) {
	// Test that URL type constants are defined correctly
	if dto.URLTypeLAN != "lan" {
		t.Errorf("URLTypeLAN = %q, expected 'lan'", dto.URLTypeLAN)
	}
	if dto.URLTypeWAN != "wan" {
		t.Errorf("URLTypeWAN = %q, expected 'wan'", dto.URLTypeWAN)
	}
	if dto.URLTypeWireGuard != "wireguard" {
		t.Errorf("URLTypeWireGuard = %q, expected 'wireguard'", dto.URLTypeWireGuard)
	}
	if dto.URLTypeMDNS != "mdns" {
		t.Errorf("URLTypeMDNS = %q, expected 'mdns'", dto.URLTypeMDNS)
	}
	if dto.URLTypeIPv6 != "ipv6" {
		t.Errorf("URLTypeIPv6 = %q, expected 'ipv6'", dto.URLTypeIPv6)
	}
	if dto.URLTypeOther != "other" {
		t.Errorf("URLTypeOther = %q, expected 'other'", dto.URLTypeOther)
	}
}

func TestGetHTTPSURLs(t *testing.T) {
	httpURLs := []dto.AccessURL{
		{Type: dto.URLTypeLAN, Name: "LAN", IPv4: "http://192.168.1.100"},
		{Type: dto.URLTypeMDNS, Name: "mDNS", IPv4: "http://tower.local"},
		{Type: dto.URLTypeIPv6, Name: "IPv6", IPv6: "http://[2001:db8::1]"},
	}

	// Test with default HTTPS port
	httpsURLs := GetHTTPSURLs(httpURLs, 443)
	if len(httpsURLs) != len(httpURLs) {
		t.Errorf("Expected %d HTTPS URLs, got %d", len(httpURLs), len(httpsURLs))
	}

	for _, url := range httpsURLs {
		if url.IPv4 != "" && !strings.Contains(url.IPv4, "https://") {
			t.Errorf("HTTPS URL should start with https://, got %q", url.IPv4)
		}
		if url.IPv6 != "" && !strings.Contains(url.IPv6, "https://") {
			t.Errorf("HTTPS URL should start with https://, got %q", url.IPv6)
		}
		if !strings.Contains(url.Name, "(HTTPS)") {
			t.Errorf("HTTPS URL name should contain '(HTTPS)', got %q", url.Name)
		}
	}

	// Test with custom port
	httpsURLsCustomPort := GetHTTPSURLs(httpURLs[:1], 8443)
	if len(httpsURLsCustomPort) > 0 && !strings.Contains(httpsURLsCustomPort[0].IPv4, ":8443") {
		t.Errorf("Custom port HTTPS URL should contain :8443, got %q", httpsURLsCustomPort[0].IPv4)
	}
}

func TestNetworkAccessURLsStructure(t *testing.T) {
	// Test the DTO structure
	urls := dto.NetworkAccessURLs{
		URLs: []dto.AccessURL{
			{Type: "lan", Name: "LAN", IPv4: "http://192.168.1.1"},
		},
	}

	if len(urls.URLs) != 1 {
		t.Errorf("Expected 1 URL, got %d", len(urls.URLs))
	}

	if urls.URLs[0].Type != "lan" {
		t.Errorf("Expected type 'lan', got %q", urls.URLs[0].Type)
	}
}

func TestAccessURLStructure(t *testing.T) {
	url := dto.AccessURL{
		Type: dto.URLTypeLAN,
		Name: "Test",
		IPv4: "http://192.168.1.1",
		IPv6: "http://[::1]",
	}

	if url.Type != "lan" {
		t.Errorf("Type mismatch")
	}
	if url.Name != "Test" {
		t.Errorf("Name mismatch")
	}
	if url.IPv4 != "http://192.168.1.1" {
		t.Errorf("IPv4 mismatch")
	}
	if url.IPv6 != "http://[::1]" {
		t.Errorf("IPv6 mismatch")
	}
}
