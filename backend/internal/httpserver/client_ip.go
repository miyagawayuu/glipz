package httpserver

import (
	"context"
	"net"
	"net/http"
	"strings"
)

type directRemoteAddrContextKey struct{}

func captureDirectRemoteAddr(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), directRemoteAddrContextKey{}, strings.TrimSpace(r.RemoteAddr))
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func clientIPFromRemoteAddr(remoteAddr string) string {
	remoteAddr = strings.TrimSpace(remoteAddr)
	if host, _, err := net.SplitHostPort(remoteAddr); err == nil && host != "" {
		if ip := net.ParseIP(host); ip != nil {
			return ip.String()
		}
		return host
	}
	if ip := net.ParseIP(remoteAddr); ip != nil {
		return ip.String()
	}
	return remoteAddr
}

func directClientIP(r *http.Request) string {
	if raw, ok := r.Context().Value(directRemoteAddrContextKey{}).(string); ok && strings.TrimSpace(raw) != "" {
		return clientIPFromRemoteAddr(raw)
	}
	return clientIPFromRemoteAddr(r.RemoteAddr)
}

func trustedProxyClientIP(r *http.Request) string {
	if xri := strings.TrimSpace(r.Header.Get("X-Real-IP")); xri != "" {
		if ip := net.ParseIP(xri); ip != nil {
			return ip.String()
		}
	}
	if xff := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); xff != "" {
		if i := strings.IndexByte(xff, ','); i >= 0 {
			xff = strings.TrimSpace(xff[:i])
		}
		if ip := net.ParseIP(xff); ip != nil {
			return ip.String()
		}
	}
	return ""
}
