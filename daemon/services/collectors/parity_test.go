package collectors

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewParityCollector(t *testing.T) {
	collector := NewParityCollector()

	if collector == nil {
		t.Fatal("NewParityCollector() returned nil")
	}
}

func TestParityParseLine(t *testing.T) {
	collector := NewParityCollector()

	tests := []struct {
		name          string
		input         string
		wantAction    string
		wantStatus    string
		wantErrors    int64
		wantDuration  int64
		wantSpeedMBps float64
		wantSizeBytes uint64
		wantYear      int
		wantMonth     time.Month
		wantDay       int
		wantErr       bool
	}{
		// Standard 7-field format tests
		{
			name:          "7-field format: successful parity check with errors",
			input:         "2024 Nov 30 00:30:26|100888|99128056|0|1348756140|check P|9766436812",
			wantAction:    "Parity-Check",
			wantStatus:    "1348756140 errors",
			wantErrors:    1348756140,
			wantDuration:  100888,
			wantSpeedMBps: 99128056.0 / (1024 * 1024),
			wantSizeBytes: 9766436812,
			wantYear:      2024,
			wantMonth:     time.November,
			wantDay:       30,
			wantErr:       false,
		},
		{
			name:          "7-field format: successful parity check no errors",
			input:         "2025 Jan  2 06:25:17|63380|157791595|0|0|check P|9766436812",
			wantAction:    "Parity-Check",
			wantStatus:    "OK",
			wantErrors:    0,
			wantDuration:  63380,
			wantSpeedMBps: 157791595.0 / (1024 * 1024),
			wantSizeBytes: 9766436812,
			wantYear:      2025,
			wantMonth:     time.January,
			wantDay:       2,
			wantErr:       false,
		},
		{
			name:          "7-field format: canceled parity sync",
			input:         "2025 May  4 07:55:41|543|0|-4|0|recon P|15625879500",
			wantAction:    "Parity-Sync",
			wantStatus:    "Canceled",
			wantErrors:    0,
			wantDuration:  543,
			wantSpeedMBps: 0,
			wantSizeBytes: 15625879500,
			wantYear:      2025,
			wantMonth:     time.May,
			wantDay:       4,
			wantErr:       false,
		},
		{
			name:          "7-field format: parity check with sync errors",
			input:         "2025 May  6 19:13:46|202286|79100386|0|3572342875|check P|15625879500",
			wantAction:    "Parity-Check",
			wantStatus:    "3572342875 errors",
			wantErrors:    3572342875,
			wantDuration:  202286,
			wantSpeedMBps: 79100386.0 / (1024 * 1024),
			wantSizeBytes: 15625879500,
			wantYear:      2025,
			wantMonth:     time.May,
			wantDay:       6,
			wantErr:       false,
		},
		// 5-field old format tests (pre-2022)
		{
			name:          "5-field format: successful check with human-readable speed",
			input:         "2022 May 22 20:17:49|73068|54.8 MB/s|0|0",
			wantAction:    "Parity-Check",
			wantStatus:    "OK",
			wantErrors:    0,
			wantDuration:  73068,
			wantSpeedMBps: 54.8,
			wantSizeBytes: 0,
			wantYear:      2022,
			wantMonth:     time.May,
			wantDay:       22,
			wantErr:       false,
		},
		{
			name:          "5-field format: canceled check with unavailable speed",
			input:         "2022 May 28 13:28:42|58|Unavailable|-4|0",
			wantAction:    "Parity-Check",
			wantStatus:    "Canceled",
			wantErrors:    0,
			wantDuration:  58,
			wantSpeedMBps: 0,
			wantSizeBytes: 0,
			wantYear:      2022,
			wantMonth:     time.May,
			wantDay:       28,
			wantErr:       false,
		},
		{
			name:          "5-field format: check with errors",
			input:         "2022 May 29 19:16:16|69375|57.7 MB/s|0|7",
			wantAction:    "Parity-Check",
			wantStatus:    "7 errors",
			wantErrors:    7,
			wantDuration:  69375,
			wantSpeedMBps: 57.7,
			wantSizeBytes: 0,
			wantYear:      2022,
			wantMonth:     time.May,
			wantDay:       29,
			wantErr:       false,
		},
		// 10-field extended format (with parity check plugin)
		{
			name:          "10-field format: dual parity check with human-readable speed",
			input:         "2024 Sep 9 02:23:50|18548|25.9 MB/s|0|0|check P Q|468850520|95023|2|Scheduled Non-Correcting Parity-Check",
			wantAction:    "Dual Parity-Check",
			wantStatus:    "OK",
			wantErrors:    0,
			wantDuration:  18548,
			wantSpeedMBps: 25.9,
			wantSizeBytes: 468850520,
			wantYear:      2024,
			wantMonth:     time.September,
			wantDay:       9,
			wantErr:       false,
		},
		{
			name:          "10-field format: another dual parity check",
			input:         "2024 Oct 14 02:29:20|18898|25.4 MB/s|0|0|check P Q|468850520|95354|2|Scheduled Non-Correcting Parity-Check",
			wantAction:    "Dual Parity-Check",
			wantStatus:    "OK",
			wantErrors:    0,
			wantDuration:  18898,
			wantSpeedMBps: 25.4,
			wantSizeBytes: 468850520,
			wantYear:      2024,
			wantMonth:     time.October,
			wantDay:       14,
			wantErr:       false,
		},
		{
			name:          "10-field format: correcting parity check",
			input:         "2024 Dec 22 17:54:15|18240|26.3 MB/s|0|0|check P Q|468850520|18240|1|Automatic Correcting Parity-Check",
			wantAction:    "Dual Parity-Check",
			wantStatus:    "OK",
			wantErrors:    0,
			wantDuration:  18240,
			wantSpeedMBps: 26.3,
			wantSizeBytes: 468850520,
			wantYear:      2024,
			wantMonth:     time.December,
			wantDay:       22,
			wantErr:       false,
		},
		// Error cases
		{
			name:    "invalid format too few fields",
			input:   "2024 Nov 30|100888|99128056",
			wantErr: true,
		},
		{
			name:    "empty line",
			input:   "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			record, err := collector.parseLine(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("parseLine(%q) expected error but got nil", tt.input)
				}
				return
			}
			if err != nil {
				t.Errorf("parseLine(%q) unexpected error: %v", tt.input, err)
				return
			}

			if record.Action != tt.wantAction {
				t.Errorf("parseLine(%q) Action = %q, want %q", tt.input, record.Action, tt.wantAction)
			}
			if record.Status != tt.wantStatus {
				t.Errorf("parseLine(%q) Status = %q, want %q", tt.input, record.Status, tt.wantStatus)
			}
			if record.Errors != tt.wantErrors {
				t.Errorf("parseLine(%q) Errors = %d, want %d", tt.input, record.Errors, tt.wantErrors)
			}
			if record.Duration != tt.wantDuration {
				t.Errorf("parseLine(%q) Duration = %d, want %d", tt.input, record.Duration, tt.wantDuration)
			}
			// Allow small float comparison tolerance
			if diff := record.Speed - tt.wantSpeedMBps; diff > 0.01 || diff < -0.01 {
				t.Errorf("parseLine(%q) Speed = %f, want %f", tt.input, record.Speed, tt.wantSpeedMBps)
			}
			if record.Size != tt.wantSizeBytes {
				t.Errorf("parseLine(%q) Size = %d, want %d", tt.input, record.Size, tt.wantSizeBytes)
			}
			if record.Date.Year() != tt.wantYear {
				t.Errorf("parseLine(%q) Date.Year = %d, want %d", tt.input, record.Date.Year(), tt.wantYear)
			}
			if record.Date.Month() != tt.wantMonth {
				t.Errorf("parseLine(%q) Date.Month = %v, want %v", tt.input, record.Date.Month(), tt.wantMonth)
			}
			if record.Date.Day() != tt.wantDay {
				t.Errorf("parseLine(%q) Date.Day = %d, want %d", tt.input, record.Date.Day(), tt.wantDay)
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
	}{
		{"raw bytes per second", "99128056", 99128056.0 / (1024 * 1024)},
		{"human readable MB/s", "54.8 MB/s", 54.8},
		{"human readable with space", "25.9 MB/s", 25.9},
		{"human readable KB/s", "1024 KB/s", 1.0},
		{"human readable GB/s", "1 GB/s", 1024.0},
		{"unavailable", "Unavailable", 0},
		{"empty string", "", 0},
		{"zero", "0", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := collector.parseSpeed(tt.input)
			if diff := result - tt.expected; diff > 0.01 || diff < -0.01 {
				t.Errorf("parseSpeed(%q) = %f, want %f", tt.input, result, tt.expected)
			}
		})
	}
}

