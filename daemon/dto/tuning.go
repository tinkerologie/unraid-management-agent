package dto

import "time"

// TuningInfo contains system tuning parameters from kernel settings,
// equivalent to what the Unraid Tips & Tweaks plugin exposes.
type TuningInfo struct {
	// CPU Power Management
	TurboBoost *TurboBoostInfo `json:"turbo_boost,omitempty"`

	// Disk Cache (vm.dirty_* parameters)
	DiskCache *DiskCacheInfo `json:"disk_cache,omitempty"`

	// Inotify limits
	Inotify *InotifyInfo `json:"inotify,omitempty"`

	// NIC Offload settings per interface
	NICOffloads map[string]*NICOffloadInfo `json:"nic_offloads,omitempty"`

	// NIC Ring Buffers per interface
	NICRingBuffers map[string]*NICRingBufferInfo `json:"nic_ring_buffers,omitempty"`

	Timestamp time.Time `json:"timestamp"`
}

// TurboBoostInfo represents Intel Turbo Boost / AMD Performance Boost state.
type TurboBoostInfo struct {
	Available bool   `json:"available" example:"true"`
	Enabled   bool   `json:"enabled" example:"true"`
	Vendor    string `json:"vendor" example:"intel"`
}

// DiskCacheInfo contains Linux VM dirty page writeback parameters.
type DiskCacheInfo struct {
	DirtyBackgroundRatio int `json:"dirty_background_ratio" example:"10"`
	DirtyRatio           int `json:"dirty_ratio" example:"20"`
	DirtyWritebackCenti  int `json:"dirty_writeback_centisecs" example:"200"`
	DirtyExpireCenti     int `json:"dirty_expire_centisecs" example:"1000"`
}

// InotifyInfo contains inotify limits from the kernel.
type InotifyInfo struct {
	MaxUserWatches   int `json:"max_user_watches" example:"524288"`
	MaxUserInstances int `json:"max_user_instances" example:"128"`
	MaxQueuedEvents  int `json:"max_queued_events" example:"16384"`
}

// NICOffloadInfo contains NIC hardware offload feature states.
type NICOffloadInfo struct {
	RxChecksumming        string `json:"rx_checksumming" example:"on"`
	TxChecksumming        string `json:"tx_checksumming" example:"on"`
	ScatterGather         string `json:"scatter_gather" example:"on"`
	TCPSegmentOffload     string `json:"tcp_segmentation_offload" example:"on"`
	GenericSegmentOffload string `json:"generic_segmentation_offload" example:"on"`
	GenericReceiveOffload string `json:"generic_receive_offload" example:"on"`
	LargeReceiveOffload   string `json:"large_receive_offload" example:"off"`
	RxVlanOffload         string `json:"rx_vlan_offload" example:"on"`
	TxVlanOffload         string `json:"tx_vlan_offload" example:"on"`
}

// NICRingBufferInfo contains NIC ring buffer sizes.
type NICRingBufferInfo struct {
	RxMax     int `json:"rx_max" example:"4096"`
	RxCurrent int `json:"rx_current" example:"256"`
	TxMax     int `json:"tx_max" example:"4096"`
	TxCurrent int `json:"tx_current" example:"256"`
}

// TurboBoostRequest is the JSON body for enabling/disabling turbo boost.
type TurboBoostRequest struct {
	Enabled bool `json:"enabled" example:"true"`
}

// DiskCacheRequest is the JSON body for setting disk cache parameters.
type DiskCacheRequest struct {
	DirtyBackgroundRatio int `json:"dirty_background_ratio" example:"10"`
	DirtyRatio           int `json:"dirty_ratio" example:"20"`
	DirtyWritebackCenti  int `json:"dirty_writeback_centisecs" example:"500"`
	DirtyExpireCenti     int `json:"dirty_expire_centisecs" example:"3000"`
}

// InotifyLimitsRequest is the JSON body for setting inotify limits.
type InotifyLimitsRequest struct {
	MaxUserWatches   int `json:"max_user_watches" validate:"required" example:"524288"`
	MaxUserInstances int `json:"max_user_instances" validate:"required" example:"512"`
	MaxQueuedEvents  int `json:"max_queued_events" validate:"required" example:"16384"`
}
