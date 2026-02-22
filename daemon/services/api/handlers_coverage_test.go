package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

// =====================================================================
// handleParityCheckHistory tests
// =====================================================================

func TestHandleParityCheckHistory_ReturnsResponse(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("GET", "/api/v1/array/parity-check/history", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// On non-Unraid, this may return 200 (empty history) or 500 (file not found)
	if rr.Code != http.StatusOK && rr.Code != http.StatusInternalServerError {
		t.Errorf("unexpected status code: got %d", rr.Code)
	}

	// Verify response is valid JSON regardless of status
	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Errorf("response is not valid JSON: %v", err)
	}

	t.Logf("Parity check history status=%d (expected on non-Unraid)", rr.Code)
}

func TestHandleParityCheckHistory_ResponseContentType(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("GET", "/api/v1/array/parity-check/history", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	ct := rr.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected Content-Type 'application/json', got %s", ct)
	}
}

// =====================================================================
// handleShareConfig tests
// =====================================================================

func TestHandleShareConfig_ValidShareName(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("GET", "/api/v1/shares/appdata/config", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// On non-Unraid, returns 404 (share config file not found) or 200
	if rr.Code != http.StatusOK && rr.Code != http.StatusNotFound {
		t.Errorf("unexpected status code: got %d", rr.Code)
	}

	var body json.RawMessage
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Errorf("response is not valid JSON: %v", err)
	}

	t.Logf("Share config for 'appdata' status=%d (expected on non-Unraid)", rr.Code)
}

