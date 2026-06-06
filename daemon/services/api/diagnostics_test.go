package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/platform"
)

func TestSelfTestEndpoint(t *testing.T) {
	reg := platform.NewRegistry()
	reg.Healthy("system")
	reg.Report("array", dto.SourceDegraded, "stale", nil)
	s := &Server{ctx: &domain.Context{Platform: reg}}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/diagnostics/self-test", nil)
	rec := httptest.NewRecorder()
	s.handleSelfTest(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var out selfTestResponse
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.OverallState != dto.SourceDegraded {
		t.Errorf("overall_state = %q, want degraded", out.OverallState)
	}
	if len(out.Subsystems) != 2 {
		t.Errorf("subsystems = %d, want 2", len(out.Subsystems))
	}
}

func TestSelfTestEndpointNilRegistry(t *testing.T) {
	s := &Server{ctx: &domain.Context{}}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/diagnostics/self-test", nil)
	rec := httptest.NewRecorder()
	s.handleSelfTest(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}
