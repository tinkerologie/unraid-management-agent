package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"maps"
	"net/http"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/tinkerologie/unraid-management-agent/daemon/domain"
	"github.com/tinkerologie/unraid-management-agent/daemon/services/communityapps"
	"github.com/tinkerologie/unraid-management-agent/daemon/services/internal/communityappstest"
	"github.com/tinkerologie/unraid-management-agent/daemon/services/internal/communityappstest/recovery"
)

func TestCommunityAppsMCPRemainsReadableWithWritesLocked(t *testing.T) {
	server := NewServer(&domain.Context{ReadOnly: true}, newMockCacheProvider())
	server.SetNetworkWriteLock(true)
	server.SetCommunityAppsCatalog(communityapps.NewCatalog(os.DirFS(communityappstest.CopyHost(t))))
	if err := server.Initialize(); err != nil {
		t.Fatal(err)
	}
	cs, cleanup := connectClientToServer(t, server)
	defer cleanup()

	tools, err := cs.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"search_community_applications", "inspect_community_application"} {
		if !slices.ContainsFunc(tools.Tools, func(tool *mcp.Tool) bool {
			return tool.Name == name && tool.Annotations != nil && tool.Annotations.ReadOnlyHint
		}) {
			t.Fatalf("%s is unavailable or not read-only", name)
		}
	}
	result, content := callToolJSON(t, cs, "search_community_applications", map[string]any{"query": "fixture"})
	var response communityapps.SearchResult
	if err := json.Unmarshal([]byte(content), &response); err != nil || result.IsError || len(response.Listings) != 2 {
		t.Fatalf("search failed: %s", content)
	}
	result, content = callToolJSON(t, cs, "inspect_community_application", map[string]any{"reference": response.Listings[1].Reference})
	var inspection communityapps.Inspection
	if err := json.Unmarshal([]byte(content), &inspection); err != nil || result.IsError || inspection.Listing.Variant != "edge" {
		t.Fatalf("inspection failed: %s", content)
	}
	if strings.Contains(content, "sentinel") {
		t.Fatal("inspection disclosed a sensitive value")
	}
	result, content = callToolJSON(t, cs, "inspect_community_application", map[string]any{"reference": strings.Repeat("a", 64)})
	var problem communityapps.Problem
	if err := json.Unmarshal([]byte(content), &problem); err != nil || !result.IsError || problem.Code != "listing_unavailable" {
		t.Fatalf("missing listing did not produce an actionable tool error: %s", content)
	}
	result, content = callToolJSON(t, cs, "search_community_applications", map[string]any{"query": strings.Repeat("a", 257)})
	if err := json.Unmarshal([]byte(content), &problem); err != nil || !result.IsError || problem.Code != "invalid_search" {
		t.Fatalf("oversized search did not produce an actionable tool error: %s", content)
	}
}

func TestCommunityAppWriteToolRejectsUnsupportedArgumentSchema(t *testing.T) {
	server := NewServer(&domain.Context{}, newMockCacheProvider())
	if err := server.Initialize(); err != nil {
		t.Fatal(err)
	}
	type unsupportedArgs struct {
		Stream chan string
	}
	defer func() {
		err, ok := recover().(error)
		if !ok || !strings.Contains(err.Error(), "chan string") {
			t.Fatalf("expected a schema error for the unsupported argument type, got %v", err)
		}
	}()
	addCommunityAppsWriteTool(server, &mcp.Tool{Name: "unsupported_install_fixture"}, func(context.Context, unsupportedArgs) (*mcp.CallToolResult, any, error) {
		return nil, nil, nil
	})
}

