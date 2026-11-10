package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/tinkerologie/unraid-management-agent/daemon/domain"
	"github.com/tinkerologie/unraid-management-agent/daemon/services/api"
)

func newMountedMCPTestServer(t *testing.T, apiToken, mcpSecret string) (*httptest.Server, *api.Server) {
	t.Helper()

	return mountMCPTestServer(t, domain.Config{APIToken: apiToken, MCPConnectSecret: mcpSecret}, nil)
}

func mountMCPTestServer(t *testing.T, config domain.Config, policies *domain.ToolPolicyStore) (*httptest.Server, *api.Server) {
	t.Helper()
	config.Version = "test-1.0.0"
	config.Port = 8043
	ctx := &domain.Context{Config: config, Hub: domain.NewEventBus(64)}

	apiServer := api.NewServer(ctx)
	mcpServer := NewServer(ctx, newMockCacheProvider())
	mcpServer.SetNetworkWriteLock(ctx.WritesLocked())
	mcpServer.SetToolPolicyStore(policies)
	if err := mcpServer.Initialize(); err != nil {
		t.Fatalf("Initialize() failed: %v", err)
	}
	apiServer.RegisterMCPRoutes(mcpServer.GetHTTPHandler())
	apiServer.RegisterMCPStatelessRoute(mcpServer.GetStatelessHTTPHandler())

	ts := httptest.NewServer(apiServer.GetRouter())
	t.Cleanup(func() {
		ts.Close()
		apiServer.Stop()
	})
	return ts, apiServer
}

