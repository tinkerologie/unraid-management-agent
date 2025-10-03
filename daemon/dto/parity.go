package dto

import "time"

// ParityCheckRecord represents a single parity check/sync operation from history
type ParityCheckRecord struct {
	Action    string    `json:"action"`     // "Parity-Check", "Parity-Sync", "Read-Check", "Clear"
	Date      time.Time `json:"date"`       // Date and time of the operation
	Duration  int64     `json:"duration_seconds"` // Duration in seconds
	Speed     float64   `json:"speed_mbps"` // Average speed in MB/s
	Status    string    `json:"status"`     // "OK", "Canceled", or error count
	Errors    int64     `json:"errors"`     // Number of errors found
	Size      uint64    `json:"size_bytes"` // Size of array checked in bytes
}

// ParityCheckHistory contains the list of parity check records
type ParityCheckHistory struct {
	Records   []ParityCheckRecord `json:"records"`
	Timestamp time.Time           `json:"timestamp"`
}

