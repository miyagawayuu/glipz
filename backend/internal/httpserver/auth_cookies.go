package httpserver

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	authAccessCookieName = "glipz_access"
	authMFACookieName    = "glipz_mfa"
	csrfCookieName       = "glipz_csrf"
	csrfHeaderName       = "X-CSRF-Token"
)

func mfaJTIKey(jti string) string {
	return "auth:mfa:jti:" + strings.TrimSpace(jti)
}

func isHTTPSRequest(r *http.Request, trustProxyHeaders bool) bool {
	if r.TLS != nil {
		return true
	}
	if trustProxyHeaders && strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https") {
		return true
	}
	return false
}

func authCookieBase(r *http.Request, name, value string, maxAge int, ttl time.Duration, trustProxyHeaders bool, httpOnly bool) http.Cookie {
	return http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		Expires:  time.Now().Add(ttl),
		HttpOnly: httpOnly,
		Secure:   isHTTPSRequest(r, trustProxyHeaders),
		SameSite: http.SameSiteLaxMode,
	}
}

func (s *Server) setAccessCookie(w http.ResponseWriter, r *http.Request, token string, ttl time.Duration) {
	c := authCookieBase(r, authAccessCookieName, token, int(ttl.Seconds()), ttl, s.cfg.TrustProxyHeaders, true)
	http.SetCookie(w, &c)
}

func (s *Server) setMFACookie(w http.ResponseWriter, r *http.Request, token string, ttl time.Duration) {
	c := authCookieBase(r, authMFACookieName, token, int(ttl.Seconds()), ttl, s.cfg.TrustProxyHeaders, true)
	http.SetCookie(w, &c)
}

func (s *Server) setCSRFCookie(w http.ResponseWriter, r *http.Request, token string, ttl time.Duration) {
	c := authCookieBase(r, csrfCookieName, token, int(ttl.Seconds()), ttl, s.cfg.TrustProxyHeaders, false)
	http.SetCookie(w, &c)
}

func (s *Server) clearAuthCookies(w http.ResponseWriter, r *http.Request) {
	for _, name := range []string{authAccessCookieName, authMFACookieName, csrfCookieName} {
		c := authCookieBase(r, name, "", -1, -time.Hour, s.cfg.TrustProxyHeaders, name != csrfCookieName)
		c.Expires = time.Unix(0, 0)
		http.SetCookie(w, &c)
	}
}

func extractBearerToken(r *http.Request) (string, bool) {
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, "Bearer ") {
		return "", false
	}
	raw := strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
	return raw, raw != ""
}

func extractAccessCredential(r *http.Request) (raw string, fromCookie bool, ok bool) {
	if raw, ok := extractBearerToken(r); ok {
		return raw, false, true
	}
	c, err := r.Cookie(authAccessCookieName)
	if err != nil || strings.TrimSpace(c.Value) == "" {
		return "", false, false
	}
	return strings.TrimSpace(c.Value), true, true
}

func extractMFACredential(r *http.Request) (raw string, fromCookie bool, ok bool) {
	if raw, ok := extractBearerToken(r); ok {
		return raw, false, true
	}
	c, err := r.Cookie(authMFACookieName)
	if err != nil || strings.TrimSpace(c.Value) == "" {
		return "", false, false
	}
	return strings.TrimSpace(c.Value), true, true
}

func requiresCSRFCheck(r *http.Request) bool {
	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
		return false
	default:
		return true
	}
}

func csrfValid(r *http.Request) bool {
	c, err := r.Cookie(csrfCookieName)
	if err != nil {
		return false
	}
	cookieToken := strings.TrimSpace(c.Value)
	headerToken := strings.TrimSpace(r.Header.Get(csrfHeaderName))
	return cookieToken != "" && headerToken != "" && cookieToken == headerToken
}

func (s *Server) storeMFAJTI(ctx context.Context, jti string, ttl time.Duration) error {
	if s.rdb == nil || strings.TrimSpace(jti) == "" {
		return nil
	}
	return s.rdb.Set(ctx, mfaJTIKey(jti), "1", ttl).Err()
}

func (s *Server) consumeMFAJTI(ctx context.Context, jti string) (bool, error) {
	if s.rdb == nil || strings.TrimSpace(jti) == "" {
		return false, nil
	}
	_, err := s.rdb.GetDel(ctx, mfaJTIKey(jti)).Result()
	if err == redis.Nil {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}
