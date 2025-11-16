package dto

import "time"

// ShareInfo contains share information
type ShareInfo struct {
	Name         string  `json:"name"`
	Path         string  `json:"path"`
	Used         uint64  `json:"used_bytes"`
	Free         uint64  `json:"free_bytes"`
	Total        uint64  `json:"total_bytes"`
	UsagePercent float64 `json:"usage_percent"`

	// Configuration fields from share config
	Comment   string `json:"comment,omitempty"`   // Share comment/description
	SMBExport bool   `json:"smb_export"`          // Is share exported via SMB?
	NFSExport bool   `json:"nfs_export"`          // Is share exported via NFS?
	Storage   string `json:"storage"`             // "cache", "array", "cache+array", or "unknown"
	UseCache  string `json:"use_cache,omitempty"` // "yes", "no", "only", "prefer"
	Security  string `json:"security,omitempty"`  // "public", "private", "secure"

	Timestamp time.Time `json:"timestamp"`
}