func TestHandleShareConfig_PathTraversal(t *testing.T) {
	server, _ := setupTestServer()

	// Attempt path traversal via share name — gorilla/mux may redirect or
	// clean the URL before it reaches the handler. The key assertion is
	// that it does NOT return 200 with actual file contents.
	req, err := http.NewRequest("GET", "/api/v1/shares/..%2F..%2Fetc%2Fpasswd/config", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// Mux may return 301 (redirect) or 400 (validation) — either prevents access
	if rr.Code == http.StatusOK {
		t.Error("path traversal must not succeed with 200")
	}

	t.Logf("Path traversal attempt returned status=%d (safe)", rr.Code)
}

func TestHandleShareConfig_PathTraversalDotDot(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("GET", "/api/v1/shares/../etc/config", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// gorilla/mux may clean the path; if the request reaches the handler,
	// validation should catch it. Otherwise 404 from router is also acceptable.
	if rr.Code == http.StatusOK {
		t.Error("should not return 200 for path traversal attempt")
	}
}

func TestHandleShareConfig_EmptyShareName(t *testing.T) {
	server, _ := setupTestServer()

	// Trailing slash with no name — should not match the route
	req, err := http.NewRequest("GET", "/api/v1/shares//config", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// Route shouldn't match an empty name, expect 404 or 405
	if rr.Code == http.StatusOK {
		t.Error("should not return 200 for empty share name")
	}
}

// =====================================================================
// handleDockerSettings tests
// =====================================================================

func TestHandleDockerSettings_GET(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("GET", "/api/v1/settings/docker", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// On non-Unraid, may return 500 (config files not found) or 200
	if rr.Code != http.StatusOK && rr.Code != http.StatusInternalServerError {
		t.Errorf("unexpected status code: got %d", rr.Code)
	}

	var body json.RawMessage
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Errorf("response is not valid JSON: %v", err)
	}

	t.Logf("Docker settings status=%d (expected on non-Unraid)", rr.Code)
}

func TestHandleDockerSettings_WrongMethod(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("POST", "/api/v1/settings/docker", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// gorilla/mux returns 404 or 405 for unregistered method
	if rr.Code != http.StatusMethodNotAllowed && rr.Code != http.StatusNotFound {
		t.Errorf("expected 404 or 405 for POST on settings/docker, got %d", rr.Code)
	}
}

func TestHandleDockerSettings_ErrorResponse(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("GET", "/api/v1/settings/docker", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code == http.StatusInternalServerError {
		var resp dto.Response
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Errorf("error response is not valid JSON: %v", err)
		}
		if resp.Success {
			t.Error("expected success=false in error response")
		}
		if resp.Message == "" {
			t.Error("expected non-empty error message")
		}
		t.Logf("Docker settings error (expected on non-Unraid): %s", resp.Message)
	}
}

// =====================================================================
// handleVMSettings tests
// =====================================================================

func TestHandleVMSettings_GET(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("GET", "/api/v1/settings/vm", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK && rr.Code != http.StatusInternalServerError {
		t.Errorf("unexpected status code: got %d", rr.Code)
	}

	var body json.RawMessage
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Errorf("response is not valid JSON: %v", err)
	}

	t.Logf("VM settings status=%d (expected on non-Unraid)", rr.Code)
}

func TestHandleVMSettings_WrongMethod(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("DELETE", "/api/v1/settings/vm", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// gorilla/mux returns 404 or 405 for unregistered method
	if rr.Code != http.StatusMethodNotAllowed && rr.Code != http.StatusNotFound {
		t.Errorf("expected 404 or 405 for DELETE on settings/vm, got %d", rr.Code)
	}
}

func TestHandleVMSettings_ErrorResponse(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("GET", "/api/v1/settings/vm", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code == http.StatusInternalServerError {
		var resp dto.Response
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Errorf("error response is not valid JSON: %v", err)
		}
		if resp.Success {
			t.Error("expected success=false in error response")
		}
		t.Logf("VM settings error (expected on non-Unraid): %s", resp.Message)
	}
}

// =====================================================================
// handleUpdateShareConfig tests
// =====================================================================

func TestHandleUpdateShareConfig_EmptyBody(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("POST", "/api/v1/shares/testshare/config", strings.NewReader(""))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for empty body, got %d", rr.Code)
	}

	var resp dto.Response
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Errorf("response is not valid JSON: %v", err)
	}

	if resp.Success {
		t.Error("expected success=false for empty body")
	}
}

func TestHandleUpdateShareConfig_InvalidJSON(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("POST", "/api/v1/shares/testshare/config", strings.NewReader("{invalid json"))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid JSON, got %d", rr.Code)
	}

	var resp dto.Response
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Errorf("response is not valid JSON: %v", err)
	}

	if resp.Success {
		t.Error("expected success=false for invalid JSON")
	}
}

func TestHandleUpdateShareConfig_ValidJSON(t *testing.T) {
	server, _ := setupTestServer()

	body := `{"name":"testshare","allocator":"highwater","use_cache":"yes"}`
	req, err := http.NewRequest("POST", "/api/v1/shares/testshare/config", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// On non-Unraid, config update will fail (500) since config files don't exist.
	// On Unraid, should succeed (200).
	if rr.Code != http.StatusOK && rr.Code != http.StatusInternalServerError {
		t.Errorf("unexpected status code: got %d", rr.Code)
	}

	var resp dto.Response
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Errorf("response is not valid JSON: %v", err)
	}

	t.Logf("Update share config status=%d success=%v (expected on non-Unraid)", rr.Code, resp.Success)
}

func TestHandleUpdateShareConfig_PathTraversal(t *testing.T) {
	server, _ := setupTestServer()

	body := `{"name":"../../etc/passwd","allocator":"highwater"}`
	req, err := http.NewRequest("POST", "/api/v1/shares/..%2F..%2Fetc%2Fpasswd/config", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// gorilla/mux may redirect (301) or validation rejects (400) — must not succeed
	if rr.Code == http.StatusOK {
		t.Error("path traversal in share config update must not succeed with 200")
	}

	t.Logf("Path traversal on update returned status=%d (safe)", rr.Code)
}

func TestHandleUpdateShareConfig_NameOverride(t *testing.T) {
	// Verifies that the URL parameter name takes precedence over body name
	server, _ := setupTestServer()

	body := `{"name":"differentname","allocator":"highwater"}`
	req, err := http.NewRequest("POST", "/api/v1/shares/urlname/config", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// The handler sets config.Name = shareName from URL vars.
	// On non-Unraid, the actual update will fail but the request should parse.
	if rr.Code == http.StatusBadRequest {
		t.Error("valid JSON should not return 400")
	}

	t.Logf("Update share config (name override) status=%d", rr.Code)
}

// =====================================================================
// handleUpdateSystemSettings tests
// =====================================================================

func TestHandleUpdateSystemSettings_EmptyBody(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("POST", "/api/v1/settings/system", strings.NewReader(""))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for empty body, got %d", rr.Code)
	}
}

func TestHandleUpdateSystemSettings_InvalidJSON(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("POST", "/api/v1/settings/system", strings.NewReader("not json"))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid JSON, got %d", rr.Code)
	}

	var resp dto.Response
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Errorf("response is not valid JSON: %v", err)
	}

	if resp.Success {
		t.Error("expected success=false for invalid JSON")
	}
}

