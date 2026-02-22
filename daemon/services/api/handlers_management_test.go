package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

// ===== Collector Management Handler Tests =====

func setupTestServerWithCollectorManager() (*Server, *mockCollectorManager) {
	ctx := &domain.Context{Config: domain.Config{Port: 8080}}
	mock := newMockCollectorManager()
	server := NewServerWithCollectorManager(ctx, mock)
	return server, mock
}

// --- handleCollectorStatus ---

func TestHandleCollectorStatus_Success(t *testing.T) {
	server, _ := setupTestServerWithCollectorManager()
	req := httptest.NewRequest("GET", "/api/v1/collectors/system", nil)
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp dto.CollectorResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if !resp.Success {
		t.Error("expected success=true")
	}
	if resp.Collector.Name != "system" {
		t.Errorf("collector name = %q, want system", resp.Collector.Name)
	}
}

func TestHandleCollectorStatus_NotFound(t *testing.T) {
	server, _ := setupTestServerWithCollectorManager()
	req := httptest.NewRequest("GET", "/api/v1/collectors/nonexistent", nil)
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestHandleCollectorStatus_NilManager(t *testing.T) {
	server, _ := setupTestServer()
	req := httptest.NewRequest("GET", "/api/v1/collectors/system", nil)
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", rr.Code, rr.Body.String())
	}
}

// --- handleCollectorEnable ---

func TestHandleCollectorEnable_Success(t *testing.T) {
	server, _ := setupTestServerWithCollectorManager()
	req := httptest.NewRequest("POST", "/api/v1/collectors/gpu/enable", nil)
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp dto.CollectorResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if !resp.Success {
		t.Error("expected success=true")
	}
}

func TestHandleCollectorEnable_NotFound(t *testing.T) {
	server, _ := setupTestServerWithCollectorManager()
	req := httptest.NewRequest("POST", "/api/v1/collectors/nonexistent/enable", nil)
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestHandleCollectorEnable_NilManager(t *testing.T) {
	server, _ := setupTestServer()
	req := httptest.NewRequest("POST", "/api/v1/collectors/system/enable", nil)
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", rr.Code, rr.Body.String())
	}
}

// --- handleCollectorDisable ---

