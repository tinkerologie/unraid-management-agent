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
	case "catalog_unavailable":
		status = http.StatusServiceUnavailable
	case "catalog_unsupported":
		status = http.StatusNotImplemented
	case "listing_unavailable":
		status = http.StatusNotFound
	}
	respondJSON(w, status, problem)
}
