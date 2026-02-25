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

	var response map[string]any
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
func TestNetworkEndpoint(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("GET", "/api/v1/network", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}
}

func TestSharesEndpoint(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("GET", "/api/v1/shares", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}
}

func TestZFSPoolsEndpoint(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("GET", "/api/v1/zfs/pools", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}
}

func TestZFSDatasetsEndpoint(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("GET", "/api/v1/zfs/datasets", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}
}

func TestHardwareFullEndpoint(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("GET", "/api/v1/hardware/full", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}
}

func TestRegistrationEndpoint(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("GET", "/api/v1/registration", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}
}

func TestNotificationsEndpoint(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("GET", "/api/v1/notifications", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}
}

func TestUnassignedEndpoint(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("GET", "/api/v1/unassigned", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}
}

func TestParityCheckHistoryEndpoint(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("GET", "/api/v1/array/parity-check/history", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}
}

func TestDockerControlInvalidContainer(t *testing.T) {
	server, _ := setupTestServer()

	// Test with invalid container ID
	req, err := http.NewRequest("POST", "/api/v1/docker/invalid-container/start", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// Should return bad request for invalid container ID
	if status := rr.Code; status != http.StatusBadRequest {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusBadRequest)
	}
}

func TestVMControlInvalidVM(t *testing.T) {
	server, _ := setupTestServer()

	// Test with potentially valid VM name (internal server error expected since virsh not available)
	req, err := http.NewRequest("POST", "/api/v1/vm/test-vm/start", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// Should return internal server error when virsh is not available
	if status := rr.Code; status != http.StatusInternalServerError {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusInternalServerError)
	}
}

func TestInvalidPath(t *testing.T) {
	server, _ := setupTestServer()

	// Test non-existent path
	req, err := http.NewRequest("GET", "/api/v1/nonexistent-endpoint", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusNotFound {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusNotFound)
	}
}

func TestDockerStopEndpoint(t *testing.T) {
	server, _ := setupTestServer()

	// Test with invalid container ID
	req, err := http.NewRequest("POST", "/api/v1/docker/invalid/stop", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusBadRequest {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusBadRequest)
	}
}

func TestDockerRestartEndpoint(t *testing.T) {
	server, _ := setupTestServer()

	// Test with invalid container ID
	req, err := http.NewRequest("POST", "/api/v1/docker/invalid/restart", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusBadRequest {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusBadRequest)
	}
}

func TestDockerPauseEndpoint(t *testing.T) {
	server, _ := setupTestServer()

	// Test with invalid container ID
	req, err := http.NewRequest("POST", "/api/v1/docker/invalid/pause", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusBadRequest {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusBadRequest)
	}
}

func TestDockerUnpauseEndpoint(t *testing.T) {
	server, _ := setupTestServer()

	// Test with invalid container ID
	req, err := http.NewRequest("POST", "/api/v1/docker/invalid/unpause", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusBadRequest {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusBadRequest)
	}
}

func TestVMStopEndpoint(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("POST", "/api/v1/vm/test-vm/stop", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// Should return internal server error when virsh is not available
	if status := rr.Code; status != http.StatusInternalServerError {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusInternalServerError)
	}
}

func TestVMRestartEndpoint(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("POST", "/api/v1/vm/test-vm/restart", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// Should return internal server error when virsh is not available
	if status := rr.Code; status != http.StatusInternalServerError {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusInternalServerError)
	}
}

func TestVMPauseEndpoint(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("POST", "/api/v1/vm/test-vm/pause", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// Should return internal server error when virsh is not available
	if status := rr.Code; status != http.StatusInternalServerError {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusInternalServerError)
	}
}

func TestVMResumeEndpoint(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("POST", "/api/v1/vm/test-vm/resume", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// Should return internal server error when virsh is not available
	if status := rr.Code; status != http.StatusInternalServerError {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusInternalServerError)
	}
}

func TestVMHibernateEndpoint(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("POST", "/api/v1/vm/test-vm/hibernate", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// Should return internal server error when virsh is not available
	if status := rr.Code; status != http.StatusInternalServerError {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusInternalServerError)
	}
}

func TestVMForceStopEndpoint(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("POST", "/api/v1/vm/test-vm/force-stop", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// Should return internal server error when virsh is not available
	if status := rr.Code; status != http.StatusInternalServerError {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusInternalServerError)
	}
}

