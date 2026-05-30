package dto

import "time"

// MoverStatus represents the current status and last-run statistics for the Unraid mover.
// @Description Mover state, schedule, last-run timing, and file/byte transfer statistics.
type MoverStatus struct {
	// Active indicates whether the mover is currently running (from var.ini shareMoverActive).
	Active bool `json:"active" example:"false"`
	// Schedule is the cron schedule expression for the mover (from var.ini shareMoverSchedule).
	Schedule string `json:"schedule,omitempty" example:"40 3 * * *"`
	// LastRunStart is the ISO-8601 timestamp when the last mover run began.
	LastRunStart string `json:"last_run_start,omitempty" example:"2026-05-30T03:40:00Z"`
	// LastRunFinish is the ISO-8601 timestamp when the last mover run ended.
	LastRunFinish string `json:"last_run_finish,omitempty" example:"2026-05-30T03:52:00Z"`
	// LastRunDurationSeconds is the duration of the last run in seconds (finish - start).
	LastRunDurationSeconds int `json:"last_run_duration_seconds" example:"720"`
	// LastRunFilesMoved is the number of files moved during the last run.
	LastRunFilesMoved uint64 `json:"last_run_files_moved" example:"1024"`
	// LastRunBytesMoved is the total bytes moved during the last run.
	LastRunBytesMoved uint64 `json:"last_run_bytes_moved" example:"5368709120"`
	// CurrentThroughputMBs is the live throughput in MB/s. Always 0 in the conservative
	// implementation (live throughput tracking is out of scope).
	CurrentThroughputMBs float64 `json:"current_throughput_mbs" example:"0"`
	// Timestamp is when this status was collected.
	Timestamp time.Time `json:"timestamp"`
}
