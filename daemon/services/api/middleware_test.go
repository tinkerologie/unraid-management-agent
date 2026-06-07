package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

func TestCorsMiddleware(t *testing.T) {
	handler := corsMiddleware("*")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	t.Run("sets CORS headers", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "*" {
			t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, "*")
		}
		if got := rr.Header().Get("Access-Control-Allow-Methods"); got != "GET, POST, PUT, PATCH, DELETE, OPTIONS" {
			t.Errorf("Access-Control-Allow-Methods = %q, want %q", got, "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		}
		if got := rr.Header().Get("Access-Control-Allow-Headers"); got != "Content-Type, Authorization" {
			t.Errorf("Access-Control-Allow-Headers = %q, want %q", got, "Content-Type, Authorization")
		}
	})

	t.Run("handles OPTIONS preflight", func(t *testing.T) {
		req := httptest.NewRequest("OPTIONS", "/test", nil)
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("OPTIONS request returned status %d, want %d", rr.Code, http.StatusOK)
		}
	})

	t.Run("passes through GET request", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("GET request returned status %d, want %d", rr.Code, http.StatusOK)
		}
	})

	t.Run("passes through POST request", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/test", nil)
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("POST request returned status %d, want %d", rr.Code, http.StatusOK)
		}
	})

	t.Run("custom origin", func(t *testing.T) {
		h := corsMiddleware("https://example.com")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		req := httptest.NewRequest("GET", "/test", nil)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "https://example.com" {
			t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, "https://example.com")
		}
	})

	t.Run("empty origin omits CORS headers", func(t *testing.T) {
		h := corsMiddleware("")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		req := httptest.NewRequest("GET", "/test", nil)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "" {
			t.Errorf("Access-Control-Allow-Origin = %q, want empty (no CORS headers when origin unconfigured)", got)
		}
	})
}

func TestLoggingMiddleware(t *testing.T) {
	called := false
	handler := loggingMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	t.Run("passes request to next handler", func(t *testing.T) {
		called = false
		req := httptest.NewRequest("GET", "/test", nil)
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if !called {
			t.Error("Next handler was not called")
		}
		if rr.Code != http.StatusOK {
			t.Errorf("Status code = %d, want %d", rr.Code, http.StatusOK)
		}
	})

	t.Run("handles different HTTP methods", func(t *testing.T) {
		methods := []string{"GET", "POST", "PUT", "DELETE", "PATCH"}
		for _, method := range methods {
			called = false
			req := httptest.NewRequest(method, "/test", nil)
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			if !called {
				t.Errorf("Handler not called for %s method", method)
			}
		}
	})
}

func TestRecoveryMiddleware(t *testing.T) {
	t.Run("recovers from panic", func(t *testing.T) {
		handler := recoveryMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			panic("test panic")
		}))

		req := httptest.NewRequest("GET", "/test", nil)
		rr := httptest.NewRecorder()

		// Should not panic
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusInternalServerError {
			t.Errorf("Status code = %d, want %d", rr.Code, http.StatusInternalServerError)
		}
	})

	t.Run("passes through without panic", func(t *testing.T) {
		handler := recoveryMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest("GET", "/test", nil)
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Status code = %d, want %d", rr.Code, http.StatusOK)
		}
	})

	t.Run("recovers from string panic", func(t *testing.T) {
		handler := recoveryMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			panic("string panic message")
		}))

		req := httptest.NewRequest("GET", "/test", nil)
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusInternalServerError {
			t.Errorf("Status code = %d, want %d", rr.Code, http.StatusInternalServerError)
		}
	})

	t.Run("recovers from error panic", func(t *testing.T) {
		handler := recoveryMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			panic("error: something went wrong")
		}))

		req := httptest.NewRequest("GET", "/test", nil)
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusInternalServerError {
			t.Errorf("Status code = %d, want %d", rr.Code, http.StatusInternalServerError)
		}
	})
}

func TestMiddlewareChain(t *testing.T) {
	// Test that middlewares can be chained
	finalHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	})

	handler := corsMiddleware("*")(loggingMiddleware(recoveryMiddleware(finalHandler)))

	req := httptest.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Chained middleware returned status %d, want %d", rr.Code, http.StatusOK)
	}

	// Check CORS headers are set
	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, "*")
	}
}

