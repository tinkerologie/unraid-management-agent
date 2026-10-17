// Package discovery provides zeroconf (mDNS/DNS-SD) advertising so that
// network integrations — such as the Home Assistant integration — can
// auto-discover the Unraid Management Agent on the local network.
package discovery

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"net"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/grandcat/zeroconf"

	"github.com/tinkerologie/unraid-management-agent/daemon/constants"
	"github.com/tinkerologie/unraid-management-agent/daemon/domain"
	"github.com/tinkerologie/unraid-management-agent/daemon/logger"
)

// retryInterval is how often Start retries when no LAN interface is available
// yet, as happens when the agent starts before the network is up at boot.
var retryInterval = 30 * time.Second

// errNoLANInterface means no interface qualifies for advertising.
var errNoLANInterface = errors.New("no LAN interface with a usable address found (docker, VPN and virtual interfaces are ignored)")

// excludedInterfacePrefixes lists interfaces that never face the LAN: Docker
// bridges and veth pairs, LXC, LXD, Incus and Podman bridges, CNI and
// Kubernetes overlays, libvirt bridges and VM taps, Unraid's macvlan/ipvlan
// shims, and VPN or overlay tunnels. It combines the MQTT and access-URL
// filters with container and tunnel prefixes.
var excludedInterfacePrefixes = []string{
	"docker", "br-", "br_", "veth", "lxcbr", "lxdbr", "incusbr", "podman",
	"cni", "flannel", "cali", "cilium", "kube", "virbr", "vnet", "vhost", "tap",
	"macvtap", "shim-", "shim_", "tun", "wg", "tailscale", "zt",
}

// netInterface is a network interface with its addresses.
type netInterface struct {
	iface net.Interface
	addrs []net.IP
}

// advertiser is a registered mDNS service.
type advertiser interface {
	Shutdown()
}

// Injection points for tests.
var (
	listInterfaces = systemInterfaces
	routeIPv4      = primaryIPv4
	netInterfaces  = net.Interfaces
	interfaceAddrs = func(iface net.Interface) ([]net.Addr, error) { return iface.Addrs() }
	registerProxy  = func(instance, service, domain string, port int, host string, ips, text []string, ifaces []net.Interface) (advertiser, error) {
		server, err := zeroconf.RegisterProxy(instance, service, domain, port, host, ips, text, ifaces)
		if err != nil {
			return nil, err
		}
		return server, nil
	}
)

// Service advertises the agent on the local network via mDNS/DNS-SD.
//
// Advertising is best-effort: registration runs alongside any system mDNS
// responder (e.g. Unraid's avahi-daemon) and a failure to register is logged
// but never fatal, mirroring the agent's other optional services.
type Service struct {
	config      domain.DiscoveryConfig
	hostname    string
	port        int
	version     string
	bindAddress string
	tlsEnabled  bool
	authEnabled bool

	mu     sync.Mutex
	server advertiser
	stop   chan struct{} // closed by Shutdown to end retries

	retryDone chan struct{} // closed when the retry goroutine returns
}

// NewService creates a discovery service that will advertise the given metadata.
// hostname is the server's hostname, port is the agent's HTTP port, version is
// the agent version string and bindAddress is the HTTP server's configured bind
// address (empty = all interfaces). When a specific bind address is set it is
// advertised instead of the auto-detected primary IP, so discovery always
// points at the endpoint the server actually listens on. tlsEnabled reports
// whether the agent serves HTTPS, so discovery consumers learn the scheme.
func NewService(config domain.DiscoveryConfig, hostname string, port int, version string, bindAddress string, tlsEnabled bool) *Service {
	return &Service{
		config:      config,
		hostname:    hostname,
		port:        port,
		version:     version,
		bindAddress: bindAddress,
		tlsEnabled:  tlsEnabled,
	}
}

// SetAuthEnabled configures whether the agent requires bearer authentication
// on HTTP and MCP endpoints so mDNS TXT metadata can advertise mcp_auth=bearer|none
// without ever publishing any token or connect secret.
func (s *Service) SetAuthEnabled(enabled bool) {
	s.authEnabled = enabled
}

// instanceName returns the advertised mDNS instance name, preferring the
// configured override and falling back to the system hostname.
func (s *Service) instanceName() string {
	if s.config.ServiceName != "" {
		return s.config.ServiceName
	}
	return s.hostname
}

