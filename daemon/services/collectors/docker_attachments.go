package collectors

import (
	"maps"
	"slices"

	"github.com/moby/moby/api/types/network"

	"github.com/tinkerologie/unraid-management-agent/daemon/dto"
)

func buildNetworkAttachments(endpoints map[string]*network.EndpointSettings) []dto.ContainerNetworkAttachment {
	if len(endpoints) == 0 {
		return nil
	}
	attachments := make([]dto.ContainerNetworkAttachment, 0, len(endpoints))
	for _, name := range slices.Sorted(maps.Keys(endpoints)) {
		endpoint := endpoints[name]
		if endpoint == nil {
			continue
		}
		attachment := dto.ContainerNetworkAttachment{
			Network: name, NetworkID: endpoint.NetworkID, MACAddress: endpoint.MacAddress.String(),
		}
		if endpoint.IPAddress.Is4() {
			attachment.IPAddress = endpoint.IPAddress.String()
		}
		if endpoint.GlobalIPv6Address.Is6() {
			attachment.IPv6Address = endpoint.GlobalIPv6Address.String()
		}
		attachments = append(attachments, attachment)
	}
	return attachments
}

func selectLegacyEndpoint(mode string, attachments []dto.ContainerNetworkAttachment) (ip, mac string) {
	if mode == "default" {
		mode = "bridge"
	}
	var primary dto.ContainerNetworkAttachment
	var firstMAC string
	for _, attachment := range attachments {
		if mode != "" && (mode == attachment.Network || mode == attachment.NetworkID) {
			primary = attachment
		}
		if firstMAC == "" {
			firstMAC = attachment.MACAddress
		}
		if ip == "" && attachment.IPAddress != "" {
			ip, mac = attachment.IPAddress, attachment.MACAddress
		}
	}
	if primary.IPAddress != "" {
		ip, mac = primary.IPAddress, primary.MACAddress
	}
	if mac == "" {
		mac = primary.MACAddress
		if mac == "" {
			mac = firstMAC
		}
	}
	return ip, mac
}
