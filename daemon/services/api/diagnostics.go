package api

import (
	"bytes"
	"fmt"
	"net/http"
	"time"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/services/diagnostics"
)

// selfTestResponse is the payload of GET /api/v1/diagnostics/self-test. It
// reports the detected Unraid version, the worst current subsystem state, the
// startup capability snapshot, and the per-subsystem source health — so an
// operator (or AI agent) can tell at a glance whether an OS update has broken a
// data source. Each subsystem includes last_healthy so a persistent degraded
// state can be dated (issue #123).
type selfTestResponse struct {
	UnraidVersion string             `json:"unraid_version"`
	OverallState  dto.SourceState    `json:"overall_state"`
	Capabilities  dto.Capabilities   `json:"capabilities"`
	Subsystems    []dto.SourceStatus `json:"subsystems"`
	Timestamp     time.Time          `json:"timestamp"`
}

// handleSelfTest godoc
//
//	@Summary		Run agent self-test
//	@Description	Returns the detected Unraid version, overall data-source health, probed capabilities, and per-subsystem source status (healthy/degraded/unavailable).
//	@Tags			Diagnostics
//	@Produce		json
//	@Success		200	{object}	selfTestResponse
//	@Router			/diagnostics/self-test [get]
func (s *Server) handleSelfTest(w http.ResponseWriter, _ *http.Request) {
	reg := s.ctx.Platform
	if reg == nil {
		respondJSON(w, http.StatusOK, selfTestResponse{
			OverallState: dto.SourceHealthy,
			Timestamp:    time.Now(),
		})
		return
	}
	respondJSON(w, http.StatusOK, selfTestResponse{
		UnraidVersion: reg.Capabilities().UnraidVersion,
		OverallState:  reg.OverallState(),
		Capabilities:  reg.Capabilities(),
		Subsystems:    reg.Snapshot(),
		Timestamp:     time.Now(),
	})
}

// handleDiagnosticsBundle godoc
//
//	@Summary		Download diagnostics bundle
//	@Description	Collects a redacted diagnostics bundle (system state, array, containers, VMs, network, recent agent/syslog logs, and redacted configuration) and returns it as a downloadable ZIP archive. Safe to attach to bug reports — secrets (MQTT credentials, etc.) are redacted. Enable Debug Logging first for richer agent logs in the bundle.
//	@Tags			Diagnostics
//	@Produce		application/zip
//	@Success		200	{string}	binary	"ZIP archive (binary)"
//	@Failure		500	{object}	dto.Response	"Failed to build diagnostics bundle"
//	@Router			/diagnostics/bundle [get]
func (s *Server) handleDiagnosticsBundle(w http.ResponseWriter, r *http.Request) {
	bundle, err := diagnostics.NewBundleService(s.ctx).CollectDiagnostics(r.Context())
	if err != nil {
		// Full detail to the log; a generic message to the client (the raw error
		// can contain internal paths).
		logger.Error("Diagnostics bundle collection failed: %v", err)
		respondJSON(w, http.StatusInternalServerError, dto.Response{
			Success:   false,
			Message:   "Failed to collect diagnostics — check the agent log",
			Timestamp: time.Now(),
		})
		return
	}

	// Buffer the archive in memory so a write error surfaces as a 500 instead of
	// a truncated download — the bundle is small (logs are capped to last-N lines).
	var buf bytes.Buffer
	if err := diagnostics.WriteArchive(&buf, bundle); err != nil {
		logger.Error("Diagnostics bundle archiving failed: %v", err)
		respondJSON(w, http.StatusInternalServerError, dto.Response{
			Success:   false,
			Message:   "Failed to build diagnostics archive — check the agent log",
			Timestamp: time.Now(),
		})
		return
	}

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", diagnostics.ArchiveFilename(bundle)))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", buf.Len()))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// The bundle contains host diagnostics — keep browsers/proxies from caching it.
	w.Header().Set("Cache-Control", "no-store")
	if _, err := w.Write(buf.Bytes()); err != nil {
		logger.Warning("Diagnostics bundle write to client failed: %v", err)
	}
}