func doInitializeAndGetSession(t *testing.T, client *http.Client, endpointURL, bearer string) string {
	t.Helper()

	initPayload := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"http-transport-test","version":"1.0"}}}`
	req, err := http.NewRequest(http.MethodPost, endpointURL, bytes.NewBufferString(initPayload))
	if err != nil {
		t.Fatalf("NewRequest initialize: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("POST initialize failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST initialize status = %d, want 200", resp.StatusCode)
	}
	sessionID := strings.TrimSpace(resp.Header.Get("Mcp-Session-Id"))
	if sessionID == "" {
		t.Fatal("expected Mcp-Session-Id header from initialize response")
	}

	notifPayload := `{"jsonrpc":"2.0","method":"notifications/initialized"}`
	nreq, err := http.NewRequest(http.MethodPost, endpointURL, bytes.NewBufferString(notifPayload))
	if err != nil {
		t.Fatalf("NewRequest notifications/initialized: %v", err)
	}
	nreq.Header.Set("Content-Type", "application/json")
	nreq.Header.Set("Accept", "application/json, text/event-stream")
	nreq.Header.Set("Mcp-Session-Id", sessionID)
	if bearer != "" {
		nreq.Header.Set("Authorization", "Bearer "+bearer)
	}
	nresp, err := client.Do(nreq)
	if err != nil {
		t.Fatalf("POST notifications/initialized failed: %v", err)
	}
	defer func() { _ = nresp.Body.Close() }()
	_, _ = io.ReadAll(nresp.Body)

	return sessionID
}

func TestMCPHTTPTransportGETAndRoutes(t *testing.T) {
	const (
		testAPIToken  = "test-api-token-1234567890"
		testMCPSecret = "mcp_secret_abcdefghijklmnopqrstuvwxyz123456"
	)

	ts, _ := newMountedMCPTestServer(t, testAPIToken, testMCPSecret)
	client := &http.Client{Timeout: 3 * time.Second}

	t.Run("sessionless GET returns framed 405 and completes before deadline", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, ts.URL+"/mcp", nil)
		if err != nil {
			t.Fatalf("NewRequest: %v", err)
		}
		req.Header.Set("Authorization", "Bearer "+testAPIToken)
		req.Header.Set("Accept", "text/event-stream")

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("GET /mcp failed or timed out: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("reading GET /mcp body failed: %v", err)
		}

		if resp.StatusCode != http.StatusMethodNotAllowed {
			t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusMethodNotAllowed)
		}
		if got := resp.Header.Get("Allow"); got != "POST, DELETE" {
			t.Errorf("Allow = %q, want %q", got, "POST, DELETE")
		}
		wantLen := strconv.Itoa(len(body))
		if got := resp.Header.Get("Content-Length"); got != wantLen || len(body) == 0 {
			t.Errorf("Content-Length = %q (body len %d), want %q", got, len(body), wantLen)
		}
	})

	t.Run("valid-session GET streams SSE immediately through loggingMiddleware", func(t *testing.T) {
		sessionID := doInitializeAndGetSession(t, client, ts.URL+"/mcp", testAPIToken)

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/mcp", nil)
		if err != nil {
			t.Fatalf("NewRequestWithContext: %v", err)
		}
		req.Header.Set("Authorization", "Bearer "+testAPIToken)
		req.Header.Set("Accept", "text/event-stream")
		req.Header.Set("Mcp-Session-Id", sessionID)

		streamClient := &http.Client{}
		resp, err := streamClient.Do(req)
		if err != nil {
			t.Fatalf("GET /mcp with session failed: %v", err)
		}
		defer func() {
			cancel()
			_ = resp.Body.Close()
		}()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", resp.StatusCode)
		}
		if got := resp.Header.Get("Content-Type"); !strings.HasPrefix(got, "text/event-stream") {
			t.Errorf("Content-Type = %q, want text/event-stream", got)
		}

		reader := bufio.NewReader(resp.Body)
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("reading initial SSE frame failed: %v", err)
		}
		if strings.TrimSpace(line) != ": ok" {
			t.Errorf("initial SSE line = %q, want %q", strings.TrimSpace(line), ": ok")
		}
	})

	t.Run("connect secret URL completes handshake and tools/list without Authorization header", func(t *testing.T) {
		secretEndpoint := ts.URL + "/mcp/" + testMCPSecret
		sessionID := doInitializeAndGetSession(t, client, secretEndpoint, "")

		toolsPayload := `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`
		req, err := http.NewRequest(http.MethodPost, secretEndpoint, bytes.NewBufferString(toolsPayload))
		if err != nil {
			t.Fatalf("NewRequest tools/list: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		req.Header.Set("Mcp-Session-Id", sessionID)

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("POST tools/list on secret URL failed: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("reading tools/list body: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("tools/list status = %d, want 200", resp.StatusCode)
		}
		if !strings.Contains(string(body), "get_system_info") {
			t.Errorf("tools/list response did not contain get_system_info: %s", string(body))
		}
	})

	t.Run("trailing slash /mcp/ route works with bearer token", func(t *testing.T) {
		sessionID := doInitializeAndGetSession(t, client, ts.URL+"/mcp/", testAPIToken)
		if sessionID == "" {
			t.Fatal("expected non-empty sessionID on /mcp/")
		}
	})

	errorTests := []struct {
		name       string
		path       string
		authHeader string
		accept     string
		sessionID  string
		wantStatus int
	}{
		{
			name:       "missing token on /mcp returns 401",
			path:       "/mcp",
			accept:     "text/event-stream",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "GET with session but missing SSE Accept returns 400",
			path:       "/mcp",
			authHeader: "Bearer " + testAPIToken,
			accept:     "application/json",
			sessionID:  "some-session-id",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "GET with unknown session ID returns 404",
			path:       "/mcp",
			authHeader: "Bearer " + testAPIToken,
			accept:     "text/event-stream",
			sessionID:  "unknown-session-id-xyz",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "unmatched /mcpx returns 404",
			path:       "/mcpx",
			authHeader: "Bearer " + testAPIToken,
			accept:     "text/event-stream",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "wrong connect secret /mcp/<wrong> returns 404",
			path:       "/mcp/wrong_secret_value_12345678901234567890",
			accept:     "text/event-stream",
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tc := range errorTests {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, ts.URL+tc.path, nil)
			if err != nil {
				t.Fatalf("NewRequest: %v", err)
			}
			if tc.authHeader != "" {
				req.Header.Set("Authorization", tc.authHeader)
			}
			if tc.accept != "" {
				req.Header.Set("Accept", tc.accept)
			}
			if tc.sessionID != "" {
				req.Header.Set("Mcp-Session-Id", tc.sessionID)
			}

			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("GET %s failed or timed out: %v", tc.path, err)
			}
			defer func() { _ = resp.Body.Close() }()
			if _, err := io.ReadAll(resp.Body); err != nil {
				t.Fatalf("reading error body to EOF failed: %v", err)
			}
			if resp.StatusCode != tc.wantStatus {
				t.Errorf("GET %s status = %d, want %d", tc.path, resp.StatusCode, tc.wantStatus)
			}
		})
	}
}

func TestMCPHTTPDiscoveryVersions(t *testing.T) {
	const token = "test-api-token"
	ts, _ := newMountedMCPTestServer(t, token, "")
	for _, tc := range []struct {
		path           string
		supportsModern bool
	}{
		{"/mcp", false},
		{"/mcp-stateless", true},
	} {
		t.Run(tc.path, func(t *testing.T) {
			resp := doMCPHTTPRequest(t, ts.Client(), http.MethodPost, ts.URL+tc.path, token, "server/discover",
				`{"jsonrpc":"2.0","id":1,"method":"server/discover","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}}}`)
			if resp.StatusCode != http.StatusOK || resp.Header.Get("Mcp-Session-Id") != "" {
				t.Fatalf("discovery status = %d, session header present = %t", resp.StatusCode, resp.Header.Get("Mcp-Session-Id") != "")
			}
			var result struct {
				Result mcp.DiscoverResult
			}
			decodeMCPEvent(t, resp, &result)
			versions := result.Result.SupportedVersions
			if slices.Contains(versions, "2026-07-28") != tc.supportsModern || !slices.Contains(versions, "2025-11-25") {
				t.Errorf("discovery advertised %v, want legacy support and modern support = %t", versions, tc.supportsModern)
			}
		})
	}
}

