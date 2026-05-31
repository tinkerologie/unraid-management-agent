package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/mux"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

// maxGoalLen bounds the agent session goal text to a sane size.
const maxGoalLen = 4096

// handleAgentStartSession starts a new on-demand agent session and returns the result.
//
//	@Summary		Start agent session
//	@Description	Start a new on-demand agent session for the given goal and run it to completion
//	@Tags			Agent
//	@Accept			json
//	@Produce		json
//	@Param			request	body		object{goal=string}	true	"Session goal"
//	@Success		200		{object}	dto.AgentSession		"Completed session"
//	@Failure		400		{object}	dto.Response			"Invalid request"
//	@Failure		500		{object}	dto.Response			"Failed to run session"
//	@Failure		503		{object}	dto.Response			"Agent disabled"
//	@Router			/agent/sessions [post]
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
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondWithError(w, http.StatusBadRequest, "request body must include a non-empty 'goal'")
		return
	}
	body.Goal = strings.TrimSpace(body.Goal)
	if body.Goal == "" {
		respondWithError(w, http.StatusBadRequest, "request body must include a non-empty 'goal'")
		return
	}
	if len(body.Goal) > maxGoalLen {
		respondWithError(w, http.StatusBadRequest, "goal too long")
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
//
//	@Summary		List agent sessions
//	@Description	Retrieve all agent sessions, newest first
//	@Tags			Agent
//	@Produce		json
//	@Success		200	{array}		dto.AgentSession	"List of sessions"
//	@Failure		503	{object}	dto.Response		"Agent disabled"
//	@Router			/agent/sessions [get]
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
//
//	@Summary		Get agent session
//	@Description	Retrieve a single agent session by ID
//	@Tags			Agent
//	@Produce		json
//	@Param			id	path		string				true	"Session ID"
//	@Success		200	{object}	dto.AgentSession	"Session"
//	@Failure		404	{object}	dto.Response		"Session not found"
//	@Failure		503	{object}	dto.Response		"Agent disabled"
//	@Router			/agent/sessions/{id} [get]
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

// handleAgentApprove resolves a pending approval and resumes the session.
//
//	@Summary		Approve or reject a pending agent action
//	@Description	Approve or reject a pending high-risk tool call, resuming the session
//	@Tags			Agent
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string								true	"Session ID"
//	@Param			request	body		object{action_id=string,approve=bool}	true	"Approval decision"
//	@Success		200		{object}	dto.AgentSession					"Updated session"
//	@Failure		400		{object}	dto.Response						"Invalid request or service error"
//	@Failure		503		{object}	dto.Response						"Agent disabled"
//	@Router			/agent/sessions/{id}/approve [post]
func (s *Server) handleAgentApprove(w http.ResponseWriter, r *http.Request) {
	if s.agentSvc == nil || !s.agentSvc.Enabled() {
		respondJSON(w, http.StatusServiceUnavailable, dto.Response{Success: false, Message: "agent is disabled", Timestamp: time.Now()})
		return
	}
	id := mux.Vars(r)["id"]
	var body struct {
		ActionID string `json:"action_id"`
		Approve  bool   `json:"approve"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ActionID == "" {
		respondWithError(w, http.StatusBadRequest, "request body must include 'action_id' and 'approve'")
		return
	}
	sess, err := s.agentSvc.ApproveAction(r.Context(), id, body.ActionID, body.Approve)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, sess)
}

// handleAgentCancel cancels a session.
//
//	@Summary		Cancel an agent session
//	@Description	Cancel a running or awaiting-approval agent session
//	@Tags			Agent
//	@Produce		json
//	@Param			id	path		string				true	"Session ID"
//	@Success		200	{object}	dto.AgentSession	"Cancelled session"
//	@Failure		400	{object}	dto.Response		"Service error"
//	@Failure		503	{object}	dto.Response		"Agent disabled"
//	@Router			/agent/sessions/{id}/cancel [post]
func (s *Server) handleAgentCancel(w http.ResponseWriter, r *http.Request) {
	if s.agentSvc == nil || !s.agentSvc.Enabled() {
		respondJSON(w, http.StatusServiceUnavailable, dto.Response{Success: false, Message: "agent is disabled", Timestamp: time.Now()})
		return
	}
	sess, err := s.agentSvc.CancelSession(mux.Vars(r)["id"])
	if err != nil {
		respondWithError(w, http.StatusBadRequest, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, sess)
}

// SystemJSON exposes the cached system info for the agent's read-only tools.
func (s *Server) SystemJSON() (any, bool) { v := s.GetSystemCache(); return v, v != nil }

// ArrayJSON exposes the cached array status for the agent's read-only tools.
func (s *Server) ArrayJSON() (any, bool) { v := s.GetArrayCache(); return v, v != nil }

// DockerJSON exposes the cached container list for the agent's read-only tools.
func (s *Server) DockerJSON() (any, bool) { v := s.GetDockerCache(); return v, v != nil }
