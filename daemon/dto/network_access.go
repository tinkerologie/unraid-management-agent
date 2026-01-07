package dto

import "time"

// NetworkAccessURLs contains all methods to access the Unraid server
type NetworkAccessURLs struct {
	URLs      []AccessURL `json:"urls"`
	Timestamp time.Time   `json:"timestamp"`
}

// AccessURL represents a single access method to the server
type AccessURL struct {
	Type string `json:"type"` // "lan", "wan", "wireguard", "mdns", "ipv6", "other"
	Name string `json:"name"`
	IPv4 string `json:"ipv4,omitempty"`
	IPv6 string `json:"ipv6,omitempty"`
}

// URLType constants for access URL types
const (
	URLTypeLAN       = "lan"
	URLTypeWAN       = "wan"
	URLTypeWireGuard = "wireguard"
	URLTypeMDNS      = "mdns"
	URLTypeIPv6      = "ipv6"
	URLTypeOther     = "other"
)
