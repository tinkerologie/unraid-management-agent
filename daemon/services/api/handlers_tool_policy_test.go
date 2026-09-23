package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/domain"
	"github.com/ruaan-deysel/unraid-management-agent/daemon/dto"
)

func setupTestServerWithToolPolicy(t *testing.T) (*Server, *domain.ToolPolicyStore) {
	t.Helper()
	server, _ := setupTestServer()
	store := domain.NewToolPolicyStore(t.TempDir(), map[string]domain.ToolPolicyValue{
		"system_reboot": domain.PolicyAsk,
	})
	store.RegisterTool("system_reboot", "Reboot system", false, true)
	store.RegisterTool("get_system_info", "Get system info", true, false)
	store.RegisterTool("container_action", "Control container", false, false)
	server.SetToolPolicyStore(store)
	return server, store
}

func TestHandleGetMCPToolPolicy_NilStore(t *testing.T) {
	server, _ := setupTestServer()

	req := httptest.NewRequest("GET", "/api/v1/mcp/tool-policy", nil)
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}

	var resp dto.MCPToolPolicyResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(resp.Tools) != 0 {
		t.Errorf("expected empty tools with nil store, got %d", len(resp.Tools))
	}
}

func TestHandleGetMCPToolPolicy_WithStore(t *testing.T) {
	server, _ := setupTestServerWithToolPolicy(t)

	req := httptest.NewRequest("GET", "/api/v1/mcp/tool-policy", nil)
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}

	var resp dto.MCPToolPolicyResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(resp.Tools) != 3 {
		t.Errorf("expected 3 tools in catalog, got %d", len(resp.Tools))
	}
	if resp.Policies["system_reboot"] != "ask" {
		t.Errorf("expected system_reboot policy 'ask', got %q", resp.Policies["system_reboot"])
	}
}

func TestHandleUpdateMCPToolPolicy_Success(t *testing.T) {
	server, store := setupTestServerWithToolPolicy(t)

	body := dto.MCPToolPolicyUpdateRequest{
		Policies: map[string]string{
			"container_action": "read_only",
			"get_system_info":  "default",
		},
	}
	jsonBody, _ := json.Marshal(body)

	req := httptest.NewRequest("PUT", "/api/v1/mcp/tool-policy", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp dto.Response
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !resp.Success {
		t.Errorf("expected success=true")
	}

	if pol := store.GetPolicy("container_action"); pol != domain.PolicyReadOnly {
		t.Errorf("expected policy for container_action to be read_only, got %q", pol)
	}
	// "default" should be omitted from stored map
	if pol := store.GetPolicy("get_system_info"); pol != domain.PolicyDefault {
		t.Errorf("expected policy for get_system_info to be default, got %q", pol)
	}
}

func TestHandleUpdateMCPToolPolicy_RawMapSuccess(t *testing.T) {
	server, store := setupTestServerWithToolPolicy(t)

	rawMap := map[string]string{
		"system_reboot": "allow",
	}
	jsonBody, _ := json.Marshal(rawMap)

	req := httptest.NewRequest("PUT", "/api/v1/mcp/tool-policy", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	if pol := store.GetPolicy("system_reboot"); pol != domain.PolicyAllow {
		t.Errorf("expected policy for system_reboot to be allow, got %q", pol)
	}
}

func TestHandleUpdateMCPToolPolicy_EmptyPoliciesReset(t *testing.T) {
	server, store := setupTestServerWithToolPolicy(t)

	// Previously system_reboot was ask
	if pol := store.GetPolicy("system_reboot"); pol != domain.PolicyAsk {
		t.Fatalf("expected initial policy to be ask, got %v", pol)
	}

	body := `{"policies":{}}`
	req := httptest.NewRequest("PUT", "/api/v1/mcp/tool-policy", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// Should now be reset to default
	if pol := store.GetPolicy("system_reboot"); pol != domain.PolicyDefault {
		t.Errorf("expected system_reboot to be reset to default, got %v", pol)
	}
}

func TestHandleUpdateMCPToolPolicy_ValidationErrors(t *testing.T) {
	server, _ := setupTestServerWithToolPolicy(t)

	testCases := []struct {
		name       string
		body       string
		wantStatus int
	}{
		{
			name:       "invalid JSON",
			body:       "{invalid json",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "missing policies field",
			body:       "{}",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "unknown tool",
			body:       `{"policies":{"unknown_nonexistent_tool":"read_only"}}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "path traversal tool name",
			body:       `{"policies":{"../../etc/passwd":"read_only"}}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "shell metacharacter tool name",
			body:       `{"policies":{"system_reboot;rm -rf /":"read_only"}}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid policy value",
			body:       `{"policies":{"container_action":"invalid_policy"}}`,
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("PUT", "/api/v1/mcp/tool-policy", bytes.NewReader([]byte(tc.body)))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()
			server.router.ServeHTTP(rr, req)

			if rr.Code != tc.wantStatus {
				t.Errorf("expected status %d, got %d: %s", tc.wantStatus, rr.Code, rr.Body.String())
			}
		})
	}
}

func TestHandleUpdateMCPToolPolicy_SaveFailureReturns500(t *testing.T) {
	// Setup server with a tool policy store pointing to a read-only path that cannot be written
	server, _ := setupTestServer()
	// /dev/null/cannot_create_dir is an unwritable path
	unwritableStore := domain.NewToolPolicyStore("/dev/null/impossible", nil)
	unwritableStore.RegisterTool("container_action", "Docker action", false, true)
	server.SetToolPolicyStore(unwritableStore)

	body := `{"policies":{"container_action":"read_only"}}`
	req := httptest.NewRequest("PUT", "/api/v1/mcp/tool-policy", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500 when save fails, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestHandleUpdateMCPToolPolicy_NilStore(t *testing.T) {
	server, _ := setupTestServer() // nil toolPolicyStore

	body := `{"policies":{"system_reboot":"allow"}}`
	req := httptest.NewRequest("PUT", "/api/v1/mcp/tool-policy", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	server.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", rr.Code)
	}
}
