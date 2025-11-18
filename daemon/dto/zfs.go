package dto

import "time"

// ZFSPool represents a ZFS storage pool
type ZFSPool struct {
	// Pool Identification
	Name    string `json:"name"`
	GUID    string `json:"guid,omitempty"`    // Unique pool GUID
	Version string `json:"version,omitempty"` // ZFS pool version

	// Health and Status
	Health string `json:"health"` // "ONLINE", "DEGRADED", "FAULTED", "OFFLINE", "UNAVAIL", "REMOVED"
	State  string `json:"state"`  // "ACTIVE", "EXPORTED", "DESTROYED", "SPARE", "L2CACHE"

	// Capacity
	SizeBytes         uint64  `json:"size_bytes"`          // Total pool size
	AllocatedBytes    uint64  `json:"allocated_bytes"`     // Space allocated
	FreeBytes         uint64  `json:"free_bytes"`          // Space available
	FragmentationPct  float64 `json:"fragmentation_percent"` // Fragmentation %
	CapacityPct       float64 `json:"capacity_percent"`    // Usage %

	// Deduplication and Compression
	DedupRatio    float64 `json:"dedup_ratio"`    // Deduplication ratio (e.g., 1.00 = no dedup)
	CompressRatio float64 `json:"compress_ratio,omitempty"` // Compression ratio (e.g., 1.50 = 1.5x)

	// Features
	Altroot    string `json:"altroot,omitempty"`    // Alternate root directory
	Readonly   bool   `json:"readonly"`             // Read-only status
	Autoexpand bool   `json:"autoexpand"`           // Auto-expand on disk replacement
	Autotrim   string `json:"autotrim,omitempty"`   // "on", "off"

	// VDEVs (Virtual Devices)
	VDEVs []ZFSVdev `json:"vdevs"` // Pool virtual devices

	// Scrub Information
	ScanStatus       string    `json:"scan_status,omitempty"`        // "scrub in progress", "scrub completed", "resilver in progress"
	ScanState        string    `json:"scan_state,omitempty"`         // "scanning", "finished", "canceled"
	ScanErrors       int       `json:"scan_errors"`                  // Errors found during last scrub
	ScanRepairedBytes uint64   `json:"scan_repaired_bytes"`          // Data repaired in last scrub
	ScanStartTime    time.Time `json:"scan_start_time,omitempty"`    // When scrub started
	ScanEndTime      time.Time `json:"scan_end_time,omitempty"`      // When scrub ended
	ScanProgressPct  float64   `json:"scan_progress_percent"`        // Scrub progress %

	// Error Counters
	ReadErrors     uint64 `json:"read_errors"`
	WriteErrors    uint64 `json:"write_errors"`
	ChecksumErrors uint64 `json:"checksum_errors"`

	Timestamp time.Time `json:"timestamp"`
}

// ZFSVdev represents a virtual device (vdev) in a ZFS pool
type ZFSVdev struct {
	Name           string       `json:"name"`            // vdev name (e.g., "raidz1-0", "mirror-1", or disk name)
	Type           string       `json:"type"`            // "disk", "mirror", "raidz1", "raidz2", "raidz3", "spare", "cache", "log"
	State          string       `json:"state"`           // "ONLINE", "DEGRADED", "FAULTED", "OFFLINE"
	ReadErrors     uint64       `json:"read_errors"`
	WriteErrors    uint64       `json:"write_errors"`
	ChecksumErrors uint64       `json:"checksum_errors"`
	Devices        []ZFSDevice  `json:"devices,omitempty"` // Underlying devices (for mirror/raidz)
}

// ZFSDevice represents a physical device in a vdev
type ZFSDevice struct {
	Name           string `json:"name"`                        // Device path (e.g., "sda1", "nvme0n1p1")
	State          string `json:"state"`                       // "ONLINE", "DEGRADED", "FAULTED", "OFFLINE"
	ReadErrors     uint64 `json:"read_errors"`
	WriteErrors    uint64 `json:"write_errors"`
	ChecksumErrors uint64 `json:"checksum_errors"`
	PhysicalPath   string `json:"physical_path,omitempty"`     // Physical device path
}

