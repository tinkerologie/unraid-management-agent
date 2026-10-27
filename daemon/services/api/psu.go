package api

import (
	"net/http"
	"time"

	"github.com/tinkerologie/unraid-management-agent/daemon/dto"
)

// GetPSUCache returns the immutable shared PSU snapshot.
func (c *CacheStore) GetPSUCache() *dto.PSUStatus { return c.psuCache.Load() }

// handlePSU godoc
//
// @Summary Get PSU telemetry
// @Description Read host-visible PSU interfaces and per-device monitoring state. Corsair RMi/HXi use kernel hwmon. Output power is DC watts; unsupported AC input power and efficiency are null. Load is derived from verified capacity. No control operations.
// @Tags Hardware
// @Produce json
// @Success 200 {object} dto.PSUStatus
// @Router /psu [get]
func (s *Server) handlePSU(w http.ResponseWriter, _ *http.Request) {
	if cached := s.GetPSUCache(); cached != nil {
		respondJSON(w, http.StatusOK, cached)
		return
	}
	respondJSON(w, http.StatusOK, dto.NewPSUStatus(dto.PSUStatePending, time.Now()))
}