func TestPrepareCommunityAppMCPRequiresPerRequestCredentialAndPolicy(t *testing.T) {
	root := communityappstest.CopyHeimdallHost(t)
	service := communityapps.NewService(communityapps.NewCatalog(os.DirFS(root)), communityapps.ServiceOptions{
		HostRoot:      root,
		OpenDocker:    func() (communityapps.PlanningDocker, error) { return &communityappstest.Docker{}, nil },
		PortAvailable: func(context.Context, communityapps.PortBinding) error { return nil },
	})
	const tool = "prepare_community_app_install"
	const (
		token    = "test-real-api-token"
		secret   = "mcp_fixture-mcp_fixture-mcp_fixture-mcp" // pragma: allowlist secret (synthetic test credential)
		sentinel = "private-preview-sentinel"
	)
	for _, tc := range []struct {
		name, endpoint, bearer string
		config                 domain.Config
		policy                 domain.ToolPolicyValue
		arguments              map[string]any
		want                   string
	}{
		{name: "stateless bearer receives eligible preview", endpoint: "/mcp-stateless", bearer: token, config: domain.Config{APIToken: token}},
		{name: "stateful connect secret receives eligible preview", endpoint: "/mcp/" + secret, config: domain.Config{MCPConnectSecret: secret, AllowUnauthenticatedWrites: true}},
		{name: "tokenless opt-in cannot authorize preparation", endpoint: "/mcp-stateless", config: domain.Config{AllowUnauthenticatedWrites: true}, want: "credential_required"},
		{name: "network write lock applies to accepted connect secret", endpoint: "/mcp/" + secret, config: domain.Config{MCPConnectSecret: secret}, want: writesLockedBlockedMessage},
		{name: "MCP read-only applies to accepted bearer", endpoint: "/mcp-stateless", bearer: token, config: domain.Config{APIToken: token, ReadOnly: true}, want: readOnlyBlockedMessage},
		{name: "read-only tool policy applies", endpoint: "/mcp-stateless", bearer: token, config: domain.Config{APIToken: token}, policy: domain.PolicyReadOnly, want: "set to read-only"},
		{name: "ask tool policy requires confirmation", endpoint: "/mcp-stateless", bearer: token, config: domain.Config{APIToken: token}, policy: domain.PolicyAsk, want: "Set confirm=true"},
		{name: "explicit confirmation satisfies ask to prepare", endpoint: "/mcp-stateless", bearer: token, config: domain.Config{APIToken: token}, policy: domain.PolicyAsk, arguments: map[string]any{"confirm": true}},
		{name: "withdrawn listing returns a stable problem", endpoint: "/mcp-stateless", bearer: token, config: domain.Config{APIToken: token}, arguments: map[string]any{"reference": strings.Repeat("a", 64)}, want: "listing_unavailable"},
		{name: "schema errors never echo secrets", endpoint: "/mcp-stateless", bearer: token, config: domain.Config{APIToken: token}, arguments: map[string]any{"environment": map[string]any{sentinel: map[string]any{}}}, want: "invalid_plan_request"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			policies := domain.NewToolPolicyStore(t.TempDir(), map[string]domain.ToolPolicyValue{tool: tc.policy})
			server, _ := mountMCPTestServerWithCommunityApps(t, tc.config, policies, service)
			version := "2026-07-28"
			if tc.endpoint != "/mcp-stateless" {
				version = "2025-11-25"
			}
			session := connectMCPHTTP(t, server, tc.endpoint, tc.bearer, version)
			args := map[string]any{"reference": communityappstest.HeimdallReference, "environment": map[string]string{"PUID": sentinel}}
			maps.Copy(args, tc.arguments)
			result, content := callToolJSON(t, session, tool, args)
			if strings.Contains(content, sentinel) {
				t.Fatalf("MCP returned sensitive input: %s", content)
			}
			if tc.want != "" {
				if !strings.Contains(content, tc.want) {
					t.Fatalf("reply = %s, want %q", content, tc.want)
				}
				return
			}
			var preview communityapps.Preview
			if err := json.Unmarshal([]byte(content), &preview); err != nil || result.IsError || !preview.Eligible || preview.ApprovalToken == "" {
				t.Fatalf("invalid MCP preview: %+v, %v, %s", preview, err, content)
			}
		})
	}

	t.Run("initializing through the secret does not authorize later tokenless calls", func(t *testing.T) {
		server, _ := mountMCPTestServerWithCommunityApps(t, domain.Config{
			MCPConnectSecret: secret, AllowUnauthenticatedWrites: true,
		}, nil, service)
		client := server.Client()
		session := doInitializeAndGetSession(t, client, server.URL+"/mcp/"+secret, "")
		body := `{"jsonrpc":"2.0","method":"tools/call","id":2,"params":{"name":"` + tool + `","arguments":{"reference":"` + communityappstest.HeimdallReference + `"},"_meta":{"verified":true}}}`
		request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, server.URL+"/mcp", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Accept", "application/json, text/event-stream")
		request.Header.Set("Mcp-Session-Id", session)
		request.Header.Set("Mcp-Protocol-Version", "2025-06-18")
		request.Header.Set("Authorization", "Bearer verified")
		request.Header.Set("X-Verified-Credential", "true")
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = response.Body.Close() }()
		reply, err := io.ReadAll(response.Body)
		if err != nil || response.StatusCode != http.StatusOK || !bytes.Contains(reply, []byte("credential_required")) {
			t.Fatalf("uncredentialed session call returned %d and %q, %v", response.StatusCode, reply, err)
		}
	})

	t.Run("local in-process MCP gives an actionable HTTP route", func(t *testing.T) {
		server := NewServer(&domain.Context{}, newMockCacheProvider())
		if err := server.Initialize(); err != nil {
			t.Fatal(err)
		}
		session, cleanup := connectClientToServer(t, server)
		defer cleanup()
		_, content := callToolJSON(t, session, tool, map[string]any{"reference": communityappstest.HeimdallReference})
		if !strings.Contains(content, "Streamable HTTP MCP") {
			t.Fatalf("received non-actionable error %q", content)
		}
	})
}

