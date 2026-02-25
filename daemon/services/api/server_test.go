package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
)

func TestNewServer(t *testing.T) {
	hub := domain.NewEventBus(10)
	ctx := &domain.Context{Hub: hub}

	server := NewServer(ctx)

	if server == nil {
		t.Fatal("NewServer() returned nil")
	}

	if server.ctx != ctx {
		t.Error("Server context not set correctly")
	}

	if server.router == nil {
		t.Error("Server router not initialized")
	}

	if server.wsHub == nil {
		t.Error("Server WebSocket hub not initialized")
	}
}

func TestServerRouterSetup(t *testing.T) {
	hub := domain.NewEventBus(10)
	ctx := &domain.Context{Hub: hub}

	server := NewServer(ctx)

	// Verify router is properly configured
	if server.router == nil {
		t.Fatal("Router should not be nil")
	}
}

func TestServerCacheInitialization(t *testing.T) {
	hub := domain.NewEventBus(10)
	ctx := &domain.Context{Hub: hub}

	server := NewServer(ctx)

	// All caches should be nil initially (before collectors start)
	if server.systemCache.Load() != nil {
		t.Error("System cache should be nil initially")
	}
	if server.arrayCache.Load() != nil {
		t.Error("Array cache should be nil initially")
	}
	if server.dockerCache.Load() != nil {
		t.Error("Docker cache should be nil initially")
	}
}

func TestServerHealthEndpoint(t *testing.T) {
	hub := domain.NewEventBus(10)
	ctx := &domain.Context{Hub: hub}

	server := NewServer(ctx)

	req, err := http.NewRequest("GET", "/api/v1/health", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// Health endpoint should return 200 OK
	if rr.Code != http.StatusOK {
		t.Errorf("Health endpoint returned %d, want %d", rr.Code, http.StatusOK)
	}
}

func TestAPIRoutes(t *testing.T) {
	hub := domain.NewEventBus(10)
	ctx := &domain.Context{Hub: hub}

	server := NewServer(ctx)

	// Test that expected routes exist by checking for 200 or appropriate status
	routes := []struct {
		method string
		path   string
	}{
		{"GET", "/api/v1/health"},
		{"GET", "/api/v1/system"},
		{"GET", "/api/v1/array"},
		{"GET", "/api/v1/disks"},
		{"GET", "/api/v1/shares"},
		{"GET", "/api/v1/docker"},
		{"GET", "/api/v1/vm"},
		{"GET", "/api/v1/ups"},
		{"GET", "/api/v1/nut"},
		{"GET", "/api/v1/gpu"},
		{"GET", "/api/v1/network"},
		{"GET", "/api/v1/registration"},
		{"GET", "/api/v1/notifications"},
		{"GET", "/api/v1/collectors/status"},
	}

	for _, route := range routes {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			req, err := http.NewRequest(route.method, route.path, nil)
			if err != nil {
				t.Fatal(err)
			}

			rr := httptest.NewRecorder()
			server.router.ServeHTTP(rr, req)

			// Route should exist (not return 404)
			if rr.Code == http.StatusNotFound {
				t.Errorf("Route %s %s not found", route.method, route.path)
			}
		})
	}
}

func TestZFSRoutes(t *testing.T) {
	hub := domain.NewEventBus(10)
	ctx := &domain.Context{Hub: hub}

	server := NewServer(ctx)

	routes := []string{
		"/api/v1/zfs/pools",
		"/api/v1/zfs/datasets",
		"/api/v1/zfs/snapshots",
		"/api/v1/zfs/arc",
	}

	for _, path := range routes {
		t.Run("GET "+path, func(t *testing.T) {
			req, err := http.NewRequest("GET", path, nil)
			if err != nil {
				t.Fatal(err)
			}

			rr := httptest.NewRecorder()
			server.router.ServeHTTP(rr, req)

			// Route should exist (not return 404)
			if rr.Code == http.StatusNotFound {
				t.Errorf("Route GET %s not found", path)
			}
		})
	}
}

