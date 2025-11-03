package dto

import "time"

// NetworkInfo contains network interface information
type NetworkInfo struct {
	Name            string    `json:"name"`
	MACAddress      string    `json:"mac_address"`
	IPAddress       string    `json:"ip_address"`
	Speed           int       `json:"speed_mbps"`
	State           string    `json:"state"`
	BytesReceived   uint64    `json:"bytes_received"`
	BytesSent       uint64    `json:"bytes_sent"`
	PacketsReceived uint64    `json:"packets_received"`
	PacketsSent     uint64    `json:"packets_sent"`
	ErrorsReceived  uint64    `json:"errors_received"`
	ErrorsSent      uint64    `json:"errors_sent"`
	Timestamp       time.Time `json:"timestamp"`
}
