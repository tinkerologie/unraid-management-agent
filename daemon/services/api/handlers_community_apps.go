package api

import (
	"net/http"

	"github.com/gorilla/mux"

	"github.com/tinkerologie/unraid-management-agent/daemon/services/communityapps"
)

// SetCommunityAppsCatalog wires the host catalog before serving requests.
func (s *Server) SetCommunityAppsCatalog(catalog *communityapps.Catalog) {
	s.communityAppsCatalog = catalog
}

// handleCommunityAppsSearch returns eligible-to-display CA Docker listings.
//
// @Summary Search Community Applications
// @Description Search the host's filtered Community Applications Docker catalog
// @Tags Community Applications
// @Produce json
// @Param q query string false "Search term (maximum 256 bytes)"
// @Success 200 {object} communityapps.SearchResult
// @Failure 400 {object} communityapps.Problem
// @Failure 501 {object} communityapps.Problem
// @Failure 503 {object} communityapps.Problem
// @Router /community-applications [get]
func (s *Server) handleCommunityAppsSearch(w http.ResponseWriter, r *http.Request) {
	result, err := s.communityAppsCatalog.Search(r.Context(), r.URL.Query().Get("q"))
	if err != nil {
		respondCatalogError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, result)
}

// handleCommunityAppInspection returns redacted published template settings.
//
// @Summary Inspect a Community Applications listing
// @Description Show the exact listing's editable fields and known install eligibility issues
// @Tags Community Applications
// @Produce json
// @Param reference path string true "Listing reference from search"
// @Success 200 {object} communityapps.Inspection
// @Failure 400 {object} communityapps.Problem
// @Failure 404 {object} communityapps.Problem
// @Failure 501 {object} communityapps.Problem
// @Failure 503 {object} communityapps.Problem
// @Router /community-applications/{reference} [get]
func (s *Server) handleCommunityAppInspection(w http.ResponseWriter, r *http.Request) {
	result, err := s.communityAppsCatalog.Inspect(r.Context(), mux.Vars(r)["reference"])
	if err != nil {
		respondCatalogError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, result)
}

func respondCatalogError(w http.ResponseWriter, err error) {
	problem := communityapps.ProblemForError(err)
	status := http.StatusBadRequest
	switch problem.Code {
	case "catalog_request_failed":
		status = http.StatusInternalServerError
	case "catalog_unavailable", "docker_unavailable", "storage_unavailable", "image_digest_unavailable", "attempt_storage_unavailable", "install_worker_unavailable":
		status = http.StatusServiceUnavailable
	case "catalog_unsupported":
		status = http.StatusNotImplemented
	case "listing_unavailable", "attempt_not_found":
		status = http.StatusNotFound
	case "credential_required":
		status = http.StatusUnauthorized
	case "read_only":
		status = http.StatusForbidden
	case "name_conflict", "port_conflict", "shared_appdata", "exposed_mount_parent", "onboarding_required", "plan_changed", "install_recovery_pending":
		status = http.StatusConflict
	case "plan_expired":
		status = http.StatusGone
	case "plan_capacity", "install_capacity":
		status = http.StatusTooManyRequests
	}
	respondJSON(w, status, problem)
}

// CommunityApps returns the catalog and install service shared with HTTP MCP.
func (s *Server) CommunityApps() (*communityapps.Catalog, *communityapps.Service) {
	return s.communityAppsCatalog, s.communityAppsService
}

// SetCommunityAppsService replaces the install service before the API starts.
// Stop closes the attached service.
func (s *Server) SetCommunityAppsService(service *communityapps.Service) {
	s.communityAppsService = service
}

