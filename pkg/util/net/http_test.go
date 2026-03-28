package net

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestHTTPAuthMiddleware(t *testing.T) {
	t.Parallel()

	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.SetBasicAuth("admin", "secret")
		rec := httptest.NewRecorder()

		NewHTTPAuthMiddleware("admin", "secret").Middleware(next).ServeHTTP(rec, req)
		require.Equal(t, http.StatusNoContent, rec.Code)
	})

	t.Run("failure", func(t *testing.T) {
		t.Parallel()

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.SetBasicAuth("admin", "wrong")
		rec := httptest.NewRecorder()

		mid := NewHTTPAuthMiddleware("admin", "secret").SetAuthFailDelay(10 * time.Millisecond)
		start := time.Now()
		mid.Middleware(next).ServeHTTP(rec, req)

		require.Equal(t, http.StatusUnauthorized, rec.Code)
		require.Equal(t, `Basic realm="Restricted"`, rec.Header().Get("WWW-Authenticate"))
		require.GreaterOrEqual(t, time.Since(start), 10*time.Millisecond)
	})

	t.Run("disabled auth when credentials empty", func(t *testing.T) {
		t.Parallel()

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()

		NewHTTPAuthMiddleware("", "").Middleware(next).ServeHTTP(rec, req)
		require.Equal(t, http.StatusNoContent, rec.Code)
	})

	t.Run("lockout returns html for non api requests", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/static/", nil)
		req.SetBasicAuth("admin", "wrong")

		mid := NewHTTPAuthMiddleware("admin", "secret")
		mid.maxFailures = 2
		mid.failureWindow = time.Minute
		mid.lockDuration = 2 * time.Second

		first := httptest.NewRecorder()
		mid.Middleware(next).ServeHTTP(first, req)
		require.Equal(t, http.StatusUnauthorized, first.Code)

		second := httptest.NewRecorder()
		mid.Middleware(next).ServeHTTP(second, req)
		require.Equal(t, http.StatusTooManyRequests, second.Code)
		require.Contains(t, second.Header().Get("Content-Type"), "text/html")
		require.Equal(t, "2", second.Header().Get("Retry-After"))
		require.Contains(t, second.Body.String(), "Dashboard Temporarily Locked")
		require.Contains(t, second.Body.String(), "location.reload()")
	})

	t.Run("lockout returns json for api requests", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/serverinfo", nil)
		req.SetBasicAuth("admin", "wrong")

		mid := NewHTTPAuthMiddleware("admin", "secret")
		mid.maxFailures = 1
		mid.failureWindow = time.Minute
		mid.lockDuration = 3 * time.Second

		rec := httptest.NewRecorder()
		mid.Middleware(next).ServeHTTP(rec, req)
		require.Equal(t, http.StatusTooManyRequests, rec.Code)
		require.Contains(t, rec.Header().Get("Content-Type"), "application/json")
		require.Equal(t, "3", rec.Header().Get("Retry-After"))

		var payload map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
		require.Equal(t, float64(http.StatusTooManyRequests), payload["code"])
		require.Equal(t, "dashboard auth temporarily locked", payload["msg"])
		require.Equal(t, float64(3), payload["retryAfter"])
	})

	t.Run("lockout expires and auth works again", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.SetBasicAuth("admin", "wrong")

		mid := NewHTTPAuthMiddleware("admin", "secret")
		mid.maxFailures = 1
		mid.failureWindow = time.Minute
		mid.lockDuration = 20 * time.Millisecond

		locked := httptest.NewRecorder()
		mid.Middleware(next).ServeHTTP(locked, req)
		require.Equal(t, http.StatusTooManyRequests, locked.Code)

		time.Sleep(30 * time.Millisecond)

		successReq := httptest.NewRequest(http.MethodGet, "/", nil)
		successReq.SetBasicAuth("admin", "secret")
		success := httptest.NewRecorder()
		mid.Middleware(next).ServeHTTP(success, successReq)
		require.Equal(t, http.StatusNoContent, success.Code)
	})
}