func TestHandleCollectorDisable_Success(t *testing.T) {
	server, _ := setupTestServerWithCollectorManager()
	req := httptest.NewRequest("POST", "/api/v1/collectors/docker/disable", nil)
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestHandleCollectorDisable_Required(t *testing.T) {
	server, _ := setupTestServerWithCollectorManager()
	req := httptest.NewRequest("POST", "/api/v1/collectors/system/disable", nil)
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestHandleCollectorDisable_NotFound(t *testing.T) {
	server, _ := setupTestServerWithCollectorManager()
	req := httptest.NewRequest("POST", "/api/v1/collectors/nonexistent/disable", nil)
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

// --- handleCollectorInterval ---

func TestHandleCollectorInterval_Success(t *testing.T) {
	server, _ := setupTestServerWithCollectorManager()
	body, _ := json.Marshal(dto.CollectorIntervalRequest{Interval: 60})
	req := httptest.NewRequest("PATCH", "/api/v1/collectors/system/interval", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestHandleCollectorInterval_InvalidBody(t *testing.T) {
	server, _ := setupTestServerWithCollectorManager()
	req := httptest.NewRequest("PATCH", "/api/v1/collectors/system/interval", bytes.NewReader([]byte("invalid")))
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestHandleCollectorInterval_TooShort(t *testing.T) {
	server, _ := setupTestServerWithCollectorManager()
	body, _ := json.Marshal(dto.CollectorIntervalRequest{Interval: 1})
	req := httptest.NewRequest("PATCH", "/api/v1/collectors/system/interval", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestHandleCollectorInterval_NotFound(t *testing.T) {
	server, _ := setupTestServerWithCollectorManager()
	body, _ := json.Marshal(dto.CollectorIntervalRequest{Interval: 30})
	req := httptest.NewRequest("PATCH", "/api/v1/collectors/nonexistent/interval", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestHandleCollectorInterval_NilManager(t *testing.T) {
	server, _ := setupTestServer()
	body, _ := json.Marshal(dto.CollectorIntervalRequest{Interval: 30})
	req := httptest.NewRequest("PATCH", "/api/v1/collectors/system/interval", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", rr.Code, rr.Body.String())
	}
}

// ===== MQTT Handler Tests =====

// --- handleMQTTStatus ---

func TestHandleMQTTStatus_NilClient(t *testing.T) {
	server, _ := setupTestServer()
	req := httptest.NewRequest("GET", "/api/v1/mqtt/status", nil)
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var status dto.MQTTStatus
	if err := json.Unmarshal(rr.Body.Bytes(), &status); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if status.Connected {
		t.Error("expected connected=false for nil client")
	}
}

func TestHandleMQTTStatus_WithClient(t *testing.T) {
	server, _ := setupTestServer()
	server.SetMQTTClient(&mockMQTTClient{connected: true})
	req := httptest.NewRequest("GET", "/api/v1/mqtt/status", nil)
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var status dto.MQTTStatus
	if err := json.Unmarshal(rr.Body.Bytes(), &status); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !status.Connected {
		t.Error("expected connected=true")
	}
}

// --- handleMQTTTest ---

func TestHandleMQTTTest_NilClient(t *testing.T) {
	server, _ := setupTestServer()
	req := httptest.NewRequest("POST", "/api/v1/mqtt/test", nil)
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestHandleMQTTTest_Success(t *testing.T) {
	server, _ := setupTestServer()
	server.SetMQTTClient(&mockMQTTClient{connected: true})
	req := httptest.NewRequest("POST", "/api/v1/mqtt/test", nil)
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestHandleMQTTTest_Failure(t *testing.T) {
	server, _ := setupTestServer()
	server.SetMQTTClient(&mockMQTTClient{testErr: fmt.Errorf("connection refused")})
	req := httptest.NewRequest("POST", "/api/v1/mqtt/test", nil)
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rr.Code, rr.Body.String())
	}
}

// --- handleMQTTPublish ---

func TestHandleMQTTPublish_NilClient(t *testing.T) {
	server, _ := setupTestServer()
	body, _ := json.Marshal(dto.MQTTPublishRequest{Topic: "test/topic", Payload: "hello"})
	req := httptest.NewRequest("POST", "/api/v1/mqtt/publish", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestHandleMQTTPublish_Success(t *testing.T) {
	server, _ := setupTestServer()
	server.SetMQTTClient(&mockMQTTClient{connected: true})
	body, _ := json.Marshal(dto.MQTTPublishRequest{Topic: "test/topic", Payload: "hello"})
	req := httptest.NewRequest("POST", "/api/v1/mqtt/publish", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestHandleMQTTPublish_EmptyTopic(t *testing.T) {
	server, _ := setupTestServer()
	server.SetMQTTClient(&mockMQTTClient{connected: true})
	body, _ := json.Marshal(dto.MQTTPublishRequest{Topic: "", Payload: "hello"})
	req := httptest.NewRequest("POST", "/api/v1/mqtt/publish", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty topic, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestHandleMQTTPublish_InvalidBody(t *testing.T) {
	server, _ := setupTestServer()
	server.SetMQTTClient(&mockMQTTClient{connected: true})
	req := httptest.NewRequest("POST", "/api/v1/mqtt/publish", bytes.NewReader([]byte("not json")))
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestHandleMQTTPublish_PublishError(t *testing.T) {
	server, _ := setupTestServer()
	server.SetMQTTClient(&mockMQTTClient{publishErr: fmt.Errorf("broker disconnected")})
	body, _ := json.Marshal(dto.MQTTPublishRequest{Topic: "test/topic", Payload: "hello"})
	req := httptest.NewRequest("POST", "/api/v1/mqtt/publish", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rr.Code, rr.Body.String())
	}
}

// ===== handleCollectorsStatus (plural) =====

func TestHandleCollectorsStatus_WithManager(t *testing.T) {
	server, _ := setupTestServerWithCollectorManager()
	req := httptest.NewRequest("GET", "/api/v1/collectors/status", nil)
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
}