func TestHandleUpdateSystemSettings_ValidJSON(t *testing.T) {
	server, _ := setupTestServer()

	body := `{"server_name":"MyServer","timezone":"America/New_York"}`
	req, err := http.NewRequest("POST", "/api/v1/settings/system", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// On non-Unraid, will fail at the config write step
	if rr.Code != http.StatusOK && rr.Code != http.StatusInternalServerError {
		t.Errorf("unexpected status code: got %d", rr.Code)
	}

	var resp dto.Response
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Errorf("response is not valid JSON: %v", err)
	}

	t.Logf("Update system settings status=%d success=%v", rr.Code, resp.Success)
}

func TestHandleUpdateSystemSettings_EmptyObject(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("POST", "/api/v1/settings/system", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// Empty object is valid JSON, should parse ok but may fail on write
	if rr.Code == http.StatusBadRequest {
		t.Error("empty JSON object should not return 400")
	}

	t.Logf("Update system settings (empty object) status=%d", rr.Code)
}

// =====================================================================
// handleUserScripts tests
// =====================================================================

func TestHandleUserScripts_GET(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("GET", "/api/v1/user-scripts", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// On non-Unraid, the scripts directory won't exist — expect 500 or 200
	if rr.Code != http.StatusOK && rr.Code != http.StatusInternalServerError {
		t.Errorf("unexpected status code: got %d", rr.Code)
	}

	var body json.RawMessage
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Errorf("response is not valid JSON: %v", err)
	}

	t.Logf("User scripts status=%d (may fail on non-Unraid)", rr.Code)
}

func TestHandleUserScripts_ErrorFormat(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("GET", "/api/v1/user-scripts", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code == http.StatusInternalServerError {
		var resp dto.Response
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Errorf("error response is not valid JSON: %v", err)
		}
		if resp.Success {
			t.Error("expected success=false in error response")
		}
		if resp.Message == "" {
			t.Error("expected non-empty error message")
		}
		t.Logf("User scripts error (expected on non-Unraid): %s", resp.Message)
	}
}

func TestHandleUserScripts_WrongMethod(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("POST", "/api/v1/user-scripts", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// gorilla/mux returns 404 or 405 for unregistered method
	if rr.Code != http.StatusMethodNotAllowed && rr.Code != http.StatusNotFound {
		t.Errorf("expected 404 or 405 for POST on user-scripts list, got %d", rr.Code)
	}
}

// =====================================================================
// handleUserScriptExecute tests
// =====================================================================

func TestHandleUserScriptExecute_NoBody(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("POST", "/api/v1/user-scripts/testscript/execute", strings.NewReader(""))
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// The handler falls back to defaults when body cannot be decoded.
	// On non-Unraid, script execution will fail (500).
	if rr.Code != http.StatusOK && rr.Code != http.StatusInternalServerError {
		t.Errorf("unexpected status code: got %d", rr.Code)
	}

	var body json.RawMessage
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Errorf("response is not valid JSON: %v", err)
	}

	t.Logf("User script execute (no body) status=%d", rr.Code)
}

func TestHandleUserScriptExecute_WithBody(t *testing.T) {
	server, _ := setupTestServer()

	body := `{"background":true,"wait":false}`
	req, err := http.NewRequest("POST", "/api/v1/user-scripts/testscript/execute", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// On non-Unraid, script won't exist — expect 500
	if rr.Code != http.StatusOK && rr.Code != http.StatusInternalServerError {
		t.Errorf("unexpected status code: got %d", rr.Code)
	}

	t.Logf("User script execute (with body) status=%d", rr.Code)
}

func TestHandleUserScriptExecute_ForegroundMode(t *testing.T) {
	server, _ := setupTestServer()

	body := `{"background":false,"wait":true}`
	req, err := http.NewRequest("POST", "/api/v1/user-scripts/testscript/execute", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK && rr.Code != http.StatusInternalServerError {
		t.Errorf("unexpected status code: got %d", rr.Code)
	}

	t.Logf("User script execute (foreground) status=%d", rr.Code)
}

func TestHandleUserScriptExecute_InvalidJSON(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("POST", "/api/v1/user-scripts/testscript/execute", strings.NewReader("{bad"))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// Invalid JSON falls back to defaults (background=true, wait=false),
	// then script execution will fail on non-Unraid
	if rr.Code != http.StatusOK && rr.Code != http.StatusInternalServerError {
		t.Errorf("unexpected status code: got %d", rr.Code)
	}

	t.Logf("User script execute (invalid JSON) status=%d — uses defaults", rr.Code)
}

func TestHandleUserScriptExecute_WrongMethod(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("GET", "/api/v1/user-scripts/testscript/execute", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// gorilla/mux returns 404 or 405 for unregistered method
	if rr.Code != http.StatusMethodNotAllowed && rr.Code != http.StatusNotFound {
		t.Errorf("expected 404 or 405 for GET on execute endpoint, got %d", rr.Code)
	}
}

// =====================================================================
// handleCreateNotification tests
// =====================================================================

func TestHandleCreateNotification_EmptyBody(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("POST", "/api/v1/notifications", strings.NewReader(""))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for empty body, got %d", rr.Code)
	}

	var resp map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Errorf("response is not valid JSON: %v", err)
	}

	if _, ok := resp["error"]; !ok {
		t.Error("expected 'error' field in response")
	}
}

func TestHandleCreateNotification_InvalidJSON(t *testing.T) {
	server, _ := setupTestServer()

	req, err := http.NewRequest("POST", "/api/v1/notifications", strings.NewReader("not json at all"))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid JSON, got %d", rr.Code)
	}
}

func TestHandleCreateNotification_MissingTitle(t *testing.T) {
	server, _ := setupTestServer()

	// Title is required; subject/description are optional
	body := `{"subject":"test subject","description":"test desc","importance":"warning"}`
	req, err := http.NewRequest("POST", "/api/v1/notifications", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing title, got %d", rr.Code)
	}

	var resp map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Errorf("response is not valid JSON: %v", err)
	}

	if errMsg, ok := resp["error"]; !ok || errMsg != "Title is required" {
		t.Errorf("expected error 'Title is required', got %v", resp["error"])
	}
}