func TestHardwareRoutes(t *testing.T) {
	hub := domain.NewEventBus(10)
	ctx := &domain.Context{Hub: hub}

	server := NewServer(ctx)

	// Only test the full hardware endpoint - individual endpoints return 404 by design
	// when cache is nil (no hardware data collected yet)
	req, err := http.NewRequest("GET", "/api/v1/hardware/full", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// Route should exist (not return 404)
	if rr.Code == http.StatusNotFound {
		t.Error("Route GET /api/v1/hardware/full not found")
	}
}

func TestControlRoutes(t *testing.T) {
	hub := domain.NewEventBus(10)
	ctx := &domain.Context{Hub: hub}

	server := NewServer(ctx)

	// Control routes should exist but may return errors without valid data
	routes := []struct {
		method string
		path   string
	}{
		{"POST", "/api/v1/system/reboot"},
		{"POST", "/api/v1/system/shutdown"},
		{"POST", "/api/v1/array/start"},
		{"POST", "/api/v1/array/stop"},
		{"POST", "/api/v1/array/parity-check/start"},
		{"POST", "/api/v1/array/parity-check/stop"},
		{"POST", "/api/v1/array/parity-check/pause"},
		{"POST", "/api/v1/array/parity-check/resume"},
	}

	for _, route := range routes {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			req, err := http.NewRequest(route.method, route.path, nil)
			if err != nil {
				t.Fatal(err)
			}

			rr := httptest.NewRecorder()
			server.router.ServeHTTP(rr, req)

			// Route should exist (not return 404)
			if rr.Code == http.StatusNotFound {
				t.Errorf("Route %s %s not found", route.method, route.path)
			}
		})
	}
}

func TestNotificationRoutes(t *testing.T) {
	hub := domain.NewEventBus(10)
	ctx := &domain.Context{Hub: hub}

	server := NewServer(ctx)

	routes := []string{
		"/api/v1/notifications",
		"/api/v1/notifications/unread",
		"/api/v1/notifications/archive",
		"/api/v1/notifications/overview",
	}

	for _, path := range routes {
		t.Run("GET "+path, func(t *testing.T) {
			req, err := http.NewRequest("GET", path, nil)
			if err != nil {
				t.Fatal(err)
			}

			rr := httptest.NewRecorder()
			server.router.ServeHTTP(rr, req)

			// Route should exist (not return 404)
			if rr.Code == http.StatusNotFound {
				t.Errorf("Route GET %s not found", path)
			}
		})
	}
}

func TestServerMiddlewareChain(t *testing.T) {
	hub := domain.NewEventBus(10)
	ctx := &domain.Context{Hub: hub}

	server := NewServer(ctx)

	req, err := http.NewRequest("GET", "/api/v1/health", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// CORS middleware should add headers
	if rr.Header().Get("Access-Control-Allow-Origin") == "" {
		t.Error("CORS middleware should set Access-Control-Allow-Origin header")
	}
}

func TestCORSHeaders(t *testing.T) {
	hub := domain.NewEventBus(10)
	ctx := &domain.Context{Hub: hub}

	server := NewServer(ctx)

	req, err := http.NewRequest("GET", "/api/v1/health", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Origin", "http://localhost:3000")

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// CORS middleware should add headers on regular requests
	origin := rr.Header().Get("Access-Control-Allow-Origin")
	if origin != "*" {
		t.Errorf("Expected Access-Control-Allow-Origin header to be '*', got %q", origin)
	}
}

func TestWebSocketRouteExists(t *testing.T) {
	hub := domain.NewEventBus(10)
	ctx := &domain.Context{Hub: hub}

	server := NewServer(ctx)

	req, err := http.NewRequest("GET", "/api/v1/ws", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	// WebSocket route should exist (may return error without proper upgrade)
	if rr.Code == http.StatusNotFound {
		t.Error("WebSocket route not found")
	}
}