// txtRecords returns the TXT records published with the service. They give
// integrations rich metadata (version + API path + friendly name + MCP metadata)
// without an extra HTTP round-trip during discovery. Secrets are never included.
func (s *Service) txtRecords() []string {
	scheme := "http"
	if s.tlsEnabled {
		scheme = "https"
	}
	mcpAuth := "none"
	if s.authEnabled {
		mcpAuth = "bearer"
	}
	return []string{
		"version=" + s.version,
		"path=/api/v1",
		"name=" + s.hostname,
		"scheme=" + scheme,
		"mcp_path=/mcp",
		"mcp_transport=streamable-http",
		"mcp_auth=" + mcpAuth,
	}
}

// Start registers the mDNS service on one LAN interface with one address.
// Registration failures are returned, but callers should treat discovery as
// optional and continue. When no LAN interface is available yet, Start logs a
// warning and keeps retrying in the background until ctx is cancelled or
// Shutdown is called, so a boot-time start before the network is up still
// gets advertised later.
func (s *Service) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.server != nil || s.stop != nil {
		return nil // already registered or retrying
	}

	// A loopback bind address is unreachable from the LAN, so advertising
	// any address would mislead integrations. Skip discovery entirely.
	if ip := net.ParseIP(s.bindAddress); ip != nil && ip.IsLoopback() {
		logger.Info("Discovery: HTTP server is bound to loopback (%s); skipping mDNS advertisement", s.bindAddress)
		return nil
	}

	err := s.registerLocked()
	if err == nil {
		return nil
	}
	if !errors.Is(err, errNoLANInterface) {
		return fmt.Errorf("registering mDNS service: %w", err)
	}

	logger.Warning("Discovery: %v; retrying every %v. If this persists, set BIND_ADDRESS to the server's LAN IP.", err, retryInterval)
	s.stop = make(chan struct{})
	s.retryDone = make(chan struct{})
	go s.retry(ctx, s.stop, s.retryDone, retryInterval)
	return nil
}

// retry registers the service once a LAN interface appears.
func (s *Service) retry(ctx context.Context, stop <-chan struct{}, done chan<- struct{}, interval time.Duration) {
	defer close(done)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-stop:
			return
		case <-ticker.C:
		}

		s.mu.Lock()
		select {
		case <-stop:
			s.mu.Unlock()
			return
		default:
		}
		err := s.registerLocked()
		s.mu.Unlock()

		switch {
		case err == nil:
			return
		case !errors.Is(err, errNoLANInterface):
			logger.Warning("Discovery service disabled: registering mDNS service: %v", err)
			return
		}
	}
}

// registerLocked selects the advertised address and registers the service on
// its interface only, so the responder joins the mDNS group and announces
// there. zeroconf still answers a query on whichever interface it arrives
// (another process may have joined the group on a Docker bridge), but every
// answer carries only the selected address. The caller holds s.mu.
func (s *Service) registerLocked() error {
	ip, iface, err := s.selectAdvertisement()
	if err != nil {
		return err
	}
	server, err := registerProxy(
		s.instanceName(),
		constants.DiscoveryServiceType,
		constants.DiscoveryDomain,
		s.port,
		s.hostname, // host whose A record points at the LAN IP
		[]string{ip.String()},
		s.txtRecords(),
		[]net.Interface{iface},
	)
	if err != nil {
		return err
	}
	s.server = server
	logger.Success(
		"Discovery: advertising %q as %s.%s on port %d (ip=%s, interface=%s)",
		s.instanceName(), constants.DiscoveryServiceType, constants.DiscoveryDomain, s.port, ip, iface.Name,
	)
	return nil
}

