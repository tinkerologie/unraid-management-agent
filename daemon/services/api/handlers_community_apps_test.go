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
	"time"

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

func TestCommunityAppRESTConfirmAndPoll(t *testing.T) {
	root := communityappstest.CopyHeimdallHost(t)
	unblock := make(chan struct{})
	docker := &communityappstest.InstallEngine{BeforePull: func(ctx context.Context) error {
		select {
		case <-unblock:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}}
	service := communityapps.NewService(communityapps.NewCatalog(os.DirFS(root)), communityapps.ServiceOptions{
		HostRoot:          root,
		OpenDocker:        func() (communityapps.PlanningDocker, error) { return docker, nil },
		OpenInstallDocker: func() (communityapps.InstallDocker, error) { return docker, nil },
		PortAvailable:     func(context.Context, communityapps.PortBinding) error { return nil },
	})
	t.Cleanup(service.Close)
	preview, err := service.Prepare(t.Context(), communityapps.PrepareRequest{
		Reference: communityappstest.HeimdallReference, Environment: map[string]string{"PUID": "private-install-api"},
	})
	if err != nil || !preview.Eligible {
		t.Fatalf("prepare returned %+v, %v", preview, err)
	}
	const bearer = "fixture-install-auth"
	server := NewServer(&domain.Context{Config: domain.Config{APIToken: bearer, WebSocketDisabled: true}})
	t.Cleanup(server.Stop)
	server.SetCommunityAppsService(service)

	request := func(ctx context.Context, method, path, body, token string) *httptest.ResponseRecorder {
		req := httptest.NewRequestWithContext(ctx, method, path, strings.NewReader(body))
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		resp := httptest.NewRecorder()
		server.GetRouter().ServeHTTP(resp, req)
		if strings.Contains(resp.Body.String(), "private-install-api") || strings.Contains(resp.Body.String(), preview.ApprovalToken) {
			t.Fatal("attempt response exposed a sensitive value")
		}
		return resp
	}

	const base = "/api/v1/community-applications"
	path := base + "/install-plans/" + preview.PlanID + "/confirm"
	checkCatalogProblem(t, request(t.Context(), http.MethodPost, path, `{"approval_token":"true"}`, bearer), http.StatusBadRequest, "invalid_plan_approval")
	checkCatalogProblem(t, request(t.Context(), http.MethodPost, path, `{"approval_token":42}`, bearer), http.StatusBadRequest, "invalid_plan_request")
	tokenBody := `{"approval_token":"` + preview.ApprovalToken + `"}`
	if resp := request(t.Context(), http.MethodPost, path, tokenBody, ""); resp.Code != http.StatusUnauthorized {
		t.Fatalf("tokenless confirmation was accepted: %d", resp.Code)
	}
	checkCatalogProblem(t, request(t.Context(), http.MethodPost, base+"/install-plans/"+strings.Repeat("A", 26)+"/confirm", tokenBody, bearer), http.StatusGone, "plan_expired")
	checkCatalogProblem(t, request(t.Context(), http.MethodGet, base+"/install-attempts/"+strings.Repeat("A", 26), "", bearer), http.StatusNotFound, "attempt_not_found")

	ctx, disconnect := context.WithCancel(t.Context())
	defer disconnect()
	resp := request(ctx, http.MethodPost, path, tokenBody, bearer)
	disconnect()
	var claim communityapps.Attempt
	if err := json.Unmarshal(resp.Body.Bytes(), &claim); err != nil || resp.Code != http.StatusAccepted || claim.ID == "" {
		t.Fatalf("confirm returned %d: %+v, %v", resp.Code, claim, err)
	}
	location := resp.Header().Get("Location")
	if location != base+"/install-attempts/"+claim.ID {
		t.Fatalf("unexpected attempt location %q", location)
	}
	poll := func() communityapps.Attempt {
		t.Helper()
		resp := request(t.Context(), http.MethodGet, location, "", bearer)
		var attempt communityapps.Attempt
		if err := json.Unmarshal(resp.Body.Bytes(), &attempt); err != nil || resp.Code != http.StatusOK || attempt.ID != claim.ID || resp.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("poll returned %d: %+v, %v", resp.Code, attempt, err)
		}
		return attempt
	}
	if pending := poll(); pending.State != communityapps.AttemptPending && pending.State != communityapps.AttemptRunning {
		t.Fatalf("slow install did not remain active: %+v", pending)
	}
	close(unblock)
	deadline := time.Now().Add(5 * time.Second)
	for state := poll(); state.State != communityapps.AttemptSucceeded; state = poll() {
		if time.Now().After(deadline) || state.State == communityapps.AttemptFailed {
			t.Fatalf("install did not succeed: %+v", state)
		}
		time.Sleep(10 * time.Millisecond)
	}
	replay := request(t.Context(), http.MethodPost, path, tokenBody, bearer)
	var done communityapps.Attempt
	if err := json.Unmarshal(replay.Body.Bytes(), &done); err != nil || replay.Code != http.StatusAccepted ||
		done.ID != claim.ID || done.State != communityapps.AttemptSucceeded || done.ContainerID != communityappstest.ContainerID {
		t.Fatalf("replay was %d: %+v, %v", replay.Code, done, err)
	}
}
