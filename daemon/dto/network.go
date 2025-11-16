package dto

import "time"

// NetworkInfo contains network interface information
type NetworkInfo struct {
	Name            string `json:"name"`
	MACAddress      string `json:"mac_address"`
	IPAddress       string `json:"ip_address"`
	Speed           int    `json:"speed_mbps"`
	State           string `json:"state"`
	BytesReceived   uint64 `json:"bytes_received"`
	BytesSent       uint64 `json:"bytes_sent"`
	PacketsReceived uint64 `json:"packets_received"`
	PacketsSent     uint64 `json:"packets_sent"`
	ErrorsReceived  uint64 `json:"errors_received"`
	ErrorsSent      uint64 `json:"errors_sent"`

	// Enhanced ethtool information
	SupportedPorts       []string `json:"supported_ports,omitempty"`
	SupportedLinkModes   []string `json:"supported_link_modes,omitempty"`
	SupportedPauseFrame  string   `json:"supported_pause_frame,omitempty"`
	SupportsAutoNeg      bool     `json:"supports_auto_negotiation,omitempty"`
	SupportedFECModes    []string `json:"supported_fec_modes,omitempty"`
	AdvertisedLinkModes  []string `json:"advertised_link_modes,omitempty"`
	AdvertisedPauseFrame string   `json:"advertised_pause_frame,omitempty"`
	AdvertisedAutoNeg    bool     `json:"advertised_auto_negotiation,omitempty"`
	AdvertisedFECModes   []string `json:"advertised_fec_modes,omitempty"`
	Duplex               string   `json:"duplex,omitempty"`
	AutoNegotiation      string   `json:"auto_negotiation,omitempty"`
	Port                 string   `json:"port,omitempty"`
	PHYAD                int      `json:"phyad,omitempty"`
	Transceiver          string   `json:"transceiver,omitempty"`
	MDIX                 string   `json:"mdix,omitempty"`
	SupportsWakeOn       []string `json:"supports_wake_on,omitempty"`
	WakeOn               string   `json:"wake_on,omitempty"`
	MessageLevel         string   `json:"message_level,omitempty"`
	LinkDetected         bool     `json:"link_detected,omitempty"`
	MTU                  int      `json:"mtu,omitempty"`

	Timestamp time.Time `json:"timestamp"`
}
