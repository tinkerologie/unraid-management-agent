package dto

import "time"

// ShareInfo contains share information
type ShareInfo struct {
	Name         string  `json:"name" example:"Media"`
	Path         string  `json:"path" example:"/mnt/user/Media"`
	Used         uint64  `json:"used_bytes" example:"5368709120000"`
	Free         uint64  `json:"free_bytes" example:"5368709120000"`
	Total        uint64  `json:"total_bytes" example:"10737418240000"`
	UsagePercent float64 `json:"usage_percent" example:"50"`

	// Configuration fields from share config
	Comment   string `json:"comment,omitempty" example:"Media storage share"` // Share comment/description
	SMBExport bool   `json:"smb_export" example:"true"`                       // Is share exported via SMB?
	NFSExport bool   `json:"nfs_export" example:"false"`                      // Is share exported via NFS?
	Storage   string `json:"storage" example:"cache+array"`                   // "cache", "array", "cache+array", or "unknown"
	UseCache  string `json:"use_cache,omitempty" example:"prefer"`            // "yes", "no", "only", "prefer"
	Security  string `json:"security,omitempty" example:"private"`            // "public", "private", "secure"

	Timestamp time.Time `json:"timestamp"`
}
