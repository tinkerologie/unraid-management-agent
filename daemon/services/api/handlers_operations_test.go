package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gorilla/mux"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

// ===== Docker/VM Operation Success Path Tests =====

func TestHandleDockerOperation_Success(t *testing.T) {
	server, _ := setupTestServer()
	req := httptest.NewRequest("POST", "/api/v1/docker/abcdef123456/start", nil)
	req = mux.SetURLVars(req, map[string]string{"id": "abcdef123456"})
	rr := httptest.NewRecorder()

	server.handleDockerOperation(rr, req, "start", func(_ string) error { return nil })

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	var resp dto.Response
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if !resp.Success {
		t.Errorf("expected Success=true, got false: %s", resp.Message)
	}
}

func TestHandleDockerOperation_SuccessLongID(t *testing.T) {
	server, _ := setupTestServer()
	longID := "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"
	req := httptest.NewRequest("POST", "/api/v1/docker/"+longID+"/stop", nil)
	req = mux.SetURLVars(req, map[string]string{"id": longID})
	rr := httptest.NewRecorder()

	server.handleDockerOperation(rr, req, "stop", func(_ string) error { return nil })

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestHandleVMOperation_Success(t *testing.T) {
	server, _ := setupTestServer()
	req := httptest.NewRequest("POST", "/api/v1/vm/test-vm/start", nil)
	req = mux.SetURLVars(req, map[string]string{"name": "test-vm"})
	rr := httptest.NewRecorder()

	server.handleVMOperation(rr, req, "start", func(_ string) error { return nil })

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	var resp dto.Response
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if !resp.Success {
		t.Errorf("expected Success=true, got false: %s", resp.Message)
	}
}

func TestHandleVMOperation_SuccessWithSpaces(t *testing.T) {
	server, _ := setupTestServer()
	req := httptest.NewRequest("POST", "/api/v1/vm/Windows+10/stop", nil)
	req = mux.SetURLVars(req, map[string]string{"name": "Windows 10"})
	rr := httptest.NewRecorder()

	server.handleVMOperation(rr, req, "stop", func(_ string) error { return nil })

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

// ===== handleCollectorsStatus Legacy Fallback Tests =====

func TestHandleCollectorsStatus_LegacyFallback(t *testing.T) {
	server, _ := setupTestServer()
	// setupTestServer creates server with nil collectorManager,
	// which triggers the legacy fallback path.
	// Set intervals to exercise both enabled and disabled branches.
	server.ctx.Intervals.System = 15
	server.ctx.Intervals.Array = 30
	server.ctx.Intervals.Disk = 30
	server.ctx.Intervals.Docker = 30
	server.ctx.Intervals.VM = 30
	server.ctx.Intervals.UPS = 0 // disabled
	server.ctx.Intervals.NUT = 0 // disabled
	server.ctx.Intervals.GPU = 60
	server.ctx.Intervals.Shares = 60
	server.ctx.Intervals.Network = 30
	server.ctx.Intervals.Hardware = 300
	server.ctx.Intervals.ZFS = 30
	server.ctx.Intervals.Notification = 30
	server.ctx.Intervals.Registration = 300
	server.ctx.Intervals.Unassigned = 60

	req := httptest.NewRequest("GET", "/api/v1/collectors/status", nil)
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}

	var status dto.CollectorsStatusResponse
	if err := json.NewDecoder(rr.Body).Decode(&status); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if status.Total != 15 {
		t.Errorf("expected 15 collectors, got %d", status.Total)
	}
	if status.DisabledCount != 2 {
		t.Errorf("expected 2 disabled, got %d", status.DisabledCount)
	}
	if status.EnabledCount != 13 {
		t.Errorf("expected 13 enabled, got %d", status.EnabledCount)
	}

	// Verify the disabled collectors
	for _, c := range status.Collectors {
		if c.Name == "ups" || c.Name == "nut" {
			if c.Enabled {
				t.Errorf("expected %s to be disabled", c.Name)
			}
			if c.Status != "disabled" {
				t.Errorf("expected %s status=disabled, got %s", c.Name, c.Status)
			}
		}
	}
}

func TestHandleCollectorsStatus_LegacyAllDisabled(t *testing.T) {
	server, _ := setupTestServer()
	// All intervals zero = all disabled
	req := httptest.NewRequest("GET", "/api/v1/collectors/status", nil)
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}

	var status dto.CollectorsStatusResponse
	if err := json.NewDecoder(rr.Body).Decode(&status); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if status.DisabledCount != 15 {
		t.Errorf("expected 15 disabled, got %d", status.DisabledCount)
	}
	if status.EnabledCount != 0 {
		t.Errorf("expected 0 enabled, got %d", status.EnabledCount)
	}
}