func TestMCPStatelessHTTPRoutes(t *testing.T) {
	const token = "test-api-token"
	ts, _ := newMountedMCPTestServer(t, token, strings.Repeat("s", 32))
	const initialize = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"transport-test","version":"1"}}}`
	for _, tc := range []struct {
		name, method, path, bearer string
		status                     int
	}{
		{"missing token", http.MethodPost, "/mcp-stateless", "", http.StatusUnauthorized},
		{"wrong token", http.MethodPost, "/mcp-stateless", "wrong", http.StatusUnauthorized},
		{"lookalike", http.MethodPost, "/mcp-statelessx", token, http.StatusNotFound},
		{"subpath", http.MethodPost, "/mcp-stateless/" + strings.Repeat("s", 32), token, http.StatusNotFound},
		{"get", http.MethodGet, "/mcp-stateless", token, http.StatusMethodNotAllowed},
		{"delete", http.MethodDelete, "/mcp-stateless", token, http.StatusMethodNotAllowed},
		{"legacy initialize", http.MethodPost, "/mcp-stateless", token, http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			payload := ""
			if tc.method == http.MethodPost {
				payload = initialize
			}
			resp := doMCPHTTPRequest(t, ts.Client(), tc.method, ts.URL+tc.path, tc.bearer, "", payload)
			if resp.StatusCode != tc.status {
				t.Fatalf("status = %d, want %d", resp.StatusCode, tc.status)
			}
			if tc.status == http.StatusMethodNotAllowed && resp.Header.Get("Allow") != "POST" {
				t.Errorf("Allow = %q, want POST", resp.Header.Get("Allow"))
			}
			if tc.status == http.StatusOK {
				var response struct {
					Result mcp.InitializeResult
				}
				decodeMCPEvent(t, resp, &response)
				if response.Result.ProtocolVersion != "2025-06-18" || resp.Header.Get("Mcp-Session-Id") != "" {
					t.Errorf("legacy initialize returned protocol %q, session header present = %t",
						response.Result.ProtocolVersion, resp.Header.Get("Mcp-Session-Id") != "")
				}
			}
		})
	}
}

func doMCPHTTPRequest(t *testing.T, client *http.Client, method, endpoint, token, mcpMethod, body string) *http.Response {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	req, err := http.NewRequestWithContext(ctx, method, endpoint, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if mcpMethod != "" {
		req.Header.Set("Mcp-Method", mcpMethod)
		req.Header.Set("Mcp-Protocol-Version", "2026-07-28")
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func decodeMCPEvent(t *testing.T, resp *http.Response, result any) {
	t.Helper()
	reader := bufio.NewScanner(resp.Body)
	for reader.Scan() {
		if data, ok := strings.CutPrefix(reader.Text(), "data: "); ok {
			if err := json.Unmarshal([]byte(data), result); err != nil {
				t.Fatalf("decode MCP event: %v", err)
			}
			return
		}
	}
	t.Fatalf("no MCP event in response (read error: %v)", reader.Err())
}

type mcpBearerTransport struct {
	next  http.RoundTripper
	token string
}

func (transport mcpBearerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.Header.Set("Authorization", "Bearer "+transport.token)
	return transport.next.RoundTrip(req)
}

func connectMCPHTTP(t *testing.T, ts *httptest.Server, path, token, version string) *mcp.ClientSession {
	t.Helper()
	client := *ts.Client()
	client.Timeout = 5 * time.Second
	if token != "" {
		client.Transport = mcpBearerTransport{next: client.Transport, token: token}
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "transport-test", Version: "1"}, nil).Connect(
		context.Background(), &mcp.StreamableClientTransport{Endpoint: ts.URL + path, HTTPClient: &client},
		&mcp.ClientSessionOptions{ProtocolVersion: version},
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	if got := cs.InitializeResult().ProtocolVersion; got != version {
		t.Fatalf("negotiated %q, want %q", got, version)
	}
	return cs
}

func TestMCPStatelessToolPolicy(t *testing.T) {
	const token = "test-api-token"
	for _, tc := range []struct {
		name, token, blockedMessage string
		readOnly                    bool
	}{
		{"network write lock", "", writesLockedBlockedMessage, false},
		{"MCP read-only mode", token, readOnlyBlockedMessage, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			policies := domain.NewToolPolicyStore(t.TempDir(), map[string]domain.ToolPolicyValue{
				"get_array_status": domain.PolicyHidden,
			})
			ts, _ := mountMCPTestServer(t, domain.Config{APIToken: tc.token, ReadOnly: tc.readOnly}, policies)
			cs := connectMCPHTTP(t, ts, "/mcp-stateless", tc.token, "2026-07-28")
			ctx := context.Background()
			list, err := cs.ListTools(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			if slices.ContainsFunc(list.Tools, func(tool *mcp.Tool) bool { return tool.Name == "get_array_status" }) {
				t.Error("hidden tool appeared in tools/list")
			}
			if _, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "get_array_status"}); err == nil || !strings.Contains(err.Error(), "unknown tool") {
				t.Errorf("hidden tool call returned %v, want unknown tool", err)
			}
			if _, got := callToolJSON(t, cs, "container_action", map[string]any{"container_id": "", "action": "stop"}); got != tc.blockedMessage {
				t.Errorf("write tool returned %q, want %q", got, tc.blockedMessage)
			}
			if result, got := callToolJSON(t, cs, "get_system_info", nil); result.IsError || !strings.Contains(got, "test-unraid") {
				t.Errorf("read tool returned %q, want system info", got)
			}
		})
	}
}

func TestMCPHTTPTransportsShareLivePolicy(t *testing.T) {
	policies := domain.NewToolPolicyStore(t.TempDir(), nil)
	ts, _ := mountMCPTestServer(t, domain.Config{}, policies)
	stateful := connectMCPHTTP(t, ts, "/mcp", "", "2025-11-25")
	stateless := connectMCPHTTP(t, ts, "/mcp-stateless", "", "2026-07-28")
	const toolName = "get_system_info"
	checkList := func(t *testing.T, cs *mcp.ClientSession, wantVisible bool) {
		t.Helper()
		list, err := cs.ListTools(context.Background(), nil)
		if err != nil {
			t.Fatal(err)
		}
		if got := slices.ContainsFunc(list.Tools, func(tool *mcp.Tool) bool { return tool.Name == toolName }); got != wantVisible {
			t.Errorf("tool %s visible = %t, want %t", toolName, got, wantVisible)
		}
	}

	checkList(t, stateful, true)
	checkList(t, stateless, true)
	t.Run("concurrent traffic and policy updates", func(t *testing.T) {
		for _, cs := range []*mcp.ClientSession{stateful, stateless} {
			t.Run(cs.InitializeResult().ProtocolVersion, func(t *testing.T) {
				t.Parallel()
				for range 5 {
					if _, err := cs.ListTools(context.Background(), nil); err != nil {
						t.Error(err)
					}
				}
			})
		}
		t.Run("update policy", func(t *testing.T) {
			t.Parallel()
			for range 5 {
				policies.Replace(nil)
				policies.Replace(map[string]domain.ToolPolicyValue{toolName: domain.PolicyHidden})
			}
		})
	})
	checkList(t, stateful, false)
	checkList(t, stateless, false)
}