// selectAdvertisement returns the address to advertise and the interface it
// belongs to. It uses, in order: the configured bind address; the source
// address of the default route, when it is on a LAN interface; the first
// IPv4 address of the preferred LAN interface (br0, then bond*, then eth*,
// then any other).
func (s *Service) selectAdvertisement() (net.IP, net.Interface, error) {
	all, err := listInterfaces()
	if err != nil {
		return nil, net.Interface{}, fmt.Errorf("listing network interfaces: %w", err)
	}
	var lan []netInterface
	for _, ni := range all {
		if isLANInterface(ni.iface) {
			lan = append(lan, ni)
		}
	}
	slices.SortStableFunc(lan, func(a, b netInterface) int {
		return cmp.Or(
			cmp.Compare(interfaceRank(a.iface.Name), interfaceRank(b.iface.Name)),
			strings.Compare(a.iface.Name, b.iface.Name),
		)
	})

	owner := func(ip net.IP) (net.Interface, bool) {
		for _, ni := range lan {
			for _, addr := range ni.addrs {
				if addr.Equal(ip) {
					return ni.iface, true
				}
			}
		}
		return net.Interface{}, false
	}

	// Unspecified addresses (0.0.0.0 / ::) mean "all interfaces" and use the
	// heuristic; loopback is handled in Start.
	if ip := net.ParseIP(s.bindAddress); ip != nil && !ip.IsUnspecified() {
		if iface, ok := owner(ip); ok {
			return ip, iface, nil
		}
		return nil, net.Interface{}, fmt.Errorf("bind address %s: %w", ip, errNoLANInterface)
	}
	if ip := routeIPv4(); ip != nil {
		if iface, ok := owner(ip); ok {
			return ip, iface, nil
		}
	}
	for _, ni := range lan {
		for _, addr := range ni.addrs {
			if v4 := addr.To4(); v4 != nil && v4.IsGlobalUnicast() {
				return v4, ni.iface, nil
			}
		}
	}
	return nil, net.Interface{}, errNoLANInterface
}

// isLANInterface reports whether mDNS may be advertised on iface: it must be
// up, multicast-capable, not loopback or point-to-point, and not a known
// virtual or tunnel interface.
func isLANInterface(iface net.Interface) bool {
	if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagMulticast == 0 {
		return false
	}
	if iface.Flags&(net.FlagLoopback|net.FlagPointToPoint) != 0 {
		return false
	}
	for _, prefix := range excludedInterfacePrefixes {
		if strings.HasPrefix(iface.Name, prefix) {
			return false
		}
	}
	return true
}

// interfaceRank orders LAN interfaces by preference: Unraid's main bridge,
// then bonds, then Ethernet NICs, then anything else.
func interfaceRank(name string) int {
	switch {
	case name == "br0":
		return 0
	case strings.HasPrefix(name, "bond"):
		return 1
	case strings.HasPrefix(name, "eth"):
		return 2
	default:
		return 3
	}
}

// systemInterfaces lists the host's interfaces with their addresses.
func systemInterfaces() ([]netInterface, error) {
	ifaces, err := netInterfaces()
	if err != nil {
		return nil, err
	}
	out := make([]netInterface, 0, len(ifaces))
	for _, iface := range ifaces {
		addrs, err := interfaceAddrs(iface)
		if err != nil {
			continue
		}
		ni := netInterface{iface: iface}
		for _, addr := range addrs {
			if ipNet, ok := addr.(*net.IPNet); ok {
				ni.addrs = append(ni.addrs, ipNet.IP)
			}
		}
		out = append(out, ni)
	}
	return out, nil
}

// primaryIPv4 returns the host's primary outbound IPv4 address — the source
// address the kernel would use to reach an off-link destination (i.e. the
// default-route interface). It returns nil if the address cannot be
// determined. No packets are sent: a UDP "connection" only resolves the route
// and local source address.
func primaryIPv4() net.IP {
	// 192.0.2.1 is TEST-NET-1 (RFC 5737); it is never routed, so this performs
	// route resolution only without generating traffic or requiring internet.
	conn, err := net.Dial("udp4", "192.0.2.1:9")
	if err != nil {
		return nil
	}
	defer func() { _ = conn.Close() }()

	addr, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok || addr.IP == nil {
		return nil
	}
	ip := addr.IP.To4()
	if ip == nil || ip.IsLoopback() || ip.IsUnspecified() {
		return nil
	}
	return ip
}

// Shutdown stops advertising the service, sending mDNS goodbye packets so
// clients can remove the entry promptly.
func (s *Service) Shutdown() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.stop != nil {
		close(s.stop)
		s.stop = nil
	}
	if s.server == nil {
		return
	}
	s.server.Shutdown()
	s.server = nil
}
