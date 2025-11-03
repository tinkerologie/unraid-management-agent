package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

func setupTestServer() (*Server, *domain.Context) {
	ctx := &domain.Context{
		Config: domain.Config{
			Port: 8080,
		},
	}

	server := NewServer(ctx)
	return server, ctx
}

func TestHealthEndpoint(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("GET", "/api/v1/health", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Errorf("failed to parse response: %v", err)
	}

	if status, ok := response["status"].(string); !ok || status != "ok" {
		t.Errorf("expected status 'ok', got %v", response["status"])
	}
}

func TestSystemEndpoint(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("GET", "/api/v1/system", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	// handler setup
	server.router.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}

	var systemInfo dto.SystemInfo
	if err := json.Unmarshal(rr.Body.Bytes(), &systemInfo); err != nil {
		t.Errorf("failed to parse system info: %v", err)
	}

	// Verify required fields are present
	if systemInfo.Hostname == "" {
		t.Error("expected hostname to be set")
	}
}

func TestArrayEndpoint(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("GET", "/api/v1/array", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	// handler setup
	server.router.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}

	var arrayStatus dto.ArrayStatus
	if err := json.Unmarshal(rr.Body.Bytes(), &arrayStatus); err != nil {
		t.Errorf("failed to parse array status: %v", err)
	}
}

func TestDisksEndpoint(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("GET", "/api/v1/disks", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	// handler setup
	server.router.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}

	var disks []dto.DiskInfo
	if err := json.Unmarshal(rr.Body.Bytes(), &disks); err != nil {
		t.Errorf("failed to parse disks: %v", err)
	}
}

func TestDockerEndpoint(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("GET", "/api/v1/docker", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	// handler setup
	server.router.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}

	var containers []dto.ContainerInfo
	if err := json.Unmarshal(rr.Body.Bytes(), &containers); err != nil {
		t.Errorf("failed to parse containers: %v", err)
	}
}

func TestVMEndpoint(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("GET", "/api/v1/vm", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	// handler setup
	server.router.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}

	var vms []dto.VMInfo
	if err := json.Unmarshal(rr.Body.Bytes(), &vms); err != nil {
		t.Errorf("failed to parse VMs: %v", err)
	}
}

func TestUPSEndpoint(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("GET", "/api/v1/ups", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	// handler setup
	server.router.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}

	var upsStatus dto.UPSStatus
	if err := json.Unmarshal(rr.Body.Bytes(), &upsStatus); err != nil {
		t.Errorf("failed to parse UPS status: %v", err)
	}
}

func TestGPUEndpoint(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("GET", "/api/v1/gpu", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	// handler setup
	server.router.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}

	var gpuMetrics []*dto.GPUMetrics
	if err := json.Unmarshal(rr.Body.Bytes(), &gpuMetrics); err != nil {
		t.Errorf("failed to parse GPU metrics: %v", err)
	}
}

func TestDockerControlEndpoint(t *testing.T) {
	server, _ := setupTestServer()

	// Test start operation - note: this will fail validation but should return 400, not 404
	req, err := http.NewRequest("POST", "/api/v1/docker/test123/start", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// Expect 400 because container validation will fail (no actual Docker daemon in test)
	// or 500 if validation passes but operation fails
	if status := rr.Code; status != http.StatusBadRequest && status != http.StatusInternalServerError {
		t.Errorf("handler returned wrong status code: got %v want %v or %v", status, http.StatusBadRequest, http.StatusInternalServerError)
	}

	var response dto.Response
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Errorf("failed to parse response: %v", err)
	}
}

func TestDockerControlInvalidOperation(t *testing.T) {
	server, _ := setupTestServer()

	// Test with invalid container ID format (special characters that won't match route)
	// This will return 404 because the route pattern won't match
	req, err := http.NewRequest("POST", "/api/v1/docker/invalid!@#/start", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// Expect 404 because route won't match with special characters
	if status := rr.Code; status != http.StatusNotFound {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusNotFound)
	}
}

func TestVMControlEndpoint(t *testing.T) {
	server, _ := setupTestServer()

	// Test start operation - note: this will fail validation but should return 400, not 404
	req, err := http.NewRequest("POST", "/api/v1/vm/testvm/start", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// Expect 400 because VM validation will fail (no actual virsh in test)
	// or 500 if validation passes but operation fails
	if status := rr.Code; status != http.StatusBadRequest && status != http.StatusInternalServerError {
		t.Errorf("handler returned wrong status code: got %v want %v or %v", status, http.StatusBadRequest, http.StatusInternalServerError)
	}

	var response dto.Response
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Errorf("failed to parse response: %v", err)
	}
}

func TestCORS(t *testing.T) {
	server, _ := setupTestServer()

	// Test CORS headers on a GET request (OPTIONS won't match routes in gorilla/mux without explicit registration)
	req, err := http.NewRequest("GET", "/api/v1/health", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}

	// Check CORS headers are set by middleware
	if rr.Header().Get("Access-Control-Allow-Origin") == "" {
		t.Error("expected Access-Control-Allow-Origin header to be set")
	}
}

func TestNotFoundRoute(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("GET", "/api/v1/nonexistent", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	// handler setup
	server.router.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusNotFound {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusNotFound)
	}
}

func TestJSONContentType(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("GET", "/api/v1/health", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	// handler setup
	server.router.ServeHTTP(rr, req)

	contentType := rr.Header().Get("Content-Type")
	if !strings.Contains(contentType, "application/json") {
		t.Errorf("expected Content-Type to contain 'application/json', got %s", contentType)
	}
}

func BenchmarkHealthEndpoint(b *testing.B) {
	server, _ := setupTestServer()
	// handler setup

	req, _ := http.NewRequest("GET", "/api/v1/health", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rr := httptest.NewRecorder()
		server.router.ServeHTTP(rr, req)
	}
}

func BenchmarkSystemEndpoint(b *testing.B) {
	server, _ := setupTestServer()
	// handler setup

	req, _ := http.NewRequest("GET", "/api/v1/system", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rr := httptest.NewRecorder()
		server.router.ServeHTTP(rr, req)
	}
}