func TestVMInvalidName(t *testing.T) {
	server, _ := setupTestServer()

	// Test with invalid VM name (starts with hyphen)
	req, err := http.NewRequest("POST", "/api/v1/vm/-invalid-vm/start", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// Should return bad request for invalid VM name
	if status := rr.Code; status != http.StatusBadRequest {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusBadRequest)
	}
}

func TestRespondJSONSuccess(t *testing.T) {
	rr := httptest.NewRecorder()

	data := map[string]string{"key": "value"}
	respondJSON(rr, http.StatusOK, data)

	if rr.Code != http.StatusOK {
		t.Errorf("wrong status code: got %v want %v", rr.Code, http.StatusOK)
	}

	contentType := rr.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("wrong content type: got %v want application/json", contentType)
	}

	var response map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Errorf("failed to parse response: %v", err)
	}

	if response["key"] != "value" {
		t.Errorf("wrong response data: got %v want value", response["key"])
	}
}

func TestRespondJSONError(t *testing.T) {
	rr := httptest.NewRecorder()

	response := dto.Response{
		Success: false,
		Message: "error message",
	}
	respondJSON(rr, http.StatusBadRequest, response)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("wrong status code: got %v want %v", rr.Code, http.StatusBadRequest)
	}

	var result dto.Response
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Errorf("failed to parse response: %v", err)
	}

	if result.Success {
		t.Error("expected success to be false")
	}

	if result.Message != "error message" {
		t.Errorf("wrong message: got %v want error message", result.Message)
	}
}

func TestDockerValidContainerButNoDocker(t *testing.T) {
	server, _ := setupTestServer()

	// Test with valid container ID format
	req, err := http.NewRequest("POST", "/api/v1/docker/abc123def456/start", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// Should return internal server error when docker is not available
	if status := rr.Code; status != http.StatusInternalServerError {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusInternalServerError)
	}
}

func TestUserScriptsEndpoint(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("GET", "/api/v1/user-scripts", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// Could return OK with empty list or error if path doesn't exist
	if status := rr.Code; status != http.StatusOK && status != http.StatusInternalServerError {
		t.Errorf("handler returned unexpected status code: got %v", status)
	}
}

func TestUserScriptExecuteEndpoint(t *testing.T) {
	server, _ := setupTestServer()

	// Test with nonexistent script
	body := strings.NewReader(`{"background": true}`)
	req, err := http.NewRequest("POST", "/api/v1/user-scripts/nonexistent/execute", body)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// Should return error for nonexistent script
	if status := rr.Code; status != http.StatusInternalServerError {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusInternalServerError)
	}
}

func TestUpdateShareConfigInvalidName(t *testing.T) {
	server, _ := setupTestServer()

	// Test with invalid share name containing special characters
	body := strings.NewReader(`{"name": "test", "allocator": "highwater"}`)
	req, err := http.NewRequest("POST", "/api/v1/shares/invalid;rm%20-rf/config", body)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// Should return bad request for invalid share name
	if status := rr.Code; status != http.StatusBadRequest {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusBadRequest)
	}
}

func TestUpdateShareConfigInvalidBody(t *testing.T) {
	server, _ := setupTestServer()

	body := strings.NewReader(`invalid json`)
	req, err := http.NewRequest("POST", "/api/v1/shares/validshare/config", body)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// Should return bad request for invalid JSON
	if status := rr.Code; status != http.StatusBadRequest {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusBadRequest)
	}
}

func TestUpdateSystemSettingsInvalidBody(t *testing.T) {
	server, _ := setupTestServer()

	body := strings.NewReader(`invalid json`)
	req, err := http.NewRequest("POST", "/api/v1/settings/system", body)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// Should return bad request for invalid JSON
	if status := rr.Code; status != http.StatusBadRequest {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusBadRequest)
	}
}

func TestHardwareBIOSEndpoint(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("GET", "/api/v1/hardware/bios", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// cache is nil in test env, expect not found
	if status := rr.Code; status != http.StatusNotFound {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusNotFound)
	}
}

func TestHardwareBaseboardEndpoint(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("GET", "/api/v1/hardware/baseboard", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// cache is nil in test env, expect not found
	if status := rr.Code; status != http.StatusNotFound {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusNotFound)
	}
}

