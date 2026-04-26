package httpserver

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSecurityHeadersSetsFallbackHeaders(t *testing.T) {
	handler := securityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	for _, key := range []string{
		"Content-Security-Policy",
		"Referrer-Policy",
		"Permissions-Policy",
		"X-Content-Type-Options",
		"X-Frame-Options",
	} {
		if got := rec.Header().Get(key); got == "" {
			t.Fatalf("%s header is empty", key)
		}
	}
}

func TestSecurityHeadersDoesNotOverwriteExistingHeaders(t *testing.T) {
	handler := securityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'none'")
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get("Content-Security-Policy"); got != "default-src 'none'" {
		t.Fatalf("Content-Security-Policy = %q, want existing header", got)
	}
}
