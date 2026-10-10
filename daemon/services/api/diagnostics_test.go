package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestHandleDiagnosticsBundle_Success(t *testing.T) {
	mockBundle := &dto.DiagnosticBundle{
		Metadata: dto.BundleMetadata{
			Hostname: "testhost",
		},
	}
	payload := []byte("PK\x03\x04mock-zip-content")

	s := &Server{
		ctx: &domain.Context{},
		collectDiagnosticsFn: func(_ context.Context, _ *domain.Context) (*dto.DiagnosticBundle, error) {
			return mockBundle, nil
		},
		writeArchiveFn: func(w io.Writer, bundle *dto.DiagnosticBundle) error {
			if bundle != mockBundle {
				t.Errorf("writeArchive received unexpected bundle: %v", bundle)
			}
			_, err := w.Write(payload)
			return err
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/diagnostics/bundle", nil)
	rec := httptest.NewRecorder()
	s.handleDiagnosticsBundle(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	if ct := rec.Header().Get("Content-Type"); ct != "application/zip" {
		t.Errorf("Content-Type = %q, want application/zip", ct)
	}

	if cd := rec.Header().Get("Content-Disposition"); !strings.Contains(cd, "unraid-diagnostics-testhost-") || !strings.HasSuffix(cd, `.zip"`) {
		t.Errorf("Content-Disposition = %q, want attachment with filename", cd)
	}

	if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", cc)
	}

	if rec.Body.String() != string(payload) {
		t.Errorf("body = %q, want %q", rec.Body.String(), string(payload))
	}
}

func TestHandleDiagnosticsBundle_CollectError(t *testing.T) {
	s := &Server{
		ctx: &domain.Context{},
		collectDiagnosticsFn: func(_ context.Context, _ *domain.Context) (*dto.DiagnosticBundle, error) {
			return nil, errors.New("collection failed")
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/diagnostics/bundle", nil)
	rec := httptest.NewRecorder()
	s.handleDiagnosticsBundle(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}

	var resp dto.Response
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Success {
		t.Errorf("expected Success = false, got true")
	}
	if !strings.Contains(resp.Message, "Failed to collect diagnostics") {
		t.Errorf("unexpected message: %s", resp.Message)
	}
}

func TestHandleDiagnosticsBundle_WriteArchiveError(t *testing.T) {
	mockBundle := &dto.DiagnosticBundle{
		Metadata: dto.BundleMetadata{
			Hostname: "testhost",
		},
	}
	s := &Server{
		ctx: &domain.Context{},
		collectDiagnosticsFn: func(_ context.Context, _ *domain.Context) (*dto.DiagnosticBundle, error) {
			return mockBundle, nil
		},
		writeArchiveFn: func(_ io.Writer, _ *dto.DiagnosticBundle) error {
			return errors.New("archive write failed")
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/diagnostics/bundle", nil)
	rec := httptest.NewRecorder()
	s.handleDiagnosticsBundle(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}

	var resp dto.Response
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Success {
		t.Errorf("expected Success = false, got true")
	}
	if !strings.Contains(resp.Message, "Failed to build diagnostics archive") {
		t.Errorf("unexpected message: %s", resp.Message)
	}
}

func TestHandleDiagnosticsBundle_DefaultWriteArchive(t *testing.T) {
	mockBundle := &dto.DiagnosticBundle{
		Metadata: dto.BundleMetadata{
			Hostname: "testhost",
		},
	}
	s := &Server{
		ctx: &domain.Context{},
		collectDiagnosticsFn: func(_ context.Context, _ *domain.Context) (*dto.DiagnosticBundle, error) {
			return mockBundle, nil
		},
		// writeArchiveFn is nil, exercising diagnostics.WriteArchive fallback
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/diagnostics/bundle", nil)
	rec := httptest.NewRecorder()
	s.handleDiagnosticsBundle(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/zip" {
		t.Errorf("Content-Type = %q, want application/zip", ct)
	}
}

