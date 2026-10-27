package api

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/tinkerologie/unraid-management-agent/daemon/dto"
)

// CapabilityRefresher re-checks capabilities on demand (rate-limited by the
// implementation) and returns the resulting snapshot.
type CapabilityRefresher interface {
	Refresh(ctx context.Context) dto.Capabilities
}

// SetCapabilityRefresher sets the refresher used by GET /capabilities?refresh=true.
func (s *Server) SetCapabilityRefresher(r CapabilityRefresher) {
	s.capabilityRefresher = r
}

// handleCapabilities godoc
//
//	@Summary		Get detected capabilities
//	@Description	Returns the capability snapshot: OS paths and binaries plus detected hardware classes (hardware.psu, hardware.nic_optics, hardware.storage_topology, hardware.gpu, hardware.ups, hardware.nut, hardware.zfs, hardware.fan_control), each with state present/absent/unknown and a detail reason, and when the snapshot was checked. PSU inventory also refreshes on USB/hwmon events and every 30 seconds. Other capabilities are re-checked every 15 minutes; refresh=true re-checks now (at most every 30 seconds, otherwise the cached snapshot is returned). Collectors in automatic mode follow the hardware state.
//	@Tags			Diagnostics
//	@Produce		json
//	@Param			refresh	query		bool	false	"Re-check capabilities now (rate-limited)"
//	@Success		200		{object}	dto.Capabilities
//	@Failure		400		{object}	dto.Response	"Invalid refresh value"
//	@Failure		503		{object}	dto.Response	"Capability detection is not available"
//	@Router			/capabilities [get]
func (s *Server) handleCapabilities(w http.ResponseWriter, r *http.Request) {
	refresh := false
	if v := r.URL.Query().Get("refresh"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			respondJSON(w, http.StatusBadRequest, dto.Response{Success: false, Message: "refresh must be true or false", Timestamp: time.Now()})
			return
		}
		refresh = b
	}
	reg := s.ctx.Platform
	if reg == nil {
		respondJSON(w, http.StatusServiceUnavailable, dto.Response{Success: false, Message: "capability detection is not available", Timestamp: time.Now()})
		return
	}
	if refresh && s.capabilityRefresher != nil {
		respondJSON(w, http.StatusOK, s.capabilityRefresher.Refresh(r.Context()))
		return
	}
	respondJSON(w, http.StatusOK, reg.Capabilities())
}
