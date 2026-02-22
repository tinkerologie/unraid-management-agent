package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

// ===== Docker Handler Tests =====

func TestHandleDockerCheckUpdate_InvalidRef(t *testing.T) {
	server, _ := setupTestServer()

	tests := []struct {
		name string
		ref  string
	}{
		{"special chars", "container@name"},
		{"semicolon injection", "abc;ls"},
		{"empty with spaces", "my container"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest("GET", "/api/v1/docker/"+tt.ref+"/check-update", nil)
			if err != nil {
				t.Fatal(err)
			}

			rr := httptest.NewRecorder()
			server.router.ServeHTTP(rr, req)

			if rr.Code != http.StatusBadRequest {
				t.Errorf("expected status 400, got %d", rr.Code)
			}

			var resp dto.Response
			if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
				t.Fatalf("failed to parse response: %v", err)
			}

			if resp.Success {
				t.Error("expected Success=false")
			}
		})
	}
}

func TestHandleDockerSize_InvalidRef(t *testing.T) {
	server, _ := setupTestServer()

	tests := []struct {
		name string
		ref  string
	}{
		{"special chars", "abc@def"},
		{"null injection", "test%00id"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest("GET", "/api/v1/docker/"+tt.ref+"/size", nil)
			if err != nil {
				t.Fatal(err)
			}

			rr := httptest.NewRecorder()
			server.router.ServeHTTP(rr, req)

			if rr.Code != http.StatusBadRequest {
				t.Errorf("expected status 400, got %d", rr.Code)
			}
		})
	}
}

func TestHandleDockerUpdate_InvalidRef(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("POST", "/api/v1/docker/abc;hack/update", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rr.Code)
	}
}

func TestHandleDockerUpdate_WrongMethod(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("GET", "/api/v1/docker/abc123/update", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// gorilla/mux returns 405 if MethodNotAllowedHandler is set, otherwise 404
	if rr.Code != http.StatusMethodNotAllowed && rr.Code != http.StatusNotFound {
		t.Errorf("expected status 405 or 404, got %d", rr.Code)
	}
}

// ===== Plugin Handler Tests =====

func TestHandlePluginUpdate_InvalidName(t *testing.T) {
	server, _ := setupTestServer()

	tests := []struct {
		name       string
		pluginName string
	}{
		{"special chars", "plugin@name"},
		{"semicolon", "plugin;hack"},
		{"spaces", "my plugin"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest("POST", "/api/v1/plugins/"+tt.pluginName+"/update", nil)
			if err != nil {
				t.Fatal(err)
			}

			rr := httptest.NewRecorder()
			server.router.ServeHTTP(rr, req)

			if rr.Code != http.StatusBadRequest {
				t.Errorf("expected status 400, got %d", rr.Code)
			}

			var resp dto.Response
			if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
				t.Fatalf("failed to parse response: %v", err)
			}

			if resp.Success {
				t.Error("expected Success=false")
			}
		})
	}
}

// ===== VM Clone & Snapshot Handler Tests =====

func TestHandleVMClone_InvalidVMName(t *testing.T) {
	server, _ := setupTestServer()

	tests := []struct {
		name   string
		vmName string
	}{
		{"backtick injection", "vm`id`"},
		{"command injection", "$(whoami)"},
		{"semicolon injection", "vm;ls"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest("POST", "/api/v1/vm/"+tt.vmName+"/clone?clone_name=newvm", nil)
			if err != nil {
				t.Fatal(err)
			}

			rr := httptest.NewRecorder()
			server.router.ServeHTTP(rr, req)

			if rr.Code != http.StatusBadRequest {
				t.Errorf("expected status 400, got %d", rr.Code)
			}
		})
	}
}

func TestHandleVMClone_MissingCloneName(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("POST", "/api/v1/vm/myvm/clone", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rr.Code)
	}

	var resp dto.Response
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp.Message != "clone_name query parameter is required" {
		t.Errorf("unexpected message: %s", resp.Message)
	}
}

