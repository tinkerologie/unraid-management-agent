package api

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetLogContent_DirectoryTraversal(t *testing.T) {
	server, _ := setupTestServer()

	tests := []struct {
		name string
		path string
	}{
		{"relative path up", "../etc/passwd"},
		{"double dot in middle", "/var/log/../etc/passwd"},
		{"multiple traversal", "../../etc/passwd"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := server.getLogContent(tt.path, "", "")
			if err == nil {
				t.Error("Expected error for directory traversal, got nil")
			}
			if err.Error() != "invalid path: directory traversal not allowed" {
				t.Errorf("Expected directory traversal error, got: %v", err)
			}
		})
	}
}

func TestGetLogContent_FileNotFound(t *testing.T) {
	server, _ := setupTestServer()

	_, err := server.getLogContent("/nonexistent/file.log", "", "")
	if err == nil {
		t.Fatal("Expected error for non-existent file")
	}
	if err.Error() != "log file not found: /nonexistent/file.log" {
		t.Errorf("Expected file not found error, got: %v", err)
	}
}

func TestGetLogContent_FullFile(t *testing.T) {
	server, _ := setupTestServer()

	// Create a temporary log file
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "test.log")
	content := "line 1\nline 2\nline 3\nline 4\nline 5\n"
	if err := os.WriteFile(logFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	result, err := server.getLogContent(logFile, "", "")
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if result.TotalLines != 5 {
		t.Errorf("Expected 5 total lines, got %d", result.TotalLines)
	}

	if result.LinesReturned != 5 {
		t.Errorf("Expected 5 lines returned, got %d", result.LinesReturned)
	}

	if len(result.Lines) != 5 {
		t.Errorf("Expected 5 lines in array, got %d", len(result.Lines))
	}

	if result.StartLine != 0 {
		t.Errorf("Expected start line 0, got %d", result.StartLine)
	}

	if result.EndLine != 5 {
		t.Errorf("Expected end line 5, got %d", result.EndLine)
	}

	if result.Path != logFile {
		t.Errorf("Expected path %s, got %s", logFile, result.Path)
	}
}

func TestGetLogContent_WithLinesParam(t *testing.T) {
	server, _ := setupTestServer()

	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "test.log")
	content := "line 1\nline 2\nline 3\nline 4\nline 5\nline 6\nline 7\nline 8\nline 9\nline 10\n"
	if err := os.WriteFile(logFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Test tail behavior - last 3 lines
	result, err := server.getLogContent(logFile, "3", "")
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if result.TotalLines != 10 {
		t.Errorf("Expected 10 total lines, got %d", result.TotalLines)
	}

	if result.LinesReturned != 3 {
		t.Errorf("Expected 3 lines returned, got %d", result.LinesReturned)
	}

	if result.StartLine != 7 {
		t.Errorf("Expected start line 7, got %d", result.StartLine)
	}

	if result.Lines[0] != "line 8" {
		t.Errorf("Expected first line to be 'line 8', got '%s'", result.Lines[0])
	}

	if result.Lines[2] != "line 10" {
		t.Errorf("Expected last line to be 'line 10', got '%s'", result.Lines[2])
	}
}

func TestGetLogContent_WithStartAndLines(t *testing.T) {
	server, _ := setupTestServer()

	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "test.log")
	content := "line 1\nline 2\nline 3\nline 4\nline 5\nline 6\nline 7\nline 8\nline 9\nline 10\n"
	if err := os.WriteFile(logFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Get lines 2-4 (indices 2, 3, 4)
	result, err := server.getLogContent(logFile, "3", "2")
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if result.LinesReturned != 3 {
		t.Errorf("Expected 3 lines returned, got %d", result.LinesReturned)
	}

	if result.StartLine != 2 {
		t.Errorf("Expected start line 2, got %d", result.StartLine)
	}

	if result.EndLine != 5 {
		t.Errorf("Expected end line 5, got %d", result.EndLine)
	}

	if result.Lines[0] != "line 3" {
		t.Errorf("Expected first line to be 'line 3', got '%s'", result.Lines[0])
	}
}

