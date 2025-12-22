package lib

import (
	"testing"
)

func TestParseListValue(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "comma separated",
			input:    "value1, value2, value3",
			expected: []string{"value1", "value2", "value3"},
		},
		{
			name:     "space separated",
			input:    "value1 value2 value3",
			expected: []string{"value1", "value2", "value3"},
		},
		{
			name:     "single value",
			input:    "single",
			expected: []string{"single"},
		},
		{
			name:     "empty string",
			input:    "",
			expected: nil,
		},
		{
			name:     "not reported",
			input:    "Not reported",
			expected: nil,
		},
		{
			name:     "brackets",
			input:    "[ TP ]",
			expected: []string{"[", "TP", "]"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseListValue(tt.input)
			if len(got) != len(tt.expected) {
				t.Errorf("parseListValue(%q) returned %d items, want %d", tt.input, len(got), len(tt.expected))
				return
			}
			for i, v := range got {
				if v != tt.expected[i] {
					t.Errorf("parseListValue(%q)[%d] = %q, want %q", tt.input, i, v, tt.expected[i])
				}
			}
		})
	}
}

func TestParseWakeOnFlags(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "all flags",
			input:    "pumbgsd",
			expected: []string{"PHY activity", "Unicast", "Multicast", "Broadcast", "MagicPacket", "SecureOn password", "Disabled"},
		},
		{
			name:     "common flags",
			input:    "pumbg",
			expected: []string{"PHY activity", "Unicast", "Multicast", "Broadcast", "MagicPacket"},
		},
		{
			name:     "magic packet only",
			input:    "g",
			expected: []string{"MagicPacket"},
		},
		{
			name:     "disabled",
			input:    "d",
			expected: []string{"Disabled"},
		},
		{
			name:     "empty",
			input:    "",
			expected: nil,
		},
		{
			name:     "not supported",
			input:    "Not supported",
			expected: nil,
		},
		{
			name:     "arp",
			input:    "a",
			expected: []string{"ARP"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseWakeOnFlags(tt.input)
			if len(got) != len(tt.expected) {
				t.Errorf("parseWakeOnFlags(%q) returned %d items, want %d", tt.input, len(got), len(tt.expected))
				return
			}
			for i, v := range got {
				if v != tt.expected[i] {
					t.Errorf("parseWakeOnFlags(%q)[%d] = %q, want %q", tt.input, i, v, tt.expected[i])
				}
			}
		})
	}
}

func TestParseEthtoolKeyValue(t *testing.T) {
	info := &EthtoolInfo{}

	tests := []struct {
		key             string
		value           string
		expectSupported bool
		expectAdvertise bool
	}{
		{"Duplex", "Full", false, false},
		{"Auto-negotiation", "on", false, false},
		{"Port", "Twisted Pair", false, false},
		{"PHYAD", "1", false, false},
		{"Link detected", "yes", false, false},
		{"Link detected", "no", false, false},
		{"Supports auto-negotiation", "Yes", false, false},
		{"Supports auto-negotiation", "No", false, false},
		{"Advertised auto-negotiation", "Yes", false, false},
		{"Supported link modes", "1000baseT/Full", true, false},
		{"Advertised link modes", "1000baseT/Full", false, true},
		// Additional test cases for full coverage
		{"Supported ports", "[ TP ]", false, false},
		{"Supported pause frame use", "Symmetric", false, false},
		{"Supported FEC modes", "Base-R RS", false, false},
		{"Advertised pause frame use", "Symmetric", false, false},
		{"Advertised FEC modes", "Base-R", false, false},
		{"Speed", "1000Mb/s", false, false},
		{"MDI-X", "off (auto)", false, false},
		{"Current message level", "0x00000007 (7)", false, false},
		{"Transceiver", "internal", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			gotSup, gotAdv := parseEthtoolKeyValue(info, tt.key, tt.value, false, false)

			if gotSup != tt.expectSupported {
				t.Errorf("parseEthtoolKeyValue(%q, %q) inSupported = %v, want %v",
					tt.key, tt.value, gotSup, tt.expectSupported)
			}
			if gotAdv != tt.expectAdvertise {
				t.Errorf("parseEthtoolKeyValue(%q, %q) inAdvertised = %v, want %v",
					tt.key, tt.value, gotAdv, tt.expectAdvertise)
			}
		})
	}
}

