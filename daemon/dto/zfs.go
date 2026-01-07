package dto

import "time"

// ZFSPool represents a ZFS storage pool
type ZFSPool struct {
	// Pool Identification
	Name    string `json:"name" example:"tank"`
	GUID    string `json:"guid,omitempty" example:"1234567890123456789"`    // Unique pool GUID
	Version string `json:"version,omitempty" example:"5000"` // ZFS pool version

	// Health and Status
	Health string `json:"health" example:"ONLINE"` // "ONLINE", "DEGRADED", "FAULTED", "OFFLINE", "UNAVAIL", "REMOVED"
	State  string `json:"state" example:"ACTIVE"`  // "ACTIVE", "EXPORTED", "DESTROYED", "SPARE", "L2CACHE"

	// Capacity
	SizeBytes         uint64  `json:"size_bytes" example:"107374182400"`          // Total pool size
	AllocatedBytes    uint64  `json:"allocated_bytes" example:"53687091200"`     // Space allocated
	FreeBytes         uint64  `json:"free_bytes" example:"53687091200"`          // Space available
	FragmentationPct  float64 `json:"fragmentation_percent" example:"5"` // Fragmentation %
	CapacityPct       float64 `json:"capacity_percent" example:"50"`    // Usage %

	// Deduplication and Compression
	DedupRatio    float64 `json:"dedup_ratio" example:"1.00"`    // Deduplication ratio (e.g., 1.00 = no dedup)
	CompressRatio float64 `json:"compress_ratio,omitempty" example:"1.50"` // Compression ratio (e.g., 1.50 = 1.5x)

	// Features
	Altroot    string `json:"altroot,omitempty" example:"-"`    // Alternate root directory
	Readonly   bool   `json:"readonly" example:"false"`             // Read-only status
	Autoexpand bool   `json:"autoexpand" example:"true"`           // Auto-expand on disk replacement
	Autotrim   string `json:"autotrim,omitempty" example:"on"`   // "on", "off"

	// VDEVs (Virtual Devices)
	VDEVs []ZFSVdev `json:"vdevs"` // Pool virtual devices

	// Scrub Information
	ScanStatus       string    `json:"scan_status,omitempty" example:"scrub completed"`        // "scrub in progress", "scrub completed", "resilver in progress"
	ScanState        string    `json:"scan_state,omitempty" example:"finished"`         // "scanning", "finished", "canceled"
	ScanErrors       int       `json:"scan_errors" example:"0"`                  // Errors found during last scrub
	ScanRepairedBytes uint64   `json:"scan_repaired_bytes" example:"0"`          // Data repaired in last scrub
	ScanStartTime    time.Time `json:"scan_start_time,omitempty"`    // When scrub started
	ScanEndTime      time.Time `json:"scan_end_time,omitempty"`      // When scrub ended
	ScanProgressPct  float64   `json:"scan_progress_percent" example:"100"`        // Scrub progress %

	// Error Counters
	ReadErrors     uint64 `json:"read_errors" example:"0"`
	WriteErrors    uint64 `json:"write_errors" example:"0"`
	ChecksumErrors uint64 `json:"checksum_errors" example:"0"`

	Timestamp time.Time `json:"timestamp"`
}

// ZFSVdev represents a virtual device (vdev) in a ZFS pool
type ZFSVdev struct {
	Name           string       `json:"name" example:"raidz1-0"`            // vdev name (e.g., "raidz1-0", "mirror-1", or disk name)
	Type           string       `json:"type" example:"raidz1"`            // "disk", "mirror", "raidz1", "raidz2", "raidz3", "spare", "cache", "log"
	State          string       `json:"state" example:"ONLINE"`           // "ONLINE", "DEGRADED", "FAULTED", "OFFLINE"
	ReadErrors     uint64       `json:"read_errors" example:"0"`
	WriteErrors    uint64       `json:"write_errors" example:"0"`
	ChecksumErrors uint64       `json:"checksum_errors" example:"0"`
	Devices        []ZFSDevice  `json:"devices,omitempty"` // Underlying devices (for mirror/raidz)
}

// ZFSDevice represents a physical device in a vdev
type ZFSDevice struct {
	Name           string `json:"name" example:"sda1"`                        // Device path (e.g., "sda1", "nvme0n1p1")
	State          string `json:"state" example:"ONLINE"`                       // "ONLINE", "DEGRADED", "FAULTED", "OFFLINE"
	ReadErrors     uint64 `json:"read_errors" example:"0"`
	WriteErrors    uint64 `json:"write_errors" example:"0"`
	ChecksumErrors uint64 `json:"checksum_errors" example:"0"`
	PhysicalPath   string `json:"physical_path,omitempty" example:"/dev/disk/by-id/ata-WDC_WD120EFBX-68B0EN0_WD-WMC4N0123456"`     // Physical device path
}

