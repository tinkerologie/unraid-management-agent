package collectors

import (
	"testing"
)

func TestNewParityCollector(t *testing.T) {
	collector := NewParityCollector()

	if collector == nil {
		t.Fatal("NewParityCollector() returned nil")
	}
}

func TestParityParseSize(t *testing.T) {
	collector := NewParityCollector()

	tests := []struct {
		name     string
		input    string
		expected uint64
		wantErr  bool
	}{
		{"bytes", "100 B", 100, false},
		{"kilobytes", "1 KB", 1024, false},
		{"megabytes", "1 MB", 1024 * 1024, false},
		{"gigabytes", "1 GB", 1024 * 1024 * 1024, false},
		{"terabytes", "1 TB", 1024 * 1024 * 1024 * 1024, false},
		{"decimal TB", "10.5 TB", uint64(10.5 * 1024 * 1024 * 1024 * 1024), false},
		{"lowercase kb", "1 kb", 1024, false},
		{"lowercase mb", "1 mb", 1024 * 1024, false},
		{"invalid format no unit", "100", 0, true},
		{"invalid format no value", "TB", 0, true},
		{"unknown unit", "100 PB", 0, true},
		{"invalid number", "abc TB", 0, true},
		{"empty string", "", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := collector.parseSize(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("parseSize(%q) expected error but got nil", tt.input)
				}
			} else {
				if err != nil {
					t.Errorf("parseSize(%q) unexpected error: %v", tt.input, err)
				}
				if result != tt.expected {
					t.Errorf("parseSize(%q) = %d, want %d", tt.input, result, tt.expected)
				}
			}
		})
	}
}

func TestParityParseDuration(t *testing.T) {
	collector := NewParityCollector()

	tests := []struct {
		name     string
		input    string
		expected int64
	}{
		{"seconds only", "30 sec", 30},
		{"minutes only", "5 min", 300},
		{"hours only", "2 hr", 7200},
		{"days only", "1 day", 86400},
		{"full duration", "1 day, 4 hr, 1 min, 28 sec", 86400 + 14400 + 60 + 28},
		{"hours and minutes", "2 hr, 30 min", 7200 + 1800},
		{"days and hours", "2 days, 5 hr", 172800 + 18000},
		{"empty string", "", 0},
		{"invalid format", "invalid", 0},
		{"minutes singular", "1 minute", 60},
		{"hours singular", "1 hour", 3600},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := collector.parseDuration(tt.input)
			if err != nil {
				t.Errorf("parseDuration(%q) unexpected error: %v", tt.input, err)
			}
			if result != tt.expected {
				t.Errorf("parseDuration(%q) = %d, want %d", tt.input, result, tt.expected)
			}
		})
	}
}

func TestParityParseSpeed(t *testing.T) {
	collector := NewParityCollector()

	tests := []struct {
		name     string
		input    string
		expected float64
		wantErr  bool
	}{
		{"typical speed", "99.1 MB/s", 99.1, false},
		{"integer speed", "100 MB/s", 100.0, false},
		{"low speed", "5.5 MB/s", 5.5, false},
		{"high speed", "250.75 MB/s", 250.75, false},
		{"zero speed", "0 MB/s", 0.0, false},
		{"invalid format", "invalid", 0, true},
		{"empty string", "", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := collector.parseSpeed(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("parseSpeed(%q) expected error but got nil", tt.input)
				}
			} else {
				if err != nil {
					t.Errorf("parseSpeed(%q) unexpected error: %v", tt.input, err)
				}
				if result != tt.expected {
					t.Errorf("parseSpeed(%q) = %f, want %f", tt.input, result, tt.expected)
				}
			}
		})
	}
}