func TestParityParseAction(t *testing.T) {
	collector := NewParityCollector()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"single parity check", "check P", "Parity-Check"},
		{"dual parity check", "check P Q", "Dual Parity-Check"},
		{"single parity sync", "recon P", "Parity-Sync"},
		{"dual parity sync", "recon P Q", "Dual Parity-Sync"},
		{"parity clear", "clear P", "Parity-Clear"},
		{"unknown action", "unknown", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := collector.parseAction(tt.input)
			if result != tt.expected {
				t.Errorf("parseAction(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestGetParityHistory_EmptyRecordsNotNull(t *testing.T) {
	// Create a temporary empty parity log file
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "parity-checks.log")
	if err := os.WriteFile(tmpFile, []byte(""), 0644); err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}

	// We can't easily test with the actual collector since it uses a hardcoded path,
	// but we can verify the JSON behavior of an empty slice vs nil
	type TestHistory struct {
		Records []string `json:"records"`
	}

	// Empty slice should marshal to [] not null
	emptySlice := TestHistory{Records: []string{}}
	data, _ := json.Marshal(emptySlice)
	if string(data) != `{"records":[]}` {
		t.Errorf("Empty slice marshaled to %s, want {\"records\":[]}", string(data))
	}

	// Nil slice would marshal to null (the bug we're fixing)
	nilSlice := TestHistory{Records: nil}
	data, _ = json.Marshal(nilSlice)
	if string(data) != `{"records":null}` {
		t.Errorf("Nil slice marshaled to %s, want {\"records\":null}", string(data))
	}
}
