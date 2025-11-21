package lib

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFileExists(t *testing.T) {
	// Create temp file
	tmpFile, err := os.CreateTemp("", "test-*")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		{"existing file", tmpFile.Name(), true},
		{"non-existing file", "/nonexistent/path/file.txt", false},
		{"empty path", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FileExists(tt.path); got != tt.expected {
				t.Errorf("FileExists(%q) = %v, want %v", tt.path, got, tt.expected)
			}
		})
	}
}

func TestReadFile(t *testing.T) {
	// Create temp file with content
	tmpFile, err := os.CreateTemp("", "test-*")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	content := "test content\nline 2\nline 3"
	if _, err := tmpFile.WriteString(content); err != nil {
		t.Fatalf("Failed to write to temp file: %v", err)
	}
	tmpFile.Close()

	t.Run("read existing file", func(t *testing.T) {
		got, err := ReadFile(tmpFile.Name())
		if err != nil {
			t.Errorf("ReadFile() error = %v", err)
		}
		if got != content {
			t.Errorf("ReadFile() = %q, want %q", got, content)
		}
	})

	t.Run("read non-existing file", func(t *testing.T) {
		_, err := ReadFile("/nonexistent/file.txt")
		if err == nil {
			t.Error("ReadFile() expected error for non-existing file")
		}
	})
}

func TestReadLines(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "test-*")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	content := "line1\nline2\nline3"
	if _, err := tmpFile.WriteString(content); err != nil {
		t.Fatalf("Failed to write to temp file: %v", err)
	}
	tmpFile.Close()

	lines, err := ReadLines(tmpFile.Name())
	if err != nil {
		t.Fatalf("ReadLines() error = %v", err)
	}

	if len(lines) != 3 {
		t.Errorf("ReadLines() returned %d lines, want 3", len(lines))
	}

	expected := []string{"line1", "line2", "line3"}
	for i, line := range lines {
		if line != expected[i] {
			t.Errorf("ReadLines()[%d] = %q, want %q", i, line, expected[i])
		}
	}
}

