package dto

import "time"

// Registration contains Unraid license/registration information
type Registration struct {
	Type             string    `json:"type"`                        // "trial", "basic", "plus", "pro", "lifetime"
	State            string    `json:"state"`                       // "valid", "expired", "invalid", "trial"
	Expiration       time.Time `json:"expiration,omitempty"`        // License expiration date
	UpdateExpiration time.Time `json:"update_expiration,omitempty"` // Update expiration date
	ServerName       string    `json:"server_name,omitempty"`       // Server name from config
	GUID             string    `json:"guid,omitempty"`              // Registration GUID
	Timestamp        time.Time `json:"timestamp"`                   // Collection timestamp
}