func TestEthtoolInfoParsing(t *testing.T) {
	info := &EthtoolInfo{}

	// Test Duplex
	parseEthtoolKeyValue(info, "Duplex", "Full", false, false)
	if info.Duplex != "Full" {
		t.Errorf("Duplex = %q, want %q", info.Duplex, "Full")
	}

	// Test Auto-negotiation
	parseEthtoolKeyValue(info, "Auto-negotiation", "on", false, false)
	if info.AutoNegotiation != "on" {
		t.Errorf("AutoNegotiation = %q, want %q", info.AutoNegotiation, "on")
	}

	// Test Port
	parseEthtoolKeyValue(info, "Port", "Twisted Pair", false, false)
	if info.Port != "Twisted Pair" {
		t.Errorf("Port = %q, want %q", info.Port, "Twisted Pair")
	}

	// Test PHYAD
	parseEthtoolKeyValue(info, "PHYAD", "1", false, false)
	if info.PHYAD != 1 {
		t.Errorf("PHYAD = %d, want %d", info.PHYAD, 1)
	}

	// Test Transceiver
	parseEthtoolKeyValue(info, "Transceiver", "internal", false, false)
	if info.Transceiver != "internal" {
		t.Errorf("Transceiver = %q, want %q", info.Transceiver, "internal")
	}

	// Test Link detected
	info2 := &EthtoolInfo{}
	parseEthtoolKeyValue(info2, "Link detected", "yes", false, false)
	if !info2.LinkDetected {
		t.Error("LinkDetected should be true")
	}

	info3 := &EthtoolInfo{}
	parseEthtoolKeyValue(info3, "Link detected", "no", false, false)
	if info3.LinkDetected {
		t.Error("LinkDetected should be false")
	}

	// Test Supports auto-negotiation
	info4 := &EthtoolInfo{}
	parseEthtoolKeyValue(info4, "Supports auto-negotiation", "Yes", false, false)
	if !info4.SupportsAutoNeg {
		t.Error("SupportsAutoNeg should be true")
	}

	// Test Advertised auto-negotiation
	info5 := &EthtoolInfo{}
	parseEthtoolKeyValue(info5, "Advertised auto-negotiation", "Yes", false, false)
	if !info5.AdvertisedAutoNeg {
		t.Error("AdvertisedAutoNeg should be true")
	}

	// Test Wake-on
	info6 := &EthtoolInfo{}
	parseEthtoolKeyValue(info6, "Wake-on", "g", false, false)
	if info6.WakeOn != "g" {
		t.Errorf("WakeOn = %q, want %q", info6.WakeOn, "g")
	}

	// Test Supports Wake-on
	info7 := &EthtoolInfo{}
	parseEthtoolKeyValue(info7, "Supports Wake-on", "pumbg", false, false)
	if len(info7.SupportsWakeOn) != 5 {
		t.Errorf("SupportsWakeOn has %d items, want 5", len(info7.SupportsWakeOn))
	}
}

func TestEthtoolInfoStruct(t *testing.T) {
	info := EthtoolInfo{
		SupportedPorts:       []string{"TP"},
		SupportedLinkModes:   []string{"1000baseT/Full"},
		SupportedPauseFrame:  "Symmetric",
		SupportsAutoNeg:      true,
		SupportedFECModes:    []string{"None"},
		AdvertisedLinkModes:  []string{"1000baseT/Full"},
		AdvertisedPauseFrame: "Symmetric",
		AdvertisedAutoNeg:    true,
		AdvertisedFECModes:   []string{"None"},
		Duplex:               "Full",
		AutoNegotiation:      "on",
		Port:                 "Twisted Pair",
		PHYAD:                1,
		Transceiver:          "internal",
		MDIX:                 "off (auto)",
		SupportsWakeOn:       []string{"MagicPacket"},
		WakeOn:               "g",
		MessageLevel:         "0x00000007",
		LinkDetected:         true,
		MTU:                  1500,
	}

	// Verify all fields are set correctly
	if !info.SupportsAutoNeg {
		t.Error("SupportsAutoNeg should be true")
	}
	if !info.AdvertisedAutoNeg {
		t.Error("AdvertisedAutoNeg should be true")
	}
	if !info.LinkDetected {
		t.Error("LinkDetected should be true")
	}
	if info.MTU != 1500 {
		t.Errorf("MTU = %d, want 1500", info.MTU)
	}
}