// ===== handleLogFile tests =====

func TestHandleLogFile_InvalidFilename(t *testing.T) {
	server, _ := setupTestServer()
	tests := []struct {
		name     string
		filename string
	}{
		{"path_traversal", "../etc/passwd"},
		{"absolute_path", "/etc/shadow"},
		{"double_dot", ".."},
		{"backslash", "sys\\log"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/v1/logs/test", nil)
			req = mux.SetURLVars(req, map[string]string{"filename": tt.filename})
			rr := httptest.NewRecorder()
			server.handleLogFile(rr, req)

			if rr.Code != http.StatusBadRequest {
				t.Errorf("expected 400 for filename %q, got %d", tt.filename, rr.Code)
			}
		})
	}
}

func TestHandleLogFile_NotFound(t *testing.T) {
	server, _ := setupTestServer()

	// Override commonLogPaths to empty so no files are found
	oldPaths := commonLogPaths
	commonLogPaths = []string{}
	t.Cleanup(func() { commonLogPaths = oldPaths })

	req := httptest.NewRequest("GET", "/api/v1/logs/nonexistent.log", nil)
	req = mux.SetURLVars(req, map[string]string{"filename": "nonexistent.log"})
	rr := httptest.NewRecorder()
	server.handleLogFile(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

func TestHandleLogFile_Success(t *testing.T) {
	server, _ := setupTestServer()

	// Create temp log file
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "myapp.log")
	if err := os.WriteFile(logFile, []byte("line1\nline2\nline3\n"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Override commonLogPaths to include our temp file
	oldPaths := commonLogPaths
	commonLogPaths = []string{logFile}
	t.Cleanup(func() { commonLogPaths = oldPaths })

	req := httptest.NewRequest("GET", "/api/v1/logs/myapp.log", nil)
	req = mux.SetURLVars(req, map[string]string{"filename": "myapp.log"})
	rr := httptest.NewRecorder()
	server.handleLogFile(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}

	var content dto.LogFileContent
	if err := json.NewDecoder(rr.Body).Decode(&content); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if content.TotalLines != 3 {
		t.Errorf("expected 3 lines, got %d", content.TotalLines)
	}
}

func TestHandleLogFile_SuccessWithPagination(t *testing.T) {
	server, _ := setupTestServer()

	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "paginated.log")
	if err := os.WriteFile(logFile, []byte("a\nb\nc\nd\ne\n"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	oldPaths := commonLogPaths
	commonLogPaths = []string{logFile}
	t.Cleanup(func() { commonLogPaths = oldPaths })

	req := httptest.NewRequest("GET", "/api/v1/logs/paginated.log?lines=2&start=1", nil)
	req = mux.SetURLVars(req, map[string]string{"filename": "paginated.log"})
	rr := httptest.NewRecorder()
	server.handleLogFile(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}

	var content dto.LogFileContent
	if err := json.NewDecoder(rr.Body).Decode(&content); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if content.LinesReturned != 2 {
		t.Errorf("expected 2 lines returned, got %d", content.LinesReturned)
	}
}