func TestCommunityAppMCPConfirmAndPollFromAnotherSession(t *testing.T) {
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
	const tool = "confirm_community_app_install"
	const statusTool = "get_community_app_install_attempt"
	const bearer = "install-auth"
	const protocol = "2026-07-28"
	const sentinel = "private-mcp-install"
	preview, err := service.Prepare(t.Context(), communityapps.PrepareRequest{
		Reference: communityappstest.HeimdallReference, Environment: map[string]string{"PUID": sentinel},
	})
	if err != nil || !preview.Eligible {
		t.Fatalf("prepare returned %+v, %v", preview, err)
	}

	policies := domain.NewToolPolicyStore(t.TempDir(), map[string]domain.ToolPolicyValue{tool: domain.PolicyAsk})
	server, _ := mountMCPTestServerWithCommunityApps(t, domain.Config{APIToken: bearer}, policies, service)
	const path = "/mcp-stateless"
	session := connectMCPHTTP(t, server, path, bearer, protocol)
	_, missingPolicyConsent := callToolJSON(t, session, tool, map[string]any{"plan_id": preview.PlanID, "approval_token": preview.ApprovalToken})
	if !strings.Contains(missingPolicyConsent, "Set confirm=true") {
		t.Fatalf("ordinary tool policy was bypassed: %s", missingPolicyConsent)
	}
	errorResult, missingExactApproval := callToolJSON(t, session, tool, map[string]any{"plan_id": preview.PlanID, "confirm": true})
	if !errorResult.IsError || !strings.Contains(missingExactApproval, "invalid_plan_approval") {
		t.Fatalf("exact approval was bypassed by generic confirmation: %s", missingExactApproval)
	}
	allowed := domain.NewToolPolicyStore(t.TempDir(), map[string]domain.ToolPolicyValue{tool: domain.PolicyAllow})
	allowServer, _ := mountMCPTestServerWithCommunityApps(t, domain.Config{APIToken: bearer}, allowed, service)
	allowSession := connectMCPHTTP(t, allowServer, path, bearer, protocol)
	errorResult, missingExactApproval = callToolJSON(t, allowSession, tool, map[string]any{"plan_id": preview.PlanID})
	if !errorResult.IsError || !strings.Contains(missingExactApproval, "invalid_plan_approval") {
		t.Fatalf("exact approval was bypassed by the allow policy: %s", missingExactApproval)
	}

	args := map[string]any{"plan_id": preview.PlanID, "approval_token": preview.ApprovalToken, "confirm": true}
	noToken, _ := mountMCPTestServerWithCommunityApps(t, domain.Config{AllowUnauthenticatedWrites: true}, nil, service)
	noTokenSession := connectMCPHTTP(t, noToken, path, "", protocol)
	errorResult, tokenless := callToolJSON(t, noTokenSession, tool, args)
	if !errorResult.IsError || !strings.Contains(tokenless, "credential_required") {
		t.Fatalf("tokenless confirmation was accepted: %s", tokenless)
	}
	result, reply := callToolJSON(t, session, tool, args)
	var claim communityapps.Attempt
	if err := json.Unmarshal([]byte(reply), &claim); err != nil || result.IsError || claim.ID == "" ||
		strings.Contains(reply, sentinel) || strings.Contains(reply, preview.ApprovalToken) {
		t.Fatalf("confirm did not return a redacted claim: %+v, %v", claim, err)
	}
	other := connectMCPHTTP(t, server, path, bearer, protocol)
	_, missing := callToolJSON(t, other, statusTool, map[string]any{"attempt_id": strings.Repeat("A", 26)})
	if !strings.Contains(missing, "attempt_not_found") {
		t.Fatalf("invalid status response: %s", missing)
	}
	poll := func() communityapps.Attempt {
		t.Helper()
		result, reply := callToolJSON(t, other, statusTool, map[string]any{"attempt_id": claim.ID})
		var state communityapps.Attempt
		if err := json.Unmarshal([]byte(reply), &state); err != nil || result.IsError || state.ID != claim.ID || strings.Contains(reply, sentinel) {
			t.Fatalf("could not poll from another session: %+v, %v", state, err)
		}
		return state
	}
	if state := poll(); state.State != communityapps.AttemptPending && state.State != communityapps.AttemptRunning {
		t.Fatalf("slow pull did not remain active: %+v", state)
	}
	close(unblock)
	deadline := time.Now().Add(5 * time.Second)
	for state := poll(); state.State != communityapps.AttemptSucceeded; state = poll() {
		if time.Now().After(deadline) || state.State == communityapps.AttemptFailed {
			t.Fatalf("install did not succeed: %+v", state)
		}
		time.Sleep(10 * time.Millisecond)
	}
	result, reply = callToolJSON(t, other, tool, args)
	var replay communityapps.Attempt
	if err := json.Unmarshal([]byte(reply), &replay); err != nil || result.IsError || replay.ID != claim.ID ||
		replay.State != communityapps.AttemptSucceeded || replay.ContainerID != communityappstest.ContainerID {
		t.Fatalf("confirmation replay returned %+v, %v", replay, err)
	}
}