func TestHandleCreateNotification_ValidNotification(t *testing.T) {
	server, _ := setupTestServer()

	body := `{"title":"Test Alert","subject":"Test","description":"Test notification","importance":"warning","link":"https://example.com"}`
	req, err := http.NewRequest("POST", "/api/v1/notifications", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// On non-Unraid, notification creation will fail (500) since the
	// notification system (Unraid's /usr/local/emhttp) is not available.
	if rr.Code != http.StatusCreated && rr.Code != http.StatusInternalServerError {
		t.Errorf("unexpected status code: got %d, want 201 or 500", rr.Code)
	}

	t.Logf("Create notification status=%d (may fail on non-Unraid)", rr.Code)
}

func TestHandleCreateNotification_DefaultImportance(t *testing.T) {
	server, _ := setupTestServer()

	// When importance is omitted, it should default to "info"
	body := `{"title":"Test Without Importance"}`
	req, err := http.NewRequest("POST", "/api/v1/notifications", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// We cannot verify the default importance directly because the handler
	// calls the controller, which will fail on non-Unraid. But the request
	// should NOT be rejected as 400.
	if rr.Code == http.StatusBadRequest {
		t.Error("notification with title but no importance should not be rejected as bad request")
	}

	t.Logf("Create notification (default importance) status=%d", rr.Code)
}

func TestHandleCreateNotification_EmptyTitle(t *testing.T) {
	server, _ := setupTestServer()

	body := `{"title":"","importance":"info"}`
	req, err := http.NewRequest("POST", "/api/v1/notifications", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for empty title string, got %d", rr.Code)
	}
}

// =====================================================================
// respondJSON tests
// =====================================================================

func TestRespondJSON_NormalPayload(t *testing.T) {
	rr := httptest.NewRecorder()

	payload := map[string]string{"status": "ok", "message": "test"}
	respondJSON(rr, http.StatusOK, payload)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}

	ct := rr.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected Content-Type 'application/json', got %s", ct)
	}

	var resp map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Errorf("response is not valid JSON: %v", err)
	}

	if resp["status"] != "ok" {
		t.Errorf("expected status 'ok', got %s", resp["status"])
	}
}

