package httpserver

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5/middleware"

	"glipz.io/backend/internal/config"
)

func TestClientIPForAuthRateLimitIgnoresProxyHeadersWhenDisabled(t *testing.T) {
	s := &Server{cfg: config.Config{TrustProxyHeaders: false}}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "203.0.113.10:12345"
	req.Header.Set("X-Forwarded-For", "127.0.0.1")
	req.Header.Set("X-Real-IP", "127.0.0.1")

	got := s.clientIPForAuthRateLimit(req)
	if got != "203.0.113.10" {
		t.Fatalf("client IP = %q, want direct remote address", got)
	}
}

func TestClientIPForAuthRateLimitUsesProxyHeadersWhenEnabled(t *testing.T) {
	s := &Server{cfg: config.Config{TrustProxyHeaders: true}}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "203.0.113.10:12345"
	req.Header.Set("X-Forwarded-For", "198.51.100.25, 203.0.113.10")

	got := s.clientIPForAuthRateLimit(req)
	if got != "198.51.100.25" {
		t.Fatalf("client IP = %q, want first trusted proxy header IP", got)
	}
}

func TestClientIPForAuthRateLimitTrustsProxyHeadersOnlyFromCIDR(t *testing.T) {
	s := &Server{cfg: config.Config{TrustProxyHeaders: true, TrustedProxyCIDRs: []string{"203.0.113.0/24"}}}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "203.0.113.10:12345"
	req.Header.Set("X-Forwarded-For", "198.51.100.25")

	if got := s.clientIPForAuthRateLimit(req); got != "198.51.100.25" {
		t.Fatalf("trusted proxy client IP = %q, want forwarded IP", got)
	}

	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "192.0.2.10:12345"
	req.Header.Set("X-Forwarded-For", "198.51.100.25")
	if got := s.clientIPForAuthRateLimit(req); got != "192.0.2.10" {
		t.Fatalf("untrusted direct client IP = %q, want direct IP", got)
	}
}

func TestClientIPForFederationRLMatchesAuthRateLimitTrustedProxyCIDR(t *testing.T) {
	s := &Server{cfg: config.Config{TrustProxyHeaders: true, TrustedProxyCIDRs: []string{"203.0.113.0/24"}}}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "203.0.113.10:12345"
	req.Header.Set("X-Forwarded-For", "198.51.100.25")
	if got := s.clientIPForFederationRL(req); got != "198.51.100.25" {
		t.Fatalf("federation RL client IP = %q, want forwarded IP", got)
	}
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "192.0.2.10:12345"
	req.Header.Set("X-Forwarded-For", "198.51.100.25")
	if got := s.clientIPForFederationRL(req); got != "192.0.2.10" {
		t.Fatalf("federation RL client IP = %q, want direct IP when peer not in trusted CIDR", got)
	}
}

func TestDebugVarsUsesDirectRemoteAddrAfterRealIP(t *testing.T) {
	s := &Server{}
	handler := captureDirectRemoteAddr(middleware.RealIP(http.HandlerFunc(s.handleDebugVars)))

	req := httptest.NewRequest(http.MethodGet, "/debug/vars", nil)
	req.RemoteAddr = "203.0.113.10:12345"
	req.Header.Set("X-Forwarded-For", "127.0.0.1")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestDebugVarsAllowsDirectLoopback(t *testing.T) {
	s := &Server{}
	handler := captureDirectRemoteAddr(http.HandlerFunc(s.handleDebugVars))

	req := httptest.NewRequest(http.MethodGet, "/debug/vars", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}