func TestParseFloat(t *testing.T) {
	tests := []struct {
		input    string
		expected float64
	}{
		{"3.14", 3.14},
		{"  42.5  ", 42.5},
		{"-10.5", -10.5},
		{"0", 0},
		{"invalid", 0},
		{"", 0},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := ParseFloat(tt.input); got != tt.expected {
				t.Errorf("ParseFloat(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestParseInt(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"42", 42},
		{"  100  ", 100},
		{"-50", -50},
		{"0", 0},
		{"invalid", 0},
		{"3.14", 0},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := ParseInt(tt.input); got != tt.expected {
				t.Errorf("ParseInt(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestParseUint64(t *testing.T) {
	tests := []struct {
		input    string
		expected uint64
	}{
		{"42", 42},
		{"  100  ", 100},
		{"0", 0},
		{"18446744073709551615", 18446744073709551615}, // max uint64
		{"invalid", 0},
		{"-1", 0}, // negative is invalid for uint
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := ParseUint64(tt.input); got != tt.expected {
				t.Errorf("ParseUint64(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestRound(t *testing.T) {
	tests := []struct {
		input    float64
		expected int
	}{
		{3.4, 3},
		{3.5, 4},
		{3.6, 4},
		{-3.4, -3},
		{-3.5, -4},
		{-3.6, -4},
		{0.0, 0},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			if got := Round(tt.input); got != tt.expected {
				t.Errorf("Round(%v) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestRoundFloat(t *testing.T) {
	tests := []struct {
		input    float64
		decimals int
		expected float64
	}{
		{3.14159, 2, 3.14},
		{3.14159, 3, 3.142},
		{3.14159, 0, 3.0},
		{-3.14159, 2, -3.14},
		{0.0, 2, 0.0},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			got := RoundFloat(tt.input, tt.decimals)
			if got != tt.expected {
				t.Errorf("RoundFloat(%v, %d) = %v, want %v", tt.input, tt.decimals, got, tt.expected)
			}
		})
	}
}

func TestParseKeyValue(t *testing.T) {
	tests := []struct {
		input         string
		expectedKey   string
		expectedValue string
	}{
		{"key=value", "key", "value"},
		{"name=\"test\"", "name", "test"},
		{"  spaced  =  value  ", "spaced", "value"},
		{"multi=equals=signs", "multi", "equals=signs"},
		{"novalue", "", ""},
		{"", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			gotKey, gotValue := ParseKeyValue(tt.input)
			if gotKey != tt.expectedKey || gotValue != tt.expectedValue {
				t.Errorf("ParseKeyValue(%q) = (%q, %q), want (%q, %q)",
					tt.input, gotKey, gotValue, tt.expectedKey, tt.expectedValue)
			}
		})
	}
}

func TestParseKeyValueMap(t *testing.T) {
	lines := []string{
		"key1=value1",
		"key2=\"value2\"",
		"# comment",
		"",
		"key3=value3",
	}

	result := ParseKeyValueMap(lines)

	if len(result) != 3 {
		t.Errorf("ParseKeyValueMap() returned %d items, want 3", len(result))
	}

	expected := map[string]string{
		"key1": "value1",
		"key2": "value2",
		"key3": "value3",
	}

	for k, v := range expected {
		if result[k] != v {
			t.Errorf("ParseKeyValueMap()[%q] = %q, want %q", k, result[k], v)
		}
	}
}

func TestBytesToGB(t *testing.T) {
	tests := []struct {
		bytes    uint64
		expected float64
	}{
		{1073741824, 1.0},           // 1 GB
		{2147483648, 2.0},           // 2 GB
		{0, 0.0},                    // 0 bytes
		{536870912, 0.5},            // 0.5 GB
		{1099511627776, 1024.0},     // 1 TB
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			got := BytesToGB(tt.bytes)
			if got != tt.expected {
				t.Errorf("BytesToGB(%d) = %v, want %v", tt.bytes, got, tt.expected)
			}
		})
	}
}

func TestBytesToMB(t *testing.T) {
	tests := []struct {
		bytes    uint64
		expected float64
	}{
		{1048576, 1.0},      // 1 MB
		{2097152, 2.0},      // 2 MB
		{0, 0.0},            // 0 bytes
		{524288, 0.5},       // 0.5 MB
		{1073741824, 1024.0}, // 1 GB in MB
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			got := BytesToMB(tt.bytes)
			if got != tt.expected {
				t.Errorf("BytesToMB(%d) = %v, want %v", tt.bytes, got, tt.expected)
			}
		})
	}
}

func TestGBToBytes(t *testing.T) {
	tests := []struct {
		gb       float64
		expected uint64
	}{
		{1.0, 1073741824},
		{2.0, 2147483648},
		{0.0, 0},
		{0.5, 536870912},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			got := GBToBytes(tt.gb)
			if got != tt.expected {
				t.Errorf("GBToBytes(%v) = %d, want %d", tt.gb, got, tt.expected)
			}
		})
	}
}

func TestMBToBytes(t *testing.T) {
	tests := []struct {
		mb       float64
		expected uint64
	}{
		{1.0, 1048576},
		{2.0, 2097152},
		{0.0, 0},
		{0.5, 524288},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			got := MBToBytes(tt.mb)
			if got != tt.expected {
				t.Errorf("MBToBytes(%v) = %d, want %d", tt.mb, got, tt.expected)
			}
		})
	}
}

func TestKBToBytes(t *testing.T) {
	tests := []struct {
		kb       float64
		expected uint64
	}{
		{1.0, 1024},
		{2.0, 2048},
		{0.0, 0},
		{1024.0, 1048576}, // 1 MB
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			got := KBToBytes(tt.kb)
			if got != tt.expected {
				t.Errorf("KBToBytes(%v) = %d, want %d", tt.kb, got, tt.expected)
			}
		})
	}
}

func TestReadFileWithDirectory(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create nested file
	nestedPath := filepath.Join(tmpDir, "subdir", "test.txt")
	if err := os.MkdirAll(filepath.Dir(nestedPath), 0755); err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}

	content := "nested content"
	if err := os.WriteFile(nestedPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}

	got, err := ReadFile(nestedPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if got != content {
		t.Errorf("ReadFile() = %q, want %q", got, content)
	}
}
