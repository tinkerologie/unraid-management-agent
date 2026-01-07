package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cskr/pubsub"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

// TestHandlersWithEmptyCache tests all handlers when cache is empty
func TestHandlersWithEmptyCache(t *testing.T) {
	hub := pubsub.New(10)
	ctx := &domain.Context{Hub: hub}
	server := NewServer(ctx)

	tests := []struct {
		name   string
		method string
		path   string
		status int
	}{
		{"System empty cache", "GET", "/api/v1/system", http.StatusOK},
		{"Array empty cache", "GET", "/api/v1/array", http.StatusOK},
		{"Disks empty cache", "GET", "/api/v1/disks", http.StatusOK},
		{"Shares empty cache", "GET", "/api/v1/shares", http.StatusOK},
		{"Docker empty cache", "GET", "/api/v1/docker", http.StatusOK},
		{"VM empty cache", "GET", "/api/v1/vm", http.StatusOK},
		{"UPS empty cache", "GET", "/api/v1/ups", http.StatusOK},
		{"NUT empty cache", "GET", "/api/v1/nut", http.StatusOK},
		{"GPU empty cache", "GET", "/api/v1/gpu", http.StatusOK},
		{"Network empty cache", "GET", "/api/v1/network", http.StatusOK},
		{"ZFS pools empty cache", "GET", "/api/v1/zfs/pools", http.StatusOK},
		{"ZFS datasets empty cache", "GET", "/api/v1/zfs/datasets", http.StatusOK},
		{"ZFS snapshots empty cache", "GET", "/api/v1/zfs/snapshots", http.StatusOK},
		{"ZFS ARC empty cache", "GET", "/api/v1/zfs/arc", http.StatusOK},
		{"Hardware empty cache", "GET", "/api/v1/hardware/full", http.StatusOK},
		{"Registration empty cache", "GET", "/api/v1/registration", http.StatusOK},
		{"Notifications empty cache", "GET", "/api/v1/notifications", http.StatusOK},
		{"Collectors status", "GET", "/api/v1/collectors/status", http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest(tt.method, tt.path, nil)
			if err != nil {
				t.Fatal(err)
			}

			rr := httptest.NewRecorder()
			server.router.ServeHTTP(rr, req)

			if rr.Code != tt.status {
				t.Errorf("handler returned wrong status code: got %v want %v", rr.Code, tt.status)
			}

			// Verify response is valid JSON
			var response interface{}
			if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
				t.Errorf("failed to parse response as JSON: %v", err)
			}
		})
	}
}

// TestHandlersWithCachedData tests handlers when cache has data
func TestHandlersWithCachedData(t *testing.T) {
	hub := pubsub.New(10)
	ctx := &domain.Context{Hub: hub}
	server := NewServer(ctx)

	// Set cache data
	systemInfo := &dto.SystemInfo{
		Hostname: "test-system",
		Uptime:   86400,
	}
	server.cacheMutex.Lock()
	server.systemCache = systemInfo
	server.cacheMutex.Unlock()

	req, err := http.NewRequest("GET", "/api/v1/system", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", rr.Code, http.StatusOK)
	}

	var response dto.SystemInfo
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Errorf("failed to parse response: %v", err)
	}

	if response.Hostname != "test-system" {
		t.Errorf("expected hostname 'test-system', got %s", response.Hostname)
	}
}

