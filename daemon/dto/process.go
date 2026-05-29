package dto

import "time"

// ProcessInfo contains information about a running process
type ProcessInfo struct {
	PID           int     `json:"pid" example:"1234"`
	User          string  `json:"user" example:"root"`
	CPUPercent    float64 `json:"cpu_percent" example:"5.2"`
	MemoryPercent float64 `json:"memory_percent" example:"2.1"`
	VSZBytes      uint64  `json:"vsz_bytes" example:"1073741824"`
	RSSBytes      uint64  `json:"rss_bytes" example:"536870912"`
	TTY           string  `json:"tty" example:"?"`
	State         string  `json:"state" example:"S"`
	Started       string  `json:"started" example:"Jan01"`
	Time          string  `json:"time" example:"0:05"`
	Command       string  `json:"command" example:"/usr/bin/docker"`

	// DiskReadBytesPerSec is the per-process disk read rate (bytes/sec), populated by
	// the /processes/io endpoint from /proc/<pid>/io. Omitted from the standard process list.
	DiskReadBytesPerSec uint64 `json:"disk_read_bytes_per_sec,omitempty" example:"1048576"`
	// DiskWriteBytesPerSec is the per-process disk write rate (bytes/sec), populated by
	// the /processes/io endpoint from /proc/<pid>/io. Omitted from the standard process list.
	DiskWriteBytesPerSec uint64 `json:"disk_write_bytes_per_sec,omitempty" example:"524288"`
}

// ProcessList contains the list of running processes
type ProcessList struct {
	Processes  []ProcessInfo `json:"processes"`
	TotalCount int           `json:"total_count" example:"150"`
	Timestamp  time.Time     `json:"timestamp"`
}
