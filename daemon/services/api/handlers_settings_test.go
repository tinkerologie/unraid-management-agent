package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

// Tests for settings/config handler endpoints that are at 0% coverage.
// These handlers create collectors internally, so on non-Unraid they return
// either error responses (500) or default values.

func TestHandleNetworkAccessURLs(t *testing.T) {
	server, _ := setupTestServer()

	req := httptest.NewRequest("GET", "/api/v1/network/access-urls", nil)
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// CollectNetworkAccessURLs always returns a result (uses net.Interfaces)
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rr.Code)
	}

	var result dto.NetworkAccessURLs
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Errorf("Failed to parse response: %v", err)
	}
}

func TestHandleDiskSettingsExtended(t *testing.T) {
	server, _ := setupTestServer()

	req := httptest.NewRequest("GET", "/api/v1/settings/disk-thresholds", nil)
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// On non-Unraid, returns either 200 with defaults or 500 with error
	if rr.Code != http.StatusOK && rr.Code != http.StatusInternalServerError {
		t.Errorf("Expected status 200 or 500, got %d", rr.Code)
	}

	if rr.Code == http.StatusInternalServerError {
		var resp dto.Response
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Errorf("Failed to parse error response: %v", err)
		}
		if resp.Success {
			t.Error("Expected Success to be false on error")
		}
	}
}

func TestHandleMoverSettings(t *testing.T) {
	server, _ := setupTestServer()

	req := httptest.NewRequest("GET", "/api/v1/settings/mover", nil)
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK && rr.Code != http.StatusInternalServerError {
		t.Errorf("Expected status 200 or 500, got %d", rr.Code)
	}

	if rr.Code == http.StatusInternalServerError {
		var resp dto.Response
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Errorf("Failed to parse error response: %v", err)
		}
		if resp.Success {
			t.Error("Expected Success to be false on error")
		}
	}
}

func TestHandleParitySchedule(t *testing.T) {
	server, _ := setupTestServer()

	req := httptest.NewRequest("GET", "/api/v1/array/parity-check/schedule", nil)
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK && rr.Code != http.StatusInternalServerError {
		t.Errorf("Expected status 200 or 500, got %d", rr.Code)
	}

	if rr.Code == http.StatusInternalServerError {
		var resp dto.Response
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Errorf("Failed to parse error response: %v", err)
		}
		if resp.Success {
			t.Error("Expected Success to be false on error")
		}
	}
}

func TestHandleServiceStatus(t *testing.T) {
	server, _ := setupTestServer()

	req := httptest.NewRequest("GET", "/api/v1/settings/services", nil)
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK && rr.Code != http.StatusInternalServerError {
		t.Errorf("Expected status 200 or 500, got %d", rr.Code)
	}

	if rr.Code == http.StatusInternalServerError {
		var resp dto.Response
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Errorf("Failed to parse error response: %v", err)
		}
		if resp.Success {
			t.Error("Expected Success to be false on error")
		}
	}
}

func TestHandlePluginList(t *testing.T) {
	server, _ := setupTestServer()

	req := httptest.NewRequest("GET", "/api/v1/plugins", nil)
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK && rr.Code != http.StatusInternalServerError {
		t.Errorf("Expected status 200 or 500, got %d", rr.Code)
	}

	if rr.Code == http.StatusInternalServerError {
		var resp dto.Response
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Errorf("Failed to parse error response: %v", err)
		}
		if resp.Success {
			t.Error("Expected Success to be false on error")
		}
	}
}

func TestHandleUpdateStatus(t *testing.T) {
	server, _ := setupTestServer()

	req := httptest.NewRequest("GET", "/api/v1/updates", nil)
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK && rr.Code != http.StatusInternalServerError {
		t.Errorf("Expected status 200 or 500, got %d", rr.Code)
	}

	if rr.Code == http.StatusInternalServerError {
		var resp dto.Response
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Errorf("Failed to parse error response: %v", err)
		}
		if resp.Success {
			t.Error("Expected Success to be false on error")
		}
	}
}

func TestHandleFlashHealth(t *testing.T) {
	server, _ := setupTestServer()

	req := httptest.NewRequest("GET", "/api/v1/system/flash", nil)
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK && rr.Code != http.StatusInternalServerError {
		t.Errorf("Expected status 200 or 500, got %d", rr.Code)
	}

	if rr.Code == http.StatusInternalServerError {
		var resp dto.Response
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Errorf("Failed to parse error response: %v", err)
		}
		if resp.Success {
			t.Error("Expected Success to be false on error")
		}
	}
}

func TestHandleNetworkServices(t *testing.T) {
	server, _ := setupTestServer()

	req := httptest.NewRequest("GET", "/api/v1/settings/network-services", nil)
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK && rr.Code != http.StatusInternalServerError {
		t.Errorf("Expected status 200 or 500, got %d", rr.Code)
	}

	if rr.Code == http.StatusInternalServerError {
		var resp dto.Response
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Errorf("Failed to parse error response: %v", err)
		}
		if resp.Success {
			t.Error("Expected Success to be false on error")
		}
	}
}

func TestHandleDockerCheckUpdates(t *testing.T) {
	server, _ := setupTestServer()

	req := httptest.NewRequest("GET", "/api/v1/docker/updates", nil)
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// Docker not available on test machine - expect 500
	if rr.Code != http.StatusOK && rr.Code != http.StatusInternalServerError {
		t.Errorf("Expected status 200 or 500, got %d", rr.Code)
	}

	if rr.Code == http.StatusInternalServerError {
		var resp dto.Response
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Errorf("Failed to parse error response: %v", err)
		}
		if resp.Success {
			t.Error("Expected Success to be false on error")
		}
	}
}

func TestHandleDockerUpdateAll(t *testing.T) {
	server, _ := setupTestServer()

	req := httptest.NewRequest("POST", "/api/v1/docker/update-all", nil)
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// Docker not available on test machine - expect 500
	if rr.Code != http.StatusOK && rr.Code != http.StatusInternalServerError {
		t.Errorf("Expected status 200 or 500, got %d", rr.Code)
	}

	if rr.Code == http.StatusInternalServerError {
		var resp dto.Response
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Errorf("Failed to parse error response: %v", err)
		}
		if resp.Success {
			t.Error("Expected Success to be false on error")
		}
	}
}

// Test server settings methods that delegate to config collectors
func TestServerGetSystemSettings(t *testing.T) {
	server, _ := setupTestServer()

	// On non-Unraid, returns nil (config files not found)
	result := server.GetSystemSettings()
	// Just verify it doesn't panic - result may be nil on non-Unraid
	_ = result
}

func TestServerGetDockerSettings(t *testing.T) {
	server, _ := setupTestServer()
	result := server.GetDockerSettings()
	_ = result
}

func TestServerGetVMSettings(t *testing.T) {
	server, _ := setupTestServer()
	result := server.GetVMSettings()
	_ = result
}

func TestServerGetDiskSettings(t *testing.T) {
	server, _ := setupTestServer()
	result := server.GetDiskSettings()
	_ = result
}

func TestServerGetShareConfig(t *testing.T) {
	server, _ := setupTestServer()
	result := server.GetShareConfig("test-share")
	_ = result
}

func TestServerGetNetworkAccessURLs(t *testing.T) {
	server, _ := setupTestServer()
	result := server.GetNetworkAccessURLs()
	// CollectNetworkAccessURLs always returns a non-nil result
	if result == nil {
		t.Error("Expected non-nil NetworkAccessURLs")
	}
}
