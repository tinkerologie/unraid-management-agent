package communityapps

import (
	"context"
	"net"
	"net/netip"
	"path"
	"slices"
	"strconv"
	"strings"

	"github.com/moby/moby/api/types/network"
)

func (settings *installSettings) resolveFields(host installHost, request PrepareRequest) []string {
	config := &settings.configuration
	fields := settings.template.Config
	keys := make(map[string]bool, len(fields))
	var issues []string
	for _, field := range fields {
		key := string(field.Kind) + ":" + field.Target
		if field.Kind == ConfigPort {
			key += "/" + strings.ToLower(field.Mode)
		}
		if keys[key] {
			issues = append(issues, "duplicate_template_setting")
			continue
		}
		keys[key] = true
		reason := settings.resolveField(host, field, request)
		if reason != "" && !slices.Contains(issues, reason) {
			issues = append(issues, reason)
		}
	}

	for _, implicit := range []environmentSetting{
		{name: "TZ", value: host.timezone},
		{name: "HOST_OS", value: "Unraid"},
		{name: "HOST_HOSTNAME", value: host.hostname},
		{name: "HOST_CONTAINERNAME", value: config.Name},
	} {
		// Native Docker Manager appends publisher Variables after implicit
		// values. The declared setting wins if it uses the same name.
		if !keys[string(ConfigEnvironment)+":"+implicit.name] {
			settings.addEnvironment(implicit.name, implicit.value)
		}
	}
	return issues
}

func (settings *installSettings) resolveField(host installHost, field TemplateConfig, request PrepareRequest) string {
	config := &settings.configuration
	value := field.Value
	if value == "" {
		value = field.Default
	}
	switch field.Kind {
	case ConfigPort:
		port, reason := resolvePort(field, value, request.Ports)
		if reason == "" {
			config.Ports = append(config.Ports, port)
		}
		return reason
	case ConfigPath:
		if isDockerSocket(field.Target) {
			return "docker_socket_mount"
		}
		if !validBindPath(field.Target) || path.Clean(field.Target) != field.Target || strings.ContainsAny(field.Target, "\"'&") {
			return "unsafe_mount_target"
		}
		if override, ok := request.Paths[field.Target]; ok {
			value = override.Source
		} else {
			value = host.defaultSource(field.Target, config.Name, value)
		}
		if value == "" {
			if field.Required {
				return "missing_required_path"
			}
			return ""
		}
		m, identity, reason := host.resolveSource(value)
		if reason != "" {
			return reason
		}
		mode := strings.ToLower(field.Mode)
		if mode == "" {
			mode = "rw"
		}
		if mode != "rw" && mode != "ro" {
			return "unsupported_mount_mode"
		}
		m.Target, m.Mode = field.Target, mode
		config.Mounts = append(config.Mounts, m)
		settings.mountIdentities = append(settings.mountIdentities, identity)
	case ConfigEnvironment:
		if override, ok := request.Environment[field.Target]; ok {
			value = override
		} else if strings.Contains(field.Default, "|") {
			// Native WebGUI selects the saved option when it matches and
			// presents the first choice otherwise.
			options := strings.Split(field.Default, "|")
			value = options[0]
			if slices.Contains(options, field.Value) {
				value = field.Value
			}
		}
		if !envName.MatchString(field.Target) {
			return "invalid_environment_name"
		}
		if len(value) > 4096 || !nativeValue(value) {
			return "unsupported_environment_value"
		}
		if field.Required && value == "" {
			return "missing_required_environment"
		}
		settings.addEnvironment(field.Target, value)
	}
	return ""
}

func (settings *installSettings) addEnvironment(name, value string) {
	settings.environment = append(settings.environment, environmentSetting{name: name, value: value})
	settings.configuration.Environment = append(settings.configuration.Environment, EnvironmentField{Name: name, Set: value != ""})
}

