package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gorilla/mux"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

// handleAgentStartSession starts a new on-demand agent session and returns the result.
func (s *Server) handleAgentStartSession(w http.ResponseWriter, r *http.Request) {
	if s.agentSvc == nil || !s.agentSvc.Enabled() {
		respondJSON(w, http.StatusServiceUnavailable, dto.Response{
			Success: false, Message: "agent is disabled", Timestamp: time.Now(),
		})
		return
	}
	var body struct {
		Goal string `json:"goal"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Goal == "" {
		respondWithError(w, http.StatusBadRequest, "request body must include a non-empty 'goal'")
		return
	}
	sess, err := s.agentSvc.StartSession(r.Context(), body.Goal)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, sess)
}

// handleAgentListSessions returns all agent sessions, newest-first.
func (s *Server) handleAgentListSessions(w http.ResponseWriter, _ *http.Request) {
	if s.agentSvc == nil {
		respondJSON(w, http.StatusServiceUnavailable, dto.Response{
			Success: false, Message: "agent is disabled", Timestamp: time.Now(),
		})
		return
	}
	respondJSON(w, http.StatusOK, s.agentSvc.ListSessions())
}

// handleAgentGetSession returns a single agent session by ID.
func (s *Server) handleAgentGetSession(w http.ResponseWriter, r *http.Request) {
	if s.agentSvc == nil {
		respondJSON(w, http.StatusServiceUnavailable, dto.Response{
			Success: false, Message: "agent is disabled", Timestamp: time.Now(),
		})
		return
	}
	id := mux.Vars(r)["id"]
	sess, ok := s.agentSvc.GetSession(id)
	if !ok {
		respondWithError(w, http.StatusNotFound, "session not found")
		return
	}
	respondJSON(w, http.StatusOK, sess)
}
