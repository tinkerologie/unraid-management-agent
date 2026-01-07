// Package dto provides data transfer objects for the Unraid Management Agent API.
package dto

import "time"

// ArrayStatus contains Unraid array status information
type ArrayStatus struct {
	State               string    `json:"state" example:"Started"`
	UsedPercent         float64   `json:"used_percent" example:"45.5"`
	FreeBytes           uint64    `json:"free_bytes" example:"54975581388800"`
	TotalBytes          uint64    `json:"total_bytes" example:"100862164623360"`
	ParityValid         bool      `json:"parity_valid" example:"true"`
	ParityCheckStatus   string    `json:"parity_check_status" example:"idle"`
	ParityCheckProgress float64   `json:"parity_check_progress" example:"0"`
	NumDisks            int       `json:"num_disks" example:"10"`
	NumDataDisks        int       `json:"num_data_disks" example:"8"`
	NumParityDisks      int       `json:"num_parity_disks" example:"2"`
	Timestamp           time.Time `json:"timestamp"`
}
