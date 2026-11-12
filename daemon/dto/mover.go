package dto

import "time"

// MoverStatus represents the current status and last-run statistics for the Unraid mover.
// @Description Mover state, schedule, last-run timing, and file/byte transfer statistics.
type MoverStatus struct {
	// Active indicates whether Mover is running.
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
	// CurrentThroughputMBs is the live decimal MB/s, or zero when unavailable.
	CurrentThroughputMBs float64 `json:"current_throughput_mbs" example:"0"`
	// Live progress fields are null when native progress is unavailable.
	ProgressTotalBytes       *uint64  `json:"progress_total_bytes" extensions:"x-nullable"`
	ProgressRemainingBytes   *uint64  `json:"progress_remaining_bytes" extensions:"x-nullable"`
	ProgressMovedBytes       *uint64  `json:"progress_moved_bytes" extensions:"x-nullable"`
	ProgressPercent          *float64 `json:"progress_percent" extensions:"x-nullable"`
	ThroughputBytesPerSecond *uint64  `json:"throughput_bytes_per_second" extensions:"x-nullable"`
	ETASeconds               *uint64  `json:"eta_seconds" extensions:"x-nullable"`
	// StatusSource identifies the activity source, native or legacy.
	StatusSource string `json:"status_source,omitempty" example:"native"`
	// Timestamp is when this status was collected.
	Timestamp time.Time `json:"timestamp"`
}
