package api

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"golang.org/x/time/rate"

	"github.com/ruaan-deysel/unraid-management-agent/daemon/logger"
)

func corsMiddleware(allowedOrigin string) mux.MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Only set CORS headers when explicitly configured.
			// When no origin is set, cross-origin browser requests are blocked
			// by default (same-origin policy). Non-browser clients are unaffected.
			if allowedOrigin != "" {
				w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			}

			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusOK)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// statusRecorder wraps http.ResponseWriter to capture the response status code.
// It preserves the http.Hijacker interface so that WebSocket upgrades still work.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (sr *statusRecorder) WriteHeader(code int) {
	sr.status = code
	sr.ResponseWriter.WriteHeader(code)
}

// Hijack delegates to the underlying ResponseWriter's Hijack method
// so that WebSocket upgrades (which require http.Hijacker) continue to work.
func (sr *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hj, ok := sr.ResponseWriter.(http.Hijacker); ok {
		return hj.Hijack()
	}
	return nil, nil, fmt.Errorf("underlying ResponseWriter does not implement http.Hijacker")
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		logger.Debug("%s %s %d %v", r.Method, r.URL.Path, rec.status, time.Since(start))
	})
}

func recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				logger.LogPanicWithStack("HTTP handler", err)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// securityHeadersMiddleware adds standard security headers to every response.
func securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Permissions-Policy", "geolocation=(), camera=(), microphone=()")

		// Swagger UI requires inline scripts/styles; use a relaxed CSP for it.
		csp := "default-src 'self'"
		if r.URL != nil && (r.URL.Path == "/swagger" || strings.HasPrefix(r.URL.Path, "/swagger/")) {
			csp = "default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; script-src 'self' 'unsafe-inline'"
		}
		w.Header().Set("Content-Security-Policy", csp)

		next.ServeHTTP(w, r)
	})
}

// bodySizeLimitMiddleware limits the maximum request body size to prevent
// memory exhaustion attacks (CWE-770).
const maxRequestBodySize int64 = 1 * 1024 * 1024 // 1 MB

func bodySizeLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)
		}
		next.ServeHTTP(w, r)
	})
}

// rateLimitMiddleware applies a global rate limit to incoming requests to
// mitigate denial-of-service and brute-force attacks.
func rateLimitMiddleware(limiter *rate.Limiter) mux.MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !limiter.Allow() {
				http.Error(w, http.StatusText(http.StatusTooManyRequests), http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// csrfMiddleware validates the Origin header on state-changing requests to
// prevent cross-site request forgery. Non-browser clients that don't send
// an Origin header are allowed through.
func csrfMiddleware(allowedOrigin string) mux.MiddlewareFunc {
	// Normalize empty origin to wildcard, consistent with corsMiddleware.
	if allowedOrigin == "" {
		allowedOrigin = "*"
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Only check state-changing methods
			if r.Method == "GET" || r.Method == "HEAD" || r.Method == "OPTIONS" {
				next.ServeHTTP(w, r)
				return
			}

			origin := r.Header.Get("Origin")
			if origin == "" {
				// Non-browser clients (curl, API tools) don't send Origin
				next.ServeHTTP(w, r)
				return
			}

			// Parse and validate origin
			parsed, err := url.Parse(origin)
			if err != nil {
				respondJSON(w, http.StatusForbidden, map[string]string{"error": "Forbidden: invalid origin"})
				return
			}

			if allowedOrigin != "*" {
				// Strict mode: origin must match configured value
				if origin == allowedOrigin {
					next.ServeHTTP(w, r)
					return
				}
				respondJSON(w, http.StatusForbidden, map[string]string{"error": "Forbidden: origin not allowed"})
				return
			}

			// Wildcard mode: verify origin matches request origin (scheme + host + port)
			// to prevent drive-by CSRF from external websites.
			originScheme := parsed.Scheme
			originHostPort := parsed.Host // includes port if present

			requestScheme := "http"
			if r.TLS != nil {
				requestScheme = "https"
			}
			requestHostPort := r.Host

			// Exact origin match (scheme, host, and port)
			if originScheme == requestScheme && originHostPort == requestHostPort {
				next.ServeHTTP(w, r)
				return
			}

			// Allow localhost aliases to match each other only if scheme and port also match
			originHost := parsed.Hostname()
			requestHost := stripHostPort(r.Host)
			if isLocalhost(originHost) && isLocalhost(requestHost) {
				// Both are localhost variants, now verify scheme and port match
				originPort := parsed.Port()
				if originPort == "" {
					if originScheme == "https" {
						originPort = "443"
					} else {
						originPort = "80"
					}
				}

				requestPort := ""
				if _, port, err := net.SplitHostPort(requestHostPort); err == nil {
					requestPort = port
				} else {
					if requestScheme == "https" {
						requestPort = "443"
					} else {
						requestPort = "80"
					}
				}

				if originScheme == requestScheme && originPort == requestPort {
					next.ServeHTTP(w, r)
					return
				}
			}

			respondJSON(w, http.StatusForbidden, map[string]string{"error": "Forbidden: origin not allowed"})
		})
	}
}

// stripHostPort removes the port portion from a host:port string.
// It also normalizes bracketed IPv6 literals without an explicit port so the
// result matches url.URL.Hostname() behavior (e.g. "[::1]" → "::1").
func stripHostPort(hostport string) string {
	host, _, err := net.SplitHostPort(hostport)
	if err == nil {
		return host
	}

	// Handle bracketed IPv6 without port, e.g. "[::1]"
	if strings.HasPrefix(hostport, "[") && strings.HasSuffix(hostport, "]") {
		return hostport[1 : len(hostport)-1]
	}

	return hostport // already just a host
}

// isLocalhost returns true if the host is a loopback address or "localhost".
func isLocalhost(host string) bool {
	switch strings.ToLower(host) {
	case "localhost", "127.0.0.1", "::1":
		return true
	}
	return false
}
