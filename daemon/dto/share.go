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

	// Exclusive share state (Unraid 6.12+). These fields are omitted when the
	// release has no exclusive-share setting or user shares are not mounted.

	// Exclusive is true when /mnt/user/<name> is a symlink to the pool, so
	// share I/O bypasses the FUSE user share layer (shfs). Taken from the
	// exclusive flag emhttp records in shares.ini.
	Exclusive *bool `json:"exclusive,omitempty" example:"true"`
	// ExclusiveEligible is true when exclusive shares are enabled globally and
	// the share lives on a single pool (primary pool only, no secondary storage
	// — on servers without array slots any primary pool with no secondary pool;
	// before Unraid 7.2 also not exported over NFS) and that pool exists.
	// Omitted when this cannot be determined (share config or disk list
	// unreadable).
	ExclusiveEligible *bool `json:"exclusive_eligible,omitempty" example:"true"`
	// ExclusivePool is the share's pool for exclusive access: the symlink
	// target when exclusive, otherwise the pool a pool-only share would be
	// exclusive on (on pools-only servers this can be the implicit first pool).
	ExclusivePool string `json:"exclusive_pool,omitempty" example:"cache"`
	// Locations lists the pools and array disks holding a top-level folder for
	// the share. Reported only when the share is eligible but not exclusive
	// (an empty list when none was found): any entry other than the share's
	// pool is what keeps it on FUSE.
	Locations *[]string `json:"locations,omitempty" swaggertype:"array,string" example:"cache,disk1"`
	// LocationsUnchecked lists the disks and pools that were not checked for a
	// folder because a device is spun down, its spin state is unknown or the
	// check failed, so they are never woken. Reported alongside Locations;
	// both are omitted when the disk list cannot be read.
	LocationsUnchecked []string `json:"locations_unchecked,omitempty" example:"disk3"`

	// SourceStatus is non-nil only when the backing data source is not healthy.
	SourceStatus *SourceStatus `json:"source_status,omitempty"`

	Timestamp time.Time `json:"timestamp"`
}
