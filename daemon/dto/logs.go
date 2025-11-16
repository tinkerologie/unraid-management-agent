package dto

import "time"

// LogFile represents metadata about a log file
type LogFile struct {
	Name       string    `json:"name"`
	Path       string    `json:"path"`
	Size       int64     `json:"size_bytes"`
	ModifiedAt time.Time `json:"modified_at"`
}

// LogFileContent represents the content of a log file with pagination support
type LogFileContent struct {
	Path       string   `json:"path"`
	Content    string   `json:"content"`
	Lines      []string `json:"lines"`
	TotalLines int      `json:"total_lines"`
	StartLine  int      `json:"start_line"`
	EndLine    int      `json:"end_line"`
}