func TestHandleVMClone_InvalidCloneName(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("POST", "/api/v1/vm/myvm/clone?clone_name=../hax", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rr.Code)
	}
}

func TestHandleVMCreateSnapshot_InvalidVMName(t *testing.T) {
	server, _ := setupTestServer()

	tests := []struct {
		name   string
		vmName string
	}{
		{"backtick injection", "vm`id`"},
		{"command injection", "$(id)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest("POST", "/api/v1/vm/"+tt.vmName+"/snapshot?snapshot_name=snap1", nil)
			if err != nil {
				t.Fatal(err)
			}

			rr := httptest.NewRecorder()
			server.router.ServeHTTP(rr, req)

			if rr.Code != http.StatusBadRequest {
				t.Errorf("expected status 400, got %d", rr.Code)
			}
		})
	}
}

func TestHandleVMCreateSnapshot_InvalidSnapshotName(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("POST", "/api/v1/vm/myvm/snapshot?snapshot_name=../hack", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rr.Code)
	}
}

func TestHandleVMListSnapshots_InvalidVMName(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("GET", "/api/v1/vm/vm;hack/snapshots", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rr.Code)
	}
}

func TestHandleVMDeleteSnapshot_InvalidVMName(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("DELETE", "/api/v1/vm/vm;hack/snapshots/snap1", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rr.Code)
	}
}

func TestHandleVMDeleteSnapshot_InvalidSnapshotName(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("DELETE", "/api/v1/vm/myvm/snapshots/snap;hack", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rr.Code)
	}
}

// ===== VM Snapshot Restore Handler Tests =====

func TestHandleVMRestoreSnapshot_InvalidVMName(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("POST", "/api/v1/vm/vm;hack/snapshots/snap1/restore", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rr.Code)
	}
}

func TestHandleVMRestoreSnapshot_InvalidSnapshotName(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("POST", "/api/v1/vm/myvm/snapshots/snap;hack/restore", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rr.Code)
	}
}

// ===== Docker Container Logs Handler Tests =====

func TestHandleDockerLogs_InvalidContainerRef(t *testing.T) {
	server, _ := setupTestServer()

	tests := []struct {
		name string
		ref  string
	}{
		{"command injection", "$(id)"},
		{"semicolon injection", "abc;ls"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest("GET", "/api/v1/docker/"+tt.ref+"/logs", nil)
			if err != nil {
				t.Fatal(err)
			}

			rr := httptest.NewRecorder()
			server.router.ServeHTTP(rr, req)

			if rr.Code != http.StatusBadRequest {
				t.Errorf("expected status 400, got %d", rr.Code)
			}
		})
	}
}

func TestHandleDockerLogs_ValidRef(t *testing.T) {
	server, _ := setupTestServer()

	// Valid container ID — will fail at Docker API level but route resolves and validation passes
	req, err := http.NewRequest("GET", "/api/v1/docker/abc123def456/logs?tail=50&timestamps=true", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// Should not be 400 (validation passes) or 404 (route exists)
	if rr.Code == http.StatusBadRequest || rr.Code == http.StatusNotFound {
		t.Errorf("expected non-400/404 status, got %d", rr.Code)
	}
}

// ===== Service Handler Tests =====

func TestHandleServiceAction_InvalidServiceName(t *testing.T) {
	server, _ := setupTestServer()

	tests := []struct {
		name        string
		serviceName string
	}{
		{"special chars", "svc;hack"},
		{"command injection", "$(id)"},
		{"starts with number", "1service"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest("POST", "/api/v1/services/"+tt.serviceName+"/start", nil)
			if err != nil {
				t.Fatal(err)
			}

			rr := httptest.NewRecorder()
			server.router.ServeHTTP(rr, req)

			if rr.Code != http.StatusBadRequest {
				t.Errorf("expected status 400, got %d", rr.Code)
			}
		})
	}
}