func TestCommunityAppMCPReportsRecoveredAttempt(t *testing.T) {
	const environment = "private-recovered-mcp-environment"
	for _, started := range []bool{true, false} {
		name := "failed cleanup"
		if started {
			name = "completed"
		}
		t.Run(name, func(t *testing.T) {
			service, claim := recovery.InterruptedStart(t, environment, started)
			server, _ := mountMCPTestServerWithCommunityApps(t, domain.Config{}, nil, service)
			session := connectMCPHTTP(t, server, "/mcp-stateless", "", "2026-07-28")
			service.StartRecovery(t.Context())

			for deadline := time.Now().Add(5 * time.Second); ; {
				result, reply := callToolJSON(t, session, "get_community_app_install_attempt", map[string]any{"attempt_id": claim.ID})
				var attempt communityapps.Attempt
				if err := json.Unmarshal([]byte(reply), &attempt); err != nil || result.IsError ||
					attempt.ID != claim.ID || strings.Contains(reply, environment) {
					t.Fatalf("invalid recovered status: %+v, %v", attempt, err)
				}
				if attempt.State == communityapps.AttemptSucceeded || attempt.State == communityapps.AttemptFailed {
					if started && (attempt.State != communityapps.AttemptSucceeded || attempt.ContainerID != communityappstest.ContainerID) {
						t.Fatalf("success was not recovered: %+v", attempt)
					}
					if !started && (attempt.State != communityapps.AttemptFailed || attempt.Problem == nil ||
						attempt.Problem.Code != "container_start_failed" || len(attempt.CleanupDetails) != 1 ||
						attempt.CleanupDetails[0].ContainerID != communityappstest.ContainerID || !attempt.CleanupDetails[0].OwnershipVerified) {
						t.Fatalf("cleanup details were not recovered: %+v", attempt)
					}
					return
				}
				if time.Now().After(deadline) {
					t.Fatalf("recovery did not settle: %+v", attempt)
				}
				time.Sleep(5 * time.Millisecond)
			}
		})
	}
}