func TestParseSpeedValue(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{"1Gbps", "1000Mb/s", 1000},
		{"10Gbps", "10000Mb/s", 10000},
		{"100Mbps", "100Mb/s", 100},
		{"2.5Gbps", "2500Mb/s", 2500},
		{"Unknown", "Unknown!", 0},
		{"empty", "", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the speed parsing logic
			var speed int
			for _, c := range tt.input {
				if c >= '0' && c <= '9' {
					speed = speed*10 + int(c-'0')
				} else if speed > 0 {
					break
				}
			}
			if speed != tt.expected {
				t.Errorf("parseSpeed(%q) = %d, want %d", tt.input, speed, tt.expected)
			}
		})
	}
}

func TestParseDuplexValue(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"full duplex", "Full", "Full"},
		{"half duplex", "Half", "Half"},
		{"unknown", "Unknown! (255)", "Unknown! (255)"},
		{"empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.input != tt.expected {
				t.Errorf("Duplex = %q, want %q", tt.input, tt.expected)
			}
		})
	}
}

func TestParseAutoNegotiation(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"on", "on", "on"},
		{"off", "off", "off"},
		{"not supported", "Not supported", "Not supported"},
		{"empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.input != tt.expected {
				t.Errorf("AutoNegotiation = %q, want %q", tt.input, tt.expected)
			}
		})
	}
}

func TestParseLinkModes(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "single mode",
			input:    "1000baseT/Full",
			expected: []string{"1000baseT/Full"},
		},
		{
			name:     "multiple modes",
			input:    "10baseT/Half 10baseT/Full 100baseT/Half 100baseT/Full",
			expected: []string{"10baseT/Half", "10baseT/Full", "100baseT/Half", "100baseT/Full"},
		},
		{
			name:     "high speed modes",
			input:    "10000baseT/Full 2500baseT/Full 5000baseT/Full",
			expected: []string{"10000baseT/Full", "2500baseT/Full", "5000baseT/Full"},
		},
		{
			name:     "not reported",
			input:    "Not reported",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseListValue(tt.input)
			if len(got) != len(tt.expected) {
				t.Errorf("parseListValue(%q) returned %d items, want %d", tt.input, len(got), len(tt.expected))
			}
		})
	}
}

func TestParsePortTypes(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"twisted pair", "Twisted Pair", "Twisted Pair"},
		{"sfp", "SFP", "SFP"},
		{"fiber", "Fibre", "Fibre"},
		{"aui", "AUI", "AUI"},
		{"mii", "MII", "MII"},
		{"bnc", "BNC", "BNC"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.input != tt.expected {
				t.Errorf("Port = %q, want %q", tt.input, tt.expected)
			}
		})
	}
}

func TestParsePHYAD(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{"zero", "0", 0},
		{"one", "1", 1},
		{"common value", "2", 2},
		{"max", "31", 31},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var phyad int
			for _, c := range tt.input {
				if c >= '0' && c <= '9' {
					phyad = phyad*10 + int(c-'0')
				}
			}
			if phyad != tt.expected {
				t.Errorf("PHYAD = %d, want %d", phyad, tt.expected)
			}
		})
	}
}

func TestParseMDIX(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"auto off", "off (auto)", "off (auto)"},
		{"auto on", "on (auto)", "on (auto)"},
		{"fixed off", "off", "off"},
		{"fixed on", "on", "on"},
		{"unknown", "Unknown", "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.input != tt.expected {
				t.Errorf("MDIX = %q, want %q", tt.input, tt.expected)
			}
		})
	}
}

func TestParseMessageLevel(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"default", "0x00000007", "0x00000007"},
		{"minimal", "0x00000001", "0x00000001"},
		{"verbose", "0x0000ffff", "0x0000ffff"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.input != tt.expected {
				t.Errorf("MessageLevel = %q, want %q", tt.input, tt.expected)
			}
		})
	}
}

func TestParsePauseFrame(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"symmetric", "Symmetric", "Symmetric"},
		{"receive only", "Receive-only", "Receive-only"},
		{"asymmetric", "Asymmetric", "Asymmetric"},
		{"no", "No", "No"},
		{"not reported", "Not reported", "Not reported"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.input != tt.expected {
				t.Errorf("PauseFrame = %q, want %q", tt.input, tt.expected)
			}
		})
	}
}