func (s *Server) requireCommunityAppsCredential(w http.ResponseWriter, r *http.Request) bool {
	w.Header().Set("Cache-Control", "no-store")
	if s.ctx.ReadOnly {
		respondCatalogError(w, &communityapps.Problem{Code: "read_only", Message: "Install preparation and confirmation require write access."})
		return false
	}
	if !hasVerifiedCredential(r.Context()) {
		w.Header().Set("WWW-Authenticate", `Bearer realm="unraid-management-agent"`)
		respondCatalogError(w, communityapps.ErrPlanCredential)
		return false
	}
	return true
}

// handleCommunityAppsPrepare returns a redacted, short-lived install preview.
//
// @Summary Prepare a Community Applications install
// @Description Resolve exact effective settings and an image digest without installing
// @Tags Community Applications
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body communityapps.PrepareRequest true "Listing and declared overrides"
// @Success 200 {object} communityapps.Preview
// @Failure 400 {object} communityapps.Problem
// @Failure 401 {object} communityapps.Problem
// @Failure 403 {object} communityapps.Problem
// @Failure 404 {object} communityapps.Problem
// @Failure 409 {object} communityapps.Problem
// @Failure 429 {object} communityapps.Problem
// @Failure 501 {object} communityapps.Problem
// @Failure 503 {object} communityapps.Problem
// @Router /community-applications/install-plans [post]
func (s *Server) handleCommunityAppsPrepare(w http.ResponseWriter, r *http.Request) {
	if !s.requireCommunityAppsCredential(w, r) {
		return
	}
	args, err := communityapps.DecodePlanArguments[communityapps.PrepareRequest](r.Body)
	if err != nil {
		respondCatalogError(w, err)
		return
	}
	result, err := s.communityAppsService.Prepare(r.Context(), args)
	if err != nil {
		respondCatalogError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, result)
}

// handleCommunityAppsConfirm accepts an exact approved install plan.
//
// @Summary Confirm a Community Applications install
// @Description Claim one approved plan and return its durable attempt. Repeat the same approval to retrieve the original attempt.
// @Tags Community Applications
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Pending install plan ID"
// @Param request body communityapps.ConfirmRequest true "Exact approval token from the reviewed preview"
// @Success 202 {object} communityapps.Attempt
// @Failure 400 {object} communityapps.Problem
// @Failure 401 {object} communityapps.Problem
// @Failure 403 {object} communityapps.Problem
// @Failure 404 {object} communityapps.Problem
// @Failure 409 {object} communityapps.Problem
// @Failure 410 {object} communityapps.Problem
// @Failure 429 {object} communityapps.Problem
// @Failure 503 {object} communityapps.Problem
// @Router /community-applications/install-plans/{id}/confirm [post]
func (s *Server) handleCommunityAppsConfirm(w http.ResponseWriter, r *http.Request) {
	if !s.requireCommunityAppsCredential(w, r) {
		return
	}
	args, err := communityapps.DecodePlanArguments[communityapps.ConfirmRequest](r.Body)
	if err != nil {
		respondCatalogError(w, err)
		return
	}
	attempt, err := s.communityAppsService.Confirm(r.Context(), mux.Vars(r)["id"], args.ApprovalToken)
	if err != nil {
		respondCatalogError(w, err)
		return
	}
	w.Header().Set("Location", "/api/v1/community-applications/install-attempts/"+attempt.ID)
	respondJSON(w, http.StatusAccepted, attempt)
}

// handleCommunityAppsAttempt returns one confirmed install's redacted status.
//
// @Summary Get a Community Applications install attempt
// @Description Return durable progress or the final install result, including any cleanup details after recovery.
// @Tags Community Applications
// @Produce json
// @Param id path string true "Install attempt ID"
// @Success 200 {object} communityapps.Attempt
// @Failure 404 {object} communityapps.Problem
// @Failure 503 {object} communityapps.Problem
// @Router /community-applications/install-attempts/{id} [get]
func (s *Server) handleCommunityAppsAttempt(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	attempt, err := s.communityAppsService.GetAttempt(mux.Vars(r)["id"])
	if err != nil {
		respondCatalogError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, attempt)
}
