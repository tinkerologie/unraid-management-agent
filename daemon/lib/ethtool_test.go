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