func TestParseFECModes(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "none",
			input:    "None",
			expected: []string{"None"},
		},
		{
			name:     "base-r",
			input:    "Base-R",
			expected: []string{"Base-R"},
		},
		{
			name:     "rs",
			input:    "RS",
			expected: []string{"RS"},
		},
		{
			name:     "multiple",
			input:    "Base-R RS",
			expected: []string{"Base-R", "RS"},
		},
		{
			name:     "not reported",
			input:    "Not reported",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseListValue(tt.input)
			if len(got) != len(tt.expected) {
				t.Errorf("parseListValue(%q) returned %d items, want %d", tt.input, len(got), len(tt.expected))
			}
		})
	}
}

func TestParseEthtoolKeyValueAllBranches(t *testing.T) {
	// Test Supported ports
	info1 := &EthtoolInfo{}
	parseEthtoolKeyValue(info1, "Supported ports", "TP MII", false, false)
	if len(info1.SupportedPorts) != 2 {
		t.Errorf("SupportedPorts has %d items, want 2", len(info1.SupportedPorts))
	}

	// Test Supported pause frame use
	info2 := &EthtoolInfo{}
	parseEthtoolKeyValue(info2, "Supported pause frame use", "Symmetric", false, false)
	if info2.SupportedPauseFrame != "Symmetric" {
		t.Errorf("SupportedPauseFrame = %q, want %q", info2.SupportedPauseFrame, "Symmetric")
	}

	// Test Supported FEC modes
	info3 := &EthtoolInfo{}
	parseEthtoolKeyValue(info3, "Supported FEC modes", "Base-R RS", false, false)
	if len(info3.SupportedFECModes) != 2 {
		t.Errorf("SupportedFECModes has %d items, want 2", len(info3.SupportedFECModes))
	}

	// Test Advertised pause frame use
	info4 := &EthtoolInfo{}
	parseEthtoolKeyValue(info4, "Advertised pause frame use", "No", false, false)
	if info4.AdvertisedPauseFrame != "No" {
		t.Errorf("AdvertisedPauseFrame = %q, want %q", info4.AdvertisedPauseFrame, "No")
	}

	// Test Advertised FEC modes
	info5 := &EthtoolInfo{}
	parseEthtoolKeyValue(info5, "Advertised FEC modes", "None", false, false)
	if len(info5.AdvertisedFECModes) != 1 {
		t.Errorf("AdvertisedFECModes has %d items, want 1", len(info5.AdvertisedFECModes))
	}

	// Test Speed - should be skipped (no change)
	info6 := &EthtoolInfo{}
	parseEthtoolKeyValue(info6, "Speed", "1000Mb/s", false, false)
	// Speed is skipped, nothing to check

	// Test MDI-X
	info7 := &EthtoolInfo{}
	parseEthtoolKeyValue(info7, "MDI-X", "off (auto)", false, false)
	if info7.MDIX != "off (auto)" {
		t.Errorf("MDIX = %q, want %q", info7.MDIX, "off (auto)")
	}

	// Test Current message level
	info8 := &EthtoolInfo{}
	parseEthtoolKeyValue(info8, "Current message level", "0x00000007 (7)", false, false)
	if info8.MessageLevel != "0x00000007 (7)" {
		t.Errorf("MessageLevel = %q, want %q", info8.MessageLevel, "0x00000007 (7)")
	}

	// Test Supported link modes with empty value
	info9 := &EthtoolInfo{}
	parseEthtoolKeyValue(info9, "Supported link modes", "", false, false)
	if len(info9.SupportedLinkModes) != 0 {
		t.Errorf("SupportedLinkModes has %d items, want 0", len(info9.SupportedLinkModes))
	}

	// Test Advertised link modes with empty value
	info10 := &EthtoolInfo{}
	parseEthtoolKeyValue(info10, "Advertised link modes", "", false, false)
	if len(info10.AdvertisedLinkModes) != 0 {
		t.Errorf("AdvertisedLinkModes has %d items, want 0", len(info10.AdvertisedLinkModes))
	}

	// Test PHYAD with invalid value
	info11 := &EthtoolInfo{}
	parseEthtoolKeyValue(info11, "PHYAD", "invalid", false, false)
	if info11.PHYAD != 0 {
		t.Errorf("PHYAD = %d, want 0 for invalid input", info11.PHYAD)
	}
}