// TestParityCheckHistoryEdgeCases tests parity check history with edge cases
func TestParityCheckHistoryEdgeCases(t *testing.T) {
	hub := pubsub.New(10)
	ctx := &domain.Context{Hub: hub}
	server := NewServer(ctx)

	req, err := http.NewRequest("GET", "/api/v1/array/parity-check/history", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// Should return OK even if file doesn't exist
	if rr.Code != http.StatusOK && rr.Code != http.StatusInternalServerError {
		t.Errorf("handler returned unexpected status code: %v", rr.Code)
	}
}

// TestInvalidDiskIDHandling tests disk endpoint with invalid ID
func TestInvalidDiskIDHandling(t *testing.T) {
	hub := pubsub.New(10)
	ctx := &domain.Context{Hub: hub}
	server := NewServer(ctx)

	req, err := http.NewRequest("GET", "/api/v1/disks/invalid-id", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// Should handle gracefully (200 or 404)
	if rr.Code != http.StatusOK && rr.Code != http.StatusNotFound && rr.Code != http.StatusInternalServerError {
		t.Errorf("handler returned unexpected status code: %v", rr.Code)
	}
}

// TestCacheLocking tests concurrent cache access
func TestCacheLocking(t *testing.T) {
	hub := pubsub.New(10)
	ctx := &domain.Context{Hub: hub}
	server := NewServer(ctx)

	// Test multiple concurrent reads
	done := make(chan bool, 5)

	for i := 0; i < 5; i++ {
		go func() {
			req, _ := http.NewRequest("GET", "/api/v1/system", nil)
			rr := httptest.NewRecorder()
			server.router.ServeHTTP(rr, req)
			done <- rr.Code == http.StatusOK
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 5; i++ {
		if !<-done {
			t.Error("concurrent read failed")
		}
	}
}

// TestResponseFormat tests that responses follow expected format
func TestResponseFormat(t *testing.T) {
	hub := pubsub.New(10)
	ctx := &domain.Context{Hub: hub}
	server := NewServer(ctx)

	req, err := http.NewRequest("GET", "/api/v1/health", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// Verify Content-Type header
	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type 'application/json', got %s", ct)
	}

	// Verify response is valid JSON
	var response map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Errorf("response is not valid JSON: %v", err)
	}
}

// TestAPIErrorResponses tests error response format
func TestAPIErrorResponses(t *testing.T) {
	hub := pubsub.New(10)
	ctx := &domain.Context{Hub: hub}
	server := NewServer(ctx)

	// Test non-existent endpoint
	req, err := http.NewRequest("GET", "/api/v1/nonexistent", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

// TestZFSPoolsEndpointEdgeCases tests ZFS pools with edge cases
func TestZFSPoolsEndpointEdgeCases(t *testing.T) {
	hub := pubsub.New(10)
	ctx := &domain.Context{Hub: hub}
	server := NewServer(ctx)

	req, err := http.NewRequest("GET", "/api/v1/zfs/pools", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("handler returned wrong status code: %v", rr.Code)
	}
}

// TestUnassignedDevicesEndpoint tests unassigned devices endpoint
func TestUnassignedDevicesEndpoint(t *testing.T) {
	hub := pubsub.New(10)
	ctx := &domain.Context{Hub: hub}
	server := NewServer(ctx)

	req, err := http.NewRequest("GET", "/api/v1/unassigned", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("handler returned wrong status code: %v", rr.Code)
	}
}

// TestNotificationEndpoints tests notification-related endpoints
func TestNotificationEndpoints(t *testing.T) {
	hub := pubsub.New(10)
	ctx := &domain.Context{Hub: hub}
	server := NewServer(ctx)

	tests := []struct {
		name string
		path string
	}{
		{"Unread notifications", "/api/v1/notifications/unread"},
		{"Archived notifications", "/api/v1/notifications/archive"},
		{"Notification overview", "/api/v1/notifications/overview"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest("GET", tt.path, nil)
			if err != nil {
				t.Fatal(err)
			}

			rr := httptest.NewRecorder()
			server.router.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Errorf("handler returned wrong status code: %v", rr.Code)
			}
		})
	}
}

// TestLogEndpoints tests log-related endpoints
func TestLogEndpoints(t *testing.T) {
	hub := pubsub.New(10)
	ctx := &domain.Context{Hub: hub}
	server := NewServer(ctx)

	req, err := http.NewRequest("GET", "/api/v1/logs", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("handler returned wrong status code: %v", rr.Code)
	}
}

// TestUserScriptsEndpointEdgeCases tests user scripts endpoint with edge cases
func TestUserScriptsEndpointEdgeCases(t *testing.T) {
	hub := pubsub.New(10)
	ctx := &domain.Context{Hub: hub}
	server := NewServer(ctx)

	req, err := http.NewRequest("GET", "/api/v1/user-scripts", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK && rr.Code != http.StatusInternalServerError {
		t.Errorf("handler returned unexpected status code: %v", rr.Code)
	}
}
