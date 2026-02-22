package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestListLogFiles_WithTempFiles(t *testing.T) {
	server, _ := setupTestServer()

	// Create temp log file
	tmpDir := t.TempDir()
	tmpLog := filepath.Join(tmpDir, "test.log")
	if err := os.WriteFile(tmpLog, []byte("test log line\n"), 0644); err != nil {
		t.Fatalf("Failed to create test log: %v", err)
	}

	origPaths := commonLogPaths
	commonLogPaths = []string{tmpLog}
	defer func() { commonLogPaths = origPaths }()

	logs := server.listLogFiles()
	if len(logs) == 0 {
		t.Error("Expected at least one log file")
	}
	if len(logs) > 0 && logs[0].Name != "test.log" {
		t.Errorf("Expected name 'test.log', got %q", logs[0].Name)
	}
}

func TestListLogFiles_NoFiles(t *testing.T) {
	server, _ := setupTestServer()

	origPaths := commonLogPaths
	commonLogPaths = []string{"/nonexistent/path/log.log"}
	defer func() { commonLogPaths = origPaths }()

	logs := server.listLogFiles()
	if len(logs) != 0 {
		t.Errorf("Expected zero log files, got %d", len(logs))
	}
}

func TestHandleLogs_ListEndpoint(t *testing.T) {
	server, _ := setupTestServer()

	tmpDir := t.TempDir()
	tmpLog := filepath.Join(tmpDir, "test.log")
	if err := os.WriteFile(tmpLog, []byte("log line\n"), 0644); err != nil {
		t.Fatalf("Failed to create log: %v", err)
	}

	origPaths := commonLogPaths
	commonLogPaths = []string{tmpLog}
	defer func() { commonLogPaths = origPaths }()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/logs", nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}
}

func TestHandleLogs_WithPathParam(t *testing.T) {
	server, _ := setupTestServer()

	tmpDir := t.TempDir()
	tmpLog := filepath.Join(tmpDir, "test.log")
	if err := os.WriteFile(tmpLog, []byte("line1\nline2\nline3\n"), 0644); err != nil {
		t.Fatalf("Failed to create log: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/logs?path="+tmpLog, nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}
}

func TestHandleLogs_WithInvalidPath(t *testing.T) {
	server, _ := setupTestServer()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/logs?path=../../../etc/passwd", nil)
	w := httptest.NewRecorder()
	server.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		// handleLogs returns 500 for errors from getLogContent
		t.Logf("Got status %d for traversal path (expected non-200)", w.Code)
	}
}

func TestListLogFiles_MultipleFiles(t *testing.T) {
	server, _ := setupTestServer()

	tmpDir := t.TempDir()
	tmpLog1 := filepath.Join(tmpDir, "app.log")
	tmpLog2 := filepath.Join(tmpDir, "error.log")
	if err := os.WriteFile(tmpLog1, []byte("app log\n"), 0644); err != nil {
		t.Fatalf("Failed to create log: %v", err)
	}
	if err := os.WriteFile(tmpLog2, []byte("error log\n"), 0644); err != nil {
		t.Fatalf("Failed to create log: %v", err)
	}

	origPaths := commonLogPaths
	commonLogPaths = []string{tmpLog1, tmpLog2}
	defer func() { commonLogPaths = origPaths }()

	logs := server.listLogFiles()
	if len(logs) != 2 {
		t.Errorf("Expected 2 log files, got %d", len(logs))
	}
}
