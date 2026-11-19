package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/auth"

	"github.com/tinkerologie/unraid-management-agent/daemon/domain"
)

func TestMCPRoutePassesAuthenticationProofWithoutRetainingTheCredential(t *testing.T) {
	const bearer = "fake-mcp-key"
	for _, test := range []struct {
		name, configured, supplied string
		verified                   bool
		wantStatus                 int
	}{
		{name: "verified", configured: bearer, supplied: "Bearer " + bearer, verified: true, wantStatus: http.StatusNoContent},
		{name: "wrong bearer", configured: bearer, supplied: "Bearer wrong-key", wantStatus: http.StatusUnauthorized},
		{name: "unconfigured and spoofed", supplied: "Bearer verified", wantStatus: http.StatusNoContent},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := NewServer(&domain.Context{Config: domain.Config{APIToken: test.configured}})
			t.Cleanup(server.Stop)
			called := false
			server.RegisterMCPStatelessRoute(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				proof := auth.TokenInfoFromContext(r.Context())
				if HasMCPRequestCredential(proof) != test.verified {
					t.Error("the MCP request authentication proof did not reflect verified transport credentials")
				}
				if test.verified && r.Header.Get("Authorization") == test.supplied {
					t.Error("MCP received the original bearer secret")
				}
				w.WriteHeader(http.StatusNoContent)
			}))
			request := httptest.NewRequest(http.MethodPost, "/mcp-stateless", nil)
			request.Header.Set("Authorization", test.supplied)
			request.Header.Set("X-MCP-Verified", "true")
			response := httptest.NewRecorder()
			server.GetRouter().ServeHTTP(response, request)
			if response.Code != test.wantStatus || called != (test.wantStatus == http.StatusNoContent) {
				t.Errorf("got status %d and handler called %t", response.Code, called)
			}
		})
	}
	if HasMCPRequestCredential(&auth.TokenInfo{Extra: map[string]any{"verified": true}}) {
		t.Error("manually constructed metadata must not substitute for verified HTTP credentials")
	}
}
