// Package discovery provides zeroconf (mDNS/DNS-SD) advertising so that
// network integrations — such as the Home Assistant integration — can
// auto-discover the Unraid Management Agent on the local network.
package discovery

import (
	"context"
	"fmt"
	"net"
	"sync"

	"github.com/grandcat/zeroconf"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/constants"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
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

	mu     sync.Mutex
	server *zeroconf.Server
}

// NewService creates a discovery service that will advertise the given metadata.
// hostname is the server's hostname, port is the agent's HTTP port, version is
// the agent version string and bindAddress is the HTTP server's configured bind
// address (empty = all interfaces). When a specific bind address is set it is
// advertised instead of the auto-detected primary IP, so discovery always
// points at the endpoint the server actually listens on.
func NewService(config domain.DiscoveryConfig, hostname string, port int, version string, bindAddress string) *Service {
	return &Service{
		config:      config,
		hostname:    hostname,
		port:        port,
		version:     version,
		bindAddress: bindAddress,
	}
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
// integrations rich metadata (version + API path + friendly name) without an
// extra HTTP round-trip during discovery.
func (s *Service) txtRecords() []string {
	return []string{
		"version=" + s.version,
		"path=/api/v1",
		"name=" + s.hostname,
	}
}

// Start registers the mDNS service. Registration failures are logged and
// returned, but callers should treat discovery as optional and continue.
//
// The context parameter is accepted for lifecycle-signature consistency with
// the agent's other services. zeroconf registration is not context-cancellable;
// teardown is handled explicitly via Shutdown during graceful shutdown.
func (s *Service) Start(_ context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.server != nil {
		return nil // already registered
	}

	// A loopback bind address is unreachable from the LAN, so advertising
	// any address would mislead integrations. Skip discovery entirely.
	if ip := net.ParseIP(s.bindAddress); ip != nil && ip.IsLoopback() {
		logger.Info("Discovery: HTTP server is bound to loopback (%s); skipping mDNS advertisement", s.bindAddress)
		return nil
	}

	server, advertised, err := s.register()
	if err != nil {
		return fmt.Errorf("registering mDNS service: %w", err)
	}

	s.server = server
	logger.Success(
		"Discovery: advertising %q as %s.%s on port %d (%s)",
		s.instanceName(), constants.DiscoveryServiceType, constants.DiscoveryDomain, s.port, advertised,
	)
	return nil
}

// register registers the service with zeroconf. When an advertise address is
// known (the configured bind address, or the detected primary LAN IPv4) it is
// advertised explicitly via RegisterProxy, so a single, reachable address is
// published regardless of how many (docker/virtual) interfaces the host has.
// Otherwise it falls back to Register, which derives addresses from the
// interface a query arrives on. The returned string describes the advertised
// address(es) for logging.
func (s *Service) register() (*zeroconf.Server, string, error) {
	if ip := s.advertiseIP(); ip != nil {
		server, err := zeroconf.RegisterProxy(
			s.instanceName(),
			constants.DiscoveryServiceType,
			constants.DiscoveryDomain,
			s.port,
			s.hostname, // host whose A record points at the LAN IP
			[]string{ip.String()},
			s.txtRecords(),
			nil, // respond on all interfaces; the explicit IP is always returned
		)
		if err != nil {
			return nil, "", err
		}
		return server, "ip=" + ip.String(), nil
	}

	server, err := zeroconf.Register(
		s.instanceName(),
		constants.DiscoveryServiceType,
		constants.DiscoveryDomain,
		s.port,
		s.txtRecords(),
		nil, // nil interfaces => advertise on all suitable interfaces
	)
	if err != nil {
		return nil, "", err
	}
	return server, "all interfaces", nil
}

// advertiseIP returns the IP to advertise via mDNS: the configured bind
// address when it is a specific, LAN-reachable IP, otherwise the detected
// primary outbound IPv4. Unspecified addresses (0.0.0.0 / ::) mean "all
// interfaces" and defer to the heuristic; loopback is handled in Start.
func (s *Service) advertiseIP() net.IP {
	if ip := net.ParseIP(s.bindAddress); ip != nil && !ip.IsUnspecified() && !ip.IsLoopback() {
		return ip
	}
	return primaryIPv4()
}

// primaryIPv4 returns the host's primary outbound IPv4 address — the source
// address the kernel would use to reach an off-link destination (i.e. the
// default-route interface). This avoids advertising docker/virtual bridge
// addresses that are unreachable from the rest of the LAN. It returns nil if
// the address cannot be determined. No packets are sent: a UDP "connection"
// only resolves the route and local source address.
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

	if s.server == nil {
		return
	}
	s.server.Shutdown()
	s.server = nil
}