// ZFSDataset represents a ZFS dataset (filesystem or volume)
type ZFSDataset struct {
	Name            string    `json:"name" example:"tank/media"`                      // Full dataset name (pool/dataset/child)
	Type            string    `json:"type" example:"filesystem"`                      // "filesystem", "volume", "snapshot"
	UsedBytes       uint64    `json:"used_bytes" example:"53687091200"`                // Space used
	AvailableBytes  uint64    `json:"available_bytes" example:"53687091200"`           // Space available
	ReferencedBytes uint64    `json:"referenced_bytes" example:"53687091200"`          // Space referenced by dataset
	CompressRatio   float64   `json:"compress_ratio" example:"1.50"`            // Compression ratio
	Mountpoint      string    `json:"mountpoint,omitempty" example:"/mnt/tank/media"`      // Mount point (if filesystem)
	QuotaBytes      uint64    `json:"quota_bytes,omitempty" example:"107374182400"`     // Dataset quota (0 = none)
	ReservationBytes uint64   `json:"reservation_bytes,omitempty" example:"0"` // Reserved space
	Compression     string    `json:"compression" example:"lz4"`               // Compression algorithm
	Readonly        bool      `json:"readonly" example:"false"`                  // Read-only status
	Timestamp       time.Time `json:"timestamp"`
}

// ZFSSnapshot represents a ZFS snapshot
type ZFSSnapshot struct {
	Name            string    `json:"name" example:"tank/media@daily-2025-01-15"`              // Snapshot name (dataset@snapshot)
	Dataset         string    `json:"dataset" example:"tank/media"`           // Parent dataset
	UsedBytes       uint64    `json:"used_bytes" example:"1073741824"`        // Space used by snapshot
	ReferencedBytes uint64    `json:"referenced_bytes" example:"53687091200"`  // Space referenced
	CreationTime    time.Time `json:"creation_time"`     // When snapshot was created
	Timestamp       time.Time `json:"timestamp"`
}

// ZFSARCStats represents ZFS ARC (Adaptive Replacement Cache) statistics
type ZFSARCStats struct {
	// ARC Size
	SizeBytes       uint64 `json:"size_bytes" example:"8589934592"`        // Current ARC size
	TargetSizeBytes uint64 `json:"target_size_bytes" example:"16106127360"` // Target ARC size
	MinSizeBytes    uint64 `json:"min_size_bytes" example:"2013265920"`    // Minimum ARC size
	MaxSizeBytes    uint64 `json:"max_size_bytes" example:"16106127360"`   // Maximum ARC size

	// Hit Ratios
	HitRatioPct    float64 `json:"hit_ratio_percent" example:"95.5"`     // Overall hit ratio %
	MRUHitRatioPct float64 `json:"mru_hit_ratio_percent" example:"92.3"` // Most Recently Used hit ratio
	MFUHitRatioPct float64 `json:"mfu_hit_ratio_percent" example:"97.8"` // Most Frequently Used hit ratio

	// Hits and Misses
	Hits   uint64 `json:"hits" example:"1000000"`   // Total cache hits
	Misses uint64 `json:"misses" example:"50000"` // Total cache misses

	// L2ARC (Level 2 ARC - SSD cache)
	L2SizeBytes uint64 `json:"l2_size_bytes,omitempty" example:"107374182400"` // L2ARC size
	L2Hits      uint64 `json:"l2_hits,omitempty" example:"500000"`       // L2ARC hits
	L2Misses    uint64 `json:"l2_misses,omitempty" example:"25000"`     // L2ARC misses

	Timestamp time.Time `json:"timestamp"`
}

// ZFSIOStats represents ZFS I/O statistics per pool
type ZFSIOStats struct {
	PoolName string `json:"pool_name" example:"tank"`

	// Operations
	ReadOps  uint64 `json:"read_ops" example:"100000"`  // Read operations
	WriteOps uint64 `json:"write_ops" example:"50000"` // Write operations

	// Bandwidth
	ReadBandwidthBytes  uint64 `json:"read_bandwidth_bytes" example:"104857600"`  // Read bandwidth (bytes/sec)
	WriteBandwidthBytes uint64 `json:"write_bandwidth_bytes" example:"52428800"` // Write bandwidth (bytes/sec)

	Timestamp time.Time `json:"timestamp"`
}

