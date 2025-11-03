// Package dto provides data transfer objects for the Unraid Management Agent API.
package dto

import "time"

// ArrayStatus contains Unraid array status information
type ArrayStatus struct {
	State               string    `json:"state"`
	UsedPercent         float64   `json:"used_percent"`
	FreeBytes           uint64    `json:"free_bytes"`
	TotalBytes          uint64    `json:"total_bytes"`
	ParityValid         bool      `json:"parity_valid"`
	ParityCheckStatus   string    `json:"parity_check_status"`
	ParityCheckProgress float64   `json:"parity_check_progress"`
	NumDisks            int       `json:"num_disks"`
	NumDataDisks        int       `json:"num_data_disks"`
	NumParityDisks      int       `json:"num_parity_disks"`
	Timestamp           time.Time `json:"timestamp"`
}