func resolvePort(field TemplateConfig, value string, overrides map[string]PortOverride) (PortBinding, string) {
	protocol := strings.ToLower(field.Mode)
	target, err := strconv.ParseUint(field.Target, 10, 16)
	if err != nil || target == 0 || (protocol != "tcp" && protocol != "udp") || strconv.FormatUint(target, 10) != field.Target {
		return PortBinding{}, "unsupported_template_port"
	}
	host, address := value, ""
	if strings.Contains(host, ":") {
		address, host, err = net.SplitHostPort(host)
		if err != nil {
			address = ""
		}
	}
	if override, ok := overrides[field.Target+"/"+protocol]; ok {
		host = strconv.Itoa(int(override.HostPort))
		if override.BindAddress != "" {
			address = override.BindAddress
		}
	} else if err != nil {
		return PortBinding{}, "unsupported_port_binding"
	}
	fixedPort, err := strconv.ParseUint(host, 10, 16)
	if err != nil || fixedPort == 0 {
		return PortBinding{}, "fixed_port_required"
	}
	if address != "" {
		bindAddress, err := netip.ParseAddr(address)
		if err != nil || !validBindAddress(bindAddress) {
			return PortBinding{}, "unsupported_port_binding"
		}
		address = bindAddress.String()
	}
	return PortBinding{
		BindAddress: address, HostPort: uint16(fixedPort), Protocol: protocol, ContainerPort: uint16(target),
	}, ""
}

func validBindAddress(addr netip.Addr) bool {
	return addr.IsValid() && !addr.Is4In6() && addr.Zone() == "" && !addr.IsMulticast() &&
		(addr.IsUnspecified() || addr.IsGlobalUnicast() || addr.IsLoopback())
}

func assignBindAddresses(ports []PortBinding, n network.Network) bool {
	defaultAddress := n.Options["com.docker.network.bridge.host_binding_ipv4"]
	if defaultAddress == "" {
		defaultAddress = "0.0.0.0"
	}
	for i, binding := range ports {
		if binding.BindAddress != "" {
			continue
		}
		address, err := netip.ParseAddr(defaultAddress)
		if err != nil || !validBindAddress(address) {
			return false
		}
		ports[i].BindAddress = address.String()
	}
	return true
}

func checkDuplicateBindings(ports []PortBinding) error {
	for i, p := range ports {
		for _, other := range ports[:i] {
			if portOverlaps(p, netip.MustParseAddr(other.BindAddress), other.HostPort, other.Protocol) {
				return ErrPortConflict
			}
		}
	}
	return nil
}

func hasNetworkFamily(n network.Network, ipv6 bool) bool {
	if ipv6 && n.EnableIPv6 || !ipv6 && n.EnableIPv4 {
		return true
	}
	// Older Engine APIs omit EnableIPv4. A configured IPAM subnet is
	// positive evidence that the bridge supports this address family.
	for _, subnet := range n.IPAM.Config {
		if subnet.Subnet.IsValid() && subnet.Subnet.Addr().Is6() == ipv6 {
			return true
		}
	}
	return false
}

func supportsFixedBindings(ports []PortBinding, n network.Network) bool {
	for _, p := range ports {
		family := "ipv4"
		enabled := hasNetworkFamily(n, false)
		if netip.MustParseAddr(p.BindAddress).Is6() {
			family, enabled = "ipv6", hasNetworkFamily(n, true)
		}
		gatewayMode := n.Options["com.docker.network.bridge.gateway_mode_"+family]
		if !enabled || gatewayMode != "" && gatewayMode != "nat" {
			// Routed networks can ignore the approved host-port binding.
			return false
		}
	}
	return true
}

func portOverlaps(wanted PortBinding, addr netip.Addr, port uint16, proto string) bool {
	if wanted.HostPort != port || wanted.Protocol != proto {
		return false
	}
	if !addr.IsValid() {
		return true
	}
	addr = addr.Unmap()
	bind := netip.MustParseAddr(wanted.BindAddress)
	if bind.BitLen() != addr.BitLen() {
		return false
	}
	return bind == addr || bind.IsUnspecified() || addr.IsUnspecified()
}

func probePort(ctx context.Context, p PortBinding) error {
	family := "4"
	if netip.MustParseAddr(p.BindAddress).Is6() {
		family = "6"
	}
	address := net.JoinHostPort(p.BindAddress, strconv.Itoa(int(p.HostPort)))
	lc := net.ListenConfig{}
	if p.Protocol == "udp" {
		listener, err := lc.ListenPacket(ctx, "udp"+family, address)
		if err != nil {
			return err
		}
		return listener.Close()
	}
	listener, err := lc.Listen(ctx, "tcp"+family, address)
	if err != nil {
		return err
	}
	return listener.Close()
}
