package api

import (
	"context"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/auth"
)

// verifiedCredential is set by authMiddlewareWithMCPSecret only after a
// configured bearer or connect secret passes constant-time comparison.
type verifiedCredential struct{}

func markVerifiedCredential(r *http.Request) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), verifiedCredential{}, true))
}

func hasVerifiedCredential(ctx context.Context) bool {
	verified, _ := ctx.Value(verifiedCredential{}).(bool)
	return verified
}

func verifyMCPRequestCredential(ctx context.Context, _ string, _ *http.Request) (*auth.TokenInfo, error) {
	if !hasVerifiedCredential(ctx) {
		return nil, auth.ErrInvalidToken
	}
	return &auth.TokenInfo{Extra: map[string]any{"verified": verifiedCredential{}}}, nil
}

// withMCPRequestCredential bridges our HTTP middleware into the SDK's
// per-message TokenInfo. A session may outlive its initializing request, so
// the MCP tools must read CallToolRequest.Extra, not the session context.
func withMCPRequestCredential(next http.Handler) http.Handler {
	verified := auth.RequireBearerToken(verifyMCPRequestCredential,
		&auth.RequireBearerTokenOptions{AllowMissingExpiration: true},
	)(next)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !hasVerifiedCredential(r.Context()) {
			next.ServeHTTP(w, r)
			return
		}
		// The SDK expects a Bearer header even when the actual accepted
		// credential came from the connect-secret path. Pass a placeholder
		// instead of retaining the configured credential in SDK metadata.
		request := r.Clone(r.Context())
		request.Header.Set("Authorization", "Bearer verified")
		verified.ServeHTTP(w, request)
	})
}

// HasMCPRequestCredential checks the API-issued transport proof attached to an
// individual MCP message. Client-controlled headers and JSON cannot supply it.
func HasMCPRequestCredential(token *auth.TokenInfo) bool {
	if token == nil {
		return false
	}
	_, verified := token.Extra["verified"].(verifiedCredential)
	return verified
}
