package collectors

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/tinkerologie/unraid-management-agent/daemon/dto"
	"github.com/tinkerologie/unraid-management-agent/daemon/lib"
	"github.com/tinkerologie/unraid-management-agent/daemon/logger"
)

// defaultWANServices are asked for the server's public IP, in order, when WAN
// IP lookup is enabled.
var defaultWANServices = []string{
	"https://api.ipify.org",
	"https://ifconfig.me/ip",
	"https://icanhazip.com",
}

// httpDoer is the HTTP client surface WAN discovery needs; *http.Client
// implements it, and tests inject a stub.
type httpDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// NetworkAccessDiscovery gathers the URLs the server can be reached at. The
// WAN entry asks external IP services, which see the server's public IP; that
// lookup can be turned off (#252), leaving only Unraid's configured WANIP.
type NetworkAccessDiscovery struct {
	wanLookup      bool
	client         httpDoer
	wanServices    []string
	networkCfgPath string
}

// NewNetworkAccessDiscovery returns a discovery that contacts the external
// WAN IP services only when wanLookup is true. Construction makes no requests.
func NewNetworkAccessDiscovery(wanLookup bool) *NetworkAccessDiscovery {
	return &NetworkAccessDiscovery{
		wanLookup: wanLookup,
		// 2s per service keeps worst-case discovery under ~6s on networks
		// without outbound internet access (issue #123); a reachable IP
		// service answers in well under a second.
		client:         &http.Client{Timeout: 2 * time.Second},
		wanServices:    defaultWANServices,
		networkCfgPath: "/boot/config/network.cfg",
	}
}

// WANLookupEnabled reports whether the external WAN IP lookup is enabled.
func (d *NetworkAccessDiscovery) WANLookupEnabled() bool {
	return d.wanLookup
}

// Collect gathers all methods to access the Unraid server including LAN IP,
// mDNS hostname, WireGuard VPN IPs, WAN IP, and IPv6 addresses.
func (d *NetworkAccessDiscovery) Collect() *dto.NetworkAccessURLs {
	var urls []dto.AccessURL

	// Get primary LAN IP
	if lanURLs := getLANAccessURLs(); len(lanURLs) > 0 {
		urls = append(urls, lanURLs...)
	}

	// Get mDNS hostname (hostname.local)
	if mdnsURL := getMDNSAccessURL(); mdnsURL != nil {
		urls = append(urls, *mdnsURL)
	}

	// Get WireGuard IPs if configured
	if wgURLs := getWireGuardAccessURLs(); len(wgURLs) > 0 {
		urls = append(urls, wgURLs...)
	}

	// Get WAN IP (public IP) if accessible
	if wanURL := d.wanAccessURL(); wanURL != nil {
		urls = append(urls, *wanURL)
	}

	// Get IPv6 addresses
	if ipv6URLs := getIPv6AccessURLs(); len(ipv6URLs) > 0 {
		urls = append(urls, ipv6URLs...)
	}

	return &dto.NetworkAccessURLs{
		URLs:      urls,
		Timestamp: time.Now(),
	}
}

// getLANAccessURLs returns all LAN IPv4 addresses
func getLANAccessURLs() []dto.AccessURL {
	var urls []dto.AccessURL

	interfaces, err := net.Interfaces()
	if err != nil {
		logger.Error("Network Access: Failed to get interfaces: %v", err)
		return urls
	}

	for _, iface := range interfaces {
		// Skip loopback, down interfaces, and virtual interfaces
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		if iface.Flags&net.FlagUp == 0 {
			continue
		}

		// Skip common virtual interface prefixes
		if strings.HasPrefix(iface.Name, "docker") ||
			strings.HasPrefix(iface.Name, "br-") ||
			strings.HasPrefix(iface.Name, "veth") ||
			strings.HasPrefix(iface.Name, "virbr") ||
			strings.HasPrefix(iface.Name, "vnet") {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}

			ip := ipNet.IP
			// Only process IPv4 addresses
			if ip.To4() == nil {
				continue
			}

			// Skip link-local addresses (169.254.x.x)
			if ip.IsLinkLocalUnicast() {
				continue
			}

			// Determine if it's a private IP (LAN)
			if isPrivateIP(ip) {
				urls = append(urls, dto.AccessURL{
					Type: dto.URLTypeLAN,
					Name: fmt.Sprintf("LAN (%s)", iface.Name),
					IPv4: fmt.Sprintf("http://%s", ip.String()),
				})
			}
		}
	}

	return urls
}

