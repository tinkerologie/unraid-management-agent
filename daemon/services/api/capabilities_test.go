package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tinkerologie/unraid-management-agent/daemon/domain"
	"github.com/tinkerologie/unraid-management-agent/daemon/dto"
	"github.com/tinkerologie/unraid-management-agent/daemon/platform"
)

type fakeCapabilityRefresher struct {
	calls int
	caps  dto.Capabilities
}

func (f *fakeCapabilityRefresher) Refresh(context.Context) dto.Capabilities {
	f.calls++
	return f.caps
}

// capabilitiesRequest calls the capabilities handler and decodes a 200 response.
func capabilitiesRequest(t *testing.T, s *Server, query string) (*httptest.ResponseRecorder, dto.Capabilities) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/capabilities"+query, nil)
	rec := httptest.NewRecorder()
	s.handleCapabilities(rec, req)
	var out dto.Capabilities
	if rec.Code == http.StatusOK {
		if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
			t.Fatalf("decode: %v", err)
		}
	}
	return rec, out
}

// TestCapabilitiesEndpoint covers the snapshot, refresh=true/false and an invalid refresh value.
func TestCapabilitiesEndpoint(t *testing.T) {
	checked := time.Date(2026, 10, 14, 9, 0, 0, 0, time.UTC)
	reg := platform.NewRegistry()
	reg.SetCapabilities(dto.Capabilities{
		UnraidVersion: "7.3.3",
		CheckedAt:     checked,
		Items: []dto.Capability{
			{Name: "hardware.nic_optics", State: dto.CapabilityAbsent, Detail: "no NIC with optics or temperature sensors"},
		},
	})
	refresher := &fakeCapabilityRefresher{caps: dto.Capabilities{UnraidVersion: "7.3.3", Items: []dto.Capability{
		{Name: "hardware.nic_optics", Available: true, State: dto.CapabilityPresent},
	}}}
	s := &Server{ctx: &domain.Context{Platform: reg}}
	s.SetCapabilityRefresher(refresher)

	rec, out := capabilitiesRequest(t, s, "")
	if rec.Code != http.StatusOK || refresher.calls != 0 {
		t.Fatalf("snapshot: status=%d refresh calls=%d", rec.Code, refresher.calls)
	}
	if out.UnraidVersion != "7.3.3" || !out.CheckedAt.Equal(checked) || len(out.Items) != 1 || out.Items[0].State != dto.CapabilityAbsent {
		t.Errorf("snapshot = %+v", out)
	}

	rec, out = capabilitiesRequest(t, s, "?refresh=true")
	if rec.Code != http.StatusOK || refresher.calls != 1 || out.Items[0].State != dto.CapabilityPresent {
		t.Fatalf("refresh: status=%d calls=%d items=%+v", rec.Code, refresher.calls, out.Items)
	}

	if _, _ = capabilitiesRequest(t, s, "?refresh=false"); refresher.calls != 1 {
		t.Errorf("refresh=false refreshed")
	}
	if rec, _ = capabilitiesRequest(t, s, "?refresh=maybe"); rec.Code != http.StatusBadRequest {
		t.Errorf("invalid refresh: status = %d, want 400", rec.Code)
	}
}

// TestCapabilitiesEndpointGuards covers a nil registry (503) and a nil refresher (current snapshot).
func TestCapabilitiesEndpointGuards(t *testing.T) {
	rec, _ := capabilitiesRequest(t, &Server{ctx: &domain.Context{}}, "?refresh=true")
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("nil registry: status = %d, want 503", rec.Code)
	}

	reg := platform.NewRegistry()
	reg.SetCapabilities(dto.Capabilities{UnraidVersion: "7.3.3"})
	rec, out := capabilitiesRequest(t, &Server{ctx: &domain.Context{Platform: reg}}, "?refresh=true")
	if rec.Code != http.StatusOK || out.UnraidVersion != "7.3.3" {
		t.Errorf("nil refresher: status = %d, body = %+v; want the current snapshot", rec.Code, out)
	}
}

// TestCapabilitiesRouteRegistered checks GET /api/v1/capabilities is routed.
func TestCapabilitiesRouteRegistered(t *testing.T) {
	reg := platform.NewRegistry()
	reg.SetCapabilities(dto.Capabilities{UnraidVersion: "7.3.3"})
	ctx := &domain.Context{Hub: domain.NewEventBus(10), Platform: reg}
	s := NewServer(ctx)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/capabilities", nil)
	rec := httptest.NewRecorder()
	s.router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/capabilities: status = %d, want 200", rec.Code)
	}
}
