package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/moby/moby/api/types/container"

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

func TestPrepareCommunityAppRESTRequiresVerifiedBearer(t *testing.T) {
	root := communityappstest.CopyHeimdallHost(t)
	service := communityapps.NewService(communityapps.NewCatalog(os.DirFS(root)), communityapps.ServiceOptions{
		HostRoot: root,
		OpenDocker: func() (communityapps.PlanningDocker, error) {
			return &communityappstest.Docker{Containers: []container.Summary{{Names: []string{"/already-installed"}}}}, nil
		},
		PortAvailable: func(context.Context, communityapps.PortBinding) error { return nil },
	})
	const path = "/api/v1/community-applications/install-plans"
	const body = `{"reference":"` + communityappstest.HeimdallReference + `","environment":{"PUID":"private-preview-sentinel"}}`
	for _, tc := range []struct {
		name, token, presented, body string
		tokenless, readOnly          bool
		status                       int
		code                         string
	}{
		{name: "ordinary tokenless write lock", body: body, status: http.StatusForbidden},
		{name: "unconfigured bearer and forged proxy proof", presented: "forged", body: body, tokenless: true, status: http.StatusUnauthorized, code: "credential_required"},
		{name: "invalid configured bearer", token: "real-token", presented: "wrong", body: body, tokenless: true, status: http.StatusUnauthorized},
		{name: "read-only refuses prepare", token: "real-token", presented: "real-token", body: body, readOnly: true, status: http.StatusForbidden, code: "read_only"},
		{name: "malformed secret input", token: "real-token", presented: "real-token", body: `{"environment":{"private-preview-sentinel":{}}}`, status: http.StatusBadRequest, code: "invalid_plan_request"},
		{name: "current host name conflict", token: "real-token", presented: "real-token", body: `{"reference":"` + communityappstest.HeimdallReference + `","name":"already-installed"}`, status: http.StatusConflict, code: "name_conflict"},
		{name: "authenticated redacted preview", token: "real-token", presented: "real-token", body: body, status: http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := NewServer(&domain.Context{Config: domain.Config{
				APIToken: tc.token, AllowUnauthenticatedWrites: tc.tokenless, WebSocketDisabled: true, ReadOnly: tc.readOnly,
			}})
			t.Cleanup(server.Stop)
			server.SetCommunityAppsService(service)
			req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(tc.body))
			req.RemoteAddr = "127.0.0.1:42422"
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Forwarded-For", "203.0.113.11")
			req.Header.Set("X-Verified-Credential", "true")
			if tc.presented != "" {
				req.Header.Set("Authorization", "Bearer "+tc.presented)
			}
			out := httptest.NewRecorder()
			server.GetRouter().ServeHTTP(out, req)
			if strings.Contains(out.Body.String(), "private-preview-sentinel") || out.Code != tc.status {
				t.Fatalf("response status=%d body=%s", out.Code, out.Body)
			}
			if tc.code != "" {
				checkCatalogProblem(t, out, tc.status, tc.code)
			}
			if tc.status == http.StatusOK {
				var preview communityapps.Preview
				if err := json.Unmarshal(out.Body.Bytes(), &preview); err != nil || preview.ApprovalToken == "" || out.Header().Get("Cache-Control") != "no-store" {
					t.Fatalf("invalid preview: %+v, %v", preview, err)
				}
			}
		})
	}
}