func TestRateLimitMiddleware(t *testing.T) {
	okHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// A near-zero refill rate isolates the burst capacity from time-based
	// replenishment for a deterministic test.
	const negligibleRefill = rate.Limit(0.001)

	requestFrom := func(handler http.Handler, addr string) int {
		req := httptest.NewRequest("GET", "/api/v1/unassigned", nil)
		req.RemoteAddr = addr
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		return rr.Code
	}

	t.Run("allows requests within burst", func(t *testing.T) {
		const burst = 5
		handler := rateLimitMiddleware(newPerClientRateLimiter(negligibleRefill, burst))(okHandler)

		for i := 0; i < burst; i++ {
			if code := requestFrom(handler, "192.168.0.10:5000"); code != http.StatusOK {
				t.Fatalf("request %d within burst returned %d, want %d", i+1, code, http.StatusOK)
			}
		}
	})

	t.Run("rejects requests beyond burst with 429", func(t *testing.T) {
		const burst = 3
		handler := rateLimitMiddleware(newPerClientRateLimiter(negligibleRefill, burst))(okHandler)

		for i := 0; i < burst; i++ {
			if code := requestFrom(handler, "192.168.0.10:5000"); code != http.StatusOK {
				t.Fatalf("request %d within burst returned %d, want %d", i+1, code, http.StatusOK)
			}
		}

		// The next request from the same client exceeds its burst.
		if code := requestFrom(handler, "192.168.0.10:5000"); code != http.StatusTooManyRequests {
			t.Errorf("request beyond burst returned %d, want %d", code, http.StatusTooManyRequests)
		}
	})

	t.Run("one client's burst does not throttle another", func(t *testing.T) {
		const burst = 2
		handler := rateLimitMiddleware(newPerClientRateLimiter(negligibleRefill, burst))(okHandler)

		// Exhaust client A's bucket entirely.
		for i := 0; i < burst; i++ {
			_ = requestFrom(handler, "192.168.0.10:5000")
		}
		if code := requestFrom(handler, "192.168.0.10:5000"); code != http.StatusTooManyRequests {
			t.Fatalf("client A beyond burst returned %d, want %d", code, http.StatusTooManyRequests)
		}

		// Client B (different IP) must still be served from its own bucket.
		if code := requestFrom(handler, "192.168.0.99:6000"); code != http.StatusOK {
			t.Errorf("client B returned %d, want %d — buckets are not per-client", code, http.StatusOK)
		}
	})

	t.Run("port-less RemoteAddr is handled", func(t *testing.T) {
		handler := rateLimitMiddleware(newPerClientRateLimiter(negligibleRefill, 1))(okHandler)
		if code := requestFrom(handler, "192.168.0.10"); code != http.StatusOK {
			t.Errorf("port-less client returned %d, want %d", code, http.StatusOK)
		}
	})
}

// TestRateLimitDefaultsAbsorbIntegrationBurst guards against regressing the
// limits back to values too small for the Home Assistant integration's
// parallel startup fetch (~20-30 endpoints at once, plus retries). See the
// rationale in middleware.go.
func TestRateLimitDefaultsAbsorbIntegrationBurst(t *testing.T) {
	const minimumParallelEndpoints = 30
	if rateLimitBurst < minimumParallelEndpoints {
		t.Errorf("rateLimitBurst = %d, want >= %d to absorb the integration's parallel fetch",
			rateLimitBurst, minimumParallelEndpoints)
	}
	if rateLimitPerSecond <= 0 {
		t.Errorf("rateLimitPerSecond = %d, want > 0", rateLimitPerSecond)
	}
}

func TestPerClientRateLimiterCleanup(t *testing.T) {
	p := newPerClientRateLimiter(rate.Limit(1), 1)
	base := time.Unix(1_700_000_000, 0)
	current := base
	p.clock = func() time.Time { return current }

	if !p.allow("10.0.0.1") {
		t.Fatal("first request should be allowed")
	}
	if len(p.clients) != 1 {
		t.Fatalf("expected 1 client bucket, got %d", len(p.clients))
	}

	// Advance past the TTL and the cleanup interval; a request from a new client
	// triggers a sweep that evicts the now-idle bucket.
	current = base.Add(rateLimiterClientTTL + rateLimiterCleanupInterval + time.Second)
	if !p.allow("10.0.0.2") {
		t.Fatal("new client request should be allowed")
	}
	if _, ok := p.clients["10.0.0.1"]; ok {
		t.Error("idle client bucket should have been swept")
	}
	if _, ok := p.clients["10.0.0.2"]; !ok {
		t.Error("active client bucket should be present")
	}
}

func TestMiddlewareChainWithPanic(t *testing.T) {
	// Test that recovery works in chain
	panicHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("chain panic")
	})

	handler := corsMiddleware("*")(loggingMiddleware(recoveryMiddleware(panicHandler)))

	req := httptest.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("Status code = %d, want %d", rr.Code, http.StatusInternalServerError)
	}

	// CORS headers should still be set
	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, "*")
	}
}
