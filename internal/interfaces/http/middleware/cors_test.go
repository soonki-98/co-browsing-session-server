package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// newCORSEngine assembles a gin engine with CORSMiddleware applied and a dummy
// GET handler that records whether the next handler ran and answers 200.
func newCORSEngine(allowedOrigins []string, nextRan *bool) *gin.Engine {
	engine := gin.New()
	engine.Use(CORSMiddleware(allowedOrigins))
	engine.GET("/serial_number", func(c *gin.Context) {
		if nextRan != nil {
			*nextRan = true
		}
		c.String(http.StatusOK, "ok")
	})
	return engine
}

// TestCORSMiddleware verifies the observable response headers/status the
// middleware produces per origin. Pure middleware cases run in parallel.
func TestCORSMiddleware(t *testing.T) {
	t.Parallel()

	const allowed = "http://localhost:3000"

	// AC-1 / FR-1·FR-2: allowed origin is reflected.
	t.Run("allowed origin is reflected into Access-Control-Allow-Origin", func(t *testing.T) {
		t.Parallel()

		var nextRan bool
		engine := newCORSEngine([]string{allowed}, &nextRan)

		req := httptest.NewRequest(http.MethodGet, "/serial_number", nil)
		req.Header.Set("Origin", allowed)
		rec := httptest.NewRecorder()
		engine.ServeHTTP(rec, req)

		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != allowed {
			t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, allowed)
		}
		if !nextRan {
			t.Error("expected next handler to run for a normal (non-preflight) request")
		}
		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
		}
	})

	// AC-6 / rule: reflected value is the exact origin, never the wildcard,
	// and credentials are always allowed for an allowed origin.
	t.Run("allowed origin gets exact reflection not wildcard and credentials true", func(t *testing.T) {
		t.Parallel()

		engine := newCORSEngine([]string{allowed}, nil)

		req := httptest.NewRequest(http.MethodGet, "/serial_number", nil)
		req.Header.Set("Origin", allowed)
		rec := httptest.NewRecorder()
		engine.ServeHTTP(rec, req)

		origin := rec.Header().Get("Access-Control-Allow-Origin")
		if origin == "*" {
			t.Error("Access-Control-Allow-Origin must not be the wildcard '*'")
		}
		if origin != allowed {
			t.Errorf("Access-Control-Allow-Origin = %q, want %q", origin, allowed)
		}
		if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
			t.Errorf("Access-Control-Allow-Credentials = %q, want %q", got, "true")
		}
	})

	// AC-2 / FR-3: disallowed origin gets no trust marker but still passes through.
	t.Run("disallowed origin has no CORS headers and request passes through", func(t *testing.T) {
		t.Parallel()

		var nextRan bool
		engine := newCORSEngine([]string{allowed}, &nextRan)

		req := httptest.NewRequest(http.MethodGet, "/serial_number", nil)
		req.Header.Set("Origin", "http://evil.example.com")
		rec := httptest.NewRecorder()
		engine.ServeHTTP(rec, req)

		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
			t.Errorf("Access-Control-Allow-Origin = %q, want empty (no trust marker)", got)
		}
		if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "" {
			t.Errorf("Access-Control-Allow-Credentials = %q, want empty", got)
		}
		if !nextRan {
			t.Error("expected request to pass through to next handler")
		}
		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
		}
	})

	// Rule: a request without an Origin header is not subject to the policy.
	t.Run("missing Origin header has no CORS headers and passes through", func(t *testing.T) {
		t.Parallel()

		var nextRan bool
		engine := newCORSEngine([]string{allowed}, &nextRan)

		req := httptest.NewRequest(http.MethodGet, "/serial_number", nil)
		rec := httptest.NewRecorder()
		engine.ServeHTTP(rec, req)

		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
			t.Errorf("Access-Control-Allow-Origin = %q, want empty", got)
		}
		if !nextRan {
			t.Error("expected request to pass through to next handler")
		}
		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
		}
	})

	// AC-4 / FR-6: each origin in a multi-origin allow-list is reflected.
	t.Run("each origin in a multi-origin allow-list is reflected", func(t *testing.T) {
		t.Parallel()

		origins := []string{"https://a.example.com", "https://b.example.com"}

		for _, origin := range origins {
			origin := origin
			t.Run(origin, func(t *testing.T) {
				t.Parallel()

				engine := newCORSEngine(origins, nil)

				req := httptest.NewRequest(http.MethodGet, "/serial_number", nil)
				req.Header.Set("Origin", origin)
				rec := httptest.NewRecorder()
				engine.ServeHTTP(rec, req)

				if got := rec.Header().Get("Access-Control-Allow-Origin"); got != origin {
					t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, origin)
				}
			})
		}
	})
}

// TestCORSMiddlewarePreflight verifies the OPTIONS preflight short-circuits
// with 204 and never reaches the main handler. AC-3 / FR-4.
func TestCORSMiddlewarePreflight(t *testing.T) {
	t.Parallel()

	const allowed = "http://localhost:3000"

	var nextRan bool
	engine := newCORSEngine([]string{allowed}, &nextRan)

	req := httptest.NewRequest(http.MethodOptions, "/serial_number", nil)
	req.Header.Set("Origin", allowed)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want %d (204 No Content)", rec.Code, http.StatusNoContent)
	}
	if nextRan {
		t.Error("preflight OPTIONS must not reach the main handler")
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != allowed {
		t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, allowed)
	}
}

// TestLoadAllowedOrigins exercises the env-driven origin loading. These cases
// mutate the process environment via t.Setenv, so they run serially.
func TestLoadAllowedOrigins(t *testing.T) {
	t.Run("unset env returns local development default", func(t *testing.T) {
		t.Setenv("CORS_ALLOWED_ORIGINS", "")

		got := LoadAllowedOrigins()
		want := []string{"http://localhost:3000"}
		if !equalStrings(got, want) {
			t.Errorf("LoadAllowedOrigins() = %v, want %v", got, want)
		}
	})

	t.Run("comma-separated env is split into each origin", func(t *testing.T) {
		t.Setenv("CORS_ALLOWED_ORIGINS", "http://a.com,http://b.com")

		got := LoadAllowedOrigins()
		want := []string{"http://a.com", "http://b.com"}
		if !equalStrings(got, want) {
			t.Errorf("LoadAllowedOrigins() = %v, want %v", got, want)
		}
	})
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
