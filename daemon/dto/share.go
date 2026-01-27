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

	// Cache pool settings (Issue #53)
	CachePool   string `json:"cache_pool,omitempty" example:"cache"`          // Primary cache pool name
	CachePool2  string `json:"cache_pool2,omitempty" example:""`              // Secondary cache pool (for mover destination)
	MoverAction string `json:"mover_action,omitempty" example:"cache->array"` // Mover action: "cache->array", "array->cache", or empty

	Timestamp time.Time `json:"timestamp"`
}
