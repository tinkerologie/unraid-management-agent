package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/services/alerting"
)

// stubDataProvider implements alerting.DataProvider with all nil/empty returns.
// It is only used to satisfy the engine constructor; no evaluation loop runs in tests.
type stubDataProvider struct{}

func (s *stubDataProvider) GetSystemCache() *dto.SystemInfo              { return nil }
func (s *stubDataProvider) GetArrayCache() *dto.ArrayStatus              { return nil }
func (s *stubDataProvider) GetDisksCache() []dto.DiskInfo                { return nil }
func (s *stubDataProvider) GetDockerCache() []dto.ContainerInfo          { return nil }
func (s *stubDataProvider) GetVMsCache() []dto.VMInfo                    { return nil }
func (s *stubDataProvider) GetUPSCache() *dto.UPSStatus                  { return nil }
func (s *stubDataProvider) GetGPUCache() []*dto.GPUMetrics               { return nil }
func (s *stubDataProvider) GetZFSPoolsCache() []dto.ZFSPool              { return nil }
func (s *stubDataProvider) GetNetworkCache() []dto.NetworkInfo           { return nil }
func (s *stubDataProvider) GetNUTCache() *dto.NUTResponse                { return nil }
func (s *stubDataProvider) GetNotificationsCache() *dto.NotificationList { return nil }
func (s *stubDataProvider) GetPluginUpdatesCache() *dto.PluginList       { return nil }
func (s *stubDataProvider) DegradedSubsystemCount() int                  { return 0 }

// setupAlertTemplateServer creates an API server with a real in-memory alertStore
// and a non-running alertEngine (no evaluation loop, safe for unit tests).
func setupAlertTemplateServer(t *testing.T) *Server {
	t.Helper()
	ctx := &domain.Context{Config: domain.Config{Port: 8080}}
	server := NewServer(ctx)

	store := alerting.NewStore(t.TempDir())
	engine := alerting.NewEngine(store, &stubDataProvider{})
	server.SetAlertEngine(engine, store)
	return server
}

func TestHandleEnableAlertTemplate_KnownTemplate(t *testing.T) {
	server := setupAlertTemplateServer(t)

	req := httptest.NewRequest("POST", "/api/v1/alerts/templates/tmpl-array-fill/enable", nil)
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var rule dto.AlertRule
	if err := json.Unmarshal(rr.Body.Bytes(), &rule); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if !rule.Enabled {
		t.Error("expected Enabled=true in returned rule")
	}
	if rule.ID != "tmpl-array-fill" {
		t.Errorf("expected ID=tmpl-array-fill, got %q", rule.ID)
	}
	if len(rule.Channels) == 0 || rule.Channels[0] != "unraid" {
		t.Errorf("expected default channel 'unraid', got %v", rule.Channels)
	}
}

func TestHandleEnableAlertTemplate_UnknownTemplate(t *testing.T) {
	server := setupAlertTemplateServer(t)

	req := httptest.NewRequest("POST", "/api/v1/alerts/templates/tmpl-does-not-exist/enable", nil)
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp dto.Response
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse error response: %v", err)
	}
	if resp.Success {
		t.Error("expected Success=false for unknown template")
	}
}

func TestHandleEnableAlertTemplate_CustomChannels(t *testing.T) {
	server := setupAlertTemplateServer(t)

	body := `{"channels": ["slack://token"]}`
	req := httptest.NewRequest("POST", "/api/v1/alerts/templates/tmpl-disk-temp-climb/enable", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.ContentLength = int64(len(body))
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var rule dto.AlertRule
	if err := json.Unmarshal(rr.Body.Bytes(), &rule); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if len(rule.Channels) != 1 || rule.Channels[0] != "slack://token" {
		t.Errorf("expected channels=[slack://token], got %v", rule.Channels)
	}
}

func TestHandleEnableAlertTemplate_Idempotent(t *testing.T) {
	server := setupAlertTemplateServer(t)

	// Enable twice — second call should succeed (update path).
	for i := range 2 {
		req := httptest.NewRequest("POST", "/api/v1/alerts/templates/tmpl-smart-reallocated/enable", nil)
		rr := httptest.NewRecorder()
		server.router.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("call %d: expected 200, got %d: %s", i+1, rr.Code, rr.Body.String())
		}
	}
}

func TestHandleEnableAlertTemplate_InvalidBody(t *testing.T) {
	server := setupAlertTemplateServer(t)

	body := `{not valid json}`
	req := httptest.NewRequest("POST", "/api/v1/alerts/templates/tmpl-array-fill/enable", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.ContentLength = int64(len(body))
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid JSON, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestHandleEnableAlertTemplate_NoAlertEngine(t *testing.T) {
	server, _ := setupTestServer() // alertStore and alertEngine are nil

	req := httptest.NewRequest("POST", "/api/v1/alerts/templates/tmpl-array-fill/enable", nil)
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", rr.Code, rr.Body.String())
	}
}