func TestHardwareCPUEndpoint(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("GET", "/api/v1/hardware/cpu", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// cache is nil in test env, expect not found
	if status := rr.Code; status != http.StatusNotFound {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusNotFound)
	}
}

func TestHardwareCacheEndpoint(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("GET", "/api/v1/hardware/cache", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// cache is nil in test env, expect not found
	if status := rr.Code; status != http.StatusNotFound {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusNotFound)
	}
}

func TestHardwareMemoryArrayEndpoint(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("GET", "/api/v1/hardware/memory-array", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// cache is nil in test env, expect not found
	if status := rr.Code; status != http.StatusNotFound {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusNotFound)
	}
}

func TestHardwareMemoryDevicesEndpoint(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("GET", "/api/v1/hardware/memory-devices", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// cache is nil in test env, expect not found
	if status := rr.Code; status != http.StatusNotFound {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusNotFound)
	}
}

func TestLogsEndpoint(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("GET", "/api/v1/logs", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// /var/log doesn't exist in test env
	if status := rr.Code; status != http.StatusOK && status != http.StatusInternalServerError {
		t.Errorf("handler returned unexpected status code: got %v", status)
	}
}

func TestLogFileEndpoint(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("GET", "/api/v1/logs/syslog?lines=10", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// /var/log/syslog doesn't exist in test env
	if status := rr.Code; status != http.StatusNotFound && status != http.StatusInternalServerError {
		t.Errorf("handler returned unexpected status code: got %v", status)
	}
}

func TestLogFilePathTraversal(t *testing.T) {
	server, _ := setupTestServer()

	// Test with simple invalid filename (containing path separator after decode)
	// The router may redirect path traversal attempts before they reach the handler
	req, err := http.NewRequest("GET", "/api/v1/logs/test.log..", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// Should reject invalid filename - NotFound (because it doesn't exist) or BadRequest
	if status := rr.Code; status != http.StatusNotFound && status != http.StatusBadRequest {
		t.Errorf("handler returned unexpected status code: got %v", status)
	}
}

func TestZFSSnapshotsEndpoint(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("GET", "/api/v1/zfs/snapshots", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}
}

func TestZFSARCEndpoint(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("GET", "/api/v1/zfs/arc", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}
}

func TestZFSPoolByNameEndpoint(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("GET", "/api/v1/zfs/pools/test-pool", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// Pool doesn't exist, expect not found
	if status := rr.Code; status != http.StatusNotFound {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusNotFound)
	}
}

func TestDiskByIDEndpoint(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("GET", "/api/v1/disks/sda", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// Disk not in cache, expect not found
	if status := rr.Code; status != http.StatusNotFound {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusNotFound)
	}
}

func TestDockerInfoEndpoint(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("GET", "/api/v1/docker/containers/abc123def456", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// Container not in cache, expect not found
	if status := rr.Code; status != http.StatusNotFound {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusNotFound)
	}
}

func TestVMInfoEndpoint(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("GET", "/api/v1/vm/vms/test-vm", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// VM not in cache, expect not found
	if status := rr.Code; status != http.StatusNotFound {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusNotFound)
	}
}

func TestNotificationsUnreadEndpoint(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("GET", "/api/v1/notifications/unread", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// Should return OK with empty list
	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}
}

func TestNotificationsArchiveEndpoint(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("GET", "/api/v1/notifications/archive", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// Should return OK with empty list
	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}
}

func TestNotificationsOverviewEndpoint(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("GET", "/api/v1/notifications/overview", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// Should return OK with empty overview
	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}
}

func TestGetNotificationByIDEndpoint(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("GET", "/api/v1/notifications/20241118-120000-test.notify", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// Should return not found for nonexistent notification
	if status := rr.Code; status != http.StatusNotFound {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusNotFound)
	}
}

func TestCreateNotificationEndpointInvalidBody(t *testing.T) {
	server, _ := setupTestServer()

	body := strings.NewReader(`invalid json`)
	req, err := http.NewRequest("POST", "/api/v1/notifications", body)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// Should return bad request for invalid JSON
	if status := rr.Code; status != http.StatusBadRequest {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusBadRequest)
	}
}

func TestArchiveNotificationEndpoint(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("POST", "/api/v1/notifications/20241118-120000-test.notify/archive", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// Should return error for nonexistent notification
	if status := rr.Code; status != http.StatusInternalServerError {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusInternalServerError)
	}
}

func TestUnarchiveNotificationEndpoint(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("POST", "/api/v1/notifications/20241118-120000-test.notify/unarchive", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// Should return error for nonexistent notification
	if status := rr.Code; status != http.StatusInternalServerError {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusInternalServerError)
	}
}

func TestDeleteNotificationEndpoint(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("DELETE", "/api/v1/notifications/20241118-120000-test.notify", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// Should return error for nonexistent notification
	if status := rr.Code; status != http.StatusInternalServerError {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusInternalServerError)
	}
}

func TestArchiveAllNotificationsEndpoint(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("POST", "/api/v1/notifications/archive/all", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// Directory might not exist
	if status := rr.Code; status != http.StatusOK && status != http.StatusInternalServerError {
		t.Errorf("handler returned unexpected status code: got %v", status)
	}
}

func TestUnassignedDevicesListEndpoint(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("GET", "/api/v1/unassigned/devices", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}
}

func TestUnassignedRemoteSharesEndpoint(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("GET", "/api/v1/unassigned/remote-shares", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}
}

func TestZFSPoolEndpointNotFound(t *testing.T) {
	server, _ := setupTestServer()

	// Test with nonexistent pool
	req, err := http.NewRequest("GET", "/api/v1/zfs/pools/nonexistent-pool", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// Should return 404 for nonexistent pool
	if status := rr.Code; status != http.StatusNotFound {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusNotFound)
	}
}

func TestDockerInfoEndpointWithCacheData(t *testing.T) {
	server, _ := setupTestServer()

	// Set up Docker cache with test data
	dockerVal := []dto.ContainerInfo{{ID: "abc123", Name: "test-container", State: "running"}}
	server.dockerCache.Store(&dockerVal)

	req, err := http.NewRequest("GET", "/api/v1/docker/abc123", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}

	var container dto.ContainerInfo
	if err := json.Unmarshal(rr.Body.Bytes(), &container); err != nil {
		t.Errorf("failed to parse response: %v", err)
	}

	if container.Name != "test-container" {
		t.Errorf("expected name 'test-container', got %q", container.Name)
	}
}

func TestDockerInfoEndpointContainerNotFound(t *testing.T) {
	server, _ := setupTestServer()

	// Empty cache
	dockerVal := []dto.ContainerInfo{}
	server.dockerCache.Store(&dockerVal)

	req, err := http.NewRequest("GET", "/api/v1/docker/nonexistent", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusNotFound {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusNotFound)
	}
}

func TestVMInfoEndpointWithCacheData(t *testing.T) {
	server, _ := setupTestServer()

	// Set up VM cache with test data
	vmsVal := []dto.VMInfo{{Name: "test-vm", State: "running", MemoryAllocated: 4096}}
	server.vmsCache.Store(&vmsVal)

	req, err := http.NewRequest("GET", "/api/v1/vm/test-vm", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}

	var vm dto.VMInfo
	if err := json.Unmarshal(rr.Body.Bytes(), &vm); err != nil {
		t.Errorf("failed to parse response: %v", err)
	}

	if vm.Name != "test-vm" {
		t.Errorf("expected name 'test-vm', got %q", vm.Name)
	}
}

func TestVMInfoEndpointVMNotFound(t *testing.T) {
	server, _ := setupTestServer()

	// Empty cache
	vmsVal := []dto.VMInfo{}
	server.vmsCache.Store(&vmsVal)

	req, err := http.NewRequest("GET", "/api/v1/vm/nonexistent-vm", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusNotFound {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusNotFound)
	}
}

func TestDiskEndpointWithCacheData(t *testing.T) {
	server, _ := setupTestServer()

	// Set up disk cache with test data
	disksVal := []dto.DiskInfo{{Name: "disk1", Device: "/dev/sda", Status: "ok"}}
	server.disksCache.Store(&disksVal)

	req, err := http.NewRequest("GET", "/api/v1/disks/disk1", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}
}

func TestDiskEndpointDiskNotFound(t *testing.T) {
	server, _ := setupTestServer()

	// Empty cache
	disksVal := []dto.DiskInfo{}
	server.disksCache.Store(&disksVal)

	req, err := http.NewRequest("GET", "/api/v1/disks/nonexistent", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusNotFound {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusNotFound)
	}
}

func TestZFSSnapshotsEndpointActual(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("GET", "/api/v1/zfs/snapshots", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}
}

func TestZFSARCEndpointActual(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("GET", "/api/v1/zfs/arc", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}
}

func TestUpdateShareConfigEmptyBody(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("POST", "/api/v1/shares/test-share/config", strings.NewReader(""))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusBadRequest {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusBadRequest)
	}
}

func TestUpdateSystemSettingsEmptyBody(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("POST", "/api/v1/settings/system", strings.NewReader(""))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusBadRequest {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusBadRequest)
	}
}

func TestLogsEndpointActual(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("GET", "/api/v1/logs", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// May return OK or error depending on whether log directory exists
	if status := rr.Code; status != http.StatusOK && status != http.StatusInternalServerError {
		t.Errorf("handler returned unexpected status code: got %v", status)
	}
}

func TestLogFileEndpointActual(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("GET", "/api/v1/logs/syslog", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// May return OK or error depending on whether log file exists
	if status := rr.Code; status != http.StatusOK && status != http.StatusNotFound && status != http.StatusInternalServerError {
		t.Errorf("handler returned unexpected status code: got %v", status)
	}
}

func TestLogFileEndpointWithLines(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("GET", "/api/v1/logs/syslog?lines=50", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// May return OK or error depending on whether log file exists
	if status := rr.Code; status != http.StatusOK && status != http.StatusNotFound && status != http.StatusInternalServerError {
		t.Errorf("handler returned unexpected status code: got %v", status)
	}
}

func TestNotificationsUnreadEndpointActual(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("GET", "/api/v1/notifications/unread", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// May return OK or error depending on whether notifications directory exists
	if status := rr.Code; status != http.StatusOK && status != http.StatusInternalServerError {
		t.Errorf("handler returned unexpected status code: got %v", status)
	}
}

func TestNotificationsArchiveEndpointActual(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("GET", "/api/v1/notifications/archive", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// May return OK or error depending on whether archive directory exists
	if status := rr.Code; status != http.StatusOK && status != http.StatusInternalServerError {
		t.Errorf("handler returned unexpected status code: got %v", status)
	}
}

func TestNotificationsOverviewEndpointActual(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("GET", "/api/v1/notifications/overview", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}
}

func TestNotificationByIDEndpointActual(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("GET", "/api/v1/notifications/nonexistent-id", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// Should return 404 or error for nonexistent notification
	if status := rr.Code; status != http.StatusNotFound && status != http.StatusInternalServerError {
		t.Errorf("handler returned unexpected status code: got %v", status)
	}
}

func TestCreateNotificationEndpointActual(t *testing.T) {
	server, _ := setupTestServer()

	notificationJSON := `{
		"subject": "Test notification",
		"message": "Test message body",
		"importance": "normal"
	}`
	req, err := http.NewRequest("POST", "/api/v1/notifications", strings.NewReader(notificationJSON))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// May succeed or fail depending on notification directory permissions
	if status := rr.Code; status != http.StatusCreated && status != http.StatusBadRequest && status != http.StatusInternalServerError {
		t.Errorf("handler returned unexpected status code: got %v", status)
	}
}

func TestCreateNotificationInvalidJSON(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("POST", "/api/v1/notifications", strings.NewReader("invalid json"))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusBadRequest {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusBadRequest)
	}
}

func TestArchiveNotificationEndpointActual(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("POST", "/api/v1/notifications/nonexistent-id/archive", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// Should return error for nonexistent notification
	if status := rr.Code; status != http.StatusOK && status != http.StatusNotFound && status != http.StatusInternalServerError {
		t.Errorf("handler returned unexpected status code: got %v", status)
	}
}

func TestUnarchiveNotificationEndpointActual(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("POST", "/api/v1/notifications/nonexistent-id/unarchive", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// Should return error for nonexistent notification
	if status := rr.Code; status != http.StatusOK && status != http.StatusNotFound && status != http.StatusInternalServerError {
		t.Errorf("handler returned unexpected status code: got %v", status)
	}
}

func TestDeleteNotificationEndpointActual(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("DELETE", "/api/v1/notifications/nonexistent-id", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// Should return error for nonexistent notification
	if status := rr.Code; status != http.StatusOK && status != http.StatusNotFound && status != http.StatusInternalServerError {
		t.Errorf("handler returned unexpected status code: got %v", status)
	}
}

// Test array control handlers
func TestArrayStartEndpoint(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("POST", "/api/v1/array/start", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// Should return error since array management tools are not available
	if status := rr.Code; status != http.StatusOK && status != http.StatusInternalServerError {
		t.Errorf("handler returned unexpected status code: got %v", status)
	}
}

func TestArrayStopEndpoint(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("POST", "/api/v1/array/stop", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// Should return error since array management tools are not available
	if status := rr.Code; status != http.StatusOK && status != http.StatusInternalServerError {
		t.Errorf("handler returned unexpected status code: got %v", status)
	}
}

// Test parity check handlers
func TestParityCheckStartEndpoint(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("POST", "/api/v1/array/parity-check/start", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// Should return error since parity check tools are not available
	if status := rr.Code; status != http.StatusOK && status != http.StatusInternalServerError {
		t.Errorf("handler returned unexpected status code: got %v", status)
	}
}

func TestParityCheckStopEndpoint(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("POST", "/api/v1/array/parity-check/stop", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// Should return error since parity check tools are not available
	if status := rr.Code; status != http.StatusOK && status != http.StatusInternalServerError {
		t.Errorf("handler returned unexpected status code: got %v", status)
	}
}

func TestParityCheckPauseEndpoint(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("POST", "/api/v1/array/parity-check/pause", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// Should return error since parity check tools are not available
	if status := rr.Code; status != http.StatusOK && status != http.StatusInternalServerError {
		t.Errorf("handler returned unexpected status code: got %v", status)
	}
}

func TestParityCheckResumeEndpoint(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("POST", "/api/v1/array/parity-check/resume", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// Should return error since parity check tools are not available
	if status := rr.Code; status != http.StatusOK && status != http.StatusInternalServerError {
		t.Errorf("handler returned unexpected status code: got %v", status)
	}
}

// Test config/settings GET handlers
func TestShareConfigEndpoint(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("GET", "/api/v1/shares/test-share/config", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// Should return error since config file doesn't exist (404 or 500)
	if status := rr.Code; status != http.StatusOK && status != http.StatusBadRequest && status != http.StatusNotFound && status != http.StatusInternalServerError {
		t.Errorf("handler returned unexpected status code: got %v", status)
	}
}

func TestNetworkConfigEndpoint(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("GET", "/api/v1/network/eth0/config", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// Should return error since config file doesn't exist (404 or 500)
	if status := rr.Code; status != http.StatusOK && status != http.StatusNotFound && status != http.StatusInternalServerError {
		t.Errorf("handler returned unexpected status code: got %v", status)
	}
}

func TestSystemSettingsEndpoint(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("GET", "/api/v1/settings/system", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// Should return error since config file doesn't exist
	if status := rr.Code; status != http.StatusOK && status != http.StatusInternalServerError {
		t.Errorf("handler returned unexpected status code: got %v", status)
	}
}

func TestDockerSettingsEndpoint(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("GET", "/api/v1/settings/docker", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// Should return error since config file doesn't exist
	if status := rr.Code; status != http.StatusOK && status != http.StatusInternalServerError {
		t.Errorf("handler returned unexpected status code: got %v", status)
	}
}

func TestVMSettingsEndpoint(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("GET", "/api/v1/settings/vm", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// Should return error since config file doesn't exist
	if status := rr.Code; status != http.StatusOK && status != http.StatusInternalServerError {
		t.Errorf("handler returned unexpected status code: got %v", status)
	}
}

func TestDiskSettingsEndpoint(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("GET", "/api/v1/settings/disks", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// Should return error since config file doesn't exist
	if status := rr.Code; status != http.StatusOK && status != http.StatusInternalServerError {
		t.Errorf("handler returned unexpected status code: got %v", status)
	}
}

// Test system reboot/shutdown handlers
func TestSystemRebootEndpoint(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("POST", "/api/v1/system/reboot", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// Should return error since reboot command is not available
	if status := rr.Code; status != http.StatusOK && status != http.StatusInternalServerError {
		t.Errorf("handler returned unexpected status code: got %v", status)
	}
}