// getMDNSAccessURL returns the mDNS hostname URL (hostname.local)
func getMDNSAccessURL() *dto.AccessURL {
	hostname, err := os.Hostname()
	if err != nil {
		logger.Error("Network Access: Failed to get hostname: %v", err)
		return nil
	}

	// Clean hostname and ensure it's valid for mDNS
	hostname = strings.ToLower(strings.TrimSpace(hostname))
	if hostname == "" {
		return nil
	}

	return &dto.AccessURL{
		Type: dto.URLTypeMDNS,
		Name: "mDNS",
		IPv4: fmt.Sprintf("http://%s.local", hostname),
	}
}

// getWireGuardAccessURLs returns WireGuard VPN IP addresses if configured
func getWireGuardAccessURLs() []dto.AccessURL {
	var urls []dto.AccessURL

	interfaces, err := net.Interfaces()
	if err != nil {
		return urls
	}

	for _, iface := range interfaces {
		// Look for WireGuard interfaces (wg0, wg1, etc.)
		if !strings.HasPrefix(iface.Name, "wg") {
			continue
		}

		if iface.Flags&net.FlagUp == 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}

			ip := ipNet.IP
			if ip.To4() != nil {
				urls = append(urls, dto.AccessURL{
					Type: dto.URLTypeWireGuard,
					Name: fmt.Sprintf("VPN (%s)", iface.Name),
					IPv4: fmt.Sprintf("http://%s", ip.String()),
				})
			} else if ip.To16() != nil && !ip.IsLinkLocalUnicast() {
				urls = append(urls, dto.AccessURL{
					Type: dto.URLTypeWireGuard,
					Name: fmt.Sprintf("VPN (%s IPv6)", iface.Name),
					IPv6: fmt.Sprintf("http://[%s]", ip.String()),
				})
			}
		}
	}

	return urls
}

// wanAccessURL returns the WAN entry. With lookup enabled it asks the WAN
// services in order, first answer wins, and falls back to Unraid's configured
// WANIP; with lookup disabled it only reads the configured WANIP.
func (d *NetworkAccessDiscovery) wanAccessURL() *dto.AccessURL {
	if d.wanLookup {
		for _, service := range d.wanServices {
			if ip := d.queryWANService(service); ip != "" {
				return wanAccessURL(ip)
			}
		}
	}
	if ip := d.configuredWANIP(); ip != "" {
		return wanAccessURL(ip)
	}
	return nil
}

// queryWANService asks one IP service for the public IP and returns it, or ""
// on any failure. The response body is closed before returning.
func (d *NetworkAccessDiscovery) queryWANService(service string) string {
	req, err := http.NewRequest(http.MethodGet, service, nil)
	if err != nil {
		return ""
	}
	//nolint:gosec // G107/G704: URL is from the trusted list of IP services
	resp, err := d.client.Do(req)
	if err != nil {
		return ""
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			logger.Debug("Error closing response body: %v", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return ""
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64))
	if err != nil {
		return ""
	}
	ip := strings.TrimSpace(string(body))
	if net.ParseIP(ip) == nil {
		return ""
	}
	return ip
}

// wanAccessURL builds the WAN access entry for ip.
func wanAccessURL(ip string) *dto.AccessURL {
	return &dto.AccessURL{
		Type: dto.URLTypeWAN,
		Name: "Remote Access (WAN)",
		IPv4: fmt.Sprintf("http://%s", ip),
	}
}

// configuredWANIP returns the WANIP value from Unraid's network.cfg, or "" if
// it is missing or not a valid IP.
func (d *NetworkAccessDiscovery) configuredWANIP() string {
	file, err := os.Open(d.networkCfgPath)
	if err != nil {
		return ""
	}
	defer func() {
		if err := file.Close(); err != nil {
			logger.Debug("Error closing network config file: %v", err)
		}
	}()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		if after, ok := strings.CutPrefix(scanner.Text(), "WANIP="); ok {
			ip := strings.Trim(after, "\"")
			if net.ParseIP(ip) != nil {
				return ip
			}
		}
	}
	return ""
}

