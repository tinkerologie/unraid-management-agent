package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/tinkerologie/unraid-management-agent/daemon/domain"
	"github.com/tinkerologie/unraid-management-agent/daemon/services/communityapps"
	"github.com/tinkerologie/unraid-management-agent/daemon/services/internal/communityappstest"
)

func TestCommunityAppsRESTUsesReadPolicy(t *testing.T) {
	root := os.DirFS(communityappstest.CopyHost(t))
	server := NewServer(&domain.Context{Config: domain.Config{APIToken: "test-catalog-token", WebSocketDisabled: true}})
	t.Cleanup(server.Stop)
	server.SetCommunityAppsCatalog(communityapps.NewCatalog(root))

	request := func(path string, token string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		out := httptest.NewRecorder()
		server.GetRouter().ServeHTTP(out, req)
		return out
	}
	const route = "/api/v1/community-applications"
	if resp := request(route, ""); resp.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated search = %d, want 401", resp.Code)
	}
	search := request(route+"?q=fixture", "test-catalog-token")
	var result communityapps.SearchResult
	if err := json.Unmarshal(search.Body.Bytes(), &result); err != nil || search.Code != http.StatusOK || len(result.Listings) != 2 {
		t.Fatalf("search status = %d, result = %s", search.Code, search.Body)
	}
	template := request(route+"/"+result.Listings[1].Reference, "test-catalog-token")
	var inspection communityapps.Inspection
	if err := json.Unmarshal(template.Body.Bytes(), &inspection); err != nil || template.Code != http.StatusOK || inspection.Listing.Variant != "edge" {
		t.Fatalf("inspect status = %d, result = %s", template.Code, template.Body)
	}
	if strings.Contains(template.Body.String(), "sentinel") {
		t.Fatal("template response disclosed a sensitive value")
	}
	if inspect := request(route+"/%2e%2e", "test-catalog-token"); inspect.Code == http.StatusOK {
		t.Fatal("inspection accepted an invalid listing reference")
	}

	server.SetCommunityAppsCatalog(communityapps.NewCatalog(fstest.MapFS{}))
	cold := request(route, "test-catalog-token")
	checkCatalogProblem(t, cold, http.StatusServiceUnavailable, "catalog_unavailable")
}

func checkCatalogProblem(t *testing.T, resp *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	var problem communityapps.Problem
	if err := json.Unmarshal(resp.Body.Bytes(), &problem); err != nil || resp.Code != status || problem.Code != code {
		t.Fatalf("received status %d and %q, want %d and %q", resp.Code, resp.Body, status, code)
	}
}
