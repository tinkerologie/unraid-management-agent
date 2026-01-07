package dto

import "time"

// NetworkInfo contains network interface information
type NetworkInfo struct {
	Name            string `json:"name" example:"eth0"`
	MACAddress      string `json:"mac_address" example:"00:11:22:33:44:55"`
	IPAddress       string `json:"ip_address" example:"192.168.1.100"`
	Speed           int    `json:"speed_mbps" example:"1000"`
	State           string `json:"state" example:"up"`
	BytesReceived   uint64 `json:"bytes_received" example:"1073741824"`
	BytesSent       uint64 `json:"bytes_sent" example:"536870912"`
	PacketsReceived uint64 `json:"packets_received" example:"1000000"`
	PacketsSent     uint64 `json:"packets_sent" example:"500000"`
	ErrorsReceived  uint64 `json:"errors_received" example:"0"`
	ErrorsSent      uint64 `json:"errors_sent" example:"0"`

	// Enhanced ethtool information
	SupportedPorts       []string `json:"supported_ports,omitempty"`
	SupportedLinkModes   []string `json:"supported_link_modes,omitempty"`
	SupportedPauseFrame  string   `json:"supported_pause_frame,omitempty" example:"Symmetric"`
	SupportsAutoNeg      bool     `json:"supports_auto_negotiation,omitempty" example:"true"`
	SupportedFECModes    []string `json:"supported_fec_modes,omitempty"`
	AdvertisedLinkModes  []string `json:"advertised_link_modes,omitempty"`
	AdvertisedPauseFrame string   `json:"advertised_pause_frame,omitempty" example:"Symmetric"`
	AdvertisedAutoNeg    bool     `json:"advertised_auto_negotiation,omitempty" example:"true"`
	AdvertisedFECModes   []string `json:"advertised_fec_modes,omitempty"`
	Duplex               string   `json:"duplex,omitempty" example:"Full"`
	AutoNegotiation      string   `json:"auto_negotiation,omitempty" example:"on"`
	Port                 string   `json:"port,omitempty" example:"Twisted Pair"`
	PHYAD                int      `json:"phyad,omitempty" example:"0"`
	Transceiver          string   `json:"transceiver,omitempty" example:"internal"`
	MDIX                 string   `json:"mdix,omitempty" example:"auto"`
	SupportsWakeOn       []string `json:"supports_wake_on,omitempty"`
	WakeOn               string   `json:"wake_on,omitempty" example:"g"`
	MessageLevel         string   `json:"message_level,omitempty" example:"0x0000003f"`
	LinkDetected         bool     `json:"link_detected,omitempty" example:"true"`
	MTU                  int      `json:"mtu,omitempty" example:"1500"`

	Timestamp time.Time `json:"timestamp"`
}