// getIPv6AccessURLs returns IPv6 addresses for primary interfaces
func getIPv6AccessURLs() []dto.AccessURL {
	var urls []dto.AccessURL

	interfaces, err := net.Interfaces()
	if err != nil {
		return urls
	}

	for _, iface := range interfaces {
		// Skip loopback and down interfaces
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
			continue
		}

		// Skip virtual interfaces
		if strings.HasPrefix(iface.Name, "docker") ||
			strings.HasPrefix(iface.Name, "br-") ||
			strings.HasPrefix(iface.Name, "veth") ||
			strings.HasPrefix(iface.Name, "virbr") ||
			strings.HasPrefix(iface.Name, "vnet") ||
			strings.HasPrefix(iface.Name, "wg") { // WireGuard handled separately
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}

			ip := ipNet.IP
			// Only process IPv6 addresses
			if ip.To4() != nil {
				continue
			}

			// Skip link-local IPv6 (fe80::)
			if ip.IsLinkLocalUnicast() {
				continue
			}

			// Skip loopback
			if ip.IsLoopback() {
				continue
			}

			// Only include global unicast addresses
			if ip.IsGlobalUnicast() {
				urls = append(urls, dto.AccessURL{
					Type: dto.URLTypeIPv6,
					Name: fmt.Sprintf("IPv6 (%s)", iface.Name),
					IPv6: fmt.Sprintf("http://[%s]", ip.String()),
				})
			}
		}
	}

	return urls
}

// isPrivateIP checks if an IP address is in a private range
func isPrivateIP(ip net.IP) bool {
	privateRanges := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
	}

	for _, cidr := range privateRanges {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}
		if network.Contains(ip) {
			return true
		}
	}

	return false
}

// GetPrimaryLANIP returns the primary LAN IP address
// This is useful for other parts of the system that need the main IP
func GetPrimaryLANIP() string {
	// Try to get the IP by connecting to a known address
	// This doesn't actually make a connection, just determines the route
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		// Fallback: get first non-loopback IP
		return getFirstNonLoopbackIP()
	}
	defer func() {
		if err := conn.Close(); err != nil {
			logger.Debug("Error closing UDP connection: %v", err)
		}
	}()

	localAddr, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok {
		return ""
	}
	return localAddr.IP.String()
}

// getFirstNonLoopbackIP returns the first non-loopback IPv4 address
func getFirstNonLoopbackIP() string {
	interfaces, err := net.Interfaces()
	if err != nil {
		return ""
	}

	for _, iface := range interfaces {
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}

			ip := ipNet.IP.To4()
			if ip != nil && !ip.IsLoopback() {
				return ip.String()
			}
		}
	}

	return ""
}

// GetHTTPSURLs returns HTTPS versions of access URLs if SSL is enabled
// port is the HTTPS port (default 443 or custom)
func GetHTTPSURLs(urls []dto.AccessURL, port int) []dto.AccessURL {
	var httpsURLs []dto.AccessURL

	portSuffix := ""
	if port != 443 {
		portSuffix = fmt.Sprintf(":%d", port)
	}

	for _, url := range urls {
		httpsURL := dto.AccessURL{
			Type: url.Type,
			Name: url.Name + " (HTTPS)",
		}

		if url.IPv4 != "" {
			// Replace http:// with https://
			ipv4 := strings.Replace(url.IPv4, "http://", "https://", 1)
			// Add port if custom port and URL doesn't already have a port after the host
			// Check if there's a colon after "https://"
			if portSuffix != "" {
				hostPart := strings.TrimPrefix(ipv4, "https://")
				if !strings.Contains(hostPart, ":") {
					ipv4 = ipv4 + portSuffix
				}
			}
			httpsURL.IPv4 = ipv4
		}

		if url.IPv6 != "" {
			ipv6 := strings.Replace(url.IPv6, "http://", "https://", 1)
			httpsURL.IPv6 = ipv6
		}

		httpsURLs = append(httpsURLs, httpsURL)
	}

	return httpsURLs
}

// Helper function used by network collector tests
func init() {
	// Register GetPrimaryLANIP for external use
	_ = lib.ExecCommand
}
