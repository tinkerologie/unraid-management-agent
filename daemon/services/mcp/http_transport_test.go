package mcp

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/services/api"
)

func newMountedMCPTestServer(t *testing.T, apiToken, mcpSecret string) (*httptest.Server, *api.Server) {
	t.Helper()

	ctx := &domain.Context{
		Config: domain.Config{
			Version:          "test-1.0.0",
			Port:             8043,
			APIToken:         apiToken,
			MCPConnectSecret: mcpSecret,
		},
		Hub: domain.NewEventBus(64),
	}

	apiServer := api.NewServer(ctx)
	mcpServer := NewServer(ctx, newMockCacheProvider())
	if err := mcpServer.Initialize(); err != nil {
		t.Fatalf("Initialize() failed: %v", err)
	}
	apiServer.RegisterMCPRoutes(mcpServer.GetHTTPHandler())

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
