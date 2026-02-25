package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
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
		if got := rr.Header().Get("Access-Control-Allow-Methods"); got != "GET, POST, PUT, DELETE, OPTIONS" {
			t.Errorf("Access-Control-Allow-Methods = %q, want %q", got, "GET, POST, PUT, DELETE, OPTIONS")
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

	t.Run("empty origin defaults to wildcard", func(t *testing.T) {
		h := corsMiddleware("")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		req := httptest.NewRequest("GET", "/test", nil)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "*" {
			t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, "*")
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
