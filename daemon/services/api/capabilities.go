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
//	@Description	Read the capability snapshot for OS paths, binaries, and detected hardware. Hardware classes include hardware.psu, hardware.nic_optics, hardware.storage_topology, hardware.gpu, hardware.ups, hardware.nut, hardware.zfs, and hardware.fan_control. A result can include a state (present, absent, or unknown) and a detail that explains the result. The snapshot checked_at field gives the time of the check. The agent checks PSU inventory every 30 seconds and on USB or hwmon events. It checks other capabilities every 15 minutes. Use refresh=true to request a new check. The agent uses the cached snapshot if the previous check was less than 30 seconds ago. Automatic collectors follow the hardware state.
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