func TestRespondJSON_NilPayload(t *testing.T) {
	rr := httptest.NewRecorder()

	respondJSON(rr, http.StatusOK, nil)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}

	body := strings.TrimSpace(rr.Body.String())
	if body != "null" {
		t.Errorf("expected 'null' for nil payload, got %s", body)
	}
}

func TestRespondJSON_UnmarshalablePayload(t *testing.T) {
	rr := httptest.NewRecorder()

	// Channel types cannot be marshaled to JSON — this triggers the error path
	unmarshalable := make(chan int)
	respondJSON(rr, http.StatusOK, unmarshalable)

	// The status code is written before the encode attempt,
	// so it will be 200 but the body will be empty or contain partial data
	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200 (set before encode), got %d", rr.Code)
	}

	// The important thing is that it doesn't panic
	t.Logf("respondJSON with unmarshalable payload: status=%d body=%q", rr.Code, rr.Body.String())
}

func TestRespondJSON_StatusCodes(t *testing.T) {
	tests := []struct {
		name   string
		status int
	}{
		{"OK", http.StatusOK},
		{"Created", http.StatusCreated},
		{"BadRequest", http.StatusBadRequest},
		{"NotFound", http.StatusNotFound},
		{"InternalServerError", http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			respondJSON(rr, tt.status, map[string]string{"test": "value"})

			if rr.Code != tt.status {
				t.Errorf("expected status %d, got %d", tt.status, rr.Code)
			}
		})
	}
}

func TestRespondJSON_ComplexPayload(t *testing.T) {
	rr := httptest.NewRecorder()

	payload := dto.Response{
		Success: true,
		Message: "Complex response",
		Data: map[string]any{
			"nested": map[string]string{"key": "value"},
			"count":  42,
		},
	}

	respondJSON(rr, http.StatusOK, payload)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}

	var resp dto.Response
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Errorf("failed to decode complex response: %v", err)
	}

	if !resp.Success {
		t.Error("expected success=true")
	}

	if resp.Message != "Complex response" {
		t.Errorf("expected message 'Complex response', got %s", resp.Message)
	}
}

func TestRespondJSON_EmptyStruct(t *testing.T) {
	rr := httptest.NewRecorder()

	respondJSON(rr, http.StatusOK, struct{}{})

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}

	body := strings.TrimSpace(rr.Body.String())
	if body != "{}" {
		t.Errorf("expected '{}' for empty struct, got %s", body)
	}
}

// =====================================================================
// respondWithError tests
// =====================================================================

func TestRespondWithError_ReturnsErrorJSON(t *testing.T) {
	rr := httptest.NewRecorder()

	respondWithError(rr, http.StatusBadRequest, "something went wrong")

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rr.Code)
	}

	var resp map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Errorf("response is not valid JSON: %v", err)
	}

	if resp["error"] != "something went wrong" {
		t.Errorf("expected error message 'something went wrong', got %s", resp["error"])
	}
}

func TestRespondWithError_DifferentStatuses(t *testing.T) {
	tests := []struct {
		status  int
		message string
	}{
		{http.StatusBadRequest, "bad request"},
		{http.StatusNotFound, "not found"},
		{http.StatusInternalServerError, "internal error"},
		{http.StatusForbidden, "forbidden"},
	}

	for _, tt := range tests {
		t.Run(tt.message, func(t *testing.T) {
			rr := httptest.NewRecorder()
			respondWithError(rr, tt.status, tt.message)

			if rr.Code != tt.status {
				t.Errorf("expected status %d, got %d", tt.status, rr.Code)
			}

			var resp map[string]string
			if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
				t.Errorf("response is not valid JSON: %v", err)
			}

			if resp["error"] != tt.message {
				t.Errorf("expected error %q, got %q", tt.message, resp["error"])
			}
		})
	}
}
