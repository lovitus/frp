package net

import (
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
}
