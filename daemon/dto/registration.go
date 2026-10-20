package dto

import "time"

// Registration contains Unraid license/registration information
type Registration struct {
	Type             string    `json:"type"`                  // "trial", "basic", "plus", "pro", "lifetime"
	State            string    `json:"state"`                 // "valid", "expired", "invalid", "trial"
	Expiration       time.Time `json:"expiration"`            // License expiration (regTm2), set only for trial and expired keys; zero when the key does not expire
	UpdateExpiration time.Time `json:"update_expiration"`     // Update eligibility expiration (regExp); zero when the key has none
	ServerName       string    `json:"server_name,omitempty"` // Server name from config
	GUID             string    `json:"guid,omitempty"`        // Registration GUID
	Timestamp        time.Time `json:"timestamp"`             // Collection timestamp
}