// ZFSDataset represents a ZFS dataset (filesystem or volume)
type ZFSDataset struct {
	Name            string    `json:"name"`                      // Full dataset name (pool/dataset/child)
	Type            string    `json:"type"`                      // "filesystem", "volume", "snapshot"
	UsedBytes       uint64    `json:"used_bytes"`                // Space used
	AvailableBytes  uint64    `json:"available_bytes"`           // Space available
	ReferencedBytes uint64    `json:"referenced_bytes"`          // Space referenced by dataset
	CompressRatio   float64   `json:"compress_ratio"`            // Compression ratio
	Mountpoint      string    `json:"mountpoint,omitempty"`      // Mount point (if filesystem)
	QuotaBytes      uint64    `json:"quota_bytes,omitempty"`     // Dataset quota (0 = none)
	ReservationBytes uint64   `json:"reservation_bytes,omitempty"` // Reserved space
	Compression     string    `json:"compression"`               // Compression algorithm
	Readonly        bool      `json:"readonly"`                  // Read-only status
	Timestamp       time.Time `json:"timestamp"`
}

// ZFSSnapshot represents a ZFS snapshot
type ZFSSnapshot struct {
	Name            string    `json:"name"`              // Snapshot name (dataset@snapshot)
	Dataset         string    `json:"dataset"`           // Parent dataset
	UsedBytes       uint64    `json:"used_bytes"`        // Space used by snapshot
	ReferencedBytes uint64    `json:"referenced_bytes"`  // Space referenced
	CreationTime    time.Time `json:"creation_time"`     // When snapshot was created
	Timestamp       time.Time `json:"timestamp"`
}

// ZFSARCStats represents ZFS ARC (Adaptive Replacement Cache) statistics
type ZFSARCStats struct {
	// ARC Size
	SizeBytes       uint64 `json:"size_bytes"`        // Current ARC size
	TargetSizeBytes uint64 `json:"target_size_bytes"` // Target ARC size
	MinSizeBytes    uint64 `json:"min_size_bytes"`    // Minimum ARC size
	MaxSizeBytes    uint64 `json:"max_size_bytes"`    // Maximum ARC size

	// Hit Ratios
	HitRatioPct    float64 `json:"hit_ratio_percent"`     // Overall hit ratio %
	MRUHitRatioPct float64 `json:"mru_hit_ratio_percent"` // Most Recently Used hit ratio
	MFUHitRatioPct float64 `json:"mfu_hit_ratio_percent"` // Most Frequently Used hit ratio

	// Hits and Misses
	Hits   uint64 `json:"hits"`   // Total cache hits
	Misses uint64 `json:"misses"` // Total cache misses

	// L2ARC (Level 2 ARC - SSD cache)
	L2SizeBytes uint64 `json:"l2_size_bytes,omitempty"` // L2ARC size
	L2Hits      uint64 `json:"l2_hits,omitempty"`       // L2ARC hits
	L2Misses    uint64 `json:"l2_misses,omitempty"`     // L2ARC misses

	Timestamp time.Time `json:"timestamp"`
}

// ZFSIOStats represents ZFS I/O statistics per pool
type ZFSIOStats struct {
	PoolName string `json:"pool_name"`

	// Operations
	ReadOps  uint64 `json:"read_ops"`  // Read operations
	WriteOps uint64 `json:"write_ops"` // Write operations

	// Bandwidth
	ReadBandwidthBytes  uint64 `json:"read_bandwidth_bytes"`  // Read bandwidth (bytes/sec)
	WriteBandwidthBytes uint64 `json:"write_bandwidth_bytes"` // Write bandwidth (bytes/sec)

	Timestamp time.Time `json:"timestamp"`
}