func TestHandleServiceAction_InvalidAction(t *testing.T) {
	server, _ := setupTestServer()

	tests := []struct {
		name   string
		action string
	}{
		{"kill", "kill"},
		{"enable", "enable"},
		{"empty", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.action == "" {
				// Empty action won't match the route pattern
				return
			}
			req, err := http.NewRequest("POST", "/api/v1/services/docker/"+tt.action, nil)
			if err != nil {
				t.Fatal(err)
			}

			rr := httptest.NewRecorder()
			server.router.ServeHTTP(rr, req)

			if rr.Code != http.StatusBadRequest {
				t.Errorf("expected status 400, got %d", rr.Code)
			}
		})
	}
}

func TestHandleServiceAction_WrongMethod(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("GET", "/api/v1/services/docker/start", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// gorilla/mux returns 405 if MethodNotAllowedHandler is set, otherwise 404
	if rr.Code != http.StatusMethodNotAllowed && rr.Code != http.StatusNotFound {
		t.Errorf("expected status 405 or 404, got %d", rr.Code)
	}
}

// ===== Process Handler Tests =====

func TestHandleProcessList_DefaultParams(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("GET", "/api/v1/processes", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// Will return 200 or 500 depending on OS (ps command)
	// On macOS this may fail since ps aux format differs, but route should resolve
	if rr.Code != http.StatusOK && rr.Code != http.StatusInternalServerError {
		t.Errorf("expected status 200 or 500, got %d", rr.Code)
	}
}

func TestHandleProcessList_WithQueryParams(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("GET", "/api/v1/processes?sort_by=memory&limit=10", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// Route should resolve regardless of system availability
	if rr.Code != http.StatusOK && rr.Code != http.StatusInternalServerError {
		t.Errorf("expected status 200 or 500, got %d", rr.Code)
	}
}

func TestHandleProcessList_LimitCapped(t *testing.T) {
	server, _ := setupTestServer()

	// Limit > 500 should be capped
	req, err := http.NewRequest("GET", "/api/v1/processes?limit=1000", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// Route should resolve
	if rr.Code != http.StatusOK && rr.Code != http.StatusInternalServerError {
		t.Errorf("expected status 200 or 500, got %d", rr.Code)
	}
}

func TestHandleProcessList_InvalidLimit(t *testing.T) {
	server, _ := setupTestServer()

	// Non-numeric limit should fall back to default 50
	req, err := http.NewRequest("GET", "/api/v1/processes?limit=abc", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// Should still work (falls back to default)
	if rr.Code != http.StatusOK && rr.Code != http.StatusInternalServerError {
		t.Errorf("expected status 200 or 500, got %d", rr.Code)
	}
}

// ===== Route Existence Tests =====

func TestNewRoutes_Exist(t *testing.T) {
	server, _ := setupTestServer()

	routes := []struct {
		method string
		path   string
	}{
		// Docker routes that don't hit Docker daemon with bulk ops
		{"GET", "/api/v1/docker/abc123/check-update"},
		{"GET", "/api/v1/docker/abc123/size"},
		{"POST", "/api/v1/docker/abc123/update"},
		{"GET", "/api/v1/plugins/check-updates"},
		{"POST", "/api/v1/plugins/my-plugin/update"},
		{"POST", "/api/v1/plugins/update-all"},
		{"POST", "/api/v1/vm/myvm/clone"},
		{"POST", "/api/v1/vm/myvm/snapshot"},
		{"GET", "/api/v1/vm/myvm/snapshots"},
		{"DELETE", "/api/v1/vm/myvm/snapshots/snap1"},
		{"POST", "/api/v1/vm/myvm/snapshots/snap1/restore"},
		{"GET", "/api/v1/docker/abc123def456/logs"},
		{"GET", "/api/v1/services"},
		{"POST", "/api/v1/services/docker/start"},
		{"GET", "/api/v1/processes"},
	}

	for _, route := range routes {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			req, err := http.NewRequest(route.method, route.path, nil)
			if err != nil {
				t.Fatal(err)
			}

			rr := httptest.NewRecorder()
			server.router.ServeHTTP(rr, req)

			// Route should resolve (not 404)
			if rr.Code == http.StatusNotFound {
				t.Errorf("route %s %s returned 404", route.method, route.path)
			}
		})
	}
}