func TestGetLogContent_StartBeyondFileSize(t *testing.T) {
	server, _ := setupTestServer()

	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "test.log")
	content := "line 1\nline 2\nline 3\n"
	if err := os.WriteFile(logFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	result, err := server.getLogContent(logFile, "5", "100")
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if result.LinesReturned != 0 {
		t.Errorf("Expected 0 lines returned, got %d", result.LinesReturned)
	}

	if len(result.Lines) != 0 {
		t.Errorf("Expected empty lines array, got %d lines", len(result.Lines))
	}
}

func TestGetLogContent_NegativeStart(t *testing.T) {
	server, _ := setupTestServer()

	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "test.log")
	content := "line 1\nline 2\nline 3\n"
	if err := os.WriteFile(logFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	result, err := server.getLogContent(logFile, "2", "-5")
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Should start from 0
	if result.StartLine != 0 {
		t.Errorf("Expected start line 0 (adjusted from negative), got %d", result.StartLine)
	}

	if result.LinesReturned != 2 {
		t.Errorf("Expected 2 lines returned, got %d", result.LinesReturned)
	}
}

func TestGetLogContent_LinesMoreThanTotal(t *testing.T) {
	server, _ := setupTestServer()

	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "test.log")
	content := "line 1\nline 2\nline 3\n"
	if err := os.WriteFile(logFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Request more lines than file has (tail behavior)
	result, err := server.getLogContent(logFile, "100", "")
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if result.LinesReturned != 3 {
		t.Errorf("Expected 3 lines returned (all available), got %d", result.LinesReturned)
	}

	if result.StartLine != 0 {
		t.Errorf("Expected start line 0, got %d", result.StartLine)
	}
}

func TestGetLogContent_EmptyFile(t *testing.T) {
	server, _ := setupTestServer()

	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "empty.log")
	if err := os.WriteFile(logFile, []byte(""), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	result, err := server.getLogContent(logFile, "", "")
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if result.TotalLines != 0 {
		t.Errorf("Expected 0 total lines, got %d", result.TotalLines)
	}

	if result.LinesReturned != 0 {
		t.Errorf("Expected 0 lines returned, got %d", result.LinesReturned)
	}
}

func TestGetLogContent_SingleLine(t *testing.T) {
	server, _ := setupTestServer()

	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "single.log")
	if err := os.WriteFile(logFile, []byte("single line\n"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	result, err := server.getLogContent(logFile, "", "")
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if result.TotalLines != 1 {
		t.Errorf("Expected 1 total line, got %d", result.TotalLines)
	}

	if result.Lines[0] != "single line" {
		t.Errorf("Expected 'single line', got '%s'", result.Lines[0])
	}
}

func TestGetLogContent_InvalidLineParam(t *testing.T) {
	server, _ := setupTestServer()

	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "test.log")
	content := "line 1\nline 2\nline 3\n"
	if err := os.WriteFile(logFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Invalid lines param - should be ignored, return all lines
	result, err := server.getLogContent(logFile, "invalid", "")
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// When lines param is invalid, it's treated as not specified, returns all
	if result.TotalLines != 3 {
		t.Errorf("Expected 3 total lines, got %d", result.TotalLines)
	}
}

func TestListLogFiles(t *testing.T) {
	server, _ := setupTestServer()

	// This just checks that listLogFiles doesn't crash
	// It will return an empty list or files from system
	files := server.listLogFiles()

	// Should return a slice (may be empty)
	if files == nil {
		t.Error("Expected non-nil slice from listLogFiles")
	}

	// Each file should have a Path
	for _, file := range files {
		if file.Path == "" {
			t.Error("Expected non-empty Path for log file")
		}
	}
}
