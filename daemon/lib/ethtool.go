package lib

import (
	"fmt"
	"strconv"
	"strings"
)

// EthtoolInfo contains parsed ethtool information for a network interface
type EthtoolInfo struct {
	SupportedPorts       []string
	SupportedLinkModes   []string
	SupportedPauseFrame  string
	SupportsAutoNeg      bool
	SupportedFECModes    []string
	AdvertisedLinkModes  []string
	AdvertisedPauseFrame string
	AdvertisedAutoNeg    bool
	AdvertisedFECModes   []string
	Duplex               string
	AutoNegotiation      string
	Port                 string
	PHYAD                int
	Transceiver          string
	MDIX                 string
	SupportsWakeOn       []string
	WakeOn               string
	MessageLevel         string
	LinkDetected         bool
	MTU                  int
}

// ParseEthtool parses ethtool output for a network interface
func ParseEthtool(ifName string) (*EthtoolInfo, error) {
	// Check if ethtool is available
	if !CommandExists("ethtool") {
		return nil, fmt.Errorf("ethtool command not found")
	}

	output, err := ExecCommandOutput("ethtool", ifName)
	if err != nil {
		return nil, fmt.Errorf("failed to execute ethtool: %w", err)
	}

	info := &EthtoolInfo{}
	lines := strings.Split(output, "\n")

	var inSupportedLinkModes bool
	var inAdvertisedLinkModes bool

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		// Parse key-value pairs
		if strings.Contains(trimmed, ":") {
			parts := strings.SplitN(trimmed, ":", 2)
			if len(parts) != 2 {
				continue
			}

			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])

			inSupportedLinkModes, inAdvertisedLinkModes = parseEthtoolKeyValue(
				info, key, value, inSupportedLinkModes, inAdvertisedLinkModes,
			)
		} else {
			// Handle multi-line values (link modes)
			if inSupportedLinkModes && trimmed != "" {
				info.SupportedLinkModes = append(info.SupportedLinkModes, trimmed)
			} else if inAdvertisedLinkModes && trimmed != "" {
				info.AdvertisedLinkModes = append(info.AdvertisedLinkModes, trimmed)
			}
		}
	}

	return info, nil
}

// parseEthtoolKeyValue parses a single key-value pair from ethtool output
func parseEthtoolKeyValue(info *EthtoolInfo, key, value string, inSupported, inAdvertised bool) (bool, bool) {
	switch key {
	case "Supported ports":
		info.SupportedPorts = parseListValue(value)
	case "Supported link modes":
		inSupported = true
		inAdvertised = false
		if value != "" {
			info.SupportedLinkModes = append(info.SupportedLinkModes, value)
		}
	case "Supported pause frame use":
		info.SupportedPauseFrame = value
	case "Supports auto-negotiation":
		info.SupportsAutoNeg = value == "Yes"
	case "Supported FEC modes":
		info.SupportedFECModes = parseListValue(value)
	case "Advertised link modes":
		inAdvertised = true
		inSupported = false
		if value != "" {
			info.AdvertisedLinkModes = append(info.AdvertisedLinkModes, value)
		}
	case "Advertised pause frame use":
		info.AdvertisedPauseFrame = value
	case "Advertised auto-negotiation":
		info.AdvertisedAutoNeg = value == "Yes"
	case "Advertised FEC modes":
		info.AdvertisedFECModes = parseListValue(value)
	case "Speed":
		// Speed is already parsed elsewhere, skip
	case "Duplex":
		info.Duplex = value
	case "Auto-negotiation":
		info.AutoNegotiation = value
	case "Port":
		info.Port = value
	case "PHYAD":
		if phyad, err := strconv.Atoi(value); err == nil {
			info.PHYAD = phyad
		}
	case "Transceiver":
		info.Transceiver = value
	case "MDI-X":
		info.MDIX = value
	case "Supports Wake-on":
		info.SupportsWakeOn = parseWakeOnFlags(value)
	case "Wake-on":
		info.WakeOn = value
	case "Current message level":
		info.MessageLevel = value
	case "Link detected":
		info.LinkDetected = value == "yes"
	}
	return inSupported, inAdvertised
}

// parseListValue parses a comma-separated or space-separated list
func parseListValue(value string) []string {
	if value == "" || value == "Not reported" {
		return nil
	}

	var result []string
	if strings.Contains(value, ",") {
		parts := strings.Split(value, ",")
		for _, part := range parts {
			trimmed := strings.TrimSpace(part)
			if trimmed != "" {
				result = append(result, trimmed)
			}
		}
	} else {
		parts := strings.Fields(value)
		result = append(result, parts...)
	}

	return result
}

// parseWakeOnFlags parses Wake-on-LAN flags
func parseWakeOnFlags(value string) []string {
	if value == "" || value == "Not supported" {
		return nil
	}

	var flags []string
	for _, char := range value {
		switch char {
		case 'p':
			flags = append(flags, "PHY activity")
		case 'u':
			flags = append(flags, "Unicast")
		case 'm':
			flags = append(flags, "Multicast")
		case 'b':
			flags = append(flags, "Broadcast")
		case 'a':
			flags = append(flags, "ARP")
		case 'g':
			flags = append(flags, "MagicPacket")
		case 's':
			flags = append(flags, "SecureOn password")
		case 'd':
			flags = append(flags, "Disabled")
		}
	}

	return flags
}
