package api

import (
	"net/http"
	"time"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
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
