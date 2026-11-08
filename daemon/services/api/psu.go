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
// @Description Read host-visible PSU interfaces and the monitoring state of each PSU. The agent reads Corsair RMi and HXi devices through kernel hwmon. Output power is in DC watts. Unsupported AC input power and efficiency are null. The agent uses verified capacity to calculate load. This route gives telemetry only.
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
