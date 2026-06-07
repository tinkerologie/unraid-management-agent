package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

// With no fan controller wired (unit-test server), the endpoint reports 503.
func TestFanSensorsNilController(t *testing.T) {
	server := NewServer(&domain.Context{Config: domain.Config{Port: 8043}})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/fans/sensors", nil)
	w := httptest.NewRecorder()
	server.handleFanSensors(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("nil controller: got %d want 503", w.Code)
	}
}

// An invalid temperature source must be rejected with 400 before the controller
// is consulted (so it fails fast even when the controller is unavailable).
func TestSetFanProfileSourceValidation(t *testing.T) {
	server := NewServer(&domain.Context{Config: domain.Config{Port: 8043}})
	body, _ := json.Marshal(dto.FanProfileRequest{
		FanID:       "hwmon0_fan1",
		ProfileName: "balanced",
		Source:      &dto.FanTempSource{Type: "bogus"},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/fans/profile", bytes.NewReader(body))
	w := httptest.NewRecorder()
	server.handleSetFanProfile(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("invalid source: got %d want 400", w.Code)
	}
}
